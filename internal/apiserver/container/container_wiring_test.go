package container

import (
	"testing"

	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	scaleApp "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/behavior/scale"
	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	appQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachebootstrap"
	actormod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/actor"
	iammod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/iam"
	ammod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/modelcatalog"
	platformmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/platform"
	statmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/statistics"
	surveymod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/survey"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	iaminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/pkg/event"
	redis "github.com/redis/go-redis/v9"
)

func TestContainerBuildActorModuleDepsUsesObjectCacheBuilderAndPolicy(t *testing.T) {
	t.Parallel()

	c := NewContainer(nil, nil, nil)
	c.cache = newTestCacheSubsystem(t, ContainerCacheOptions{
		TTL: ContainerCacheTTLOptions{Testee: 5},
	}, nil)

	wire := actormod.WireInput{
		RedisClient:   c.CacheClient(cacheplane.FamilyObject),
		CacheBuilder:  c.CacheBuilder(cacheplane.FamilyObject),
		TesteePolicy:  c.CachePolicy(cachepolicy.PolicyTestee),
		Observer:      c.cacheObserver(),
		TopicResolver: c.eventCatalog,
		MySQLLimiter:  c.backpressure.MySQL,
	}
	if wire.CacheBuilder != c.CacheBuilder(cacheplane.FamilyObject) {
		t.Fatalf("cache builder = %#v, want %#v", wire.CacheBuilder, c.CacheBuilder(cacheplane.FamilyObject))
	}
	if wire.TesteePolicy != c.CachePolicy(cachepolicy.PolicyTestee) {
		t.Fatalf("policy = %#v, want %#v", wire.TesteePolicy, c.CachePolicy(cachepolicy.PolicyTestee))
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
	c.cache = newTestCacheSubsystem(t, ContainerCacheOptions{
		TTL: ContainerCacheTTLOptions{Questionnaire: 7},
	}, nil)

	wire := surveymod.WireInput{
		EventPublisher:   c.eventPublisher,
		RankCacheBuilder: c.CacheBuilder(cacheplane.FamilyRank),
		IdentityService:  c.resolveIdentityService(),
	}
	if wire.EventPublisher != c.eventPublisher {
		t.Fatalf("event publisher = %#v, want %#v", wire.EventPublisher, c.eventPublisher)
	}
	if wire.RankCacheBuilder != c.CacheBuilder(cacheplane.FamilyRank) {
		t.Fatalf("rank cache builder = %#v, want %#v", wire.RankCacheBuilder, c.CacheBuilder(cacheplane.FamilyRank))
	}
	if wire.IdentityService != nil {
		t.Fatalf("identity service = %#v, want nil without IAM", wire.IdentityService)
	}
}

func TestContainerBuildScaleModuleDepsUsesSharedApplicationWiring(t *testing.T) {
	t.Parallel()

	c := NewContainer(nil, nil, nil)
	c.eventPublisher = event.NewNopEventPublisher()
	c.cache = newTestCacheSubsystem(t, ContainerCacheOptions{
		TTL: ContainerCacheTTLOptions{Scale: 7, ScaleList: 11},
	}, nil)

	wire := ammod.WireInput{
		EventPublisher:   c.eventPublisher,
		RankCacheBuilder: c.CacheBuilder(cacheplane.FamilyRank),
	}
	if wire.EventPublisher != c.eventPublisher {
		t.Fatalf("event publisher = %#v, want %#v", wire.EventPublisher, c.eventPublisher)
	}
	if wire.RankCacheBuilder != c.CacheBuilder(cacheplane.FamilyRank) {
		t.Fatalf("rank cache builder = %#v, want %#v", wire.RankCacheBuilder, c.CacheBuilder(cacheplane.FamilyRank))
	}
}

func TestAssessmentModelModuleRegistersAggregateAndLegacyNames(t *testing.T) {
	t.Parallel()

	c := NewContainer(nil, nil, nil)
	module := &AssessmentModelModule{
		Scale:       &ScaleModule{},
		Personality: &PersonalityModelModule{},
	}
	c.SetAssessmentModelModule(module)

	if c.ScaleModule != module.Scale || c.PersonalityModelModule != module.Personality {
		t.Fatalf("legacy field aliases not wired to assessment model module")
	}
	got := c.GetLoadedModules()
	if len(got) != 3 {
		t.Fatalf("GetLoadedModules() = %v, want 3 entries", got)
	}
	if got[0] != "modelcatalog" || got[1] != "scale" || got[2] != "personalitymodel" {
		t.Fatalf("GetLoadedModules() = %v, want [modelcatalog scale personalitymodel]", got)
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

	wire := statmod.WireInput{
		RedisClient:         c.redisCache,
		FallbackRedisClient: c.CacheClient(cacheplane.FamilyQuery),
		CacheBuilder:        c.CacheBuilder(cacheplane.FamilyQuery),
		QueryPolicy:         c.CachePolicy(cachepolicy.PolicyStatsQuery),
		LockManager:         c.CacheLockManager(),
		Observer:            c.cacheObserver(),
		MetaRedisClient:     c.CacheClient(cacheplane.FamilyMeta),
	}
	if wire.FallbackRedisClient != queryClient {
		t.Fatalf("redis client = %#v, want query cache %#v", wire.FallbackRedisClient, queryClient)
	}
	if wire.CacheBuilder != c.CacheBuilder(cacheplane.FamilyQuery) {
		t.Fatalf("cache builder = %#v, want %#v", wire.CacheBuilder, c.CacheBuilder(cacheplane.FamilyQuery))
	}
	if wire.QueryPolicy != c.CachePolicy(cachepolicy.PolicyStatsQuery) {
		t.Fatalf("policy = %#v, want %#v", wire.QueryPolicy, c.CachePolicy(cachepolicy.PolicyStatsQuery))
	}
	if wire.LockManager == nil {
		t.Fatalf("lock manager = %#v, want *redisadapter.Manager", wire.LockManager)
	}
	if wire.Observer != c.cacheObserver() {
		t.Fatalf("observer = %#v, want %#v", wire.Observer, c.cacheObserver())
	}

	c.cacheOptions.DisableStatisticsCache = true
	wire = statmod.WireInput{
		DisableStatisticsCache: true,
		FallbackRedisClient:    queryClient,
	}
	if !wire.DisableStatisticsCache {
		t.Fatal("DisableStatisticsCache = false, want true")
	}
}

func TestContainerBuildStatisticsModuleDepsHandlesNilContainer(t *testing.T) {
	t.Parallel()

	wire := statmod.WireInput{}
	if wire.RedisClient != nil || wire.FallbackRedisClient != nil {
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
	if bindings.WarmScale != nil || bindings.WarmQuestionnaire != nil || bindings.WarmScaleList != nil || bindings.WarmStatsSystem != nil {
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
	if bindings.WarmScale == nil || bindings.WarmQuestionnaire == nil || bindings.WarmScaleList == nil {
		t.Fatalf("static warm callbacks should be wired when static family is available: %#v", bindings)
	}
	if bindings.WarmStatsSystem == nil || bindings.WarmStatsQuestionnaire == nil || bindings.WarmStatsPlan == nil {
		t.Fatalf("statistics warm callbacks should be wired when query family is available: %#v", bindings)
	}

	c.cacheOptions.DisableStatisticsCache = true
	bindings = newCacheGovernanceAdapter(c).bindings()
	if bindings.WarmStatsSystem != nil || bindings.WarmStatsQuestionnaire != nil || bindings.WarmStatsPlan != nil {
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
	c.cache = newTestCacheSubsystem(t, ContainerCacheOptions{}, nil)
	c.cache.BindGovernance(cachebootstrap.GovernanceBindings{})
	c.CodesService = &codesServiceStub{}
	c.QRCodeObjectKeyPrefix = "rest-prefix"

	evaluationManagement := assessmentApp.NewManagementService(nil, nil, nil, nil)
	planCommand := planApp.NewCommandService(nil, nil, nil, nil, nil, nil)
	planQuery := planApp.NewQueryService(nil, nil, nil)
	questionnaireQuery := appQuestionnaire.NewQueryService(nil, nil, nil, nil)
	scaleQuery := scaleApp.NewQueryService(nil, nil, nil, nil, nil)
	categoryService := scaleApp.NewCategoryService()

	c.SurveyModule = &SurveyModule{
		Questionnaire: &QuestionnaireSubModule{QueryService: questionnaireQuery},
		AnswerSheet:   &AnswerSheetSubModule{},
	}
	c.ScaleModule = &ScaleModule{QueryService: scaleQuery, CategoryService: categoryService}
	c.ActorModule = &ActorModule{}
	c.EvaluationModule = &EvaluationModule{ManagementService: evaluationManagement}
	c.PlanModule = &PlanModule{CommandService: planCommand, QueryService: planQuery}
	c.StatisticsModule = &StatisticsModule{}

	deps := c.BuildRESTDeps(nil)
	if deps.RateLimit != nil {
		t.Fatalf("RateLimit = %#v, want nil passthrough before router defaulting", deps.RateLimit)
	}
	if deps.Survey.QuestionnaireQueryService != questionnaireQuery {
		t.Fatalf("survey query service not extracted correctly: %#v", deps.Survey)
	}
	if deps.Scale.QueryService != scaleQuery || deps.Scale.CategoryService != categoryService {
		t.Fatalf("scale application services not extracted correctly: %#v", deps.Scale)
	}
	if deps.Evaluation.ManagementService != evaluationManagement || deps.Plan.CommandService != planCommand || deps.Plan.QueryService != planQuery || !deps.Statistics.Enabled {
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
	scaleQuery := scaleApp.NewQueryService(nil, nil, nil, nil, nil)
	categoryService := scaleApp.NewCategoryService()
	c.SurveyModule = &SurveyModule{
		Questionnaire: &QuestionnaireSubModule{QueryService: questionnaireQuery},
	}
	c.ScaleModule = &ScaleModule{
		QueryService:    scaleQuery,
		CategoryService: categoryService,
	}

	deps := c.BuildGRPCDeps(nil)
	if deps.Server != nil {
		t.Fatalf("Server = %#v, want nil passthrough", deps.Server)
	}
	if deps.Survey.QuestionnaireQueryService != questionnaireQuery {
		t.Fatalf("Survey.QuestionnaireQueryService = %#v, want %#v", deps.Survey.QuestionnaireQueryService, questionnaireQuery)
	}
	if deps.Scale.QueryService != scaleQuery || deps.Scale.CategoryService != categoryService {
		t.Fatalf("scale deps not extracted correctly: %#v", deps.Scale)
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
