package main

import (
	"context"
	"fmt"
	"hash/fnv"
	"math/rand"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	"github.com/mattn/go-isatty"
	"golang.org/x/sync/errgroup"
)

const (
	defaultPlanID              = "614186929759466030"
	planEnrollmentSampleRate   = 5
	planTaskCompletionTimeout  = 5 * time.Minute
	planTaskCompletionInterval = 2 * time.Second
	planTaskCompletedOffset    = 2 * time.Hour
	planTaskTimeLayout         = "2006-01-02 15:04:05"
	planScheduleBatchFactor    = 4
	planTaskBufferFactor       = 4
	planMaxInFlightFactor      = 8
	planMinMaxInFlightTasks    = 20
	seedPlanPaceInterval       = 3 * time.Minute
	seedPlanPaceSleep          = 15 * time.Second
	seedPlanRecoverableRetries = 3
	seedPlanRecoverableMinWait = 30 * time.Second
	seedPlanRecoverableMaxWait = 120 * time.Second
)

type seedPlanPacerCtxKey struct{}

type seedPlanPacer struct {
	mu          sync.Mutex
	startedAt   time.Time
	interval    time.Duration
	pause       time.Duration
	nextPauseAt time.Time
	sleepUntil  time.Time
	logger      interface{ Infow(string, ...interface{}) }
	verbose     bool
}

func newSeedPlanPacer(
	startedAt time.Time,
	interval time.Duration,
	pause time.Duration,
	logger interface{ Infow(string, ...interface{}) },
	verbose bool,
) *seedPlanPacer {
	if interval <= 0 || pause <= 0 {
		return nil
	}
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	return &seedPlanPacer{
		startedAt: startedAt,
		interval:  interval,
		pause:     pause,
		logger:    logger,
		verbose:   verbose,
	}
}

func withSeedPlanPacer(ctx context.Context, pacer *seedPlanPacer) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if pacer == nil {
		return ctx
	}
	return context.WithValue(ctx, seedPlanPacerCtxKey{}, pacer)
}

func seedPlanPacerFromContext(ctx context.Context) *seedPlanPacer {
	if ctx == nil {
		return nil
	}
	pacer, _ := ctx.Value(seedPlanPacerCtxKey{}).(*seedPlanPacer)
	return pacer
}

func (p *seedPlanPacer) nextDelay(now time.Time) (time.Duration, bool) {
	if p == nil || p.interval <= 0 || p.pause <= 0 {
		return 0, false
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.startedAt.IsZero() {
		p.startedAt = now
	}
	if p.nextPauseAt.IsZero() {
		p.nextPauseAt = p.startedAt.Add(p.interval)
	}

	if !p.sleepUntil.IsZero() && now.Before(p.sleepUntil) {
		return p.sleepUntil.Sub(now), false
	}
	if now.Before(p.nextPauseAt) {
		return 0, false
	}

	p.sleepUntil = now.Add(p.pause)
	for !p.nextPauseAt.After(now) {
		p.nextPauseAt = p.nextPauseAt.Add(p.interval)
	}
	return p.pause, true
}

func (p *seedPlanPacer) Wait(ctx context.Context, reason string) error {
	if p == nil {
		return nil
	}
	delay, freshPause := p.nextDelay(time.Now())
	if delay <= 0 {
		return nil
	}

	if freshPause && p.verbose && p.logger != nil {
		p.logger.Infow("Seed plan pacing pause",
			"reason", reason,
			"pause_seconds", int(delay.Seconds()),
			"interval_seconds", int(p.interval.Seconds()),
		)
	}
	return sleepWithContext(ctx, delay)
}

func waitForSeedPlanPacer(ctx context.Context, reason string) error {
	return seedPlanPacerFromContext(ctx).Wait(ctx, reason)
}

func seedPlanBackfill(
	ctx context.Context,
	deps *dependencies,
	_ *seedContext,
	planID string,
	planMode string,
	planTesteeIDsRaw string,
	planWorkers int,
	planSubmitWorkers int,
	planWaitWorkers int,
	planMaxInFlightTasks int,
	planExpireRate float64,
	planProcessExistingOnly bool,
	testeePageSize, testeeOffset, testeeLimit int,
	verbose bool,
) error {
	if deps == nil {
		return fmt.Errorf("dependencies are nil")
	}
	if deps.APIClient == nil {
		return fmt.Errorf("api client is not initialized")
	}
	if planMode == planModeRemote && deps.CollectionClient == nil {
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
	planExpireRate = normalizePlanExpireRate(planExpireRate)
	logger.Infow("Plan backfill started",
		"plan_id", planID,
		"plan_mode", planMode,
		"org_id", orgID,
		"plan_workers", planWorkers,
		"plan_submit_workers", planSubmitWorkers,
		"plan_wait_workers", planWaitWorkers,
		"plan_max_inflight_tasks", planMaxInFlightTasks,
		"plan_expire_rate", planExpireRate,
		"plan_process_existing_only", planProcessExistingOnly,
		"testee_page_size", testeePageSize,
		"testee_offset", testeeOffset,
		"testee_limit", testeeLimit,
		"verbose", verbose,
	)

	ctx = withSeedPlanPacer(
		ctx,
		newSeedPlanPacer(
			time.Now(),
			seedPlanPaceInterval,
			seedPlanPaceSleep,
			logger,
			verbose,
		),
	)

	prewarmAPIToken(ctx, deps.APIClient, orgID, logger)

	gateway, cleanupGateway, err := newPlanSeedGateway(ctx, deps, planMode, !verbose)
	if err != nil {
		return fmt.Errorf("initialize plan seed gateway (%s): %w", planMode, err)
	}
	if cleanupGateway != nil {
		defer func() {
			if cleanupErr := cleanupGateway(); cleanupErr != nil {
				logger.Warnw("Failed to cleanup plan seed gateway",
					"plan_id", planID,
					"plan_mode", planMode,
					"error", cleanupErr.Error(),
				)
			}
		}()
	}

	planResp, err := gateway.GetPlan(ctx, planID)
	if err != nil {
		if planMode == planModeLocal {
			return fmt.Errorf("load plan %s in local mode: %w; local mode reads plan data from --local-mysql-dsn/--local-mongo-uri/--local-redis-* instead of %s, so verify those local connections point to the environment that contains this plan or rerun with --plan-mode remote", planID, err, strings.TrimSpace(deps.APIClient.baseURL))
		}
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

	scaleResp, err := gateway.GetScale(ctx, planResp.ScaleCode)
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

	detail, err := gateway.GetQuestionnaireDetail(ctx, scaleResp.QuestionnaireCode)
	if err != nil {
		return fmt.Errorf("load questionnaire %s: %w", scaleResp.QuestionnaireCode, err)
	}
	if detail == nil {
		return fmt.Errorf("questionnaire %s not found", scaleResp.QuestionnaireCode)
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
	explicitPlanTesteeIDs = applyTesteeLimitToIDs(explicitPlanTesteeIDs, testeeLimit)

	var (
		testees         []*TesteeResponse
		selectedTestees []*TesteeResponse
		selectionMode   string
		loadedTesteeCnt int
	)
	if len(explicitPlanTesteeIDs) > 0 {
		testees, err = loadExplicitPlanTestees(ctx, gateway, explicitPlanTesteeIDs)
		if err != nil {
			return err
		}
		selectedTestees = testees
		selectionMode = "explicit"
		loadedTesteeCnt = len(testees)
	} else if planProcessExistingOnly {
		pageSize := testeePageSize
		if pageSize < 100 {
			pageSize = 100
		}
		testees, err = loadApiserverTestees(ctx, gateway, orgID, pageSize, testeeOffset, testeeLimit)
		if err != nil {
			return err
		}
		sortTesteesByCreatedAt(testees)
		selectedTestees = testees
		selectionMode = "recovery_all"
		loadedTesteeCnt = len(testees)
	} else {
		pageSize := testeePageSize
		if pageSize < 100 {
			pageSize = 100
		}
		selectedTestees, loadedTesteeCnt, err = streamSamplePlanEnrollmentTestees(
			ctx,
			gateway,
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
	if verbose {
		logger.Infow("Running plan backfill with worker pool",
			"plan_id", planID,
			"org_id", orgID,
			"workers", planWorkers,
			"selected_testee_count", len(selectedTestees),
		)
	}

	existingStats, err := inspectExistingPlanTasks(ctx, gateway, deps.Logger, planID, selectedTestees, planWorkers, verbose)
	if err != nil {
		return err
	}
	logger.Infow("Inspected existing plan tasks before backfill",
		"plan_id", planID,
		"org_id", orgID,
		"selected_testee_count", len(selectedTestees),
		"existing_task_stats", existingStats,
	)

	enrolledCount := 0
	failedEnrollments := 0
	if planProcessExistingOnly {
		if existingStats.Total == 0 {
			logger.Infow("No existing plan tasks found for recovery mode",
				"plan_id", planID,
				"org_id", orgID,
				"selected_testee_count", len(selectedTestees),
			)
			return nil
		}
		logger.Infow("Skipping plan enrollment because recovery mode is enabled",
			"plan_id", planID,
			"org_id", orgID,
			"selected_testee_count", len(selectedTestees),
		)
	} else {
		enrolledCount, failedEnrollments, err = enrollPlanTesteesConcurrently(ctx, gateway, deps.Logger, planID, selectedTestees, planWorkers, verbose)
		if err != nil {
			return err
		}
	}

	executionStats, err := scheduleAndProcessPlanTasks(
		ctx,
		gateway,
		deps,
		planID,
		planMode,
		orgID,
		scaleResp.QuestionnaireVersion,
		detail,
		selectedTestees,
		planWorkers,
		planSubmitWorkers,
		planWaitWorkers,
		planMaxInFlightTasks,
		planExpireRate,
		verbose,
	)
	if err != nil {
		return err
	}

	logger.Infow("Plan backfill completed",
		"plan_id", planID,
		"org_id", orgID,
		"enrolled_testees", enrolledCount,
		"failed_enrollments", failedEnrollments,
		"submitted_answersheets", executionStats.SubmittedCount,
		"completed_tasks", executionStats.CompletedCount,
		"expired_tasks", executionStats.ExpiredCount,
		"recovered_tasks", executionStats.RecoveredCount,
		"max_inflight_observed", executionStats.MaxInFlightObserved,
		"skipped_tasks", executionStats.SkippedCount,
		"opened_tasks", executionStats.OpenedCount,
		"schedule_stats", executionStats.ScheduleStats,
		"failed_schedule_batches", executionStats.FailedScheduleBatches,
		"failed_task_list_loads", executionStats.FailedTaskListLoads,
		"failed_task_executions", executionStats.FailedTaskExecutions,
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
	gateway PlanSeedGateway,
	logger interface {
		Warnw(string, ...interface{})
		Infow(string, ...interface{})
	},
	planID string,
	selectedTestees []*TesteeResponse,
	workers int,
	verbose bool,
) (int, int, error) {
	var enrolledCount atomic.Int64
	var failedCount atomic.Int64
	if err := runPlanTesteeWorkerPool(ctx, selectedTestees, workers, func(ctx context.Context, testee *TesteeResponse) error {
		err := runSeedPlanOperationWithRecovery(ctx, logger, verbose, "enroll_testee_into_plan", testee.ID, func() error {
			startDate, startDateSource, err := planStartDateFromAuditTimes(testee.CreatedAt, testee.UpdatedAt, time.Now())
			if err != nil {
				return fmt.Errorf("derive start_date for testee %s: %w", testee.ID, err)
			}
			if startDateSource != "created_at" {
				logger.Warnw("Plan backfill falling back when deriving start_date",
					"plan_id", planID,
					"testee_id", testee.ID,
					"start_date", startDate,
					"source", startDateSource,
					"created_at", testee.CreatedAt,
					"updated_at", testee.UpdatedAt,
				)
			}

			if err := waitForSeedPlanPacer(ctx, "enroll_testee_into_plan"); err != nil {
				return err
			}

			resp, err := gateway.EnrollTestee(ctx, EnrollTesteeRequest{
				PlanID:    planID,
				TesteeID:  testee.ID,
				StartDate: startDate,
			})
			if err != nil {
				return fmt.Errorf("enroll testee %s into plan %s: %w", testee.ID, planID, err)
			}

			if verbose {
				logger.Infow("Testee enrolled into plan",
					"plan_id", planID,
					"testee_id", testee.ID,
					"start_date", startDate,
					"start_date_source", startDateSource,
					"task_count", len(resp.Tasks),
				)
			}
			enrolledCount.Add(1)
			return nil
		})
		if err != nil {
			failedCount.Add(1)
			if verbose {
				logger.Warnw("Plan enrollment failed after recovery attempts",
					"plan_id", planID,
					"testee_id", testee.ID,
					"error", err.Error(),
				)
			}
			return nil
		}
		return nil
	}); err != nil {
		return 0, 0, err
	}
	return int(enrolledCount.Load()), int(failedCount.Load()), nil
}

func scheduleAndProcessPlanTasks(
	ctx context.Context,
	gateway PlanSeedGateway,
	deps *dependencies,
	planID string,
	planMode string,
	orgID int64,
	questionnaireVersion string,
	detail *QuestionnaireDetailResponse,
	selectedTestees []*TesteeResponse,
	workers int,
	submitWorkers int,
	waitWorkers int,
	maxInFlightTasks int,
	planExpireRate float64,
	verbose bool,
) (*seedPlanExecutionStats, error) {
	var submittedCount atomic.Int64
	var skippedCount atomic.Int64
	var completedCount atomic.Int64
	var expiredCount atomic.Int64
	var recoveredCount atomic.Int64
	var failedScheduleBatchCount atomic.Int64
	var failedTaskListCount atomic.Int64
	var failedTaskExecutionCount atomic.Int64
	var reservedOpenTask atomic.Bool
	var inflightCount atomic.Int64
	var maxInflightObserved atomic.Int64

	aggregateScheduleStats := &TaskScheduleStatsResponse{}
	totalOpenedCount := 0
	scheduleSource := planApp.TaskSchedulerSourceSeedData
	submitWorkers, waitWorkers, maxInFlightTasks = normalizePlanTaskExecutionConcurrency(workers, submitWorkers, waitWorkers, maxInFlightTasks)
	taskBufferSize := normalizePlanTaskBufferSize(submitWorkers, maxInFlightTasks)
	batches := chunkPlanTestees(selectedTestees, normalizePlanScheduleBatchSize(workers))
	dashboard := newPlanSeedDashboard(
		planMode,
		len(batches),
		&submittedCount,
		&completedCount,
		&expiredCount,
		&skippedCount,
		&recoveredCount,
		&inflightCount,
		&maxInflightObserved,
		&failedTaskExecutionCount,
	)
	defer dashboard.Finish()

	if verbose {
		deps.Logger.Infow("Running plan task execution pipeline",
			"plan_id", planID,
			"org_id", orgID,
			"submit_workers", submitWorkers,
			"wait_workers", waitWorkers,
			"max_inflight_tasks", maxInFlightTasks,
			"task_buffer_size", taskBufferSize,
		)
	}

	for batchIndex, batch := range batches {
		dashboard.SetCurrentBatch(batchIndex + 1)
		var scheduleResp *TaskListResponse
		err := runSeedPlanOperationWithRecovery(ctx, deps.Logger, verbose, "schedule_pending_plan_tasks", fmt.Sprintf("batch_%d", batchIndex+1), func() error {
			if err := waitForSeedPlanPacer(ctx, "schedule_pending_plan_tasks"); err != nil {
				return err
			}
			resp, err := gateway.SchedulePendingTasks(ctx, SchedulePendingTasksRequest{
				Source:    scheduleSource,
				PlanID:    planID,
				TesteeIDs: collectPlanTesteeIDs(batch),
			})
			if err != nil {
				return err
			}
			scheduleResp = resp
			return nil
		})
		if err != nil {
			failedScheduleBatchCount.Add(1)
			dashboard.IncrementScheduleFailures()
			deps.Logger.Warnw("Skipping schedule batch after recovery attempts failed",
				"plan_id", planID,
				"org_id", orgID,
				"batch_index", batchIndex+1,
				"batch_count", len(batches),
				"batch_testee_count", len(batch),
				"error", err.Error(),
			)
			continue
		}

		totalOpenedCount += len(scheduleResp.Tasks)
		dashboard.AddOpenedTasks(len(scheduleResp.Tasks))
		mergeTaskScheduleStats(aggregateScheduleStats, scheduleResp.Stats)
		if verbose {
			deps.Logger.Infow("Scheduled pending plan tasks",
				"plan_id", planID,
				"org_id", orgID,
				"source", scheduleSource,
				"batch_index", batchIndex+1,
				"batch_count", len(batches),
				"batch_testee_count", len(batch),
				"opened_count", len(scheduleResp.Tasks),
				"schedule_stats", scheduleResp.Stats,
				"mini_program_delivery", "skipped",
			)
		}

		taskJobs, err := collectPlanTaskJobs(
			ctx,
			gateway,
			deps,
			planID,
			batch,
			verbose,
			&skippedCount,
			&failedTaskListCount,
		)
		if err != nil {
			return nil, err
		}
		if len(taskJobs) == 0 {
			continue
		}
		dashboard.AddDiscoveredTasks(len(taskJobs))

		err = runPlanTaskExecutionPipeline(
			ctx,
			gateway,
			deps,
			planID,
			orgID,
			questionnaireVersion,
			detail,
			taskJobs,
			submitWorkers,
			waitWorkers,
			taskBufferSize,
			maxInFlightTasks,
			planExpireRate,
			verbose,
			&reservedOpenTask,
			&submittedCount,
			&skippedCount,
			&completedCount,
			&expiredCount,
			&recoveredCount,
			&failedTaskExecutionCount,
			&inflightCount,
			&maxInflightObserved,
			dashboard,
		)
		if err != nil {
			return nil, err
		}
	}

	return &seedPlanExecutionStats{
		OpenedCount:           totalOpenedCount,
		ScheduleStats:         aggregateScheduleStats,
		SubmittedCount:        int(submittedCount.Load()),
		SkippedCount:          int(skippedCount.Load()),
		CompletedCount:        int(completedCount.Load()),
		ExpiredCount:          int(expiredCount.Load()),
		RecoveredCount:        int(recoveredCount.Load()),
		MaxInFlightObserved:   int(maxInflightObserved.Load()),
		FailedScheduleBatches: int(failedScheduleBatchCount.Load()),
		FailedTaskListLoads:   int(failedTaskListCount.Load()),
		FailedTaskExecutions:  int(failedTaskExecutionCount.Load()),
	}, nil
}

func runPlanTesteeWorkerPool(
	ctx context.Context,
	testees []*TesteeResponse,
	workers int,
	fn func(context.Context, *TesteeResponse) error,
) error {
	if len(testees) == 0 {
		return nil
	}
	if workers <= 0 {
		workers = 1
	}
	if fn == nil {
		return fmt.Errorf("plan worker function is nil")
	}

	jobs := make(chan *TesteeResponse, workers)
	g, gctx := errgroup.WithContext(ctx)

	for i := 0; i < workers; i++ {
		g.Go(func() error {
			for {
				select {
				case <-gctx.Done():
					return nil
				case testee, ok := <-jobs:
					if !ok {
						return nil
					}
					if testee == nil {
						continue
					}
					if err := fn(gctx, testee); err != nil {
						return err
					}
				}
			}
		})
	}

	g.Go(func() error {
		defer close(jobs)
		for _, testee := range testees {
			select {
			case <-gctx.Done():
				return nil
			case jobs <- testee:
			}
		}
		return nil
	})

	return g.Wait()
}

type planTaskJob struct {
	testee *TesteeResponse
	task   TaskResponse
}

type planTaskStatusStats struct {
	Total     int `json:"total"`
	Pending   int `json:"pending"`
	Opened    int `json:"opened"`
	Completed int `json:"completed"`
	Expired   int `json:"expired"`
	Canceled  int `json:"canceled"`
	Unknown   int `json:"unknown"`
}

type seedPlanExecutionStats struct {
	OpenedCount           int
	ScheduleStats         *TaskScheduleStatsResponse
	SubmittedCount        int
	SkippedCount          int
	CompletedCount        int
	ExpiredCount          int
	RecoveredCount        int
	MaxInFlightObserved   int
	FailedEnrollments     int
	FailedScheduleBatches int
	FailedTaskListLoads   int
	FailedTaskExecutions  int
}

type planTaskWaitJob struct {
	testee   *TesteeResponse
	task     TaskResponse
	attempts int
}

func normalizePlanTaskExecutionConcurrency(workers, submitWorkers, waitWorkers, maxInFlightTasks int) (int, int, int) {
	baseWorkers := workers
	if baseWorkers <= 0 {
		baseWorkers = 1
	}

	if submitWorkers <= 0 {
		submitWorkers = baseWorkers
	}
	if waitWorkers <= 0 {
		waitWorkers = baseWorkers
	}
	if submitWorkers <= 0 {
		submitWorkers = 1
	}
	if waitWorkers <= 0 {
		waitWorkers = 1
	}

	if maxInFlightTasks <= 0 {
		maxInFlightTasks = max(submitWorkers, waitWorkers) * planMaxInFlightFactor
		if maxInFlightTasks < planMinMaxInFlightTasks {
			maxInFlightTasks = planMinMaxInFlightTasks
		}
	}
	if maxInFlightTasks < submitWorkers {
		maxInFlightTasks = submitWorkers
	}
	if maxInFlightTasks < waitWorkers {
		maxInFlightTasks = waitWorkers
	}
	return submitWorkers, waitWorkers, maxInFlightTasks
}

func runPlanTaskExecutionPipeline(
	ctx context.Context,
	gateway PlanSeedGateway,
	deps *dependencies,
	planID string,
	orgID int64,
	questionnaireVersion string,
	detail *QuestionnaireDetailResponse,
	jobs []planTaskJob,
	submitWorkers int,
	waitWorkers int,
	taskBufferSize int,
	maxInFlightTasks int,
	planExpireRate float64,
	verbose bool,
	reservedOpenTask *atomic.Bool,
	submittedCount *atomic.Int64,
	skippedCount *atomic.Int64,
	completedCount *atomic.Int64,
	expiredCount *atomic.Int64,
	recoveredCount *atomic.Int64,
	failedTaskExecutionCount *atomic.Int64,
	inflightCount *atomic.Int64,
	maxInflightObserved *atomic.Int64,
	dashboard *planSeedDashboard,
) error {
	if len(jobs) == 0 {
		return nil
	}
	if submitWorkers <= 0 {
		submitWorkers = 1
	}
	if waitWorkers <= 0 {
		waitWorkers = 1
	}
	if taskBufferSize < submitWorkers {
		taskBufferSize = submitWorkers
	}
	if maxInFlightTasks < submitWorkers {
		maxInFlightTasks = submitWorkers
	}
	if maxInFlightTasks < waitWorkers {
		maxInFlightTasks = waitWorkers
	}

	jobCh := make(chan planTaskJob, taskBufferSize)
	waitCh := make(chan planTaskWaitJob, maxInFlightTasks)
	inflightSlots := make(chan struct{}, maxInFlightTasks)

	var submitWG sync.WaitGroup
	for i := 0; i < submitWorkers; i++ {
		submitWG.Add(1)
		go func() {
			defer submitWG.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case job, ok := <-jobCh:
					if !ok {
						return
					}
					processPlanTaskSubmitStage(
						ctx,
						gateway,
						deps,
						planID,
						orgID,
						questionnaireVersion,
						detail,
						job,
						planExpireRate,
						verbose,
						reservedOpenTask,
						submittedCount,
						skippedCount,
						expiredCount,
						recoveredCount,
						failedTaskExecutionCount,
						inflightSlots,
						waitCh,
						inflightCount,
						maxInflightObserved,
						dashboard,
					)
				}
			}
		}()
	}

	var waitWG sync.WaitGroup
	for i := 0; i < waitWorkers; i++ {
		waitWG.Add(1)
		go func() {
			defer waitWG.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case waitJob, ok := <-waitCh:
					if !ok {
						return
					}
					processPlanTaskWaitStage(
						ctx,
						deps,
						planID,
						orgID,
						waitJob,
						verbose,
						completedCount,
						failedTaskExecutionCount,
						inflightSlots,
						inflightCount,
						dashboard,
					)
				}
			}
		}()
	}

	for _, job := range jobs {
		select {
		case <-ctx.Done():
			close(jobCh)
			submitWG.Wait()
			close(waitCh)
			waitWG.Wait()
			return ctx.Err()
		case jobCh <- job:
		}
	}

	close(jobCh)
	submitWG.Wait()
	close(waitCh)
	waitWG.Wait()

	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

func processPlanTaskSubmitStage(
	ctx context.Context,
	gateway PlanSeedGateway,
	deps *dependencies,
	planID string,
	orgID int64,
	questionnaireVersion string,
	detail *QuestionnaireDetailResponse,
	job planTaskJob,
	planExpireRate float64,
	verbose bool,
	reservedOpenTask *atomic.Bool,
	submittedCount *atomic.Int64,
	skippedCount *atomic.Int64,
	expiredCount *atomic.Int64,
	recoveredCount *atomic.Int64,
	failedTaskExecutionCount *atomic.Int64,
	inflightSlots chan struct{},
	waitCh chan<- planTaskWaitJob,
	inflightCount *atomic.Int64,
	maxInflightObserved *atomic.Int64,
	dashboard *planSeedDashboard,
) {
	if job.testee == nil || strings.TrimSpace(job.task.ID) == "" {
		dashboard.AdvanceTask()
		return
	}

	if reservedOpenTask != nil && reservedOpenTask.CompareAndSwap(false, true) {
		skippedCount.Add(1)
		if verbose {
			deps.Logger.Infow("Leaving one opened plan task unprocessed to keep plan active",
				"plan_id", planID,
				"testee_id", job.testee.ID,
				"task_id", job.task.ID,
				"seq", job.task.Seq,
			)
		}
		dashboard.AdvanceTask()
		return
	}

	if shouldExpirePlanTask(job.task, planExpireRate) {
		err := runSeedPlanOperationWithRecovery(ctx, deps.Logger, verbose, "expire_plan_task", job.task.ID, func() error {
			finalTask, recovered, err := expirePlanTaskWithRecovery(ctx, gateway, orgID, job.task)
			if err != nil {
				return fmt.Errorf("expire task %s for testee %s: %w", job.task.ID, job.testee.ID, err)
			}
			if recovered && recoveredCount != nil {
				recoveredCount.Add(1)
			}
			switch normalizeTaskStatus(finalTask.Status) {
			case "expired":
				expiredCount.Add(1)
			default:
				skippedCount.Add(1)
			}
			if verbose {
				deps.Logger.Infow("Plan task expired intentionally",
					"plan_id", planID,
					"testee_id", job.testee.ID,
					"task_id", job.task.ID,
					"seq", job.task.Seq,
					"expire_rate", planExpireRate,
					"recovered", recovered,
					"final_status", finalTask.Status,
				)
			}
			return nil
		})
		if err != nil {
			failedTaskExecutionCount.Add(1)
			if verbose {
				deps.Logger.Warnw("Skipping task after recovery attempts failed",
					"plan_id", planID,
					"org_id", orgID,
					"testee_id", job.testee.ID,
					"task_id", job.task.ID,
					"error", err.Error(),
				)
			}
		}
		dashboard.AdvanceTask()
		return
	}

	req, err := buildPlanSubmissionRequest(detail, questionnaireVersion, job.testee, job.task, verbose, deps.Logger)
	if err != nil {
		failedTaskExecutionCount.Add(1)
		if verbose {
			deps.Logger.Warnw("Skipping task because answersheet request build failed",
				"plan_id", planID,
				"org_id", orgID,
				"testee_id", job.testee.ID,
				"task_id", job.task.ID,
				"error", err.Error(),
			)
		}
		dashboard.AdvanceTask()
		return
	}

	if verbose {
		logSubmitRequest(deps.Logger, *req, job.testee.ID)
	}

	select {
	case <-ctx.Done():
		dashboard.AdvanceTask()
		return
	case inflightSlots <- struct{}{}:
	}

	currentInflight := inflightCount.Add(1)
	updateMaxInFlightCounter(maxInflightObserved, currentInflight)
	dashboard.Refresh()

	releaseSlot := func() {
		<-inflightSlots
		inflightCount.Add(-1)
	}

	var attempts int
	err = runSeedPlanOperationWithRecovery(ctx, deps.Logger, verbose, "submit_plan_answersheet", job.task.ID, func() error {
		if err := waitForSeedPlanPacer(ctx, "submit_plan_answersheet"); err != nil {
			return err
		}
		submitAttempts, submitErr := submitAnswerSheetWithRetry(ctx, deps.APIClient, *req, submitMaxRetry)
		attempts = submitAttempts
		if submitErr != nil {
			return fmt.Errorf(
				"submit answersheet for testee %s task %s failed after %d attempts: %w",
				job.testee.ID,
				job.task.ID,
				submitAttempts,
				submitErr,
			)
		}
		return nil
	})
	if err != nil {
		releaseSlot()
		failedTaskExecutionCount.Add(1)
		if verbose {
			deps.Logger.Warnw("Skipping task after recovery attempts failed",
				"plan_id", planID,
				"org_id", orgID,
				"testee_id", job.testee.ID,
				"task_id", job.task.ID,
				"error", err.Error(),
			)
		}
		dashboard.AdvanceTask()
		return
	}

	submittedCount.Add(1)
	dashboard.Refresh()

	waitJob := planTaskWaitJob{
		testee:   job.testee,
		task:     job.task,
		attempts: attempts,
	}
	select {
	case <-ctx.Done():
		releaseSlot()
		dashboard.AdvanceTask()
		return
	case waitCh <- waitJob:
	}
}

func processPlanTaskWaitStage(
	ctx context.Context,
	deps *dependencies,
	planID string,
	orgID int64,
	waitJob planTaskWaitJob,
	verbose bool,
	completedCount *atomic.Int64,
	failedTaskExecutionCount *atomic.Int64,
	inflightSlots chan struct{},
	inflightCount *atomic.Int64,
	dashboard *planSeedDashboard,
) {
	releaseSlot := func() {
		<-inflightSlots
		inflightCount.Add(-1)
	}
	defer func() {
		releaseSlot()
		dashboard.AdvanceTask()
	}()

	err := waitForTaskCompletion(ctx, deps.Logger, deps.APIClient, orgID, waitJob.task.ID, verbose)
	if err != nil {
		failedTaskExecutionCount.Add(1)
		if verbose {
			deps.Logger.Warnw("Skipping task after completion wait failed",
				"plan_id", planID,
				"org_id", orgID,
				"testee_id", waitJob.testee.ID,
				"task_id", waitJob.task.ID,
				"error", err.Error(),
			)
		}
		return
	}

	completedCount.Add(1)
	if verbose {
		deps.Logger.Infow("Plan task completed",
			"plan_id", planID,
			"testee_id", waitJob.testee.ID,
			"task_id", waitJob.task.ID,
			"seq", waitJob.task.Seq,
			"attempts", waitJob.attempts,
		)
	}
	dashboard.Refresh()
}

func updateMaxInFlightCounter(counter *atomic.Int64, current int64) {
	if counter == nil {
		return
	}
	for {
		existing := counter.Load()
		if current <= existing {
			return
		}
		if counter.CompareAndSwap(existing, current) {
			return
		}
	}
}

func runPlanTaskWorkerPool(
	ctx context.Context,
	jobs []planTaskJob,
	workers int,
	bufferSize int,
	fn func(context.Context, planTaskJob) error,
) error {
	if len(jobs) == 0 {
		return nil
	}
	if workers <= 0 {
		workers = 1
	}
	if bufferSize < workers {
		bufferSize = workers
	}
	if fn == nil {
		return fmt.Errorf("plan task worker function is nil")
	}

	jobCh := make(chan planTaskJob, bufferSize)
	g, gctx := errgroup.WithContext(ctx)

	for i := 0; i < workers; i++ {
		g.Go(func() error {
			for {
				select {
				case <-gctx.Done():
					return nil
				case job, ok := <-jobCh:
					if !ok {
						return nil
					}
					if job.testee == nil || strings.TrimSpace(job.task.ID) == "" {
						continue
					}
					if err := fn(gctx, job); err != nil {
						return err
					}
				}
			}
		})
	}

	g.Go(func() error {
		defer close(jobCh)
		for _, job := range jobs {
			select {
			case <-gctx.Done():
				return nil
			case jobCh <- job:
			}
		}
		return nil
	})

	return g.Wait()
}

func collectPlanTaskJobs(
	ctx context.Context,
	gateway PlanSeedGateway,
	deps *dependencies,
	planID string,
	testees []*TesteeResponse,
	verbose bool,
	skippedCount *atomic.Int64,
	failedTaskListCount *atomic.Int64,
) ([]planTaskJob, error) {
	taskJobs := make([]planTaskJob, 0, len(testees))
	for _, testee := range testees {
		if testee == nil || strings.TrimSpace(testee.ID) == "" {
			continue
		}
		var taskList *TaskListResponse
		err := runSeedPlanOperationWithRecovery(ctx, deps.Logger, verbose, "list_plan_tasks_for_testee", testee.ID, func() error {
			if err := waitForSeedPlanPacer(ctx, "list_plan_tasks_for_testee"); err != nil {
				return err
			}
			resp, err := gateway.ListTasksByTesteeAndPlan(ctx, testee.ID, planID)
			if err != nil {
				return err
			}
			taskList = resp
			return nil
		})
		if err != nil {
			if failedTaskListCount != nil {
				failedTaskListCount.Add(1)
			}
			if verbose {
				deps.Logger.Warnw("Skipping testee because listing plan tasks failed after recovery attempts",
					"plan_id", planID,
					"testee_id", testee.ID,
					"error", err.Error(),
				)
			}
			continue
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
			case "opened":
				taskJobs = append(taskJobs, planTaskJob{testee: testee, task: task})
			default:
				skippedCount.Add(1)
				if verbose {
					deps.Logger.Warnw("Skipping task with unsupported status",
						"plan_id", planID,
						"testee_id", testee.ID,
						"task_id", task.ID,
						"status", task.Status,
					)
				}
			}
		}
	}
	return taskJobs, nil
}

func inspectExistingPlanTasks(
	ctx context.Context,
	gateway PlanSeedGateway,
	logger interface {
		Warnw(string, ...interface{})
	},
	planID string,
	testees []*TesteeResponse,
	workers int,
	verbose bool,
) (*planTaskStatusStats, error) {
	stats := &planTaskStatusStats{}
	var mu sync.Mutex

	err := runPlanTesteeWorkerPool(ctx, testees, workers, func(ctx context.Context, testee *TesteeResponse) error {
		if testee == nil || strings.TrimSpace(testee.ID) == "" {
			return nil
		}
		var taskList *TaskListResponse
		err := runSeedPlanOperationWithRecovery(ctx, logger, verbose, "inspect_existing_plan_tasks", testee.ID, func() error {
			if err := waitForSeedPlanPacer(ctx, "inspect_existing_plan_tasks"); err != nil {
				return err
			}
			resp, err := gateway.ListTasksByTesteeAndPlan(ctx, testee.ID, planID)
			if err != nil {
				return err
			}
			taskList = resp
			return nil
		})
		if err != nil {
			if verbose {
				logger.Warnw("Skipping testee in existing task inspection after recovery attempts failed",
					"plan_id", planID,
					"testee_id", testee.ID,
					"error", err.Error(),
				)
			}
			return nil
		}
		local := summarizePlanTaskStatuses(taskList.Tasks)
		mu.Lock()
		mergePlanTaskStatusStats(stats, local)
		mu.Unlock()
		return nil
	})
	if err != nil {
		return nil, err
	}
	return stats, nil
}

func summarizePlanTaskStatuses(tasks []TaskResponse) *planTaskStatusStats {
	stats := &planTaskStatusStats{}
	for _, task := range tasks {
		stats.Total++
		switch normalizeTaskStatus(task.Status) {
		case "pending":
			stats.Pending++
		case "opened":
			stats.Opened++
		case "completed":
			stats.Completed++
		case "expired":
			stats.Expired++
		case "canceled":
			stats.Canceled++
		default:
			stats.Unknown++
		}
	}
	return stats
}

func mergePlanTaskStatusStats(dst *planTaskStatusStats, src *planTaskStatusStats) {
	if dst == nil || src == nil {
		return
	}
	dst.Total += src.Total
	dst.Pending += src.Pending
	dst.Opened += src.Opened
	dst.Completed += src.Completed
	dst.Expired += src.Expired
	dst.Canceled += src.Canceled
	dst.Unknown += src.Unknown
}

func expirePlanTaskWithRecovery(ctx context.Context, gateway PlanSeedGateway, orgID int64, task TaskResponse) (*TaskResponse, bool, error) {
	if err := waitForSeedPlanPacer(ctx, "expire_plan_task"); err != nil {
		return nil, false, err
	}
	expiredTask, err := gateway.ExpireTask(ctx, task.ID)
	if err == nil {
		return expiredTask, false, nil
	}

	if err := waitForSeedPlanPacer(ctx, "fetch_expire_plan_task_state"); err != nil {
		return nil, false, err
	}
	currentTask, getErr := gateway.GetTask(ctx, task.ID)
	if getErr != nil {
		return nil, false, fmt.Errorf("expire failed: %w; additionally failed to fetch current task state: %v", err, getErr)
	}

	switch normalizeTaskStatus(currentTask.Status) {
	case "expired", "completed", "canceled":
		return currentTask, true, nil
	default:
		return nil, false, fmt.Errorf("expire failed: %w; current task status=%s org_id=%d", err, currentTask.Status, orgID)
	}
}

func runSeedPlanOperationWithRecovery(
	ctx context.Context,
	logger interface{ Warnw(string, ...interface{}) },
	verbose bool,
	operation string,
	resourceID string,
	fn func() error,
) error {
	if fn == nil {
		return fmt.Errorf("seed plan operation %s is nil", operation)
	}
	var lastErr error
	for attempt := 0; attempt <= seedPlanRecoverableRetries; attempt++ {
		if attempt > 0 {
			delay := seedPlanRecoverableDelay()
			if verbose {
				logger.Warnw("Seed plan recoverable error, waiting before retry",
					"operation", operation,
					"resource_id", resourceID,
					"attempt", attempt,
					"max_attempts", seedPlanRecoverableRetries,
					"delay_seconds", int(delay.Seconds()),
					"error", lastErr.Error(),
				)
			}
			if err := sleepWithContext(ctx, delay); err != nil {
				return err
			}
		}

		if err := fn(); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			lastErr = err
			if !isSeedPlanRecoverableError(err) || attempt == seedPlanRecoverableRetries {
				return err
			}
			continue
		}
		return nil
	}
	return lastErr
}

func isSeedPlanRecoverableError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	recoverablePatterns := []string{
		"context deadline exceeded",
		"client.timeout exceeded",
		"http_status=500",
		"http_status=502",
		"http_status=503",
		"http_status=504",
		"http error: status=500",
		"http error: status=502",
		"http error: status=503",
		"http error: status=504",
		"connection reset by peer",
		"broken pipe",
		"tls handshake timeout",
		"timeout awaiting headers",
		"i/o timeout",
	}
	for _, pattern := range recoverablePatterns {
		if strings.Contains(msg, pattern) {
			return true
		}
	}
	return false
}

func seedPlanRecoverableDelay() time.Duration {
	if seedPlanRecoverableMaxWait <= seedPlanRecoverableMinWait {
		return seedPlanRecoverableMinWait
	}
	span := seedPlanRecoverableMaxWait - seedPlanRecoverableMinWait
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	return seedPlanRecoverableMinWait + time.Duration(rng.Int63n(int64(span)+1))
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

func applyTesteeLimitToIDs(ids []string, limit int) []string {
	if limit <= 0 || len(ids) <= limit {
		return ids
	}
	return append([]string(nil), ids[:limit]...)
}

func loadApiserverTestees(
	ctx context.Context,
	client interface {
		ListTesteesByOrg(context.Context, int64, int, int) (*ApiserverTesteeListResponse, error)
	},
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
	client interface {
		ListTesteesByOrg(context.Context, int64, int, int) (*ApiserverTesteeListResponse, error)
	},
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
	client interface {
		GetTesteeByID(context.Context, string) (*ApiserverTesteeResponse, error)
	},
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

func collectPlanTesteeIDs(testees []*TesteeResponse) []string {
	ids := make([]string, 0, len(testees))
	for _, testee := range testees {
		if testee == nil {
			continue
		}
		id := strings.TrimSpace(testee.ID)
		if id == "" {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

func normalizePlanScheduleBatchSize(workers int) int {
	if workers <= 0 {
		return 1
	}
	size := workers * planScheduleBatchFactor
	if size < workers {
		size = workers
	}
	return size
}

func normalizePlanTaskBufferSize(submitWorkers int, maxInFlightTasks int) int {
	if submitWorkers <= 0 {
		submitWorkers = 1
	}
	size := submitWorkers * planTaskBufferFactor
	if size < submitWorkers {
		size = submitWorkers
	}
	if maxInFlightTasks > size {
		size = maxInFlightTasks
	}
	return size
}

func chunkPlanTestees(testees []*TesteeResponse, batchSize int) [][]*TesteeResponse {
	if len(testees) == 0 {
		return nil
	}
	if batchSize <= 0 {
		batchSize = 1
	}

	batches := make([][]*TesteeResponse, 0, (len(testees)+batchSize-1)/batchSize)
	for start := 0; start < len(testees); start += batchSize {
		end := start + batchSize
		if end > len(testees) {
			end = len(testees)
		}
		batches = append(batches, testees[start:end])
	}
	return batches
}

func mergeTaskScheduleStats(dst *TaskScheduleStatsResponse, src *TaskScheduleStatsResponse) {
	if dst == nil || src == nil {
		return
	}
	dst.PendingCount += src.PendingCount
	dst.OpenedCount += src.OpenedCount
	dst.FailedCount += src.FailedCount
	dst.ExpiredCount += src.ExpiredCount
	dst.ExpireFailedCount += src.ExpireFailedCount
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
		TaskCompletedAt:      seedPlanTaskCompletedAt(task),
		Answers:              answers,
	}
	return req, nil
}

func waitForTaskCompletion(
	ctx context.Context,
	logger interface{ Warnw(string, ...interface{}) },
	client *APIClient,
	orgID int64,
	taskID string,
	verbose bool,
) error {
	deadline := time.NewTimer(planTaskCompletionTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(planTaskCompletionInterval)
	defer ticker.Stop()

	for {
		var task *TaskResponse
		err := runSeedPlanOperationWithRecovery(ctx, logger, verbose, "wait_for_plan_task_completion", taskID, func() error {
			if err := waitForSeedPlanPacer(ctx, "wait_for_plan_task_completion"); err != nil {
				return err
			}
			resp, err := client.GetTask(ctx, taskID)
			if err != nil {
				return err
			}
			task = resp
			return nil
		})
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

func seedPlanTaskCompletedAt(task TaskResponse) string {
	plannedAt, err := time.ParseInLocation(planTaskTimeLayout, strings.TrimSpace(task.PlannedAt), time.Local)
	if err != nil || plannedAt.IsZero() {
		return ""
	}
	return plannedAt.Add(planTaskCompletedOffset).Format(time.RFC3339)
}

type planSeedDashboard struct {
	mu                   sync.Mutex
	enabled              bool
	finished             bool
	rendered             bool
	startedAt            time.Time
	planMode             string
	totalBatches         int
	currentBatch         int
	openedTasks          int
	discoveredTasks      int
	processedTasks       int
	scheduleFailureCount int
	submitted            *atomic.Int64
	completed            *atomic.Int64
	expired              *atomic.Int64
	skipped              *atomic.Int64
	recovered            *atomic.Int64
	inflight             *atomic.Int64
	maxInflight          *atomic.Int64
	failedExecutions     *atomic.Int64
}

func newPlanSeedDashboard(
	planMode string,
	totalBatches int,
	submitted *atomic.Int64,
	completed *atomic.Int64,
	expired *atomic.Int64,
	skipped *atomic.Int64,
	recovered *atomic.Int64,
	inflight *atomic.Int64,
	maxInflight *atomic.Int64,
	failedExecutions *atomic.Int64,
) *planSeedDashboard {
	enabled := isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())
	dashboard := &planSeedDashboard{
		enabled:          enabled,
		startedAt:        time.Now(),
		planMode:         strings.TrimSpace(planMode),
		totalBatches:     totalBatches,
		submitted:        submitted,
		completed:        completed,
		expired:          expired,
		skipped:          skipped,
		recovered:        recovered,
		inflight:         inflight,
		maxInflight:      maxInflight,
		failedExecutions: failedExecutions,
	}
	if dashboard.enabled {
		dashboard.renderLocked()
	}
	return dashboard
}

func (d *planSeedDashboard) SetCurrentBatch(batch int) {
	if d == nil || !d.enabled {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.finished {
		return
	}
	if batch < 0 {
		batch = 0
	}
	if d.totalBatches > 0 && batch > d.totalBatches {
		batch = d.totalBatches
	}
	d.currentBatch = batch
	d.renderLocked()
}

func (d *planSeedDashboard) IncrementScheduleFailures() {
	if d == nil || !d.enabled {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.finished {
		return
	}
	d.scheduleFailureCount++
	d.renderLocked()
}

func (d *planSeedDashboard) AddOpenedTasks(delta int) {
	if d == nil || !d.enabled || delta <= 0 {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.finished {
		return
	}
	d.openedTasks += delta
	d.renderLocked()
}

func (d *planSeedDashboard) AddDiscoveredTasks(delta int) {
	if d == nil || !d.enabled || delta <= 0 {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.finished {
		return
	}
	d.discoveredTasks += delta
	d.renderLocked()
}

func (d *planSeedDashboard) AdvanceTask() {
	if d == nil || !d.enabled {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.finished {
		return
	}
	if d.processedTasks < d.discoveredTasks {
		d.processedTasks++
	}
	d.renderLocked()
}

func (d *planSeedDashboard) Refresh() {
	if d == nil || !d.enabled {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.finished {
		return
	}
	d.renderLocked()
}

func (d *planSeedDashboard) Finish() {
	if d == nil || !d.enabled {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.finished {
		return
	}
	d.currentBatch = d.totalBatches
	d.processedTasks = d.discoveredTasks
	d.renderLocked()
	fmt.Fprintln(os.Stderr)
	d.finished = true
}

func (d *planSeedDashboard) renderLocked() {
	if !d.enabled {
		return
	}

	if d.rendered {
		fmt.Fprintf(os.Stderr, "\x1b[%dF", 3)
	}

	elapsed := time.Since(d.startedAt).Round(time.Second)
	planLine := fmt.Sprintf(
		"plan(%s)      [%s] %d/%d batches elapsed=%s opened=%d schedule_failures=%d",
		d.planModeLabel(),
		renderDashboardBar(d.currentBatch, d.totalBatches, 24),
		d.currentBatch,
		max(d.totalBatches, 0),
		elapsed,
		d.openedTasks,
		d.scheduleFailureCount,
	)

	inflight := atomicLoadInt64(d.inflight)
	maxInflight := atomicLoadInt64(d.maxInflight)
	taskLine := fmt.Sprintf(
		"task-flow(remote) [%s] %d/%d tasks inflight=%d max=%d",
		renderDashboardBar(d.processedTasks, d.discoveredTasks, 24),
		d.processedTasks,
		d.discoveredTasks,
		inflight,
		maxInflight,
	)

	statsLine := fmt.Sprintf(
		"stats          submitted=%d completed=%d expired=%d skipped=%d recovered=%d failed=%d",
		atomicLoadInt64(d.submitted),
		atomicLoadInt64(d.completed),
		atomicLoadInt64(d.expired),
		atomicLoadInt64(d.skipped),
		atomicLoadInt64(d.recovered),
		atomicLoadInt64(d.failedExecutions),
	)

	fmt.Fprintf(os.Stderr, "%s\n%s\n%s", planLine, taskLine, statsLine)
	d.rendered = true
}

func (d *planSeedDashboard) planModeLabel() string {
	mode := strings.ToLower(strings.TrimSpace(d.planMode))
	if mode == "" {
		return "unknown"
	}
	return mode
}

func renderDashboardBar(current, total, width int) string {
	if width <= 0 {
		width = 24
	}
	if total <= 0 {
		total = max(current, 1)
	}
	if current < 0 {
		current = 0
	}
	if current > total {
		current = total
	}
	progressRatio := float64(current) / float64(total)
	filled := int(progressRatio * float64(width))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	return strings.Repeat("=", filled) + strings.Repeat(" ", width-filled)
}

func atomicLoadInt64(counter *atomic.Int64) int64 {
	if counter == nil {
		return 0
	}
	return counter.Load()
}

type planTaskProgressBar struct {
	mu          sync.Mutex
	total       int
	current     int
	label       string
	startedAt   time.Time
	extraStatus func() string
	enabled     bool
	finished    bool
}

func newPlanTaskProgressBar(label string, total int, extraStatus func() string) *planTaskProgressBar {
	enabled := total > 0 && (isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd()))
	bar := &planTaskProgressBar{
		total:       total,
		label:       label,
		startedAt:   time.Now(),
		extraStatus: extraStatus,
		enabled:     enabled,
	}
	if enabled {
		bar.renderLocked()
	}
	return bar
}

func (b *planTaskProgressBar) Advance() {
	if b == nil || !b.enabled {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.finished {
		return
	}
	if b.current < b.total {
		b.current++
	}
	b.renderLocked()
}

func (b *planTaskProgressBar) Finish() {
	if b == nil || !b.enabled {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.finished {
		return
	}
	b.current = b.total
	b.renderLocked()
	fmt.Fprintln(os.Stderr)
	b.finished = true
}

func (b *planTaskProgressBar) Fail() {
	if b == nil || !b.enabled {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.finished {
		return
	}
	b.renderLocked()
	fmt.Fprintln(os.Stderr)
	b.finished = true
}

func (b *planTaskProgressBar) Close() {
	if b == nil || !b.enabled {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.finished {
		return
	}
	fmt.Fprintln(os.Stderr)
	b.finished = true
}

func (b *planTaskProgressBar) renderLocked() {
	if !b.enabled {
		return
	}
	const width = 24
	progressRatio := 1.0
	if b.total > 0 {
		progressRatio = float64(b.current) / float64(b.total)
	}
	filled := int(progressRatio * width)
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("=", filled) + strings.Repeat(" ", width-filled)
	elapsed := time.Since(b.startedAt).Round(time.Second)
	line := fmt.Sprintf("\r%s [%s] %d/%d items elapsed=%s", b.label, bar, b.current, b.total, elapsed)
	if b.extraStatus != nil {
		if extra := strings.TrimSpace(b.extraStatus()); extra != "" {
			line += " " + extra
		}
	}
	fmt.Fprint(os.Stderr, line)
}

func normalizePlanExpireRate(rate float64) float64 {
	switch {
	case rate < 0:
		return 0
	case rate > 1:
		return 1
	default:
		return rate
	}
}

func shouldExpirePlanTask(task TaskResponse, expireRate float64) bool {
	if expireRate <= 0 {
		return false
	}
	if expireRate >= 1 {
		return true
	}

	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.TrimSpace(task.ID)))
	threshold := uint32(expireRate * 10000)
	return h.Sum32()%10000 < threshold
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
