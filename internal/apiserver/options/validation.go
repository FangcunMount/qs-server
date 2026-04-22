package options

import (
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
)

// Validate 验证命令行参数
func (o *Options) Validate() []error {
	var errs []error

	errs = append(errs, o.GenericServerRunOptions.Validate()...)
	errs = append(errs, o.MySQLOptions.Validate()...)
	errs = append(errs, o.Log.Validate()...)
	errs = append(errs, o.OSSOptions.Validate()...)
	errs = append(errs, validateRateLimit(o.RateLimit)...)
	errs = append(errs, validateBackpressureOptions(o.Backpressure)...)
	errs = append(errs, validatePlanScheduler(o.PlanScheduler)...)
	errs = append(errs, validateBehaviorPendingReconcile(o.BehaviorPendingReconcile)...)
	errs = append(errs, validateStatisticsSync(o.StatisticsSync)...)
	errs = append(errs, validateCacheOptions(o.Cache)...)

	errs = append(errs, redisplane.ValidateRuntimeOptions(
		o.RedisRuntime,
		[]redisplane.Family{
			redisplane.FamilyStatic,
			redisplane.FamilyObject,
			redisplane.FamilyQuery,
			redisplane.FamilyMeta,
			redisplane.FamilySDK,
			redisplane.FamilyLock,
		},
		o.RedisProfiles,
		"redis_runtime",
	)...)

	return errs
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
	if opts.TTLJitterRatio < 0 || opts.TTLJitterRatio > 1 {
		errs = append(errs, fmt.Errorf("cache.ttl_jitter_ratio must be between 0 and 1"))
	}
	for _, family := range []struct {
		name string
		opt  *CacheFamilyOptions
	}{
		{name: "static", opt: opts.Static},
		{name: "object", opt: opts.Object},
		{name: "query", opt: opts.Query},
		{name: "meta", opt: opts.Meta},
		{name: "sdk", opt: opts.SDK},
		{name: "lock", opt: opts.Lock},
	} {
		errs = append(errs, validateCacheFamilyPolicy(family.name, family.opt)...)
	}
	if opts.Warmup != nil && opts.Warmup.Hotset != nil && opts.Warmup.Hotset.Enable {
		if opts.Warmup.Hotset.TopN <= 0 {
			errs = append(errs, fmt.Errorf("cache.warmup.hotset.top_n must be greater than 0 when enabled"))
		}
		if opts.Warmup.Hotset.MaxItemsPerKind <= 0 {
			errs = append(errs, fmt.Errorf("cache.warmup.hotset.max_items_per_kind must be greater than 0 when enabled"))
		}
	}
	return errs
}

func validateCacheFamilyPolicy(name string, route *CacheFamilyOptions) []error {
	if route == nil {
		return nil
	}

	var errs []error
	if route.TTL < 0 {
		errs = append(errs, fmt.Errorf("cache.%s.ttl cannot be negative", name))
	}
	if route.NegativeTTL < 0 {
		errs = append(errs, fmt.Errorf("cache.%s.negative_ttl cannot be negative", name))
	}
	if route.TTLJitterRatio < 0 || route.TTLJitterRatio > 1 {
		errs = append(errs, fmt.Errorf("cache.%s.ttl_jitter_ratio must be between 0 and 1", name))
	}
	return errs
}
