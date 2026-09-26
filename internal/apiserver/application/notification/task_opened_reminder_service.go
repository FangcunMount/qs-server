package notification

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	modelcatalogApp "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	testeeDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	wechatmini "github.com/FangcunMount/qs-server/internal/apiserver/port/wechatmini"
)

const taskOpenedReminderVersion uint32 = 1

// TaskOpenedReminderRequest is the broker's immutable reference to a Task
// opening. It carries neither OpenID nor the bearer entry URL.
type TaskOpenedReminderRequest struct {
	OpeningEventID string
	Intent         TaskOpenedReminderIntent
}

type TaskOpenedReminderService interface {
	ProcessTaskOpenedReminder(context.Context, TaskOpenedReminderRequest) error
}

type taskOpenedReminderService struct {
	tasks      planApp.TaskReminderStateReader
	testees    testeeLookup
	identities iambridge.MiniProgramRecipientCandidateReader
	batches    ReminderBatchLedger
	deliveries ReminderDeliveryLedger
	app        iambridge.WeChatAppConfigProvider
	templates  wechatmini.MiniProgramSubscribeSender
	receipts   wechatmini.MiniProgramSubscribeReceiptSender
	context    planApp.TaskNotificationContextReader
	titles     modelcatalogApp.PublishedModelTitleResolver
	config     *Config
	now        func() time.Time
}

func NewTaskOpenedReminderService(
	tasks planApp.TaskReminderStateReader,
	testees testeeLookup,
	identities iambridge.MiniProgramRecipientCandidateReader,
	batches ReminderBatchLedger,
	deliveries ReminderDeliveryLedger,
	app iambridge.WeChatAppConfigProvider,
	templates wechatmini.MiniProgramSubscribeSender,
	receipts wechatmini.MiniProgramSubscribeReceiptSender,
	contextReader planApp.TaskNotificationContextReader,
	titles modelcatalogApp.PublishedModelTitleResolver,
	config *Config,
) TaskOpenedReminderService {
	return &taskOpenedReminderService{
		tasks: tasks, testees: testees, identities: identities, batches: batches,
		deliveries: deliveries, app: app, templates: templates, receipts: receipts,
		context: contextReader, titles: titles, config: config, now: time.Now,
	}
}

func (s *taskOpenedReminderService) ProcessTaskOpenedReminder(
	ctx context.Context, request TaskOpenedReminderRequest,
) error {
	if s == nil || s.tasks == nil || s.testees == nil || s.identities == nil || s.batches == nil ||
		s.deliveries == nil || s.templates == nil || s.receipts == nil || s.config == nil || s.now == nil {
		return fmt.Errorf("durable task reminder is not configured")
	}
	intent := request.Intent
	if request.OpeningEventID == "" || intent.OrgID <= 0 || intent.TaskID == "" || intent.TesteeID == "" || intent.OpenAt.IsZero() {
		return fmt.Errorf("complete task opening reminder reference is required")
	}
	base := s.renderer(s.config.TaskOpenedTemplateID)
	appID, appSecret, err := base.getWechatAppConfig(ctx)
	if err != nil {
		return err
	}
	key := ReminderBatchKey{
		OrgID: intent.OrgID, TaskID: intent.TaskID, OpeningEventID: request.OpeningEventID,
		ScheduleRevision: intent.ScheduleRevision, ReminderVersion: taskOpenedReminderVersion,
	}
	batch, found, err := s.batches.FindBatch(ctx, key)
	if err != nil {
		return err
	}
	if !found {
		batch, err = s.freezeOpening(ctx, key, intent, appID)
		if err != nil {
			return err
		}
	}
	if batch.Suppressed {
		return nil
	}
	if batch.AppID != appID {
		return fmt.Errorf("frozen reminder AppID differs from current sender configuration")
	}
	if batch.TemplateID == "" {
		return fmt.Errorf("frozen reminder template is missing")
	}
	renderer := s.renderer(batch.TemplateID)
	for _, recipient := range batch.Recipients {
		if err := s.deliverOne(ctx, intent, key, batch, recipient, appSecret, renderer); err != nil {
			return err
		}
	}
	return nil
}

func (s *taskOpenedReminderService) freezeOpening(
	ctx context.Context, key ReminderBatchKey, intent TaskOpenedReminderIntent, appID string,
) (ReminderBatch, error) {
	_, decision, err := s.currentDecision(ctx, intent)
	if err != nil {
		return ReminderBatch{}, err
	}
	if decision.SuppressCode != "" {
		return s.batches.SuppressEmpty(ctx, key, appID, s.config.TaskOpenedTemplateID, decision.SuppressCode, s.now())
	}
	testee, profileID, err := s.currentTestee(ctx, intent.TesteeID)
	if err != nil {
		return ReminderBatch{}, err
	}
	if testeeDomain.IsSeeddataMockSource(testeeDomain.Source(testee.Source)) {
		return s.batches.SuppressEmpty(ctx, key, appID, s.config.TaskOpenedTemplateID, "seeddata_mock", s.now())
	}
	if profileID == "" {
		return s.batches.SuppressEmpty(ctx, key, appID, s.config.TaskOpenedTemplateID, "profile_missing", s.now())
	}
	candidates, err := ResolveSelfRecipientCandidates(ctx, s.identities, profileID, appID)
	if err != nil {
		return ReminderBatch{}, err
	}
	if len(candidates) == 0 {
		return s.batches.SuppressEmpty(ctx, key, appID, s.config.TaskOpenedTemplateID, "no_self_identity", s.now())
	}
	// IAM may have taken time to respond. Recheck the one-hour limit and Task
	// state before freezing a sendable set.
	_, decision, err = s.currentDecision(ctx, intent)
	if err != nil {
		return ReminderBatch{}, err
	}
	if decision.SuppressCode != "" {
		return s.batches.SuppressEmpty(ctx, key, appID, s.config.TaskOpenedTemplateID, decision.SuppressCode, s.now())
	}
	recipients := make([]ReminderRecipientIdentity, 0, len(candidates))
	for _, candidate := range candidates {
		recipients = append(recipients, ReminderRecipientIdentity{
			UserID: candidate.UserID, LoginIdentityID: candidate.LoginIdentityID,
		})
	}
	return s.batches.FreezeRecipients(ctx, key, appID, s.config.TaskOpenedTemplateID, recipients, s.now())
}

func (s *taskOpenedReminderService) deliverOne(
	ctx context.Context, intent TaskOpenedReminderIntent, batchKey ReminderBatchKey,
	batch ReminderBatch, recipient ReminderRecipientIdentity, appSecret string, renderer *taskOpenedService,
) error {
	key := ReminderDeliveryKey{
		OrgID: batchKey.OrgID, TaskID: batchKey.TaskID, OpeningEventID: batchKey.OpeningEventID,
		ScheduleRevision: batchKey.ScheduleRevision, ReminderVersion: batchKey.ReminderVersion,
		LoginIdentityID: recipient.LoginIdentityID, AppID: batch.AppID, TemplateID: batch.TemplateID,
	}
	token, claimed, err := s.deliveries.Claim(ctx, key, 2*time.Minute, s.now())
	if err != nil || !claimed {
		return err
	}
	unsent := func(reason string) error {
		settleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		var settled bool
		var settleErr error
		if reason == "" {
			settled, settleErr = s.deliveries.ReleaseUnsent(settleCtx, key, token, s.now())
		} else {
			settled, settleErr = s.deliveries.SuppressUnsent(settleCtx, key, token, reason, s.now())
		}
		if settleErr == nil && !settled {
			return fmt.Errorf("reminder unsent responsibility was not settled")
		}
		return settleErr
	}
	state, decision, err := s.currentDecision(ctx, intent)
	if err != nil {
		_ = unsent("")
		return err
	}
	if decision.SuppressCode != "" {
		return unsent(decision.SuppressCode)
	}
	_, profileID, err := s.currentTestee(ctx, intent.TesteeID)
	if err != nil {
		_ = unsent("")
		return err
	}
	if profileID == "" {
		return unsent("profile_missing")
	}
	candidates, err := ResolveSelfRecipientCandidates(ctx, s.identities, profileID, batch.AppID)
	if err != nil {
		_ = unsent("")
		return err
	}
	var openID string
	for _, candidate := range candidates {
		if candidate.UserID == recipient.UserID && candidate.LoginIdentityID == recipient.LoginIdentityID {
			openID = candidate.OpenID
			break
		}
	}
	if openID == "" {
		return unsent("identity_no_longer_self")
	}
	spec, err := renderer.loadTemplateSpec(ctx, batch.AppID, appSecret, batch.TemplateID)
	if err != nil {
		_ = unsent("")
		return err
	}
	testeeID, _ := strconv.ParseUint(intent.TesteeID, 10, 64)
	dto := TaskOpenedDTO{OrgID: intent.OrgID, TaskID: intent.TaskID, TesteeID: testeeID,
		EntryURL: state.EntryURL, OpenAt: intent.OpenAt}
	message := wechatmini.SubscribeMessage{
		ToUser: openID, TemplateID: batch.TemplateID, Page: renderer.buildPagePath(state.EntryURL),
		MiniProgramState: "formal", Lang: "zh_CN",
		Data: renderer.buildTemplateData(spec, renderer.resolveTaskOpenedTemplateData(ctx, dto)),
	}
	if message.Page == "" {
		_ = unsent("")
		return fmt.Errorf("reminder mini-program page path is missing")
	}
	// Template/IAM work may have delayed the send or changed the Task. Perform
	// the final Task check immediately before crossing the external boundary.
	latest, decision, err := s.currentDecision(ctx, intent)
	if err != nil {
		_ = unsent("")
		return err
	}
	if decision.SuppressCode != "" {
		return unsent(decision.SuppressCode)
	}
	if latest.EntryURL != state.EntryURL {
		_ = unsent("")
		return fmt.Errorf("task entry changed while preparing reminder")
	}
	started, err := s.deliveries.BeginExternalCall(ctx, key, token, s.now())
	if err != nil {
		return err
	}
	if !started {
		return fmt.Errorf("reminder external call marker was not acquired")
	}
	settleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	// Persisting the call marker can itself cross the deadline. The platform
	// must not be called after that point. Keep the row for manual review: a
	// process lost around this marker cannot safely infer an unsent outcome.
	if !s.now().Before(intent.OpenAt.Add(TaskOpenedReminderWindow)) {
		marked, markErr := s.deliveries.MarkUnknown(settleCtx, key, token, "deadline_after_call_marker", s.now())
		if markErr != nil {
			return markErr
		}
		if !marked {
			return fmt.Errorf("expired reminder call marker could not be recorded")
		}
		return nil
	}
	receipt, sendErr := s.receipts.SendSubscribeMessageWithReceipt(ctx, batch.AppID, appSecret, message)
	if sendErr != nil || !receipt.Accepted {
		var marked bool
		marked, err = s.deliveries.MarkUnknown(settleCtx, key, token, "platform_result_unknown", s.now())
		if err != nil {
			return err
		}
		if !marked {
			return fmt.Errorf("unknown reminder outcome could not be recorded")
		}
		logger.L(ctx).Warnw("task reminder platform result requires manual review",
			"task_id", intent.TaskID, "opening_event_id", batchKey.OpeningEventID)
		return nil
	}
	confirmed, err := s.deliveries.Confirm(settleCtx, key, token, receipt.PlatformMessageID, s.now())
	if err != nil {
		return err
	}
	if !confirmed {
		return fmt.Errorf("reminder platform receipt could not be confirmed")
	}
	return nil
}

func (s *taskOpenedReminderService) currentDecision(
	ctx context.Context, intent TaskOpenedReminderIntent,
) (*planApp.TaskReminderState, ReminderTaskDecision, error) {
	state, err := s.tasks.GetTaskReminderState(ctx, intent.OrgID, intent.TaskID)
	if err != nil {
		return nil, ReminderTaskDecision{}, err
	}
	decision, err := EvaluateTaskOpenedReminder(intent, state, s.now())
	return state, decision, err
}

func (s *taskOpenedReminderService) currentTestee(
	ctx context.Context, testeeID string,
) (*testeeApp.TesteeResult, string, error) {
	id, err := strconv.ParseUint(testeeID, 10, 64)
	if err != nil {
		return nil, "", fmt.Errorf("invalid reminder testee ID: %w", err)
	}
	testee, err := s.testees.GetByID(ctx, id)
	if err != nil {
		return nil, "", err
	}
	if testee == nil {
		return nil, "", fmt.Errorf("reminder testee is unavailable")
	}
	if testee.ProfileID == nil {
		return testee, "", nil
	}
	return testee, strconv.FormatUint(*testee.ProfileID, 10), nil
}

func (s *taskOpenedReminderService) renderer(templateID string) *taskOpenedService {
	config := *s.config
	config.TaskOpenedTemplateID = templateID
	return &taskOpenedService{
		taskContextReader: s.context, publishedTitleResolver: s.titles,
		wechatAppService: s.app, sender: s.templates, config: &config,
	}
}
