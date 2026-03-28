package context

import (
	"errors"
	"fmt"
)

func (s ContextSlice) Validate() error {
	if s.TaskID == "" {
		return errors.New("context slice task id is required")
	}
	if !validContextView(s.View) {
		return fmt.Errorf("invalid context view %q", s.View)
	}
	return nil
}

func validContextView(view ContextView) bool {
	switch view {
	case ContextViewPlanning, ContextViewExecution, ContextViewVerification:
		return true
	default:
		return false
	}
}
