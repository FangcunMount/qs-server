package cachebootstrap

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/application/cachegovernance"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachemodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachehotset"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/bootstrap"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease/redisadapter"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	redis "github.com/redis/go-redis/v9"
)

// GovernanceBindings 描述 warmup/governance 最终装配所需的业务回调。
type GovernanceBindings struct {
	ListPublishedScaleCodes         func(context.Context) ([]string, error)
	ListPublishedQuestionnaireCodes func(context.Context) ([]string, error)
	LookupScaleQuestionnaireCode    func(context.Context, string) (string, error)
	WarmScale                       func(context.Context, string) error
	WarmQuestionnaire               func(context.Context, string) error
	WarmScaleList                   func(context.Context) error
	WarmStatsSystem                 func(context.Context, int64) error
	WarmStatsQuestionnaire          func(context.Context, int64, string) error
	WarmStatsPlan                   func(context.Context, int64, uint64) error
}

// Subsystem 收口 apiserver cache 子系统运行时与治理装配。
type Subsystem struct {
	component   string
	cacheConfig CacheOptions

	statusRegistry *observability.FamilyStatusRegistry
	runtime        *cacheplane.Runtime
	handles        map[cacheplane.Family]*cacheplane.Handle
	policyCatalog  *cachepolicy.PolicyCatalog
	observer       *observability.ComponentObserver

	hotsetRecorder  cachetarget.HotsetRecorder
	hotsetInspector cachetarget.HotsetInspector
	lockManager     locklease.Manager

	warmupCoordinator cachegov.Coordinator
	statusService     cachegov.StatusService
}

// NewSubsystem 创建 cache 子系统组合根，并完成 runtime/policy/hotset/lock 的基础装配。
func NewSubsystem(component string, resolver cacheplane.Resolver, runtimeOptions *genericoptions.RedisRuntimeOptions, cacheConfig CacheOptions) *Subsystem {
	if component == "" {
		component = "apiserver"
	}
	return NewSubsystemFromRuntime(cacheplanebootstrap.BuildRuntime(context.Background(), cacheplanebootstrap.Options{
		Component:      component,
		RuntimeOptions: runtimeOptions,
		Resolver:       resolver,
		LockName:       "lock_lease",
	}), cacheConfig)
}

// NewSubsystemFromRuntime creates cache governance and policy wiring from a shared Redis runtime bundle.
func NewSubsystemFromRuntime(runtimeBundle *cacheplanebootstrap.RuntimeBundle, cacheConfig CacheOptions) *Subsystem {
	component := "apiserver"
	var statusRegistry *observability.FamilyStatusRegistry
	var runtime *cacheplane.Runtime
	var handles map[cacheplane.Family]*cacheplane.Handle
	var lockManager locklease.Manager
	if runtimeBundle != nil {
		if runtimeBundle.Component != "" {
			component = runtimeBundle.Component
		}
		statusRegistry = runtimeBundle.StatusRegistry
		runtime = runtimeBundle.Runtime
		handles = runtimeBundle.Handles
		lockManager = runtimeBundle.LockManager
	}
	if statusRegistry == nil {
		statusRegistry = observability.NewFamilyStatusRegistry(component)
	}

	s := &Subsystem{
		component:      component,
		cacheConfig:    cacheConfig,
		statusRegistry: statusRegistry,
		runtime:        runtime,
		handles:        handles,
		observer:       observability.NewComponentObserver(component),
	}
	s.policyCatalog = newPolicyCatalog(cacheConfig)
	s.hotsetRecorder = cachehotset.NewRedisStoreWithObserver(
		s.Client(cacheplane.FamilyMeta),
		s.Builder(cacheplane.FamilyMeta),
		cachehotset.Options{
			Enable:          cacheConfig.Warmup.HotsetEnable,
			TopN:            cacheConfig.Warmup.HotsetTopN,
			MaxItemsPerKind: cacheConfig.Warmup.MaxItemsPerKind,
		},
		s.observer,
	)
	if inspector, ok := s.hotsetRecorder.(cachetarget.HotsetInspector); ok {
		s.hotsetInspector = inspector
	}
	s.lockManager = lockManager
	if s.lockManager == nil {
		s.lockManager = redisadapter.NewManager(component, "lock_lease", s.Handle(cacheplane.FamilyLock))
	}
	s.warnMetaCacheAvailability()
	return s
}

// BindGovernance 在 warmup callbacks 就绪后完成 governance/status 的最终装配。
func (s *Subsystem) BindGovernance(bindings GovernanceBindings) {
	if s == nil {
		return
	}
	s.warmupCoordinator = cachegov.NewCoordinator(cachegov.Config{
		Enable:          s.cacheConfig.Warmup.Enable,
		StartupStatic:   s.cacheConfig.Warmup.StartupStatic,
		StartupQuery:    s.cacheConfig.Warmup.StartupQuery,
		HotsetEnable:    s.cacheConfig.Warmup.HotsetEnable,
		HotsetTopN:      s.cacheConfig.Warmup.HotsetTopN,
		MaxItemsPerKind: s.cacheConfig.Warmup.MaxItemsPerKind,
	}, cachegov.Dependencies{
		Runtime:                         cachegov.NewFamilyRuntime(s.warmupFamilies()),
		StatisticsSeeds:                 s.cacheConfig.StatisticsWarmup,
		Hotset:                          s.hotsetRecorder,
		ListPublishedScaleCodes:         bindings.ListPublishedScaleCodes,
		ListPublishedQuestionnaireCodes: bindings.ListPublishedQuestionnaireCodes,
		LookupScaleQuestionnaireCode:    bindings.LookupScaleQuestionnaireCode,
		WarmScale:                       bindings.WarmScale,
		WarmQuestionnaire:               bindings.WarmQuestionnaire,
		WarmScaleList:                   bindings.WarmScaleList,
		WarmStatsSystem:                 bindings.WarmStatsSystem,
		WarmStatsQuestionnaire:          bindings.WarmStatsQuestionnaire,
		WarmStatsPlan:                   bindings.WarmStatsPlan,
	})
	s.statusService = cachegov.NewStatusService(s.component, s.statusRegistry, s.hotsetInspector, s.warmupCoordinator)
}

func (s *Subsystem) Component() string {
	if s == nil {
		return ""
	}
	return s.component
}

func (s *Subsystem) StatusRegistry() *observability.FamilyStatusRegistry {
	if s == nil {
		return nil
	}
	return s.statusRegistry
}

func (s *Subsystem) Runtime() *cacheplane.Runtime {
	if s == nil {
		return nil
	}
	return s.runtime
}

func (s *Subsystem) Handle(family cacheplane.Family) *cacheplane.Handle {
	if s == nil {
		return nil
	}
	if s.handles != nil {
		if handle, ok := s.handles[family]; ok {
			return handle
		}
	}
	if s.runtime == nil {
		return nil
	}
	return s.runtime.Handle(context.Background(), family)
}

func (s *Subsystem) Client(family cacheplane.Family) redis.UniversalClient {
	handle := s.Handle(family)
	if handle == nil {
		return nil
	}
	return handle.Client
}

func (s *Subsystem) Builder(family cacheplane.Family) *keyspace.Builder {
	handle := s.Handle(family)
	if handle == nil {
		return nil
	}
	return handle.Builder
}

func (s *Subsystem) Policy(key cachepolicy.CachePolicyKey) cachepolicy.CachePolicy {
	if s == nil || s.policyCatalog == nil {
		return cachepolicy.CachePolicy{}
	}
	return s.policyCatalog.Policy(key)
}

func (s *Subsystem) Observer() *observability.ComponentObserver {
	if s == nil {
		return nil
	}
	return s.observer
}

func (s *Subsystem) HotsetRecorder() cachetarget.HotsetRecorder {
	if s == nil {
		return nil
	}
	return s.hotsetRecorder
}

func (s *Subsystem) HotsetInspector() cachetarget.HotsetInspector {
	if s == nil {
		return nil
	}
	return s.hotsetInspector
}

func (s *Subsystem) LockManager() locklease.Manager {
	if s == nil {
		return nil
	}
	return s.lockManager
}

func (s *Subsystem) WarmupCoordinator() cachegov.Coordinator {
	if s == nil {
		return nil
	}
	return s.warmupCoordinator
}

func (s *Subsystem) StatusService() cachegov.StatusService {
	if s == nil {
		return nil
	}
	return s.statusService
}

func (s *Subsystem) warnMetaCacheAvailability() {
	metaHandle := s.Handle(cacheplane.FamilyMeta)
	metaRedisCache := s.Client(cacheplane.FamilyMeta)
	if s.cacheConfig.Warmup.HotsetEnable && metaRedisCache == nil {
		logger.L(context.Background()).Warnw("meta_cache unavailable while hotset governance is enabled; hotset recording and hot-target warmup will degrade",
			"component", s.component,
			"family", string(cacheplane.FamilyMeta),
			"profile", handleProfile(metaHandle),
		)
	}
	if metaRedisCache == nil {
		logger.L(context.Background()).Warnw("meta_cache unavailable; version-token query caches will run uncached where required",
			"component", s.component,
			"family", string(cacheplane.FamilyMeta),
			"profile", handleProfile(metaHandle),
		)
	}
}

func handleProfile(handle *cacheplane.Handle) string {
	if handle == nil {
		return ""
	}
	return handle.Profile
}

func (s *Subsystem) warmupFamilies() map[cachemodel.Family]bool {
	families := map[cachemodel.Family]bool{}
	if staticHandle := s.Handle(cacheplane.FamilyStatic); staticHandle != nil {
		families[cachemodel.FamilyStatic] = staticHandle.AllowWarmup
	}
	if queryHandle := s.Handle(cacheplane.FamilyQuery); queryHandle != nil {
		families[cachemodel.FamilyQuery] = queryHandle.AllowWarmup
	}
	return families
}

func newPolicyCatalog(cacheConfig CacheOptions) *cachepolicy.PolicyCatalog {
	return cachepolicy.NewPolicyCatalog(map[cachemodel.Family]cachepolicy.CachePolicy{
		cachemodel.FamilyStatic: {
			Compress:     resolvePolicySwitch(cacheConfig.Static.Compress, cacheConfig.CompressPayload),
			Singleflight: resolvePolicySwitch(cacheConfig.Static.Singleflight, true),
			Negative:     resolvePolicySwitch(cacheConfig.Static.Negative, false),
			NegativeTTL:  firstPositiveDuration(cacheConfig.Static.NegativeTTL, cacheConfig.TTL.Negative),
			JitterRatio:  firstPositiveFloat(cacheConfig.Static.TTLJitterRatio, cacheConfig.TTLJitterRatio),
		},
		cachemodel.FamilyObject: {
			Compress:     resolvePolicySwitch(cacheConfig.Object.Compress, cacheConfig.CompressPayload),
			Singleflight: resolvePolicySwitch(cacheConfig.Object.Singleflight, true),
			Negative:     resolvePolicySwitch(cacheConfig.Object.Negative, false),
			NegativeTTL:  firstPositiveDuration(cacheConfig.Object.NegativeTTL, cacheConfig.TTL.Negative),
			JitterRatio:  firstPositiveFloat(cacheConfig.Object.TTLJitterRatio, cacheConfig.TTLJitterRatio),
		},
		cachemodel.FamilyQuery: {
			TTL:          cacheConfig.Query.TTL,
			NegativeTTL:  firstPositiveDuration(cacheConfig.Query.NegativeTTL, cacheConfig.TTL.Negative),
			Compress:     resolvePolicySwitch(cacheConfig.Query.Compress, cacheConfig.CompressPayload),
			Singleflight: resolvePolicySwitch(cacheConfig.Query.Singleflight, false),
			Negative:     resolvePolicySwitch(cacheConfig.Query.Negative, false),
			JitterRatio:  firstPositiveFloat(cacheConfig.Query.TTLJitterRatio, cacheConfig.TTLJitterRatio),
		},
		cachemodel.FamilySDK: {
			Compress:     resolvePolicySwitch(cacheConfig.SDK.Compress, false),
			Singleflight: resolvePolicySwitch(cacheConfig.SDK.Singleflight, false),
			Negative:     resolvePolicySwitch(cacheConfig.SDK.Negative, false),
			NegativeTTL:  cacheConfig.SDK.NegativeTTL,
			JitterRatio:  firstPositiveFloat(cacheConfig.SDK.TTLJitterRatio, cacheConfig.TTLJitterRatio),
		},
		cachemodel.FamilyLock: {
			Compress:     resolvePolicySwitch(cacheConfig.Lock.Compress, false),
			Singleflight: resolvePolicySwitch(cacheConfig.Lock.Singleflight, false),
			Negative:     resolvePolicySwitch(cacheConfig.Lock.Negative, false),
			NegativeTTL:  cacheConfig.Lock.NegativeTTL,
			JitterRatio:  firstPositiveFloat(cacheConfig.Lock.TTLJitterRatio, cacheConfig.TTLJitterRatio),
		},
	}, map[cachepolicy.CachePolicyKey]cachepolicy.CachePolicy{
		cachepolicy.PolicyScale: {
			TTL: cacheConfig.TTL.Scale,
		},
		cachepolicy.PolicyScaleList: {
			TTL:          cacheConfig.TTL.ScaleList,
			Singleflight: cachepolicy.PolicySwitchDisabled,
		},
		cachepolicy.PolicyQuestionnaire: {
			TTL:         cacheConfig.TTL.Questionnaire,
			NegativeTTL: cacheConfig.TTL.Negative,
			Negative:    cachepolicy.PolicySwitchEnabled,
		},
		cachepolicy.PolicyAssessmentDetail: {
			TTL:          cacheConfig.TTL.AssessmentDetail,
			Singleflight: cachepolicy.PolicySwitchEnabled,
		},
		cachepolicy.PolicyAssessmentList: {
			TTL:          cacheConfig.TTL.AssessmentList,
			Singleflight: cachepolicy.PolicySwitchDisabled,
		},
		cachepolicy.PolicyTestee: {
			TTL:         cacheConfig.TTL.Testee,
			NegativeTTL: cacheConfig.TTL.Negative,
			Negative:    cachepolicy.PolicySwitchEnabled,
		},
		cachepolicy.PolicyPlan: {
			TTL:          cacheConfig.TTL.Plan,
			Singleflight: cachepolicy.PolicySwitchEnabled,
		},
		cachepolicy.PolicyStatsQuery: {
			Singleflight: cachepolicy.PolicySwitchDisabled,
		},
	})
}

func firstPositiveDuration(values ...time.Duration) time.Duration {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func firstPositiveFloat(values ...float64) float64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func resolvePolicySwitch(explicit *bool, defaultValue bool) cachepolicy.PolicySwitch {
	if explicit != nil {
		return cachepolicy.PolicySwitchFromBool(*explicit)
	}
	return cachepolicy.PolicySwitchFromBool(defaultValue)
}
