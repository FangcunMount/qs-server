package platform

import (
	"fmt"

	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	codesapp "github.com/FangcunMount/qs-server/internal/apiserver/application/codes"
	modelcatalogApp "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	notificationApp "github.com/FangcunMount/qs-server/internal/apiserver/application/notification"
	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	qrcodeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/qrcode"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	objectstorageport "github.com/FangcunMount/qs-server/internal/apiserver/infra/objectstorage/port"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	wechatmini "github.com/FangcunMount/qs-server/internal/apiserver/port/wechatmini"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	redis "github.com/redis/go-redis/v9"
)

// IntegrationState holds platform integration services owned by the composition root.
type IntegrationState struct {
	CodesService                       codesapp.CodesService
	QRCodeGenerator                    wechatmini.QRCodeGenerator
	SubscribeSender                    wechatmini.MiniProgramSubscribeSender
	QRCodeObjectStore                  objectstorageport.PublicObjectStore
	QRCodeObjectKeyPrefix              string
	QRCodeService                      qrcodeApp.QRCodeService
	MiniProgramTaskNotificationService notificationApp.MiniProgramTaskNotificationService
}

// GeneratorWireInput carries cache-plane inputs for QR code infrastructure.
type GeneratorWireInput struct {
	SDKRedis   redis.UniversalClient
	SDKBuilder *keyspace.Builder
}

// WireGenerator builds QR code generator infrastructure.
func WireGenerator(in GeneratorWireInput) IntegrationState {
	gen := NewQRCodeGeneratorInfra(QRCodeCacheAccess(in))
	return IntegrationState{
		QRCodeGenerator: gen.Generator,
		SubscribeSender: gen.SubscribeSender,
	}
}

// WireCodes initializes the codes application service when absent.
func WireCodes(existing codesapp.CodesService) codesapp.CodesService {
	if existing != nil {
		return existing
	}
	return NewCodesService()
}

// QRCodeServiceWireInput carries dependencies for optional QR code application service.
type QRCodeServiceWireInput struct {
	State            IntegrationState
	WeChatAppService *iam.WeChatAppService
	WeChatOptions    *options.WeChatOptions
	OSSOptions       *options.OSSOptions
}

// QRCodeServiceWireResult describes QR code service initialization outcome.
type QRCodeServiceWireResult struct {
	State             IntegrationState
	SkipReason        string
	UseIAMWeChatApp   bool
	DirectAppID       string
	ObjectStoreBucket string
}

// WireQRCodeService initializes the QR code application service when dependencies are available.
func WireQRCodeService(in QRCodeServiceWireInput) (QRCodeServiceWireResult, error) {
	result, err := InitQRCodeService(QRCodeServiceInput{
		Generator:        in.State.QRCodeGenerator,
		WeChatAppService: in.WeChatAppService,
		ObjectStore:      in.State.QRCodeObjectStore,
		WeChatOptions:    in.WeChatOptions,
		OSSOptions:       in.OSSOptions,
	})
	if err != nil {
		return QRCodeServiceWireResult{State: in.State}, err
	}
	state := in.State
	if result.ObjectStore != nil {
		state.QRCodeObjectStore = result.ObjectStore
		state.QRCodeObjectKeyPrefix = result.ObjectKeyPrefix
	}
	if result.Service != nil {
		state.QRCodeService = result.Service
	}
	return QRCodeServiceWireResult{
		State:             state,
		SkipReason:        result.SkipReason,
		UseIAMWeChatApp:   result.UseIAMWeChatApp,
		DirectAppID:       result.DirectAppID,
		ObjectStoreBucket: result.ObjectStoreBucket,
	}, nil
}

// MiniProgramNotificationWireInput carries dependencies for task notification wiring.
type MiniProgramNotificationWireInput struct {
	State                  IntegrationState
	WeChatAppService       *iam.WeChatAppService
	ProfileLinkService     *iam.ProfileLinkService
	IdentityService        *iam.IdentityService
	PublishedTitleResolver modelcatalogApp.PublishedModelTitleResolver
	TesteeQuery            testeeApp.TesteeQueryService
	TaskContext            planApp.TaskNotificationContextReader
	WeChatOptions          *options.WeChatOptions
}

// MiniProgramNotificationWireResult describes notification service initialization outcome.
type MiniProgramNotificationWireResult struct {
	Service    notificationApp.MiniProgramTaskNotificationService
	TemplateID string
	SkipReason string
}

// WireMiniProgramTaskNotificationService initializes the mini-program task notification service.
func WireMiniProgramTaskNotificationService(in MiniProgramNotificationWireInput) MiniProgramNotificationWireResult {
	var recipientResolver iambridge.MiniProgramRecipientResolver
	if in.ProfileLinkService != nil && in.IdentityService != nil {
		recipientResolver = ResolveMiniProgramRecipientResolver(in.ProfileLinkService, in.IdentityService)
	}
	result := InitMiniProgramTaskNotificationService(MiniProgramTaskNotificationDeps{
		TesteeQueryService:            in.TesteeQuery,
		TaskNotificationContextReader: in.TaskContext,
		PublishedTitleResolver:        in.PublishedTitleResolver,
		RecipientResolver:             recipientResolver,
		WeChatAppService:              in.WeChatAppService,
		SubscribeSender:               in.State.SubscribeSender,
	}, in.WeChatOptions)
	return MiniProgramNotificationWireResult{
		Service:    result.Service,
		TemplateID: result.TemplateID,
		SkipReason: result.SkipReason,
	}
}

// FormatSkipMessage prefixes platform skip reasons for container logging.
func FormatSkipMessage(service string, reason string) string {
	if reason == "" {
		return ""
	}
	return fmt.Sprintf("%s not initialized (%s)", service, reason)
}
