package options

import (
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

// Validate 验证命令行参数
func (o *Options) Validate() []error {
	var errs []error

	errs = append(errs, o.GenericServerRunOptions.Validate()...)
	errs = append(errs, o.MySQLOptions.Validate()...)
	errs = append(errs, o.Log.Validate()...)
	errs = append(errs, o.OSSOptions.Validate()...)
	errs = append(errs, o.AssessmentAssets.Validate()...)
	if o.MessagingOptions == nil {
		errs = append(errs, fmt.Errorf("messaging is required"))
	} else {
		errs = append(errs, o.MessagingOptions.Validate()...)
	}
	if o.IAMOptions != nil && o.IAMOptions.AuthzSync != nil {
		errs = append(errs, o.IAMOptions.AuthzSync.Delivery.Validate("iam.authz-sync.delivery")...)
	}
	if o.AssessmentAssets != nil && o.AssessmentAssets.Enabled && (o.OSSOptions == nil || !o.OSSOptions.Enabled) {
		errs = append(errs, fmt.Errorf("oss.enabled must be true when assessment_assets.enabled is true"))
	}
	errs = append(errs, validateRateLimit(o.RateLimit)...)
	errs = append(errs, validateBackpressureOptions(o.Backpressure)...)
	errs = append(errs, validatePlanScheduler(o.PlanScheduler)...)
	errs = append(errs, validateBehaviorPendingReconcile(o.BehaviorPendingReconcile)...)
	errs = append(errs, validateEvaluationConsistencyReconcile(o.EvaluationConsistencyReconcile)...)
	errs = append(errs, validateBehaviorJourneyScan(o.BehaviorJourneyScan)...)
	errs = append(errs, validateOutboxRelay(o.OutboxRelay, o.MySQLOptions.MaxOpenConnections)...)
	errs = append(errs, validateStatisticsSync(o.StatisticsSync)...)
	errs = append(errs, validateCacheOptions(o.Cache)...)
	errs = append(errs, validateRetryGovernance(o.SystemGovernance)...)

	errs = append(errs, redisruntime.ValidateRuntimeOptions(
		o.RedisRuntime,
		[]redisruntime.Family{
			redisruntime.FamilyStatic,
			redisruntime.FamilyObject,
			redisruntime.FamilyQuery,
			redisruntime.FamilyMeta,
			redisruntime.FamilyRank,
			redisruntime.FamilySDK,
			redisruntime.FamilyOps,
			redisruntime.FamilyLock,
		},
		o.RedisProfiles,
		"redis_runtime",
	)...)

	return errs
}

func validateRetryGovernance(opts *SystemGovernanceOptions) []error {
	if opts == nil || opts.Retry == nil {
		return nil
	}
	var errs []error
	for name, policy := range map[string]*RetryPolicyOptions{"business": opts.Retry.Business, "outbox": opts.Retry.Outbox} {
		if policy == nil {
			errs = append(errs, fmt.Errorf("system_governance.retry.%s is required", name))
			continue
		}
		if policy.MaxAutomaticAttempts < 1 {
			errs = append(errs, fmt.Errorf("system_governance.retry.%s.max_automatic_attempts must be greater than 0", name))
		}
		hardMax := retrygovernance.HardMaxBusinessAttempts
		if name == "outbox" {
			hardMax = retrygovernance.HardMaxOutboxAttempts
		}
		if policy.MaxAutomaticAttempts > hardMax {
			errs = append(errs, fmt.Errorf("system_governance.retry.%s.max_automatic_attempts cannot exceed %d", name, hardMax))
		}
		if policy.BaseDelay <= 0 || policy.MaxDelay < policy.BaseDelay {
			errs = append(errs, fmt.Errorf("system_governance.retry.%s delays are invalid", name))
		}
		if policy.JitterFraction < 0 || policy.JitterFraction > 1 {
			errs = append(errs, fmt.Errorf("system_governance.retry.%s.jitter_fraction must be between 0 and 1", name))
		}
	}
	return errs
}

// ValidateCacheOptions validates only the cache section for policy reload.
// Unrelated process settings are deliberately outside the reload transaction.
func ValidateCacheOptions(options *CacheOptions) []error {
	return validateCacheOptions(options)
}

func validateRateLimit(opts *RateLimitOptions) []error {
	if opts == nil || !opts.Enabled {
		return nil
	}

	var errs []error
	checks := []struct {
		valid bool
		msg   string
	}{
		{opts.SubmitGlobalQPS > 0 && opts.SubmitGlobalBurst > 0, "rate_limit.submit_* must be greater than 0"},
		{opts.SubmitUserQPS > 0 && opts.SubmitUserBurst > 0, "rate_limit.submit_user_* must be greater than 0"},
		{opts.AdminSubmitGlobalQPS > 0 && opts.AdminSubmitGlobalBurst > 0, "rate_limit.admin_submit_* must be greater than 0"},
		{opts.AdminSubmitUserQPS > 0 && opts.AdminSubmitUserBurst > 0, "rate_limit.admin_submit_user_* must be greater than 0"},
		{opts.QueryGlobalQPS > 0 && opts.QueryGlobalBurst > 0, "rate_limit.query_* must be greater than 0"},
		{opts.QueryUserQPS > 0 && opts.QueryUserBurst > 0, "rate_limit.query_user_* must be greater than 0"},
		{opts.WaitReportGlobalQPS > 0 && opts.WaitReportGlobalBurst > 0, "rate_limit.wait_report_* must be greater than 0"},
		{opts.WaitReportUserQPS > 0 && opts.WaitReportUserBurst > 0, "rate_limit.wait_report_user_* must be greater than 0"},
	}
	for _, check := range checks {
		if !check.valid {
			errs = append(errs, fmt.Errorf("%s", check.msg))
		}
	}
	return errs
}

func validateBackpressureOptions(opts *BackpressureOptions) []error {
	if opts == nil {
		return nil
	}

	var errs []error
	for _, dep := range []struct {
		name string
		opt  *DependencyBackpressure
	}{
		{name: "mysql", opt: opts.MySQL},
		{name: "mongo", opt: opts.Mongo},
		{name: "iam", opt: opts.IAM},
	} {
		if dep.opt == nil || !dep.opt.Enabled {
			continue
		}
		if dep.opt.MaxInflight <= 0 {
			errs = append(errs, fmt.Errorf("backpressure.%s.max_inflight must be greater than 0", dep.name))
		}
		if dep.opt.TimeoutMs < 0 {
			errs = append(errs, fmt.Errorf("backpressure.%s.timeout_ms cannot be negative", dep.name))
		}
	}
	return errs
}

func validatePlanScheduler(opts *PlanSchedulerOptions) []error {
	if opts == nil || !opts.Enable {
		return nil
	}

	var errs []error
	if len(opts.OrgIDs) == 0 {
		errs = append(errs, fmt.Errorf("plan_scheduler.org_ids cannot be empty when enabled"))
	}
	if opts.InitialDelay < 0 {
		errs = append(errs, fmt.Errorf("plan_scheduler.initial_delay cannot be negative"))
	}
	if opts.Interval <= 0 {
		errs = append(errs, fmt.Errorf("plan_scheduler.interval must be greater than 0"))
	}
	if opts.PendingLookback <= 0 {
		errs = append(errs, fmt.Errorf("plan_scheduler.pending_lookback must be greater than 0"))
	}
	if opts.LockKey == "" {
		errs = append(errs, fmt.Errorf("plan_scheduler.lock_key cannot be empty when enabled"))
	}
	if opts.LockTTL <= 0 {
		errs = append(errs, fmt.Errorf("plan_scheduler.lock_ttl must be greater than 0"))
	}
	if opts.Interval > 0 && opts.LockTTL > opts.Interval {
		errs = append(errs, fmt.Errorf("plan_scheduler.lock_ttl must be less than or equal to plan_scheduler.interval"))
	}
	return errs
}

func validateBehaviorPendingReconcile(opts *BehaviorPendingReconcileOptions) []error {
	if opts == nil || !opts.Enable {
		return nil
	}

	var errs []error
	if opts.Interval <= 0 {
		errs = append(errs, fmt.Errorf("behavior_pending_reconcile.interval must be greater than 0"))
	}
	if opts.BatchLimit <= 0 {
		errs = append(errs, fmt.Errorf("behavior_pending_reconcile.batch_limit must be greater than 0"))
	}
	if opts.LockKey == "" {
		errs = append(errs, fmt.Errorf("behavior_pending_reconcile.lock_key cannot be empty when enabled"))
	}
	if opts.LockTTL <= 0 {
		errs = append(errs, fmt.Errorf("behavior_pending_reconcile.lock_ttl must be greater than 0"))
	}
	return errs
}

func validateEvaluationConsistencyReconcile(opts *EvaluationConsistencyReconcileOptions) []error {
	if opts == nil || !opts.Enable {
		return nil
	}

	var errs []error
	if opts.Interval <= 0 {
		errs = append(errs, fmt.Errorf("evaluation_consistency_reconcile.interval must be greater than 0"))
	}
	if opts.BatchLimit <= 0 {
		errs = append(errs, fmt.Errorf("evaluation_consistency_reconcile.batch_limit must be greater than 0"))
	}
	if opts.LockKey == "" {
		errs = append(errs, fmt.Errorf("evaluation_consistency_reconcile.lock_key cannot be empty when enabled"))
	}
	if opts.LockTTL <= 0 {
		errs = append(errs, fmt.Errorf("evaluation_consistency_reconcile.lock_ttl must be greater than 0"))
	}
	return errs
}

func validateBehaviorJourneyScan(opts *BehaviorJourneyScanOptions) []error {
	if opts == nil || !opts.Enable {
		return nil
	}
	var errs []error
	if opts.Interval <= 0 {
		errs = append(errs, fmt.Errorf("behavior_journey_scan.interval must be greater than 0"))
	}
	if opts.BatchSize <= 0 {
		errs = append(errs, fmt.Errorf("behavior_journey_scan.batch_size must be greater than 0"))
	}
	if len(opts.OrgIDs) == 0 {
		errs = append(errs, fmt.Errorf("behavior_journey_scan.org_ids cannot be empty when enabled"))
	}
	if opts.LockKey == "" {
		errs = append(errs, fmt.Errorf("behavior_journey_scan.lock_key cannot be empty when enabled"))
	}
	if opts.LockTTL <= 0 {
		errs = append(errs, fmt.Errorf("behavior_journey_scan.lock_ttl must be greater than 0"))
	}
	return errs
}

func validateOutboxRelay(opts *OutboxRelayOptions, mysqlMaxOpen int) []error {
	if opts == nil {
		return nil
	}

	var errs []error
	maxWorkers := maxOutboxPublishWorkers(mysqlMaxOpen, 0.8)
	for _, relay := range []struct {
		name string
		opt  *OutboxRelayStoreOptions
	}{
		{name: "mongo", opt: opts.Mongo},
		{name: "assessment", opt: opts.Assessment},
	} {
		if relay.opt == nil {
			continue
		}
		if relay.opt.Interval <= 0 {
			errs = append(errs, fmt.Errorf("outbox_relay.%s.interval must be greater than 0", relay.name))
		}
		if relay.opt.BatchSize <= 0 {
			errs = append(errs, fmt.Errorf("outbox_relay.%s.batch_size must be greater than 0", relay.name))
		}
		if relay.opt.PublishWorkers <= 0 {
			errs = append(errs, fmt.Errorf("outbox_relay.%s.publish_workers must be greater than 0", relay.name))
		}
		if maxWorkers > 0 && relay.opt.PublishWorkers > maxWorkers {
			errs = append(errs, fmt.Errorf("outbox_relay.%s.publish_workers (%d) must be <= mysql max_open * 0.8 (%d)", relay.name, relay.opt.PublishWorkers, maxWorkers))
		}
	}
	return errs
}

func maxOutboxPublishWorkers(mysqlMaxOpen int, ratio float64) int {
	if mysqlMaxOpen <= 0 {
		return 0
	}
	if ratio <= 0 {
		ratio = 0.8
	}
	return int(float64(mysqlMaxOpen) * ratio)
}

func validateStatisticsSync(opts *StatisticsSyncOptions) []error {
	if opts == nil || !opts.Enable {
		return nil
	}

	var errs []error
	if len(opts.OrgIDs) == 0 {
		errs = append(errs, fmt.Errorf("statistics_sync.org_ids cannot be empty when enabled"))
	}
	if _, err := time.ParseInLocation("15:04", opts.RunAt, time.Local); err != nil {
		errs = append(errs, fmt.Errorf("statistics_sync.run_at must be in HH:MM format"))
	}
	if opts.RepairWindowDays <= 0 {
		errs = append(errs, fmt.Errorf("statistics_sync.repair_window_days must be greater than 0"))
	}
	if opts.LockKey == "" {
		errs = append(errs, fmt.Errorf("statistics_sync.lock_key cannot be empty when enabled"))
	}
	if opts.LockTTL <= 0 {
		errs = append(errs, fmt.Errorf("statistics_sync.lock_ttl must be greater than 0"))
	}
	return errs
}

func validateCacheOptions(opts *CacheOptions) []error {
	if opts == nil {
		return nil
	}
	var errs []error
	if opts.Defaults == nil {
		return nil
	}
	if opts.Defaults.TTLJitterRatio < 0 || opts.Defaults.TTLJitterRatio > 1 {
		errs = append(errs, fmt.Errorf("cache.ttl_jitter_ratio must be between 0 and 1"))
	}
	for _, family := range []struct {
		name string
		opt  *CacheFamilyOptions
	}{
		{name: "static", opt: opts.Defaults.Static},
		{name: "object", opt: opts.Defaults.Object},
		{name: "query", opt: opts.Defaults.Query},
	} {
		errs = append(errs, validateCacheFamilyPolicy(family.name, family.opt)...)
	}
	if opts.Capabilities != nil {
		ensureCacheCapabilities(opts.Capabilities)
		for name, capability := range map[string]*CapabilityPolicyOptions{
			"survey.questionnaire":         opts.Capabilities.Survey.Questionnaire,
			"modelcatalog.published_model": opts.Capabilities.ModelCatalog.PublishedModel,
			"evaluation.assessment_detail": opts.Capabilities.Evaluation.AssessmentDetail,
			"evaluation.assessment_list":   opts.Capabilities.Evaluation.AssessmentList,
			"actor.testee":                 opts.Capabilities.Actor.Testee,
			"plan.detail":                  opts.Capabilities.Plan.Detail,
			"statistics.query":             opts.Capabilities.Statistics.Query,
		} {
			errs = append(errs, validateCapabilityPolicy(name, capability)...)
		}
	}
	var warmup *WarmupOptions
	if opts.Governance != nil {
		warmup = opts.Governance.Warmup
	}
	if warmup != nil && warmup.Hotset != nil && warmup.Hotset.Enable {
		if warmup.Hotset.TopN <= 0 {
			errs = append(errs, fmt.Errorf("cache.warmup.hotset.top_n must be greater than 0 when enabled"))
		}
		if warmup.Hotset.MaxItemsPerKind <= 0 {
			errs = append(errs, fmt.Errorf("cache.warmup.hotset.max_items_per_kind must be greater than 0 when enabled"))
		}
	}
	return errs
}

func validateCapabilityPolicy(name string, capability *CapabilityPolicyOptions) []error {
	if capability == nil {
		return nil
	}
	var errs []error
	if capability.TTL < 0 {
		errs = append(errs, fmt.Errorf("cache.capabilities.%s.ttl cannot be negative", name))
	}
	if capability.NegativeTTL < 0 {
		errs = append(errs, fmt.Errorf("cache.capabilities.%s.negative_ttl cannot be negative", name))
	}
	if capability.TTLJitterRatio < 0 || capability.TTLJitterRatio > 1 {
		errs = append(errs, fmt.Errorf("cache.capabilities.%s.ttl_jitter_ratio must be between 0 and 1", name))
	}
	return errs
}

func validateCacheFamilyPolicy(name string, route *CacheFamilyOptions) []error {
	if route == nil {
		return nil
	}

	var errs []error
	if route.NegativeTTL < 0 {
		errs = append(errs, fmt.Errorf("cache.%s.negative_ttl cannot be negative", name))
	}
	if route.TTLJitterRatio < 0 || route.TTLJitterRatio > 1 {
		errs = append(errs, fmt.Errorf("cache.%s.ttl_jitter_ratio must be between 0 and 1", name))
	}
	return errs
}
