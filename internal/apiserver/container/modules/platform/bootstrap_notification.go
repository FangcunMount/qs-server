package platform

import (
	"strings"

	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	scaleApp "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/behavior/scale"
	notificationApp "github.com/FangcunMount/qs-server/internal/apiserver/application/notification"
	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	wechatmini "github.com/FangcunMount/qs-server/internal/apiserver/port/wechatmini"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
)

// MiniProgramTaskNotificationDeps groups cross-module ports for task notifications.
type MiniProgramTaskNotificationDeps struct {
	TesteeQueryService            testeeApp.TesteeQueryService
	TaskNotificationContextReader planApp.TaskNotificationContextReader
	ScaleQuery                    scaleApp.ScaleQueryService
	RecipientResolver             iambridge.MiniProgramRecipientResolver
	WeChatAppService              *iam.WeChatAppService
	SubscribeSender               wechatmini.MiniProgramSubscribeSender
}

// BuildMiniProgramTaskNotificationConfig maps WeChat options to notification config.
func BuildMiniProgramTaskNotificationConfig(wechatOptions *options.WeChatOptions) *notificationApp.Config {
	if wechatOptions == nil {
		return nil
	}
	return &notificationApp.Config{
		WeChatAppID:          wechatOptions.WeChatAppID,
		PagePath:             wechatOptions.PagePath,
		AppID:                wechatOptions.AppID,
		AppSecret:            wechatOptions.AppSecret,
		TaskOpenedTemplateID: wechatOptions.TaskOpenedTemplateID,
	}
}

// MiniProgramTaskNotificationInitResult describes optional notification service wiring.
type MiniProgramTaskNotificationInitResult struct {
	Service    notificationApp.MiniProgramTaskNotificationService
	SkipReason string
	TemplateID string
}

// InitMiniProgramTaskNotificationService builds task.opened notification service when configured.
func InitMiniProgramTaskNotificationService(
	deps MiniProgramTaskNotificationDeps,
	wechatOptions *options.WeChatOptions,
) MiniProgramTaskNotificationInitResult {
	result := MiniProgramTaskNotificationInitResult{}
	if deps.SubscribeSender == nil {
		result.SkipReason = "subscribe sender not available"
		return result
	}
	if deps.TesteeQueryService == nil {
		result.SkipReason = "testee query service not available"
		return result
	}
	if deps.TaskNotificationContextReader == nil {
		result.SkipReason = "plan task notification context reader not available"
		return result
	}
	if wechatOptions == nil {
		result.SkipReason = "wechat options not provided"
		return result
	}
	if strings.TrimSpace(wechatOptions.TaskOpenedTemplateID) == "" {
		result.SkipReason = "missing task-opened-template-id"
		return result
	}
	if wechatOptions.WeChatAppID == "" && (wechatOptions.AppID == "" || wechatOptions.AppSecret == "") {
		result.SkipReason = "missing wechat app config"
		return result
	}

	result.Service = notificationApp.NewMiniProgramTaskNotificationService(
		deps.TesteeQueryService,
		deps.TaskNotificationContextReader,
		deps.ScaleQuery,
		deps.RecipientResolver,
		deps.WeChatAppService,
		deps.SubscribeSender,
		BuildMiniProgramTaskNotificationConfig(wechatOptions),
	)
	result.TemplateID = wechatOptions.TaskOpenedTemplateID
	return result
}

// ResolveMiniProgramRecipientResolver builds IAM recipient resolver when enabled.
func ResolveMiniProgramRecipientResolver(
	profileLink *iam.ProfileLinkService,
	identity *iam.IdentityService,
) iambridge.MiniProgramRecipientResolver {
	if profileLink == nil || identity == nil {
		return nil
	}
	return iam.NewMiniProgramRecipientResolver(profileLink, identity)
}
