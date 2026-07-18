package process

import (
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	systemgov "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	cachepolicy "github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/cache/governance"
	cachebootstrap "github.com/FangcunMount/qs-server/internal/apiserver/cache/subsystem"
	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	mysqlsystemgov "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/systemgovernance"
	redissystemgov "github.com/FangcunMount/qs-server/internal/apiserver/infra/redis/systemgovernance"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	resiliencesubsystem "github.com/FangcunMount/qs-server/internal/apiserver/resilience/subsystem"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	cacheplanebootstrap "github.com/FangcunMount/qs-server/internal/pkg/redisruntime/bootstrap"
	redisobserve "github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	controlredis "github.com/FangcunMount/qs-server/internal/pkg/resilience/control/redisadapter"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
	locksubsystem "github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease/subsystem"
	"gorm.io/gorm"
)

type containerOptionsInput struct {
	cacheSubsystem    *cachebootstrap.Subsystem
	resilience        *resiliencesubsystem.Subsystem
	eventSubsystem    *eventsubsystem.Subsystem
	actionAuditStore  systemgov.ActionAuditStore
	actionAuditRunner *systemgov.ActionAuditRecoveryRunner
}

func (s *server) buildContainerOptions(input containerOptionsInput) container.ContainerOptions {
	resilience := input.resilience
	var locks *locksubsystem.Subsystem
	if resilience != nil {
		locks = resilience.Locks()
	}
	return container.ContainerOptions{
		EventSubsystem:             input.eventSubsystem,
		Cache:                      s.buildContainerCacheOptions(),
		CacheSubsystem:             input.cacheSubsystem,
		LockSubsystem:              locks,
		Resilience:                 resilience,
		PlanEntryBaseURL:           s.config.Plan.EntryBaseURL,
		StatisticsRepairWindowDays: statisticsRepairWindowDays(s.config),
		ReportStatus:               s.config.Cache.Capabilities.ReportStatus,
		Signaling:                  s.config.Signaling,
		SystemGovernance:           s.config.SystemGovernance,
		ActionAuditStore:           input.actionAuditStore,
		ActionAuditRunner:          input.actionAuditRunner,
	}
}

func buildActionAuditRuntime(db *gorm.DB, runtime *cacheplanebootstrap.RuntimeBundle) (systemgov.ActionAuditStore, *systemgov.ActionAuditRecoveryRunner) {
	primary := mysqlsystemgov.NewActionAuditStore(db)
	var fallback systemgov.ActionAuditFallbackStore
	if runtime != nil {
		if ops := runtime.Handle(redisruntime.FamilyOps); ops != nil && ops.Client != nil {
			fallback = redissystemgov.NewActionAuditFallbackStore(ops.Client, ops.Builder)
		}
	}
	recoverable := systemgov.NewRecoverableActionAuditStore(primary, fallback)
	runner := systemgov.NewActionAuditRecoveryRunner(recoverable, func(err error) {
		log.Warnf("governance audit recovery failed: %v", err)
	})
	return recoverable, runner
}

func (s *server) buildResilienceSubsystem(runtime *cacheplanebootstrap.RuntimeBundle) (*resiliencesubsystem.Subsystem, error) {
	renewalEnabled := s.config.LockLease != nil && s.config.LockLease.RenewalEnabled
	var lockHandle *redisruntime.Handle
	var lockStatus *redisobserve.FamilyStatusRegistry
	if runtime != nil {
		lockHandle = runtime.Handle(redisruntime.FamilyLock)
		lockStatus = runtime.StatusRegistry
	}
	locks := locksubsystem.New(locksubsystem.Options{
		Component:      "apiserver",
		Handle:         lockHandle,
		StatusRegistry: lockStatus,
		RenewalEnabled: renewalEnabled,
		Warn:           func(message string) { log.Warn(message) },
		EnabledWorkloads: map[locklease.WorkloadID]bool{
			locklease.WorkloadPlanSchedulerLeader:            s.config.PlanScheduler != nil && s.config.PlanScheduler.Enable,
			locklease.WorkloadStatisticsSyncLeader:           s.config.StatisticsSync != nil && s.config.StatisticsSync.Enable,
			locklease.WorkloadStatisticsSync:                 true,
			locklease.WorkloadBehaviorPendingReconcile:       s.config.BehaviorPendingReconcile != nil && s.config.BehaviorPendingReconcile.Enable,
			locklease.WorkloadEvaluationConsistencyReconcile: s.config.EvaluationConsistencyReconcile != nil && s.config.EvaluationConsistencyReconcile.Enable,
			locklease.WorkloadBehaviorJourneyScanLeader:      s.config.BehaviorJourneyScan != nil && s.config.BehaviorJourneyScan.Enable,
		},
	})
	var stateStore *controlredis.Store
	if runtime != nil {
		if ops := runtime.Handle(redisruntime.FamilyOps); ops != nil {
			stateStore = controlredis.NewStore(ops.Client, ops.Builder)
		}
	}
	return resiliencesubsystem.New(resiliencesubsystem.Options{
		RateLimit:    s.config.RateLimit,
		Backpressure: s.config.Backpressure,
		Locks:        locks,
		StateStore:   stateStore,
	})
}

func buildEventConsumerOptions(cfg *config.Config) map[string]eventsubsystem.ConsumerOptions {
	result := map[string]eventsubsystem.ConsumerOptions{}
	if cfg == nil || cfg.Eventing == nil || cfg.Eventing.Consumers == nil || cfg.Eventing.Consumers.ModelCatalogHotRank == nil {
		return result
	}
	hotRank := cfg.Eventing.Consumers.ModelCatalogHotRank
	result["modelcatalog.hot_rank_projection"] = eventsubsystem.ConsumerOptions{Enabled: hotRank.Enabled, Channel: hotRank.Channel}
	return result
}

func buildEventProfileOptions(cfg *config.Config) (eventsubsystem.ProfileOptions, eventsubsystem.ProfileOptions) {
	if cfg == nil || cfg.OutboxRelay == nil {
		return eventsubsystem.ProfileOptions{}, eventsubsystem.ProfileOptions{}
	}
	mongoProfile := eventsubsystem.ProfileOptions{}
	assessmentProfile := eventsubsystem.ProfileOptions{}
	if cfg.OutboxRelay.Mongo != nil {
		mongoProfile.Interval = cfg.OutboxRelay.Mongo.Interval
		mongoProfile.BatchSize = cfg.OutboxRelay.Mongo.BatchSize
		mongoProfile.PublishWorkers = cfg.OutboxRelay.Mongo.PublishWorkers
		mongoProfile.ImmediateMaxConcurrent = cfg.OutboxRelay.Mongo.ImmediateMaxConcurrent
	}
	if cfg.OutboxRelay.Assessment != nil {
		assessmentProfile.Interval = cfg.OutboxRelay.Assessment.Interval
		assessmentProfile.BatchSize = cfg.OutboxRelay.Assessment.BatchSize
		assessmentProfile.PublishWorkers = cfg.OutboxRelay.Assessment.PublishWorkers
		assessmentProfile.ImmediateMaxConcurrent = cfg.OutboxRelay.Assessment.ImmediateMaxConcurrent
	}
	return mongoProfile, assessmentProfile
}

func (s *server) buildContainerCacheOptions() container.ContainerCacheOptions {
	if s == nil || s.config == nil {
		return container.ContainerCacheOptions{}
	}
	options := buildContainerCacheOptions(s.config.Cache)
	options.Signal = buildCacheSignalOptions(s.config.Signaling)
	return options
}

func buildCacheSignalOptions(signaling *genericoptions.SignalingOptions) cachebootstrap.SignalOptions {
	options := cachebootstrap.SignalOptions{Prefix: "qs:signal", BufferSize: 100}
	if signaling == nil || signaling.Redis == nil {
		return options
	}
	redis := signaling.Redis
	options.Enabled = redis.Enabled
	if redis.Prefix != "" {
		options.Prefix = redis.Prefix
	}
	options.Channel = redis.Channel
	if redis.BufferSize > 0 {
		options.BufferSize = redis.BufferSize
	}
	return options
}

func buildContainerCacheOptions(cacheCfg *apiserveroptions.CacheOptions) container.ContainerCacheOptions {
	if cacheCfg == nil {
		return container.ContainerCacheOptions{}
	}
	capabilities := map[sharedcache.Capability]cachepolicy.Binding{}
	if c := cacheCfg.Capabilities; c != nil {
		capabilities[cachepolicy.CapabilitySurveyQuestionnaire] = buildCapabilityBinding(c.Survey.Questionnaire)
		capabilities[cachepolicy.CapabilityModelCatalogPublished] = buildCapabilityBinding(c.ModelCatalog.PublishedModel)
		capabilities[cachepolicy.CapabilityEvaluationAssessmentDetail] = buildCapabilityBinding(c.Evaluation.AssessmentDetail)
		capabilities[cachepolicy.CapabilityEvaluationAssessmentList] = buildCapabilityBinding(c.Evaluation.AssessmentList)
		capabilities[cachepolicy.CapabilityActorTestee] = buildCapabilityBinding(c.Actor.Testee)
		capabilities[cachepolicy.CapabilityPlanDetail] = buildCapabilityBinding(c.Plan.Detail)
		capabilities[cachepolicy.CapabilityStatisticsQuery] = buildCapabilityBinding(c.Statistics.Query)
		reportStatus := cachepolicy.Binding{Enabled: true}
		if c.ReportStatus != nil {
			reportStatus.Policy.TTL = time.Duration(c.ReportStatus.TTLSeconds) * time.Second
		}
		capabilities[cachepolicy.CapabilityReportStatus] = reportStatus
	}
	defaults := cacheCfg.Defaults
	return container.ContainerCacheOptions{
		Capabilities:       capabilities,
		TTLJitterRatio:     defaults.TTLJitterRatio,
		StatisticsWarmup:   buildStatisticsWarmupConfig(cacheCfg),
		StatisticsOverview: buildStatisticsOverviewOptions(cacheCfg),
		Warmup:             buildWarmupOptions(cacheCfg),
		CompressPayload:    defaults.CompressPayload,
		Static:             buildCacheFamilyOptions(defaults.Static),
		Object:             buildCacheFamilyOptions(defaults.Object),
		Query:              buildCacheFamilyOptions(defaults.Query),
	}
}

func buildCapabilityBinding(capability *apiserveroptions.CapabilityPolicyOptions) cachepolicy.Binding {
	if capability == nil {
		return cachepolicy.Binding{}
	}
	return cachepolicy.Binding{Enabled: capability.Enabled, Policy: sharedcache.Policy{
		TTL: capability.TTL, NegativeTTL: capability.NegativeTTL, JitterRatio: capability.TTLJitterRatio,
		Compress:     sharedcache.PolicySwitchFromBoolPtr(capability.Compress),
		Singleflight: sharedcache.PolicySwitchFromBoolPtr(capability.Singleflight),
		Negative:     sharedcache.PolicySwitchFromBoolPtr(capability.Negative),
	}}
}

func buildStatisticsWarmupConfig(cacheCfg *apiserveroptions.CacheOptions) *cachegov.StatisticsWarmupConfig {
	if cacheCfg == nil || cacheCfg.Governance == nil || cacheCfg.Governance.StatisticsWarmup == nil || !cacheCfg.Governance.StatisticsWarmup.Enable {
		return nil
	}
	config := cacheCfg.Governance.StatisticsWarmup
	return &cachegov.StatisticsWarmupConfig{
		OrgIDs: config.OrgIDs, OverviewPresets: config.OverviewPresets,
		WarmOnStartup: config.WarmOnStartup,
	}
}

func buildStatisticsOverviewOptions(cacheCfg *apiserveroptions.CacheOptions) cachebootstrap.StatisticsReadGuardOptions {
	defaults := apiserveroptions.NewCacheOptions().Governance.StatisticsOverview
	if cacheCfg == nil || cacheCfg.Governance == nil || cacheCfg.Governance.StatisticsOverview == nil {
		if defaults == nil {
			return cachebootstrap.StatisticsReadGuardOptions{}
		}
		return cachebootstrap.StatisticsReadGuardOptions{
			ServiceSingleflight: defaults.ServiceSingleflight,
			StaleOnTimeout:      defaults.StaleOnTimeout,
			LoadTimeout:         defaults.LoadTimeout,
		}
	}
	config := cacheCfg.Governance.StatisticsOverview
	return cachebootstrap.StatisticsReadGuardOptions{
		ServiceSingleflight: config.ServiceSingleflight, StaleOnTimeout: config.StaleOnTimeout, LoadTimeout: config.LoadTimeout,
	}
}

func buildWarmupOptions(cacheCfg *apiserveroptions.CacheOptions) container.ContainerWarmupOptions {
	if cacheCfg == nil || cacheCfg.Governance == nil || cacheCfg.Governance.Warmup == nil {
		return container.ContainerWarmupOptions{}
	}
	warmup := cacheCfg.Governance.Warmup
	options := container.ContainerWarmupOptions{
		Enable: warmup.Enable,
	}
	if warmup.Startup != nil {
		options.StartupStatic = warmup.Startup.Static
		options.StartupQuery = warmup.Startup.Query
	}
	if warmup.Hotset != nil {
		options.HotsetEnable = warmup.Hotset.Enable
		options.HotsetTopN = warmup.Hotset.TopN
		options.MaxItemsPerKind = warmup.Hotset.MaxItemsPerKind
	}
	return options
}

func buildCacheFamilyOptions(family *apiserveroptions.CacheFamilyOptions) container.ContainerCacheFamilyOptions {
	if family == nil {
		return container.ContainerCacheFamilyOptions{}
	}
	return container.ContainerCacheFamilyOptions{
		NegativeTTL:    family.NegativeTTL,
		TTLJitterRatio: family.TTLJitterRatio,
		Compress:       family.Compress,
		Singleflight:   family.Singleflight,
		Negative:       family.Negative,
	}
}

func statisticsRepairWindowDays(cfg *config.Config) int {
	if cfg.StatisticsSync == nil {
		return 0
	}
	return cfg.StatisticsSync.RepairWindowDays
}
