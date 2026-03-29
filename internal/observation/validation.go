package observation

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

func (r EvidenceRecord) Validate() error {
	var errs []error

	if strings.TrimSpace(string(r.ID)) == "" {
		errs = append(errs, errors.New("evidence id is required"))
	}
	if !validEvidenceType(r.Type) {
		errs = append(errs, fmt.Errorf("invalid evidence type %q", r.Type))
	}
	if err := r.Source.Validate(); err != nil {
		errs = append(errs, err)
	}
	if err := r.TimeRange.Validate(); err != nil {
		errs = append(errs, err)
	}
	if err := r.Confidence.Validate(); err != nil {
		errs = append(errs, err)
	}
	if err := r.Normalization.Validate(); err != nil {
		errs = append(errs, err)
	}
	if len(r.Claims) == 0 {
		errs = append(errs, errors.New("evidence must contain at least one claim"))
	}
	for i, claim := range r.Claims {
		if strings.TrimSpace(claim.Subject) == "" {
			errs = append(errs, fmt.Errorf("claim %d subject is required", i))
		}
		if strings.TrimSpace(claim.Predicate) == "" {
			errs = append(errs, fmt.Errorf("claim %d predicate is required", i))
		}
	}
	if r.Artifact != nil && strings.TrimSpace(r.Artifact.ObjectKey) == "" {
		errs = append(errs, errors.New("artifact object key is required when artifact is present"))
	}

	return errors.Join(errs...)
}

func (s EvidenceSource) Validate() error {
	var errs []error
	if strings.TrimSpace(s.Kind) == "" {
		errs = append(errs, errors.New("evidence source kind is required"))
	}
	if strings.TrimSpace(s.Adapter) == "" {
		errs = append(errs, errors.New("evidence source adapter is required"))
	}
	if strings.TrimSpace(s.Reference) == "" {
		errs = append(errs, errors.New("evidence source reference is required"))
	}
	if strings.TrimSpace(s.Provenance) == "" {
		errs = append(errs, errors.New("evidence source provenance is required"))
	}
	return errors.Join(errs...)
}

func (t EvidenceTimeRange) Validate() error {
	if t.ObservedAt.IsZero() {
		return errors.New("observed_at is required")
	}
	if t.Start != nil && t.End != nil && t.Start.After(*t.End) {
		return errors.New("time range start must be before end")
	}
	return nil
}

func (c EvidenceConfidence) Validate() error {
	if c.Score < 0 || c.Score > 1 {
		return errors.New("evidence confidence score must be within [0,1]")
	}
	if strings.TrimSpace(c.Reason) == "" {
		return errors.New("evidence confidence reason is required")
	}
	return nil
}

func (n EvidenceNormalizationResult) Validate() error {
	switch n.Status {
	case EvidenceNormalizationNormalized, EvidenceNormalizationPartial:
		return nil
	case EvidenceNormalizationRejected:
		if strings.TrimSpace(n.RejectedReason) == "" {
			return errors.New("rejected normalization must include a rejected_reason")
		}
		return nil
	default:
		return fmt.Errorf("invalid normalization status %q", n.Status)
	}
}

func validEvidenceType(t EvidenceType) bool {
	switch t {
	case EvidenceTypeLedgerTransaction,
		EvidenceTypeDocumentStatement,
		EvidenceTypeEventSignal,
		EvidenceTypeMarketPolicy,
		EvidenceTypeCalendarDeadline,
		EvidenceTypeExtractedTable,
		EvidenceTypeAgenticParse,
		EvidenceTypeTransactionBatch,
		EvidenceTypeRecurringSubscription,
		EvidenceTypeLateNightSpendingSignal,
		EvidenceTypeDebtObligationSnapshot,
		EvidenceTypePortfolioAllocationSnap,
		EvidenceTypePayslipStatement,
		EvidenceTypeCreditCardStatement,
		EvidenceTypeTaxDocument,
		EvidenceTypeBrokerStatement:
		return true
	default:
		return false
	}
}

func (r LifeEventRecord) Validate() error {
	if strings.TrimSpace(r.ID) == "" {
		return errors.New("life event id is required")
	}
	if strings.TrimSpace(r.UserID) == "" {
		return errors.New("life event user_id is required")
	}
	if !validLifeEventKind(r.Kind) {
		return fmt.Errorf("invalid life event kind %q", r.Kind)
	}
	if strings.TrimSpace(r.Source) == "" {
		return errors.New("life event source is required")
	}
	if strings.TrimSpace(r.Provenance) == "" {
		return errors.New("life event provenance is required")
	}
	if r.ObservedAt.IsZero() {
		return errors.New("life event observed_at is required")
	}
	if r.Confidence < 0 || r.Confidence > 1 {
		return errors.New("life event confidence must be within [0,1]")
	}
	if countNonNilLifeEventPayloads(r) != 1 {
		return errors.New("life event record must include exactly one typed payload")
	}
	switch r.Kind {
	case LifeEventSalaryChange:
		if r.SalaryChange == nil || r.SalaryChange.EffectiveAt.IsZero() {
			return errors.New("salary_change event requires typed payload with effective_at")
		}
	case LifeEventNewChild:
		if r.NewChild == nil || r.NewChild.ExpectedCareStartAt.IsZero() {
			return errors.New("new_child event requires typed payload with expected_care_start_at")
		}
	case LifeEventJobChange:
		if r.JobChange == nil || r.JobChange.BenefitsEnrollmentDeadlineAt.IsZero() {
			return errors.New("job_change event requires typed payload with benefits enrollment deadline")
		}
	case LifeEventHousingChange:
		if r.HousingChange == nil || r.HousingChange.EffectiveAt.IsZero() {
			return errors.New("housing_change event requires typed payload with effective_at")
		}
	}
	return nil
}

func (r LifeEventRecord) KindSummary() string {
	return string(r.Kind)
}

func (r LifeEventRecord) WindowStart() *time.Time {
	switch r.Kind {
	case LifeEventSalaryChange:
		if r.SalaryChange != nil {
			start := r.SalaryChange.EffectiveAt.UTC()
			return &start
		}
	case LifeEventNewChild:
		if r.NewChild != nil {
			start := r.NewChild.ExpectedCareStartAt.UTC()
			return &start
		}
	case LifeEventJobChange:
		if r.JobChange != nil {
			start := r.JobChange.BenefitsEnrollmentDeadlineAt.AddDate(0, 0, -14).UTC()
			return &start
		}
	case LifeEventHousingChange:
		if r.HousingChange != nil {
			start := r.HousingChange.EffectiveAt.UTC()
			return &start
		}
	}
	return nil
}

func (r LifeEventRecord) WindowEnd() *time.Time {
	start := r.WindowStart()
	if start == nil {
		return nil
	}
	end := start.AddDate(0, 3, 0)
	return &end
}

func (r CalendarDeadlineRecord) Validate() error {
	if strings.TrimSpace(r.ID) == "" {
		return errors.New("calendar deadline id is required")
	}
	if strings.TrimSpace(r.UserID) == "" {
		return errors.New("calendar deadline user_id is required")
	}
	if strings.TrimSpace(r.Kind) == "" {
		return errors.New("calendar deadline kind is required")
	}
	if strings.TrimSpace(r.Source) == "" {
		return errors.New("calendar deadline source is required")
	}
	if strings.TrimSpace(r.Provenance) == "" {
		return errors.New("calendar deadline provenance is required")
	}
	if r.ObservedAt.IsZero() || r.DeadlineAt.IsZero() {
		return errors.New("calendar deadline observed_at and deadline_at are required")
	}
	if strings.TrimSpace(r.Description) == "" {
		return errors.New("calendar deadline description is required")
	}
	if r.Confidence < 0 || r.Confidence > 1 {
		return errors.New("calendar deadline confidence must be within [0,1]")
	}
	if r.RelatedEventKind != "" && !validLifeEventKind(r.RelatedEventKind) {
		return fmt.Errorf("invalid related event kind %q", r.RelatedEventKind)
	}
	return nil
}

func validLifeEventKind(kind LifeEventKind) bool {
	switch kind {
	case LifeEventSalaryChange, LifeEventNewChild, LifeEventJobChange, LifeEventHousingChange:
		return true
	default:
		return false
	}
}

func countNonNilLifeEventPayloads(r LifeEventRecord) int {
	count := 0
	if r.SalaryChange != nil {
		count++
	}
	if r.NewChild != nil {
		count++
	}
	if r.JobChange != nil {
		count++
	}
	if r.HousingChange != nil {
		count++
	}
	return count
}
