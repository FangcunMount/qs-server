package container

import (
	"fmt"
	"strings"

	qrcodeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/objectstorage/aliyunoss"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/wechatapi"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
	"github.com/silenceper/wechat/v2/cache"
)

// initQRCodeGenerator 初始化小程序码生成器（基础设施层）。
func (c *Container) initQRCodeGenerator() {
	wechatCache := c.buildWeChatSDKCache()
	c.QRCodeGenerator = wechatapi.NewQRCodeGenerator(wechatCache)
	c.SubscribeSender = wechatapi.NewSubscribeSender(wechatCache)
	c.printf("📱 QRCode generator initialized (infrastructure layer)\n")
}

func (c *Container) buildWeChatSDKCache() cache.Cache {
	if client := c.CacheClient(redisplane.FamilySDK); client != nil {
		return wechatapi.NewRedisCacheAdapterWithBuilder(client, c.CacheBuilder(redisplane.FamilySDK))
	}
	return cache.NewMemory()
}

func (c *Container) initQRCodeObjectStore(ossOptions *options.OSSOptions) error {
	if ossOptions == nil || !ossOptions.Enabled {
		c.QRCodeObjectStore = nil
		c.QRCodeObjectKeyPrefix = ""
		return nil
	}
	if c.QRCodeObjectStore != nil {
		return nil
	}

	store, err := aliyunoss.NewPublicObjectStore(ossOptions)
	if err != nil {
		return fmt.Errorf("initialize qrcode object store: %w", err)
	}
	c.QRCodeObjectStore = store
	c.QRCodeObjectKeyPrefix = ossOptions.ObjectKeyPrefix
	c.printf("🪣 QRCode object store initialized (bucket: %s)\n", ossOptions.Bucket)
	return nil
}

func (c *Container) resolveWeChatAppService() *iam.WeChatAppService {
	if c == nil || c.IAMModule == nil || !c.IAMModule.IsEnabled() {
		return nil
	}
	return c.IAMModule.WeChatAppService()
}

func (c *Container) buildQRCodeServiceConfig(wechatOptions *options.WeChatOptions, ossOptions *options.OSSOptions) *qrcodeApp.Config {
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

func (c *Container) wireQRCodeServiceDependencies() {
	newModuleGraph(c).postWireQRCodeService()
}

// InitQRCodeService 初始化小程序码生成服务（应用层）。
// 从配置中读取 wechat_app_id，然后从 IAM 查询微信应用信息。
func (c *Container) InitQRCodeService(wechatOptions *options.WeChatOptions, ossOptions *options.OSSOptions) error {
	if c == nil {
		return nil
	}

	// 如果基础设施层未初始化，则应用层服务也不初始化。
	if c.QRCodeGenerator == nil {
		c.printf("⚠️  QRCode service not initialized (generator not available)\n")
		return nil
	}

	// 如果未提供配置，则不初始化。
	if wechatOptions == nil {
		c.printf("⚠️  QRCode service not initialized (wechat options not provided)\n")
		return nil
	}

	if wechatOptions.WeChatAppID == "" && (wechatOptions.AppID == "" || wechatOptions.AppSecret == "") {
		c.printf("⚠️  QRCode service not initialized (missing config: wechat-app-id or app-id/app-secret)\n")
		return nil
	}
	if wechatOptions.PagePath == "" {
		c.printf("⚠️  QRCode service not initialized (missing page-path)\n")
		return nil
	}

	if err := c.initQRCodeObjectStore(ossOptions); err != nil {
		return err
	}

	config := c.buildQRCodeServiceConfig(wechatOptions, ossOptions)
	if wechatOptions.WeChatAppID != "" {
		c.printf("📱 QRCode service will use IAM to query wechat app (wechat_app_id: %s)\n", wechatOptions.WeChatAppID)
	} else {
		c.printf("📱 QRCode service will use direct config (app_id: %s)\n", wechatOptions.AppID)
	}

	c.QRCodeService = qrcodeApp.NewService(
		c.QRCodeGenerator,
		config,
		c.resolveWeChatAppService(),
		c.QRCodeObjectStore,
	)
	c.wireQRCodeServiceDependencies()
	c.printf("📱 QRCode service initialized (application layer, page_path: %s)\n", wechatOptions.PagePath)
	return nil
}
