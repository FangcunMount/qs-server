package notification

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	testeeApp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/testee"
	scaleApp "github.com/FangcunMount/qs-server/internal/apiserver/application/scale"
	domainTestee "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainPlan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	wechatmini "github.com/FangcunMount/qs-server/internal/apiserver/port/wechatmini"
)

var templateKeyPattern = regexp.MustCompile(`\{\{([a-zA-Z_]+\d+)\.DATA\}\}`)

// Config 是小程序 task 通知配置。
type Config struct {
	WeChatAppID          string
	PagePath             string
	AppID                string
	AppSecret            string
	TaskOpenedTemplateID string
}

type testeeLookup interface {
	GetByID(ctx context.Context, testeeID uint64) (*testeeApp.TesteeResult, error)
}

type taskLookup interface {
	FindByID(ctx context.Context, id domainPlan.AssessmentTaskID) (*domainPlan.AssessmentTask, error)
	FindByTesteeID(ctx context.Context, testeeID domainTestee.ID) ([]*domainPlan.AssessmentTask, error)
}

type planLookup interface {
	FindByID(ctx context.Context, id domainPlan.AssessmentPlanID) (*domainPlan.AssessmentPlan, error)
}

type scaleLookup interface {
	GetByCode(ctx context.Context, code string) (*scaleApp.ScaleResult, error)
}

type templateSpec struct {
	keys []string
}

type taskOpenedTemplateData struct {
	planName     string
	planDate     string
	planProgress string
	warmPrompt   string
}

type taskOpenedService struct {
	testeeQueryService testeeLookup
	taskRepo           taskLookup
	planRepo           planLookup
	scaleQueryService  scaleLookup
	recipientResolver  iambridge.MiniProgramRecipientResolver
	wechatAppService   iambridge.WeChatAppConfigProvider
	sender             wechatmini.MiniProgramSubscribeSender
	config             *Config

	templateCache sync.Map
}

// NewMiniProgramTaskNotificationService 创建 task.opened 小程序通知服务。
func NewMiniProgramTaskNotificationService(
	testeeQueryService testeeLookup,
	taskRepo taskLookup,
	planRepo planLookup,
	scaleQueryService scaleLookup,
	recipientResolver iambridge.MiniProgramRecipientResolver,
	wechatAppService iambridge.WeChatAppConfigProvider,
	sender wechatmini.MiniProgramSubscribeSender,
	config *Config,
) MiniProgramTaskNotificationService {
	return &taskOpenedService{
		testeeQueryService: testeeQueryService,
		taskRepo:           taskRepo,
		planRepo:           planRepo,
		scaleQueryService:  scaleQueryService,
		recipientResolver:  recipientResolver,
		wechatAppService:   wechatAppService,
		sender:             sender,
		config:             config,
	}
}

func (s *taskOpenedService) SendTaskOpened(ctx context.Context, dto TaskOpenedDTO) (*TaskOpenedResult, error) {
	l := logger.L(ctx)
	result := &TaskOpenedResult{
		TemplateID: s.taskOpenedTemplateID(),
	}
	if s == nil || s.sender == nil || s.config == nil {
		result.Skipped = true
		result.Message = "mini program notifier not configured"
		return result, nil
	}
	if result.TemplateID == "" {
		result.Skipped = true
		result.Message = "task opened template id not configured"
		return result, nil
	}

	testeeResult, err := s.testeeQueryService.GetByID(ctx, dto.TesteeID)
	if err != nil {
		return nil, fmt.Errorf("get testee: %w", err)
	}

	appID, appSecret, err := s.getWechatAppConfig(ctx)
	if err != nil {
		return nil, err
	}

	recipients, source, err := s.resolveRecipients(ctx, testeeResult)
	if err != nil {
		return nil, err
	}
	if len(recipients) == 0 {
		l.Warnw("task.opened mini program notification skipped because no recipients were resolved",
			"action", "send_task_opened_miniprogram_notification",
			"task_id", dto.TaskID,
			"testee_id", dto.TesteeID,
			"template_id", result.TemplateID,
		)
		result.Skipped = true
		result.Message = "no mini program recipients resolved"
		return result, nil
	}

	spec, err := s.loadTemplateSpec(ctx, appID, appSecret, result.TemplateID)
	if err != nil {
		return nil, err
	}
	page := s.buildPagePath(dto.EntryURL)
	data := s.buildTemplateData(spec, s.resolveTaskOpenedTemplateData(ctx, dto))

	result.RecipientOpenIDs = recipients
	result.RecipientSource = source

	l.Infow("task.opened mini program notification prepared",
		"action", "send_task_opened_miniprogram_notification",
		"task_id", dto.TaskID,
		"testee_id", dto.TesteeID,
		"template_id", result.TemplateID,
		"recipient_source", source,
		"recipient_count", len(recipients),
		"recipient_open_ids", strings.Join(recipients, ","),
		"page", page,
		"template_data", fmt.Sprintf("%v", data),
	)

	var sent int
	var sendErrs []string
	for _, openID := range recipients {
		if err := s.sender.SendSubscribeMessage(ctx, appID, appSecret, wechatmini.SubscribeMessage{
			ToUser:           openID,
			TemplateID:       result.TemplateID,
			Page:             page,
			MiniProgramState: "formal",
			Lang:             "zh_CN",
			Data:             data,
		}); err != nil {
			l.Warnw("task.opened mini program notification delivery failed",
				"action", "send_task_opened_miniprogram_notification",
				"task_id", dto.TaskID,
				"testee_id", dto.TesteeID,
				"template_id", result.TemplateID,
				"recipient_open_id", openID,
				"page", page,
				"error", err.Error(),
			)
			sendErrs = append(sendErrs, fmt.Sprintf("%s: %v", openID, err))
			continue
		}
		l.Infow("task.opened mini program notification delivered",
			"action", "send_task_opened_miniprogram_notification",
			"task_id", dto.TaskID,
			"testee_id", dto.TesteeID,
			"template_id", result.TemplateID,
			"recipient_open_id", openID,
			"page", page,
		)
		sent++
	}

	result.SentCount = sent
	if sent == 0 {
		l.Errorw("task.opened mini program notification failed for all recipients",
			"action", "send_task_opened_miniprogram_notification",
			"task_id", dto.TaskID,
			"testee_id", dto.TesteeID,
			"template_id", result.TemplateID,
			"recipient_source", source,
			"recipient_count", len(recipients),
			"recipient_open_ids", strings.Join(recipients, ","),
			"errors", strings.Join(sendErrs, "; "),
		)
		return result, fmt.Errorf("send task opened message failed: %s", strings.Join(sendErrs, "; "))
	}
	if len(sendErrs) > 0 {
		result.Message = "partial delivery: " + strings.Join(sendErrs, "; ")
		l.Warnw("task.opened mini program notification partially delivered",
			"action", "send_task_opened_miniprogram_notification",
			"task_id", dto.TaskID,
			"testee_id", dto.TesteeID,
			"template_id", result.TemplateID,
			"recipient_source", source,
			"recipient_count", len(recipients),
			"sent_count", sent,
			"recipient_open_ids", strings.Join(recipients, ","),
			"errors", strings.Join(sendErrs, "; "),
		)
		return result, nil
	}
	l.Infow("task.opened mini program notification delivered to all recipients",
		"action", "send_task_opened_miniprogram_notification",
		"task_id", dto.TaskID,
		"testee_id", dto.TesteeID,
		"template_id", result.TemplateID,
		"recipient_source", source,
		"recipient_count", len(recipients),
		"sent_count", sent,
		"recipient_open_ids", strings.Join(recipients, ","),
	)
	return result, nil
}

func (s *taskOpenedService) taskOpenedTemplateID() string {
	if s == nil || s.config == nil {
		return ""
	}
	return strings.TrimSpace(s.config.TaskOpenedTemplateID)
}

func (s *taskOpenedService) getWechatAppConfig(ctx context.Context) (appID, appSecret string, err error) {
	if s.config == nil {
		return "", "", fmt.Errorf("mini program notification config is nil")
	}
	if s.config.WeChatAppID != "" && s.wechatAppService != nil && s.wechatAppService.IsEnabled() {
		resp, err := s.wechatAppService.ResolveWeChatAppConfig(ctx, s.config.WeChatAppID)
		if err != nil {
			return "", "", fmt.Errorf("get wechat app from IAM: %w", err)
		}
		if resp == nil || resp.AppID == "" || resp.AppSecret == "" {
			return "", "", fmt.Errorf("wechat app config from IAM is incomplete")
		}
		return resp.AppID, resp.AppSecret, nil
	}
	if s.config.AppID != "" && s.config.AppSecret != "" {
		return s.config.AppID, s.config.AppSecret, nil
	}
	return "", "", fmt.Errorf("wechat mini program config is missing")
}

func (s *taskOpenedService) resolveRecipients(ctx context.Context, testeeResult *testeeApp.TesteeResult) ([]string, string, error) {
	if testeeResult == nil || testeeResult.ProfileID == nil {
		return nil, "", nil
	}
	if s.recipientResolver == nil || !s.recipientResolver.IsEnabled() {
		return nil, "", nil
	}
	recipients, err := s.recipientResolver.ResolveMiniProgramRecipients(ctx, strconv.FormatUint(*testeeResult.ProfileID, 10))
	if err != nil {
		return nil, "", err
	}
	if recipients == nil {
		return nil, "", nil
	}
	return recipients.OpenIDs, recipients.Source, nil
}

func (s *taskOpenedService) loadTemplateSpec(ctx context.Context, appID, appSecret, templateID string) (*templateSpec, error) {
	expected := s.expectedTaskOpenedTemplateSpec(templateID)
	if expected == nil {
		return nil, fmt.Errorf("unsupported task opened template id: %s", templateID)
	}

	cacheKey := appID + ":" + templateID
	if cached, ok := s.templateCache.Load(cacheKey); ok {
		if spec, ok := cached.(*templateSpec); ok {
			return spec, nil
		}
	}

	templates, err := s.sender.ListTemplates(ctx, appID, appSecret)
	if err != nil {
		return nil, fmt.Errorf("list mini program templates: %w", err)
	}
	for _, tmpl := range templates {
		if tmpl.ID != templateID {
			continue
		}
		keys := extractTemplateKeys(tmpl.Content)
		if !slices.Equal(keys, expected.keys) {
			return nil, fmt.Errorf("template %s keys mismatch: got %v want %v", templateID, keys, expected.keys)
		}
		spec := &templateSpec{keys: append([]string(nil), expected.keys...)}
		s.templateCache.Store(cacheKey, spec)
		return spec, nil
	}
	return nil, fmt.Errorf("template %s not found in mini program template list", templateID)
}

func (s *taskOpenedService) expectedTaskOpenedTemplateSpec(templateID string) *templateSpec {
	if strings.TrimSpace(templateID) == "" || strings.TrimSpace(templateID) != s.taskOpenedTemplateID() {
		return nil
	}
	return &templateSpec{
		keys: []string{"thing5", "date1", "character_string2", "thing3"},
	}
}

func extractTemplateKeys(content string) []string {
	matches := templateKeyPattern.FindAllStringSubmatch(content, -1)
	keys := make([]string, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		if len(match) < 2 || match[1] == "" {
			continue
		}
		if _, ok := seen[match[1]]; ok {
			continue
		}
		seen[match[1]] = struct{}{}
		keys = append(keys, match[1])
	}
	return keys
}

func (s *taskOpenedService) buildTemplateData(spec *templateSpec, data taskOpenedTemplateData) map[string]string {
	if spec == nil {
		return nil
	}
	return map[string]string{
		spec.keys[0]: data.planName,
		spec.keys[1]: data.planDate,
		spec.keys[2]: data.planProgress,
		spec.keys[3]: data.warmPrompt,
	}
}

func (s *taskOpenedService) resolveTaskOpenedTemplateData(ctx context.Context, dto TaskOpenedDTO) taskOpenedTemplateData {
	data := taskOpenedTemplateData{
		planName:     "测评计划",
		planDate:     formatTaskOpenedDate(dto.OpenAt),
		planProgress: "1/1",
		warmPrompt:   "请及时完成本次测评任务",
	}
	if s == nil || s.taskRepo == nil || strings.TrimSpace(dto.TaskID) == "" {
		return data
	}

	taskID, err := domainPlan.ParseAssessmentTaskID(dto.TaskID)
	if err != nil {
		logger.L(ctx).Warnw("failed to parse task id for mini program notification",
			"action", "resolve_task_opened_template_data",
			"task_id", dto.TaskID,
			"error", err.Error(),
		)
		return data
	}

	task, err := s.taskRepo.FindByID(ctx, taskID)
	if err != nil {
		logger.L(ctx).Warnw("failed to load task for mini program notification",
			"action", "resolve_task_opened_template_data",
			"task_id", dto.TaskID,
			"error", err.Error(),
		)
		return data
	}
	if task == nil {
		return data
	}

	if !task.GetPlannedAt().IsZero() {
		data.planDate = formatTaskOpenedDate(task.GetPlannedAt())
	}
	if task.GetSeq() > 0 {
		data.planProgress = strconv.Itoa(task.GetSeq())
	}

	if s.planRepo != nil {
		parentPlan, err := s.planRepo.FindByID(ctx, task.GetPlanID())
		if err != nil {
			logger.L(ctx).Warnw("failed to load plan for mini program notification",
				"action", "resolve_task_opened_template_data",
				"task_id", dto.TaskID,
				"plan_id", task.GetPlanID().String(),
				"error", err.Error(),
			)
		} else if parentPlan != nil && parentPlan.GetTotalTimes() > 0 && task.GetSeq() > 0 {
			data.planProgress = fmt.Sprintf("%d/%d", task.GetSeq(), parentPlan.GetTotalTimes())
		}
	}

	if scaleTitle := s.resolveScaleTitle(ctx, task.GetScaleCode()); scaleTitle != "" {
		data.planName = scaleTitle
	} else if code := strings.TrimSpace(task.GetScaleCode()); code != "" {
		data.planName = code
	}

	data.warmPrompt = s.buildWarmPrompt(ctx, task)
	return data
}

func (s *taskOpenedService) resolveScaleTitle(ctx context.Context, scaleCode string) string {
	if s == nil || s.scaleQueryService == nil || strings.TrimSpace(scaleCode) == "" {
		return ""
	}
	result, err := s.scaleQueryService.GetByCode(ctx, scaleCode)
	if err != nil {
		logger.L(ctx).Warnw("failed to load scale for mini program notification",
			"action", "resolve_task_opened_template_data",
			"scale_code", scaleCode,
			"error", err.Error(),
		)
		return ""
	}
	if result == nil {
		return ""
	}
	return strings.TrimSpace(result.Title)
}

func (s *taskOpenedService) buildWarmPrompt(ctx context.Context, task *domainPlan.AssessmentTask) string {
	const fallback = "请及时完成本次测评任务"

	if s == nil || s.taskRepo == nil || task == nil {
		return fallback
	}
	tasks, err := s.taskRepo.FindByTesteeID(ctx, task.GetTesteeID())
	if err != nil {
		logger.L(ctx).Warnw("failed to count unfinished tasks for mini program notification",
			"action", "resolve_task_opened_template_data",
			"task_id", task.GetID().String(),
			"testee_id", task.GetTesteeID().String(),
			"error", err.Error(),
		)
		return fallback
	}

	count := 0
	for _, item := range tasks {
		if item == nil || item.GetStatus().IsTerminal() {
			continue
		}
		if sameLocalDate(item.GetPlannedAt(), task.GetPlannedAt()) {
			count++
		}
	}
	if count <= 0 {
		return fallback
	}
	return fmt.Sprintf("今天有 %d 个任务未完成", count)
}

func formatTaskOpenedDate(openAt time.Time) string {
	if openAt.IsZero() {
		return time.Now().Local().Format("2006.01.02")
	}
	return openAt.Local().Format("2006.01.02")
}

func sameLocalDate(left, right time.Time) bool {
	if left.IsZero() || right.IsZero() {
		return false
	}
	left = left.Local()
	right = right.Local()
	return left.Year() == right.Year() && left.Month() == right.Month() && left.Day() == right.Day()
}

func (s *taskOpenedService) buildPagePath(entryURL string) string {
	pagePath := strings.TrimSpace(s.config.PagePath)
	if pagePath == "" {
		return ""
	}

	u, err := url.Parse(entryURL)
	if err != nil {
		return pagePath
	}
	q := u.Query()
	pageQuery := url.Values{}
	if token := q.Get("token"); token != "" {
		pageQuery.Set("token", token)
	}
	if taskID := q.Get("task_id"); taskID != "" {
		pageQuery.Set("task_id", taskID)
	}
	if len(pageQuery) == 0 {
		return pagePath
	}
	return pagePath + "?" + pageQuery.Encode()
}
