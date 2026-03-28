package observation

import (
	"errors"
	"fmt"
	"strings"
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
