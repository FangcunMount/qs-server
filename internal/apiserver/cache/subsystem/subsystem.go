package cachebootstrap

import (
	"context"
	"sync"

	"github.com/FangcunMount/component-base/pkg/logger"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	cachegov "github.com/FangcunMount/qs-server/internal/apiserver/cache/governance"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/hotset"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/model"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/cachesignal"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/locklease/redisadapter"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/bootstrap"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
	redis "github.com/redis/go-redis/v9"
)

// GovernanceBindings 描述 warmup/governance 最终装配所需的业务回调。
type GovernanceBindings struct {
	ListPublishedScaleCodes         func(context.Context) ([]string, error)
	ListPublishedQuestionnaireCodes func(context.Context) ([]string, error)
	LookupScaleQuestionnaireCode    func(context.Context, string) (string, error)
	WarmScale                       func(context.Context, string) error
	WarmQuestionnaire               func(context.Context, string) error
	WarmPublishedTypologyModel      func(context.Context, string) error
	WarmStatsOverview               func(context.Context, int64, string) error
	WarmStatsSystem                 func(context.Context, int64) error
	WarmStatsQuestionnaire          func(context.Context, int64, string) error
	WarmStatsPlan                   func(context.Context, int64, uint64) error
}

// Subsystem 收口 apiserver cache 子系统运行时与治理装配。
type Subsystem struct {
	component   string
	cacheConfig CacheOptions

	statusRegistry *observability.FamilyStatusRegistry
	runtime        *redisruntime.Runtime
	handles        map[redisruntime.Family]*redisruntime.Handle
	effective      *sharedcache.Registry
	observer       *observability.ComponentObserver

	hotsetRecorder  cachetarget.HotsetRecorder
	hotsetInspector cachetarget.HotsetInspector
	lockManager     locklease.Manager

	warmupCoordinator cachegov.Coordinator
	statusService     cachegov.StatusService
	policyReloader    *cachegov.PolicyReloader
	notifier          *cachesignal.Notifier

	lifecycleMu sync.Mutex
	started     bool
	cancel      context.CancelFunc
}

// NewSubsystem 创建 cache 子系统组合根，并完成 runtime/policy/hotset/lock 的基础装配。
func NewSubsystem(component string, resolver redisruntime.Resolver, runtimeOptions *genericoptions.RedisRuntimeOptions, cacheConfig CacheOptions) *Subsystem {
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
	var runtime *redisruntime.Runtime
	var handles map[redisruntime.Family]*redisruntime.Handle
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
		observer:       observability.NewComponentObserver(component, statusRegistry),
	}
	s.effective = cachepolicy.NewEffectiveRegistry(newPolicyCatalog(cacheConfig))
	s.hotsetRecorder = cachehotset.NewRedisStoreWithObserver(
		s.Client(redisruntime.FamilyMeta),
		s.Builder(redisruntime.FamilyMeta),
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
		s.lockManager = redisadapter.NewManagerWithObservers(component, "lock_lease", s.Handle(redisruntime.FamilyLock), nil, s.observer)
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
		WarmPublishedTypologyModel:      bindings.WarmPublishedTypologyModel,
		WarmStatsOverview:               bindings.WarmStatsOverview,
		WarmStatsSystem:                 bindings.WarmStatsSystem,
		WarmStatsQuestionnaire:          bindings.WarmStatsQuestionnaire,
		WarmStatsPlan:                   bindings.WarmStatsPlan,
	})
	s.statusService = cachegov.NewStatusService(s.component, s.statusRegistry, s.hotsetInspector, s.warmupCoordinator, s.effective, s.policyReloader)
}

// BindPolicyReloader installs the process-owned configuration candidate loader.
// Constructors remain side-effect free; the loader is invoked only by an
// authenticated governance action.
func (s *Subsystem) BindPolicyReloader(loader cachegov.PolicyCandidateLoader) {
	if s == nil {
		return
	}
	s.policyReloader = cachegov.NewPolicyReloader(s.component, s.effective, loader)
	if s.statusService != nil {
		s.statusService = cachegov.NewStatusService(s.component, s.statusRegistry, s.hotsetInspector, s.warmupCoordinator, s.effective, s.policyReloader)
	}
}

func (s *Subsystem) PolicyReloader() *cachegov.PolicyReloader {
	if s == nil {
		return nil
	}
	return s.policyReloader
}

// BindSignalNotifier transfers signal-watcher ownership to the cache subsystem.
func (s *Subsystem) BindSignalNotifier(notifier *cachesignal.Notifier) {
	if s == nil {
		return
	}
	s.lifecycleMu.Lock()
	s.notifier = notifier
	s.lifecycleMu.Unlock()
}

func (s *Subsystem) SignalNotifier() *cachesignal.Notifier {
	if s == nil {
		return nil
	}
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	return s.notifier
}

// Start starts signal watching and startup warmup. It is safe to call repeatedly.
func (s *Subsystem) Start(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.lifecycleMu.Lock()
	if s.started {
		s.lifecycleMu.Unlock()
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.started = true
	s.cancel = cancel
	notifier := s.notifier
	coordinator := s.warmupCoordinator
	s.lifecycleMu.Unlock()

	if notifier != nil {
		cachegov.StartCacheSignalWatcher(
			runCtx,
			coordinator,
			notifier.QuestionnaireSignaler(),
			notifier.ScaleSignaler(),
			notifier.TypologyModelSignaler(),
		)
	}
	if coordinator != nil {
		go func() {
			if err := coordinator.WarmStartup(runCtx); err != nil && runCtx.Err() == nil {
				logger.L(runCtx).Warnw("Cache warmup failed", "error", err)
				return
			}
			if runCtx.Err() == nil {
				logger.L(runCtx).Infow("Cache warmup completed")
			}
		}()
	}
	return nil
}

// Close cancels all subsystem goroutines. It is safe to call repeatedly.
func (s *Subsystem) Close() error {
	if s == nil {
		return nil
	}
	s.lifecycleMu.Lock()
	cancel := s.cancel
	s.cancel = nil
	s.started = false
	s.lifecycleMu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
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

func (s *Subsystem) Runtime() *redisruntime.Runtime {
	if s == nil {
		return nil
	}
	return s.runtime
}

func (s *Subsystem) Handle(family redisruntime.Family) *redisruntime.Handle {
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

func (s *Subsystem) Client(family redisruntime.Family) redis.UniversalClient {
	handle := s.Handle(family)
	if handle == nil {
		return nil
	}
	return handle.Client
}

func (s *Subsystem) Builder(family redisruntime.Family) *keyspace.Builder {
	handle := s.Handle(family)
	if handle == nil {
		return nil
	}
	return handle.Builder
}

func (s *Subsystem) EffectiveRegistry() *sharedcache.Registry {
	if s == nil {
		return nil
	}
	return s.effective
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

func (s *Subsystem) WarmupCoordinator() statisticsApp.WarmupCoordinator {
	if s == nil {
		return nil
	}
	return s.warmupCoordinator
}

func (s *Subsystem) StatusService() statisticsApp.GovernanceStatusReader {
	if s == nil {
		return nil
	}
	return s.statusService
}

func (s *Subsystem) warnMetaCacheAvailability() {
	metaHandle := s.Handle(redisruntime.FamilyMeta)
	metaRedisCache := s.Client(redisruntime.FamilyMeta)
	if s.cacheConfig.Warmup.HotsetEnable && metaRedisCache == nil {
		logger.L(context.Background()).Warnw("meta_cache unavailable while hotset governance is enabled; hotset recording and hot-target warmup will degrade",
			"component", s.component,
			"family", string(redisruntime.FamilyMeta),
			"profile", handleProfile(metaHandle),
		)
	}
	if metaRedisCache == nil {
		logger.L(context.Background()).Warnw("meta_cache unavailable; version-token query caches will run uncached where required",
			"component", s.component,
			"family", string(redisruntime.FamilyMeta),
			"profile", handleProfile(metaHandle),
		)
	}
}

func handleProfile(handle *redisruntime.Handle) string {
	if handle == nil {
		return ""
	}
	return handle.Profile
}

func (s *Subsystem) warmupFamilies() map[cachemodel.Family]bool {
	families := map[cachemodel.Family]bool{}
	if staticHandle := s.Handle(redisruntime.FamilyStatic); staticHandle != nil {
		families[cachemodel.FamilyStatic] = staticHandle.AllowWarmup
	}
	if queryHandle := s.Handle(redisruntime.FamilyQuery); queryHandle != nil {
		families[cachemodel.FamilyQuery] = queryHandle.AllowWarmup
	}
	return families
}

func newPolicyCatalog(cacheConfig CacheOptions) *cachepolicy.PolicyCatalog {
	return cachepolicy.NewPolicyCatalog(cachepolicy.CachePolicy{
		Compress:    cachepolicy.PolicySwitchFromBool(cacheConfig.CompressPayload),
		JitterRatio: cacheConfig.TTLJitterRatio,
	}, map[cachemodel.Family]cachepolicy.CachePolicy{
		cachemodel.FamilyStatic: {
			Compress:     cachepolicy.PolicySwitchFromBoolPtr(cacheConfig.Static.Compress),
			Singleflight: resolvePolicySwitch(cacheConfig.Static.Singleflight, true),
			Negative:     resolvePolicySwitch(cacheConfig.Static.Negative, false),
			NegativeTTL:  cacheConfig.Static.NegativeTTL,
			JitterRatio:  cacheConfig.Static.TTLJitterRatio,
		},
		cachemodel.FamilyObject: {
			Compress:     cachepolicy.PolicySwitchFromBoolPtr(cacheConfig.Object.Compress),
			Singleflight: resolvePolicySwitch(cacheConfig.Object.Singleflight, true),
			Negative:     resolvePolicySwitch(cacheConfig.Object.Negative, false),
			NegativeTTL:  cacheConfig.Object.NegativeTTL,
			JitterRatio:  cacheConfig.Object.TTLJitterRatio,
		},
		cachemodel.FamilyQuery: {
			NegativeTTL:  cacheConfig.Query.NegativeTTL,
			Compress:     cachepolicy.PolicySwitchFromBoolPtr(cacheConfig.Query.Compress),
			Singleflight: resolvePolicySwitch(cacheConfig.Query.Singleflight, false),
			Negative:     resolvePolicySwitch(cacheConfig.Query.Negative, false),
			JitterRatio:  cacheConfig.Query.TTLJitterRatio,
		},
	}, cacheConfig.Capabilities)
}

// BuildEffectiveCapabilities resolves a complete immutable candidate using the
// same catalog path as production bootstrap.
func BuildEffectiveCapabilities(cacheConfig CacheOptions) []sharedcache.EffectiveCapability {
	registry := cachepolicy.NewEffectiveRegistry(newPolicyCatalog(cacheConfig))
	return registry.All()
}

func resolvePolicySwitch(explicit *bool, defaultValue bool) cachepolicy.PolicySwitch {
	if explicit != nil {
		return cachepolicy.PolicySwitchFromBool(*explicit)
	}
	return cachepolicy.PolicySwitchFromBool(defaultValue)
}
