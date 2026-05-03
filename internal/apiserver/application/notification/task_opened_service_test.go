package notification

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	scaleApp "github.com/FangcunMount/qs-server/internal/apiserver/application/scale"
	testeeDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainPlan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	wechatmini "github.com/FangcunMount/qs-server/internal/apiserver/port/wechatmini"
)

type testeeLookupStub struct {
	result *testeeApp.TesteeResult
	err    error
}

func (s *testeeLookupStub) GetByID(context.Context, uint64) (*testeeApp.TesteeResult, error) {
	return s.result, s.err
}

type scaleLookupStub struct {
	result *scaleApp.ScaleResult
	err    error
}

func (s *scaleLookupStub) GetByCode(context.Context, string) (*scaleApp.ScaleResult, error) {
	return s.result, s.err
}

type recipientResolverStub struct {
	enabled    bool
	recipients *iambridge.MiniProgramRecipients
	err        error
	callCount  int
	childID    string
}

func (s *recipientResolverStub) IsEnabled() bool { return s.enabled }
func (s *recipientResolverStub) ResolveMiniProgramRecipients(_ context.Context, childID string) (*iambridge.MiniProgramRecipients, error) {
	s.callCount++
	s.childID = childID
	return s.recipients, s.err
}

type wechatAppLookupStub struct{}

func (s *wechatAppLookupStub) IsEnabled() bool { return false }
func (s *wechatAppLookupStub) ResolveWeChatAppConfig(context.Context, string) (*iambridge.WeChatAppConfig, error) {
	return nil, fmt.Errorf("disabled")
}

type senderStub struct {
	templates []wechatmini.SubscribeTemplate
	sent      []wechatmini.SubscribeMessage
}

func (s *senderStub) SendSubscribeMessage(_ context.Context, appID, appSecret string, msg wechatmini.SubscribeMessage) error {
	if appID == "" || appSecret == "" {
		return fmt.Errorf("missing app config")
	}
	s.sent = append(s.sent, msg)
	return nil
}

func (s *senderStub) ListTemplates(context.Context, string, string) ([]wechatmini.SubscribeTemplate, error) {
	return s.templates, nil
}

func buildTaskOpenedFixture(t *testing.T, testeeID uint64, seq int, totalTimes int) (*domainPlan.AssessmentPlan, *domainPlan.AssessmentTask, []*domainPlan.AssessmentTask) {
	t.Helper()

	planAggregate, err := domainPlan.NewAssessmentPlan(1, "scale-code", domainPlan.PlanScheduleByWeek, 1, totalTimes)
	if err != nil {
		t.Fatalf("NewAssessmentPlan returned error: %v", err)
	}

	plannedAt := time.Date(2026, 4, 8, 0, 0, 0, 0, time.Local)
	currentTask := domainPlan.NewAssessmentTask(planAggregate.GetID(), seq, 1, testeeDomain.NewID(testeeID), "scale-code", plannedAt)
	openAt := time.Date(2026, 4, 3, 10, 30, 0, 0, time.Local)
	expireAt := openAt.Add(24 * time.Hour)
	currentTask.RestoreFromRepository(currentTask.GetID(), domainPlan.TaskStatusOpened, &openAt, &expireAt, nil, nil, "entry-token", "https://collect.example.com/entry?token=abc123&task_id="+currentTask.GetID().String())

	peerTask := domainPlan.NewAssessmentTask(planAggregate.GetID(), seq+1, 1, testeeDomain.NewID(testeeID), "scale-code", plannedAt)
	peerOpenAt := openAt.Add(5 * time.Minute)
	peerExpireAt := peerOpenAt.Add(24 * time.Hour)
	peerTask.RestoreFromRepository(peerTask.GetID(), domainPlan.TaskStatusOpened, &peerOpenAt, &peerExpireAt, nil, nil, "entry-token-2", "")

	return planAggregate, currentTask, []*domainPlan.AssessmentTask{currentTask, peerTask}
}

type taskNotificationContextReaderStub struct {
	result *planApp.TaskNotificationContext
	err    error
}

func (s *taskNotificationContextReaderStub) GetTaskNotificationContext(context.Context, string) (*planApp.TaskNotificationContext, error) {
	return s.result, s.err
}

func notificationContextFromFixture(planAggregate *domainPlan.AssessmentPlan, task *domainPlan.AssessmentTask, tasks []*domainPlan.AssessmentTask) *planApp.TaskNotificationContext {
	count := 0
	for _, item := range tasks {
		if item == nil || item.GetStatus().IsTerminal() {
			continue
		}
		if item.GetPlannedAt().Equal(task.GetPlannedAt()) {
			count++
		}
	}
	return &planApp.TaskNotificationContext{
		TaskID:                     task.GetID().String(),
		PlanID:                     task.GetPlanID().String(),
		ScaleCode:                  task.GetScaleCode(),
		PlannedAt:                  task.GetPlannedAt(),
		Seq:                        task.GetSeq(),
		TotalTimes:                 planAggregate.GetTotalTimes(),
		UnfinishedSameDayTaskCount: count,
	}
}

func TestSendTaskOpenedFallsBackToGuardians(t *testing.T) {
	profileID := uint64(1001)
	planAggregate, task, tasks := buildTaskOpenedFixture(t, 12, 2, 4)
	resolver := &recipientResolverStub{
		enabled: true,
		recipients: &iambridge.MiniProgramRecipients{
			OpenIDs: []string{"openid-guardian"},
			Source:  "guardian",
		},
	}
	sender := &senderStub{
		templates: []wechatmini.SubscribeTemplate{
			{
				ID:      "tmpl-1",
				Content: "计划名称\n{{thing5.DATA}}\n计划时间\n{{date1.DATA}}\n计划进展\n{{character_string2.DATA}}\n温馨提示\n{{thing3.DATA}}",
			},
		},
	}

	service := NewMiniProgramTaskNotificationService(
		&testeeLookupStub{result: &testeeApp.TesteeResult{
			ID:        12,
			ProfileID: &profileID,
			Name:      "张三",
		}},
		&taskNotificationContextReaderStub{result: notificationContextFromFixture(planAggregate, task, tasks)},
		&scaleLookupStub{result: &scaleApp.ScaleResult{Code: "scale-code", Title: "儿童抑郁量表"}},
		resolver,
		&wechatAppLookupStub{},
		sender,
		&Config{
			PagePath:             "pages/questionnaire/index",
			AppID:                "wx-app",
			AppSecret:            "wx-secret",
			TaskOpenedTemplateID: "tmpl-1",
		},
	)

	result, err := service.SendTaskOpened(context.Background(), TaskOpenedDTO{
		TaskID:   task.GetID().String(),
		TesteeID: 12,
		EntryURL: "https://collect.example.com/entry?token=abc123&task_id=" + task.GetID().String(),
		OpenAt:   time.Date(2026, 4, 3, 10, 30, 0, 0, time.Local),
	})
	if err != nil {
		t.Fatalf("SendTaskOpened returned error: %v", err)
	}
	if result.SentCount != 1 || result.RecipientSource != "guardian" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if resolver.callCount != 1 || resolver.childID != "1001" {
		t.Fatalf("expected recipient resolver call, got callCount=%d childID=%q", resolver.callCount, resolver.childID)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("expected one sent message, got %d", len(sender.sent))
	}
	if sender.sent[0].ToUser != "openid-guardian" {
		t.Fatalf("unexpected recipient: %#v", sender.sent[0])
	}
	if got := sender.sent[0].Page; !strings.HasPrefix(got, "pages/questionnaire/index?") || !strings.Contains(got, "token=abc123") || !strings.Contains(got, "task_id="+task.GetID().String()) {
		t.Fatalf("unexpected page path: %s", got)
	}
	if sender.sent[0].Data["thing5"] != "儿童抑郁量表" ||
		sender.sent[0].Data["date1"] != "2026.04.08" ||
		sender.sent[0].Data["character_string2"] != "2/4" ||
		sender.sent[0].Data["thing3"] != "今天有 2 个任务未完成" {
		t.Fatalf("expected template data to be populated: %#v", sender.sent[0].Data)
	}
}

func TestSendTaskOpenedPrefersDirectTesteeUser(t *testing.T) {
	profileID := uint64(2002)
	planAggregate, task, tasks := buildTaskOpenedFixture(t, 22, 1, 3)
	resolver := &recipientResolverStub{
		enabled: true,
		recipients: &iambridge.MiniProgramRecipients{
			OpenIDs: []string{"openid-testee"},
			Source:  "testee",
		},
	}
	sender := &senderStub{
		templates: []wechatmini.SubscribeTemplate{
			{
				ID:      "tmpl-1",
				Content: "计划名称\n{{thing5.DATA}}\n计划时间\n{{date1.DATA}}\n计划进展\n{{character_string2.DATA}}\n温馨提示\n{{thing3.DATA}}",
			},
		},
	}

	service := NewMiniProgramTaskNotificationService(
		&testeeLookupStub{result: &testeeApp.TesteeResult{
			ID:        22,
			ProfileID: &profileID,
			Name:      "李四",
		}},
		&taskNotificationContextReaderStub{result: notificationContextFromFixture(planAggregate, task, tasks[:1])},
		&scaleLookupStub{result: &scaleApp.ScaleResult{Code: "scale-code", Title: "执行功能测评"}},
		resolver,
		&wechatAppLookupStub{},
		sender,
		&Config{
			PagePath:             "pages/questionnaire/index",
			AppID:                "wx-app",
			AppSecret:            "wx-secret",
			TaskOpenedTemplateID: "tmpl-1",
		},
	)

	result, err := service.SendTaskOpened(context.Background(), TaskOpenedDTO{
		TaskID:   task.GetID().String(),
		TesteeID: 22,
	})
	if err != nil {
		t.Fatalf("SendTaskOpened returned error: %v", err)
	}
	if result.RecipientSource != "testee" || result.SentCount != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if resolver.callCount != 1 || resolver.childID != "2002" {
		t.Fatalf("expected recipient resolver call, got callCount=%d childID=%q", resolver.callCount, resolver.childID)
	}
	if len(sender.sent) != 1 || sender.sent[0].ToUser != "openid-testee" {
		t.Fatalf("unexpected sent payload: %#v", sender.sent)
	}
	if sender.sent[0].Data["thing5"] != "执行功能测评" ||
		sender.sent[0].Data["date1"] != "2026.04.08" ||
		sender.sent[0].Data["character_string2"] != "1/3" ||
		sender.sent[0].Data["thing3"] != "今天有 1 个任务未完成" {
		t.Fatalf("unexpected template data: %#v", sender.sent[0].Data)
	}
}

func TestSendTaskOpenedFailsWhenTemplateKeysMismatch(t *testing.T) {
	profileID := uint64(3003)
	planAggregate, task, tasks := buildTaskOpenedFixture(t, 33, 1, 2)
	sender := &senderStub{
		templates: []wechatmini.SubscribeTemplate{
			{
				ID:      "tmpl-1",
				Content: "{{thing5.DATA}}\n{{date1.DATA}}\n{{thing3.DATA}}",
			},
		},
	}

	service := NewMiniProgramTaskNotificationService(
		&testeeLookupStub{result: &testeeApp.TesteeResult{
			ID:        33,
			ProfileID: &profileID,
			Name:      "王五",
		}},
		&taskNotificationContextReaderStub{result: notificationContextFromFixture(planAggregate, task, tasks)},
		&scaleLookupStub{result: &scaleApp.ScaleResult{Code: "scale-code", Title: "儿童抑郁量表"}},
		&recipientResolverStub{
			enabled: true,
			recipients: &iambridge.MiniProgramRecipients{
				OpenIDs: []string{"openid-testee"},
				Source:  "testee",
			},
		},
		&wechatAppLookupStub{},
		sender,
		&Config{
			PagePath:             "pages/questionnaire/index",
			AppID:                "wx-app",
			AppSecret:            "wx-secret",
			TaskOpenedTemplateID: "tmpl-1",
		},
	)

	_, err := service.SendTaskOpened(context.Background(), TaskOpenedDTO{
		TaskID:   task.GetID().String(),
		TesteeID: 33,
		OpenAt:   time.Date(2026, 4, 3, 10, 30, 0, 0, time.Local),
	})
	if err == nil {
		t.Fatalf("expected template key mismatch error")
	}
	if !strings.Contains(err.Error(), "keys mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}
