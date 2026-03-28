package workflows

import (
	"context"
	"fmt"
	"time"

	contextview "github.com/kobelakers/personal-cfo-os/internal/context"
	"github.com/kobelakers/personal-cfo-os/internal/governance"
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

type DebtVsInvestWorkflow struct {
	Intake            taskspec.DeterministicIntakeService
	QueryTransaction  tools.QueryTransactionTool
	QueryLiability    tools.QueryLiabilityTool
	QueryPortfolio    tools.QueryPortfolioTool
	ComputeMetrics    tools.ComputeDebtDecisionMetricsTool
	ArtifactTool      tools.GenerateTaskArtifactTool
	ReducerEngine     reducers.DeterministicReducerEngine
	ContextAssembler  contextview.ContextAssembler
	Planner           *planning.DeterministicPlanner
	Skill             skills.DebtOptimizationSkill
	CoverageChecker   verification.EvidenceCoverageChecker
	BusinessValidator verification.BusinessValidator
	SuccessChecker    verification.SuccessCriteriaChecker
	Oracle            verification.TrajectoryOracle
	RiskClassifier    governance.DefaultRiskClassifier
	ApprovalDecider   governance.ApprovalDecider
	ApprovalPolicy    governance.ApprovalPolicy
	ToolPolicy        *governance.ToolExecutionPolicy
	ArtifactProducer  ArtifactProducer
	Runtime           *runtimepkg.LocalWorkflowRuntime
	Now               func() time.Time
}

func (w DebtVsInvestWorkflow) Run(
	ctx context.Context,
	userID string,
	rawInput string,
	current state.FinancialWorldState,
) (DebtDecisionRunResult, error) {
	intake := w.Intake.Parse(rawInput)
	if !intake.Accepted || intake.TaskSpec == nil {
		return DebtDecisionRunResult{Intake: intake}, fmt.Errorf("task intake rejected: %s", intake.FailureReason)
	}
	spec := *intake.TaskSpec
	now := time.Now().UTC()
	if w.Now != nil {
		now = w.Now().UTC()
	}
	if current.UserID == "" {
		current.UserID = userID
	}
	workflowID := "workflow-debt-vs-invest-" + now.Format("20060102150405")
	execCtx := runtimepkg.ExecutionContext{
		WorkflowID:    workflowID,
		TaskID:        spec.ID,
		CorrelationID: workflowID,
		Attempt:       1,
	}
	localRuntime := w.Runtime
	if localRuntime == nil {
		localRuntime = &runtimepkg.LocalWorkflowRuntime{
			Controller:      runtimepkg.DefaultWorkflowController{},
			CheckpointStore: runtimepkg.NewInMemoryCheckpointStore(),
			Journal:         &runtimepkg.CheckpointJournal{},
			Timeline:        &runtimepkg.WorkflowTimeline{WorkflowID: workflowID, TraceID: workflowID},
			EventLog:        &observability.EventLog{},
			Now:             w.Now,
		}
	}

	input := observationInput(spec, userID)
	transactionEvidence, err := w.QueryTransaction.QueryEvidence(ctx, input)
	if err != nil {
		return DebtDecisionRunResult{}, err
	}
	liabilityEvidence, err := w.QueryLiability.QueryEvidence(ctx, input)
	if err != nil {
		return DebtDecisionRunResult{}, err
	}
	portfolioEvidence, err := w.QueryPortfolio.QueryEvidence(ctx, input)
	if err != nil {
		return DebtDecisionRunResult{}, err
	}
	evidence := dedupeEvidence(append(append(transactionEvidence, liabilityEvidence...), portfolioEvidence...))
	_, _, err = localRuntime.Checkpoint(execCtx, runtimepkg.WorkflowStatePlanning, runtimepkg.WorkflowStatePlanning, current.Version.Sequence, "decision evidence collected")
	if err != nil {
		return DebtDecisionRunResult{}, err
	}

	patch, err := w.ReducerEngine.BuildPatch(current, evidence, spec.ID, workflowID, "observed")
	if err != nil {
		return DebtDecisionRunResult{}, err
	}
	updatedState, diff, err := state.DefaultStateReducer{}.ApplyEvidencePatch(current, patch)
	if err != nil {
		return DebtDecisionRunResult{}, err
	}
	_, _, err = localRuntime.Checkpoint(execCtx, runtimepkg.WorkflowStateActing, runtimepkg.WorkflowStateActing, diff.ToVersion, "decision state updated")
	if err != nil {
		return DebtDecisionRunResult{}, err
	}

	assembler := w.ContextAssembler
	if assembler == nil {
		assembler = contextview.DefaultContextAssembler{}
	}
	planner := w.Planner
	if planner == nil {
		planner = &planning.DeterministicPlanner{}
	}
	planningContext, err := assembler.Assemble(spec, updatedState, nil, evidence, contextview.ContextViewPlanning)
	if err != nil {
		return DebtDecisionRunResult{}, err
	}
	plan := planner.CreatePlan(spec, planningContext, workflowID)

	skillOutput := w.Skill.Analyze(updatedState)
	riskAssessment := w.RiskClassifier.Classify(updatedState, "debt_vs_invest_recommendation")
	report := DebtDecisionReport{
		TaskID:           spec.ID,
		WorkflowID:       workflowID,
		Conclusion:       skillOutput.Conclusion,
		Reasons:          skillOutput.Reasons,
		Actions:          skillOutput.Actions,
		Metrics:          w.ComputeMetrics.Compute(updatedState),
		EvidenceIDs:      collectEvidenceIDs(evidence),
		ApprovalRequired: riskAssessment.Level == governance.ActionRiskHigh || riskAssessment.Level == governance.ActionRiskCritical,
		Confidence:       skillOutput.Confidence,
		GeneratedAt:      now,
	}

	artifactContent, err := w.ArtifactTool.Generate(report)
	if err != nil {
		return DebtDecisionRunResult{}, err
	}
	producer := w.ArtifactProducer
	if producer == nil {
		producer = StaticArtifactProducer{Now: w.Now}
	}
	reportArtifact := producer.ProduceArtifact(workflowID, spec.ID, ArtifactKindDebtDecisionReport, artifactContent, report.Conclusion, w.Skill.Name())

	coverageChecker := w.CoverageChecker
	if coverageChecker == nil {
		coverageChecker = verification.DefaultEvidenceCoverageChecker{}
	}
	businessValidator := w.BusinessValidator
	if businessValidator == nil {
		businessValidator = verification.DebtDecisionBusinessValidator{}
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
		return DebtDecisionRunResult{}, err
	}
	coverageResult := coverageToVerificationResult(spec, coverageReport)
	businessResult, err := businessValidator.Validate(ctx, spec, updatedState, evidence, report)
	if err != nil {
		return DebtDecisionRunResult{}, err
	}
	verificationResults := []verification.VerificationResult{coverageResult, businessResult}
	successResult, err := successChecker.Check(spec, verificationResults, report)
	if err != nil {
		return DebtDecisionRunResult{}, err
	}
	verificationResults = append(verificationResults, successResult)
	oracleVerdict, err := oracle.Evaluate(ctx, "debt-vs-invest", verificationResults)
	if err != nil {
		return DebtDecisionRunResult{}, err
	}

	approvalPolicy := w.ApprovalPolicy
	if approvalPolicy.Name == "" {
		approvalPolicy = governance.ApprovalPolicy{
			Name:          "debt-vs-invest-approval",
			MinRiskLevel:  governance.ActionRiskHigh,
			RequiredRoles: []string{"operator"},
			AutoApprove:   false,
		}
	}
	decision, audit, err := w.ApprovalDecider.Decide(governance.ActionRequest{
		Actor:         "governance_agent",
		ActorRoles:    []string{"analyst"},
		Action:        "debt_vs_invest_recommendation",
		Resource:      report.TaskID,
		RiskLevel:     riskAssessment.Level,
		CorrelationID: workflowID,
	}, approvalPolicy, w.ToolPolicy)
	if err != nil {
		return DebtDecisionRunResult{}, err
	}

	runtimeState := runtimepkg.WorkflowStateCompleted
	if needsReplan(verificationResults) {
		nextState, _, err := localRuntime.HandleFailure(execCtx, runtimepkg.WorkflowStateVerifying, runtimepkg.FailureCategoryValidation, "decision verification failed; workflow should replan")
		if err != nil {
			return DebtDecisionRunResult{}, err
		}
		runtimeState = nextState
	}
	if decision.Outcome == governance.PolicyDecisionRequireApproval {
		nextState, err := localRuntime.PauseForApproval(execCtx, runtimepkg.WorkflowStateVerifying, runtimepkg.HumanApprovalPending{
			ApprovalID:      workflowID + "-approval",
			WorkflowID:      workflowID,
			RequestedAction: "debt_vs_invest_recommendation",
			RequiredRoles:   approvalPolicy.RequiredRoles,
			RequestedAt:     now,
		})
		if err != nil {
			return DebtDecisionRunResult{}, err
		}
		runtimeState = nextState
	}

	return DebtDecisionRunResult{
		WorkflowID:       workflowID,
		Intake:           intake,
		TaskSpec:         spec,
		Plan:             plan,
		Evidence:         evidence,
		UpdatedState:     updatedState,
		Report:           report,
		Artifacts:        []WorkflowArtifact{reportArtifact},
		CoverageReport:   coverageReport,
		Verification:     verificationResults,
		Oracle:           oracleVerdict,
		RiskAssessment:   riskAssessment,
		ApprovalDecision: &decision,
		ApprovalAudit:    &audit,
		RuntimeState:     runtimeState,
	}, nil
}

func collectEvidenceIDs(records []observation.EvidenceRecord) []observation.EvidenceID {
	result := make([]observation.EvidenceID, 0, len(records))
	for _, record := range records {
		result = append(result, record.ID)
	}
	return result
}
