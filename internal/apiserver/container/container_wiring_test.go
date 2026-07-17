package container

import (
	"testing"

	"github.com/FangcunMount/component-base/pkg/event"
	evaluationoperator "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/operator"
	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	appQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/catalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/subsystem"
	actormod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/actor"
	iammod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/iam"
	ammod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/modelcatalog"
	platformmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/platform"
	statmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/statistics"
	surveymod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/survey"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	iaminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	locksubsystem "github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease/subsystem"
	redis "github.com/redis/go-redis/v9"
)

func TestContainerBuildActorModuleDepsUsesObjectCacheBuilderAndPolicy(t *testing.T) {
	t.Parallel()

	c := NewContainer(nil, nil, nil)
	c.cache = newTestCacheSubsystem(t, ContainerCacheOptions{
		Capabilities: map[sharedcache.Capability]cachepolicy.Binding{
			cachepolicy.CapabilityActorTestee: {Enabled: true, Policy: sharedcache.Policy{TTL: 5}},
		},
	}, nil)
	provider := c.CachePolicyProvider()

	wire := actormod.WireInput{
		RedisClient:   c.CacheClient(redisruntime.FamilyObject),
		CacheBuilder:  c.CacheBuilder(redisruntime.FamilyObject),
		CachePolicies: provider,
		Observer:      c.cacheObserver(),
		MySQLLimiter:  c.backpressure.MySQL,
	}
	if wire.CacheBuilder != c.CacheBuilder(redisruntime.FamilyObject) {
		t.Fatalf("cache builder = %#v, want %#v", wire.CacheBuilder, c.CacheBuilder(redisruntime.FamilyObject))
	}
	if wire.CachePolicies != provider {
		t.Fatalf("policy provider = %#v, want %#v", wire.CachePolicies, provider)
	}
	if wire.ProfileLinkService != nil || wire.IdentityService != nil || wire.IAMClient != nil || wire.OperationAccountSvc != nil {
		t.Fatalf("unexpected IAM deps in actor wire input: %#v", wire)
	}
	if wire.Observer != c.cacheObserver() {
		t.Fatalf("observer = %#v, want %#v", wire.Observer, c.cacheObserver())
	}
}

func TestContainerBuildSurveyModuleDepsUsesSharedApplicationWiring(t *testing.T) {
	t.Parallel()

	c := NewContainer(nil, nil, nil)
	c.eventPublisher = event.NewNopEventPublisher()
	c.cache = newTestCacheSubsystem(t, ContainerCacheOptions{}, nil)

	wire := surveymod.WireInput{
		EventPublisher:  c.eventPublisher,
		IdentityService: c.resolveIdentityService(),
	}
	if wire.EventPublisher != c.eventPublisher {
		t.Fatalf("event publisher = %#v, want %#v", wire.EventPublisher, c.eventPublisher)
	}
	if wire.IdentityService != nil {
		t.Fatalf("identity service = %#v, want nil without IAM", wire.IdentityService)
	}
}

func TestContainerInitEventSubsystemRequiresInjectedSubsystem(t *testing.T) {
	t.Parallel()

	c := NewContainer(nil, nil, nil)
	if err := c.initEventSubsystem(); err == nil {
		t.Fatal("initEventSubsystem() error = nil, want missing subsystem failure")
	}
}

func TestContainerInitEventSubsystemUsesInjectedPublisher(t *testing.T) {
	t.Parallel()

	subsystem := &eventsubsystem.Subsystem{}
	c := NewContainerWithOptions(nil, nil, nil, ContainerOptions{EventSubsystem: subsystem})
	if err := c.initEventSubsystem(); err != nil {
		t.Fatalf("initEventSubsystem() error = %v", err)
	}
	if c.eventSubsystem != subsystem {
		t.Fatalf("eventSubsystem = %p, want %p", c.eventSubsystem, subsystem)
	}
	if c.eventPublisher != subsystem.Publisher() {
		t.Fatalf("eventPublisher = %#v, want injected subsystem publisher %#v", c.eventPublisher, subsystem.Publisher())
	}
}

func TestContainerBuildScaleModuleDepsUsesSharedApplicationWiring(t *testing.T) {
	t.Parallel()

	c := NewContainer(nil, nil, nil)
	c.eventPublisher = event.NewNopEventPublisher()
	c.cache = newTestCacheSubsystem(t, ContainerCacheOptions{}, nil)

	wire := ammod.WireInput{
		EventPublisher:   c.eventPublisher,
		RankCacheBuilder: c.CacheBuilder(redisruntime.FamilyRank),
	}
	if wire.EventPublisher != c.eventPublisher {
		t.Fatalf("event publisher = %#v, want %#v", wire.EventPublisher, c.eventPublisher)
	}
	if wire.RankCacheBuilder != c.CacheBuilder(redisruntime.FamilyRank) {
		t.Fatalf("rank cache builder = %#v, want %#v", wire.RankCacheBuilder, c.CacheBuilder(redisruntime.FamilyRank))
	}
}

func TestAssessmentModelModuleRegistersAggregateOnly(t *testing.T) {
	t.Parallel()

	c := NewContainer(nil, nil, nil)
	module := &AssessmentModelModule{
		HotRank: &ammod.HotRank{},
	}
	c.SetAssessmentModelModule(module)

	if c.AssessmentModelModule != module {
		t.Fatalf("assessment model module fields not wired")
	}
	got := c.GetLoadedModules()
	if len(got) != 1 {
		t.Fatalf("GetLoadedModules() = %v, want 1 entry", got)
	}
	if got[0] != "modelcatalog" {
		t.Fatalf("GetLoadedModules() = %v, want [modelcatalog]", got)
	}
}

func TestContainerBuildStatisticsModuleDepsSelectsQueryCacheAndLockManager(t *testing.T) {
	t.Parallel()

	queryClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	t.Cleanup(func() { _ = queryClient.Close() })

	c := NewContainer(nil, nil, nil)
	c.cache = newTestCacheSubsystem(t, ContainerCacheOptions{}, map[string]redis.UniversalClient{
		"query": queryClient,
	})
	c.locks = locksubsystem.New(locksubsystem.Options{Component: "apiserver"})

	wire := statmod.WireInput{
		FallbackRedisClient: c.CacheClient(redisruntime.FamilyQuery),
		CacheBuilder:        c.CacheBuilder(redisruntime.FamilyQuery),
		CachePolicies:       c.CachePolicyProvider(),
		LockManager:         c.LockManager(),
		Observer:            c.cacheObserver(),
		MetaRedisClient:     c.CacheClient(redisruntime.FamilyMeta),
	}
	if wire.FallbackRedisClient != queryClient {
		t.Fatalf("redis client = %#v, want query cache %#v", wire.FallbackRedisClient, queryClient)
	}
	if wire.CacheBuilder != c.CacheBuilder(redisruntime.FamilyQuery) {
		t.Fatalf("cache builder = %#v, want %#v", wire.CacheBuilder, c.CacheBuilder(redisruntime.FamilyQuery))
	}
	if wire.CachePolicies != c.CachePolicyProvider() {
		t.Fatalf("policy provider = %#v, want %#v", wire.CachePolicies, c.CachePolicyProvider())
	}
	if wire.LockManager == nil {
		t.Fatalf("lock manager = %#v, want *redisadapter.Manager", wire.LockManager)
	}
	if wire.Observer != c.cacheObserver() {
		t.Fatalf("observer = %#v, want %#v", wire.Observer, c.cacheObserver())
	}

}

func TestContainerBuildStatisticsModuleDepsHandlesNilContainer(t *testing.T) {
	t.Parallel()

	wire := statmod.WireInput{}
	if wire.FallbackRedisClient != nil {
		t.Fatalf("empty wire input should not preset redis clients: %#v", wire)
	}
}

func TestCacheGovernanceAdapterSelectsWarmupCallbacksByAvailableFamilies(t *testing.T) {
	t.Parallel()

	c := NewContainer(nil, nil, nil)
	c.cache = newTestCacheSubsystem(t, ContainerCacheOptions{}, nil)
	bindings := newCacheGovernanceAdapter(c).bindings()
	if bindings.ListPublishedScaleCodes == nil || bindings.ListPublishedQuestionnaireCodes == nil || bindings.LookupScaleQuestionnaireCode == nil {
		t.Fatalf("list/lookup callbacks should always be wired: %#v", bindings)
	}
	if bindings.WarmScale != nil || bindings.WarmQuestionnaire != nil || bindings.WarmStatsOverview != nil {
		t.Fatalf("warm callbacks = %#v, want nil without available cache clients", bindings)
	}

	staticClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	queryClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	t.Cleanup(func() {
		_ = staticClient.Close()
		_ = queryClient.Close()
	})
	c.cache = newTestCacheSubsystem(t, ContainerCacheOptions{}, map[string]redis.UniversalClient{
		"static": staticClient,
		"query":  queryClient,
	})
	bindings = newCacheGovernanceAdapter(c).bindings()
	if bindings.WarmScale == nil || bindings.WarmQuestionnaire == nil {
		t.Fatalf("model and questionnaire warm callbacks should be wired: %#v", bindings)
	}
	if bindings.WarmStatsOverview == nil {
		t.Fatalf("statistics warm callbacks should be wired when query family is available: %#v", bindings)
	}

	c.cache = newTestCacheSubsystem(t, ContainerCacheOptions{
		Capabilities: map[sharedcache.Capability]cachepolicy.Binding{
			cachepolicy.CapabilityStatisticsQuery: {Enabled: false},
		},
	}, map[string]redis.UniversalClient{"static": staticClient, "query": queryClient})
	bindings = newCacheGovernanceAdapter(c).bindings()
	if bindings.WarmStatsOverview != nil {
		t.Fatalf("statistics warm callbacks = %#v, want nil when statistics cache is disabled", bindings)
	}
}

func TestContainerInitCodesServiceDoesNotOverwriteExistingImplementation(t *testing.T) {
	t.Parallel()

	existing := &codesServiceStub{}
	c := NewContainer(nil, nil, nil)
	c.CodesService = existing

	platformmod.InstallFrom(c)

	if c.CodesService != existing {
		t.Fatalf("CodesService = %#v, want existing implementation %#v", c.CodesService, existing)
	}
}

func TestContainerBuildQRCodeServiceConfigUsesOSSOverrides(t *testing.T) {
	t.Parallel()

	config := platformmod.BuildQRCodeServiceConfig(
		&options.WeChatOptions{
			WeChatAppID: "wechat-app",
			PagePath:    "pages/task/index",
			AppID:       "appid",
			AppSecret:   "secret",
		},
		&options.OSSOptions{
			ObjectKeyPrefix: "custom-prefix",
			PublicBaseURL:   "https://cdn.example.com/qrcode/",
		},
	)

	if config == nil {
		t.Fatal("config = nil, want non-nil")
		return
	}
	if config.WeChatAppID != "wechat-app" || config.PagePath != "pages/task/index" {
		t.Fatalf("unexpected base config: %#v", config)
	}
	if config.ObjectKeyPrefix != "custom-prefix" {
		t.Fatalf("ObjectKeyPrefix = %q, want custom-prefix", config.ObjectKeyPrefix)
	}
	if config.PublicURLPrefix != "https://cdn.example.com/qrcode" {
		t.Fatalf("PublicURLPrefix = %q, want trimmed OSS base URL", config.PublicURLPrefix)
	}
}

func TestContainerInitQRCodeServiceSkipsWithoutGenerator(t *testing.T) {
	t.Parallel()

	c := NewContainer(nil, nil, nil)
	if err := c.InitQRCodeService(&options.WeChatOptions{
		AppID:     "appid",
		AppSecret: "secret",
		PagePath:  "pages/task/index",
	}, nil); err != nil {
		t.Fatalf("InitQRCodeService() error = %v, want nil", err)
	}
	if c.QRCodeService != nil {
		t.Fatalf("QRCodeService = %#v, want nil when generator is unavailable", c.QRCodeService)
	}
}

func TestContainerInitQRCodeServiceCreatesServiceWithDirectConfig(t *testing.T) {
	t.Parallel()

	c := NewContainer(nil, nil, nil)
	c.QRCodeGenerator = &qrCodeGeneratorStub{}

	if err := c.InitQRCodeService(&options.WeChatOptions{
		AppID:     "appid",
		AppSecret: "secret",
		PagePath:  "pages/task/index",
	}, nil); err != nil {
		t.Fatalf("InitQRCodeService() error = %v, want nil", err)
	}
	if c.QRCodeService == nil {
		t.Fatal("QRCodeService = nil, want initialized service")
	}
}

func TestContainerInitMiniProgramTaskNotificationServiceSkipsWithoutTemplateID(t *testing.T) {
	t.Parallel()

	c := NewContainer(nil, nil, nil)
	c.SubscribeSender = &subscribeSenderStub{}

	c.InitMiniProgramTaskNotificationService(&options.WeChatOptions{
		AppID:     "appid",
		AppSecret: "secret",
		PagePath:  "pages/task/index",
	})

	if c.MiniProgramTaskNotificationService != nil {
		t.Fatalf("MiniProgramTaskNotificationService = %#v, want nil when template id is missing", c.MiniProgramTaskNotificationService)
	}
}

func TestContainerBuildRESTDepsExposesRouterFacingDependencies(t *testing.T) {
	t.Parallel()

	c := NewContainer(nil, nil, nil)
	c.eventSubsystem = &eventsubsystem.Subsystem{}
	c.cache = newTestCacheSubsystem(t, ContainerCacheOptions{}, nil)
	c.cache.BindGovernance(cachebootstrap.GovernanceBindings{})
	c.CodesService = &codesServiceStub{}
	c.QRCodeObjectKeyPrefix = "rest-prefix"

	evaluationRecovery := evaluationoperator.NewRecoveryService(nil, nil, nil, nil)
	planCommand := planApp.NewCommandService(nil, nil, nil, nil, nil, nil)
	planQuery := planApp.NewQueryService(nil, nil, nil)
	questionnaireQuery := appQuestionnaire.NewQueryService(nil, nil, nil, nil)
	c.SurveyModule = &SurveyModule{
		Questionnaire: &QuestionnaireSubModule{QueryService: questionnaireQuery},
		AnswerSheet:   &AnswerSheetSubModule{},
	}
	c.ActorModule = &ActorModule{}
	c.EvaluationModule = &EvaluationModule{OperatorRecovery: evaluationRecovery}
	c.PlanModule = &PlanModule{CommandService: planCommand, QueryService: planQuery}
	c.StatisticsModule = &StatisticsModule{}

	deps := c.BuildRESTDeps(nil)
	if deps.RateLimit != nil {
		t.Fatalf("RateLimit = %#v, want nil passthrough before router defaulting", deps.RateLimit)
	}
	if deps.Survey.QuestionnaireQueryService != questionnaireQuery {
		t.Fatalf("survey query service not extracted correctly: %#v", deps.Survey)
	}
	if deps.Evaluation.OperatorRecoveryService == nil || deps.Plan.CommandService != planCommand || deps.Plan.QueryService != planQuery || !deps.Statistics.Enabled {
		t.Fatalf("evaluation/plan/statistics dependencies not extracted correctly")
	}
	if deps.CodesService != c.CodesService {
		t.Fatalf("CodesService = %#v, want %#v", deps.CodesService, c.CodesService)
	}
	if deps.GovernanceStatusService != c.CacheGovernanceStatusService() {
		t.Fatalf("GovernanceStatusService = %#v, want %#v", deps.GovernanceStatusService, c.CacheGovernanceStatusService())
	}
	if deps.EventStatusService == nil {
		t.Fatalf("EventStatusService = nil, want read-only event status service")
	}
	if deps.QRCodeObjectKeyPrefix != "rest-prefix" {
		t.Fatalf("QRCodeObjectKeyPrefix = %q, want rest-prefix", deps.QRCodeObjectKeyPrefix)
	}
}

func TestContainerBuildRESTDepsWiresStatisticsGovernanceDependencies(t *testing.T) {
	t.Parallel()

	c := NewContainer(nil, nil, nil)
	c.cache = newTestCacheSubsystem(t, ContainerCacheOptions{
		Warmup: cachebootstrap.WarmupOptions{
			Enable:        true,
			StartupStatic: true,
		},
	}, nil)
	c.StatisticsModule = &StatisticsModule{}

	if err := c.initWarmupCoordinator(); err != nil {
		t.Fatalf("initWarmupCoordinator() error = %v", err)
	}

	deps := c.BuildRESTDeps(nil)
	if !deps.Statistics.Enabled {
		t.Fatal("statistics deps disabled, want enabled")
	}
	if deps.Statistics.WarmupCoordinator == nil {
		t.Fatal("warmup coordinator = nil, want wired dependency")
	}
	if deps.Statistics.CacheGovernanceStatusService == nil {
		t.Fatal("cache governance status service = nil, want wired dependency")
	}
}

func TestContainerBuildGRPCDepsExposesTransportSpecificDependencies(t *testing.T) {
	t.Parallel()

	c := NewContainer(nil, nil, nil)
	c.cache = newTestCacheSubsystem(t, ContainerCacheOptions{}, nil)
	c.cache.BindGovernance(cachebootstrap.GovernanceBindings{})
	c.QRCodeService = &qrCodeServiceStub{}
	c.MiniProgramTaskNotificationService = &miniProgramTaskNotificationServiceStub{}
	authzSnapshot := &iaminfra.AuthzSnapshotLoader{}
	c.IAMModule = iammod.NewTestModule(iammod.TestModuleOptions{AuthzSnapshotLoader: authzSnapshot})

	questionnaireQuery := appQuestionnaire.NewQueryService(nil, nil, nil, nil)
	c.SurveyModule = &SurveyModule{
		Questionnaire: &QuestionnaireSubModule{QueryService: questionnaireQuery},
	}

	deps := c.BuildGRPCDeps(nil)
	if deps.Server != nil {
		t.Fatalf("Server = %#v, want nil passthrough", deps.Server)
	}
	if deps.Survey.QuestionnaireQueryService != questionnaireQuery {
		t.Fatalf("Survey.QuestionnaireQueryService = %#v, want %#v", deps.Survey.QuestionnaireQueryService, questionnaireQuery)
	}
	if deps.WarmupCoordinator != c.WarmupCoordinator() {
		t.Fatalf("WarmupCoordinator = %#v, want %#v", deps.WarmupCoordinator, c.WarmupCoordinator())
	}
	if deps.QRCodeService != c.QRCodeService || deps.MiniProgramTaskNotificationService != c.MiniProgramTaskNotificationService {
		t.Fatalf("shared app services not extracted correctly")
	}
	if deps.IAM.AuthzSnapshotLoader != authzSnapshot {
		t.Fatalf("AuthzSnapshotLoader = %#v, want %#v", deps.IAM.AuthzSnapshotLoader, authzSnapshot)
	}
}
