package agents

import (
	"fmt"
	"strings"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/analysis"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/protocol"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

type TaskGenerationAgentHandler struct{}

func (TaskGenerationAgentHandler) Name() string      { return RecipientTaskGenerationAgent }
func (TaskGenerationAgentHandler) Recipient() string { return RecipientTaskGenerationAgent }
func (TaskGenerationAgentHandler) RequestKind() protocol.MessageKind {
	return protocol.MessageKindTaskGenerationRequest
}

func (TaskGenerationAgentHandler) Handle(handlerCtx AgentHandlerContext, envelope protocol.AgentEnvelope) (AgentHandlerResult, error) {
	payload := envelope.Payload.TaskGenerationRequest
	if payload == nil {
		return AgentHandlerResult{}, &AgentExecutionError{
			Recipient: RecipientTaskGenerationAgent,
			Kind:      envelope.Kind,
			Failure:   protocol.AgentFailure{Category: protocol.AgentFailureBadPayload, Message: "task generation request payload is required"},
		}
	}

	eventKind, eventEvidenceIDs := classifyLifeEvent(payload.EventEvidence)
	if eventKind == "" {
		return AgentHandlerResult{}, &AgentExecutionError{
			Recipient: RecipientTaskGenerationAgent,
			Kind:      envelope.Kind,
			Failure:   protocol.AgentFailure{Category: protocol.AgentFailureValidation, Message: "task generation requires life event evidence"},
		}
	}
	if len(payload.ValidatedBlockResults) == 0 {
		return AgentHandlerResult{}, &AgentExecutionError{
			Recipient: RecipientTaskGenerationAgent,
			Kind:      envelope.Kind,
			Failure:   protocol.AgentFailure{Category: protocol.AgentFailureValidation, Message: "task generation requires validated block results"},
		}
	}

	now := time.Now().UTC()
	if handlerCtx.Now != nil {
		now = handlerCtx.Now().UTC()
	}
	generated, dependencies, suppressions := generateFollowUpTasks(
		envelope.Task,
		envelope.Metadata.CorrelationID,
		eventKind,
		eventEvidenceIDs,
		payload.Memories,
		payload.StateDiff,
		payload.ValidatedBlockResults,
		payload.EventEvidence,
		now,
	)
	graph := taskspec.TaskGraph{
		GraphID:           envelope.Metadata.CorrelationID + "-generated-tasks",
		ParentWorkflowID:  envelope.Metadata.CorrelationID,
		ParentTaskID:      envelope.Task.ID,
		TriggerSource:     taskspec.TaskTriggerSourceLifeEvent,
		GeneratedAt:       now,
		GeneratedTasks:    generated,
		Dependencies:      dependencies,
		SuppressionNotes:  suppressions,
		GenerationSummary: fmt.Sprintf("generated %d follow-up tasks for life event %q", len(generated), eventKind),
	}
	if err := graph.Validate(); err != nil {
		return AgentHandlerResult{}, &AgentExecutionError{
			Recipient: RecipientTaskGenerationAgent,
			Kind:      envelope.Kind,
			Failure:   protocol.AgentFailure{Category: protocol.AgentFailureValidation, Message: "generated task graph is invalid"},
			Cause:     err,
		}
	}
	return AgentHandlerResult{
		Kind: protocol.MessageKindTaskGenerationResult,
		Body: protocol.AgentResultBody{
			TaskGenerationResult: &protocol.TaskGenerationResultPayload{TaskGraph: graph},
		},
	}, nil
}

func classifyLifeEvent(records []observation.EvidenceRecord) (string, []string) {
	evidenceIDs := make([]string, 0, len(records))
	for _, record := range records {
		evidenceIDs = append(evidenceIDs, string(record.ID))
		for _, claim := range record.Claims {
			if claim.Predicate == "life_event_kind" {
				value := strings.Trim(claim.ValueJSON, "\"")
				if value != "" {
					return value, evidenceIDs
				}
			}
		}
	}
	return "", evidenceIDs
}

func generateFollowUpTasks(
	parent taskspec.TaskSpec,
	workflowID string,
	eventKind string,
	eventEvidenceIDs []string,
	memories []memory.MemoryRecord,
	diff state.StateDiff,
	blockResults []analysis.BlockResultEnvelope,
	triggerEvidence []observation.EvidenceRecord,
	now time.Time,
) ([]taskspec.GeneratedTaskSpec, []taskspec.TaskDependency, []string) {
	unique := make(map[taskspec.UserIntentType]taskspec.GeneratedTaskSpec)
	suppressions := make([]string, 0)
	deadlineEvidenceIDs, dueWindow := deriveDueWindow(triggerEvidence)
	memoryIDs := collectReasonMemoryIDs(memories, blockResults)
	hasCashflowBlock := hasBlockResult(blockResults, "cashflow")
	hasDebtBlock := hasBlockResult(blockResults, "debt")
	hasTaxBlock := hasBlockResult(blockResults, "tax")
	hasPortfolioBlock := hasBlockResult(blockResults, "portfolio")
	add := func(spec taskspec.TaskSpec, priority taskspec.TaskPriority, reasons []taskspec.TaskGenerationReason, dueWindow taskspec.TaskDueWindow, requiresApproval bool) {
		if shouldSuppressGeneratedTask(spec.UserIntentType, memories) {
			suppressions = append(suppressions, fmt.Sprintf("suppressed duplicate follow-up task for intent %q because retrieved memories indicate an existing recent decision", spec.UserIntentType))
			return
		}
		if _, exists := unique[spec.UserIntentType]; exists {
			suppressions = append(suppressions, fmt.Sprintf("suppressed duplicate follow-up task for intent %q inside the same generation pass", spec.UserIntentType))
			return
		}
		unique[spec.UserIntentType] = taskspec.GeneratedTaskSpec{
			Task: spec,
			Metadata: taskspec.GeneratedTaskMetadata{
				GeneratedAt:       now,
				ParentWorkflowID:  workflowID,
				ParentTaskID:      parent.ID,
				TriggerSource:     taskspec.TaskTriggerSourceLifeEvent,
				Priority:          priority,
				DueWindow:         dueWindow,
				RequiresApproval:  requiresApproval,
				GenerationReasons: reasons,
			},
		}
	}

	baseReasons := []taskspec.TaskGenerationReason{
		{
			Code:             taskspec.TaskGenerationReasonLifeEventImpact,
			Description:      "life event impact analysis produced a follow-up task",
			LifeEventKind:    eventKind,
			EvidenceIDs:      eventEvidenceIDs,
			MemoryIDs:        memoryIDs,
			StateDiffFields:  append([]string{}, diff.ChangedFields...),
			DeadlineEvidence: deadlineEvidenceIDs,
		},
	}
	switch eventKind {
	case "salary_change":
		if hasTaxBlock {
			add(buildGeneratedTaskSpec(parent, taskspec.UserIntentTaxOptimization, "Review tax withholding and tax-advantaged opportunities after salary change.", now), taskspec.TaskPriorityHigh, baseReasons, dueWindow, false)
		}
		if hasPortfolioBlock {
			add(buildGeneratedTaskSpec(parent, taskspec.UserIntentPortfolioRebalance, "Review whether salary change should trigger portfolio contribution or rebalance updates.", now), taskspec.TaskPriorityMedium, baseReasons, dueWindow, false)
		}
	case "new_child":
		if hasTaxBlock {
			add(buildGeneratedTaskSpec(parent, taskspec.UserIntentTaxOptimization, "Review childcare-related tax benefits and dependent updates after a new child event.", now), taskspec.TaskPriorityHigh, baseReasons, dueWindow, false)
		}
		if hasCashflowBlock {
			add(buildGeneratedTaskSpec(parent, taskspec.UserIntentMonthlyReview, "Run a focused monthly review after the new child event to re-check cashflow and childcare cost impact.", now), taskspec.TaskPriorityMedium, baseReasons, dueWindow, false)
		}
	case "job_change":
		if hasTaxBlock {
			add(buildGeneratedTaskSpec(parent, taskspec.UserIntentTaxOptimization, "Review withholding, benefits enrollment, and tax optimization after job change.", now), taskspec.TaskPriorityHigh, baseReasons, dueWindow, false)
		}
		if hasPortfolioBlock {
			add(buildGeneratedTaskSpec(parent, taskspec.UserIntentPortfolioRebalance, "Review portfolio contribution and allocation implications after job change.", now), taskspec.TaskPriorityMedium, baseReasons, dueWindow, false)
		}
	case "housing_change":
		if hasDebtBlock {
			add(buildGeneratedTaskSpec(parent, taskspec.UserIntentDebtVsInvest, "Re-check debt, liquidity, and investment tradeoffs after housing change.", now), taskspec.TaskPriorityHigh, baseReasons, dueWindow, true)
		}
		if hasPortfolioBlock {
			add(buildGeneratedTaskSpec(parent, taskspec.UserIntentPortfolioRebalance, "Review liquidity and allocation implications after housing change.", now), taskspec.TaskPriorityMedium, baseReasons, dueWindow, false)
		}
	}

	orderedIntents := []taskspec.UserIntentType{
		taskspec.UserIntentMonthlyReview,
		taskspec.UserIntentDebtVsInvest,
		taskspec.UserIntentTaxOptimization,
		taskspec.UserIntentPortfolioRebalance,
	}
	generated := make([]taskspec.GeneratedTaskSpec, 0, len(unique))
	for _, intent := range orderedIntents {
		item, ok := unique[intent]
		if ok {
			generated = append(generated, item)
		}
	}

	dependencies := make([]taskspec.TaskDependency, 0, 2)
	if taxTask, ok := unique[taskspec.UserIntentTaxOptimization]; ok {
		if portfolioTask, ok := unique[taskspec.UserIntentPortfolioRebalance]; ok {
			dependencies = append(dependencies, taskspec.TaskDependency{
				UpstreamTaskID:   taxTask.Task.ID,
				DownstreamTaskID: portfolioTask.Task.ID,
				Reason:           "portfolio follow-up should incorporate tax impact analysis first",
				Mandatory:        false,
			})
		}
	}
	return generated, dependencies, suppressions
}

func hasBlockResult(results []analysis.BlockResultEnvelope, domain string) bool {
	for _, item := range results {
		switch domain {
		case "cashflow":
			if item.Cashflow != nil {
				return true
			}
		case "debt":
			if item.Debt != nil {
				return true
			}
		case "tax":
			if item.Tax != nil {
				return true
			}
		case "portfolio":
			if item.Portfolio != nil {
				return true
			}
		}
	}
	return false
}

func collectReasonMemoryIDs(memories []memory.MemoryRecord, blockResults []analysis.BlockResultEnvelope) []string {
	seen := make(map[string]struct{}, len(memories))
	result := make([]string, 0, len(memories))
	for _, item := range blockResults {
		for _, id := range item.MemoryIDsUsed() {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			result = append(result, id)
		}
	}
	for _, item := range memories {
		if _, ok := seen[item.ID]; ok {
			continue
		}
		seen[item.ID] = struct{}{}
		result = append(result, item.ID)
		if len(result) >= 4 {
			break
		}
	}
	return result
}

func deriveDueWindow(records []observation.EvidenceRecord) ([]string, taskspec.TaskDueWindow) {
	deadlineEvidenceIDs := make([]string, 0)
	var earliest *time.Time
	for _, record := range records {
		if record.Type != observation.EvidenceTypeCalendarDeadline {
			continue
		}
		deadlineEvidenceIDs = append(deadlineEvidenceIDs, string(record.ID))
		if record.TimeRange.End == nil {
			continue
		}
		deadline := record.TimeRange.End.UTC()
		if earliest == nil || deadline.Before(*earliest) {
			copyDeadline := deadline
			earliest = &copyDeadline
		}
	}
	if earliest == nil {
		return deadlineEvidenceIDs, taskspec.TaskDueWindow{}
	}
	return deadlineEvidenceIDs, taskspec.TaskDueWindow{NotAfter: earliest}
}

func buildGeneratedTaskSpec(parent taskspec.TaskSpec, intent taskspec.UserIntentType, goal string, now time.Time) taskspec.TaskSpec {
	areas := []string{"cashflow"}
	switch intent {
	case taskspec.UserIntentTaxOptimization:
		areas = []string{"tax", "cashflow"}
	case taskspec.UserIntentPortfolioRebalance:
		areas = []string{"portfolio", "cashflow"}
	case taskspec.UserIntentDebtVsInvest:
		areas = []string{"liability", "cashflow", "portfolio"}
	case taskspec.UserIntentMonthlyReview:
		areas = []string{"cashflow", "liability", "portfolio", "tax"}
	}
	return taskspec.TaskSpec{
		ID:    fmt.Sprintf("task-follow-up-%s-%s", intent, now.Format("20060102150405")),
		Goal:  goal,
		Scope: taskspec.TaskScope{Areas: areas, Notes: []string{"generated_follow_up"}},
		Constraints: taskspec.ConstraintSet{
			Hard: []string{
				"must remain grounded in life event evidence and validated block results",
				"financial metrics must remain deterministic",
			},
		},
		RiskLevel: taskspec.RiskLevelMedium,
		SuccessCriteria: []taskspec.SuccessCriteria{
			{ID: "follow-up-grounding", Description: "follow-up task remains grounded in life event evidence, state diff, and validated analysis"},
		},
		RequiredEvidence: []taskspec.RequiredEvidenceRef{
			{Type: "event_signal", Reason: "follow-up task must remain grounded in triggering event", Mandatory: true},
		},
		ApprovalRequirement: parent.ApprovalRequirement,
		UserIntentType:      intent,
		CreatedAt:           now,
	}
}

func shouldSuppressGeneratedTask(intent taskspec.UserIntentType, memories []memory.MemoryRecord) bool {
	needle := ""
	switch intent {
	case taskspec.UserIntentDebtVsInvest:
		needle = "latest decision"
	case taskspec.UserIntentMonthlyReview:
		needle = "monthly review"
	default:
		return false
	}
	for _, memory := range memories {
		if strings.Contains(strings.ToLower(memory.Summary), needle) {
			return true
		}
		for _, fact := range memory.Facts {
			if strings.Contains(strings.ToLower(fact.Key), needle) || strings.Contains(strings.ToLower(fact.Value), needle) {
				return true
			}
		}
	}
	return false
}
