package workflows

import (
	"context"
	"fmt"
	"time"

	contextview "github.com/kobelakers/personal-cfo-os/internal/context"
	"github.com/kobelakers/personal-cfo-os/internal/governance"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/planning"
	runtimepkg "github.com/kobelakers/personal-cfo-os/internal/runtime"
	"github.com/kobelakers/personal-cfo-os/internal/skills"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
	"github.com/kobelakers/personal-cfo-os/internal/tools"
	"github.com/kobelakers/personal-cfo-os/internal/verification"
)

type MonthlyReviewWorkflow struct {
	Intake               taskspec.DeterministicIntakeService
	ReviewService        MonthlyReviewService
	MemoryService        memory.WorkflowMemoryService
	MemoryWritePolicy    governance.MemoryWritePolicy
	ContextAssembler     contextview.ContextAssembler
	Planner              *planning.DeterministicPlanner
	Skill                skills.MonthlyReviewSkill
	CashflowMetrics      tools.ComputeCashflowMetricsTool
	TaxSignals           tools.ComputeTaxSignalTool
	ArtifactService      ArtifactService
	VerificationPipeline verification.Pipeline
	ApprovalService      governance.ApprovalService
	Runtime              runtimepkg.WorkflowRuntime
	Now                  func() time.Time
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
	workflowRuntime := runtimepkg.ResolveWorkflowRuntime(w.Runtime, workflowID, w.Now)

	observed, err := w.ReviewService.ObserveAndReduce(ctx, spec, userID, workflowID, current)
	if err != nil {
		return MonthlyReviewRunResult{}, err
	}
	if _, _, err = workflowRuntime.Checkpoint(execCtx, runtimepkg.WorkflowStatePlanning, runtimepkg.WorkflowStatePlanning, current.Version.Sequence, "observation completed"); err != nil {
		return MonthlyReviewRunResult{}, err
	}
	if _, _, err = workflowRuntime.Checkpoint(execCtx, runtimepkg.WorkflowStateActing, runtimepkg.WorkflowStateActing, observed.Diff.ToVersion, "state updated from evidence patch"); err != nil {
		return MonthlyReviewRunResult{}, err
	}

	memoryResult, err := w.memoryService(workflowID).SyncMonthlyReview(ctx, spec, workflowID, observed.UpdatedState, observed.Evidence)
	if err != nil {
		return MonthlyReviewRunResult{}, err
	}

	assembler := w.contextAssembler()
	planningContext, err := assembler.Assemble(spec, observed.UpdatedState, memoryResult.Retrieved, observed.Evidence, contextview.ContextViewPlanning)
	if err != nil {
		return MonthlyReviewRunResult{}, err
	}
	planner := w.planner()
	plan := planner.CreatePlan(spec, planningContext, workflowID)
	if _, err = assembler.Assemble(spec, observed.UpdatedState, memoryResult.Retrieved, observed.Evidence, contextview.ContextViewExecution); err != nil {
		return MonthlyReviewRunResult{}, err
	}
	if _, _, err = workflowRuntime.Checkpoint(execCtx, runtimepkg.WorkflowStateActing, runtimepkg.WorkflowStateActing, observed.UpdatedState.Version.Sequence, "execution context assembled"); err != nil {
		return MonthlyReviewRunResult{}, err
	}

	skillOutput := w.Skill.Generate(observed.UpdatedState, observed.Evidence)
	report := MonthlyReviewReport{
		TaskID:                  spec.ID,
		WorkflowID:              workflowID,
		Summary:                 skillOutput.Summary,
		CashflowMetrics:         w.CashflowMetrics.Compute(observed.UpdatedState),
		TaxSignals:              w.TaxSignals.Compute(observed.UpdatedState),
		RiskItems:               skillOutput.RiskItems,
		OptimizationSuggestions: skillOutput.Suggestions,
		TodoItems:               skillOutput.TodoItems,
		ApprovalRequired:        observed.UpdatedState.RiskState.OverallRisk == "high",
		Confidence:              skillOutput.Confidence,
		GeneratedAt:             now,
	}

	artifactService := w.artifacts()
	reportArtifact, err := artifactService.Produce(workflowID, spec.ID, ArtifactKindMonthlyReviewReport, report, report.Summary, w.Skill.Name())
	if err != nil {
		return MonthlyReviewRunResult{}, err
	}

	if _, err = assembler.Assemble(spec, observed.UpdatedState, memoryResult.Retrieved, observed.Evidence, contextview.ContextViewVerification); err != nil {
		return MonthlyReviewRunResult{}, err
	}
	verificationResult, err := w.verificationPipeline().VerifyMonthlyReview(ctx, spec, observed.UpdatedState, observed.Evidence, report)
	if err != nil {
		return MonthlyReviewRunResult{}, err
	}

	runtimeState := runtimepkg.WorkflowStateCompleted
	shouldReplan := verification.NeedsReplan(verificationResult.Results)
	if shouldReplan {
		nextState, _, err := workflowRuntime.HandleFailure(execCtx, runtimepkg.WorkflowStateVerifying, runtimepkg.FailureCategoryValidation, "verification failed; workflow should replan")
		if err != nil {
			return MonthlyReviewRunResult{}, err
		}
		runtimeState = nextState
	}

	var approvalDecision *governance.PolicyDecision
	var approvalAudit *governance.AuditEvent
	approvalService := w.approvals()
	approvalEvaluation, err := approvalService.EvaluateAction(observed.UpdatedState, workflowID, "monthly_review_report", report.TaskID, "governance_agent", []string{"analyst"}, report.ApprovalRequired)
	if err != nil {
		return MonthlyReviewRunResult{}, err
	}
	if approvalEvaluation.Decision != nil {
		approvalDecision = approvalEvaluation.Decision
	}
	if approvalEvaluation.Audit != nil {
		approvalAudit = approvalEvaluation.Audit
	}

	if !shouldReplan {
		reportEvaluation, err := approvalService.EvaluateReport(workflowID, "report_agent", "user", false)
		if err != nil {
			return MonthlyReviewRunResult{}, err
		}
		if reportEvaluation.Decision.Outcome == governance.PolicyDecisionRedact {
			report.Summary = "[REDACTED] " + report.Summary
		}
		if approvalDecision != nil && approvalDecision.Outcome == governance.PolicyDecisionRequireApproval {
			nextState, err := workflowRuntime.PauseForApproval(execCtx, runtimepkg.WorkflowStateVerifying, runtimepkg.HumanApprovalPending{
				ApprovalID:      workflowID + "-approval",
				WorkflowID:      workflowID,
				RequestedAction: "monthly_review_report",
				RequiredRoles:   approvalService.ApprovalPolicy.RequiredRoles,
				RequestedAt:     now,
			})
			if err != nil {
				return MonthlyReviewRunResult{}, err
			}
			runtimeState = nextState
			report.ApprovalRequired = true
		}
	}

	return MonthlyReviewRunResult{
		WorkflowID:        workflowID,
		Intake:            intake,
		TaskSpec:          spec,
		Plan:              plan,
		Evidence:          observed.Evidence,
		UpdatedState:      observed.UpdatedState,
		Report:            report,
		Artifacts:         []WorkflowArtifact{reportArtifact},
		GeneratedMemories: memoryResult.GeneratedIDs,
		CoverageReport:    verificationResult.CoverageReport,
		Verification:      verificationResult.Results,
		Oracle:            verificationResult.OracleVerdict,
		RiskAssessment:    approvalEvaluation.RiskAssessment,
		ApprovalDecision:  approvalDecision,
		ApprovalAudit:     approvalAudit,
		RuntimeState:      runtimeState,
	}, nil
}

func (w MonthlyReviewWorkflow) contextAssembler() contextview.ContextAssembler {
	if w.ContextAssembler != nil {
		return w.ContextAssembler
	}
	return contextview.DefaultContextAssembler{}
}

func (w MonthlyReviewWorkflow) planner() *planning.DeterministicPlanner {
	if w.Planner != nil {
		return w.Planner
	}
	return &planning.DeterministicPlanner{}
}

func (w MonthlyReviewWorkflow) artifacts() ArtifactService {
	service := w.ArtifactService
	if service.Now == nil {
		service.Now = w.Now
	}
	return service
}

func (w MonthlyReviewWorkflow) verificationPipeline() verification.Pipeline {
	return w.VerificationPipeline
}

func (w MonthlyReviewWorkflow) approvals() governance.ApprovalService {
	return w.ApprovalService
}

func (w MonthlyReviewWorkflow) memoryService(workflowID string) memory.WorkflowMemoryService {
	service := w.MemoryService
	if service.Now == nil {
		service.Now = w.Now
	}
	if service.Gate == nil {
		service.Gate = governance.MemoryWriteGateService{
			PolicyEngine:  w.ApprovalService.PolicyEngine,
			Policy:        w.memoryWritePolicy(),
			CorrelationID: workflowID,
		}
	}
	return service
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

func (w MonthlyReviewWorkflow) now() time.Time {
	if w.Now != nil {
		return w.Now().UTC()
	}
	return time.Now().UTC()
}
