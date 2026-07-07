package policy

import "time"

// TimeoutPolicy bounds evaluation execution duration.
type TimeoutPolicy struct {
	Limit time.Duration
}

// DefaultTimeoutPolicy returns the default evaluation execution timeout.
func DefaultTimeoutPolicy() TimeoutPolicy {
	return TimeoutPolicy{Limit: 30 * time.Second}
}
