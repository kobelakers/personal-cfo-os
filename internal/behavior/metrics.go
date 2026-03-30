package behavior

import (
	"encoding/json"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/finance"
	"github.com/kobelakers/personal-cfo-os/internal/observation"
)

func ComputeMetrics(evidence BehaviorEvidence, asOf time.Time) BehaviorMetricBundle {
	current := evidence.CurrentState
	lateNightCount := claimInt(evidence.Evidence, "late_night_spending_count")
	lateNightRatio := claimFloat(evidence.Evidence, "late_night_spending_frequency")
	if lateNightRatio == 0 {
		lateNightRatio = current.BehaviorState.LateNightSpendingFrequency
	}
	duplicateCount := claimInt(evidence.Evidence, "duplicate_subscription_count")
	if duplicateCount == 0 {
		duplicateCount = current.BehaviorState.DuplicateSubscriptionCount
	}
	recurringCount := len(current.BehaviorState.RecurringSubscriptions)
	variableExpense := current.CashflowState.MonthlyVariableExpenseCents
	netIncome := current.CashflowState.MonthlyNetIncomeCents
	outflow := current.CashflowState.MonthlyOutflowCents
	var variableRatio float64
	if outflow > 0 {
		variableRatio = float64(variableExpense) / float64(outflow)
	}
	netPressure := 0.0
	if netIncome < 0 {
		netPressure = 1.0
	} else if current.CashflowState.SavingsRate < 0.10 {
		netPressure = 0.4
	}
	duplicatePressure := float64(duplicateCount) / 3.0
	if duplicatePressure > 1 {
		duplicatePressure = 1
	}
	score := 0.45*clamp(variableRatio) + 0.30*clamp(lateNightRatio) + 0.15*duplicatePressure + 0.10*netPressure
	score = round(score, 4)

	metricRefs := []finance.MetricRecord{
		{
			Ref:          "duplicate_subscription_count",
			Domain:       "behavior",
			Name:         "duplicate subscription count",
			ValueType:    finance.MetricValueTypeInt64,
			Int64Value:   int64(duplicateCount),
			Unit:         "count",
			AsOf:         asOf,
			EvidenceRefs: evidenceIDs(evidence.Evidence, observation.EvidenceTypeRecurringSubscription),
			Derivation:   "derived from recurring subscription evidence or behavior state",
		},
		{
			Ref:          "late_night_spend_count",
			Domain:       "behavior",
			Name:         "late night spend count",
			ValueType:    finance.MetricValueTypeInt64,
			Int64Value:   int64(lateNightCount),
			Unit:         "count",
			AsOf:         asOf,
			EvidenceRefs: evidenceIDs(evidence.Evidence, observation.EvidenceTypeLateNightSpendingSignal),
			Derivation:   "derived from late-night spending evidence",
		},
		{
			Ref:          "late_night_spend_ratio",
			Domain:       "behavior",
			Name:         "late night spend ratio",
			ValueType:    finance.MetricValueTypeFloat64,
			Float64Value: round(lateNightRatio, 4),
			Unit:         "ratio",
			AsOf:         asOf,
			EvidenceRefs: evidenceIDs(evidence.Evidence, observation.EvidenceTypeLateNightSpendingSignal),
			Derivation:   "derived from late-night spending evidence or behavior state",
		},
		{
			Ref:          "discretionary_pressure_score",
			Domain:       "behavior",
			Name:         "discretionary pressure score",
			ValueType:    finance.MetricValueTypeFloat64,
			Float64Value: score,
			Unit:         "score",
			AsOf:         asOf,
			EvidenceRefs: evidenceIDs(evidence.Evidence, observation.EvidenceTypeTransactionBatch, observation.EvidenceTypeRecurringSubscription, observation.EvidenceTypeLateNightSpendingSignal),
			Derivation:   "weighted combination of variable expense pressure, late-night ratio, duplicate subscriptions, and net income pressure",
		},
		{
			Ref:          "recurring_subscription_count",
			Domain:       "behavior",
			Name:         "recurring subscription merchant count",
			ValueType:    finance.MetricValueTypeInt64,
			Int64Value:   int64(recurringCount),
			Unit:         "count",
			AsOf:         asOf,
			EvidenceRefs: evidenceIDs(evidence.Evidence, observation.EvidenceTypeRecurringSubscription),
			Derivation:   "derived from behavior state recurring subscriptions",
		},
	}
	return BehaviorMetricBundle{
		Metrics: BehaviorMetrics{
			DuplicateSubscriptionCount: duplicateCount,
			LateNightSpendCount:        lateNightCount,
			LateNightSpendRatio:        round(lateNightRatio, 4),
			DiscretionaryPressureScore: score,
			RecurringSubscriptionCount: recurringCount,
			MonthlyVariableExpenseCents: variableExpense,
			MonthlyNetIncomeCents:       netIncome,
		},
		Records: metricRefs,
	}
}

func evidenceIDs(records []observation.EvidenceRecord, allowed ...observation.EvidenceType) []string {
	typeSet := make(map[observation.EvidenceType]struct{}, len(allowed))
	for _, item := range allowed {
		typeSet[item] = struct{}{}
	}
	result := make([]string, 0)
	for _, record := range records {
		if len(typeSet) > 0 {
			if _, ok := typeSet[record.Type]; !ok {
				continue
			}
		}
		result = append(result, string(record.ID))
	}
	return result
}

func claimInt(records []observation.EvidenceRecord, predicate string) int {
	for _, record := range records {
		for _, claim := range record.Claims {
			if claim.Predicate != predicate {
				continue
			}
			var value int
			if err := json.Unmarshal([]byte(claim.ValueJSON), &value); err == nil {
				return value
			}
		}
	}
	return 0
}

func claimFloat(records []observation.EvidenceRecord, predicate string) float64 {
	for _, record := range records {
		for _, claim := range record.Claims {
			if claim.Predicate != predicate {
				continue
			}
			var value float64
			if err := json.Unmarshal([]byte(claim.ValueJSON), &value); err == nil {
				return value
			}
		}
	}
	return 0
}

func clamp(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func round(value float64, places int) float64 {
	factor := 1.0
	for i := 0; i < places; i++ {
		factor *= 10
	}
	if value >= 0 {
		return float64(int(value*factor+0.5)) / factor
	}
	return float64(int(value*factor-0.5)) / factor
}
