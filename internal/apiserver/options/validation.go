package options

import "fmt"

// Validate 验证命令行参数
func (o *Options) Validate() []error {
	var errs []error

	errs = append(errs, o.GenericServerRunOptions.Validate()...)
	errs = append(errs, o.MySQLOptions.Validate()...)
	errs = append(errs, o.Log.Validate()...)
	if o.RateLimit != nil && o.RateLimit.Enabled {
		if o.RateLimit.SubmitGlobalQPS <= 0 || o.RateLimit.SubmitGlobalBurst <= 0 {
			errs = append(errs, fmt.Errorf("rate_limit.submit_* must be greater than 0"))
		}
		if o.RateLimit.SubmitUserQPS <= 0 || o.RateLimit.SubmitUserBurst <= 0 {
			errs = append(errs, fmt.Errorf("rate_limit.submit_user_* must be greater than 0"))
		}
		if o.RateLimit.AdminSubmitGlobalQPS <= 0 || o.RateLimit.AdminSubmitGlobalBurst <= 0 {
			errs = append(errs, fmt.Errorf("rate_limit.admin_submit_* must be greater than 0"))
		}
		if o.RateLimit.AdminSubmitUserQPS <= 0 || o.RateLimit.AdminSubmitUserBurst <= 0 {
			errs = append(errs, fmt.Errorf("rate_limit.admin_submit_user_* must be greater than 0"))
		}
		if o.RateLimit.QueryGlobalQPS <= 0 || o.RateLimit.QueryGlobalBurst <= 0 {
			errs = append(errs, fmt.Errorf("rate_limit.query_* must be greater than 0"))
		}
		if o.RateLimit.QueryUserQPS <= 0 || o.RateLimit.QueryUserBurst <= 0 {
			errs = append(errs, fmt.Errorf("rate_limit.query_user_* must be greater than 0"))
		}
		if o.RateLimit.WaitReportGlobalQPS <= 0 || o.RateLimit.WaitReportGlobalBurst <= 0 {
			errs = append(errs, fmt.Errorf("rate_limit.wait_report_* must be greater than 0"))
		}
		if o.RateLimit.WaitReportUserQPS <= 0 || o.RateLimit.WaitReportUserBurst <= 0 {
			errs = append(errs, fmt.Errorf("rate_limit.wait_report_user_* must be greater than 0"))
		}
	}

	if o.Backpressure != nil {
		validateBackpressure := func(name string, dep *DependencyBackpressure) {
			if dep == nil || !dep.Enabled {
				return
			}
			if dep.MaxInflight <= 0 {
				errs = append(errs, fmt.Errorf("backpressure.%s.max_inflight must be greater than 0", name))
			}
			if dep.TimeoutMs < 0 {
				errs = append(errs, fmt.Errorf("backpressure.%s.timeout_ms cannot be negative", name))
			}
		}
		validateBackpressure("mysql", o.Backpressure.MySQL)
		validateBackpressure("mongo", o.Backpressure.Mongo)
		validateBackpressure("iam", o.Backpressure.IAM)
	}

	if o.PlanScheduler != nil && o.PlanScheduler.Enable {
		if len(o.PlanScheduler.OrgIDs) == 0 {
			errs = append(errs, fmt.Errorf("plan_scheduler.org_ids cannot be empty when enabled"))
		}
		if o.PlanScheduler.InitialDelay < 0 {
			errs = append(errs, fmt.Errorf("plan_scheduler.initial_delay cannot be negative"))
		}
		if o.PlanScheduler.Interval <= 0 {
			errs = append(errs, fmt.Errorf("plan_scheduler.interval must be greater than 0"))
		}
		if o.PlanScheduler.LockKey == "" {
			errs = append(errs, fmt.Errorf("plan_scheduler.lock_key cannot be empty when enabled"))
		}
		if o.PlanScheduler.LockTTL <= 0 {
			errs = append(errs, fmt.Errorf("plan_scheduler.lock_ttl must be greater than 0"))
		}
		if o.PlanScheduler.Interval > 0 && o.PlanScheduler.LockTTL > o.PlanScheduler.Interval {
			errs = append(errs, fmt.Errorf("plan_scheduler.lock_ttl must be less than or equal to plan_scheduler.interval"))
		}
	}

	if o.Cache != nil {
		if o.Cache.TTLJitterRatio < 0 || o.Cache.TTLJitterRatio > 1 {
			errs = append(errs, fmt.Errorf("cache.ttl_jitter_ratio must be between 0 and 1"))
		}
		validateCacheRoute := func(name string, route *CacheRouteOptions) {
			if route == nil {
				return
			}
			if route.TTL < 0 {
				errs = append(errs, fmt.Errorf("cache.%s.ttl cannot be negative", name))
			}
			if route.NegativeTTL < 0 {
				errs = append(errs, fmt.Errorf("cache.%s.negative_ttl cannot be negative", name))
			}
			if route.TTLJitterRatio < 0 || route.TTLJitterRatio > 1 {
				errs = append(errs, fmt.Errorf("cache.%s.ttl_jitter_ratio must be between 0 and 1", name))
			}
			if route.RedisProfile != "" && len(o.RedisProfiles) > 0 {
				if _, ok := o.RedisProfiles[route.RedisProfile]; !ok {
					errs = append(errs, fmt.Errorf("cache.%s.redis_profile references missing redis_profiles entry %q", name, route.RedisProfile))
				}
			}
		}
		validateCacheRoute("static", o.Cache.Static)
		validateCacheRoute("object", o.Cache.Object)
		validateCacheRoute("query", o.Cache.Query)
		validateCacheRoute("meta", o.Cache.Meta)
		validateCacheRoute("sdk", o.Cache.SDK)
		validateCacheRoute("lock", o.Cache.Lock)

		if o.Cache.Warmup != nil && o.Cache.Warmup.Hotset != nil && o.Cache.Warmup.Hotset.Enable {
			if o.Cache.Warmup.Hotset.TopN <= 0 {
				errs = append(errs, fmt.Errorf("cache.warmup.hotset.top_n must be greater than 0 when enabled"))
			}
			if o.Cache.Warmup.Hotset.MaxItemsPerKind <= 0 {
				errs = append(errs, fmt.Errorf("cache.warmup.hotset.max_items_per_kind must be greater than 0 when enabled"))
			}
		}
		if o.Cache.Meta != nil && o.Cache.Meta.RedisProfile != "" && o.Cache.Warmup != nil && o.Cache.Warmup.Hotset != nil && o.Cache.Warmup.Hotset.Enable {
			if len(o.RedisProfiles) > 0 {
				if _, ok := o.RedisProfiles[o.Cache.Meta.RedisProfile]; !ok {
					errs = append(errs, fmt.Errorf("cache.meta.redis_profile %q is required when hotset governance is enabled", o.Cache.Meta.RedisProfile))
				}
			}
		}
	}

	return errs
}
