package options

import (
	"fmt"
	"time"
)

// RetryDispatchOptions bounds delivery of already-authorized future plans.
// This is neither a business retry policy nor the SDK evidence maintenance loop.
type RetryDispatchOptions struct {
	Interval time.Duration `json:"interval" mapstructure:"interval"`
	Budget   time.Duration `json:"budget" mapstructure:"budget"`
	Batch    int           `json:"batch" mapstructure:"batch"`
}

func (o *RetryGovernanceOptions) DispatchLimits() (RetryDispatchOptions, bool, error) {
	limits := RetryDispatchOptions{Interval: 5 * time.Second, Budget: 5 * time.Second, Batch: 50}
	automatic := true
	if o != nil {
		if o.AutomaticRetryEnabled != nil {
			automatic = *o.AutomaticRetryEnabled
		}
		if o.Dispatch != nil {
			if o.Dispatch.Interval != 0 {
				limits.Interval = o.Dispatch.Interval
			}
			if o.Dispatch.Budget != 0 {
				limits.Budget = o.Dispatch.Budget
			}
			if o.Dispatch.Batch != 0 {
				limits.Batch = o.Dispatch.Batch
			}
		}
	}
	if limits.Interval <= 0 || limits.Budget <= 0 || limits.Batch < 1 || limits.Batch > 50 {
		return RetryDispatchOptions{}, false, fmt.Errorf("retry dispatch requires positive interval/budget and batch1..50 per owner")
	}
	return limits, automatic, nil
}
