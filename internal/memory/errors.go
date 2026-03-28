package memory

import "errors"

type PolicyDeniedError struct {
	Reason string
}

func (e *PolicyDeniedError) Error() string {
	if e == nil {
		return ""
	}
	if e.Reason == "" {
		return "memory write denied by policy"
	}
	return "memory write denied by policy: " + e.Reason
}

func IsPolicyDenied(err error) bool {
	var target *PolicyDeniedError
	return errors.As(err, &target)
}
