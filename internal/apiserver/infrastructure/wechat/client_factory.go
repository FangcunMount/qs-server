package wechat

import (
	"context"
	"sync"

	"github.com/redis/go-redis/v9"
	wechatSDK "github.com/silenceper/wechat/v2"
	"github.com/silenceper/wechat/v2/cache"
	"github.com/silenceper/wechat/v2/miniprogram"
	mpConfig "github.com/silenceper/wechat/v2/miniprogram/config"
	"github.com/silenceper/wechat/v2/officialaccount"
	oaConfig "github.com/silenceper/wechat/v2/officialaccount/config"

	domainWechat "github.com/fangcun-mount/qs-server/internal/apiserver/domain/wechat"
	wechatPort "github.com/fangcun-mount/qs-server/internal/apiserver/domain/wechat/port"
	"github.com/fangcun-mount/qs-server/internal/pkg/code"
	"github.com/fangcun-mount/qs-server/pkg/errors"
)

// WxClientFactory 微信客户端工厂（按 app 产出 SDK 客户端）
// 实现 wechatPort.WechatSDK 接口（防腐层）
type WxClientFactory struct {
	wc        *wechatSDK.Wechat
	appRepo   wechatPort.AppRepository
	rdb       *redis.Client
	miniCache sync.Map // appID -> *miniprogram.MiniProgram
	oaCache   sync.Map // appID -> *officialaccount.OfficialAccount
}

// NewWxClientFactory 创建微信客户端工厂
func NewWxClientFactory(appRepo wechatPort.AppRepository, rdb *redis.Client) *WxClientFactory {
	return &WxClientFactory{
		wc:      wechatSDK.NewWechat(),
		appRepo: appRepo,
		rdb:     rdb,
	}
}

// GetMini 获取小程序客户端
func (f *WxClientFactory) GetMini(ctx context.Context, appID string) (*miniprogram.MiniProgram, error) {
	// 1. 从缓存获取
	if client, ok := f.miniCache.Load(appID); ok {
		return client.(*miniprogram.MiniProgram), nil
	}

	// 2. 从数据库加载配置
	app, err := f.appRepo.FindByPlatformAndAppID(ctx, domainWechat.PlatformMini, appID)
	if err != nil {
		return nil, errors.WithCode(code.ErrDatabase, "failed to find mini app: %v", err)
	}
	if !app.IsEnabled() {
		return nil, errors.WithCode(code.ErrValidation, "mini app is disabled")
	}

	// 3. 创建客户端
	// 使用内存缓存（简化处理，生产环境建议使用 Redis）
	// TODO: 实现 Redis 缓存适配器
	memCache := cache.NewMemory()

	cfg := &mpConfig.Config{
		AppID:     app.AppID(),
		AppSecret: app.Secret(),
		Cache:     memCache,
	}

	mini := f.wc.GetMiniProgram(cfg)

	// 4. 缓存
	f.miniCache.Store(appID, mini)

	return mini, nil
}

// GetOA 获取公众号客户端
func (f *WxClientFactory) GetOA(ctx context.Context, appID string) (*officialaccount.OfficialAccount, error) {
	// 1. 从缓存获取
	if client, ok := f.oaCache.Load(appID); ok {
		return client.(*officialaccount.OfficialAccount), nil
	}

	// 2. 从数据库加载配置
	app, err := f.appRepo.FindByPlatformAndAppID(ctx, domainWechat.PlatformOA, appID)
	if err != nil {
		return nil, errors.WithCode(code.ErrDatabase, "failed to find oa app: %v", err)
	}
	if !app.IsEnabled() {
		return nil, errors.WithCode(code.ErrValidation, "oa app is disabled")
	}

	// 3. 创建客户端
	// 使用内存缓存（简化处理，生产环境建议使用 Redis）
	memCache := cache.NewMemory()

	cfg := &oaConfig.Config{
		AppID:          app.AppID(),
		AppSecret:      app.Secret(),
		Token:          app.Token(),
		EncodingAESKey: app.EncodingAESKey(),
		Cache:          memCache,
	}

	oa := f.wc.GetOfficialAccount(cfg)

	// 4. 缓存
	f.oaCache.Store(appID, oa)

	return oa, nil
}

// ClearCache 清除缓存（配置更新时调用）
func (f *WxClientFactory) ClearCache(appID string) {
	f.miniCache.Delete(appID)
	f.oaCache.Delete(appID)
}

// ClearAllCache 清除所有缓存
func (f *WxClientFactory) ClearAllCache() {
	f.miniCache = sync.Map{}
	f.oaCache = sync.Map{}
}

// Code2Session 小程序code换session
func (f *WxClientFactory) Code2Session(ctx context.Context, appID, jsCode string) (openID, sessionKey, unionID string, err error) {
	mini, err := f.GetMini(ctx, appID)
	if err != nil {
		return "", "", "", err
	}

	result, err := mini.GetAuth().Code2Session(jsCode)
	if err != nil {
		return "", "", "", errors.WithCode(code.ErrUnknown, "code2session failed: %v", err)
	}

	if result.ErrCode != 0 {
		return "", "", "", errors.WithCode(code.ErrUnknown, "code2session failed: %s", result.ErrMsg)
	}

	return result.OpenID, result.SessionKey, result.UnionID, nil
}

// DecryptPhoneNumber 解密小程序手机号
func (f *WxClientFactory) DecryptPhoneNumber(ctx context.Context, appID, sessionKey, encryptedData, iv string) (phone string, err error) {
	mini, err := f.GetMini(ctx, appID)
	if err != nil {
		return "", err
	}

	_, err = mini.GetEncryptor().Decrypt(sessionKey, encryptedData, iv)
	if err != nil {
		return "", errors.WithCode(code.ErrUnknown, "decrypt phone failed: %v", err)
	}

	// TODO: 解析手机号
	// 这里简化处理，实际需要解析返回的JSON
	// result 包含解密后的数据，需要进一步处理

	return "", nil
}
