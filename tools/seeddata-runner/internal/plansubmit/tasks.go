package plansubmit

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"
)

const (
	planOpenTaskPageSize             = 100
	planOpenTaskSubmitRequestTimeout = 15 * time.Second
	planOpenTaskSubmitHTTPRetryMax   = 0
	planOpenTaskSubmitMaxAttempts    = 2
	planOpenTaskSubmitRetryBackoff   = 2 * time.Second
	planOpenTaskRecentSubmitTTL      = 10 * time.Minute
)

type planTaskJob struct {
	testeeID string
	task     TaskResponse
}

type planOpenTaskSubmitStats struct {
	OpenedCount          int
	SubmittedCount       int
	SkippedCount         int
	FailedTaskListLoads  int
	FailedTaskExecutions int
}

type planTaskLogger interface {
	Warnw(string, ...interface{})
	Debugw(string, ...interface{})
	Infow(string, ...interface{})
}

type recentPlanTaskTracker struct {
	mu          sync.Mutex
	ttl         time.Duration
	submittedAt map[string]time.Time
}

func newRecentPlanTaskTracker(ttl time.Duration) *recentPlanTaskTracker {
	if ttl <= 0 {
		ttl = planOpenTaskRecentSubmitTTL
	}
	return &recentPlanTaskTracker{
		ttl:         ttl,
		submittedAt: make(map[string]time.Time),
	}
}

func (t *recentPlanTaskTracker) Seen(taskID string) bool {
	if t == nil {
		return false
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return false
	}

	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pruneLocked(now)

	until, ok := t.submittedAt[taskID]
	return ok && now.Before(until)
}

func (t *recentPlanTaskTracker) Remember(taskID string) {
	if t == nil {
		return
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return
	}

	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pruneLocked(now)
	t.submittedAt[taskID] = now.Add(t.ttl)
}

func (t *recentPlanTaskTracker) pruneLocked(now time.Time) {
	for taskID, expiresAt := range t.submittedAt {
		if !now.Before(expiresAt) {
			delete(t.submittedAt, taskID)
		}
	}
}

func listOpenPlanTaskJobs(
	ctx context.Context,
	gateway planTaskSubmitGateway,
	logger planTaskLogger,
	planID string,
	verbose bool,
) ([]planTaskJob, error) {
	planID = normalizePlanID(planID)
	if planID == "" {
		return nil, fmt.Errorf("plan-id is required")
	}

	resourceID := planID
	jobs := make([]planTaskJob, 0, planOpenTaskPageSize)
	for page := 1; ; page++ {
		req := ListPlanTaskWindowRequest{
			PlanID:   planID,
			Status:   "opened",
			Page:     page,
			PageSize: planOpenTaskPageSize,
		}

		var windowResp *PlanTaskWindowResponse
		err := runSeedPlanOperationWithRecovery(ctx, logger, verbose, "list_opened_plan_tasks", resourceID, func() error {
			resp, err := gateway.ListPlanTaskWindow(ctx, req)
			if err != nil {
				return err
			}
			windowResp = resp
			return nil
		})
		if err != nil {
			return nil, err
		}
		if windowResp == nil {
			windowResp = &PlanTaskWindowResponse{}
		}

		jobs = appendOpenPlanTaskJobs(jobs, windowResp.Tasks, planID, logger, verbose)
		if !hasMorePlanTaskWindow(windowResp, req.Page, req.PageSize) {
			return jobs, nil
		}
	}
}

func appendOpenPlanTaskJobs(
	jobs []planTaskJob,
	tasks []TaskResponse,
	planID string,
	logger planTaskLogger,
	verbose bool,
) []planTaskJob {
	sortedTasks := append([]TaskResponse(nil), tasks...)
	sortTasksBySeq(sortedTasks)

	for _, task := range sortedTasks {
		taskTesteeID := strings.TrimSpace(task.TesteeID)
		switch normalizeTaskStatus(task.Status) {
		case "opened":
			if taskTesteeID == "" {
				logger.Warnw("Skipping opened task without testee_id",
					"plan_id", planID,
					"task_id", task.ID,
				)
				continue
			}
			jobs = append(jobs, planTaskJob{
				testeeID: taskTesteeID,
				task:     task,
			})
		default:
			if verbose {
				logger.Debugw("Skipping non-open plan task while scanning daemon backlog",
					"plan_id", planID,
					"task_id", task.ID,
					"status", task.Status,
				)
			}
		}
	}
	return jobs
}

func hasMorePlanTaskWindow(taskList *PlanTaskWindowResponse, page int, pageSize int) bool {
	if taskList == nil {
		return false
	}
	if taskList.HasMore {
		return true
	}
	if taskList.Page > 0 {
		page = taskList.Page
	}
	if taskList.PageSize > 0 {
		pageSize = taskList.PageSize
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = len(taskList.Tasks)
	}
	return pageSize > 0 && len(taskList.Tasks) >= pageSize
}

func normalizeTaskStatus(status string) string {
	return strings.ToLower(strings.TrimSpace(status))
}

func buildPlanTaskSubmitRequest(
	detail *QuestionnaireDetailResponse,
	questionnaireVersion string,
	task TaskResponse,
	verbose bool,
	logger planTaskLogger,
) (*SubmitAnswerSheetRequest, error) {
	if detail == nil {
		return nil, fmt.Errorf("questionnaire detail is nil")
	}
	if strings.TrimSpace(questionnaireVersion) == "" {
		return nil, fmt.Errorf("questionnaire version is empty")
	}
	if strings.TrimSpace(detail.Version) != questionnaireVersion {
		return nil, fmt.Errorf(
			"questionnaire version mismatch while building plan answersheet: questionnaire_code=%s expected=%s loaded=%s; retry after refreshing the scale/questionnaire cache path",
			detail.Code,
			questionnaireVersion,
			detail.Version,
		)
	}

	testeeID := strings.TrimSpace(task.TesteeID)
	if testeeID == "" {
		return nil, fmt.Errorf("task %s has empty testee_id", task.ID)
	}

	rngSeed := time.Now().UnixNano()
	rngSeed += int64(parseID(testeeID))
	rngSeed += int64(parseID(task.ID))
	rng := rand.New(rand.NewSource(rngSeed))
	answers := buildAnswers(detail, rng)
	if len(answers) == 0 {
		return nil, fmt.Errorf(
			"no supported answers generated for questionnaire %s, question_types=%v",
			detail.Code,
			collectQuestionTypes(detail),
		)
	}
	if verbose {
		logBuiltAnswers(logger, answers, detail.Code, testeeID)
	}

	invalidAnswers := validateAnswers(detail, answers)
	if len(invalidAnswers) > 0 {
		logger.Warnw("Invalid answers detected for plan task submission",
			"testee_id", testeeID,
			"task_id", task.ID,
			"questionnaire_code", detail.Code,
			"invalid_count", len(invalidAnswers),
			"invalid_answers", invalidAnswers,
		)
	}

	testeeIDUint := parseID(testeeID)
	if testeeIDUint == 0 {
		return nil, fmt.Errorf("invalid testee id: %s", testeeID)
	}

	return &SubmitAnswerSheetRequest{
		QuestionnaireCode:    detail.Code,
		QuestionnaireVersion: questionnaireVersion,
		Title:                detail.Title,
		TesteeID:             testeeIDUint,
		TaskID:               task.ID,
		Answers:              answers,
	}, nil
}

func submitPlanTaskAnswerSheet(
	ctx context.Context,
	client adminAnswerSheetSubmitClient,
	req SubmitAnswerSheetRequest,
) (int, error) {
	return submitAdminAnswerSheet(ctx, client, req, adminAnswerSheetSubmitPolicy{
		Timeout:      planOpenTaskSubmitRequestTimeout,
		HTTPRetryMax: planOpenTaskSubmitHTTPRetryMax,
		MaxAttempts:  planOpenTaskSubmitMaxAttempts,
		RetryBackoff: planOpenTaskSubmitRetryBackoff,
		Retryable:    isSeedPlanRecoverableError,
	})
}
