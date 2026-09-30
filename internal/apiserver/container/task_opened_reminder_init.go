package container

import (
	"fmt"

	notificationApp "github.com/FangcunMount/qs-server/internal/apiserver/application/notification"
	platformmod "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/platform"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	mysqlnotification "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/notification"
	wechatmini "github.com/FangcunMount/qs-server/internal/apiserver/port/wechatmini"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
)

// InitTaskOpenedReminderService is called after the Plan, IAM, and WeChat
// modules are ready. The producer flag controls new opening intents, not the
// consumer: queued intents must remain processable after the flag is turned off.
// An enabled producer still fails startup when a consumer dependency is absent.
func (c *Container) InitTaskOpenedReminderService(wechatOptions *options.WeChatOptions) error {
	if c == nil {
		return nil
	}
	service, err := initTaskOpenedReminderConsumer(c.TaskOpenedReminderEnabled(), func() (notificationApp.TaskOpenedReminderService, error) {
		return c.buildTaskOpenedReminderService(wechatOptions)
	})
	if err != nil {
		return err
	}
	c.TaskOpenedReminderService = service
	return nil
}

func initTaskOpenedReminderConsumer(
	required bool, build func() (notificationApp.TaskOpenedReminderService, error),
) (notificationApp.TaskOpenedReminderService, error) {
	service, err := build()
	if err != nil && !required {
		return nil, nil
	}
	return service, err
}

func (c *Container) buildTaskOpenedReminderService(
	wechatOptions *options.WeChatOptions,
) (notificationApp.TaskOpenedReminderService, error) {
	if c.PlanModule == nil || c.PlanModule.TaskReminderStateReader == nil || c.mysqlDB == nil ||
		c.IAMModule == nil || !c.IAMModule.IsEnabled() || c.SubscribeSender == nil || wechatOptions == nil ||
		c.TesteeQuery() == nil {
		return nil, fmt.Errorf("durable task reminder dependencies are unavailable")
	}
	receipts, ok := c.SubscribeSender.(wechatmini.MiniProgramSubscribeReceiptSender)
	if !ok {
		return nil, fmt.Errorf("durable task reminder requires a receipt-capable WeChat sender")
	}
	identities := iam.NewMiniProgramRecipientCandidateReader(c.ProfileLinkService(), c.IAMModule.Client())
	if identities == nil {
		return nil, fmt.Errorf("durable task reminder requires IAM recipient lookup")
	}
	config := platformmod.BuildMiniProgramTaskNotificationConfig(wechatOptions)
	if config == nil || config.TaskOpenedTemplateID == "" || config.PagePath == "" {
		return nil, fmt.Errorf("durable task reminder requires a template and mini-program page")
	}
	if (config.WeChatAppID != "" && c.WeChatAppService() == nil) ||
		(config.WeChatAppID == "" && (config.AppID == "" || config.AppSecret == "")) {
		return nil, fmt.Errorf("durable task reminder requires a resolvable WeChat AppID and secret")
	}
	return notificationApp.NewTaskOpenedReminderService(
		c.PlanModule.TaskReminderStateReader, c.TesteeQuery(), identities,
		mysqlnotification.NewReminderBatchLedger(c.mysqlDB),
		mysqlnotification.NewReminderDeliveryLedger(c.mysqlDB),
		c.WeChatAppService(), c.SubscribeSender, receipts,
		c.PlanModule.TaskNotificationContextReader, c.PublishedModelTitleResolver(), config,
	), nil
}
