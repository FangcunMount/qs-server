package container

import (
	"context"
	"errors"
	"io"
	"reflect"
	"testing"

	codesapp "github.com/FangcunMount/qs-server/internal/apiserver/application/codes"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/cachepolicy"
	wechatPort "github.com/FangcunMount/qs-server/internal/apiserver/infra/wechatapi/port"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/internal/pkg/rediskey"
	"github.com/FangcunMount/qs-server/internal/pkg/redislock"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
	"github.com/FangcunMount/qs-server/pkg/event"
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

type fakeModule struct {
	info         assembler.ModuleInfo
	checkHealth  error
	cleanup      error
	checkCalls   int
	cleanupCalls int
}

func (*fakeModule) Initialize(...interface{}) error { return nil }

func (m *fakeModule) CheckHealth() error {
	m.checkCalls++
	return m.checkHealth
}

func (m *fakeModule) Cleanup() error {
	m.cleanupCalls++
	return m.cleanup
}

func (m *fakeModule) ModuleInfo() assembler.ModuleInfo { return m.info }

func TestContainerModulesAreInstanceScoped(t *testing.T) {
	t.Parallel()

	left := NewContainer(nil, nil, nil)
	right := NewContainer(nil, nil, nil)

	leftModule := &fakeModule{info: assembler.ModuleInfo{Name: "left"}}
	rightModule := &fakeModule{info: assembler.ModuleInfo{Name: "right"}}

	left.registerModule("left", leftModule)
	right.registerModule("right", rightModule)

	if err := left.checkModulesHealth(context.Background()); err != nil {
		t.Fatalf("left checkModulesHealth() error = %v", err)
	}
	if leftModule.checkCalls != 1 {
		t.Fatalf("left module check calls = %d, want 1", leftModule.checkCalls)
	}
	if rightModule.checkCalls != 0 {
		t.Fatalf("right module check calls = %d, want 0", rightModule.checkCalls)
	}

	if got := left.GetLoadedModules(); !reflect.DeepEqual(got, []string{"left"}) {
		t.Fatalf("left GetLoadedModules() = %v, want [left]", got)
	}
	if got := right.GetLoadedModules(); !reflect.DeepEqual(got, []string{"right"}) {
		t.Fatalf("right GetLoadedModules() = %v, want [right]", got)
	}
}

func TestContainerCleanupUsesRegisteredModules(t *testing.T) {
	t.Parallel()

	c := NewContainer(nil, nil, nil)
	first := &fakeModule{info: assembler.ModuleInfo{Name: "survey"}}
	second := &fakeModule{info: assembler.ModuleInfo{Name: "plan"}}
	c.registerModule("survey", first)
	c.registerModule("plan", second)
	c.initialized = true

	if err := c.Cleanup(); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}

	if first.cleanupCalls != 1 || second.cleanupCalls != 1 {
		t.Fatalf("cleanup calls = (%d, %d), want (1, 1)", first.cleanupCalls, second.cleanupCalls)
	}
	if c.initialized {
		t.Fatal("container initialized = true, want false")
	}
}

func TestContainerCheckModulesHealthReturnsModuleError(t *testing.T) {
	t.Parallel()

	c := NewContainer(nil, nil, nil)
	want := errors.New("boom")
	c.registerModule("broken", &fakeModule{
		info:        assembler.ModuleInfo{Name: "broken"},
		checkHealth: want,
	})

	if err := c.checkModulesHealth(context.Background()); err == nil || !errors.Is(err, want) {
		t.Fatalf("checkModulesHealth() error = %v, want wrapped %v", err, want)
	}
}

func TestContainerBuildActorModuleInitializeParamsUsesObjectCacheBuilderAndPolicy(t *testing.T) {
	t.Parallel()

	c := NewContainer(nil, nil, nil)
	builder := rediskey.NewBuilderWithNamespace("actor")
	policy := cachepolicy.CachePolicy{TTL: 5}
	c.objectRedisHandle = &redisplane.Handle{Builder: builder}
	c.policyCatalog = cachepolicy.NewPolicyCatalog(nil, map[cachepolicy.CachePolicyKey]cachepolicy.CachePolicy{
		cachepolicy.PolicyTestee: policy,
	})

	params := c.buildActorModuleInitializeParams()
	if len(params) != 8 {
		t.Fatalf("len(params) = %d, want 8", len(params))
	}
	if params[4] != builder {
		t.Fatalf("cache builder = %#v, want %#v", params[4], builder)
	}
	gotPolicy, ok := params[5].(cachepolicy.CachePolicy)
	if !ok {
		t.Fatalf("policy arg type = %T, want cachepolicy.CachePolicy", params[5])
	}
	if gotPolicy != policy {
		t.Fatalf("policy = %#v, want %#v", gotPolicy, policy)
	}
	if !isNilInterfaceValue(params[1]) || !isNilInterfaceValue(params[2]) || !isNilInterfaceValue(params[6]) || !isNilInterfaceValue(params[7]) {
		t.Fatalf("unexpected IAM deps in params: %#v", params)
	}
}

func TestContainerBuildSurveyModuleInitializeParamsUsesStaticCacheBuilderAndPolicy(t *testing.T) {
	t.Parallel()

	c := NewContainer(nil, nil, nil)
	builder := rediskey.NewBuilderWithNamespace("survey")
	policy := cachepolicy.CachePolicy{TTL: 7}
	c.eventPublisher = event.NewNopEventPublisher()
	c.staticRedisHandle = &redisplane.Handle{Builder: builder}
	c.policyCatalog = cachepolicy.NewPolicyCatalog(nil, map[cachepolicy.CachePolicyKey]cachepolicy.CachePolicy{
		cachepolicy.PolicyQuestionnaire: policy,
	})

	params := c.buildSurveyModuleInitializeParams()
	if len(params) != 7 {
		t.Fatalf("len(params) = %d, want 7", len(params))
	}
	if params[1] != c.eventPublisher {
		t.Fatalf("event publisher = %#v, want %#v", params[1], c.eventPublisher)
	}
	if params[3] != builder {
		t.Fatalf("cache builder = %#v, want %#v", params[3], builder)
	}
	gotPolicy, ok := params[5].(cachepolicy.CachePolicy)
	if !ok {
		t.Fatalf("policy arg type = %T, want cachepolicy.CachePolicy", params[5])
	}
	if gotPolicy != policy {
		t.Fatalf("policy = %#v, want %#v", gotPolicy, policy)
	}
	if !isNilInterfaceValue(params[4]) {
		t.Fatalf("identity service = %#v, want nil without IAM", params[4])
	}
}

func TestContainerBuildStatisticsModuleInitializeParamsSelectsQueryCacheAndLockManager(t *testing.T) {
	t.Parallel()

	queryClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	t.Cleanup(func() { _ = queryClient.Close() })

	lockBuilder := rediskey.NewBuilderWithNamespace("lock")
	queryBuilder := rediskey.NewBuilderWithNamespace("query")
	policy := cachepolicy.CachePolicy{TTL: 11}

	c := NewContainer(nil, nil, nil)
	c.queryRedisCache = queryClient
	c.queryRedisHandle = &redisplane.Handle{Builder: queryBuilder}
	c.lockRedisHandle = &redisplane.Handle{Builder: lockBuilder}
	c.policyCatalog = cachepolicy.NewPolicyCatalog(nil, map[cachepolicy.CachePolicyKey]cachepolicy.CachePolicy{
		cachepolicy.PolicyStatsQuery: policy,
	})

	params := c.buildStatisticsModuleInitializeParams()
	if len(params) != 8 {
		t.Fatalf("len(params) = %d, want 8", len(params))
	}
	if params[1] != queryClient {
		t.Fatalf("redis client = %#v, want query cache %#v", params[1], queryClient)
	}
	if params[2] != queryBuilder {
		t.Fatalf("cache builder = %#v, want %#v", params[2], queryBuilder)
	}
	gotPolicy, ok := params[5].(cachepolicy.CachePolicy)
	if !ok {
		t.Fatalf("policy arg type = %T, want cachepolicy.CachePolicy", params[5])
	}
	if gotPolicy != policy {
		t.Fatalf("policy = %#v, want %#v", gotPolicy, policy)
	}
	lockManager, ok := params[7].(*redislock.Manager)
	if !ok || lockManager == nil {
		t.Fatalf("lock manager = %#v, want *redislock.Manager", params[7])
	}

	c.cacheOptions.DisableStatisticsCache = true
	params = c.buildStatisticsModuleInitializeParams()
	if !isNilInterfaceValue(params[1]) {
		t.Fatalf("redis client with disabled statistics cache = %#v, want nil", params[1])
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

func TestContainerGetContainerInfoReflectsModulesAndInfrastructure(t *testing.T) {
	t.Parallel()

	redisClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	t.Cleanup(func() { _ = redisClient.Close() })

	c := NewContainer(&gorm.DB{}, &mongo.Database{}, redisClient)
	c.registerModule("survey", &fakeModule{info: assembler.ModuleInfo{Name: "survey", Version: "1.0.0"}})
	c.registerModule("plan", &fakeModule{info: assembler.ModuleInfo{Name: "plan", Version: "1.0.0"}})
	c.initialized = true

	info := c.GetContainerInfo()
	if got := info["initialized"]; got != true {
		t.Fatalf("initialized = %#v, want true", got)
	}
	infra, ok := info["infrastructure"].(map[string]bool)
	if !ok {
		t.Fatalf("infrastructure type = %T, want map[string]bool", info["infrastructure"])
	}
	if !infra["mysql"] || !infra["mongodb"] || !infra["redis"] {
		t.Fatalf("infrastructure = %#v, want all backends present", infra)
	}
	modules, ok := info["modules"].(map[string]interface{})
	if !ok {
		t.Fatalf("modules type = %T, want map[string]interface{}", info["modules"])
	}
	if _, exists := modules["survey"]; !exists {
		t.Fatalf("modules = %#v, want survey entry", modules)
	}
	if _, exists := modules["plan"]; !exists {
		t.Fatalf("modules = %#v, want plan entry", modules)
	}
}

func isNilInterfaceValue(value interface{}) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

type codesServiceStub struct{}

func (*codesServiceStub) Apply(context.Context, string, int, string, map[string]interface{}) ([]string, error) {
	return nil, nil
}

type qrCodeGeneratorStub struct{}

func (*qrCodeGeneratorStub) GenerateQRCode(context.Context, string, string, string, int) (io.Reader, error) {
	return nil, nil
}

func (*qrCodeGeneratorStub) GenerateUnlimitedQRCode(context.Context, string, string, string, string, int, bool, map[string]int, bool) (io.Reader, error) {
	return nil, nil
}

type subscribeSenderStub struct{}

func (*subscribeSenderStub) SendSubscribeMessage(context.Context, string, string, wechatPort.SubscribeMessage) error {
	return nil
}

func (*subscribeSenderStub) ListTemplates(context.Context, string, string) ([]wechatPort.SubscribeTemplate, error) {
	return nil, nil
}

var _ codesapp.CodesService = (*codesServiceStub)(nil)
var _ wechatPort.QRCodeGenerator = (*qrCodeGeneratorStub)(nil)
var _ wechatPort.MiniProgramSubscribeSender = (*subscribeSenderStub)(nil)
