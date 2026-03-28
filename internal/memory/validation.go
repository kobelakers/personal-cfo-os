package memory

import (
	"errors"
	"fmt"
	"strings"
)

func (r MemoryRecord) Validate() error {
	var errs []error
	if strings.TrimSpace(r.ID) == "" {
		errs = append(errs, errors.New("memory record id is required"))
	}
	if !validMemoryKind(r.Kind) {
		errs = append(errs, fmt.Errorf("invalid memory kind %q", r.Kind))
	}
	if strings.TrimSpace(r.Summary) == "" {
		errs = append(errs, errors.New("memory summary is required"))
	}
	if len(r.Facts) == 0 {
		errs = append(errs, errors.New("memory must contain at least one fact"))
	}
	for i, fact := range r.Facts {
		if strings.TrimSpace(fact.Key) == "" {
			errs = append(errs, fmt.Errorf("memory fact %d key is required", i))
		}
		if strings.TrimSpace(fact.Value) == "" {
			errs = append(errs, fmt.Errorf("memory fact %d value is required", i))
		}
	}
	if err := r.Source.Validate(); err != nil {
		errs = append(errs, err)
	}
	if err := r.Confidence.Validate(); err != nil {
		errs = append(errs, err)
	}
	if !r.UpdatedAt.IsZero() && !r.CreatedAt.IsZero() && r.UpdatedAt.Before(r.CreatedAt) {
		errs = append(errs, errors.New("memory updated_at cannot be before created_at"))
	}
	for _, supersedes := range r.Supersedes {
		if supersedes.MemoryID == r.ID {
			errs = append(errs, errors.New("memory record cannot supersede itself"))
		}
	}
	for _, conflict := range r.Conflicts {
		if conflict.MemoryID == r.ID {
			errs = append(errs, errors.New("memory record cannot conflict with itself"))
		}
	}
	return errors.Join(errs...)
}

func (s MemorySource) Validate() error {
	if len(s.EvidenceIDs) == 0 && s.TaskID == "" && s.WorkflowID == "" {
		return errors.New("memory source requires at least one provenance reference")
	}
	return nil
}

func (c MemoryConfidence) Validate() error {
	if c.Score < 0 || c.Score > 1 {
		return errors.New("memory confidence must be within [0,1]")
	}
	if strings.TrimSpace(c.Rationale) == "" {
		return errors.New("memory confidence rationale is required")
	}
	return nil
}

func validMemoryKind(kind MemoryKind) bool {
	switch kind {
	case MemoryKindEpisodic, MemoryKindSemantic, MemoryKindProcedural, MemoryKindPolicy:
		return true
	default:
		return false
	}
}
