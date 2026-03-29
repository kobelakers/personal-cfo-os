package model

import "time"

func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts:    2,
		InitialBackoff: 200 * time.Millisecond,
		MaxBackoff:     2 * time.Second,
	}
}

func shouldRetryProviderError(err error) bool {
	providerErr, ok := err.(*ProviderError)
	if !ok {
		return false
	}
	return providerErr.Retryable
}

func backoffForAttempt(policy RetryPolicy, attempt int) time.Duration {
	backoff := policy.InitialBackoff
	if backoff <= 0 {
		backoff = 200 * time.Millisecond
	}
	for i := 1; i < attempt; i++ {
		backoff *= 2
		if policy.MaxBackoff > 0 && backoff > policy.MaxBackoff {
			return policy.MaxBackoff
		}
	}
	if policy.MaxBackoff > 0 && backoff > policy.MaxBackoff {
		return policy.MaxBackoff
	}
	return backoff
}
