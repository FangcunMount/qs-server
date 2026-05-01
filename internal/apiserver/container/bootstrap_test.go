package container

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	assessmentApp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/assessment"
	scaleApp "github.com/FangcunMount/qs-server/internal/apiserver/application/scale"
	appQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachebootstrap"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	iaminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	handlerpkg "github.com/FangcunMount/qs-server/internal/apiserver/transport/rest/handler"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/pkg/event"
	"github.com/gin-gonic/gin"
	redis "github.com/redis/go-redis/v9"
)

func TestContainerBuildActorModuleDepsUsesObjectCacheBuilderAndPolicy(t *testing.T) {
	t.Parallel()

	c := NewContainer(nil, nil, nil)
	c.cache = newTestCacheSubsystem(t, ContainerCacheOptions{
		TTL: ContainerCacheTTLOptions{Testee: 5},
	}, nil)

	deps := c.buildActorModuleDeps()
	if deps.CacheBuilder != c.CacheBuilder(cacheplane.FamilyObject) {
		t.Fatalf("cache builder = %#v, want %#v", deps.CacheBuilder, c.CacheBuilder(cacheplane.FamilyObject))
	}
	if deps.TesteePolicy != c.CachePolicy(cachepolicy.PolicyTestee) {
		t.Fatalf("policy = %#v, want %#v", deps.TesteePolicy, c.CachePolicy(cachepolicy.PolicyTestee))
	}
	if deps.GuardianshipService != nil || deps.IdentityService != nil || deps.OperatorAuthz != nil || deps.OperationAccountSvc != nil {
		t.Fatalf("unexpected IAM deps in actor deps: %#v", deps)
	}
	if deps.Observer != c.cacheObserver() {
		t.Fatalf("observer = %#v, want %#v", deps.Observer, c.cacheObserver())
	}
}

func TestContainerBuildSurveyModuleDepsUsesSharedApplicationWiring(t *testing.T) {
	t.Parallel()

	c := NewContainer(nil, nil, nil)
	c.eventPublisher = event.NewNopEventPublisher()
	c.cache = newTestCacheSubsystem(t, ContainerCacheOptions{
		TTL: ContainerCacheTTLOptions{Questionnaire: 7},
	}, nil)

	deps := c.buildSurveyModuleDeps()
	if deps.EventPublisher != c.eventPublisher {
		t.Fatalf("event publisher = %#v, want %#v", deps.EventPublisher, c.eventPublisher)
	}
	if deps.RankCacheBuilder != c.CacheBuilder(cacheplane.FamilyRank) {
		t.Fatalf("rank cache builder = %#v, want %#v", deps.RankCacheBuilder, c.CacheBuilder(cacheplane.FamilyRank))
	}
	if deps.IdentityService != nil {
		t.Fatalf("identity service = %#v, want nil without IAM", deps.IdentityService)
	}
	if deps.ScaleSyncer == nil {
		t.Fatal("scale syncer = nil, want explicit syncer")
	}
}

func TestContainerBuildScaleModuleDepsUsesSharedApplicationWiring(t *testing.T) {
	t.Parallel()

	c := NewContainer(nil, nil, nil)
	c.eventPublisher = event.NewNopEventPublisher()
	c.cache = newTestCacheSubsystem(t, ContainerCacheOptions{
		TTL: ContainerCacheTTLOptions{Scale: 7, ScaleList: 11},
	}, nil)

	deps := c.buildScaleModuleDeps()
	if deps.EventPublisher != c.eventPublisher {
		t.Fatalf("event publisher = %#v, want %#v", deps.EventPublisher, c.eventPublisher)
	}
	if deps.RankCacheBuilder != c.CacheBuilder(cacheplane.FamilyRank) {
		t.Fatalf("rank cache builder = %#v, want %#v", deps.RankCacheBuilder, c.CacheBuilder(cacheplane.FamilyRank))
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

	deps := c.buildStatisticsModuleDeps()
	if deps.RedisClient != queryClient {
		t.Fatalf("redis client = %#v, want query cache %#v", deps.RedisClient, queryClient)
	}
	if deps.CacheBuilder != c.CacheBuilder(cacheplane.FamilyQuery) {
		t.Fatalf("cache builder = %#v, want %#v", deps.CacheBuilder, c.CacheBuilder(cacheplane.FamilyQuery))
	}
	if deps.QueryPolicy != c.CachePolicy(cachepolicy.PolicyStatsQuery) {
		t.Fatalf("policy = %#v, want %#v", deps.QueryPolicy, c.CachePolicy(cachepolicy.PolicyStatsQuery))
	}
	if deps.LockManager == nil {
		t.Fatalf("lock manager = %#v, want *redisadapter.Manager", deps.LockManager)
	}
	if _, ok := interface{}(deps.VersionStore).(interface {
		Current(context.Context, string) (uint64, error)
	}); !ok {
		t.Fatalf("version store = %#v, want VersionTokenStore", deps.VersionStore)
	}
	if deps.Observer != c.cacheObserver() {
		t.Fatalf("observer = %#v, want %#v", deps.Observer, c.cacheObserver())
	}

	c.cacheOptions.DisableStatisticsCache = true
	deps = c.buildStatisticsModuleDeps()
	if !isNilInterfaceValue(deps.RedisClient) {
		t.Fatalf("redis client with disabled statistics cache = %#v, want nil", deps.RedisClient)
	}
}

func TestContainerBuildStatisticsModuleDepsHandlesNilContainer(t *testing.T) {
	t.Parallel()

	var c *Container
	deps := c.buildStatisticsModuleDeps()
	if deps.VersionStore == nil {
		t.Fatal("version store = nil, want fallback static store")
	}
	if !isNilInterfaceValue(deps.RedisClient) {
		t.Fatalf("redis client = %#v, want nil for nil container", deps.RedisClient)
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

	c.initCodesService()

	if c.CodesService != existing {
		t.Fatalf("CodesService = %#v, want existing implementation %#v", c.CodesService, existing)
	}
}

func TestContainerBuildQRCodeServiceConfigUsesOSSOverrides(t *testing.T) {
	t.Parallel()

	config := (&Container{}).buildQRCodeServiceConfig(
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

	evaluationManagement := assessmentApp.NewManagementService(nil, nil)
	planHandler := handlerpkg.NewPlanHandler(nil, nil)
	statisticsHandler := handlerpkg.NewStatisticsHandler(nil, nil, nil, nil, nil, nil, nil)
	questionnaireQuery := appQuestionnaire.NewQueryService(nil, nil, nil, nil)
	scaleQuery := scaleApp.NewQueryService(nil, nil, nil, nil, nil)
	categoryService := scaleApp.NewCategoryService()

	c.SurveyModule = &assembler.SurveyModule{
		Questionnaire: &assembler.QuestionnaireSubModule{QueryService: questionnaireQuery},
		AnswerSheet:   &assembler.AnswerSheetSubModule{},
	}
	c.ScaleModule = &assembler.ScaleModule{QueryService: scaleQuery, CategoryService: categoryService}
	c.ActorModule = &assembler.ActorModule{}
	c.EvaluationModule = &assembler.EvaluationModule{ManagementService: evaluationManagement}
	c.PlanModule = &assembler.PlanModule{Handler: planHandler}
	c.StatisticsModule = &assembler.StatisticsModule{Handler: statisticsHandler}

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
	if deps.Evaluation.ManagementService != evaluationManagement || deps.Plan.Handler != planHandler || deps.Statistics.Handler != statisticsHandler {
		t.Fatalf("evaluation/plan/statistics handlers not extracted correctly")
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

func TestContainerInitWarmupCoordinatorRebindsStatisticsHandlerGovernance(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	c := NewContainer(nil, nil, nil)
	c.cache = newTestCacheSubsystem(t, ContainerCacheOptions{
		Warmup: cachebootstrap.WarmupOptions{
			Enable:        true,
			StartupStatic: true,
		},
	}, nil)
	statisticsHandler := handlerpkg.NewStatisticsHandler(nil, nil, nil, nil, nil, nil, nil)
	c.StatisticsModule = &assembler.StatisticsModule{Handler: statisticsHandler}

	if err := c.initWarmupCoordinator(); err != nil {
		t.Fatalf("initWarmupCoordinator() error = %v", err)
	}
	newModuleGraph(c).postWireCacheGovernanceDependencies()

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/internal/v1/cache/governance/status", nil)
	statisticsHandler.CacheGovernanceStatus(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var payload struct {
		Code int `json:"code"`
		Data struct {
			Summary struct {
				FamilyTotal int `json:"family_total"`
			} `json:"summary"`
			Families []struct {
				Family string `json:"family"`
			} `json:"families"`
			Warmup struct {
				Enabled bool `json:"enabled"`
			} `json:"warmup"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload.Code != 0 {
		t.Fatalf("code = %d, want 0", payload.Code)
	}
	if payload.Data.Summary.FamilyTotal == 0 || len(payload.Data.Families) == 0 {
		t.Fatalf("cache governance status was not rebound: summary=%+v families=%+v", payload.Data.Summary, payload.Data.Families)
	}
	if !payload.Data.Warmup.Enabled {
		t.Fatal("warmup.enabled = false, want true")
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
	c.IAMModule = &IAMModule{authzSnapshotLoader: authzSnapshot}

	questionnaireQuery := appQuestionnaire.NewQueryService(nil, nil, nil, nil)
	scaleQuery := scaleApp.NewQueryService(nil, nil, nil, nil, nil)
	categoryService := scaleApp.NewCategoryService()
	c.SurveyModule = &assembler.SurveyModule{
		Questionnaire: &assembler.QuestionnaireSubModule{QueryService: questionnaireQuery},
	}
	c.ScaleModule = &assembler.ScaleModule{
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
