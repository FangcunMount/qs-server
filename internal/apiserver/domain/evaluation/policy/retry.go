package policy

import "time"

// RetryPolicy configures bounded retries for transient evaluation failures.
type RetryPolicy struct {
	MaxAttempts int
	Backoff     time.Duration
}

// DefaultRetryPolicy returns a conservative retry policy for evaluation execution.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{MaxAttempts: 3, Backoff: 200 * time.Millisecond}
}
