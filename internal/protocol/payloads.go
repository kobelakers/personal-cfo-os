package protocol

import (
	"github.com/kobelakers/personal-cfo-os/internal/analysis"
	contextview "github.com/kobelakers/personal-cfo-os/internal/context"
	"github.com/kobelakers/personal-cfo-os/internal/governance"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/planning"
	"github.com/kobelakers/personal-cfo-os/internal/reporting"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/verification"
)

type PlanRequestPayload struct {
	CurrentState state.FinancialWorldState    `json:"current_state"`
	Memories     []memory.MemoryRecord        `json:"memories,omitempty"`
	Evidence     []observation.EvidenceRecord `json:"evidence"`
	PlanningView contextview.ContextView      `json:"planning_view"`
}

type MemorySyncRequestPayload struct {
	CurrentState   state.FinancialWorldState    `json:"current_state"`
	Evidence       []observation.EvidenceRecord `json:"evidence"`
	ConclusionHint string                       `json:"conclusion_hint,omitempty"`
}

type ReportDraftRequestPayload struct {
	CurrentState state.FinancialWorldState      `json:"current_state"`
	Memories     []memory.MemoryRecord          `json:"memories,omitempty"`
	Evidence     []observation.EvidenceRecord   `json:"evidence"`
	Plan         planning.ExecutionPlan         `json:"plan"`
	BlockResults []analysis.BlockResultEnvelope `json:"block_results,omitempty"`
}

type VerificationRequestPayload struct {
	CurrentState              state.FinancialWorldState              `json:"current_state"`
	Evidence                  []observation.EvidenceRecord           `json:"evidence"`
	Memories                  []memory.MemoryRecord                  `json:"memories,omitempty"`
	Plan                      planning.ExecutionPlan                 `json:"plan"`
	BlockResults              []analysis.BlockResultEnvelope         `json:"block_results,omitempty"`
	BlockVerificationContexts []contextview.BlockVerificationContext `json:"block_verification_contexts,omitempty"`
	FinalVerificationContext  contextview.BlockVerificationContext   `json:"final_verification_context"`
	Report                    reporting.ReportPayload                `json:"report"`
}

type GovernanceEvaluationRequestPayload struct {
	CurrentState    state.FinancialWorldState `json:"current_state"`
	Report          reporting.ReportPayload   `json:"report"`
	RequestedAction string                    `json:"requested_action"`
	Actor           string                    `json:"actor"`
	ActorRoles      []string                  `json:"actor_roles,omitempty"`
	ContainsPII     bool                      `json:"contains_pii"`
	Audience        string                    `json:"audience"`
	ForceApproval   bool                      `json:"force_approval"`
}

type ReportFinalizeRequestPayload struct {
	Draft              reporting.ReportPayload   `json:"draft"`
	DisclosureDecision governance.PolicyDecision `json:"disclosure_decision"`
}

type CashflowAnalysisRequestPayload struct {
	CurrentState     state.FinancialWorldState         `json:"current_state"`
	RelevantMemories []memory.MemoryRecord             `json:"relevant_memories,omitempty"`
	RelevantEvidence []observation.EvidenceRecord      `json:"relevant_evidence,omitempty"`
	Block            planning.ExecutionBlock           `json:"block"`
	ExecutionContext contextview.BlockExecutionContext `json:"execution_context"`
}

type DebtAnalysisRequestPayload struct {
	CurrentState     state.FinancialWorldState         `json:"current_state"`
	RelevantMemories []memory.MemoryRecord             `json:"relevant_memories,omitempty"`
	RelevantEvidence []observation.EvidenceRecord      `json:"relevant_evidence,omitempty"`
	Block            planning.ExecutionBlock           `json:"block"`
	ExecutionContext contextview.BlockExecutionContext `json:"execution_context"`
}

type PlanResultPayload struct {
	Plan planning.ExecutionPlan `json:"plan"`
}

type MemorySyncResultPayload struct {
	Result    memory.WorkflowMemoryResult `json:"result"`
	AuditRefs []string                    `json:"audit_refs,omitempty"`
}

type ReportDraftResultPayload struct {
	Draft reporting.ReportPayload `json:"draft"`
}

type VerificationResultPayload struct {
	Result verification.PipelineResult `json:"result"`
}

type GovernanceEvaluationResultPayload struct {
	Approval          governance.ApprovalEvaluation `json:"approval"`
	Disclosure        governance.ReportEvaluation   `json:"disclosure"`
	RedactionRequired bool                          `json:"redaction_required"`
}

type ReportFinalizeResultPayload struct {
	Report    reporting.ReportPayload      `json:"report"`
	Artifacts []reporting.WorkflowArtifact `json:"artifacts,omitempty"`
}

type CashflowAnalysisResultPayload struct {
	Result analysis.CashflowBlockResult `json:"result"`
}

type DebtAnalysisResultPayload struct {
	Result analysis.DebtBlockResult `json:"result"`
}

type AgentRequestBody struct {
	PlanRequest                 *PlanRequestPayload                 `json:"plan_request,omitempty"`
	MemorySyncRequest           *MemorySyncRequestPayload           `json:"memory_sync_request,omitempty"`
	ReportDraftRequest          *ReportDraftRequestPayload          `json:"report_draft_request,omitempty"`
	VerificationRequest         *VerificationRequestPayload         `json:"verification_request,omitempty"`
	GovernanceEvaluationRequest *GovernanceEvaluationRequestPayload `json:"governance_evaluation_request,omitempty"`
	ReportFinalizeRequest       *ReportFinalizeRequestPayload       `json:"report_finalize_request,omitempty"`
	CashflowAnalysisRequest     *CashflowAnalysisRequestPayload     `json:"cashflow_analysis_request,omitempty"`
	DebtAnalysisRequest         *DebtAnalysisRequestPayload         `json:"debt_analysis_request,omitempty"`
}

type AgentResultBody struct {
	PlanResult                 *PlanResultPayload                 `json:"plan_result,omitempty"`
	MemorySyncResult           *MemorySyncResultPayload           `json:"memory_sync_result,omitempty"`
	ReportDraftResult          *ReportDraftResultPayload          `json:"report_draft_result,omitempty"`
	VerificationResult         *VerificationResultPayload         `json:"verification_result,omitempty"`
	GovernanceEvaluationResult *GovernanceEvaluationResultPayload `json:"governance_evaluation_result,omitempty"`
	ReportFinalizeResult       *ReportFinalizeResultPayload       `json:"report_finalize_result,omitempty"`
	CashflowAnalysisResult     *CashflowAnalysisResultPayload     `json:"cashflow_analysis_result,omitempty"`
	DebtAnalysisResult         *DebtAnalysisResultPayload         `json:"debt_analysis_result,omitempty"`
}
