package options

import (
	"strings"
	"testing"
	"time"
)

func TestOptionsValidatePlanScheduler(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Options)
		wantErr string
	}{
		{
			name: "disabled scheduler skips validation",
			mutate: func(opts *Options) {
				opts.PlanScheduler.Enable = false
				opts.PlanScheduler.LockTTL = 2 * time.Minute
				opts.PlanScheduler.Interval = time.Minute
				opts.PlanScheduler.OrgIDs = nil
			},
		},
		{
			name: "enabled scheduler rejects lock ttl longer than interval",
			mutate: func(opts *Options) {
				opts.PlanScheduler.Enable = true
				opts.PlanScheduler.Interval = time.Minute
				opts.PlanScheduler.LockTTL = 2 * time.Minute
			},
			wantErr: "plan_scheduler.lock_ttl must be less than or equal to plan_scheduler.interval",
		},
		{
			name: "enabled scheduler requires org ids",
			mutate: func(opts *Options) {
				opts.PlanScheduler.Enable = true
				opts.PlanScheduler.OrgIDs = nil
			},
			wantErr: "plan_scheduler.org_ids cannot be empty when enabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := NewOptions()
			tt.mutate(opts)

			errs := opts.Validate()
			if tt.wantErr == "" {
				for _, err := range errs {
					if strings.Contains(err.Error(), "plan_scheduler.") {
						t.Fatalf("unexpected plan scheduler validation error: %v", err)
					}
				}
				return
			}

			for _, err := range errs {
				if strings.Contains(err.Error(), tt.wantErr) {
					return
				}
			}
			t.Fatalf("expected validation error containing %q, got %v", tt.wantErr, errs)
		})
	}
}
