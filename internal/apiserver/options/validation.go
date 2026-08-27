package options

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

// Validate 验证命令行参数
func (o *Options) Validate() []error {
	var errs []error

	errs = append(errs, o.GenericServerRunOptions.Validate()...)
	if o.GRPCOptions == nil {
		errs = append(errs, fmt.Errorf("grpc is required"))
	} else {
		errs = append(errs, o.GRPCOptions.Validate()...)
	}
	errs = append(errs, o.MySQLOptions.Validate()...)
	if o.MongoDBOptions == nil {
		errs = append(errs, fmt.Errorf("mongodb is required"))
	} else {
		errs = append(errs, o.MongoDBOptions.Validate()...)
	}
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
	errs = append(errs, validateEvaluationConsistencyAudit(o.EvaluationConsistencyAudit)...)
	errs = append(errs, validateLeaseRecovery("evaluation_lease_recovery", o.EvaluationLeaseRecovery)...)
	errs = append(errs, validateLeaseRecovery("interpretation_lease_recovery", o.InterpretationLeaseRecovery)...)
	errs = append(errs, validateLeaseRecovery("ai_explanation_prompt_evaluation_lease_recovery", o.AIExplanationPromptEvaluationLeaseRecovery)...)
	errs = append(errs, validateLeaseRecovery("ai_explanation_participant_lease_recovery", o.AIExplanationParticipantLeaseRecovery)...)
	errs = append(errs, validateEvaluationMaintenanceLockIsolation(o)...)
	errs = append(errs, validateReportCatalogAudit(o.ReportCatalogAudit)...)
	errs = append(errs, validateMongoConsistencyAudit(o.MongoConsistencyAudit)...)
	errs = append(errs, validateOutboxRelay(o.OutboxRelay, o.MySQLOptions.MaxOpenConnections, o.Backpressure)...)
	errs = append(errs, validateStatisticsSync(o.StatisticsSync)...)
	errs = append(errs, validateCacheOptions(o.Cache)...)
	errs = append(errs, o.RuntimeState.Validate("runtime_state")...)
	errs = append(errs, validateSystemGovernance(o.SystemGovernance)...)
	if err := o.DelegatedSubject.Validate(); err != nil {
		errs = append(errs, err)
	}
	errs = append(errs, o.AIExplanation.Validate()...)
	if o.AIExplanationPromptEvaluationLeaseRecovery != nil && o.AIExplanationPromptEvaluationLeaseRecovery.Enable &&
		(o.AIExplanation == nil || !o.AIExplanation.Enabled || !o.AIExplanation.Evaluation.Enabled) {
		errs = append(errs, fmt.Errorf("ai_explanation_prompt_evaluation_lease_recovery requires ai_explanation.enabled and ai_explanation.evaluation.enabled"))
	}
	if o.AIExplanationParticipantLeaseRecovery != nil && o.AIExplanationParticipantLeaseRecovery.Enable &&
		(o.AIExplanation == nil || !o.AIExplanation.Enabled || !o.AIExplanation.ParticipantEnabled) {
		errs = append(errs, fmt.Errorf("ai_explanation_participant_lease_recovery requires ai_explanation.enabled and ai_explanation.participant_enabled"))
	}
	if o.AIExplanation != nil && o.AIExplanation.ParticipantEnabled && (o.DelegatedSubject == nil || !o.DelegatedSubject.Enabled) {
		errs = append(errs, fmt.Errorf("delegated_subject.enabled must be true when ai_explanation.participant_enabled is true"))
	}

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

func validateSystemGovernance(opts *SystemGovernanceOptions) []error {
	errs := validateRetryGovernance(opts)
	if opts == nil {
		return errs
	}
	names := make([]string, 0, len(opts.Components))
	for name := range opts.Components {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		component := opts.Components[name]
		if component == nil {
			continue
		}
		discovery := component.DiscoveryMode()
		if discovery != "single" && discovery != "dns" {
			errs = append(errs, fmt.Errorf("system_governance.components.%s.discovery must be single or dns", name))
			continue
		}
		if component.Timeout < 0 {
			errs = append(errs, fmt.Errorf("system_governance.components.%s.timeout cannot be negative", name))
		}
		if discovery == "dns" {
			if component.MinimumInstances < 1 || component.MinimumInstances > 16 {
				errs = append(errs, fmt.Errorf("system_governance.components.%s.minimum_instances must be between 1 and 16 for dns discovery", name))
			}
			for endpointName, endpoint := range map[string]string{
				"resilience_url":       component.ResilienceURL,
				"cache_url":            component.CacheURL,
				"cache_governance_url": component.CacheGovernanceURL,
			} {
				if strings.TrimSpace(endpoint) == "" {
					continue
				}
				parsed, err := url.Parse(endpoint)
				if err != nil || parsed.Scheme != "http" || parsed.Hostname() == "" {
					errs = append(errs, fmt.Errorf("system_governance.components.%s.%s must be an absolute http URL for dns discovery", name, endpointName))
				}
			}
		}
	}
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
	if lease := opts.Retry.Lease; lease != nil {
		if lease.RunDuration <= 0 {
			errs = append(errs, fmt.Errorf("system_governance.retry.lease.run_duration must be greater than 0"))
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
	} else if opts.PendingLookback != 24*time.Hour {
		errs = append(errs, fmt.Errorf("plan_scheduler.pending_lookback must remain 24h for task opening-window compatibility"))
	}
	if opts.BatchSize <= 0 {
		errs = append(errs, fmt.Errorf("plan_scheduler.batch_size must be greater than 0"))
	}
	if opts.MaxTasksPerTick <= 0 {
		errs = append(errs, fmt.Errorf("plan_scheduler.max_tasks_per_tick must be greater than 0"))
	} else if opts.BatchSize > opts.MaxTasksPerTick {
		errs = append(errs, fmt.Errorf("plan_scheduler.batch_size must be less than or equal to plan_scheduler.max_tasks_per_tick"))
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

func validateEvaluationConsistencyAudit(opts *EvaluationConsistencyAuditOptions) []error {
	if opts == nil || !opts.Enable {
		return nil
	}
	var errs []error
	if opts.InitialDelay < 0 {
		errs = append(errs, fmt.Errorf("evaluation_consistency_audit.initial_delay cannot be negative"))
	}
	if opts.BatchInterval <= 0 {
		errs = append(errs, fmt.Errorf("evaluation_consistency_audit.batch_interval must be greater than 0"))
	}
	if opts.CycleInterval < 6*time.Hour || opts.CycleInterval > 24*time.Hour {
		errs = append(errs, fmt.Errorf("evaluation_consistency_audit.cycle_interval must be between 6h and 24h"))
	}
	if opts.BatchSize <= 0 {
		errs = append(errs, fmt.Errorf("evaluation_consistency_audit.batch_size must be greater than 0"))
	}
	if opts.BatchTimeout <= 0 {
		errs = append(errs, fmt.Errorf("evaluation_consistency_audit.batch_timeout must be greater than 0"))
	}
	if opts.LockKey == "" {
		errs = append(errs, fmt.Errorf("evaluation_consistency_audit.lock_key cannot be empty when enabled"))
	}
	if opts.LockTTL <= 0 {
		errs = append(errs, fmt.Errorf("evaluation_consistency_audit.lock_ttl must be greater than 0"))
	}
	return errs
}

func validateLeaseRecovery(name string, opts *LeaseRecoveryOptions) []error {
	if opts == nil || !opts.Enable {
		return nil
	}
	var errs []error
	if opts.Interval < 10*time.Second || opts.Interval > 30*time.Second {
		errs = append(errs, fmt.Errorf("%s.interval must be between 10s and 30s", name))
	}
	if opts.BatchLimit <= 0 {
		errs = append(errs, fmt.Errorf("%s.batch_limit must be greater than 0", name))
	}
	if opts.LockKey == "" {
		errs = append(errs, fmt.Errorf("%s.lock_key cannot be empty when enabled", name))
	}
	if opts.LockTTL <= 0 {
		errs = append(errs, fmt.Errorf("%s.lock_ttl must be greater than 0", name))
	}
	return errs
}

func validateEvaluationMaintenanceLockIsolation(opts *Options) []error {
	if opts == nil {
		return nil
	}
	keys := make(map[string]string, 4)
	add := func(name, key string, enabled bool) []error {
		if !enabled || strings.TrimSpace(key) == "" {
			return nil
		}
		if owner, exists := keys[key]; exists {
			return []error{fmt.Errorf("%s.lock_key must be independent from %s.lock_key", name, owner)}
		}
		keys[key] = name
		return nil
	}

	var errs []error
	errs = append(errs, add("evaluation_consistency_audit", optionLockKey(opts.EvaluationConsistencyAudit), opts.EvaluationConsistencyAudit != nil && opts.EvaluationConsistencyAudit.Enable)...)
	errs = append(errs, add("evaluation_lease_recovery", leaseRecoveryLockKey(opts.EvaluationLeaseRecovery), opts.EvaluationLeaseRecovery != nil && opts.EvaluationLeaseRecovery.Enable)...)
	errs = append(errs, add("interpretation_lease_recovery", leaseRecoveryLockKey(opts.InterpretationLeaseRecovery), opts.InterpretationLeaseRecovery != nil && opts.InterpretationLeaseRecovery.Enable)...)
	errs = append(errs, add("ai_explanation_prompt_evaluation_lease_recovery", leaseRecoveryLockKey(opts.AIExplanationPromptEvaluationLeaseRecovery), opts.AIExplanationPromptEvaluationLeaseRecovery != nil && opts.AIExplanationPromptEvaluationLeaseRecovery.Enable)...)
	errs = append(errs, add("ai_explanation_participant_lease_recovery", leaseRecoveryLockKey(opts.AIExplanationParticipantLeaseRecovery), opts.AIExplanationParticipantLeaseRecovery != nil && opts.AIExplanationParticipantLeaseRecovery.Enable)...)
	return errs
}

func optionLockKey(opts *EvaluationConsistencyAuditOptions) string {
	if opts == nil {
		return ""
	}
	return opts.LockKey
}

func leaseRecoveryLockKey(opts *LeaseRecoveryOptions) string {
	if opts == nil {
		return ""
	}
	return opts.LockKey
}

func validateReportCatalogAudit(opts *ReportCatalogAuditOptions) []error {
	if opts == nil || !opts.Enable {
		return nil
	}
	var errs []error
	if opts.InitialDelay < 0 {
		errs = append(errs, fmt.Errorf("report_catalog_audit.initial_delay cannot be negative"))
	}
	if opts.TickInterval <= 0 {
		errs = append(errs, fmt.Errorf("report_catalog_audit.tick_interval must be greater than 0"))
	}
	if opts.CycleInterval <= 0 {
		errs = append(errs, fmt.Errorf("report_catalog_audit.cycle_interval must be greater than 0"))
	}
	if opts.BatchSize <= 0 || opts.BatchSize > 500 {
		errs = append(errs, fmt.Errorf("report_catalog_audit.batch_size must be between 1 and 500"))
	}
	if opts.BatchTimeout <= 0 {
		errs = append(errs, fmt.Errorf("report_catalog_audit.batch_timeout must be greater than 0"))
	}
	if opts.LockKey == "" {
		errs = append(errs, fmt.Errorf("report_catalog_audit.lock_key cannot be empty when enabled"))
	}
	if opts.LockTTL <= 0 {
		errs = append(errs, fmt.Errorf("report_catalog_audit.lock_ttl must be greater than 0"))
	}
	return errs
}

func validateMongoConsistencyAudit(opts *MongoConsistencyAuditOptions) []error {
	if opts == nil || !opts.Enable {
		return nil
	}
	var errs []error
	if opts.InitialDelay < 0 {
		errs = append(errs, fmt.Errorf("mongo_consistency_audit.initial_delay cannot be negative"))
	}
	if opts.TickInterval <= 0 {
		errs = append(errs, fmt.Errorf("mongo_consistency_audit.tick_interval must be greater than 0"))
	}
	if opts.CycleInterval <= 0 {
		errs = append(errs, fmt.Errorf("mongo_consistency_audit.cycle_interval must be greater than 0"))
	}
	if opts.BatchSize <= 0 || opts.BatchSize > 500 {
		errs = append(errs, fmt.Errorf("mongo_consistency_audit.batch_size must be between 1 and 500"))
	}
	if opts.BatchTimeout <= 0 {
		errs = append(errs, fmt.Errorf("mongo_consistency_audit.batch_timeout must be greater than 0"))
	}
	if opts.MaxSamples < 0 || opts.MaxSamples > 100 {
		errs = append(errs, fmt.Errorf("mongo_consistency_audit.max_samples must be between 0 and 100"))
	}
	if strings.TrimSpace(opts.LockKey) == "" {
		errs = append(errs, fmt.Errorf("mongo_consistency_audit.lock_key cannot be empty when enabled"))
	}
	if opts.LockTTL <= 0 {
		errs = append(errs, fmt.Errorf("mongo_consistency_audit.lock_ttl must be greater than 0"))
	}
	return errs
}

func validateOutboxRelay(opts *OutboxRelayOptions, mysqlMaxOpen int, backpressure *BackpressureOptions) []error {
	if opts == nil {
		return nil
	}

	var errs []error
	mongoMaxInflight := 0
	if backpressure != nil && backpressure.Mongo != nil && backpressure.Mongo.Enabled {
		mongoMaxInflight = backpressure.Mongo.MaxInflight
	}
	for _, relay := range []struct {
		name          string
		opt           *OutboxRelayStoreOptions
		maxWorkers    int
		capacityLabel string
	}{
		{
			name:          "mongo",
			opt:           opts.Mongo,
			maxWorkers:    maxDependencyPublishWorkers(mongoMaxInflight, 0.8),
			capacityLabel: "backpressure.mongo.max_inflight",
		},
		{
			name:          "assessment",
			opt:           opts.Assessment,
			maxWorkers:    maxDependencyPublishWorkers(mysqlMaxOpen, 0.8),
			capacityLabel: "mysql max_open",
		},
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
		if relay.maxWorkers > 0 && relay.opt.PublishWorkers > relay.maxWorkers {
			errs = append(errs, fmt.Errorf(
				"outbox_relay.%s.publish_workers (%d) must be <= %s * 0.8 (%d)",
				relay.name,
				relay.opt.PublishWorkers,
				relay.capacityLabel,
				relay.maxWorkers,
			))
		}
		if relay.opt.ImmediateMaxConcurrent <= 0 {
			errs = append(errs, fmt.Errorf("outbox_relay.%s.immediate_max_concurrent must be greater than 0", relay.name))
		}
		publisherConcurrency := relay.opt.PublishWorkers + relay.opt.ImmediateMaxConcurrent
		if relay.maxWorkers > 0 && publisherConcurrency > relay.maxWorkers {
			errs = append(errs, fmt.Errorf(
				"outbox_relay.%s publish_workers + immediate_max_concurrent (%d) must be <= %s * 0.8 (%d)",
				relay.name,
				publisherConcurrency,
				relay.capacityLabel,
				relay.maxWorkers,
			))
		}
	}
	return errs
}

func maxDependencyPublishWorkers(capacity int, ratio float64) int {
	if capacity <= 0 {
		return 0
	}
	if ratio <= 0 {
		ratio = 0.8
	}
	return int(float64(capacity) * ratio)
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
		errs = append(errs, fmt.Errorf("cache.defaults.ttl_jitter_ratio must be between 0 and 1"))
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
			"evaluation.assessment_access": opts.Capabilities.Evaluation.AssessmentAccess,
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
			errs = append(errs, fmt.Errorf("cache.governance.warmup.hotset.top_n must be greater than 0 when enabled"))
		}
		if warmup.Hotset.MaxItemsPerKind <= 0 {
			errs = append(errs, fmt.Errorf("cache.governance.warmup.hotset.max_items_per_kind must be greater than 0 when enabled"))
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
