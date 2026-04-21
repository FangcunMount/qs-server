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

func TestOptionsValidateBehaviorPendingReconcile(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Options)
		wantErr string
	}{
		{
			name: "disabled reconcile skips validation",
			mutate: func(opts *Options) {
				opts.BehaviorPendingReconcile.Enable = false
				opts.BehaviorPendingReconcile.Interval = 0
				opts.BehaviorPendingReconcile.BatchLimit = 0
				opts.BehaviorPendingReconcile.LockKey = ""
				opts.BehaviorPendingReconcile.LockTTL = 0
			},
		},
		{
			name: "enabled reconcile requires positive interval",
			mutate: func(opts *Options) {
				opts.BehaviorPendingReconcile.Interval = 0
			},
			wantErr: "behavior_pending_reconcile.interval must be greater than 0",
		},
		{
			name: "enabled reconcile requires positive batch limit",
			mutate: func(opts *Options) {
				opts.BehaviorPendingReconcile.BatchLimit = 0
			},
			wantErr: "behavior_pending_reconcile.batch_limit must be greater than 0",
		},
		{
			name: "enabled reconcile requires lock key",
			mutate: func(opts *Options) {
				opts.BehaviorPendingReconcile.LockKey = ""
			},
			wantErr: "behavior_pending_reconcile.lock_key cannot be empty when enabled",
		},
		{
			name: "enabled reconcile requires positive lock ttl",
			mutate: func(opts *Options) {
				opts.BehaviorPendingReconcile.LockTTL = 0
			},
			wantErr: "behavior_pending_reconcile.lock_ttl must be greater than 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := NewOptions()
			tt.mutate(opts)

			errs := opts.Validate()
			if tt.wantErr == "" {
				for _, err := range errs {
					if strings.Contains(err.Error(), "behavior_pending_reconcile.") {
						t.Fatalf("unexpected behavior pending reconcile validation error: %v", err)
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

func TestOptionsValidateStatisticsSync(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Options)
		wantErr string
	}{
		{
			name: "disabled statistics sync skips validation",
			mutate: func(opts *Options) {
				opts.StatisticsSync.Enable = false
				opts.StatisticsSync.OrgIDs = nil
				opts.StatisticsSync.RunAt = "bad"
				opts.StatisticsSync.RepairWindowDays = 0
				opts.StatisticsSync.LockKey = ""
				opts.StatisticsSync.LockTTL = 0
			},
		},
		{
			name: "enabled statistics sync requires org ids",
			mutate: func(opts *Options) {
				opts.StatisticsSync.OrgIDs = nil
			},
			wantErr: "statistics_sync.org_ids cannot be empty when enabled",
		},
		{
			name: "enabled statistics sync requires valid run_at",
			mutate: func(opts *Options) {
				opts.StatisticsSync.RunAt = "bad"
			},
			wantErr: "statistics_sync.run_at must be in HH:MM format",
		},
		{
			name: "enabled statistics sync requires positive repair window",
			mutate: func(opts *Options) {
				opts.StatisticsSync.RepairWindowDays = 0
			},
			wantErr: "statistics_sync.repair_window_days must be greater than 0",
		},
		{
			name: "enabled statistics sync requires lock key",
			mutate: func(opts *Options) {
				opts.StatisticsSync.LockKey = ""
			},
			wantErr: "statistics_sync.lock_key cannot be empty when enabled",
		},
		{
			name: "enabled statistics sync requires positive lock ttl",
			mutate: func(opts *Options) {
				opts.StatisticsSync.LockTTL = 0
			},
			wantErr: "statistics_sync.lock_ttl must be greater than 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := NewOptions()
			tt.mutate(opts)

			errs := opts.Validate()
			if tt.wantErr == "" {
				for _, err := range errs {
					if strings.Contains(err.Error(), "statistics_sync.") {
						t.Fatalf("unexpected statistics sync validation error: %v", err)
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

func TestOptionsValidateCacheRoutes(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Options)
		wantErr string
	}{
		{
			name: "rejects out-of-range jitter",
			mutate: func(opts *Options) {
				opts.Cache.TTLJitterRatio = 2
			},
			wantErr: "cache.ttl_jitter_ratio must be between 0 and 1",
		},
		{
			name: "rejects missing named profile when profiles declared",
			mutate: func(opts *Options) {
				opts.RedisProfiles["static_cache"] = opts.RedisOptions
				opts.RedisRuntime.Families["query_result"].AllowFallbackDefault = boolPtr(false)
				opts.RedisRuntime.Families["query_result"].RedisProfile = "query_cache"
			},
			wantErr: "redis_runtime.families.query_result.redis_profile references missing redis_profiles entry",
		},
		{
			name: "rejects invalid hotset size",
			mutate: func(opts *Options) {
				opts.Cache.Warmup.Hotset.TopN = 0
			},
			wantErr: "cache.warmup.hotset.top_n must be greater than 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := NewOptions()
			tt.mutate(opts)
			errs := opts.Validate()
			for _, err := range errs {
				if strings.Contains(err.Error(), tt.wantErr) {
					return
				}
			}
			t.Fatalf("expected validation error containing %q, got %v", tt.wantErr, errs)
		})
	}
}
