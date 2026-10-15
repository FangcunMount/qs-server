package plan_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	testeeapp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	notificationapp "github.com/FangcunMount/qs-server/internal/apiserver/application/notification"
	planapp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	domainplan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	mysqlnotification "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/notification"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	wechatmini "github.com/FangcunMount/qs-server/internal/apiserver/port/wechatmini"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	legacywire "github.com/FangcunMount/reliable-messaging/wire/legacy"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// The enabled producer's original payload reaches the real notification service
// after the producer has been disabled. Task state and notification ledgers use
// the disposable MySQL; only IAM lookup and the external WeChat response are fixtures.
func assertDisabledProducerBacklogSettlesReminder(t *testing.T, db *gorm.DB, tasks domainplan.AssessmentTaskRepository, payload []byte) {
	t.Helper()
	for _, name := range []string{"000087_task_opened_reminder_delivery.up.sql", "000088_task_opened_reminder_batch.up.sql"} {
		schema, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "pkg", "migration", "migrations", "mysql", name))
		require.NoError(t, err)
		require.NoError(t, db.Exec(string(schema)).Error)
	}
	decoded, recognized, err := legacywire.Decode(payload)
	require.NoError(t, err)
	require.True(t, recognized)
	var opening struct {
		ID   string                                       `json:"id"`
		Data eventpayload.TaskOpenedReminderRequestedData `json:"data"`
	}
	require.NoError(t, json.Unmarshal(decoded.Payload, &opening))
	request := notificationapp.TaskOpenedReminderRequest{OpeningEventID: opening.ID, Intent: notificationapp.TaskOpenedReminderIntent{
		OrgID: opening.Data.OrgID, TaskID: opening.Data.TaskID, TesteeID: opening.Data.TesteeID,
		ScheduleRevision: opening.Data.ScheduleRevision, OpenAt: opening.Data.OpenAt,
	}}
	sender := &backlogReceiptSender{}
	newConsumer := func() notificationapp.TaskOpenedReminderService {
		return notificationapp.NewTaskOpenedReminderService(planapp.NewTaskReminderStateReader(tasks), backlogTesteeLookup{},
			backlogRecipientReader{}, mysqlnotification.NewReminderBatchLedger(db), mysqlnotification.NewReminderDeliveryLedger(db),
			nil, sender, sender, nil, nil, &notificationapp.Config{
				AppID: "m2-backlog-app", AppSecret: "isolated-test-only", PagePath: "pages/task/index", TaskOpenedTemplateID: "m2-backlog-template",
			})
	}
	require.NoError(t, newConsumer().ProcessTaskOpenedReminder(t.Context(), request))
	key := notificationapp.ReminderDeliveryKey{OrgID: request.Intent.OrgID, TaskID: request.Intent.TaskID,
		OpeningEventID: request.OpeningEventID, ScheduleRevision: request.Intent.ScheduleRevision,
		LoginIdentityID: "m2-backlog-identity", AppID: "m2-backlog-app", TemplateID: "m2-backlog-template", ReminderVersion: 1}
	delivery, err := mysqlnotification.NewReminderDeliveryLedger(db).Read(t.Context(), key)
	require.NoError(t, err)
	require.Equal(t, notificationapp.ReminderConfirmed, delivery.State)
	require.Len(t, sender.sent, 1)
	require.Equal(t, "m2-backlog-openid", sender.sent[0].ToUser)
	require.Equal(t, "m2-backlog-template", sender.sent[0].TemplateID)
	require.Contains(t, sender.sent[0].Page, "task_id="+request.Intent.TaskID)
	// A rebuilt service has no memory of the first send. It must use the real
	// persisted settlement rather than resend the same immutable opening.
	require.NoError(t, newConsumer().ProcessTaskOpenedReminder(t.Context(), request))
	require.Len(t, sender.sent, 1, "redelivery after producer shutdown must not repeat a settled send")
	var preserved []byte
	require.NoError(t, db.Raw("SELECT payload FROM rm_outbox WHERE message_id = ?", decoded.UUID).Row().Scan(&preserved))
	require.Equal(t, payload, preserved)
}

type backlogTesteeLookup struct{}

func (backlogTesteeLookup) GetByID(_ context.Context, id uint64) (*testeeapp.TesteeResult, error) {
	profile := uint64(1001)
	return &testeeapp.TesteeResult{ID: id, OrgID: 501, ProfileID: &profile}, nil
}

type backlogRecipientReader struct{}

func (backlogRecipientReader) ListLinkedUsers(context.Context, string) ([]iambridge.MiniProgramLinkedUser, error) {
	return []iambridge.MiniProgramLinkedUser{{UserID: "m2-backlog-user", Relations: []string{"self"}}}, nil
}

func (backlogRecipientReader) ReadCandidates(context.Context, string, string, []string) ([]iambridge.MiniProgramRecipientCandidate, error) {
	return []iambridge.MiniProgramRecipientCandidate{{UserID: "m2-backlog-user", LoginIdentityID: "m2-backlog-identity",
		AppID: "m2-backlog-app", OpenID: "m2-backlog-openid", Relations: []string{"self"}}}, nil
}

type backlogReceiptSender struct{ sent []wechatmini.SubscribeMessage }

func (*backlogReceiptSender) SendSubscribeMessage(context.Context, string, string, wechatmini.SubscribeMessage) error {
	return fmt.Errorf("durable backlog must use the receipt sender")
}

func (*backlogReceiptSender) ListTemplates(context.Context, string, string) ([]wechatmini.SubscribeTemplate, error) {
	return []wechatmini.SubscribeTemplate{{ID: "m2-backlog-template", Content: "{{thing5.DATA}}{{date1.DATA}}{{character_string2.DATA}}{{thing3.DATA}}"}}, nil
}

func (s *backlogReceiptSender) SendSubscribeMessageWithReceipt(_ context.Context, _, _ string, message wechatmini.SubscribeMessage) (wechatmini.SubscribeSendReceipt, error) {
	s.sent = append(s.sent, message)
	return wechatmini.SubscribeSendReceipt{Accepted: true, PlatformMessageID: "m2-backlog-receipt"}, nil
}
