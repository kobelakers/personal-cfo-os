package observation

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

type EventObservationAdapter struct {
	AdapterName string
	Events      []LifeEventRecord
	Normalizer  EvidenceNormalizer
	Now         func() time.Time
}

func (a EventObservationAdapter) SourceType() string {
	return "event"
}

func (a EventObservationAdapter) Observe(ctx context.Context, request ObservationRequest) ([]EvidenceRecord, error) {
	_ = ctx
	normalizer := a.Normalizer
	if normalizer == nil {
		normalizer = CanonicalEvidenceNormalizer{}
	}
	now := time.Now().UTC()
	if a.Now != nil {
		now = a.Now().UTC()
	}
	userID := strings.TrimSpace(request.Params["user_id"])
	eventID := strings.TrimSpace(request.Params["event_id"])
	eventKind := strings.TrimSpace(request.Params["event_kind"])

	selected := make([]LifeEventRecord, 0, len(a.Events))
	for _, event := range a.Events {
		if userID != "" && event.UserID != userID {
			continue
		}
		if eventID != "" && event.ID != eventID {
			continue
		}
		if eventKind != "" && string(event.Kind) != eventKind {
			continue
		}
		selected = append(selected, event)
	}

	records := make([]EvidenceRecord, 0, len(selected))
	for _, event := range selected {
		if err := event.Validate(); err != nil {
			return nil, err
		}
		record, err := buildLifeEventEvidence(event, now)
		if err != nil {
			return nil, err
		}
		normalized, err := normalizer.Normalize(ctx, record)
		if err != nil {
			return nil, err
		}
		records = append(records, normalized)
	}
	return records, nil
}

type CalendarDeadlineObservationAdapter struct {
	AdapterName string
	Deadlines   []CalendarDeadlineRecord
	Normalizer  EvidenceNormalizer
	Now         func() time.Time
}

func (a CalendarDeadlineObservationAdapter) SourceType() string {
	return "calendar_deadline"
}

func (a CalendarDeadlineObservationAdapter) Observe(ctx context.Context, request ObservationRequest) ([]EvidenceRecord, error) {
	_ = ctx
	normalizer := a.Normalizer
	if normalizer == nil {
		normalizer = CanonicalEvidenceNormalizer{}
	}
	now := time.Now().UTC()
	if a.Now != nil {
		now = a.Now().UTC()
	}
	userID := strings.TrimSpace(request.Params["user_id"])
	eventID := strings.TrimSpace(request.Params["event_id"])
	eventKind := strings.TrimSpace(request.Params["event_kind"])

	selected := make([]CalendarDeadlineRecord, 0, len(a.Deadlines))
	for _, deadline := range a.Deadlines {
		if userID != "" && deadline.UserID != userID {
			continue
		}
		if eventID != "" && deadline.RelatedEventID != "" && deadline.RelatedEventID != eventID {
			continue
		}
		if eventKind != "" && string(deadline.RelatedEventKind) != eventKind {
			continue
		}
		selected = append(selected, deadline)
	}
	sort.Slice(selected, func(i, j int) bool {
		return selected[i].DeadlineAt.Before(selected[j].DeadlineAt)
	})

	records := make([]EvidenceRecord, 0, len(selected))
	for _, deadline := range selected {
		if err := deadline.Validate(); err != nil {
			return nil, err
		}
		record, err := buildCalendarDeadlineEvidence(deadline, now)
		if err != nil {
			return nil, err
		}
		normalized, err := normalizer.Normalize(ctx, record)
		if err != nil {
			return nil, err
		}
		records = append(records, normalized)
	}
	return records, nil
}

func buildLifeEventEvidence(event LifeEventRecord, observedAt time.Time) (EvidenceRecord, error) {
	claims := []EvidenceClaim{
		{Subject: "life_event", Predicate: "life_event_kind", Object: event.ID, ValueJSON: mustJSON(string(event.Kind))},
	}
	summary := "life event trigger"
	start := event.WindowStart()
	end := event.WindowEnd()

	switch event.Kind {
	case LifeEventSalaryChange:
		claims = append(claims,
			EvidenceClaim{Subject: "cashflow", Predicate: "monthly_income_delta_cents", Object: event.ID, ValueJSON: mustJSON(event.SalaryChange.NewMonthlyIncomeCents - event.SalaryChange.PreviousMonthlyIncomeCents)},
			EvidenceClaim{Subject: "event", Predicate: "effective_at", Object: event.ID, ValueJSON: mustJSON(event.SalaryChange.EffectiveAt.UTC().Format(time.RFC3339))},
		)
		summary = "salary change event normalized into cashflow/tax/portfolio impact evidence"
	case LifeEventNewChild:
		claims = append(claims,
			EvidenceClaim{Subject: "tax", Predicate: "childcare_tax_signal", Object: event.ID, ValueJSON: mustJSON(event.NewChild.ChildcareTaxEligible)},
			EvidenceClaim{Subject: "cashflow", Predicate: "childcare_cost_delta_cents", Object: event.ID, ValueJSON: mustJSON(event.NewChild.EstimatedMonthlyCostCents)},
			EvidenceClaim{Subject: "event", Predicate: "expected_care_start_at", Object: event.ID, ValueJSON: mustJSON(event.NewChild.ExpectedCareStartAt.UTC().Format(time.RFC3339))},
		)
		summary = "new child event normalized into childcare tax and cashflow impact evidence"
	case LifeEventJobChange:
		claims = append(claims,
			EvidenceClaim{Subject: "cashflow", Predicate: "monthly_income_delta_cents", Object: event.ID, ValueJSON: mustJSON(event.JobChange.NewMonthlyIncomeCents - event.JobChange.PreviousMonthlyIncomeCents)},
			EvidenceClaim{Subject: "tax", Predicate: "withholding_review_required", Object: event.ID, ValueJSON: mustJSON(true)},
			EvidenceClaim{Subject: "event", Predicate: "benefits_enrollment_deadline_at", Object: event.ID, ValueJSON: mustJSON(event.JobChange.BenefitsEnrollmentDeadlineAt.UTC().Format(time.RFC3339))},
		)
		summary = "job change event normalized into compensation and withholding impact evidence"
	case LifeEventHousingChange:
		claims = append(claims,
			EvidenceClaim{Subject: "cashflow", Predicate: "housing_cost_delta_cents", Object: event.ID, ValueJSON: mustJSON(event.HousingChange.NewMonthlyHousingCostCents - event.HousingChange.PreviousMonthlyHousingCostCents)},
			EvidenceClaim{Subject: "liability", Predicate: "mortgage_balance_cents", Object: event.ID, ValueJSON: mustJSON(event.HousingChange.MortgageBalanceCents)},
			EvidenceClaim{Subject: "event", Predicate: "effective_at", Object: event.ID, ValueJSON: mustJSON(event.HousingChange.EffectiveAt.UTC().Format(time.RFC3339))},
		)
		summary = "housing change event normalized into housing-cost and liability impact evidence"
	default:
		return EvidenceRecord{}, fmt.Errorf("unsupported life event kind %q", event.Kind)
	}

	return EvidenceRecord{
		ID:   EvidenceID("evidence-life-event-" + event.ID),
		Type: EvidenceTypeEventSignal,
		Source: EvidenceSource{
			Kind:       "life_event",
			Adapter:    "event",
			Reference:  event.ID,
			Provenance: event.Provenance,
		},
		TimeRange: EvidenceTimeRange{
			ObservedAt: observedAt,
			Start:      start,
			End:        end,
		},
		Confidence: EvidenceConfidence{
			Score:  event.Confidence,
			Reason: "derived from structured life event source",
		},
		Claims:        claims,
		Normalization: EvidenceNormalizationResult{Status: EvidenceNormalizationNormalized},
		Summary:       summary,
		CreatedAt:     observedAt,
	}, nil
}

func buildCalendarDeadlineEvidence(deadline CalendarDeadlineRecord, observedAt time.Time) (EvidenceRecord, error) {
	return EvidenceRecord{
		ID:   EvidenceID("evidence-calendar-deadline-" + deadline.ID),
		Type: EvidenceTypeCalendarDeadline,
		Source: EvidenceSource{
			Kind:       "calendar_deadline",
			Adapter:    "calendar_deadline",
			Reference:  deadline.ID,
			Provenance: deadline.Provenance,
		},
		TimeRange: EvidenceTimeRange{
			ObservedAt: observedAt,
			Start:      &deadline.ObservedAt,
			End:        &deadline.DeadlineAt,
		},
		Confidence: EvidenceConfidence{
			Score:  deadline.Confidence,
			Reason: "derived from structured deadline source",
		},
		Claims: []EvidenceClaim{
			{Subject: "deadline", Predicate: "deadline_kind", Object: deadline.ID, ValueJSON: mustJSON(deadline.Kind)},
			{Subject: "deadline", Predicate: "deadline_at", Object: deadline.ID, ValueJSON: mustJSON(deadline.DeadlineAt.UTC().Format(time.RFC3339))},
			{Subject: "deadline", Predicate: "description", Object: deadline.ID, ValueJSON: mustJSON(deadline.Description)},
			{Subject: "deadline", Predicate: "related_event_kind", Object: deadline.ID, ValueJSON: mustJSON(string(deadline.RelatedEventKind))},
		},
		Normalization: EvidenceNormalizationResult{Status: EvidenceNormalizationNormalized},
		Summary:       deadline.Description,
		CreatedAt:     observedAt,
	}, nil
}

func mustJSON(value any) string {
	payload, _ := json.Marshal(value)
	return string(payload)
}
