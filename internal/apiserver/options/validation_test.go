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
		{
			name: "enabled scheduler keeps fixed opening window",
			mutate: func(opts *Options) {
				opts.PlanScheduler.Enable = true
				opts.PlanScheduler.PendingLookback = 6 * time.Hour
			},
			wantErr: "plan_scheduler.pending_lookback must remain 24h",
		},
		{
			name: "enabled scheduler requires positive batch size",
			mutate: func(opts *Options) {
				opts.PlanScheduler.Enable = true
				opts.PlanScheduler.BatchSize = 0
			},
			wantErr: "plan_scheduler.batch_size must be greater than 0",
		},
		{
			name: "enabled scheduler caps a phase above one page",
			mutate: func(opts *Options) {
				opts.PlanScheduler.Enable = true
				opts.PlanScheduler.BatchSize = 200
				opts.PlanScheduler.MaxTasksPerTick = 100
			},
			wantErr: "plan_scheduler.batch_size must be less than or equal to plan_scheduler.max_tasks_per_tick",
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

func TestOptionsValidateEvaluationConsistencyAudit(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Options)
		wantErr string
	}{
		{
			name: "disabled audit skips validation",
			mutate: func(opts *Options) {
				opts.EvaluationConsistencyAudit.Enable = false
				opts.EvaluationConsistencyAudit.CycleInterval = 0
				opts.EvaluationConsistencyAudit.BatchSize = 0
			},
		},
		{
			name: "audit cycle is bounded to governance window",
			mutate: func(opts *Options) {
				opts.EvaluationConsistencyAudit.CycleInterval = time.Hour
			},
			wantErr: "evaluation_consistency_audit.cycle_interval must be between 6h and 24h",
		},
		{
			name: "audit requires positive batch size",
			mutate: func(opts *Options) {
				opts.EvaluationConsistencyAudit.BatchSize = 0
			},
			wantErr: "evaluation_consistency_audit.batch_size must be greater than 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := NewOptions()
			tt.mutate(opts)

			errs := opts.Validate()
			if tt.wantErr == "" {
				for _, err := range errs {
					if strings.Contains(err.Error(), "evaluation_consistency_audit.") {
						t.Fatalf("unexpected evaluation consistency audit validation error: %v", err)
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

func TestOptionsValidateLeaseRecoveryCadence(t *testing.T) {
	for _, name := range []string{"evaluation_lease_recovery", "interpretation_lease_recovery"} {
		t.Run(name, func(t *testing.T) {
			opts := NewOptions()
			var target *LeaseRecoveryOptions
			if name == "evaluation_lease_recovery" {
				target = opts.EvaluationLeaseRecovery
			} else {
				target = opts.InterpretationLeaseRecovery
			}
			target.Interval = 31 * time.Second
			errs := opts.Validate()
			for _, err := range errs {
				if strings.Contains(err.Error(), name+".interval must be between 10s and 30s") {
					return
				}
			}
			t.Fatalf("expected %s interval validation error, got %v", name, errs)
		})
	}
}

func TestOptionsValidateEvaluationMaintenanceLockIsolation(t *testing.T) {
	opts := NewOptions()
	opts.InterpretationLeaseRecovery.LockKey = opts.EvaluationLeaseRecovery.LockKey

	errs := opts.Validate()
	for _, err := range errs {
		if strings.Contains(err.Error(), "interpretation_lease_recovery.lock_key must be independent from evaluation_lease_recovery.lock_key") {
			return
		}
	}
	t.Fatalf("expected independent maintenance lock validation error, got %v", errs)
}

func TestOptionsValidateOutboxRelay(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Options)
		wantErr string
	}{
		{
			name: "nil outbox relay skips validation",
			mutate: func(opts *Options) {
				opts.OutboxRelay = nil
			},
		},
		{
			name: "mongo relay requires positive interval",
			mutate: func(opts *Options) {
				opts.OutboxRelay.Mongo.Interval = 0
			},
			wantErr: "outbox_relay.mongo.interval must be greater than 0",
		},
		{
			name: "mongo relay requires positive batch size",
			mutate: func(opts *Options) {
				opts.OutboxRelay.Mongo.BatchSize = 0
			},
			wantErr: "outbox_relay.mongo.batch_size must be greater than 0",
		},
		{
			name: "mongo relay requires positive publish workers",
			mutate: func(opts *Options) {
				opts.OutboxRelay.Mongo.PublishWorkers = 0
			},
			wantErr: "outbox_relay.mongo.publish_workers must be greater than 0",
		},
		{
			name: "mongo relay workers are capped by mongo backpressure",
			mutate: func(opts *Options) {
				opts.Backpressure.Mongo.MaxInflight = 10
				opts.OutboxRelay.Mongo.PublishWorkers = 9
			},
			wantErr: "outbox_relay.mongo.publish_workers (9) must be <= backpressure.mongo.max_inflight * 0.8 (8)",
		},
		{
			name: "mysql pool does not cap mongo relay workers",
			mutate: func(opts *Options) {
				opts.MySQLOptions.MaxOpenConnections = 10
				opts.OutboxRelay.Assessment.PublishWorkers = 8
				opts.Backpressure.Mongo.MaxInflight = 100
				opts.OutboxRelay.Mongo.PublishWorkers = 20
			},
		},
		{
			name: "disabled mongo backpressure does not impose a worker cap",
			mutate: func(opts *Options) {
				opts.Backpressure.Mongo.Enabled = false
				opts.Backpressure.Mongo.MaxInflight = 1
				opts.OutboxRelay.Mongo.PublishWorkers = 64
			},
		},
		{
			name: "nil backpressure does not impose a mongo worker cap",
			mutate: func(opts *Options) {
				opts.Backpressure = nil
				opts.OutboxRelay.Mongo.PublishWorkers = 64
			},
		},
		{
			name: "assessment relay requires positive interval",
			mutate: func(opts *Options) {
				opts.OutboxRelay.Assessment.Interval = 0
			},
			wantErr: "outbox_relay.assessment.interval must be greater than 0",
		},
		{
			name: "assessment relay requires positive batch size",
			mutate: func(opts *Options) {
				opts.OutboxRelay.Assessment.BatchSize = 0
			},
			wantErr: "outbox_relay.assessment.batch_size must be greater than 0",
		},
		{
			name: "assessment relay requires positive publish workers",
			mutate: func(opts *Options) {
				opts.OutboxRelay.Assessment.PublishWorkers = 0
			},
			wantErr: "outbox_relay.assessment.publish_workers must be greater than 0",
		},
		{
			name: "assessment relay workers are capped by mysql pool",
			mutate: func(opts *Options) {
				opts.MySQLOptions.MaxOpenConnections = 10
				opts.OutboxRelay.Assessment.PublishWorkers = 9
			},
			wantErr: "outbox_relay.assessment.publish_workers (9) must be <= mysql max_open * 0.8 (8)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := NewOptions()
			tt.mutate(opts)

			errs := opts.Validate()
			if tt.wantErr == "" {
				for _, err := range errs {
					if strings.Contains(err.Error(), "outbox_relay.") {
						t.Fatalf("unexpected outbox relay validation error: %v", err)
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

func TestOptionsValidateCacheConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Options)
		wantErr string
	}{
		{
			name: "rejects out-of-range jitter",
			mutate: func(opts *Options) {
				opts.Cache.Defaults.TTLJitterRatio = 2
			},
			wantErr: "cache.defaults.ttl_jitter_ratio must be between 0 and 1",
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
				opts.Cache.Governance.Warmup.Hotset.TopN = 0
			},
			wantErr: "cache.governance.warmup.hotset.top_n must be greater than 0",
		},
		{
			name: "rejects invalid hotset retention cap",
			mutate: func(opts *Options) {
				opts.Cache.Governance.Warmup.Hotset.MaxItemsPerKind = 0
			},
			wantErr: "cache.governance.warmup.hotset.max_items_per_kind must be greater than 0",
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

func TestOptionsValidateRetryHardCaps(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Options)
		want   string
	}{
		{name: "business", mutate: func(opts *Options) { opts.SystemGovernance.Retry.Business.MaxAutomaticAttempts = 4 }, want: "business.max_automatic_attempts cannot exceed 3"},
		{name: "outbox", mutate: func(opts *Options) { opts.SystemGovernance.Retry.Outbox.MaxAutomaticAttempts = 31 }, want: "outbox.max_automatic_attempts cannot exceed 30"},
		{name: "transport disabled", mutate: func(opts *Options) {
			opts.MessagingOptions.Delivery.Enable = false
			opts.MessagingOptions.Delivery.MaxAttempts = 9
		}, want: "messaging.delivery.max_attempts must be between 1 and 8"},
		{name: "iam transport disabled", mutate: func(opts *Options) {
			opts.IAMOptions.AuthzSync.Delivery.Enable = false
			opts.IAMOptions.AuthzSync.Delivery.MaxAttempts = 9
		}, want: "iam.authz-sync.delivery.max_attempts must be between 1 and 8"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := NewOptions()
			tt.mutate(opts)
			for _, err := range opts.Validate() {
				if strings.Contains(err.Error(), tt.want) {
					return
				}
			}
			t.Fatalf("expected validation error containing %q", tt.want)
		})
	}
}

func TestOptionsValidateSystemGovernanceComponentDiscovery(t *testing.T) {
	tests := []struct {
		name   string
		config *GovernanceComponentOptions
		want   string
	}{
		{
			name: "rejects unknown discovery",
			config: &GovernanceComponentOptions{
				Discovery: "round_robin", Timeout: time.Second,
			},
			want: "discovery must be single or dns",
		},
		{
			name: "rejects zero dns minimum",
			config: &GovernanceComponentOptions{
				Discovery: "dns", MinimumInstances: 0, Timeout: time.Second,
			},
			want: "minimum_instances must be between 1 and 16",
		},
		{
			name: "rejects https dns endpoint",
			config: &GovernanceComponentOptions{
				Discovery: "dns", MinimumInstances: 2, Timeout: time.Second,
				ResilienceURL: "https://collection/governance/resilience",
			},
			want: "must be an absolute http URL for dns discovery",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := NewOptions()
			opts.SystemGovernance.Components["collection-server"] = tt.config
			for _, err := range opts.Validate() {
				if strings.Contains(err.Error(), tt.want) {
					return
				}
			}
			t.Fatalf("expected validation error containing %q, got %v", tt.want, opts.Validate())
		})
	}
}

func TestOptionsValidateSystemGovernanceSingleAndDNSDiscovery(t *testing.T) {
	for _, config := range []*GovernanceComponentOptions{
		{
			ResilienceURL:      "http://127.0.0.1:18083/governance/resilience",
			CacheURL:           "http://127.0.0.1:18083/governance/redis",
			CacheGovernanceURL: "http://127.0.0.1:18083/governance/cache",
			Timeout:            time.Second,
		},
		{
			Discovery: "dns", MinimumInstances: 2,
			ResilienceURL:      "http://qs-collection-server:8080/governance/resilience",
			CacheURL:           "http://qs-collection-server:8080/governance/redis",
			CacheGovernanceURL: "http://qs-collection-server:8080/governance/cache",
			Timeout:            time.Second,
		},
	} {
		opts := NewOptions()
		opts.SystemGovernance.Components["collection-server"] = config
		for _, err := range opts.Validate() {
			if strings.Contains(err.Error(), "system_governance.components.collection-server") {
				t.Fatalf("unexpected component validation error: %v", err)
			}
		}
	}
}
