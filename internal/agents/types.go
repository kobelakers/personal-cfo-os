package agents

import (
	"context"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/analysis"
	contextview "github.com/kobelakers/personal-cfo-os/internal/context"
	"github.com/kobelakers/personal-cfo-os/internal/governance"
	"github.com/kobelakers/personal-cfo-os/internal/memory"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/planning"
	"github.com/kobelakers/personal-cfo-os/internal/protocol"
	"github.com/kobelakers/personal-cfo-os/internal/reporting"
	"github.com/kobelakers/personal-cfo-os/internal/state"
	"github.com/kobelakers/personal-cfo-os/internal/taskspec"
	"github.com/kobelakers/personal-cfo-os/internal/verification"
)

const (
	RecipientPlannerAgent      = "planner_agent"
	RecipientMemorySteward     = "memory_steward"
	RecipientReportAgent       = "report_agent"
	RecipientVerificationAgent = "verification_agent"
	RecipientGovernanceAgent   = "governance_agent"
	RecipientCashflowAgent     = "cashflow_agent"
	RecipientDebtAgent         = "debt_agent"
)

type DomainAgent interface {
	RegisteredSystemAgent
}

type CashflowAgent interface{ DomainAgent }
type DebtAgent interface{ DomainAgent }
type TaxAgent interface{ DomainAgent }
type PortfolioAgent interface{ DomainAgent }
type BehaviorAgent interface{ DomainAgent }

type AgentHandlerContext struct {
	Context    context.Context
	DispatchID string
	TraceID    string
	Now        func() time.Time
}

type AgentHandlerResult struct {
	Kind                  protocol.MessageKind
	Body                  protocol.AgentResultBody
	ProducedArtifacts     []reporting.WorkflowArtifact
	ProducedMemories      []string
	VerificationResults   []verification.VerificationResult
	EmittedWorkflowEvents []protocol.WorkflowEvent
}

type RegisteredSystemAgent interface {
	Name() string
	Recipient() string
	RequestKind() protocol.MessageKind
	Handle(handlerCtx AgentHandlerContext, envelope protocol.AgentEnvelope) (AgentHandlerResult, error)
}

type PlannerAgent interface{ RegisteredSystemAgent }
type MemorySteward interface{ RegisteredSystemAgent }
type VerificationAgent interface{ RegisteredSystemAgent }
type GovernanceAgent interface{ RegisteredSystemAgent }
type ReportAgent interface{ RegisteredSystemAgent }

type AgentRegistry interface {
	Register(agent RegisteredSystemAgent) error
	Resolve(recipient string, kind protocol.MessageKind) (RegisteredSystemAgent, error)
}

type AgentDispatcher interface {
	Dispatch(ctx context.Context, envelope protocol.AgentEnvelope) (AgentDispatchResult, error)
}

type AgentExecutor interface {
	Execute(handlerCtx AgentHandlerContext, agent RegisteredSystemAgent, envelope protocol.AgentEnvelope) (AgentHandlerResult, error)
}

type AgentDispatchResult struct {
	DispatchID string
	Request    protocol.AgentEnvelope
	Response   protocol.AgentResponse
}

type StepDispatchMetadata struct {
	RequestMetadata  protocol.ProtocolMetadata `json:"request_metadata"`
	ResponseMetadata protocol.ProtocolMetadata `json:"response_metadata"`
	EmittedEvents    []protocol.WorkflowEvent  `json:"emitted_events,omitempty"`
}

type SystemStepMetadata struct {
	WorkflowID    string
	Sender        string
	Task          taskspec.TaskSpec
	StateRef      protocol.StateReference
	CorrelationID string
	CausationID   string
}

type PlanStepResult struct {
	Metadata StepDispatchMetadata   `json:"metadata"`
	Plan     planning.ExecutionPlan `json:"plan"`
}

type MemorySyncStepResult struct {
	Metadata  StepDispatchMetadata        `json:"metadata"`
	Result    memory.WorkflowMemoryResult `json:"result"`
	AuditRefs []string                    `json:"audit_refs,omitempty"`
}

type ReportDraftStepResult struct {
	Metadata StepDispatchMetadata    `json:"metadata"`
	Draft    reporting.ReportPayload `json:"draft"`
}

type VerificationStepInput struct {
	CurrentState              state.FinancialWorldState              `json:"current_state"`
	Evidence                  []observation.EvidenceRecord           `json:"evidence"`
	Memories                  []memory.MemoryRecord                  `json:"memories,omitempty"`
	Plan                      planning.ExecutionPlan                 `json:"plan"`
	BlockResults              []analysis.BlockResultEnvelope         `json:"block_results,omitempty"`
	BlockVerificationContexts []contextview.BlockVerificationContext `json:"block_verification_contexts,omitempty"`
	FinalVerificationContext  contextview.BlockVerificationContext   `json:"final_verification_context"`
	Report                    reporting.ReportPayload                `json:"report"`
}

type VerificationStepResult struct {
	Metadata StepDispatchMetadata        `json:"metadata"`
	Result   verification.PipelineResult `json:"result"`
}

type GovernanceStepResult struct {
	Metadata          StepDispatchMetadata          `json:"metadata"`
	Approval          governance.ApprovalEvaluation `json:"approval"`
	Disclosure        governance.ReportEvaluation   `json:"disclosure"`
	RedactionRequired bool                          `json:"redaction_required"`
}

type ReportFinalizeStepResult struct {
	Metadata  StepDispatchMetadata         `json:"metadata"`
	Report    reporting.ReportPayload      `json:"report"`
	Artifacts []reporting.WorkflowArtifact `json:"artifacts,omitempty"`
}

type AnalysisBlockStepResult struct {
	Metadata StepDispatchMetadata         `json:"metadata"`
	Block    planning.ExecutionBlock      `json:"block"`
	Result   analysis.BlockResultEnvelope `json:"result"`
}

type SystemStepBus interface {
	DispatchPlan(ctx context.Context, meta SystemStepMetadata, current state.FinancialWorldState, memories []memory.MemoryRecord, evidence []observation.EvidenceRecord) (PlanStepResult, error)
	DispatchMemorySync(ctx context.Context, meta SystemStepMetadata, current state.FinancialWorldState, evidence []observation.EvidenceRecord, conclusionHint string) (MemorySyncStepResult, error)
	DispatchAnalysisBlock(ctx context.Context, meta SystemStepMetadata, block planning.ExecutionBlock, current state.FinancialWorldState, memories []memory.MemoryRecord, evidence []observation.EvidenceRecord, executionContext contextview.BlockExecutionContext) (AnalysisBlockStepResult, error)
	DispatchReportDraft(ctx context.Context, meta SystemStepMetadata, current state.FinancialWorldState, memories []memory.MemoryRecord, evidence []observation.EvidenceRecord, plan planning.ExecutionPlan, blockResults []analysis.BlockResultEnvelope) (ReportDraftStepResult, error)
	DispatchVerification(ctx context.Context, meta SystemStepMetadata, input VerificationStepInput) (VerificationStepResult, error)
	DispatchGovernance(ctx context.Context, meta SystemStepMetadata, current state.FinancialWorldState, report reporting.ReportPayload) (GovernanceStepResult, error)
	DispatchReportFinalize(ctx context.Context, meta SystemStepMetadata, draft reporting.ReportPayload, disclosureDecision governance.PolicyDecision) (ReportFinalizeStepResult, error)
}

type PlannerInputs struct {
	Assembler contextview.ContextAssembler
	Planner   *planning.DeterministicPlanner
}
