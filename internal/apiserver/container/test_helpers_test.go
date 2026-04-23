package container

import (
	"context"
	"io"
	"reflect"
	"testing"

	cbdatabase "github.com/FangcunMount/component-base/pkg/database"
	codesapp "github.com/FangcunMount/qs-server/internal/apiserver/application/codes"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachebootstrap"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/assembler"
	wechatPort "github.com/FangcunMount/qs-server/internal/apiserver/infra/wechatapi/port"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	redis "github.com/redis/go-redis/v9"
)

type fakeModule struct {
	info         assembler.ModuleInfo
	checkHealth  error
	cleanup      error
	checkCalls   int
	cleanupCalls int
}

type fakeRedisResolver struct {
	defaultClient redis.UniversalClient
	profiles      map[string]redis.UniversalClient
}

func (r fakeRedisResolver) GetRedisClient() (redis.UniversalClient, error) {
	return r.defaultClient, nil
}

func (r fakeRedisResolver) GetRedisClientByProfile(profile string) (redis.UniversalClient, error) {
	if client, ok := r.profiles[profile]; ok {
		return client, nil
	}
	return nil, nil
}

func (r fakeRedisResolver) GetRedisProfileStatus(profile string) cbdatabase.RedisProfileStatus {
	if _, ok := r.profiles[profile]; ok {
		return cbdatabase.RedisProfileStatus{State: cbdatabase.RedisProfileStateAvailable}
	}
	return cbdatabase.RedisProfileStatus{State: cbdatabase.RedisProfileStateMissing}
}

func newTestCacheSubsystem(t *testing.T, opts ContainerCacheOptions, profileClients map[string]redis.UniversalClient) *cachebootstrap.Subsystem {
	t.Helper()

	runtimeOpts := &genericoptions.RedisRuntimeOptions{
		Namespace: "test",
		Families: map[string]*genericoptions.RedisRuntimeFamilyRoute{
			"static_meta":  {RedisProfile: "static", NamespaceSuffix: "static"},
			"object_view":  {RedisProfile: "object", NamespaceSuffix: "object"},
			"query_result": {RedisProfile: "query", NamespaceSuffix: "query"},
			"meta_hotset":  {RedisProfile: "meta", NamespaceSuffix: "meta"},
			"sdk_token":    {RedisProfile: "sdk", NamespaceSuffix: "sdk"},
			"lock_lease":   {RedisProfile: "lock", NamespaceSuffix: "lock"},
		},
	}

	return cachebootstrap.NewSubsystem("apiserver", fakeRedisResolver{profiles: profileClients}, runtimeOpts, opts)
}

func (m *fakeModule) CheckHealth() error {
	m.checkCalls++
	return m.checkHealth
}

func (m *fakeModule) Cleanup() error {
	m.cleanupCalls++
	return m.cleanup
}

func (m *fakeModule) ModuleInfo() assembler.ModuleInfo { return m.info }

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

var _ assembler.Module = (*fakeModule)(nil)
var _ codesapp.CodesService = (*codesServiceStub)(nil)
var _ wechatPort.QRCodeGenerator = (*qrCodeGeneratorStub)(nil)
var _ wechatPort.MiniProgramSubscribeSender = (*subscribeSenderStub)(nil)
