package notification

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	planDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	wechatmini "github.com/FangcunMount/qs-server/internal/apiserver/port/wechatmini"
	"github.com/stretchr/testify/require"
)

type reminderTaskReaderStub struct {
	state *planApp.TaskReminderState
	err   error
}

func (s *reminderTaskReaderStub) GetTaskReminderState(context.Context, int64, string) (*planApp.TaskReminderState, error) {
	if s.err != nil {
		return nil, s.err
	}
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
func (s *reminderDeliveryStub) Reject(_ context.Context, _ ReminderDeliveryKey, _ string, code int64, _ time.Time) (bool, error) {
	s.state = ReminderRejected
	s.unknownCode = fmt.Sprintf("platform_rejected_%d", code)
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
	calls             int
	err               error
	withoutMessageID  bool
	platformErrorCode int64
}

func (s *receiptSenderStub) SendSubscribeMessageWithReceipt(_ context.Context, _, _ string, _ wechatmini.SubscribeMessage) (wechatmini.SubscribeSendReceipt, error) {
	s.calls++
	if s.err != nil {
		return wechatmini.SubscribeSendReceipt{}, s.err
	}
	if s.platformErrorCode != 0 {
		return wechatmini.SubscribeSendReceipt{PlatformErrorCode: s.platformErrorCode}, nil
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
		ID: 12, OrgID: 7, ProfileID: &profileID,
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
		ID: 12, OrgID: 7, ProfileID: &profileID,
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
		ID: 12, OrgID: 7, ProfileID: &profileID,
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

func TestDurableReminderRejectsTesteeFromAnotherOrganization(t *testing.T) {
	service := &taskOpenedReminderService{testees: &testeeLookupStub{result: &testeeApp.TesteeResult{
		ID: 12, OrgID: 8,
	}}}
	_, _, err := service.currentTestee(context.Background(), 7, "12")
	require.ErrorContains(t, err, "opening organization")
}

func TestDurableReminderDoesNotCallPlatformWhenTaskChangesDuringCallMarker(t *testing.T) {
	for _, tc := range []struct {
		name     string
		change   func(*reminderTaskReaderStub)
		wantCode string
	}{
		{"completed", func(reader *reminderTaskReaderStub) {
			reader.state.Status = planDomain.TaskStatusCompleted
		}, "task_changed_after_call_marker"},
		{"unreadable", func(reader *reminderTaskReaderStub) {
			reader.err = errors.New("task read unavailable")
		}, "task_state_unknown_after_call_marker"},
	} {
		t.Run(tc.name, func(t *testing.T) {
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
			deliveries := &reminderDeliveryStub{onBegin: func() { tc.change(reader) }}
			receipts := &receiptSenderStub{}
			service := NewTaskOpenedReminderService(reader, &testeeLookupStub{result: &testeeApp.TesteeResult{
				ID: 12, OrgID: 7, ProfileID: &profileID,
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
			require.Zero(t, receipts.calls, "a Task change must not start an external send")
			require.Equal(t, ReminderManualRequired, deliveries.state)
			require.Equal(t, tc.wantCode, deliveries.unknownCode)
			require.NoError(t, service.ProcessTaskOpenedReminder(context.Background(), request))
			require.Zero(t, receipts.calls, "a marked unknown result must not be sent automatically")
		})
	}
}

func TestDurableReminderClassifiesSuccessRejectionAndUnknownWithoutResend(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		code   int64
		want   ReminderDeliveryState
		reason string
	}{
		{name: "accepted", want: ReminderConfirmed},
		{name: "explicit_rejection", code: 43101, want: ReminderRejected, reason: "platform_rejected_43101"},
		{name: "lost_reply", err: errors.New("response lost"), want: ReminderManualRequired, reason: "platform_result_unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opened := time.Now().Add(-time.Minute).Round(time.Millisecond)
			expires := opened.Add(24 * time.Hour)
			reader := &reminderTaskReaderStub{state: &planApp.TaskReminderState{OrgID: 7, TaskID: "task-1", TesteeID: "12", Status: planDomain.TaskStatusOpened, ScheduleRevision: 3, OpenAt: &opened, ExpireAt: &expires, EntryURL: "/pages/task/index?task_id=task-1"}}
			profile := uint64(1001)
			identities := &reminderCandidateReaderStub{linked: []iambridge.MiniProgramLinkedUser{{UserID: "self-user", Relations: []string{"self"}}}, candidates: []iambridge.MiniProgramRecipientCandidate{{UserID: "self-user", LoginIdentityID: "self-identity", AppID: "wx-app", OpenID: "fixture-openid", Relations: []string{"self"}}}}
			ledger := &reminderDeliveryStub{}
			sender := &receiptSenderStub{err: tc.err, platformErrorCode: tc.code}
			service := NewTaskOpenedReminderService(reader, &testeeLookupStub{result: &testeeApp.TesteeResult{ID: 12, OrgID: 7, ProfileID: &profile}}, identities, &reminderBatchStub{}, ledger, nil, &senderStub{templates: []wechatmini.SubscribeTemplate{{ID: "tmpl-1", Content: "{{thing5.DATA}}{{date1.DATA}}{{character_string2.DATA}}{{thing3.DATA}}"}}}, sender, nil, nil, &Config{AppID: "wx-app", AppSecret: "fixture-only", TaskOpenedTemplateID: "tmpl-1", PagePath: "pages/task/index"})
			req := TaskOpenedReminderRequest{OpeningEventID: "original-event", Intent: TaskOpenedReminderIntent{OrgID: 7, TaskID: "task-1", TesteeID: "12", ScheduleRevision: 3, OpenAt: opened}}
			require.NoError(t, service.ProcessTaskOpenedReminder(t.Context(), req))
			require.Equal(t, tc.want, ledger.state)
			require.Equal(t, tc.reason, ledger.unknownCode)
			require.NoError(t, service.ProcessTaskOpenedReminder(t.Context(), req))
			require.Equal(t, 1, sender.calls)
			require.Equal(t, 1, ledger.beginCount)
		})
	}
}

// Template lookup is an external dependency and can outlive the first IAM
// eligibility read. Recheck the frozen identity before persisting a send marker.
type recipientChangeDuringTemplateLookup struct {
	*senderStub
	change func()
}

func (s recipientChangeDuringTemplateLookup) ListTemplates(ctx context.Context, appID, secret string) ([]wechatmini.SubscribeTemplate, error) {
	s.change()
	return s.senderStub.ListTemplates(ctx, appID, secret)
}

func TestDurableReminderRechecksFrozenRecipientAfterTemplatePreparation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		change  func(*reminderCandidateReaderStub)
		want    ReminderDeliveryState
		wantErr bool
	}{
		{"self_revoked", func(r *reminderCandidateReaderStub) { r.linked = nil }, ReminderSuppressed, false},
		{"identity_disabled", func(r *reminderCandidateReaderStub) { r.candidates = nil }, ReminderSuppressed, false},
		{"iam_unavailable", func(r *reminderCandidateReaderStub) { r.readErr = errors.New("IAM temporarily unavailable") }, ReminderPending, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opened := time.Now().Add(-time.Minute).Round(time.Millisecond)
			expires := opened.Add(24 * time.Hour)
			task := &reminderTaskReaderStub{state: &planApp.TaskReminderState{OrgID: 7, TaskID: "task-1", TesteeID: "12", Status: planDomain.TaskStatusOpened, ScheduleRevision: 3, OpenAt: &opened, ExpireAt: &expires, EntryURL: "/pages/task/index?task_id=task-1"}}
			profile := uint64(1001)
			identities := &reminderCandidateReaderStub{linked: []iambridge.MiniProgramLinkedUser{{UserID: "self-user", Relations: []string{"self"}}}, candidates: []iambridge.MiniProgramRecipientCandidate{{UserID: "self-user", LoginIdentityID: "self-identity", AppID: "wx-app", OpenID: "fixture-openid", Relations: []string{"self"}}}}
			batch := &reminderBatchStub{}
			ledger := &reminderDeliveryStub{}
			receipts := &receiptSenderStub{}
			changed := false
			templates := recipientChangeDuringTemplateLookup{senderStub: &senderStub{templates: []wechatmini.SubscribeTemplate{{ID: "tmpl-1", Content: "{{thing5.DATA}}{{date1.DATA}}{{character_string2.DATA}}{{thing3.DATA}}"}}}, change: func() {
				if !changed {
					changed = true
					tc.change(identities)
				}
			}}
			service := NewTaskOpenedReminderService(task, &testeeLookupStub{result: &testeeApp.TesteeResult{ID: 12, OrgID: 7, ProfileID: &profile}}, identities, batch, ledger, nil, templates, receipts, nil, nil, &Config{AppID: "wx-app", AppSecret: "fixture-only", PagePath: "pages/task/index", TaskOpenedTemplateID: "tmpl-1"})
			request := TaskOpenedReminderRequest{OpeningEventID: "original-event", Intent: TaskOpenedReminderIntent{OrgID: 7, TaskID: "task-1", TesteeID: "12", ScheduleRevision: 3, OpenAt: opened}}
			err := service.ProcessTaskOpenedReminder(t.Context(), request)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Zero(t, receipts.calls, "stale or unavailable IAM qualification must not send")
			require.Zero(t, ledger.beginCount, "no external-call marker before final eligibility")
			require.Equal(t, tc.want, ledger.state)
			require.Equal(t, []ReminderRecipientIdentity{{UserID: "self-user", LoginIdentityID: "self-identity"}}, batch.batch.Recipients, "do not expand or rewrite the frozen recipient set")
			if tc.wantErr {
				identities.readErr = nil
				identities.linked = append(identities.linked, iambridge.MiniProgramLinkedUser{UserID: "new-user", Relations: []string{"self"}})
				identities.candidates = append(identities.candidates, iambridge.MiniProgramRecipientCandidate{UserID: "new-user", LoginIdentityID: "new-identity", AppID: "wx-app", OpenID: "new-fixture-openid", Relations: []string{"self"}})
				require.NoError(t, service.ProcessTaskOpenedReminder(t.Context(), request))
				require.Equal(t, 1, receipts.calls, "only the original frozen recipient resumes after IAM recovery")
				require.Equal(t, ReminderConfirmed, ledger.state)
			}

		})
	}
}
