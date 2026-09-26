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
// modules are ready. An enabled producer must never silently fall back to the
// old best-effort sender when a durable consumer dependency is unavailable.
func (c *Container) InitTaskOpenedReminderService(wechatOptions *options.WeChatOptions) error {
	if c == nil || !c.TaskOpenedReminderEnabled() {
		return nil
	}
	if c.PlanModule == nil || c.PlanModule.TaskReminderStateReader == nil || c.mysqlDB == nil ||
		c.IAMModule == nil || !c.IAMModule.IsEnabled() || c.SubscribeSender == nil || wechatOptions == nil ||
		c.TesteeQuery() == nil {
		return fmt.Errorf("durable task reminder dependencies are unavailable")
	}
	receipts, ok := c.SubscribeSender.(wechatmini.MiniProgramSubscribeReceiptSender)
	if !ok {
		return fmt.Errorf("durable task reminder requires a receipt-capable WeChat sender")
	}
	identities := iam.NewMiniProgramRecipientCandidateReader(c.ProfileLinkService(), c.IAMModule.Client())
	if identities == nil {
		return fmt.Errorf("durable task reminder requires IAM recipient lookup")
	}
	config := platformmod.BuildMiniProgramTaskNotificationConfig(wechatOptions)
	if config == nil || config.TaskOpenedTemplateID == "" || config.PagePath == "" {
		return fmt.Errorf("durable task reminder requires a template and mini-program page")
	}
	if (config.WeChatAppID != "" && c.WeChatAppService() == nil) ||
		(config.WeChatAppID == "" && (config.AppID == "" || config.AppSecret == "")) {
		return fmt.Errorf("durable task reminder requires a resolvable WeChat AppID and secret")
	}
	c.TaskOpenedReminderService = notificationApp.NewTaskOpenedReminderService(
		c.PlanModule.TaskReminderStateReader, c.TesteeQuery(), identities,
		mysqlnotification.NewReminderBatchLedger(c.mysqlDB),
		mysqlnotification.NewReminderDeliveryLedger(c.mysqlDB),
		c.WeChatAppService(), c.SubscribeSender, receipts,
		c.PlanModule.TaskNotificationContextReader, c.PublishedModelTitleResolver(), config,
	)
	return nil
}
