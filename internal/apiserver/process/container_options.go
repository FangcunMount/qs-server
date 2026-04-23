package process

import (
	"github.com/FangcunMount/component-base/pkg/messaging"
	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachebootstrap"
	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/eventconfig"
)

type containerOptionsInput struct {
	mqPublisher    messaging.Publisher
	publishMode    eventconfig.PublishMode
	cacheSubsystem *cachebootstrap.Subsystem
}

func (s *server) buildContainerOptions(input containerOptionsInput) container.ContainerOptions {
	return container.ContainerOptions{
		MQPublisher:                input.mqPublisher,
		PublisherMode:              input.publishMode,
		Cache:                      s.buildContainerCacheOptions(),
		CacheSubsystem:             input.cacheSubsystem,
		PlanEntryBaseURL:           s.config.Plan.EntryBaseURL,
		StatisticsRepairWindowDays: statisticsRepairWindowDays(s.config),
	}
}

func (s *server) buildContainerCacheOptions() container.ContainerCacheOptions {
	cacheCfg := s.config.Cache
	if cacheCfg == nil {
		return container.ContainerCacheOptions{}
	}

	var ttl container.ContainerCacheTTLOptions
	if cacheCfg.TTL != nil {
		ttl = container.ContainerCacheTTLOptions{
			Scale:            cacheCfg.TTL.Scale,
			ScaleList:        cacheCfg.TTL.ScaleList,
			Questionnaire:    cacheCfg.TTL.Questionnaire,
			AssessmentDetail: cacheCfg.TTL.AssessmentDetail,
			AssessmentList:   cacheCfg.TTL.AssessmentList,
			Testee:           cacheCfg.TTL.Testee,
			Plan:             cacheCfg.TTL.Plan,
			Negative:         cacheCfg.TTL.Negative,
		}
	}

	return container.ContainerCacheOptions{
		DisableEvaluationCache: cacheCfg.DisableEvaluationCache,
		DisableStatisticsCache: cacheCfg.DisableStatisticsCache,
		TTL:                    ttl,
		TTLJitterRatio:         cacheCfg.TTLJitterRatio,
		StatisticsWarmup:       buildStatisticsWarmupConfig(cacheCfg),
		Warmup:                 buildWarmupOptions(cacheCfg),
		CompressPayload:        cacheCfg.CompressPayload,
		Static:                 buildCacheFamilyOptions(cacheCfg.Static),
		Object:                 buildCacheFamilyOptions(cacheCfg.Object),
		Query:                  buildQueryFamilyOptions(cacheCfg.Query),
		Meta:                   container.ContainerCacheFamilyOptions{},
		SDK:                    buildCacheFamilyOptions(cacheCfg.SDK),
		Lock:                   buildCacheFamilyOptions(cacheCfg.Lock),
	}
}

func buildStatisticsWarmupConfig(cacheCfg *apiserveroptions.CacheOptions) *cachegov.StatisticsWarmupConfig {
	if cacheCfg == nil || cacheCfg.StatisticsWarmup == nil || !cacheCfg.StatisticsWarmup.Enable {
		return nil
	}
	return &cachegov.StatisticsWarmupConfig{
		OrgIDs:             cacheCfg.StatisticsWarmup.OrgIDs,
		QuestionnaireCodes: cacheCfg.StatisticsWarmup.QuestionnaireCodes,
		PlanIDs:            cacheCfg.StatisticsWarmup.PlanIDs,
	}
}

func buildWarmupOptions(cacheCfg *apiserveroptions.CacheOptions) container.ContainerWarmupOptions {
	if cacheCfg == nil || cacheCfg.Warmup == nil {
		return container.ContainerWarmupOptions{}
	}
	options := container.ContainerWarmupOptions{
		Enable: cacheCfg.Warmup.Enable,
	}
	if cacheCfg.Warmup.Startup != nil {
		options.StartupStatic = cacheCfg.Warmup.Startup.Static
		options.StartupQuery = cacheCfg.Warmup.Startup.Query
	}
	if cacheCfg.Warmup.Hotset != nil {
		options.HotsetEnable = cacheCfg.Warmup.Hotset.Enable
		options.HotsetTopN = cacheCfg.Warmup.Hotset.TopN
		options.MaxItemsPerKind = cacheCfg.Warmup.Hotset.MaxItemsPerKind
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

func buildQueryFamilyOptions(family *apiserveroptions.CacheFamilyOptions) container.ContainerCacheFamilyOptions {
	options := buildCacheFamilyOptions(family)
	if family != nil {
		options.TTL = family.TTL
	}
	return options
}

func statisticsRepairWindowDays(cfg *config.Config) int {
	if cfg.StatisticsSync == nil {
		return 0
	}
	return cfg.StatisticsSync.RepairWindowDays
}
