package platform

import (
	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	modelcatalogApp "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	"github.com/FangcunMount/qs-server/internal/apiserver/container/compose"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
)

// InstallHost extends the shared compose seam with platform integration bindings.
type InstallHost interface {
	compose.Host
	PlatformState() IntegrationState
	ApplyPlatformState(IntegrationState)
	WeChatAppService() *iam.WeChatAppService
	ProfileLinkService() *iam.ProfileLinkService
	PublishedModelTitleResolver() modelcatalogApp.PublishedModelTitleResolver
	TesteeQuery() testeeApp.TesteeQueryService
	TaskNotificationContext() planApp.TaskNotificationContextReader
}

// InstallFrom wires codes service and QR code generator infrastructure.
func InstallFrom(host InstallHost) {
	if host == nil {
		return
	}
	state := host.PlatformState()
	state.CodesService = WireCodes(state.CodesService)
	if state.CodesService != nil {
		host.Printf("🔑 CodesService initialized\n")
	}
	gen := WireGenerator(GeneratorWireInput{
		SDKRedis:   host.CacheClient(cacheplane.FamilySDK),
		SDKBuilder: host.CacheBuilder(cacheplane.FamilySDK),
	})
	state.QRCodeGenerator = gen.QRCodeGenerator
	state.SubscribeSender = gen.SubscribeSender
	host.ApplyPlatformState(state)
	host.Printf("📱 QRCode generator initialized (infrastructure layer)\n")
}

// InitQRCodeServiceFrom wires the optional QR code application service.
func InitQRCodeServiceFrom(host InstallHost, wechatOptions *options.WeChatOptions, ossOptions *options.OSSOptions) error {
	if host == nil {
		return nil
	}
	result, err := WireQRCodeService(QRCodeServiceWireInput{
		State:            host.PlatformState(),
		WeChatAppService: host.WeChatAppService(),
		WeChatOptions:    wechatOptions,
		OSSOptions:       ossOptions,
	})
	if err != nil {
		return err
	}
	if msg := FormatSkipMessage("QRCode service", result.SkipReason); msg != "" {
		host.Printf("⚠️  %s\n", msg)
		return nil
	}
	host.ApplyPlatformState(result.State)
	if result.ObjectStoreBucket != "" {
		host.Printf("🪣 QRCode object store initialized (bucket: %s)\n", result.ObjectStoreBucket)
	}
	if result.UseIAMWeChatApp && wechatOptions != nil {
		host.Printf("📱 QRCode service will use IAM to query wechat app (wechat_app_id: %s)\n", wechatOptions.WeChatAppID)
	} else if result.DirectAppID != "" {
		host.Printf("📱 QRCode service will use direct config (app_id: %s)\n", result.DirectAppID)
	}
	if wechatOptions != nil && result.State.QRCodeService != nil {
		host.Printf("📱 QRCode service initialized (application layer, page_path: %s)\n", wechatOptions.PagePath)
	}
	return nil
}

// InitMiniProgramNotificationFrom wires the optional mini-program task notification service.
func InitMiniProgramNotificationFrom(host InstallHost, wechatOptions *options.WeChatOptions) {
	if host == nil {
		return
	}
	result := WireMiniProgramTaskNotificationService(MiniProgramNotificationWireInput{
		State:                  host.PlatformState(),
		WeChatAppService:       host.WeChatAppService(),
		ProfileLinkService:     host.ProfileLinkService(),
		IdentityService:        host.IdentityService(),
		PublishedTitleResolver: host.PublishedModelTitleResolver(),
		TesteeQuery:            host.TesteeQuery(),
		TaskContext:            host.TaskNotificationContext(),
		WeChatOptions:          wechatOptions,
	})
	if msg := FormatSkipMessage("MiniProgram task notification service", result.SkipReason); msg != "" {
		host.Printf("⚠️  %s\n", msg)
		return
	}
	state := host.PlatformState()
	state.MiniProgramTaskNotificationService = result.Service
	host.ApplyPlatformState(state)
	host.Printf("📨 MiniProgram task notification service initialized (template_id: %s)\n", result.TemplateID)
}
