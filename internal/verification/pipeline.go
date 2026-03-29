package verification

import (
	"context"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/analysis"
	contextview "github.com/kobelakers/personal-cfo-os/internal/context"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/planning"
	"github.com/kobelakers/personal-cfo-os/internal/reporting"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
)

type Pipeline struct {
	CoverageChecker        EvidenceCoverageChecker
	DeterministicValidator DeterministicValidator
	BusinessValidator      BusinessValidator
	SuccessChecker         SuccessCriteriaChecker
	Oracle                 TrajectoryOracle
	Now                    func() time.Time
}

type PipelineResult struct {
	CoverageReport EvidenceCoverageReport          `json:"coverage_report"`
	BlockResults   map[string][]VerificationResult `json:"block_results,omitempty"`
	FinalResults   []VerificationResult            `json:"final_results,omitempty"`
	Results        []VerificationResult            `json:"results"`
	OracleVerdict  OracleVerdict                   `json:"oracle_verdict"`
}

func (p Pipeline) VerifyMonthlyReview(
	ctx context.Context,
	spec taskspec.TaskSpec,
	currentState state.FinancialWorldState,
	evidence []observation.EvidenceRecord,
	memories []memory.MemoryRecord,
	plan planning.ExecutionPlan,
	blockResults []analysis.BlockResultEnvelope,
	blockVerificationContexts []contextview.BlockVerificationContext,
	finalVerificationContext contextview.BlockVerificationContext,
	output any,
) (PipelineResult, error) {
	return p.verify(
		ctx,
		"monthly-review",
		spec,
		currentState,
		evidence,
		memories,
		plan,
		blockResults,
		blockVerificationContexts,
		finalVerificationContext,
		output,
		true,
	)
}

func (p Pipeline) VerifyDebtDecision(
	ctx context.Context,
	spec taskspec.TaskSpec,
	currentState state.FinancialWorldState,
	evidence []observation.EvidenceRecord,
	memories []memory.MemoryRecord,
	plan planning.ExecutionPlan,
	blockResults []analysis.BlockResultEnvelope,
	blockVerificationContexts []contextview.BlockVerificationContext,
	finalVerificationContext contextview.BlockVerificationContext,
	output any,
) (PipelineResult, error) {
	return p.verify(
		ctx,
		"debt-vs-invest",
		spec,
		currentState,
		evidence,
		memories,
		plan,
		blockResults,
		blockVerificationContexts,
		finalVerificationContext,
		output,
		false,
	)
}

func (p Pipeline) VerifyTaxOptimization(
	ctx context.Context,
	spec taskspec.TaskSpec,
	currentState state.FinancialWorldState,
	evidence []observation.EvidenceRecord,
	memories []memory.MemoryRecord,
	plan planning.ExecutionPlan,
	blockResults []analysis.BlockResultEnvelope,
	blockVerificationContexts []contextview.BlockVerificationContext,
	finalVerificationContext contextview.BlockVerificationContext,
	output any,
) (PipelineResult, error) {
	return p.verify(
		ctx,
		"tax-optimization",
		spec,
		currentState,
		evidence,
		memories,
		plan,
		blockResults,
		blockVerificationContexts,
		finalVerificationContext,
		output,
		false,
	)
}

func (p Pipeline) VerifyPortfolioRebalance(
	ctx context.Context,
	spec taskspec.TaskSpec,
	currentState state.FinancialWorldState,
	evidence []observation.EvidenceRecord,
	memories []memory.MemoryRecord,
	plan planning.ExecutionPlan,
	blockResults []analysis.BlockResultEnvelope,
	blockVerificationContexts []contextview.BlockVerificationContext,
	finalVerificationContext contextview.BlockVerificationContext,
	output any,
) (PipelineResult, error) {
	return p.verify(
		ctx,
		"portfolio-rebalance",
		spec,
		currentState,
		evidence,
		memories,
		plan,
		blockResults,
		blockVerificationContexts,
		finalVerificationContext,
		output,
		false,
	)
}

func (p Pipeline) VerifyLifeEventBlockPass(
	ctx context.Context,
	spec taskspec.TaskSpec,
	currentState state.FinancialWorldState,
	evidence []observation.EvidenceRecord,
	memories []memory.MemoryRecord,
	plan planning.ExecutionPlan,
	blockResults []analysis.BlockResultEnvelope,
	blockVerificationContexts []contextview.BlockVerificationContext,
) (PipelineResult, error) {
	coverageChecker := p.coverageChecker()
	oracle := p.oracle()
	if err := plan.Validate(); err != nil {
		return PipelineResult{}, err
	}
	coverageReport, err := coverageChecker.Check(spec, evidence)
	if err != nil {
		return PipelineResult{}, err
	}
	coverageResult := withScope(CoverageToVerificationResult(spec, coverageReport, p.now()), VerificationScopeFinal, "", "")
	blockMap, _, err := p.verifyBlocks(spec, currentState, plan, blockResults, blockVerificationContexts)
	if err != nil {
		return PipelineResult{}, err
	}
	flattened := []VerificationResult{coverageResult}
	flattened = append(flattened, flattenBlockResults(plan, blockMap)...)
	verdict, err := oracle.Evaluate(ctx, "life-event-analysis-blocks", flattened)
	if err != nil {
		return PipelineResult{}, err
	}
	return PipelineResult{
		CoverageReport: coverageReport,
		BlockResults:   blockMap,
		Results:        flattened,
		OracleVerdict:  verdict,
	}, nil
}

func (p Pipeline) VerifyLifeEventGeneratedTasksAndFinal(
	ctx context.Context,
	spec taskspec.TaskSpec,
	currentState state.FinancialWorldState,
	evidence []observation.EvidenceRecord,
	memories []memory.MemoryRecord,
	plan planning.ExecutionPlan,
	blockResults []analysis.BlockResultEnvelope,
	taskGraph taskspec.TaskGraph,
	finalVerificationContext contextview.BlockVerificationContext,
	reportOutput any,
) (PipelineResult, error) {
	coverageChecker := p.coverageChecker()
	successChecker := p.successChecker()
	oracle := p.oracle()
	if err := plan.Validate(); err != nil {
		return PipelineResult{}, err
	}
	if err := taskGraph.Validate(); err != nil {
		return PipelineResult{}, err
	}
	coverageReport, err := coverageChecker.Check(spec, evidence)
	if err != nil {
		return PipelineResult{}, err
	}
	coverageResult := withScope(CoverageToVerificationResult(spec, coverageReport, p.now()), VerificationScopeFinal, "", "")
	finalResults, err := p.verifyLifeEventFinal(spec, evidence, memories, blockResults, taskGraph, finalVerificationContext, reportOutput)
	if err != nil {
		return PipelineResult{}, err
	}
	flattened := []VerificationResult{coverageResult}
	flattened = append(flattened, finalResults...)
	successResult, err := successChecker.Check(spec, flattened, reportOutput)
	if err != nil {
		return PipelineResult{}, err
	}
	successResult = withScope(successResult, VerificationScopeFinal, "", "")
	finalResults = append(finalResults, successResult)
	flattened = append(flattened, successResult)
	verdict, err := oracle.Evaluate(ctx, "life-event-generated-tasks-final", flattened)
	if err != nil {
		return PipelineResult{}, err
	}
	return PipelineResult{
		CoverageReport: coverageReport,
		FinalResults:   finalResults,
		Results:        flattened,
		OracleVerdict:  verdict,
	}, nil
}

func (p Pipeline) verify(
	ctx context.Context,
	scenario string,
	spec taskspec.TaskSpec,
	currentState state.FinancialWorldState,
	evidence []observation.EvidenceRecord,
	memories []memory.MemoryRecord,
	plan planning.ExecutionPlan,
	blockResults []analysis.BlockResultEnvelope,
	blockVerificationContexts []contextview.BlockVerificationContext,
	finalVerificationContext contextview.BlockVerificationContext,
	output any,
	monthlyReview bool,
) (PipelineResult, error) {
	coverageChecker := p.coverageChecker()
	successChecker := p.successChecker()
	oracle := p.oracle()

	if err := plan.Validate(); err != nil {
		return PipelineResult{}, err
	}

	coverageReport, err := coverageChecker.Check(spec, evidence)
	if err != nil {
		return PipelineResult{}, err
	}
	coverageResult := withScope(CoverageToVerificationResult(spec, coverageReport, p.now()), VerificationScopeFinal, "", "")

	blockMap, severeFailure, err := p.verifyBlocks(spec, currentState, plan, blockResults, blockVerificationContexts)
	if err != nil {
		return PipelineResult{}, err
	}

	flattened := []VerificationResult{coverageResult}
	flattened = append(flattened, flattenBlockResults(plan, blockMap)...)

	if severeFailure {
		shortCircuit := VerificationResult{
			Status:                  VerificationStatusNeedsReplan,
			Scope:                   VerificationScopeFinal,
			Validator:               "final_validation_short_circuit",
			Message:                 "block-level verification failed; final report validation skipped",
			FailedRules:             []string{"block_verification_short_circuit"},
			RecommendedReplanAction: "repair failed block outputs before regenerating the final report",
			Severity:                "high",
			Details: map[string]any{
				"plan_id":               plan.PlanID,
				"selected_memory_count": len(memories),
			},
			EvidenceCoverage: fullCoverage(spec.ID),
			CheckedAt:        p.now(),
		}
		finalResults := []VerificationResult{shortCircuit}
		flattened = append(flattened, finalResults...)
		verdict, err := oracle.Evaluate(ctx, scenario, flattened)
		if err != nil {
			return PipelineResult{}, err
		}
		return PipelineResult{
			CoverageReport: coverageReport,
			BlockResults:   blockMap,
			FinalResults:   finalResults,
			Results:        flattened,
			OracleVerdict:  verdict,
		}, nil
	}

	finalResults, err := p.verifyFinal(ctx, spec, currentState, evidence, finalVerificationContext, output, monthlyReview)
	if err != nil {
		return PipelineResult{}, err
	}
	flattened = append(flattened, finalResults...)

	successResult, err := successChecker.Check(spec, flattened, output)
	if err != nil {
		return PipelineResult{}, err
	}
	successResult = withScope(successResult, VerificationScopeFinal, "", "")
	finalResults = append(finalResults, successResult)
	flattened = append(flattened, successResult)

	verdict, err := oracle.Evaluate(ctx, scenario, flattened)
	if err != nil {
		return PipelineResult{}, err
	}
	return PipelineResult{
		CoverageReport: coverageReport,
		BlockResults:   blockMap,
		FinalResults:   finalResults,
		Results:        flattened,
		OracleVerdict:  verdict,
	}, nil
}

func (p Pipeline) verifyBlocks(
	spec taskspec.TaskSpec,
	currentState state.FinancialWorldState,
	plan planning.ExecutionPlan,
	results []analysis.BlockResultEnvelope,
	contexts []contextview.BlockVerificationContext,
) (map[string][]VerificationResult, bool, error) {
	resultByID := make(map[string]analysis.BlockResultEnvelope, len(results))
	for _, item := range results {
		if err := item.Validate(); err != nil {
			return nil, false, err
		}
		resultByID[item.BlockID] = item
	}
	contextByID := make(map[string]contextview.BlockVerificationContext, len(contexts))
	for _, item := range contexts {
		contextByID[item.BlockID] = item
	}

	blockResults := make(map[string][]VerificationResult, len(plan.Blocks))
	severe := false
	for _, block := range plan.Blocks {
		envelope, ok := resultByID[string(block.ID)]
		if !ok {
			result := VerificationResult{
				Status:                  VerificationStatusFail,
				Scope:                   VerificationScopeBlock,
				BlockID:                 string(block.ID),
				BlockKind:               string(block.Kind),
				Validator:               "block_presence_validator",
				Message:                 "required block result is missing",
				FailedRules:             []string{"schema_invalid"},
				RecommendedReplanAction: "dispatch the missing analysis block before drafting the report",
				Severity:                "high",
				EvidenceCoverage:        fullCoverage(spec.ID),
				CheckedAt:               p.now(),
			}
			blockResults[string(block.ID)] = []VerificationResult{result}
			severe = true
			continue
		}
		verificationContext, ok := contextByID[string(block.ID)]
		if !ok {
			result := VerificationResult{
				Status:                  VerificationStatusFail,
				Scope:                   VerificationScopeBlock,
				BlockID:                 string(block.ID),
				BlockKind:               string(block.Kind),
				Validator:               "block_context_validator",
				Message:                 "block verification context is missing",
				FailedRules:             []string{"schema_invalid"},
				RecommendedReplanAction: "assemble block verification context before verifying the report",
				Severity:                "high",
				EvidenceCoverage:        fullCoverage(spec.ID),
				CheckedAt:               p.now(),
			}
			blockResults[string(block.ID)] = []VerificationResult{result}
			severe = true
			continue
		}

		var blockResult VerificationResult
		switch {
		case envelope.Cashflow != nil:
			blockResult = CashflowBlockValidator{}.Validate(spec, block, *envelope.Cashflow, verificationContext, currentState)
		case envelope.Debt != nil:
			blockResult = DebtBlockValidator{}.Validate(spec, block, *envelope.Debt, verificationContext, currentState)
		case envelope.Tax != nil:
			blockResult = TaxBlockValidator{}.Validate(spec, block, *envelope.Tax, verificationContext, currentState)
		case envelope.Portfolio != nil:
			blockResult = PortfolioBlockValidator{}.Validate(spec, block, *envelope.Portfolio, verificationContext, currentState)
		default:
			blockResult = VerificationResult{
				Status:                  VerificationStatusFail,
				Scope:                   VerificationScopeBlock,
				BlockID:                 string(block.ID),
				BlockKind:               string(block.Kind),
				Validator:               "block_schema_validator",
				Message:                 "typed block result is missing",
				FailedRules:             []string{"schema_invalid"},
				RecommendedReplanAction: "rerun block analysis and return a typed block result",
				Severity:                "high",
				EvidenceCoverage:        fullCoverage(spec.ID),
				CheckedAt:               p.now(),
			}
		}
		blockResults[string(block.ID)] = []VerificationResult{blockResult}
		severe = severe || severeBlockFailure(blockResult)
	}
	return blockResults, severe, nil
}

func (p Pipeline) verifyFinal(
	ctx context.Context,
	spec taskspec.TaskSpec,
	currentState state.FinancialWorldState,
	evidence []observation.EvidenceRecord,
	finalVerificationContext contextview.BlockVerificationContext,
	output any,
	monthlyReview bool,
) ([]VerificationResult, error) {
	finalResults := make([]VerificationResult, 0, 4)

	contextResult := VerificationResult{
		Status:                  VerificationStatusPass,
		Scope:                   VerificationScopeFinal,
		Validator:               "final_verification_context_validator",
		Message:                 "final verification context is valid",
		RecommendedReplanAction: "rebuild final verification context with grounded evidence",
		Severity:                "low",
		Details: map[string]any{
			"selected_memory_ids":   finalVerificationContext.SelectedMemoryIDs,
			"selected_evidence_ids": finalVerificationContext.SelectedEvidenceIDs,
			"selected_state_blocks": finalVerificationContext.SelectedStateBlocks,
		},
		EvidenceCoverage: fullCoverage(spec.ID),
		CheckedAt:        p.now(),
	}
	if len(finalVerificationContext.SelectedEvidenceIDs) == 0 || finalVerificationContext.ResultSummary == "" {
		contextResult.Status = VerificationStatusNeedsReplan
		contextResult.Message = "final verification context is incomplete"
		contextResult.FailedRules = []string{"final_context_incomplete"}
		contextResult.Severity = "medium"
	}
	finalResults = append(finalResults, contextResult)

	switch spec.UserIntentType {
	case taskspec.UserIntentMonthlyReview:
		deterministicResult, err := p.deterministicValidator().Validate(ctx, spec, currentState, evidence, output)
		if err != nil {
			return nil, err
		}
		finalResults = append(finalResults, withScope(deterministicResult, VerificationScopeFinal, "", ""))
		businessResult, err := p.businessValidator(true).Validate(ctx, spec, currentState, evidence, output)
		if err != nil {
			return nil, err
		}
		finalResults = append(finalResults, withScope(businessResult, VerificationScopeFinal, "", ""))
		return finalResults, nil
	case taskspec.UserIntentTaxOptimization:
		deterministicResult, err := TaxOptimizationDeterministicValidator{}.Validate(ctx, spec, currentState, evidence, output)
		if err != nil {
			return nil, err
		}
		finalResults = append(finalResults, withScope(deterministicResult, VerificationScopeFinal, "", ""))
		businessResult, err := TaxOptimizationBusinessValidator{}.Validate(ctx, spec, currentState, evidence, output)
		if err != nil {
			return nil, err
		}
		finalResults = append(finalResults, withScope(businessResult, VerificationScopeFinal, "", ""))
		return finalResults, nil
	case taskspec.UserIntentPortfolioRebalance:
		deterministicResult, err := PortfolioRebalanceDeterministicValidator{}.Validate(ctx, spec, currentState, evidence, output)
		if err != nil {
			return nil, err
		}
		finalResults = append(finalResults, withScope(deterministicResult, VerificationScopeFinal, "", ""))
		businessResult, err := PortfolioRebalanceBusinessValidator{}.Validate(ctx, spec, currentState, evidence, output)
		if err != nil {
			return nil, err
		}
		finalResults = append(finalResults, withScope(businessResult, VerificationScopeFinal, "", ""))
		return finalResults, nil
	}

	businessResult, err := p.businessValidator(false).Validate(ctx, spec, currentState, evidence, output)
	if err != nil {
		return nil, err
	}
	finalResults = append(finalResults, withScope(businessResult, VerificationScopeFinal, "", ""))
	return finalResults, nil
}

func (p Pipeline) verifyLifeEventFinal(
	spec taskspec.TaskSpec,
	evidence []observation.EvidenceRecord,
	memories []memory.MemoryRecord,
	blockResults []analysis.BlockResultEnvelope,
	taskGraph taskspec.TaskGraph,
	finalVerificationContext contextview.BlockVerificationContext,
	output any,
) ([]VerificationResult, error) {
	report, ok := output.(reporting.LifeEventAssessmentReport)
	finalResults := make([]VerificationResult, 0, 4)

	contextResult := VerificationResult{
		Status:                  VerificationStatusPass,
		Scope:                   VerificationScopeFinal,
		Validator:               "life_event_final_context_validator",
		Message:                 "life-event final verification context is valid",
		RecommendedReplanAction: "rebuild life-event final verification context with grounded evidence and task graph",
		Severity:                "low",
		Details: map[string]any{
			"selected_memory_ids":   finalVerificationContext.SelectedMemoryIDs,
			"selected_evidence_ids": finalVerificationContext.SelectedEvidenceIDs,
			"selected_state_blocks": finalVerificationContext.SelectedStateBlocks,
		},
		EvidenceCoverage: fullCoverage(spec.ID),
		CheckedAt:        p.now(),
	}
	if len(finalVerificationContext.SelectedEvidenceIDs) == 0 || finalVerificationContext.ResultSummary == "" {
		contextResult.Status = VerificationStatusNeedsReplan
		contextResult.Message = "life-event final verification context is incomplete"
		contextResult.FailedRules = []string{"final_context_incomplete"}
		contextResult.Severity = "medium"
	}
	finalResults = append(finalResults, contextResult)

	taskGrounding := VerificationResult{
		Status:                  VerificationStatusPass,
		Scope:                   VerificationScopeFinal,
		Validator:               "generated_task_grounding_validator",
		Message:                 "generated tasks are grounded in validated blocks, evidence, state diff, and memories",
		RecommendedReplanAction: "regenerate follow-up tasks from validated block results and event evidence",
		Severity:                "medium",
		Details: map[string]any{
			"generated_task_count": len(taskGraph.GeneratedTasks),
			"memory_count":         len(memories),
		},
		EvidenceCoverage: fullCoverage(spec.ID),
		CheckedAt:        p.now(),
	}
	failedRules := make([]string, 0)
	seenIntents := make(map[taskspec.UserIntentType]struct{}, len(taskGraph.GeneratedTasks))
	allowedEvidence := make(map[string]struct{}, len(evidence))
	for _, item := range evidence {
		allowedEvidence[string(item.ID)] = struct{}{}
	}
	availableBlockIDs := make(map[string]struct{}, len(blockResults))
	for _, item := range blockResults {
		availableBlockIDs[item.BlockID] = struct{}{}
	}
	for _, generated := range taskGraph.GeneratedTasks {
		if _, exists := seenIntents[generated.Task.UserIntentType]; exists {
			failedRules = append(failedRules, "duplicate_generated_task")
		}
		seenIntents[generated.Task.UserIntentType] = struct{}{}
		if len(generated.Metadata.GenerationReasons) == 0 {
			failedRules = append(failedRules, "generated_task_reason_missing")
			continue
		}
		reason := generated.Metadata.GenerationReasons[0]
		if len(reason.EvidenceIDs) == 0 && len(reason.StateDiffFields) == 0 && len(reason.MemoryIDs) == 0 {
			failedRules = append(failedRules, "generated_task_ungrounded")
		}
		for _, id := range reason.EvidenceIDs {
			if _, ok := allowedEvidence[id]; !ok {
				failedRules = append(failedRules, "generated_task_unknown_evidence")
				break
			}
		}
	}
	if len(failedRules) > 0 {
		taskGrounding.Status = VerificationStatusNeedsReplan
		taskGrounding.Message = "generated task validation failed"
		taskGrounding.FailedRules = uniqueStrings(failedRules)
		taskGrounding.RecommendedReplanAction = "repair generated task grounding, deduplication, and evidence linkage before runtime registration"
		taskGrounding.MissingEvidence = missingEvidenceFromTaskGraph(taskGraph)
	}
	finalResults = append(finalResults, taskGrounding)

	assessmentResult := VerificationResult{
		Status:                  VerificationStatusPass,
		Scope:                   VerificationScopeFinal,
		Validator:               "life_event_assessment_consistency_validator",
		Message:                 "life-event assessment is consistent with generated tasks and validated block outputs",
		RecommendedReplanAction: "rebuild life-event assessment from validated block outputs and generated task graph",
		Severity:                "medium",
		Details: map[string]any{
			"plan_id":          taskGraph.ParentWorkflowID,
			"block_result_ids": keysFromSet(availableBlockIDs),
		},
		EvidenceCoverage: fullCoverage(spec.ID),
		CheckedAt:        p.now(),
	}
	if !ok {
		assessmentResult.Status = VerificationStatusNeedsReplan
		assessmentResult.Message = "life-event assessment payload is missing"
		assessmentResult.FailedRules = []string{"assessment_missing"}
		finalResults = append(finalResults, assessmentResult)
		return finalResults, nil
	}
	if report.EventSummary == "" || len(report.GeneratedTaskIDs) == 0 {
		assessmentResult.Status = VerificationStatusNeedsReplan
		assessmentResult.Message = "life-event assessment is incomplete"
		assessmentResult.FailedRules = []string{"assessment_incomplete"}
	}
	finalResults = append(finalResults, assessmentResult)
	return finalResults, nil
}

func NeedsReplan(results []VerificationResult) bool {
	for _, result := range results {
		if result.Status == VerificationStatusFail || result.Status == VerificationStatusNeedsReplan {
			return true
		}
	}
	return false
}

func CoverageToVerificationResult(spec taskspec.TaskSpec, report EvidenceCoverageReport, checkedAt time.Time) VerificationResult {
	status := VerificationStatusPass
	message := "required evidence coverage satisfied"
	missing := make([]string, 0)
	for i, item := range report.Items {
		if !item.Covered && i < len(spec.RequiredEvidence) && spec.RequiredEvidence[i].Mandatory {
			status = VerificationStatusNeedsReplan
			message = "mandatory evidence missing: " + item.RequirementID
			missing = append(missing, item.RequirementID)
		}
	}
	result := VerificationResult{
		Status:                  status,
		Scope:                   VerificationScopeFinal,
		Validator:               "evidence_coverage_checker",
		Message:                 message,
		MissingEvidence:         missing,
		RecommendedReplanAction: "collect missing mandatory evidence",
		Severity:                "medium",
		EvidenceCoverage:        report,
		CheckedAt:               checkedAt,
	}
	if len(missing) > 0 {
		result.FailedRules = []string{"required_evidence"}
		result.Details = map[string]any{"missing_requirement_ids": missing}
	}
	return result
}

func flattenBlockResults(plan planning.ExecutionPlan, byBlock map[string][]VerificationResult) []VerificationResult {
	result := make([]VerificationResult, 0, len(byBlock))
	for _, block := range plan.Blocks {
		result = append(result, byBlock[string(block.ID)]...)
	}
	return result
}

func withScope(result VerificationResult, scope VerificationScope, blockID string, blockKind string) VerificationResult {
	result.Scope = scope
	result.BlockID = blockID
	result.BlockKind = blockKind
	return result
}

func missingEvidenceFromTaskGraph(graph taskspec.TaskGraph) []string {
	missing := make([]string, 0)
	for _, task := range graph.GeneratedTasks {
		if len(task.Metadata.GenerationReasons) == 0 {
			continue
		}
		reason := task.Metadata.GenerationReasons[0]
		if len(reason.EvidenceIDs) == 0 {
			missing = append(missing, task.Task.ID)
		}
	}
	return missing
}

func uniqueStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func keysFromSet(items map[string]struct{}) []string {
	result := make([]string, 0, len(items))
	for item := range items {
		result = append(result, item)
	}
	return result
}

func (p Pipeline) coverageChecker() EvidenceCoverageChecker {
	if p.CoverageChecker != nil {
		return p.CoverageChecker
	}
	return DefaultEvidenceCoverageChecker{}
}

func (p Pipeline) deterministicValidator() DeterministicValidator {
	if p.DeterministicValidator != nil {
		return p.DeterministicValidator
	}
	return MonthlyReviewDeterministicValidator{}
}

func (p Pipeline) businessValidator(monthlyReview bool) BusinessValidator {
	if p.BusinessValidator != nil {
		return p.BusinessValidator
	}
	if monthlyReview {
		return MonthlyReviewBusinessValidator{}
	}
	return DebtDecisionBusinessValidator{}
}

func (p Pipeline) successChecker() SuccessCriteriaChecker {
	if p.SuccessChecker != nil {
		return p.SuccessChecker
	}
	return DefaultSuccessCriteriaChecker{}
}

func (p Pipeline) oracle() TrajectoryOracle {
	if p.Oracle != nil {
		return p.Oracle
	}
	return BaselineTrajectoryOracle{}
}

func (p Pipeline) now() time.Time {
	if p.Now != nil {
		return p.Now().UTC()
	}
	return time.Now().UTC()
}
