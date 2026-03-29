package reducers

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/state"
)

type DeterministicReducerEngine struct {
	Now func() time.Time
}

type CashflowReducer struct{}
type LiabilityReducer struct{}
type PortfolioReducer struct{}
type TaxReducer struct{}
type BehaviorReducer struct{}
type RiskReducer struct{}
type WorkflowReducer struct{}

func (e DeterministicReducerEngine) BuildPatch(
	current state.FinancialWorldState,
	evidence []observation.EvidenceRecord,
	taskID string,
	workflowID string,
	phase string,
) (state.EvidencePatch, error) {
	if len(evidence) == 0 {
		return state.EvidencePatch{}, fmt.Errorf("reducers require at least one evidence record")
	}

	now := time.Now().UTC()
	if e.Now != nil {
		now = e.Now().UTC()
	}

	cashflow, err := CashflowReducer{}.Reduce(current.CashflowState, evidence)
	if err != nil {
		return state.EvidencePatch{}, err
	}
	liability, err := LiabilityReducer{}.Reduce(current.LiabilityState, evidence)
	if err != nil {
		return state.EvidencePatch{}, err
	}
	portfolio, err := PortfolioReducer{}.Reduce(current.PortfolioState, evidence)
	if err != nil {
		return state.EvidencePatch{}, err
	}
	tax, err := TaxReducer{}.Reduce(current.TaxState, evidence)
	if err != nil {
		return state.EvidencePatch{}, err
	}
	behavior, err := BehaviorReducer{}.Reduce(current.BehaviorState, evidence)
	if err != nil {
		return state.EvidencePatch{}, err
	}
	risk := RiskReducer{}.Reduce(current.RiskState, cashflow, liability, portfolio, tax, behavior)
	workflow := WorkflowReducer{}.Reduce(current.WorkflowState, taskID, workflowID, phase, evidence, now)

	mutations, err := buildMutations(evidence, map[string]any{
		"cashflow":  cashflow,
		"liability": liability,
		"portfolio": portfolio,
		"tax":       tax,
		"behavior":  behavior,
		"risk":      risk,
		"workflow":  workflow,
	})
	if err != nil {
		return state.EvidencePatch{}, err
	}

	return state.EvidencePatch{
		Evidence:  evidence,
		Mutations: mutations,
		Summary:   fmt.Sprintf("applied %d evidence records into financial world state", len(evidence)),
		AppliedAt: now,
	}, nil
}

func (CashflowReducer) Reduce(current state.CashflowState, evidence []observation.EvidenceRecord) (state.CashflowState, error) {
	next := current
	for _, record := range evidence {
		switch record.Type {
		case observation.EvidenceTypeTransactionBatch:
			if value, ok, err := claimInt64(record, "monthly_inflow_cents"); err != nil {
				return state.CashflowState{}, err
			} else if ok {
				next.MonthlyInflowCents = value
			}
			if value, ok, err := claimInt64(record, "monthly_outflow_cents"); err != nil {
				return state.CashflowState{}, err
			} else if ok {
				next.MonthlyOutflowCents = value
			}
			if value, ok, err := claimInt64(record, "monthly_net_income_cents"); err != nil {
				return state.CashflowState{}, err
			} else if ok {
				next.MonthlyNetIncomeCents = value
			}
			if value, ok, err := claimInt64(record, "monthly_fixed_expense_cents"); err != nil {
				return state.CashflowState{}, err
			} else if ok {
				next.MonthlyFixedExpenseCents = value
			}
			if value, ok, err := claimInt64(record, "monthly_variable_expense_cents"); err != nil {
				return state.CashflowState{}, err
			} else if ok {
				next.MonthlyVariableExpenseCents = value
			}
			if record.TimeRange.Start != nil {
				next.LastComputedMonth = record.TimeRange.Start.UTC().Format("2006-01")
			}
		case observation.EvidenceTypeEventSignal:
			if value, ok, err := claimInt64(record, "monthly_income_delta_cents"); err != nil {
				return state.CashflowState{}, err
			} else if ok {
				next.MonthlyInflowCents += value
				next.MonthlyNetIncomeCents += value
			}
			if value, ok, err := claimInt64(record, "housing_cost_delta_cents"); err != nil {
				return state.CashflowState{}, err
			} else if ok {
				next.MonthlyOutflowCents += value
				next.MonthlyFixedExpenseCents += value
				next.MonthlyNetIncomeCents -= value
			}
			if value, ok, err := claimInt64(record, "childcare_cost_delta_cents"); err != nil {
				return state.CashflowState{}, err
			} else if ok {
				next.MonthlyOutflowCents += value
				next.MonthlyVariableExpenseCents += value
				next.MonthlyNetIncomeCents -= value
			}
		}
	}
	if next.MonthlyInflowCents > 0 {
		savings := float64(next.MonthlyInflowCents-next.MonthlyOutflowCents) / float64(next.MonthlyInflowCents)
		if savings < 0 {
			savings = 0
		}
		next.SavingsRate = roundTo(savings, 4)
	}
	return next, nil
}

func (LiabilityReducer) Reduce(current state.LiabilityState, evidence []observation.EvidenceRecord) (state.LiabilityState, error) {
	next := current
	for _, record := range evidence {
		switch record.Type {
		case observation.EvidenceTypeDebtObligationSnapshot:
			if value, ok, err := claimInt64(record, "total_debt_cents"); err != nil {
				return state.LiabilityState{}, err
			} else if ok {
				next.TotalDebtCents = value
			}
			if value, ok, err := claimFloat64(record, "average_apr"); err != nil {
				return state.LiabilityState{}, err
			} else if ok {
				next.AverageAPR = value
			}
			if value, ok, err := claimFloat64(record, "debt_burden_ratio"); err != nil {
				return state.LiabilityState{}, err
			} else if ok {
				next.DebtBurdenRatio = value
			}
			if value, ok, err := claimFloat64(record, "minimum_payment_pressure"); err != nil {
				return state.LiabilityState{}, err
			} else if ok {
				next.MinimumPaymentPressure = value
			}
			if accounts, ok, err := claimLiabilityAccounts(record, "accounts"); err != nil {
				return state.LiabilityState{}, err
			} else if ok {
				next.Accounts = accounts
			}
		case observation.EvidenceTypeEventSignal:
			if value, ok, err := claimInt64(record, "mortgage_balance_cents"); err != nil {
				return state.LiabilityState{}, err
			} else if ok && value > 0 {
				next.TotalDebtCents = value
			}
		}
	}
	return next, nil
}

func (PortfolioReducer) Reduce(current state.PortfolioState, evidence []observation.EvidenceRecord) (state.PortfolioState, error) {
	next := current
	for _, record := range evidence {
		switch record.Type {
		case observation.EvidenceTypePortfolioAllocationSnap, observation.EvidenceTypeBrokerStatement:
			if value, ok, err := claimInt64(record, "total_investable_assets_cents"); err != nil {
				return state.PortfolioState{}, err
			} else if ok {
				next.TotalInvestableAssetsCents = value
			}
			if value, ok, err := claimMapFloat(record, "asset_allocations"); err != nil {
				return state.PortfolioState{}, err
			} else if ok {
				next.AssetAllocations = value
			}
			if value, ok, err := claimMapFloat(record, "target_allocations"); err != nil {
				return state.PortfolioState{}, err
			} else if ok {
				next.TargetAllocations = value
			}
			if value, ok, err := claimMapFloat(record, "allocation_drift"); err != nil {
				return state.PortfolioState{}, err
			} else if ok {
				next.AllocationDrift = value
			}
			if value, ok, err := claimFloat64(record, "emergency_fund_months"); err != nil {
				return state.PortfolioState{}, err
			} else if ok {
				next.EmergencyFundMonths = value
			}
		}
	}
	return next, nil
}

func (TaxReducer) Reduce(current state.TaxState, evidence []observation.EvidenceRecord) (state.TaxState, error) {
	next := current
	for _, record := range evidence {
		switch record.Type {
		case observation.EvidenceTypePayslipStatement:
			if value, ok, err := claimFloat64(record, "effective_tax_rate"); err != nil {
				return state.TaxState{}, err
			} else if ok {
				next.EffectiveTaxRate = value
			}
			if value, ok, err := claimInt64(record, "childcare_benefit_cents"); err != nil {
				return state.TaxState{}, err
			} else if ok {
				next.TaxAdvantagedContributionCents = value
			}
			if value, ok, err := claimBool(record, "childcare_tax_signal"); err != nil {
				return state.TaxState{}, err
			} else if ok {
				next.ChildcareTaxSignal = value
			}
		case observation.EvidenceTypeTaxDocument:
			if value, ok, err := claimBool(record, "childcare_tax_signal"); err != nil {
				return state.TaxState{}, err
			} else if ok {
				next.ChildcareTaxSignal = value
			}
			if value, ok, err := claimString(record, "tax_signal_reason"); err != nil {
				return state.TaxState{}, err
			} else if ok && value != "" {
				next.FamilyTaxNotes = appendUnique(next.FamilyTaxNotes, value)
			}
		case observation.EvidenceTypeEventSignal:
			if value, ok, err := claimBool(record, "childcare_tax_signal"); err != nil {
				return state.TaxState{}, err
			} else if ok {
				next.ChildcareTaxSignal = value
			}
			if value, ok, err := claimBool(record, "withholding_review_required"); err != nil {
				return state.TaxState{}, err
			} else if ok && value {
				next.FamilyTaxNotes = appendUnique(next.FamilyTaxNotes, "withholding_review_required")
			}
		case observation.EvidenceTypeCalendarDeadline:
			if value, ok, err := claimString(record, "description"); err != nil {
				return state.TaxState{}, err
			} else if ok && value != "" {
				next.UpcomingDeadlines = appendUnique(next.UpcomingDeadlines, value)
			}
		}
	}
	return next, nil
}

func (BehaviorReducer) Reduce(current state.BehaviorState, evidence []observation.EvidenceRecord) (state.BehaviorState, error) {
	next := current
	for _, record := range evidence {
		switch record.Type {
		case observation.EvidenceTypeRecurringSubscription:
			if value, ok, err := claimInt(record, "duplicate_subscription_count"); err != nil {
				return state.BehaviorState{}, err
			} else if ok {
				next.DuplicateSubscriptionCount = value
			}
			if value, ok, err := claimStringSlice(record, "recurring_subscription_merchants"); err != nil {
				return state.BehaviorState{}, err
			} else if ok {
				next.RecurringSubscriptions = value
			}
		case observation.EvidenceTypeLateNightSpendingSignal:
			if value, ok, err := claimFloat64(record, "late_night_spending_frequency"); err != nil {
				return state.BehaviorState{}, err
			} else if ok {
				next.LateNightSpendingFrequency = value
			}
		}
	}

	next.AnomalyFlags = next.AnomalyFlags[:0]
	next.InterventionQueue = next.InterventionQueue[:0]
	if next.LateNightSpendingFrequency >= 0.3 {
		next.AnomalyFlags = append(next.AnomalyFlags, "late_night_spending")
		next.InterventionQueue = append(next.InterventionQueue, "review late-night discretionary spending")
	}
	if next.DuplicateSubscriptionCount >= 2 {
		next.AnomalyFlags = append(next.AnomalyFlags, "duplicate_subscriptions")
		next.InterventionQueue = append(next.InterventionQueue, "review recurring subscriptions")
	}
	return next, nil
}

func (RiskReducer) Reduce(
	current state.RiskState,
	cashflow state.CashflowState,
	liability state.LiabilityState,
	portfolio state.PortfolioState,
	tax state.TaxState,
	behavior state.BehaviorState,
) state.RiskState {
	next := current
	switch {
	case cashflow.SavingsRate < 0.05 || portfolio.EmergencyFundMonths < 1:
		next.LiquidityRisk = "high"
	case cashflow.SavingsRate < 0.2 || portfolio.EmergencyFundMonths < 3:
		next.LiquidityRisk = "medium"
	default:
		next.LiquidityRisk = "low"
	}

	maxAllocationDrift := 0.0
	for _, drift := range portfolio.AllocationDrift {
		value := drift
		if value < 0 {
			value = -value
		}
		if value > maxAllocationDrift {
			maxAllocationDrift = value
		}
	}
	switch {
	case maxAllocationDrift >= 0.15:
		next.ConcentrationRisk = "high"
	case maxAllocationDrift >= 0.08:
		next.ConcentrationRisk = "medium"
	default:
		next.ConcentrationRisk = "low"
	}

	switch {
	case liability.DebtBurdenRatio >= 0.35 || liability.MinimumPaymentPressure >= 0.2:
		next.DebtStressLevel = "high"
	case liability.DebtBurdenRatio >= 0.2 || liability.MinimumPaymentPressure >= 0.1:
		next.DebtStressLevel = "medium"
	default:
		next.DebtStressLevel = "low"
	}

	next.ComplianceFlags = next.ComplianceFlags[:0]
	if tax.ChildcareTaxSignal {
		next.ComplianceFlags = append(next.ComplianceFlags, "family_tax_followup")
	}
	if behavior.DuplicateSubscriptionCount >= 2 {
		next.ComplianceFlags = append(next.ComplianceFlags, "subscription_review")
	}

	if next.LiquidityRisk == "high" || next.DebtStressLevel == "high" || next.ConcentrationRisk == "high" {
		next.OverallRisk = "high"
	} else if next.LiquidityRisk == "medium" || next.ConcentrationRisk == "medium" || next.DebtStressLevel == "medium" {
		next.OverallRisk = "medium"
	} else {
		next.OverallRisk = "low"
	}
	return next
}

func (WorkflowReducer) Reduce(
	current state.WorkflowState,
	taskID string,
	workflowID string,
	phase string,
	evidence []observation.EvidenceRecord,
	now time.Time,
) state.WorkflowState {
	next := current
	next.ActiveWorkflowID = workflowID
	next.LastTaskID = taskID
	if phase != "" {
		next.Phase = phase
	}
	next.LastUpdatedAt = now
	if len(evidence) > 0 {
		next.EvidenceStatus = "observed"
	}
	return next
}

func buildMutations(evidence []observation.EvidenceRecord, blocks map[string]any) ([]state.StateMutation, error) {
	if len(evidence) == 0 {
		return nil, fmt.Errorf("build mutations requires evidence")
	}
	keys := make([]string, 0, len(blocks))
	for key := range blocks {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	primaryEvidenceID := evidence[0].ID
	mutations := make([]state.StateMutation, 0, len(keys))
	for _, key := range keys {
		payload, err := json.Marshal(blocks[key])
		if err != nil {
			return nil, err
		}
		mutations = append(mutations, state.StateMutation{
			Path:       key,
			Operation:  "replace",
			ValueJSON:  string(payload),
			EvidenceID: primaryEvidenceID,
		})
	}
	return mutations, nil
}

func claimInt64(record observation.EvidenceRecord, predicate string) (int64, bool, error) {
	for _, claim := range record.Claims {
		if claim.Predicate != predicate {
			continue
		}
		var value int64
		if err := json.Unmarshal([]byte(claim.ValueJSON), &value); err != nil {
			return 0, false, err
		}
		return value, true, nil
	}
	return 0, false, nil
}

func claimInt(record observation.EvidenceRecord, predicate string) (int, bool, error) {
	for _, claim := range record.Claims {
		if claim.Predicate != predicate {
			continue
		}
		var value int
		if err := json.Unmarshal([]byte(claim.ValueJSON), &value); err != nil {
			return 0, false, err
		}
		return value, true, nil
	}
	return 0, false, nil
}

func claimFloat64(record observation.EvidenceRecord, predicate string) (float64, bool, error) {
	for _, claim := range record.Claims {
		if claim.Predicate != predicate {
			continue
		}
		var value float64
		if err := json.Unmarshal([]byte(claim.ValueJSON), &value); err != nil {
			return 0, false, err
		}
		return value, true, nil
	}
	return 0, false, nil
}

func claimBool(record observation.EvidenceRecord, predicate string) (bool, bool, error) {
	for _, claim := range record.Claims {
		if claim.Predicate != predicate {
			continue
		}
		var value bool
		if err := json.Unmarshal([]byte(claim.ValueJSON), &value); err != nil {
			return false, false, err
		}
		return value, true, nil
	}
	return false, false, nil
}

func claimString(record observation.EvidenceRecord, predicate string) (string, bool, error) {
	for _, claim := range record.Claims {
		if claim.Predicate != predicate {
			continue
		}
		var value string
		if err := json.Unmarshal([]byte(claim.ValueJSON), &value); err != nil {
			return "", false, err
		}
		return value, true, nil
	}
	return "", false, nil
}

func claimStringSlice(record observation.EvidenceRecord, predicate string) ([]string, bool, error) {
	for _, claim := range record.Claims {
		if claim.Predicate != predicate {
			continue
		}
		var value []string
		if err := json.Unmarshal([]byte(claim.ValueJSON), &value); err != nil {
			return nil, false, err
		}
		return value, true, nil
	}
	return nil, false, nil
}

func claimMapFloat(record observation.EvidenceRecord, predicate string) (map[string]float64, bool, error) {
	for _, claim := range record.Claims {
		if claim.Predicate != predicate {
			continue
		}
		var value map[string]float64
		if err := json.Unmarshal([]byte(claim.ValueJSON), &value); err != nil {
			return nil, false, err
		}
		return value, true, nil
	}
	return nil, false, nil
}

func claimLiabilityAccounts(record observation.EvidenceRecord, predicate string) ([]state.LiabilityAccount, bool, error) {
	for _, claim := range record.Claims {
		if claim.Predicate != predicate {
			continue
		}
		var raw []struct {
			Name            string  `json:"name"`
			BalanceCents    int64   `json:"balance_cents"`
			AnnualRate      float64 `json:"annual_rate"`
			MinimumDueCents int64   `json:"minimum_due_cents"`
		}
		if err := json.Unmarshal([]byte(claim.ValueJSON), &raw); err != nil {
			return nil, false, err
		}
		accounts := make([]state.LiabilityAccount, 0, len(raw))
		for _, item := range raw {
			accounts = append(accounts, state.LiabilityAccount{
				Name:            item.Name,
				BalanceCents:    item.BalanceCents,
				AnnualRate:      item.AnnualRate,
				MinimumDueCents: item.MinimumDueCents,
			})
		}
		return accounts, true, nil
	}
	return nil, false, nil
}

func appendUnique(items []string, value string) []string {
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}

func roundTo(value float64, precision int) float64 {
	scale := 1.0
	for i := 0; i < precision; i++ {
		scale *= 10
	}
	return float64(int(value*scale+0.5)) / scale
}
