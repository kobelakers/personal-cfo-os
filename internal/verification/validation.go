package verification

import (
	"errors"
	"fmt"
)

func (r VerificationResult) Validate() error {
	if !validVerificationStatus(r.Status) {
		return fmt.Errorf("invalid verification status %q", r.Status)
	}
	if r.Validator == "" {
		return errors.New("verification validator is required")
	}
	if err := r.EvidenceCoverage.Validate(); err != nil {
		return err
	}
	if r.CheckedAt.IsZero() {
		return errors.New("verification checked_at is required")
	}
	return nil
}

func (o OracleVerdict) Validate() error {
	if o.Scenario == "" {
		return errors.New("oracle scenario is required")
	}
	if o.Score < 0 || o.Score > 1 {
		return errors.New("oracle score must be within [0,1]")
	}
	if o.CheckedAt.IsZero() {
		return errors.New("oracle checked_at is required")
	}
	return nil
}

func (r EvidenceCoverageReport) Validate() error {
	if r.TaskID == "" {
		return errors.New("evidence coverage task id is required")
	}
	if r.CoverageRatio < 0 || r.CoverageRatio > 1 {
		return errors.New("coverage ratio must be within [0,1]")
	}
	return nil
}

func validVerificationStatus(status VerificationStatus) bool {
	switch status {
	case VerificationStatusPass, VerificationStatusFail, VerificationStatusNeedsReplan, VerificationStatusBlocked:
		return true
	default:
		return false
	}
}
