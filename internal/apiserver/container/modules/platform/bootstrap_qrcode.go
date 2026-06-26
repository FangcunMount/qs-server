package platform

import (
	"fmt"
	"strings"

	qrcodeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	qrcodeObjectStorage "github.com/FangcunMount/qs-server/internal/apiserver/infra/objectstorage"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/objectstorage/aliyunoss"
	objectstorageport "github.com/FangcunMount/qs-server/internal/apiserver/infra/objectstorage/port"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/wechatapi"
	wechatmini "github.com/FangcunMount/qs-server/internal/apiserver/port/wechatmini"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
	redis "github.com/redis/go-redis/v9"
	"github.com/silenceper/wechat/v2/cache"
)

// QRCodeGeneratorResult holds WeChat mini-program infrastructure services.
type QRCodeGeneratorResult struct {
	Generator       wechatmini.QRCodeGenerator
	SubscribeSender wechatmini.MiniProgramSubscribeSender
}

// QRCodeCacheAccess resolves cache-plane clients used by WeChat SDK adapters.
type QRCodeCacheAccess struct {
	SDKRedis   redis.UniversalClient
	SDKBuilder *keyspace.Builder
}

// NewQRCodeGeneratorInfra builds QR code generator and subscribe sender infrastructure.
func NewQRCodeGeneratorInfra(access QRCodeCacheAccess) QRCodeGeneratorResult {
	wechatCache := BuildWeChatSDKCache(access)
	return QRCodeGeneratorResult{
		Generator:       wechatapi.NewQRCodeGenerator(wechatCache),
		SubscribeSender: wechatapi.NewSubscribeSender(wechatCache),
	}
}

// BuildWeChatSDKCache returns the WeChat SDK cache adapter.
func BuildWeChatSDKCache(access QRCodeCacheAccess) cache.Cache {
	if access.SDKRedis != nil {
		return wechatapi.NewRedisCacheAdapterWithBuilder(access.SDKRedis, access.SDKBuilder)
	}
	return cache.NewMemory()
}

// QRCodeObjectStoreResult holds object-store wiring for QR assets.
type QRCodeObjectStoreResult struct {
	Store     objectstorageport.PublicObjectStore
	KeyPrefix string
}

// InitQRCodeObjectStore configures optional OSS-backed QR asset storage.
func InitQRCodeObjectStore(existing objectstorageport.PublicObjectStore, ossOptions *options.OSSOptions) (QRCodeObjectStoreResult, error) {
	if ossOptions == nil || !ossOptions.Enabled {
		return QRCodeObjectStoreResult{}, nil
	}
	if existing != nil {
		return QRCodeObjectStoreResult{Store: existing, KeyPrefix: ossOptions.ObjectKeyPrefix}, nil
	}
	store, err := aliyunoss.NewPublicObjectStore(ossOptions)
	if err != nil {
		return QRCodeObjectStoreResult{}, fmt.Errorf("initialize qrcode object store: %w", err)
	}
	return QRCodeObjectStoreResult{Store: store, KeyPrefix: ossOptions.ObjectKeyPrefix}, nil
}

// BuildQRCodeServiceConfig maps process options to application config.
func BuildQRCodeServiceConfig(wechatOptions *options.WeChatOptions, ossOptions *options.OSSOptions) *qrcodeApp.Config {
	if wechatOptions == nil {
		return nil
	}

	config := &qrcodeApp.Config{
		WeChatAppID:     wechatOptions.WeChatAppID,
		PagePath:        wechatOptions.PagePath,
		AppID:           wechatOptions.AppID,
		AppSecret:       wechatOptions.AppSecret,
		ObjectKeyPrefix: "qrcode",
		PublicURLPrefix: qrcodeApp.QRCodeURLPrefix,
	}
	if ossOptions != nil && ossOptions.ObjectKeyPrefix != "" {
		config.ObjectKeyPrefix = ossOptions.ObjectKeyPrefix
	}
	if ossOptions != nil && strings.TrimSpace(ossOptions.PublicBaseURL) != "" {
		config.PublicURLPrefix = strings.TrimRight(strings.TrimSpace(ossOptions.PublicBaseURL), "/")
	}
	return config
}

// QRCodeServiceInput collects dependencies for application-layer QR code service.
type QRCodeServiceInput struct {
	Generator        wechatmini.QRCodeGenerator
	WeChatAppService *iam.WeChatAppService
	ObjectStore      objectstorageport.PublicObjectStore
	WeChatOptions    *options.WeChatOptions
	OSSOptions       *options.OSSOptions
}

// QRCodeServiceInitResult is the outcome of optional QR code service initialization.
type QRCodeServiceInitResult struct {
	Service           qrcodeApp.QRCodeService
	ObjectStore       objectstorageport.PublicObjectStore
	ObjectKeyPrefix   string
	SkipReason        string
	ObjectStoreBucket string
	UseIAMWeChatApp   bool
	DirectAppID       string
}

// InitQRCodeService builds the application-layer QR code service when configured.
func InitQRCodeService(in QRCodeServiceInput) (QRCodeServiceInitResult, error) {
	result := QRCodeServiceInitResult{
		ObjectStore: in.ObjectStore,
	}
	if in.Generator == nil {
		result.SkipReason = "generator not available"
		return result, nil
	}
	if in.WeChatOptions == nil {
		result.SkipReason = "wechat options not provided"
		return result, nil
	}
	if in.WeChatOptions.WeChatAppID == "" && (in.WeChatOptions.AppID == "" || in.WeChatOptions.AppSecret == "") {
		result.SkipReason = "missing config: wechat-app-id or app-id/app-secret"
		return result, nil
	}
	if in.WeChatOptions.PagePath == "" {
		result.SkipReason = "missing page-path"
		return result, nil
	}

	storeResult, err := InitQRCodeObjectStore(in.ObjectStore, in.OSSOptions)
	if err != nil {
		return result, err
	}
	result.ObjectStore = storeResult.Store
	result.ObjectKeyPrefix = storeResult.KeyPrefix
	if in.OSSOptions != nil && in.OSSOptions.Enabled {
		result.ObjectStoreBucket = in.OSSOptions.Bucket
	}

	config := BuildQRCodeServiceConfig(in.WeChatOptions, in.OSSOptions)
	imageStore := qrcodeObjectStorage.NewQRCodeAssetStore(qrcodeObjectStorage.QRCodeAssetStoreOptions{
		ObjectStore:     result.ObjectStore,
		ObjectKeyPrefix: config.ObjectKeyPrefix,
		PublicURLPrefix: config.PublicURLPrefix,
		LocalStorageDir: qrcodeApp.QRCodeStorageDir,
	})
	if in.WeChatOptions.WeChatAppID != "" {
		result.UseIAMWeChatApp = true
	} else {
		result.DirectAppID = in.WeChatOptions.AppID
	}

	result.Service = qrcodeApp.NewService(
		in.Generator,
		config,
		in.WeChatAppService,
		imageStore,
	)
	return result, nil
}
