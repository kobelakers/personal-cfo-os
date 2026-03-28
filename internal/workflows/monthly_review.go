package workflows

import (
	"context"
	"fmt"
	"strings"
	"time"

	contextview "github.com/kobelakers/personal-cfo-os/internal/context"
	"github.com/kobelakers/personal-cfo-os/internal/governance"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/observability"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/planning"
	"github.com/kobelakers/personal-cfo-os/internal/reducers"
	runtimepkg "github.com/kobelakers/personal-cfo-os/internal/runtime"
	"github.com/kobelakers/personal-cfo-os/internal/skills"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
	"github.com/kobelakers/personal-cfo-os/internal/tools"
	"github.com/kobelakers/personal-cfo-os/internal/verification"
)

type MonthlyReviewWorkflow struct {
	Intake                 taskspec.DeterministicIntakeService
	QueryTransaction       tools.QueryTransactionTool
	QueryLiability         tools.QueryLiabilityTool
	QueryPortfolio         tools.QueryPortfolioTool
	ParseDocument          tools.ParseDocumentTool
	CashflowMetrics        tools.ComputeCashflowMetricsTool
	TaxSignals             tools.ComputeTaxSignalTool
	ArtifactTool           tools.GenerateTaskArtifactTool
	ReducerEngine          reducers.DeterministicReducerEngine
	MemoryWriter           memory.MemoryWriter
	MemoryRetriever        memory.HybridRetriever
	ContextAssembler       contextview.ContextAssembler
	Planner                *planning.DeterministicPlanner
	Skill                  skills.MonthlyReviewSkill
	ArtifactProducer       ArtifactProducer
	CoverageChecker        verification.EvidenceCoverageChecker
	DeterministicValidator verification.DeterministicValidator
	BusinessValidator      verification.BusinessValidator
	SuccessChecker         verification.SuccessCriteriaChecker
	Oracle                 verification.TrajectoryOracle
	RiskClassifier         governance.DefaultRiskClassifier
	ApprovalDecider        governance.ApprovalDecider
	PolicyEngine           governance.StaticPolicyEngine
	ApprovalPolicy         governance.ApprovalPolicy
	ToolPolicy             *governance.ToolExecutionPolicy
	MemoryWritePolicy      governance.MemoryWritePolicy
	ReportPolicy           governance.ReportDisclosurePolicy
	Runtime                *runtimepkg.LocalWorkflowRuntime
	Now                    func() time.Time
}

func (w MonthlyReviewWorkflow) Run(
	ctx context.Context,
	userID string,
	rawInput string,
	current state.FinancialWorldState,
) (MonthlyReviewRunResult, error) {
	intake := w.Intake.Parse(rawInput)
	if !intake.Accepted || intake.TaskSpec == nil {
		return MonthlyReviewRunResult{Intake: intake}, fmt.Errorf("task intake rejected: %s", intake.FailureReason)
	}
	spec := *intake.TaskSpec
	now := w.now()
	if current.UserID == "" {
		current.UserID = userID
	}

	workflowID := "workflow-monthly-review-" + now.Format("20060102150405")
	execCtx := runtimepkg.ExecutionContext{
		WorkflowID:    workflowID,
		TaskID:        spec.ID,
		CorrelationID: workflowID,
		Attempt:       1,
	}
	localRuntime := w.runtime(workflowID)

	input := observationInput(spec, userID)
	transactionEvidence, err := w.QueryTransaction.QueryEvidence(ctx, input)
	if err != nil {
		return MonthlyReviewRunResult{}, err
	}
	liabilityEvidence, err := w.QueryLiability.QueryEvidence(ctx, input)
	if err != nil {
		return MonthlyReviewRunResult{}, err
	}
	portfolioEvidence, err := w.QueryPortfolio.QueryEvidence(ctx, input)
	if err != nil {
		return MonthlyReviewRunResult{}, err
	}
	documentEvidence, err := w.ParseDocument.ParseEvidence(ctx, input)
	if err != nil {
		return MonthlyReviewRunResult{}, err
	}
	evidence := dedupeEvidence(append(append(append(transactionEvidence, liabilityEvidence...), portfolioEvidence...), documentEvidence...))
	_, _, err = localRuntime.Checkpoint(execCtx, runtimepkg.WorkflowStatePlanning, runtimepkg.WorkflowStatePlanning, current.Version.Sequence, "observation completed")
	if err != nil {
		return MonthlyReviewRunResult{}, err
	}

	patch, err := w.ReducerEngine.BuildPatch(current, evidence, spec.ID, workflowID, "observed")
	if err != nil {
		return MonthlyReviewRunResult{}, err
	}
	updatedState, diff, err := state.DefaultStateReducer{}.ApplyEvidencePatch(current, patch)
	if err != nil {
		return MonthlyReviewRunResult{}, err
	}
	_, _, err = localRuntime.Checkpoint(execCtx, runtimepkg.WorkflowStateActing, runtimepkg.WorkflowStateActing, diff.ToVersion, "state updated from evidence patch")
	if err != nil {
		return MonthlyReviewRunResult{}, err
	}

	memoryIDs, err := w.writeDerivedMemories(ctx, spec, workflowID, updatedState, evidence, now)
	if err != nil {
		return MonthlyReviewRunResult{}, err
	}
	retrievedMemories, err := w.retrieveWorkflowMemories(ctx, spec)
	if err != nil {
		return MonthlyReviewRunResult{}, err
	}

	assembler := w.ContextAssembler
	if assembler == nil {
		assembler = contextview.DefaultContextAssembler{}
	}
	planner := w.Planner
	if planner == nil {
		planner = &planning.DeterministicPlanner{}
	}
	planningContext, err := assembler.Assemble(spec, updatedState, retrievedMemories, evidence, contextview.ContextViewPlanning)
	if err != nil {
		return MonthlyReviewRunResult{}, err
	}
	plan := planner.CreatePlan(spec, planningContext, workflowID)
	_, err = assembler.Assemble(spec, updatedState, retrievedMemories, evidence, contextview.ContextViewExecution)
	if err != nil {
		return MonthlyReviewRunResult{}, err
	}
	_, _, err = localRuntime.Checkpoint(execCtx, runtimepkg.WorkflowStateActing, runtimepkg.WorkflowStateActing, updatedState.Version.Sequence, "execution context assembled")
	if err != nil {
		return MonthlyReviewRunResult{}, err
	}

	skillOutput := w.Skill.Generate(updatedState, evidence)
	report := MonthlyReviewReport{
		TaskID:                  spec.ID,
		WorkflowID:              workflowID,
		Summary:                 skillOutput.Summary,
		CashflowMetrics:         w.CashflowMetrics.Compute(updatedState),
		TaxSignals:              w.TaxSignals.Compute(updatedState),
		RiskItems:               skillOutput.RiskItems,
		OptimizationSuggestions: skillOutput.Suggestions,
		TodoItems:               skillOutput.TodoItems,
		ApprovalRequired:        updatedState.RiskState.OverallRisk == "high",
		Confidence:              skillOutput.Confidence,
		GeneratedAt:             now,
	}

	artifactContent, err := w.ArtifactTool.Generate(report)
	if err != nil {
		return MonthlyReviewRunResult{}, err
	}
	producer := w.ArtifactProducer
	if producer == nil {
		producer = StaticArtifactProducer{Now: w.Now}
	}
	reportArtifact := producer.ProduceArtifact(workflowID, spec.ID, ArtifactKindMonthlyReviewReport, artifactContent, report.Summary, w.Skill.Name())

	coverageChecker := w.CoverageChecker
	if coverageChecker == nil {
		coverageChecker = verification.DefaultEvidenceCoverageChecker{}
	}
	deterministicValidator := w.DeterministicValidator
	if deterministicValidator == nil {
		deterministicValidator = verification.MonthlyReviewDeterministicValidator{}
	}
	businessValidator := w.BusinessValidator
	if businessValidator == nil {
		businessValidator = verification.MonthlyReviewBusinessValidator{}
	}
	successChecker := w.SuccessChecker
	if successChecker == nil {
		successChecker = verification.DefaultSuccessCriteriaChecker{}
	}
	oracle := w.Oracle
	if oracle == nil {
		oracle = verification.BaselineTrajectoryOracle{}
	}

	coverageReport, err := coverageChecker.Check(spec, evidence)
	if err != nil {
		return MonthlyReviewRunResult{}, err
	}
	coverageResult := coverageToVerificationResult(spec, coverageReport)
	deterministicResult, err := deterministicValidator.Validate(ctx, spec, updatedState, evidence, report)
	if err != nil {
		return MonthlyReviewRunResult{}, err
	}
	businessResult, err := businessValidator.Validate(ctx, spec, updatedState, evidence, report)
	if err != nil {
		return MonthlyReviewRunResult{}, err
	}
	verificationResults := []verification.VerificationResult{coverageResult, deterministicResult, businessResult}
	successResult, err := successChecker.Check(spec, verificationResults, report)
	if err != nil {
		return MonthlyReviewRunResult{}, err
	}
	verificationResults = append(verificationResults, successResult)
	oracleVerdict, err := oracle.Evaluate(ctx, "monthly-review", verificationResults)
	if err != nil {
		return MonthlyReviewRunResult{}, err
	}

	runtimeState := runtimepkg.WorkflowStateCompleted
	shouldReplan := needsReplan(verificationResults)
	if shouldReplan {
		nextState, _, err := localRuntime.HandleFailure(execCtx, runtimepkg.WorkflowStateVerifying, runtimepkg.FailureCategoryValidation, "verification failed; workflow should replan")
		if err != nil {
			return MonthlyReviewRunResult{}, err
		}
		runtimeState = nextState
	}

	riskAssessment := w.RiskClassifier.Classify(updatedState, "monthly_review_report")
	var decision *governance.PolicyDecision
	var audit *governance.AuditEvent
	if !shouldReplan {
		decision, audit, err = w.evaluateApproval(workflowID, report, riskAssessment)
		if err != nil {
			return MonthlyReviewRunResult{}, err
		}
		if decision != nil && decision.Outcome == governance.PolicyDecisionRequireApproval {
			pending := runtimepkg.HumanApprovalPending{
				ApprovalID:      workflowID + "-approval",
				WorkflowID:      workflowID,
				RequestedAction: "monthly_review_report",
				RequiredRoles:   w.ApprovalPolicy.RequiredRoles,
				RequestedAt:     now,
			}
			nextState, err := localRuntime.PauseForApproval(execCtx, runtimepkg.WorkflowStateVerifying, pending)
			if err != nil {
				return MonthlyReviewRunResult{}, err
			}
			runtimeState = nextState
			report.ApprovalRequired = true
		}
	}

	reportDecision, _, err := w.PolicyEngine.EvaluateReport(governance.ReportRequest{
		Actor:         "report_agent",
		Audience:      "user",
		ContainsPII:   false,
		CorrelationID: workflowID,
	}, w.reportPolicy())
	if err != nil {
		return MonthlyReviewRunResult{}, err
	}
	if reportDecision.Outcome == governance.PolicyDecisionRedact {
		report.Summary = "[REDACTED] " + report.Summary
	}
	return MonthlyReviewRunResult{
		WorkflowID:        workflowID,
		Intake:            intake,
		TaskSpec:          spec,
		Plan:              plan,
		Evidence:          evidence,
		UpdatedState:      updatedState,
		Report:            report,
		Artifacts:         []WorkflowArtifact{reportArtifact},
		GeneratedMemories: memoryIDs,
		CoverageReport:    coverageReport,
		Verification:      verificationResults,
		Oracle:            oracleVerdict,
		RiskAssessment:    riskAssessment,
		ApprovalDecision:  decision,
		ApprovalAudit:     audit,
		RuntimeState:      runtimeState,
	}, nil
}

func (w MonthlyReviewWorkflow) writeDerivedMemories(
	ctx context.Context,
	spec taskspec.TaskSpec,
	workflowID string,
	current state.FinancialWorldState,
	evidence []observation.EvidenceRecord,
	now time.Time,
) ([]string, error) {
	if w.MemoryWriter == nil {
		return nil, nil
	}
	policy := w.memoryWritePolicy()
	records := deriveMonthlyReviewMemories(spec, workflowID, current, evidence, now)
	ids := make([]string, 0, len(records))
	for _, record := range records {
		decision, _, err := w.PolicyEngine.EvaluateMemoryWrite(record, policy, workflowID)
		if err != nil {
			return nil, err
		}
		if decision.Outcome != governance.PolicyDecisionAllow {
			return nil, fmt.Errorf("memory write denied for %s: %s", record.ID, decision.Reason)
		}
		if err := w.MemoryWriter.Write(ctx, record); err != nil {
			return nil, err
		}
		ids = append(ids, record.ID)
	}
	return ids, nil
}

func (w MonthlyReviewWorkflow) retrieveWorkflowMemories(ctx context.Context, spec taskspec.TaskSpec) ([]memory.MemoryRecord, error) {
	if w.MemoryRetriever == nil {
		return nil, nil
	}
	results, err := w.MemoryRetriever.Retrieve(ctx, memory.RetrievalQuery{
		Text:         spec.Goal,
		LexicalTerms: spec.Scope.Areas,
		SemanticHint: "monthly financial review, risk signals, optimization suggestions",
		TopK:         4,
	})
	if err != nil {
		return nil, err
	}
	memories := make([]memory.MemoryRecord, 0, len(results))
	for _, result := range results {
		memories = append(memories, result.Memory)
	}
	return memories, nil
}

func (w MonthlyReviewWorkflow) runtime(workflowID string) *runtimepkg.LocalWorkflowRuntime {
	if w.Runtime != nil {
		return w.Runtime
	}
	return &runtimepkg.LocalWorkflowRuntime{
		Controller:      runtimepkg.DefaultWorkflowController{},
		CheckpointStore: runtimepkg.NewInMemoryCheckpointStore(),
		Journal:         &runtimepkg.CheckpointJournal{},
		Timeline:        &runtimepkg.WorkflowTimeline{WorkflowID: workflowID, TraceID: workflowID},
		EventLog:        &observability.EventLog{},
		Now:             w.Now,
	}
}

func (w MonthlyReviewWorkflow) memoryWritePolicy() governance.MemoryWritePolicy {
	if w.MemoryWritePolicy.MinConfidence != 0 || w.MemoryWritePolicy.RequireEvidence || len(w.MemoryWritePolicy.AllowKinds) > 0 {
		return w.MemoryWritePolicy
	}
	return governance.MemoryWritePolicy{
		MinConfidence:   0.7,
		RequireEvidence: false,
		AllowKinds: []memory.MemoryKind{
			memory.MemoryKindEpisodic,
			memory.MemoryKindSemantic,
			memory.MemoryKindProcedural,
			memory.MemoryKindPolicy,
		},
	}
}

func (w MonthlyReviewWorkflow) reportPolicy() governance.ReportDisclosurePolicy {
	if w.ReportPolicy.Audience != "" {
		return w.ReportPolicy
	}
	return governance.ReportDisclosurePolicy{
		Audience: "user",
		AllowPII: false,
	}
}

func (w MonthlyReviewWorkflow) evaluateApproval(
	workflowID string,
	report MonthlyReviewReport,
	riskAssessment governance.RiskAssessment,
) (*governance.PolicyDecision, *governance.AuditEvent, error) {
	approvalPolicy := w.ApprovalPolicy
	if approvalPolicy.Name == "" {
		approvalPolicy = governance.ApprovalPolicy{
			Name:          "monthly-review-approval",
			MinRiskLevel:  governance.ActionRiskHigh,
			RequiredRoles: []string{"operator"},
			AutoApprove:   false,
		}
	}
	request := governance.ActionRequest{
		Actor:         "governance_agent",
		ActorRoles:    []string{"analyst"},
		Action:        "monthly_review_report",
		Resource:      report.TaskID,
		RiskLevel:     riskAssessment.Level,
		CorrelationID: workflowID,
	}
	decision, audit, err := w.ApprovalDecider.Decide(request, approvalPolicy, w.ToolPolicy)
	if err != nil {
		return nil, nil, err
	}
	if report.ApprovalRequired && decision.Outcome == governance.PolicyDecisionAllow {
		decision.Outcome = governance.PolicyDecisionRequireApproval
		decision.Reason = "report flagged approval_required from workflow risk assessment"
		audit.Outcome = string(governance.PolicyDecisionRequireApproval)
		audit.Reason = decision.Reason
	}
	return &decision, &audit, nil
}

func deriveMonthlyReviewMemories(
	spec taskspec.TaskSpec,
	workflowID string,
	current state.FinancialWorldState,
	evidence []observation.EvidenceRecord,
	now time.Time,
) []memory.MemoryRecord {
	records := make([]memory.MemoryRecord, 0, 5)
	if current.BehaviorState.DuplicateSubscriptionCount > 0 {
		records = append(records, memory.MemoryRecord{
			ID:      workflowID + "-memory-subscriptions",
			Kind:    memory.MemoryKindSemantic,
			Summary: "User has recurring subscriptions that should be reviewed during monthly review.",
			Facts: []memory.MemoryFact{
				{Key: "duplicate_subscription_count", Value: fmt.Sprintf("%d", current.BehaviorState.DuplicateSubscriptionCount)},
			},
			Source: memory.MemorySource{
				EvidenceIDs: evidenceIDs(evidence, "duplicate_subscription_count"),
				TaskID:      spec.ID,
				WorkflowID:  workflowID,
				Actor:       "memory_steward",
			},
			Confidence: memory.MemoryConfidence{Score: 0.88, Rationale: "derived from recurring subscription evidence"},
			CreatedAt:  now,
			UpdatedAt:  now,
		})
	}
	if current.BehaviorState.LateNightSpendingFrequency > 0 {
		records = append(records, memory.MemoryRecord{
			ID:      workflowID + "-memory-late-night",
			Kind:    memory.MemoryKindEpisodic,
			Summary: "A late-night spending pattern was observed in the current review window.",
			Facts: []memory.MemoryFact{
				{Key: "late_night_spending_frequency", Value: fmt.Sprintf("%.4f", current.BehaviorState.LateNightSpendingFrequency)},
			},
			Source: memory.MemorySource{
				EvidenceIDs: evidenceIDsByType(evidence, "late_night_spending_signal"),
				TaskID:      spec.ID,
				WorkflowID:  workflowID,
				Actor:       "memory_steward",
			},
			Confidence: memory.MemoryConfidence{Score: 0.72, Rationale: "derived from late-night spending signal"},
			CreatedAt:  now,
			UpdatedAt:  now,
		})
	}
	if current.LiabilityState.DebtBurdenRatio > 0 {
		records = append(records, memory.MemoryRecord{
			ID:      workflowID + "-memory-debt-pressure",
			Kind:    memory.MemoryKindSemantic,
			Summary: "Monthly review observed current debt pressure and minimum payment load.",
			Facts: []memory.MemoryFact{
				{Key: "debt_burden_ratio", Value: fmt.Sprintf("%.4f", current.LiabilityState.DebtBurdenRatio)},
				{Key: "minimum_payment_pressure", Value: fmt.Sprintf("%.4f", current.LiabilityState.MinimumPaymentPressure)},
			},
			Source: memory.MemorySource{
				EvidenceIDs: evidenceIDsByType(evidence, "debt_obligation_snapshot"),
				TaskID:      spec.ID,
				WorkflowID:  workflowID,
				Actor:       "memory_steward",
			},
			Confidence: memory.MemoryConfidence{Score: 0.9, Rationale: "derived from debt obligation snapshot"},
			CreatedAt:  now,
			UpdatedAt:  now,
		})
	}
	if current.TaxState.ChildcareTaxSignal {
		facts := []memory.MemoryFact{
			{Key: "childcare_tax_signal", Value: "true"},
		}
		if notes := strings.Join(current.TaxState.FamilyTaxNotes, "; "); strings.TrimSpace(notes) != "" {
			facts = append(facts, memory.MemoryFact{Key: "family_tax_notes", Value: notes})
		}
		records = append(records, memory.MemoryRecord{
			ID:      workflowID + "-memory-tax-signal",
			Kind:    memory.MemoryKindSemantic,
			Summary: "Family-related tax optimization signal was present during the review.",
			Facts:   facts,
			Source: memory.MemorySource{
				EvidenceIDs: evidenceIDsByType(evidence, "payslip_statement", "tax_document"),
				TaskID:      spec.ID,
				WorkflowID:  workflowID,
				Actor:       "memory_steward",
			},
			Confidence: memory.MemoryConfidence{Score: 0.84, Rationale: "consistent payroll and tax document signal"},
			CreatedAt:  now,
			UpdatedAt:  now,
		})
	}
	records = append(records, memory.MemoryRecord{
		ID:      workflowID + "-memory-procedure",
		Kind:    memory.MemoryKindProcedural,
		Summary: "Monthly review should always cover cashflow, debt, portfolio, tax, behavior, and risk blocks.",
		Facts: []memory.MemoryFact{
			{Key: "monthly_review_checklist", Value: "cashflow,debt,portfolio,tax,behavior,risk"},
		},
		Source: memory.MemorySource{
			TaskID:     spec.ID,
			WorkflowID: workflowID,
			Actor:      "memory_steward",
		},
		Confidence: memory.MemoryConfidence{Score: 0.95, Rationale: "workflow-generated procedural memory"},
		CreatedAt:  now,
		UpdatedAt:  now,
	})
	return records
}

func observationInput(spec taskspec.TaskSpec, userID string) map[string]string {
	input := map[string]string{
		"task_id": spec.ID,
		"user_id": userID,
	}
	if spec.Scope.Start != nil {
		input["start"] = spec.Scope.Start.UTC().Format(time.RFC3339)
	}
	if spec.Scope.End != nil {
		input["end"] = spec.Scope.End.UTC().Format(time.RFC3339)
	}
	return input
}

func dedupeEvidence(records []observation.EvidenceRecord) []observation.EvidenceRecord {
	seen := make(map[observation.EvidenceID]struct{}, len(records))
	result := make([]observation.EvidenceRecord, 0, len(records))
	for _, record := range records {
		if _, ok := seen[record.ID]; ok {
			continue
		}
		seen[record.ID] = struct{}{}
		result = append(result, record)
	}
	return result
}

func coverageToVerificationResult(spec taskspec.TaskSpec, report verification.EvidenceCoverageReport) verification.VerificationResult {
	status := verification.VerificationStatusPass
	message := "required evidence coverage satisfied"
	for i, item := range report.Items {
		if !item.Covered && i < len(spec.RequiredEvidence) && spec.RequiredEvidence[i].Mandatory {
			status = verification.VerificationStatusNeedsReplan
			message = "mandatory evidence missing: " + item.RequirementID
			break
		}
	}
	return verification.VerificationResult{
		Status:           status,
		Validator:        "evidence_coverage_checker",
		Message:          message,
		EvidenceCoverage: report,
		CheckedAt:        time.Now().UTC(),
	}
}

func needsReplan(results []verification.VerificationResult) bool {
	for _, result := range results {
		if result.Status == verification.VerificationStatusFail || result.Status == verification.VerificationStatusNeedsReplan {
			return true
		}
	}
	return false
}

func evidenceIDs(records []observation.EvidenceRecord, predicate string) []observation.EvidenceID {
	ids := make([]observation.EvidenceID, 0)
	for _, record := range records {
		for _, claim := range record.Claims {
			if claim.Predicate == predicate {
				ids = append(ids, record.ID)
				break
			}
		}
	}
	return ids
}

func evidenceIDsByType(records []observation.EvidenceRecord, types ...string) []observation.EvidenceID {
	allowed := make(map[string]struct{}, len(types))
	for _, evidenceType := range types {
		allowed[evidenceType] = struct{}{}
	}
	ids := make([]observation.EvidenceID, 0)
	for _, record := range records {
		if _, ok := allowed[string(record.Type)]; ok {
			ids = append(ids, record.ID)
		}
	}
	return ids
}

func (w MonthlyReviewWorkflow) now() time.Time {
	if w.Now != nil {
		return w.Now().UTC()
	}
	return time.Now().UTC()
}
