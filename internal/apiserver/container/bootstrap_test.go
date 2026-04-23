package container

import (
	"context"
	"testing"

	scaleApp "github.com/FangcunMount/qs-server/internal/apiserver/application/scale"
	appQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachebootstrap"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	iaminfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	handlerpkg "github.com/FangcunMount/qs-server/internal/apiserver/interface/restful/handler"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
	"github.com/FangcunMount/qs-server/pkg/event"
	redis "github.com/redis/go-redis/v9"
)

func TestContainerBuildActorModuleDepsUsesObjectCacheBuilderAndPolicy(t *testing.T) {
	t.Parallel()

	c := NewContainer(nil, nil, nil)
	c.cache = newTestCacheSubsystem(t, ContainerCacheOptions{
		TTL: ContainerCacheTTLOptions{Testee: 5},
	}, nil)

	deps := c.buildActorModuleDeps()
	if deps.CacheBuilder != c.CacheBuilder(redisplane.FamilyObject) {
		t.Fatalf("cache builder = %#v, want %#v", deps.CacheBuilder, c.CacheBuilder(redisplane.FamilyObject))
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

func TestContainerBuildSurveyModuleDepsUsesStaticCacheBuilderAndPolicy(t *testing.T) {
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
	if deps.CacheBuilder != c.CacheBuilder(redisplane.FamilyStatic) {
		t.Fatalf("cache builder = %#v, want %#v", deps.CacheBuilder, c.CacheBuilder(redisplane.FamilyStatic))
	}
	if deps.QuestionnairePolicy != c.CachePolicy(cachepolicy.PolicyQuestionnaire) {
		t.Fatalf("policy = %#v, want %#v", deps.QuestionnairePolicy, c.CachePolicy(cachepolicy.PolicyQuestionnaire))
	}
	if deps.IdentityService != nil {
		t.Fatalf("identity service = %#v, want nil without IAM", deps.IdentityService)
	}
	if deps.Observer != c.cacheObserver() {
		t.Fatalf("observer = %#v, want %#v", deps.Observer, c.cacheObserver())
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
	if deps.CacheBuilder != c.CacheBuilder(redisplane.FamilyQuery) {
		t.Fatalf("cache builder = %#v, want %#v", deps.CacheBuilder, c.CacheBuilder(redisplane.FamilyQuery))
	}
	if deps.QueryPolicy != c.CachePolicy(cachepolicy.PolicyStatsQuery) {
		t.Fatalf("policy = %#v, want %#v", deps.QueryPolicy, c.CachePolicy(cachepolicy.PolicyStatsQuery))
	}
	if deps.LockManager == nil {
		t.Fatalf("lock manager = %#v, want *redislock.Manager", deps.LockManager)
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

	questionnaireHandler := handlerpkg.NewQuestionnaireHandler(nil, nil, nil, nil)
	answerSheetHandler := handlerpkg.NewAnswerSheetHandler(nil, nil)
	scaleHandler := handlerpkg.NewScaleHandler(nil, nil, nil, nil, nil)
	testeeHandler := handlerpkg.NewTesteeHandler(nil, nil, nil, nil, nil, nil, nil, nil)
	operatorClinicianHandler := handlerpkg.NewOperatorClinicianHandler(nil, nil, nil, nil, nil, nil, nil, nil)
	assessmentEntryHandler := handlerpkg.NewAssessmentEntryHandler(nil, nil, nil, nil)
	evaluationHandler := handlerpkg.NewEvaluationHandler(nil, nil, nil, nil)
	planHandler := handlerpkg.NewPlanHandler(nil, nil)
	statisticsHandler := handlerpkg.NewStatisticsHandler(nil, nil, nil, nil, nil, nil, nil)

	c.SurveyModule = &assembler.SurveyModule{
		Questionnaire: &assembler.QuestionnaireSubModule{Handler: questionnaireHandler},
		AnswerSheet:   &assembler.AnswerSheetSubModule{Handler: answerSheetHandler},
	}
	c.ScaleModule = &assembler.ScaleModule{Handler: scaleHandler}
	c.ActorModule = &assembler.ActorModule{
		TesteeHandler:            testeeHandler,
		OperatorClinicianHandler: operatorClinicianHandler,
		AssessmentEntryHandler:   assessmentEntryHandler,
	}
	c.EvaluationModule = &assembler.EvaluationModule{Handler: evaluationHandler}
	c.PlanModule = &assembler.PlanModule{Handler: planHandler}
	c.StatisticsModule = &assembler.StatisticsModule{Handler: statisticsHandler}

	deps := c.BuildRESTDeps(nil)
	if deps.RateLimit != nil {
		t.Fatalf("RateLimit = %#v, want nil passthrough before router defaulting", deps.RateLimit)
	}
	if deps.Survey.QuestionnaireHandler != questionnaireHandler || deps.Survey.AnswerSheetHandler != answerSheetHandler {
		t.Fatalf("survey handlers not extracted correctly: %#v", deps.Survey)
	}
	if deps.Scale.Handler != scaleHandler || deps.Actor.TesteeHandler != testeeHandler || deps.Actor.OperatorClinicianHandler != operatorClinicianHandler || deps.Actor.AssessmentEntryHandler != assessmentEntryHandler {
		t.Fatalf("actor/scale handlers not extracted correctly: %#v %#v", deps.Scale, deps.Actor)
	}
	if deps.Evaluation.Handler != evaluationHandler || deps.Plan.Handler != planHandler || deps.Statistics.Handler != statisticsHandler {
		t.Fatalf("evaluation/plan/statistics handlers not extracted correctly")
	}
	if deps.CodesService != c.CodesService {
		t.Fatalf("CodesService = %#v, want %#v", deps.CodesService, c.CodesService)
	}
	if deps.GovernanceStatusService != c.CacheGovernanceStatusService() {
		t.Fatalf("GovernanceStatusService = %#v, want %#v", deps.GovernanceStatusService, c.CacheGovernanceStatusService())
	}
	if deps.QRCodeObjectKeyPrefix != "rest-prefix" {
		t.Fatalf("QRCodeObjectKeyPrefix = %q, want rest-prefix", deps.QRCodeObjectKeyPrefix)
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

	questionnaireQuery := appQuestionnaire.NewQueryService(nil, nil, nil)
	scaleQuery := scaleApp.NewQueryService(nil, nil, nil, nil)
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
	if deps.Scale.QuestionnaireQueryService != questionnaireQuery {
		t.Fatalf("Scale.QuestionnaireQueryService = %#v, want %#v", deps.Scale.QuestionnaireQueryService, questionnaireQuery)
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
