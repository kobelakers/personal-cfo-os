package model

import "time"

func DefaultTimeoutPolicy() TimeoutPolicy {
	return TimeoutPolicy{RequestTimeout: 20 * time.Second}
}
