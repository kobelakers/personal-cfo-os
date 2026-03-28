package state

import (
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observation"
)

type LiabilityAccount struct {
	Name            string  `json:"name"`
	BalanceCents    int64   `json:"balance_cents"`
	AnnualRate      float64 `json:"annual_rate"`
	MinimumDueCents int64   `json:"minimum_due_cents"`
}

type CashflowState struct {
	MonthlyNetIncomeCents       int64   `json:"monthly_net_income_cents"`
	MonthlyFixedExpenseCents    int64   `json:"monthly_fixed_expense_cents"`
	MonthlyVariableExpenseCents int64   `json:"monthly_variable_expense_cents"`
	SavingsRate                 float64 `json:"savings_rate"`
}

type LiabilityState struct {
	TotalDebtCents int64              `json:"total_debt_cents"`
	AverageAPR     float64            `json:"average_apr"`
	Accounts       []LiabilityAccount `json:"accounts,omitempty"`
}

type PortfolioState struct {
	TotalInvestableAssetsCents int64              `json:"total_investable_assets_cents"`
	AssetAllocations           map[string]float64 `json:"asset_allocations,omitempty"`
	EmergencyFundMonths        float64            `json:"emergency_fund_months"`
}

type TaxState struct {
	EffectiveTaxRate               float64  `json:"effective_tax_rate"`
	TaxAdvantagedContributionCents int64    `json:"tax_advantaged_contribution_cents"`
	UpcomingDeadlines              []string `json:"upcoming_deadlines,omitempty"`
}

type BehaviorState struct {
	AnomalyFlags      []string `json:"anomaly_flags,omitempty"`
	InterventionQueue []string `json:"intervention_queue,omitempty"`
}

type WorkflowState struct {
	ActiveWorkflowID string    `json:"active_workflow_id,omitempty"`
	Phase            string    `json:"phase"`
	PendingApprovals []string  `json:"pending_approvals,omitempty"`
	LastUpdatedAt    time.Time `json:"last_updated_at"`
}

type RiskState struct {
	LiquidityRisk     string   `json:"liquidity_risk"`
	ConcentrationRisk string   `json:"concentration_risk"`
	DebtStressLevel   string   `json:"debt_stress_level"`
	ComplianceFlags   []string `json:"compliance_flags,omitempty"`
}

type StateVersion struct {
	Sequence   uint64    `json:"sequence"`
	SnapshotID string    `json:"snapshot_id"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type FinancialWorldState struct {
	UserID         string         `json:"user_id"`
	CashflowState  CashflowState  `json:"cashflow_state"`
	LiabilityState LiabilityState `json:"liability_state"`
	PortfolioState PortfolioState `json:"portfolio_state"`
	TaxState       TaxState       `json:"tax_state"`
	BehaviorState  BehaviorState  `json:"behavior_state"`
	WorkflowState  WorkflowState  `json:"workflow_state"`
	RiskState      RiskState      `json:"risk_state"`
	Version        StateVersion   `json:"version"`
}

type StateSnapshot struct {
	State      FinancialWorldState `json:"state"`
	CapturedAt time.Time           `json:"captured_at"`
	Reason     string              `json:"reason"`
}

type StateDiff struct {
	FromVersion   uint64                   `json:"from_version"`
	ToVersion     uint64                   `json:"to_version"`
	ChangedFields []string                 `json:"changed_fields"`
	EvidenceIDs   []observation.EvidenceID `json:"evidence_ids"`
}

type StateMutation struct {
	Path       string                 `json:"path"`
	Operation  string                 `json:"operation"`
	ValueJSON  string                 `json:"value_json"`
	EvidenceID observation.EvidenceID `json:"evidence_id"`
}

type EvidencePatch struct {
	Evidence  []observation.EvidenceRecord `json:"evidence"`
	Mutations []StateMutation              `json:"mutations"`
	Summary   string                       `json:"summary"`
	AppliedAt time.Time                    `json:"applied_at"`
}
