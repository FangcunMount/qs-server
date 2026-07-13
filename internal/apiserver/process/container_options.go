package process

import (
	"time"

	cachepolicy "github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/cache/governance"
	cachebootstrap "github.com/FangcunMount/qs-server/internal/apiserver/cache/subsystem"
	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
)

type containerOptionsInput struct {
	cacheSubsystem *cachebootstrap.Subsystem
	backpressure   container.BackpressureOptions
	eventSubsystem *eventsubsystem.Subsystem
}

func (s *server) buildContainerOptions(input containerOptionsInput) container.ContainerOptions {
	return container.ContainerOptions{
		EventSubsystem:             input.eventSubsystem,
		Cache:                      s.buildContainerCacheOptions(),
		CacheSubsystem:             input.cacheSubsystem,
		Backpressure:               input.backpressure,
		PlanEntryBaseURL:           s.config.Plan.EntryBaseURL,
		StatisticsRepairWindowDays: statisticsRepairWindowDays(s.config),
		ReportStatus:               s.config.Cache.Capabilities.ReportStatus,
		Signaling:                  s.config.Signaling,
		SystemGovernance:           s.config.SystemGovernance,
	}
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
		Capabilities:            capabilities,
		TTLJitterRatio:          defaults.TTLJitterRatio,
		StatisticsWarmup:        buildStatisticsWarmupConfig(cacheCfg),
		StatisticsSystem:        buildStatisticsSystemOptions(cacheCfg),
		StatisticsOverview:      buildStatisticsOverviewOptions(cacheCfg),
		StatisticsQuestionnaire: buildStatisticsQuestionnaireOptions(cacheCfg),
		Warmup:                  buildWarmupOptions(cacheCfg),
		CompressPayload:         defaults.CompressPayload,
		Static:                  buildCacheFamilyOptions(defaults.Static),
		Object:                  buildCacheFamilyOptions(defaults.Object),
		Query:                   buildCacheFamilyOptions(defaults.Query),
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
		QuestionnaireCodes: config.QuestionnaireCodes, PlanIDs: config.PlanIDs, WarmOnStartup: config.WarmOnStartup,
	}
}

func buildStatisticsSystemOptions(cacheCfg *apiserveroptions.CacheOptions) cachebootstrap.StatisticsSystemOptions {
	defaults := apiserveroptions.NewCacheOptions().Governance.StatisticsSystem
	if cacheCfg == nil || cacheCfg.Governance == nil || cacheCfg.Governance.StatisticsSystem == nil {
		if defaults == nil {
			return cachebootstrap.StatisticsSystemOptions{}
		}
		return cachebootstrap.StatisticsSystemOptions{
			ServiceSingleflight:     defaults.ServiceSingleflight,
			DisableRealtimeFallback: defaults.DisableRealtimeFallback,
			StaleOnTimeout:          defaults.StaleOnTimeout,
			LoadTimeout:             defaults.LoadTimeout,
		}
	}
	config := cacheCfg.Governance.StatisticsSystem
	return cachebootstrap.StatisticsSystemOptions{
		ServiceSingleflight: config.ServiceSingleflight, DisableRealtimeFallback: config.DisableRealtimeFallback,
		StaleOnTimeout: config.StaleOnTimeout, LoadTimeout: config.LoadTimeout,
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

func buildStatisticsQuestionnaireOptions(cacheCfg *apiserveroptions.CacheOptions) cachebootstrap.StatisticsReadGuardOptions {
	defaults := apiserveroptions.NewCacheOptions().Governance.StatisticsQuestionnaire
	if cacheCfg == nil || cacheCfg.Governance == nil || cacheCfg.Governance.StatisticsQuestionnaire == nil {
		if defaults == nil {
			return cachebootstrap.StatisticsReadGuardOptions{}
		}
		return cachebootstrap.StatisticsReadGuardOptions{
			ServiceSingleflight: defaults.ServiceSingleflight,
			StaleOnTimeout:      defaults.StaleOnTimeout,
			LoadTimeout:         defaults.LoadTimeout,
		}
	}
	config := cacheCfg.Governance.StatisticsQuestionnaire
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
