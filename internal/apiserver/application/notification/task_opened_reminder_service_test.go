package notification

import (
	"context"
	"errors"
	"testing"
	"time"

	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	planDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	wechatmini "github.com/FangcunMount/qs-server/internal/apiserver/port/wechatmini"
	"github.com/stretchr/testify/require"
)

type reminderTaskReaderStub struct{ state *planApp.TaskReminderState }

func (s *reminderTaskReaderStub) GetTaskReminderState(context.Context, int64, string) (*planApp.TaskReminderState, error) {
	return s.state, nil
}

type reminderBatchStub struct{ batch *ReminderBatch }

func (s *reminderBatchStub) FindBatch(context.Context, ReminderBatchKey) (ReminderBatch, bool, error) {
	if s.batch == nil {
		return ReminderBatch{}, false, nil
	}
	return *s.batch, true, nil
}

func (s *reminderBatchStub) ReadBatch(context.Context, ReminderBatchKey) (ReminderBatch, error) {
	return *s.batch, nil
}

func (s *reminderBatchStub) FreezeRecipients(_ context.Context, key ReminderBatchKey, appID, templateID string, identities []ReminderRecipientIdentity, _ time.Time) (ReminderBatch, error) {
	if s.batch == nil {
		s.batch = &ReminderBatch{Key: key, AppID: appID, TemplateID: templateID, Recipients: identities}
	}
	return *s.batch, nil
}

func (s *reminderBatchStub) SuppressEmpty(_ context.Context, key ReminderBatchKey, appID, templateID, code string, _ time.Time) (ReminderBatch, error) {
	if s.batch == nil {
		s.batch = &ReminderBatch{Key: key, AppID: appID, TemplateID: templateID, Suppressed: true, ResolutionCode: code}
	}
	return *s.batch, nil
}

type reminderDeliveryStub struct {
	state        ReminderDeliveryState
	beginCount   int
	unknownCount int
	unknownCode  string
	onBegin      func()
}

func (s *reminderDeliveryStub) EnsurePending(context.Context, ReminderDeliveryKey, string, time.Time) (ReminderDelivery, error) {
	return ReminderDelivery{}, nil
}
func (s *reminderDeliveryStub) Claim(context.Context, ReminderDeliveryKey, time.Duration, time.Time) (string, bool, error) {
	if s.state != "" && s.state != ReminderPending {
		return "", false, nil
	}
	s.state = ReminderClaimed
	return "claim-1", true, nil
}
func (s *reminderDeliveryStub) BeginExternalCall(context.Context, ReminderDeliveryKey, string, time.Time) (bool, error) {
	s.beginCount++
	s.state = ReminderSending
	if s.onBegin != nil {
		s.onBegin()
	}
	return true, nil
}
func (s *reminderDeliveryStub) Confirm(context.Context, ReminderDeliveryKey, string, string, time.Time) (bool, error) {
	s.state = ReminderConfirmed
	return true, nil
}
func (s *reminderDeliveryStub) MarkUnknown(_ context.Context, _ ReminderDeliveryKey, _, code string, _ time.Time) (bool, error) {
	s.unknownCount++
	s.unknownCode = code
	s.state = ReminderManualRequired
	return true, nil
}
func (s *reminderDeliveryStub) ReleaseUnsent(context.Context, ReminderDeliveryKey, string, time.Time) (bool, error) {
	s.state = ReminderPending
	return true, nil
}
func (s *reminderDeliveryStub) SuppressUnsent(context.Context, ReminderDeliveryKey, string, string, time.Time) (bool, error) {
	s.state = ReminderSuppressed
	return true, nil
}
func (s *reminderDeliveryStub) Read(context.Context, ReminderDeliveryKey) (ReminderDelivery, error) {
	return ReminderDelivery{State: s.state}, nil
}
func (s *reminderDeliveryStub) ListNeedsReview(context.Context, int64, time.Time, int) ([]ReminderDelivery, error) {
	return nil, nil
}

type receiptSenderStub struct {
	calls            int
	err              error
	withoutMessageID bool
}

func (s *receiptSenderStub) SendSubscribeMessageWithReceipt(_ context.Context, _, _ string, _ wechatmini.SubscribeMessage) (wechatmini.SubscribeSendReceipt, error) {
	s.calls++
	if s.err != nil {
		return wechatmini.SubscribeSendReceipt{}, s.err
	}
	receipt := wechatmini.SubscribeSendReceipt{Accepted: true}
	if !s.withoutMessageID {
		receipt.PlatformMessageID = "wechat-123"
	}
	return receipt, nil
}

func TestDurableReminderUnknownResultNeverSendsAgain(t *testing.T) {
	opened := time.Date(2026, 9, 26, 10, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	expires := opened.Add(24 * time.Hour)
	task := &reminderTaskReaderStub{state: &planApp.TaskReminderState{
		OrgID: 7, TaskID: "task-1", TesteeID: "12", Status: planDomain.TaskStatusOpened,
		ScheduleRevision: 3, OpenAt: &opened, ExpireAt: &expires,
		EntryURL: "https://collect.example/entry?token=secret&task_id=task-1",
	}}
	profileID := uint64(1001)
	identities := &reminderCandidateReaderStub{
		linked: []iambridge.MiniProgramLinkedUser{
			{UserID: "self-user", Relations: []string{"self"}},
			{UserID: "parent-user", Relations: []string{"parent"}},
		},
		candidates: []iambridge.MiniProgramRecipientCandidate{
			{UserID: "self-user", LoginIdentityID: "self-identity", AppID: "wx-app", OpenID: "self-openid", Relations: []string{"self"}},
		},
	}
	batches := &reminderBatchStub{}
	deliveries := &reminderDeliveryStub{}
	receipts := &receiptSenderStub{err: errors.New("response lost after send")}
	templates := &senderStub{templates: []wechatmini.SubscribeTemplate{{
		ID: "tmpl-1", Content: "{{thing5.DATA}}{{date1.DATA}}{{character_string2.DATA}}{{thing3.DATA}}",
	}}}
	service := NewTaskOpenedReminderService(task, &testeeLookupStub{result: &testeeApp.TesteeResult{
		ID: 12, ProfileID: &profileID,
	}}, identities, batches, deliveries, &wechatAppLookupStub{}, templates, receipts, nil, nil,
		&Config{AppID: "wx-app", AppSecret: "wx-secret", PagePath: "pages/task/index", TaskOpenedTemplateID: "tmpl-1"},
	).(*taskOpenedReminderService)
	service.now = func() time.Time { return opened.Add(time.Minute) }
	request := TaskOpenedReminderRequest{OpeningEventID: "event-1", Intent: TaskOpenedReminderIntent{
		OrgID: 7, TaskID: "task-1", TesteeID: "12", ScheduleRevision: 3, OpenAt: opened,
	}}
	require.NoError(t, service.ProcessTaskOpenedReminder(context.Background(), request))
	require.Equal(t, []ReminderRecipientIdentity{{UserID: "self-user", LoginIdentityID: "self-identity"}}, batches.batch.Recipients)
	require.Equal(t, ReminderManualRequired, deliveries.state)
	require.Equal(t, 1, deliveries.beginCount)
	require.Equal(t, 1, deliveries.unknownCount)
	require.Equal(t, 1, receipts.calls)
	require.NoError(t, service.ProcessTaskOpenedReminder(context.Background(), request))
	require.Equal(t, 1, receipts.calls, "unknown platform outcome must never be retried automatically")
}

func TestDurableReminderAcceptsPlatformSuccessWithoutMessageID(t *testing.T) {
	opened := time.Date(2026, 9, 26, 10, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	expires := opened.Add(24 * time.Hour)
	task := &reminderTaskReaderStub{state: &planApp.TaskReminderState{
		OrgID: 7, TaskID: "task-1", TesteeID: "12", Status: planDomain.TaskStatusOpened,
		ScheduleRevision: 3, OpenAt: &opened, ExpireAt: &expires,
		EntryURL: "https://collect.example/entry?token=secret&task_id=task-1",
	}}
	profileID := uint64(1001)
	identities := &reminderCandidateReaderStub{
		linked: []iambridge.MiniProgramLinkedUser{{UserID: "self-user", Relations: []string{"self"}}},
		candidates: []iambridge.MiniProgramRecipientCandidate{{
			UserID: "self-user", LoginIdentityID: "self-identity", AppID: "wx-app",
			OpenID: "self-openid", Relations: []string{"self"},
		}},
	}
	deliveries := &reminderDeliveryStub{}
	receipts := &receiptSenderStub{withoutMessageID: true}
	service := NewTaskOpenedReminderService(task, &testeeLookupStub{result: &testeeApp.TesteeResult{
		ID: 12, ProfileID: &profileID,
	}}, identities, &reminderBatchStub{}, deliveries, &wechatAppLookupStub{},
		&senderStub{templates: []wechatmini.SubscribeTemplate{{
			ID: "tmpl-1", Content: "{{thing5.DATA}}{{date1.DATA}}{{character_string2.DATA}}{{thing3.DATA}}",
		}}}, receipts, nil, nil,
		&Config{AppID: "wx-app", AppSecret: "wx-secret", PagePath: "pages/task/index", TaskOpenedTemplateID: "tmpl-1"},
	).(*taskOpenedReminderService)
	service.now = func() time.Time { return opened.Add(time.Minute) }
	request := TaskOpenedReminderRequest{OpeningEventID: "event-1", Intent: TaskOpenedReminderIntent{
		OrgID: 7, TaskID: "task-1", TesteeID: "12", ScheduleRevision: 3, OpenAt: opened,
	}}
	require.NoError(t, service.ProcessTaskOpenedReminder(context.Background(), request))
	require.Equal(t, ReminderConfirmed, deliveries.state)
	require.Equal(t, 1, receipts.calls)
	require.Zero(t, deliveries.unknownCount)
	require.NoError(t, service.ProcessTaskOpenedReminder(context.Background(), request))
	require.Equal(t, 1, receipts.calls, "accepted response must not cause another send")
}

func TestDurableReminderSuppressesBeforeExternalCallAfterTaskCompletes(t *testing.T) {
	opened := time.Date(2026, 9, 26, 10, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	expires := opened.Add(24 * time.Hour)
	reader := &reminderTaskReaderStub{state: &planApp.TaskReminderState{
		OrgID: 7, TaskID: "task-1", TesteeID: "12", Status: planDomain.TaskStatusCompleted,
		ScheduleRevision: 3, OpenAt: &opened, ExpireAt: &expires,
	}}
	batches := &reminderBatchStub{}
	receipts := &receiptSenderStub{}
	service := NewTaskOpenedReminderService(reader, &testeeLookupStub{}, &reminderCandidateReaderStub{}, batches,
		&reminderDeliveryStub{}, &wechatAppLookupStub{}, &senderStub{}, receipts, nil, nil,
		&Config{AppID: "wx-app", AppSecret: "wx-secret", TaskOpenedTemplateID: "tmpl-1"},
	).(*taskOpenedReminderService)
	service.now = func() time.Time { return opened.Add(time.Minute) }
	require.NoError(t, service.ProcessTaskOpenedReminder(context.Background(), TaskOpenedReminderRequest{
		OpeningEventID: "event-1", Intent: TaskOpenedReminderIntent{
			OrgID: 7, TaskID: "task-1", TesteeID: "12", ScheduleRevision: 3, OpenAt: opened,
		},
	}))
	require.True(t, batches.batch.Suppressed)
	require.Equal(t, "task_not_opened", batches.batch.ResolutionCode)
	require.Zero(t, receipts.calls)
}

func TestDurableReminderDoesNotCallPlatformWhenMarkerCrossesOneHourDeadline(t *testing.T) {
	opened := time.Date(2026, 9, 26, 10, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	expires := opened.Add(24 * time.Hour)
	reader := &reminderTaskReaderStub{state: &planApp.TaskReminderState{
		OrgID: 7, TaskID: "task-1", TesteeID: "12", Status: planDomain.TaskStatusOpened,
		ScheduleRevision: 3, OpenAt: &opened, ExpireAt: &expires,
		EntryURL: "https://collect.example/entry?token=secret&task_id=task-1",
	}}
	profileID := uint64(1001)
	identities := &reminderCandidateReaderStub{
		linked: []iambridge.MiniProgramLinkedUser{{UserID: "self-user", Relations: []string{"self"}}},
		candidates: []iambridge.MiniProgramRecipientCandidate{{
			UserID: "self-user", LoginIdentityID: "self-identity", AppID: "wx-app",
			OpenID: "self-openid", Relations: []string{"self"},
		}},
	}
	current := opened.Add(59*time.Minute + 59*time.Second)
	deliveries := &reminderDeliveryStub{onBegin: func() { current = opened.Add(time.Hour) }}
	receipts := &receiptSenderStub{}
	service := NewTaskOpenedReminderService(reader, &testeeLookupStub{result: &testeeApp.TesteeResult{
		ID: 12, ProfileID: &profileID,
	}}, identities, &reminderBatchStub{}, deliveries, &wechatAppLookupStub{},
		&senderStub{templates: []wechatmini.SubscribeTemplate{{
			ID: "tmpl-1", Content: "{{thing5.DATA}}{{date1.DATA}}{{character_string2.DATA}}{{thing3.DATA}}",
		}}}, receipts, nil, nil,
		&Config{AppID: "wx-app", AppSecret: "wx-secret", PagePath: "pages/task/index", TaskOpenedTemplateID: "tmpl-1"},
	).(*taskOpenedReminderService)
	service.now = func() time.Time { return current }
	request := TaskOpenedReminderRequest{OpeningEventID: "event-1", Intent: TaskOpenedReminderIntent{
		OrgID: 7, TaskID: "task-1", TesteeID: "12", ScheduleRevision: 3, OpenAt: opened,
	}}
	require.NoError(t, service.ProcessTaskOpenedReminder(context.Background(), request))
	require.Zero(t, receipts.calls)
	require.Equal(t, ReminderManualRequired, deliveries.state)
	require.Equal(t, "deadline_after_call_marker", deliveries.unknownCode)
	require.NoError(t, service.ProcessTaskOpenedReminder(context.Background(), request))
	require.Zero(t, receipts.calls, "a late marker must never lead to a later automatic send")
}
