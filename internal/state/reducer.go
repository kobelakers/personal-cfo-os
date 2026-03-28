package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kobelakers/personal-cfo-os/internal/observation"
)

type DefaultStateReducer struct{}

func (DefaultStateReducer) ApplyEvidencePatch(current FinancialWorldState, patch EvidencePatch) (FinancialWorldState, StateDiff, error) {
	if err := patch.Validate(); err != nil {
		return FinancialWorldState{}, StateDiff{}, err
	}

	evidenceSet := make(map[string]struct{}, len(patch.Evidence))
	evidenceIDs := make([]string, 0, len(patch.Evidence))
	for _, item := range patch.Evidence {
		evidenceSet[string(item.ID)] = struct{}{}
		evidenceIDs = append(evidenceIDs, string(item.ID))
	}

	next := current
	changedFields := make([]string, 0, len(patch.Mutations))
	for _, mutation := range patch.Mutations {
		if _, exists := evidenceSet[string(mutation.EvidenceID)]; !exists {
			return FinancialWorldState{}, StateDiff{}, fmt.Errorf("mutation references unknown evidence id %q", mutation.EvidenceID)
		}
		if err := applyMutation(&next, mutation); err != nil {
			return FinancialWorldState{}, StateDiff{}, err
		}
		changedFields = append(changedFields, mutation.Path)
	}

	appliedAt := patch.AppliedAt
	if appliedAt.IsZero() {
		appliedAt = time.Now().UTC()
	}
	next.Version.Sequence = current.Version.Sequence + 1
	next.Version.SnapshotID = fmt.Sprintf("state-v%d", next.Version.Sequence)
	next.Version.UpdatedAt = appliedAt
	next.WorkflowState.LastUpdatedAt = appliedAt

	diff := StateDiff{
		FromVersion:   current.Version.Sequence,
		ToVersion:     next.Version.Sequence,
		ChangedFields: changedFields,
		EvidenceIDs:   toEvidenceIDs(evidenceIDs),
	}

	return next, diff, nil
}

func (s FinancialWorldState) Snapshot(reason string, capturedAt time.Time) StateSnapshot {
	if capturedAt.IsZero() {
		capturedAt = time.Now().UTC()
	}
	return StateSnapshot{
		State:      s,
		CapturedAt: capturedAt,
		Reason:     reason,
	}
}

func applyMutation(state *FinancialWorldState, mutation StateMutation) error {
	switch mutation.Path {
	case "cashflow.monthly_net_income_cents":
		var value int64
		if err := json.Unmarshal([]byte(mutation.ValueJSON), &value); err != nil {
			return err
		}
		state.CashflowState.MonthlyNetIncomeCents = value
	case "liability.total_debt_cents":
		var value int64
		if err := json.Unmarshal([]byte(mutation.ValueJSON), &value); err != nil {
			return err
		}
		state.LiabilityState.TotalDebtCents = value
	case "workflow.phase":
		var value string
		if err := json.Unmarshal([]byte(mutation.ValueJSON), &value); err != nil {
			return err
		}
		state.WorkflowState.Phase = value
	case "risk.compliance_flags":
		var value []string
		if err := json.Unmarshal([]byte(mutation.ValueJSON), &value); err != nil {
			return err
		}
		state.RiskState.ComplianceFlags = value
	default:
		return fmt.Errorf("unsupported state mutation path %q", mutation.Path)
	}
	return nil
}

func toEvidenceIDs(ids []string) []observation.EvidenceID {
	result := make([]observation.EvidenceID, 0, len(ids))
	for _, id := range ids {
		result = append(result, observation.EvidenceID(id))
	}
	return result
}

func (p EvidencePatch) Validate() error {
	var errs []error
	if len(p.Evidence) == 0 {
		errs = append(errs, errors.New("evidence patch requires evidence"))
	}
	if len(p.Mutations) == 0 {
		errs = append(errs, errors.New("evidence patch requires mutations"))
	}
	for _, evidence := range p.Evidence {
		if err := evidence.Validate(); err != nil {
			errs = append(errs, err)
		}
	}
	for i, mutation := range p.Mutations {
		if mutation.Path == "" {
			errs = append(errs, fmt.Errorf("mutation %d path is required", i))
		}
		if mutation.Operation == "" {
			errs = append(errs, fmt.Errorf("mutation %d operation is required", i))
		}
		if mutation.ValueJSON == "" {
			errs = append(errs, fmt.Errorf("mutation %d value_json is required", i))
		}
		if mutation.EvidenceID == "" {
			errs = append(errs, fmt.Errorf("mutation %d evidence_id is required", i))
		}
	}
	return errors.Join(errs...)
}
