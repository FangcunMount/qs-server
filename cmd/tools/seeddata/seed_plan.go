package main

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
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
		return fmt.Errorf(
			"questionnaire version mismatch: scale=%s questionnaire=%s",
			scaleResp.QuestionnaireVersion,
			detail.Version,
		)
	}
	if verbose {
		debugLogQuestionnaire(detail, logger)
	}

	explicitPlanTesteeIDs := parsePlanTesteeIDs(planTesteeIDsRaw)

	pageSize := testeePageSize
	if pageSize < 100 {
		pageSize = 100
	}
	testees, err := loadApiserverTestees(ctx, deps.APIClient, orgID, pageSize, testeeOffset, testeeLimit)
	if err != nil {
		return err
	}
	sortTesteesByCreatedAt(testees)
	selectedTestees, selectionMode, err := selectPlanEnrollmentTestees(testees, planID, explicitPlanTesteeIDs)
	if err != nil {
		return err
	}
	logger.Infow("Loaded testees for plan backfill",
		"plan_id", planID,
		"org_id", orgID,
		"loaded_testee_count", len(testees),
		"selected_testee_count", len(selectedTestees),
		"selection_mode", selectionMode,
		"sample_rate", fmt.Sprintf("1/%d", planEnrollmentSampleRate),
		"explicit_testee_ids", explicitPlanTesteeIDs,
	)
	if len(selectedTestees) == 0 {
		logger.Infow("No testees found for plan backfill", "plan_id", planID, "org_id", orgID)
		return nil
	}

	enrolledCount := 0
	for _, testee := range selectedTestees {
		startDate, err := planStartDateFromCreatedAt(testee.CreatedAt)
		if err != nil {
			return fmt.Errorf("derive start_date for testee %s: %w", testee.ID, err)
		}

		resp, err := deps.APIClient.EnrollTestee(ctx, EnrollTesteeRequest{
			PlanID:    planID,
			TesteeID:  testee.ID,
			StartDate: startDate,
		})
		if err != nil {
			return fmt.Errorf("enroll testee %s into plan %s: %w", testee.ID, planID, err)
		}

		logger.Infow("Testee enrolled into plan",
			"plan_id", planID,
			"testee_id", testee.ID,
			"start_date", startDate,
			"created_task_count", len(resp.Tasks),
		)
		enrolledCount++
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

	submittedCount := 0
	skippedCount := 0
	completedCount := 0

	for _, testee := range testees {
		taskList, err := deps.APIClient.ListTasksByTesteeAndPlan(ctx, testee.ID, planID)
		if err != nil {
			return fmt.Errorf("list tasks for testee %s plan %s: %w", testee.ID, planID, err)
		}
		tasks := append([]TaskResponse(nil), taskList.Tasks...)
		sortTasksBySeq(tasks)

		for _, task := range tasks {
			switch normalizeTaskStatus(task.Status) {
			case "completed", "canceled":
				skippedCount++
				if verbose {
					logger.Debugw("Skipping terminal plan task",
						"plan_id", planID,
						"testee_id", testee.ID,
						"task_id", task.ID,
						"status", task.Status,
					)
				}
				continue
			case "pending", "expired":
				skippedCount++
				if verbose {
					logger.Debugw("Skipping non-open plan task",
						"plan_id", planID,
						"testee_id", testee.ID,
						"task_id", task.ID,
						"status", task.Status,
					)
				}
				continue
			case "opened":
				// 继续处理
			default:
				skippedCount++
				logger.Warnw("Skipping task with unsupported status",
					"plan_id", planID,
					"testee_id", testee.ID,
					"task_id", task.ID,
					"status", task.Status,
				)
				continue
			}

			req, err := buildPlanSubmissionRequest(detail, scaleResp.QuestionnaireVersion, testee, task, verbose, logger)
			if err != nil {
				return fmt.Errorf("build answersheet for testee %s task %s: %w", testee.ID, task.ID, err)
			}

			if verbose {
				logSubmitRequest(logger, *req, testee.ID)
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
			submittedCount++

			if err := waitForTaskCompletion(ctx, deps.APIClient, orgID, task.ID); err != nil {
				return fmt.Errorf("wait for task %s completion: %w", task.ID, err)
			}
			completedCount++

			if verbose {
				logger.Infow("Plan task completed",
					"plan_id", planID,
					"testee_id", testee.ID,
					"task_id", task.ID,
					"seq", task.Seq,
					"attempts", attempts,
				)
			}
		}
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

func selectPlanEnrollmentTestees(testees []*TesteeResponse, planID string, explicitIDs []string) ([]*TesteeResponse, string, error) {
	if len(testees) == 0 {
		return nil, "empty", nil
	}

	if len(explicitIDs) > 0 {
		byID := make(map[string]*TesteeResponse, len(testees))
		for _, testee := range testees {
			if testee == nil || strings.TrimSpace(testee.ID) == "" {
				continue
			}
			byID[testee.ID] = testee
		}

		selected := make([]*TesteeResponse, 0, len(explicitIDs))
		missing := make([]string, 0)
		for _, id := range explicitIDs {
			testee, ok := byID[id]
			if !ok || testee == nil {
				missing = append(missing, id)
				continue
			}
			selected = append(selected, testee)
		}
		if len(missing) > 0 {
			return nil, "explicit", fmt.Errorf(
				"plan testee ids not found in loaded testees: %s (adjust --testee-offset/--testee-limit or remove --plan-testee-ids)",
				strings.Join(missing, ","),
			)
		}
		return selected, "explicit", nil
	}

	selectedCount := (len(testees) + planEnrollmentSampleRate - 1) / planEnrollmentSampleRate
	if selectedCount <= 0 {
		selectedCount = 1
	}
	if selectedCount > len(testees) {
		selectedCount = len(testees)
	}

	candidates := append([]*TesteeResponse(nil), testees...)
	rngSeed := time.Now().UnixNano()
	if id := parseID(planID); id > 0 {
		rngSeed ^= int64(id)
	}
	rng := rand.New(rand.NewSource(rngSeed))
	rng.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})

	selected := append([]*TesteeResponse(nil), candidates[:selectedCount]...)
	sortTesteesByCreatedAt(selected)
	return selected, "sample", nil
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
		return nil, fmt.Errorf("questionnaire version mismatch: detail=%s expected=%s", detail.Version, questionnaireVersion)
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

func planStartDateFromCreatedAt(createdAt time.Time) (string, error) {
	if createdAt.IsZero() {
		return "", fmt.Errorf("created_at is zero")
	}
	return createdAt.In(time.Local).Format("2006-01-02"), nil
}
