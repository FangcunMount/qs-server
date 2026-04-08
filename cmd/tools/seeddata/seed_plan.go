package main

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	"golang.org/x/sync/errgroup"
)

const (
	defaultPlanID              = "614186929759466030"
	planEnrollmentSampleRate   = 5
	planTaskCompletionTimeout  = 5 * time.Minute
	planTaskCompletionInterval = 2 * time.Second
)

func seedPlanBackfill(
	ctx context.Context,
	deps *dependencies,
	_ *seedContext,
	planID string,
	planTesteeIDsRaw string,
	planWorkers int,
	testeePageSize, testeeOffset, testeeLimit int,
	verbose bool,
) error {
	if deps == nil {
		return fmt.Errorf("dependencies are nil")
	}
	if deps.APIClient == nil {
		return fmt.Errorf("api client is not initialized")
	}
	if deps.CollectionClient == nil {
		return fmt.Errorf("collection client is not initialized")
	}
	orgID := deps.Config.Global.OrgID
	if orgID <= 0 {
		return fmt.Errorf("global.orgId must be set in seeddata config")
	}
	planID = strings.TrimSpace(planID)
	if planID == "" {
		planID = defaultPlanID
	}

	logger := deps.Logger
	logger.Infow("Plan backfill started",
		"plan_id", planID,
		"org_id", orgID,
		"plan_workers", planWorkers,
		"testee_page_size", testeePageSize,
		"testee_offset", testeeOffset,
		"testee_limit", testeeLimit,
		"verbose", verbose,
	)

	prewarmAPIToken(ctx, deps.APIClient, orgID, logger)

	planResp, err := deps.APIClient.GetPlan(ctx, planID)
	if err != nil {
		return fmt.Errorf("load plan %s: %w", planID, err)
	}
	if planResp == nil {
		return fmt.Errorf("plan %s not found", planID)
	}
	if planResp.OrgID != orgID {
		return fmt.Errorf("plan %s does not belong to org %d", planID, orgID)
	}
	if normalizeTaskStatus(planResp.Status) != "active" {
		return fmt.Errorf("plan %s is not active, current status=%s", planID, planResp.Status)
	}
	if strings.TrimSpace(planResp.ScaleCode) == "" {
		return fmt.Errorf("plan %s has empty scale_code", planID)
	}

	scaleResp, err := deps.CollectionClient.GetScale(ctx, planResp.ScaleCode)
	if err != nil {
		return fmt.Errorf("load scale %s: %w", planResp.ScaleCode, err)
	}
	if scaleResp == nil {
		return fmt.Errorf("scale %s not found", planResp.ScaleCode)
	}
	if strings.TrimSpace(scaleResp.QuestionnaireCode) == "" {
		return fmt.Errorf("scale %s has empty questionnaire_code", planResp.ScaleCode)
	}
	if strings.TrimSpace(scaleResp.QuestionnaireVersion) == "" {
		return fmt.Errorf("scale %s has empty questionnaire_version", planResp.ScaleCode)
	}

	questionnaireCache := make(map[string]*QuestionnaireDetailResponse)
	var questionnaireCacheMu sync.RWMutex
	detail := getQuestionnaireDetail(ctx, deps.CollectionClient, scaleResp.QuestionnaireCode, questionnaireCache, &questionnaireCacheMu, logger)
	if detail == nil {
		return fmt.Errorf("load questionnaire %s failed", scaleResp.QuestionnaireCode)
	}
	if strings.TrimSpace(detail.Version) != scaleResp.QuestionnaireVersion {
		return newPlanQuestionnaireVersionMismatchError(
			planResp.ScaleCode,
			scaleResp.QuestionnaireCode,
			scaleResp.QuestionnaireVersion,
			detail.Version,
		)
	}
	if verbose {
		debugLogQuestionnaire(detail, logger)
	}

	explicitPlanTesteeIDs := parsePlanTesteeIDs(planTesteeIDsRaw)

	var (
		testees         []*TesteeResponse
		selectedTestees []*TesteeResponse
		selectionMode   string
		loadedTesteeCnt int
	)
	if len(explicitPlanTesteeIDs) > 0 {
		testees, err = loadExplicitPlanTestees(ctx, deps.APIClient, explicitPlanTesteeIDs)
		if err != nil {
			return err
		}
		selectedTestees = testees
		selectionMode = "explicit"
		loadedTesteeCnt = len(testees)
	} else {
		pageSize := testeePageSize
		if pageSize < 100 {
			pageSize = 100
		}
		selectedTestees, loadedTesteeCnt, err = streamSamplePlanEnrollmentTestees(
			ctx,
			deps.APIClient,
			orgID,
			pageSize,
			testeeOffset,
			testeeLimit,
			planID,
		)
		if err != nil {
			return err
		}
		testees = selectedTestees
		selectionMode = "sample"
	}
	logger.Infow("Loaded testees for plan backfill",
		"plan_id", planID,
		"org_id", orgID,
		"loaded_testee_count", loadedTesteeCnt,
		"selected_testee_count", len(selectedTestees),
		"selection_mode", selectionMode,
		"sample_rate", fmt.Sprintf("1/%d", planEnrollmentSampleRate),
		"explicit_testee_ids", explicitPlanTesteeIDs,
	)
	if len(selectedTestees) == 0 {
		logger.Infow("No testees found for plan backfill", "plan_id", planID, "org_id", orgID)
		return nil
	}

	planWorkers = normalizePlanWorkers(planWorkers, len(selectedTestees))
	logger.Infow("Running plan backfill with worker pool",
		"plan_id", planID,
		"org_id", orgID,
		"workers", planWorkers,
		"selected_testee_count", len(selectedTestees),
	)

	enrolledCount, err := enrollPlanTesteesConcurrently(ctx, deps, planID, selectedTestees, planWorkers)
	if err != nil {
		return err
	}

	scheduleSource := planApp.TaskSchedulerSourceSeedData
	scheduleResp, err := deps.APIClient.SchedulePendingTasks(ctx, "", scheduleSource)
	if err != nil {
		return fmt.Errorf("schedule pending tasks for org %d: %w", orgID, err)
	}
	logger.Infow("Scheduled pending plan tasks",
		"plan_id", planID,
		"org_id", orgID,
		"source", scheduleSource,
		"opened_count", len(scheduleResp.Tasks),
		"mini_program_delivery", "skipped",
	)

	submittedCount, skippedCount, completedCount, err := processPlanTasksConcurrently(
		ctx,
		deps,
		planID,
		orgID,
		scaleResp.QuestionnaireVersion,
		detail,
		selectedTestees,
		planWorkers,
		verbose,
	)
	if err != nil {
		return err
	}

	logger.Infow("Plan backfill completed",
		"plan_id", planID,
		"org_id", orgID,
		"enrolled_testees", enrolledCount,
		"submitted_answersheets", submittedCount,
		"completed_tasks", completedCount,
		"skipped_tasks", skippedCount,
		"opened_tasks", len(scheduleResp.Tasks),
	)

	return nil
}

func normalizePlanWorkers(workers, testeeCount int) int {
	if workers <= 0 {
		workers = 1
	}
	if testeeCount > 0 && workers > testeeCount {
		return testeeCount
	}
	return workers
}

func enrollPlanTesteesConcurrently(
	ctx context.Context,
	deps *dependencies,
	planID string,
	selectedTestees []*TesteeResponse,
	workers int,
) (int, error) {
	var enrolledCount atomic.Int64
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(workers)

	for _, testee := range selectedTestees {
		testee := testee
		g.Go(func() error {
			startDate, startDateSource, err := planStartDateFromAuditTimes(testee.CreatedAt, testee.UpdatedAt, time.Now())
			if err != nil {
				return fmt.Errorf("derive start_date for testee %s: %w", testee.ID, err)
			}
			if startDateSource != "created_at" {
				deps.Logger.Warnw("Plan backfill falling back when deriving start_date",
					"plan_id", planID,
					"testee_id", testee.ID,
					"start_date", startDate,
					"source", startDateSource,
					"created_at", testee.CreatedAt,
					"updated_at", testee.UpdatedAt,
				)
			}

			resp, err := deps.APIClient.EnrollTestee(ctx, EnrollTesteeRequest{
				PlanID:    planID,
				TesteeID:  testee.ID,
				StartDate: startDate,
			})
			if err != nil {
				return fmt.Errorf("enroll testee %s into plan %s: %w", testee.ID, planID, err)
			}

			deps.Logger.Infow("Testee enrolled into plan",
				"plan_id", planID,
				"testee_id", testee.ID,
				"start_date", startDate,
				"start_date_source", startDateSource,
				"task_count", len(resp.Tasks),
			)
			enrolledCount.Add(1)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return 0, err
	}
	return int(enrolledCount.Load()), nil
}

func processPlanTasksConcurrently(
	ctx context.Context,
	deps *dependencies,
	planID string,
	orgID int64,
	questionnaireVersion string,
	detail *QuestionnaireDetailResponse,
	selectedTestees []*TesteeResponse,
	workers int,
	verbose bool,
) (int, int, int, error) {
	var submittedCount atomic.Int64
	var skippedCount atomic.Int64
	var completedCount atomic.Int64

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(workers)

	for _, testee := range selectedTestees {
		testee := testee
		g.Go(func() error {
			return processPlanTasksForTestee(
				ctx,
				deps,
				planID,
				orgID,
				questionnaireVersion,
				detail,
				testee,
				verbose,
				&submittedCount,
				&skippedCount,
				&completedCount,
			)
		})
	}

	if err := g.Wait(); err != nil {
		return 0, 0, 0, err
	}
	return int(submittedCount.Load()), int(skippedCount.Load()), int(completedCount.Load()), nil
}

func processPlanTasksForTestee(
	ctx context.Context,
	deps *dependencies,
	planID string,
	orgID int64,
	questionnaireVersion string,
	detail *QuestionnaireDetailResponse,
	testee *TesteeResponse,
	verbose bool,
	submittedCount *atomic.Int64,
	skippedCount *atomic.Int64,
	completedCount *atomic.Int64,
) error {
	taskList, err := deps.APIClient.ListTasksByTesteeAndPlan(ctx, testee.ID, planID)
	if err != nil {
		return fmt.Errorf("list tasks for testee %s plan %s: %w", testee.ID, planID, err)
	}
	tasks := append([]TaskResponse(nil), taskList.Tasks...)
	sortTasksBySeq(tasks)

	for _, task := range tasks {
		switch normalizeTaskStatus(task.Status) {
		case "completed", "canceled":
			skippedCount.Add(1)
			if verbose {
				deps.Logger.Debugw("Skipping terminal plan task",
					"plan_id", planID,
					"testee_id", testee.ID,
					"task_id", task.ID,
					"status", task.Status,
				)
			}
			continue
		case "pending", "expired":
			skippedCount.Add(1)
			if verbose {
				deps.Logger.Debugw("Skipping non-open plan task",
					"plan_id", planID,
					"testee_id", testee.ID,
					"task_id", task.ID,
					"status", task.Status,
				)
			}
			continue
		case "opened":
			// continue
		default:
			skippedCount.Add(1)
			deps.Logger.Warnw("Skipping task with unsupported status",
				"plan_id", planID,
				"testee_id", testee.ID,
				"task_id", task.ID,
				"status", task.Status,
			)
			continue
		}

		req, err := buildPlanSubmissionRequest(detail, questionnaireVersion, testee, task, verbose, deps.Logger)
		if err != nil {
			return fmt.Errorf("build answersheet for testee %s task %s: %w", testee.ID, task.ID, err)
		}

		if verbose {
			logSubmitRequest(deps.Logger, *req, testee.ID)
		}

		attempts, err := submitAnswerSheetWithRetry(ctx, deps.APIClient, *req, submitMaxRetry)
		if err != nil {
			return fmt.Errorf(
				"submit answersheet for testee %s task %s failed after %d attempts: %w",
				testee.ID,
				task.ID,
				attempts,
				err,
			)
		}
		submittedCount.Add(1)

		if err := waitForTaskCompletion(ctx, deps.APIClient, orgID, task.ID); err != nil {
			return fmt.Errorf("wait for task %s completion: %w", task.ID, err)
		}
		completedCount.Add(1)

		if verbose {
			deps.Logger.Infow("Plan task completed",
				"plan_id", planID,
				"testee_id", testee.ID,
				"task_id", task.ID,
				"seq", task.Seq,
				"attempts", attempts,
			)
		}
	}

	return nil
}

func parsePlanTesteeIDs(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	items := strings.Split(raw, ",")
	seen := make(map[string]struct{}, len(items))
	ids := make([]string, 0, len(items))
	for _, item := range items {
		id := strings.TrimSpace(item)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids
}

func loadApiserverTestees(
	ctx context.Context,
	client *APIClient,
	orgID int64,
	pageSize, offset, limit int,
) ([]*TesteeResponse, error) {
	testees := make([]*TesteeResponse, 0, 64)
	err := iterateTesteesFromApiserver(ctx, client, orgID, pageSize, offset, limit, func(batch []*TesteeResponse) error {
		testees = append(testees, batch...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return testees, nil
}

func streamSamplePlanEnrollmentTestees(
	ctx context.Context,
	client *APIClient,
	orgID int64,
	pageSize, offset, limit int,
	planID string,
) ([]*TesteeResponse, int, error) {
	rngSeed := time.Now().UnixNano()
	if id := parseID(planID); id > 0 {
		rngSeed ^= int64(id)
	}
	rng := rand.New(rand.NewSource(rngSeed))

	selected := make([]*TesteeResponse, 0, 64)
	var fallback *TesteeResponse
	loadedCount := 0

	err := iterateTesteesFromApiserver(ctx, client, orgID, pageSize, offset, limit, func(batch []*TesteeResponse) error {
		for _, testee := range batch {
			if testee == nil || strings.TrimSpace(testee.ID) == "" {
				continue
			}
			loadedCount++
			if rng.Intn(planEnrollmentSampleRate) == 0 {
				selected = append(selected, testee)
			}
			if fallback == nil || rng.Intn(loadedCount) == 0 {
				fallback = testee
			}
		}
		return nil
	})
	if err != nil {
		return nil, loadedCount, err
	}

	if len(selected) == 0 && fallback != nil {
		selected = append(selected, fallback)
	}

	sortTesteesByCreatedAt(selected)
	return selected, loadedCount, nil
}

func loadExplicitPlanTestees(
	ctx context.Context,
	client *APIClient,
	testeeIDs []string,
) ([]*TesteeResponse, error) {
	testees := make([]*TesteeResponse, 0, len(testeeIDs))
	for _, testeeID := range testeeIDs {
		resp, err := client.GetTesteeByID(ctx, testeeID)
		if err != nil {
			return nil, err
		}
		if resp == nil || strings.TrimSpace(resp.ID) == "" {
			return nil, fmt.Errorf("testee %s not found", testeeID)
		}
		if resp.CreatedAt.IsZero() {
			return nil, newExplicitPlanZeroCreatedAtError(testeeID)
		}
		testees = append(testees, &TesteeResponse{
			ID:        resp.ID,
			CreatedAt: resp.CreatedAt,
			UpdatedAt: resp.UpdatedAt,
		})
	}
	sortTesteesByCreatedAt(testees)
	return testees, nil
}

func sortTesteesByCreatedAt(testees []*TesteeResponse) {
	sort.SliceStable(testees, func(i, j int) bool {
		left, right := testees[i], testees[j]
		if left == nil {
			return false
		}
		if right == nil {
			return true
		}
		if left.CreatedAt.Equal(right.CreatedAt) {
			return parseID(left.ID) < parseID(right.ID)
		}
		return left.CreatedAt.Before(right.CreatedAt)
	})
}

func sortTasksBySeq(tasks []TaskResponse) {
	sort.SliceStable(tasks, func(i, j int) bool {
		if tasks[i].Seq == tasks[j].Seq {
			return parseID(tasks[i].ID) < parseID(tasks[j].ID)
		}
		return tasks[i].Seq < tasks[j].Seq
	})
}

func buildPlanSubmissionRequest(
	detail *QuestionnaireDetailResponse,
	questionnaireVersion string,
	testee *TesteeResponse,
	task TaskResponse,
	verbose bool,
	logger interface {
		Warnw(string, ...interface{})
		Infow(string, ...interface{})
		Debugw(string, ...interface{})
	},
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

	rngSeed := time.Now().UnixNano()
	if testee != nil {
		rngSeed += int64(parseID(testee.ID))
	}
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
		logBuiltAnswers(logger, answers, detail.Code, testee.ID)
	}

	invalidAnswers := validateAnswers(detail, answers)
	if len(invalidAnswers) > 0 {
		logger.Warnw("Invalid answers detected for plan submission",
			"testee_id", testee.ID,
			"task_id", task.ID,
			"questionnaire_code", detail.Code,
			"invalid_count", len(invalidAnswers),
			"invalid_answers", invalidAnswers,
		)
	}

	testeeID := parseID(testee.ID)
	if testeeID == 0 {
		return nil, fmt.Errorf("invalid testee id: %s", testee.ID)
	}

	req := &SubmitAnswerSheetRequest{
		QuestionnaireCode:    detail.Code,
		QuestionnaireVersion: questionnaireVersion,
		Title:                detail.Title,
		TesteeID:             testeeID,
		TaskID:               task.ID,
		Answers:              answers,
	}
	return req, nil
}

func waitForTaskCompletion(ctx context.Context, client *APIClient, orgID int64, taskID string) error {
	deadline := time.NewTimer(planTaskCompletionTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(planTaskCompletionInterval)
	defer ticker.Stop()

	for {
		task, err := client.GetTask(ctx, taskID)
		if err != nil {
			return err
		}

		switch normalizeTaskStatus(task.Status) {
		case "completed":
			if task.AssessmentID == nil || strings.TrimSpace(*task.AssessmentID) == "" {
				return fmt.Errorf("task %s completed without assessment_id", taskID)
			}
			return nil
		case "canceled", "expired":
			return fmt.Errorf("task %s ended in terminal status %s before completion", taskID, task.Status)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("task %s did not complete within %s (org_id=%d)", taskID, planTaskCompletionTimeout, orgID)
		case <-ticker.C:
		}
	}
}

func normalizeTaskStatus(status string) string {
	return strings.ToLower(strings.TrimSpace(status))
}

func planStartDateFromAuditTimes(createdAt, updatedAt, now time.Time) (string, string, error) {
	switch {
	case !createdAt.IsZero():
		return createdAt.In(time.Local).Format("2006-01-02"), "created_at", nil
	case !updatedAt.IsZero():
		return updatedAt.In(time.Local).Format("2006-01-02"), "updated_at", nil
	case !now.IsZero():
		return now.In(time.Local).Format("2006-01-02"), "now", nil
	default:
		return "", "", fmt.Errorf("created_at and updated_at are both zero")
	}
}

func newPlanQuestionnaireVersionMismatchError(
	scaleCode string,
	questionnaireCode string,
	scaleQuestionnaireVersion string,
	loadedQuestionnaireVersion string,
) error {
	normalizedScaleCode := strings.ToLower(strings.TrimSpace(scaleCode))
	return fmt.Errorf(
		"questionnaire version mismatch for plan backfill: scale_code=%s questionnaire_code=%s scale_questionnaire_version=%s loaded_questionnaire_version=%s; seeddata loads questionnaire detail by code only, so this usually means the scale still comes from apiserver Redis cache or the scale is bound to a different questionnaire version; if you changed scale.questionnaire_version directly in MongoDB, delete Redis key scale:%s (or <cache.namespace>:scale:%s) and retry",
		scaleCode,
		questionnaireCode,
		scaleQuestionnaireVersion,
		loadedQuestionnaireVersion,
		normalizedScaleCode,
		normalizedScaleCode,
	)
}

func newExplicitPlanZeroCreatedAtError(testeeID string) error {
	return fmt.Errorf(
		"explicit plan backfill requires non-zero created_at: testee_id=%s; seeddata refuses to fall back to updated_at/now when --plan-testee-ids is used; if the database already has created_at, refresh /api/v1/testees/%s or delete Redis key testee:info:%s (or <cache.namespace>:testee:info:%s) and retry",
		testeeID,
		testeeID,
		testeeID,
		testeeID,
	)
}
