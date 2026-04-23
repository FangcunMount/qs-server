package container

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
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
