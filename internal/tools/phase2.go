package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kobelakers/personal-cfo-os/internal/observation"
	"github.com/kobelakers/personal-cfo-os/internal/state"
)

type QueryTransactionTool struct {
	Adapter observation.ObservationAdapter
}

func (t QueryTransactionTool) Name() string { return "query_transaction_tool" }

func (t QueryTransactionTool) Query(ctx context.Context, input map[string]string) (string, error) {
	records, err := t.QueryEvidence(ctx, input)
	if err != nil {
		return "", err
	}
	return marshalString(records)
}

func (t QueryTransactionTool) QueryEvidence(ctx context.Context, input map[string]string) ([]observation.EvidenceRecord, error) {
	if t.Adapter == nil {
		return nil, fmt.Errorf("query transaction tool requires adapter")
	}
	records, err := t.Adapter.Observe(ctx, observation.ObservationRequest{
		TaskID:     input["task_id"],
		SourceKind: "ledger",
		Params:     input,
	})
	if err != nil {
		return nil, err
	}
	return filterEvidence(records, observation.EvidenceTypeTransactionBatch, observation.EvidenceTypeRecurringSubscription, observation.EvidenceTypeLateNightSpendingSignal), nil
}

type QueryLiabilityTool struct {
	Adapter observation.ObservationAdapter
}

func (t QueryLiabilityTool) Name() string { return "query_liability_tool" }

func (t QueryLiabilityTool) Query(ctx context.Context, input map[string]string) (string, error) {
	records, err := t.QueryEvidence(ctx, input)
	if err != nil {
		return "", err
	}
	return marshalString(records)
}

func (t QueryLiabilityTool) QueryEvidence(ctx context.Context, input map[string]string) ([]observation.EvidenceRecord, error) {
	if t.Adapter == nil {
		return nil, fmt.Errorf("query liability tool requires adapter")
	}
	records, err := t.Adapter.Observe(ctx, observation.ObservationRequest{
		TaskID:     input["task_id"],
		SourceKind: "ledger",
		Params:     input,
	})
	if err != nil {
		return nil, err
	}
	return filterEvidence(records, observation.EvidenceTypeDebtObligationSnapshot), nil
}

type QueryPortfolioTool struct {
	LedgerAdapter   observation.ObservationAdapter
	DocumentAdapter observation.ObservationAdapter
}

func (t QueryPortfolioTool) Name() string { return "query_portfolio_tool" }

func (t QueryPortfolioTool) Query(ctx context.Context, input map[string]string) (string, error) {
	records, err := t.QueryEvidence(ctx, input)
	if err != nil {
		return "", err
	}
	return marshalString(records)
}

func (t QueryPortfolioTool) QueryEvidence(ctx context.Context, input map[string]string) ([]observation.EvidenceRecord, error) {
	records := make([]observation.EvidenceRecord, 0, 2)
	if t.LedgerAdapter != nil {
		ledgerRecords, err := t.LedgerAdapter.Observe(ctx, observation.ObservationRequest{
			TaskID:     input["task_id"],
			SourceKind: "ledger",
			Params:     input,
		})
		if err != nil {
			return nil, err
		}
		records = append(records, filterEvidence(ledgerRecords, observation.EvidenceTypePortfolioAllocationSnap)...)
	}
	if t.DocumentAdapter != nil {
		documentRecords, err := t.DocumentAdapter.Observe(ctx, observation.ObservationRequest{
			TaskID:     input["task_id"],
			SourceKind: "document",
			Params:     input,
		})
		if err != nil {
			return nil, err
		}
		records = append(records, filterEvidence(documentRecords, observation.EvidenceTypeBrokerStatement)...)
	}
	return records, nil
}

type ParseDocumentTool struct {
	Structured observation.ObservationAdapter
	Agentic    observation.ObservationAdapter
}

func (t ParseDocumentTool) Name() string { return "parse_document_tool" }

func (t ParseDocumentTool) Parse(ctx context.Context, _ []byte) (string, error) {
	records, err := t.ParseEvidence(ctx, map[string]string{})
	if err != nil {
		return "", err
	}
	return marshalString(records)
}

func (t ParseDocumentTool) ParseEvidence(ctx context.Context, input map[string]string) ([]observation.EvidenceRecord, error) {
	records := make([]observation.EvidenceRecord, 0, 4)
	if t.Structured != nil {
		structuredRecords, err := t.Structured.Observe(ctx, observation.ObservationRequest{
			TaskID:     input["task_id"],
			SourceKind: "document",
			Params:     input,
		})
		if err != nil {
			return nil, err
		}
		records = append(records, structuredRecords...)
	}
	if t.Agentic != nil {
		agenticRecords, err := t.Agentic.Observe(ctx, observation.ObservationRequest{
			TaskID:     input["task_id"],
			SourceKind: "document",
			Params:     input,
		})
		if err != nil {
			return nil, err
		}
		records = append(records, agenticRecords...)
	}
	return records, nil
}

type ComputeCashflowMetricsTool struct{}

func (t ComputeCashflowMetricsTool) Name() string { return "compute_cashflow_metrics_tool" }

func (t ComputeCashflowMetricsTool) Simulate(_ context.Context, input map[string]string) (string, error) {
	return marshalString(input)
}

func (t ComputeCashflowMetricsTool) Compute(current state.FinancialWorldState) map[string]any {
	return map[string]any{
		"monthly_inflow_cents":          current.CashflowState.MonthlyInflowCents,
		"monthly_outflow_cents":         current.CashflowState.MonthlyOutflowCents,
		"monthly_net_income_cents":      current.CashflowState.MonthlyNetIncomeCents,
		"savings_rate":                  current.CashflowState.SavingsRate,
		"late_night_spending_frequency": current.BehaviorState.LateNightSpendingFrequency,
		"duplicate_subscription_count":  current.BehaviorState.DuplicateSubscriptionCount,
	}
}

type ComputeDebtDecisionMetricsTool struct{}

func (t ComputeDebtDecisionMetricsTool) Name() string { return "compute_debt_decision_metrics_tool" }

func (t ComputeDebtDecisionMetricsTool) Simulate(_ context.Context, input map[string]string) (string, error) {
	return marshalString(input)
}

func (t ComputeDebtDecisionMetricsTool) Compute(current state.FinancialWorldState) map[string]any {
	return map[string]any{
		"debt_burden_ratio":        current.LiabilityState.DebtBurdenRatio,
		"minimum_payment_pressure": current.LiabilityState.MinimumPaymentPressure,
		"average_apr":              current.LiabilityState.AverageAPR,
		"monthly_net_income_cents": current.CashflowState.MonthlyNetIncomeCents,
		"allocation_drift":         current.PortfolioState.AllocationDrift,
		"overall_risk":             current.RiskState.OverallRisk,
	}
}

type ComputeTaxSignalTool struct{}

func (t ComputeTaxSignalTool) Name() string { return "compute_tax_signal_tool" }

func (t ComputeTaxSignalTool) Simulate(_ context.Context, input map[string]string) (string, error) {
	return marshalString(input)
}

func (t ComputeTaxSignalTool) Compute(current state.FinancialWorldState) map[string]any {
	return map[string]any{
		"effective_tax_rate":          current.TaxState.EffectiveTaxRate,
		"childcare_tax_signal":        current.TaxState.ChildcareTaxSignal,
		"tax_advantaged_contribution": current.TaxState.TaxAdvantagedContributionCents,
		"family_tax_notes":            current.TaxState.FamilyTaxNotes,
	}
}

type GenerateTaskArtifactTool struct{}

func (t GenerateTaskArtifactTool) Name() string { return "generate_task_artifact_tool" }

func (t GenerateTaskArtifactTool) Execute(_ context.Context, input map[string]string) (string, error) {
	return marshalString(input)
}

func (t GenerateTaskArtifactTool) Generate(content any) (string, error) {
	return marshalString(content)
}

func filterEvidence(records []observation.EvidenceRecord, types ...observation.EvidenceType) []observation.EvidenceRecord {
	allowed := make(map[observation.EvidenceType]struct{}, len(types))
	for _, evidenceType := range types {
		allowed[evidenceType] = struct{}{}
	}
	filtered := make([]observation.EvidenceRecord, 0, len(records))
	for _, record := range records {
		if _, ok := allowed[record.Type]; ok {
			filtered = append(filtered, record)
		}
	}
	return filtered
}

func marshalString(value any) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}
