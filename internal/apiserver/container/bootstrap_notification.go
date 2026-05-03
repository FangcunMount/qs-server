package container

import (
	"strings"

	notificationApp "github.com/FangcunMount/qs-server/internal/apiserver/application/notification"
	scaleApp "github.com/FangcunMount/qs-server/internal/apiserver/application/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
)

type miniProgramTaskNotificationDeps struct {
	wechatAppService  *iam.WeChatAppService
	recipientResolver iambridge.MiniProgramRecipientResolver
	scaleQuery        scaleApp.ScaleQueryService
}

func (c *Container) resolveMiniProgramTaskNotificationDeps() miniProgramTaskNotificationDeps {
	deps := miniProgramTaskNotificationDeps{}
	if c == nil {
		return deps
	}
	if c.IAMModule != nil && c.IAMModule.IsEnabled() {
		deps.wechatAppService = c.IAMModule.WeChatAppService()
		deps.recipientResolver = iam.NewMiniProgramRecipientResolver(
			c.IAMModule.GuardianshipService(),
			c.IAMModule.IdentityService(),
		)
	}
	if c.ScaleModule != nil {
		deps.scaleQuery = c.ScaleModule.QueryService
	}
	return deps
}

func buildMiniProgramTaskNotificationConfig(wechatOptions *options.WeChatOptions) *notificationApp.Config {
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

// InitMiniProgramTaskNotificationService 初始化 task.opened 小程序消息服务。
func (c *Container) InitMiniProgramTaskNotificationService(wechatOptions *options.WeChatOptions) {
	if c == nil {
		return
	}
	if c.SubscribeSender == nil {
		c.printf("⚠️  MiniProgram task notification service not initialized (subscribe sender not available)\n")
		return
	}
	if c.ActorModule == nil || c.ActorModule.TesteeQueryService == nil {
		c.printf("⚠️  MiniProgram task notification service not initialized (testee query service not available)\n")
		return
	}
	if c.PlanModule == nil || c.PlanModule.TaskNotificationContextReader == nil {
		c.printf("⚠️  MiniProgram task notification service not initialized (plan task notification context reader not available)\n")
		return
	}
	if wechatOptions == nil {
		c.printf("⚠️  MiniProgram task notification service not initialized (wechat options not provided)\n")
		return
	}
	if strings.TrimSpace(wechatOptions.TaskOpenedTemplateID) == "" {
		c.printf("⚠️  MiniProgram task notification service not initialized (missing task-opened-template-id)\n")
		return
	}
	if wechatOptions.WeChatAppID == "" && (wechatOptions.AppID == "" || wechatOptions.AppSecret == "") {
		c.printf("⚠️  MiniProgram task notification service not initialized (missing wechat app config)\n")
		return
	}

	deps := c.resolveMiniProgramTaskNotificationDeps()
	c.MiniProgramTaskNotificationService = notificationApp.NewMiniProgramTaskNotificationService(
		c.ActorModule.TesteeQueryService,
		c.PlanModule.TaskNotificationContextReader,
		deps.scaleQuery,
		deps.recipientResolver,
		deps.wechatAppService,
		c.SubscribeSender,
		buildMiniProgramTaskNotificationConfig(wechatOptions),
	)
	c.printf("📨 MiniProgram task notification service initialized (template_id: %s)\n", wechatOptions.TaskOpenedTemplateID)
}
