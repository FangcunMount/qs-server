package main

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func seedPlanProcessTasks(
	ctx context.Context,
	deps *dependencies,
	opts planProcessOptions,
) (*seedPlanExecutionStats, error) {
	if deps == nil {
		return nil, fmt.Errorf("dependencies are nil")
	}
	if deps.APIClient == nil {
		return nil, fmt.Errorf("api client is not initialized")
	}

	logger := deps.Logger
	planID := normalizePlanID(opts.PlanID)
	planExpireRate := normalizePlanExpireRate(opts.PlanExpireRate)
	logger.Infow("Plan task processing started",
		"plan_id", planID,
		"org_id", deps.Config.Global.OrgID,
		"plan_workers", opts.PlanWorkers,
		"plan_submit_workers", opts.PlanSubmitWorkers,
		"plan_wait_workers", opts.PlanWaitWorkers,
		"plan_max_inflight_tasks", opts.PlanMaxInFlightTasks,
		"plan_submit_queue_size", opts.PlanSubmitQueueSize,
		"plan_submit_qps", opts.PlanSubmitQPS,
		"plan_submit_burst", opts.PlanSubmitBurst,
		"plan_expire_rate", planExpireRate,
		"scope_testee_count", len(opts.ScopeTesteeIDs),
		"continuous", opts.Continuous,
		"verbose", opts.Verbose,
	)

	session, err := openPlanProcessSession(ctx, deps, planID, opts.Verbose)
	if err != nil {
		return nil, err
	}
	defer session.Close()

	scaleResp, detail, err := loadPlanProcessQuestionnaire(session.ctx, session, opts.Verbose)
	if err != nil {
		return nil, err
	}

	executionStats, err := scheduleAndProcessPlanTasks(
		session.ctx,
		session.gateway,
		deps,
		session.planID,
		session.orgID,
		scaleResp.QuestionnaireVersion,
		detail,
		opts.ScopeTesteeIDs,
		opts.PlanWorkers,
		opts.PlanSubmitWorkers,
		opts.PlanWaitWorkers,
		opts.PlanMaxInFlightTasks,
		opts.PlanSubmitQueueSize,
		opts.PlanSubmitQPS,
		opts.PlanSubmitBurst,
		planExpireRate,
		opts.Verbose,
		opts.Continuous,
	)
	if err != nil {
		return nil, err
	}

	logger.Infow("Plan task processing completed",
		"plan_id", session.planID,
		"org_id", session.orgID,
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
	return executionStats, nil
}

func scheduleAndProcessPlanTasks(
	ctx context.Context,
	gateway planProcessGateway,
	deps *dependencies,
	planID string,
	orgID int64,
	questionnaireVersion string,
	detail *QuestionnaireDetailResponse,
	scopeTesteeIDs []string,
	workers int,
	submitWorkers int,
	waitWorkers int,
	maxInFlightTasks int,
	submitQueueSize int,
	submitQPS float64,
	submitBurst int,
	planExpireRate float64,
	verbose bool,
	continuous bool,
) (*seedPlanExecutionStats, error) {
	var reservedOpenTask atomic.Bool
	submitWorkers, waitWorkers, maxInFlightTasks = normalizePlanTaskExecutionConcurrency(workers, submitWorkers, waitWorkers, maxInFlightTasks)
	taskBufferSize := normalizePlanTaskBufferSize(submitWorkers, maxInFlightTasks)
	submitQueueSize, submitQPS, submitBurst = normalizePlanSubmitQueueConfig(submitWorkers, maxInFlightTasks, submitQueueSize, submitQPS, submitBurst)
	discoveryLimit := max(taskBufferSize, maxInFlightTasks)
	pendingScheduleScopeLimit := normalizePlanScheduleBatchSize(max(submitWorkers, waitWorkers))
	if maxInFlightTasks > 0 && pendingScheduleScopeLimit > maxInFlightTasks {
		pendingScheduleScopeLimit = maxInFlightTasks
	}
	if pendingScheduleScopeLimit <= 0 {
		pendingScheduleScopeLimit = 1
	}
	submitController := newSeedPlanSubmitController()
	submitDispatchController := newSeedPlanSubmitDispatchController(submitQPS, submitBurst)
	aggregateStats := &seedPlanExecutionStats{
		ScheduleStats: &TaskScheduleStatsResponse{},
	}

	if verbose {
		deps.Logger.Infow("Running plan task execution pipeline",
			"plan_id", planID,
			"org_id", orgID,
			"submit_workers", submitWorkers,
			"wait_workers", waitWorkers,
			"max_inflight_tasks", maxInFlightTasks,
			"task_buffer_size", taskBufferSize,
			"submit_queue_size", submitQueueSize,
			"submit_qps", submitQPS,
			"submit_burst", submitBurst,
			"task_page_size", planProcessTaskPageSize,
			"task_window_limit", discoveryLimit,
			"pending_schedule_scope_limit", pendingScheduleScopeLimit,
			"continuous", continuous,
		)
	}

	for cycle := 1; ; cycle++ {
		cycleStats, more, err := runPlanTaskProcessingCycle(
			ctx,
			gateway,
			deps,
			planID,
			orgID,
			questionnaireVersion,
			detail,
			scopeTesteeIDs,
			workers,
			submitWorkers,
			waitWorkers,
			taskBufferSize,
			maxInFlightTasks,
			submitQueueSize,
			discoveryLimit,
			pendingScheduleScopeLimit,
			planExpireRate,
			verbose,
			&reservedOpenTask,
			submitController,
			submitDispatchController,
		)
		if cycleStats != nil {
			mergeSeedPlanExecutionStats(aggregateStats, cycleStats)
		}
		if err != nil {
			return aggregateStats, err
		}
		if !continuous {
			return aggregateStats, nil
		}

		if cycleStats == nil {
			cycleStats = &seedPlanExecutionStats{}
		}
		idle := isSeedPlanExecutionStatsIdle(cycleStats)
		deps.Logger.Infow("Plan task processing cycle completed",
			"plan_id", planID,
			"cycle", cycle,
			"opened_tasks", cycleStats.OpenedCount,
			"submitted_answersheets", cycleStats.SubmittedCount,
			"completed_tasks", cycleStats.CompletedCount,
			"expired_tasks", cycleStats.ExpiredCount,
			"skipped_tasks", cycleStats.SkippedCount,
			"failed_task_executions", cycleStats.FailedTaskExecutions,
			"failed_task_list_loads", cycleStats.FailedTaskListLoads,
			"max_inflight_observed", cycleStats.MaxInFlightObserved,
			"more_opened_tasks_pending", more,
			"idle", idle,
		)

		sleepDuration := planProcessActiveSleep
		switch {
		case more:
			sleepDuration = 0
		case idle:
			sleepDuration = planProcessIdleSleep
		}
		if sleepDuration <= 0 {
			continue
		}
		if err := sleepWithContext(ctx, sleepDuration); err != nil {
			return aggregateStats, err
		}
	}
}

func schedulePlanTaskBatch(
	ctx context.Context,
	gateway planProcessGateway,
	deps *dependencies,
	planID string,
	orgID int64,
	batchIndex int,
	batchCount int,
	testeeIDs []string,
	verbose bool,
) (*TaskListResponse, error) {
	var scheduleResp *TaskListResponse
	resourceID := fmt.Sprintf("batch_%d", batchIndex)
	if len(testeeIDs) == 0 {
		resourceID = planID
	}
	err := runSeedPlanOperationWithRecovery(ctx, deps.Logger, verbose, "schedule_pending_plan_tasks", resourceID, func() error {
		if err := waitForSeedPlanPacer(ctx, "schedule_pending_plan_tasks"); err != nil {
			return err
		}
		resp, err := gateway.SchedulePendingTasks(ctx, SchedulePendingTasksRequest{
			PlanID:    planID,
			TesteeIDs: testeeIDs,
		})
		if err != nil {
			return err
		}
		scheduleResp = resp
		return nil
	})
	if err != nil {
		return nil, err
	}
	if scheduleResp == nil {
		scheduleResp = &TaskListResponse{}
	}

	if verbose {
		deps.Logger.Infow("Scheduled pending plan tasks",
			"plan_id", planID,
			"org_id", orgID,
			"batch_index", batchIndex,
			"batch_count", batchCount,
			"batch_testee_count", len(testeeIDs),
			"opened_count", len(scheduleResp.Tasks),
			"schedule_stats", scheduleResp.Stats,
			"mini_program_delivery", "skipped",
		)
	}
	return scheduleResp, nil
}

func runPlanTaskProcessingCycle(
	ctx context.Context,
	gateway planProcessGateway,
	deps *dependencies,
	planID string,
	orgID int64,
	questionnaireVersion string,
	detail *QuestionnaireDetailResponse,
	scopeTesteeIDs []string,
	workers int,
	submitWorkers int,
	waitWorkers int,
	taskBufferSize int,
	maxInFlightTasks int,
	submitQueueSize int,
	discoveryLimit int,
	pendingScheduleScopeLimit int,
	planExpireRate float64,
	verbose bool,
	reservedOpenTask *atomic.Bool,
	submitController *seedPlanSubmitController,
	submitDispatchController *seedPlanSubmitDispatchController,
) (*seedPlanExecutionStats, bool, error) {
	var submittedCount atomic.Int64
	var skippedCount atomic.Int64
	var completedCount atomic.Int64
	var expiredCount atomic.Int64
	var recoveredCount atomic.Int64
	var failedScheduleBatchCount atomic.Int64
	var failedTaskListCount atomic.Int64
	var failedTaskExecutionCount atomic.Int64
	var inflightCount atomic.Int64
	var maxInflightObserved atomic.Int64

	stats := &seedPlanExecutionStats{
		ScheduleStats: &TaskScheduleStatsResponse{},
	}
	batches := chunkPlanTesteeIDs(scopeTesteeIDs, normalizePlanScheduleBatchSize(workers))
	totalBatches := len(batches)
	if totalBatches == 0 {
		totalBatches = 1
	}
	dashboard := newPlanSeedDashboard(
		totalBatches,
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

	moreDiscovered := false
	runBatch := func(batchIndex int, batchTesteeIDs []string) error {
		dashboard.SetCurrentBatch(batchIndex)

		var (
			taskJobs  []planTaskJob
			batchMore bool
			err       error
		)
		if len(batchTesteeIDs) == 0 {
			taskJobs, batchMore, err = collectPlanTaskJobWindowByPlan(
				ctx,
				gateway,
				deps,
				planID,
				discoveryLimit,
				verbose,
				&skippedCount,
				&failedTaskListCount,
			)
		} else {
			taskJobs, batchMore, err = collectPlanTaskJobWindowForTesteeIDs(
				ctx,
				gateway,
				deps,
				planID,
				batchTesteeIDs,
				discoveryLimit,
				verbose,
				&skippedCount,
				&failedTaskListCount,
			)
		}
		if err != nil {
			return err
		}
		if batchMore {
			moreDiscovered = true
		}
		if len(taskJobs) == 0 {
			var scheduleBatchTesteeIDs []string
			if len(batchTesteeIDs) == 0 {
				scheduleBatchTesteeIDs, batchMore, err = collectSchedulablePendingTesteeWindowByPlan(
					ctx,
					gateway,
					deps,
					planID,
					pendingScheduleScopeLimit,
					verbose,
					&skippedCount,
					&failedTaskListCount,
				)
				if err != nil {
					return err
				}
			} else {
				scheduleBatchTesteeIDs = append([]string(nil), batchTesteeIDs...)
			}
			if batchMore {
				moreDiscovered = true
			}
			if len(scheduleBatchTesteeIDs) == 0 {
				return nil
			}

			scheduleResp, err := schedulePlanTaskBatch(ctx, gateway, deps, planID, orgID, batchIndex, totalBatches, scheduleBatchTesteeIDs, verbose)
			if err != nil {
				failedScheduleBatchCount.Add(1)
				dashboard.IncrementScheduleFailures()
				deps.Logger.Warnw("Skipping schedule batch after recovery attempts failed",
					"plan_id", planID,
					"org_id", orgID,
					"batch_index", batchIndex,
					"batch_count", totalBatches,
					"batch_testee_count", len(scheduleBatchTesteeIDs),
					"error", err.Error(),
				)
				return nil
			}

			stats.OpenedCount += len(scheduleResp.Tasks)
			dashboard.AddOpenedTasks(len(scheduleResp.Tasks))
			mergeTaskScheduleStats(stats.ScheduleStats, scheduleResp.Stats)

			taskJobs = appendPlanTaskJobsFromTasks(nil, scheduleResp.Tasks, "", planID, deps, verbose, &skippedCount)
			if len(taskJobs) > discoveryLimit {
				taskJobs = taskJobs[:discoveryLimit]
				moreDiscovered = true
			}
			if len(taskJobs) == 0 {
				return nil
			}
		}
		dashboard.AddDiscoveredTasks(len(taskJobs))

		return runPlanTaskExecutionPipeline(
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
			submitQueueSize,
			planExpireRate,
			verbose,
			reservedOpenTask,
			&submittedCount,
			&skippedCount,
			&completedCount,
			&expiredCount,
			&recoveredCount,
			&failedTaskExecutionCount,
			submitController,
			submitDispatchController,
			&inflightCount,
			&maxInflightObserved,
			dashboard,
		)
	}

	if len(batches) == 0 {
		if err := runBatch(1, nil); err != nil {
			return nil, false, err
		}
	} else {
		for index, batch := range batches {
			if err := runBatch(index+1, batch); err != nil {
				return nil, false, err
			}
		}
	}

	stats.SubmittedCount = int(submittedCount.Load())
	stats.SkippedCount = int(skippedCount.Load())
	stats.CompletedCount = int(completedCount.Load())
	stats.ExpiredCount = int(expiredCount.Load())
	stats.RecoveredCount = int(recoveredCount.Load())
	stats.MaxInFlightObserved = int(maxInflightObserved.Load())
	stats.FailedScheduleBatches = int(failedScheduleBatchCount.Load())
	stats.FailedTaskListLoads = int(failedTaskListCount.Load())
	stats.FailedTaskExecutions = int(failedTaskExecutionCount.Load())
	return stats, moreDiscovered, nil
}

func mergeSeedPlanExecutionStats(dst *seedPlanExecutionStats, src *seedPlanExecutionStats) {
	if dst == nil || src == nil {
		return
	}
	dst.OpenedCount += src.OpenedCount
	if dst.ScheduleStats == nil {
		dst.ScheduleStats = &TaskScheduleStatsResponse{}
	}
	mergeTaskScheduleStats(dst.ScheduleStats, src.ScheduleStats)
	dst.SubmittedCount += src.SubmittedCount
	dst.SkippedCount += src.SkippedCount
	dst.CompletedCount += src.CompletedCount
	dst.ExpiredCount += src.ExpiredCount
	dst.RecoveredCount += src.RecoveredCount
	if src.MaxInFlightObserved > dst.MaxInFlightObserved {
		dst.MaxInFlightObserved = src.MaxInFlightObserved
	}
	dst.FailedEnrollments += src.FailedEnrollments
	dst.FailedScheduleBatches += src.FailedScheduleBatches
	dst.FailedTaskListLoads += src.FailedTaskListLoads
	dst.FailedTaskExecutions += src.FailedTaskExecutions
}

func isSeedPlanExecutionStatsIdle(stats *seedPlanExecutionStats) bool {
	if stats == nil {
		return true
	}
	return stats.OpenedCount == 0 &&
		stats.SubmittedCount == 0 &&
		stats.CompletedCount == 0 &&
		stats.ExpiredCount == 0 &&
		stats.RecoveredCount == 0 &&
		stats.FailedScheduleBatches == 0 &&
		stats.FailedTaskListLoads == 0 &&
		stats.FailedTaskExecutions == 0
}

func runPlanTaskExecutionPipeline(
	ctx context.Context,
	gateway planProcessGateway,
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
	submitQueueSize int,
	planExpireRate float64,
	verbose bool,
	reservedOpenTask *atomic.Bool,
	submittedCount *atomic.Int64,
	skippedCount *atomic.Int64,
	completedCount *atomic.Int64,
	expiredCount *atomic.Int64,
	recoveredCount *atomic.Int64,
	failedTaskExecutionCount *atomic.Int64,
	submitController *seedPlanSubmitController,
	submitDispatchController *seedPlanSubmitDispatchController,
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
	if planExpireRate >= 1 {
		return runPlanTaskExpireOnlyExecution(
			ctx,
			gateway,
			deps,
			planID,
			orgID,
			jobs,
			submitWorkers,
			verbose,
			reservedOpenTask,
			skippedCount,
			expiredCount,
			recoveredCount,
			failedTaskExecutionCount,
			dashboard,
		)
	}
	if waitWorkers <= 0 {
		waitWorkers = 1
	}
	if taskBufferSize < submitWorkers {
		taskBufferSize = submitWorkers
	}
	if submitQueueSize < submitWorkers {
		submitQueueSize = submitWorkers
	}
	if maxInFlightTasks < submitWorkers {
		maxInFlightTasks = submitWorkers
	}
	if maxInFlightTasks < waitWorkers {
		maxInFlightTasks = waitWorkers
	}

	submitQueueCh := make(chan planTaskJob, submitQueueSize)
	jobCh := make(chan planTaskJob, taskBufferSize)
	waitCh := make(chan planTaskWaitJob, maxInFlightTasks)
	inflightSlots := make(chan struct{}, maxInFlightTasks)

	var dispatchWG sync.WaitGroup
	dispatchWG.Add(1)
	go func() {
		defer dispatchWG.Done()
		defer close(jobCh)
		for {
			select {
			case <-ctx.Done():
				return
			case job, ok := <-submitQueueCh:
				if !ok {
					return
				}
				if err := waitForSeedPlanPacer(ctx, "dispatch_plan_answersheet_submit"); err != nil {
					return
				}
				if err := submitController.Wait(ctx); err != nil {
					return
				}
				if err := submitDispatchController.Wait(ctx); err != nil {
					return
				}
				select {
				case <-ctx.Done():
					return
				case jobCh <- job:
				}
			}
		}
	}()

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
						submitController,
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
						gateway,
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
			close(submitQueueCh)
			dispatchWG.Wait()
			submitWG.Wait()
			close(waitCh)
			waitWG.Wait()
			return ctx.Err()
		case submitQueueCh <- job:
		}
	}

	close(submitQueueCh)
	dispatchWG.Wait()
	submitWG.Wait()
	close(waitCh)
	waitWG.Wait()

	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

func runPlanTaskExpireOnlyExecution(
	ctx context.Context,
	gateway planProcessGateway,
	deps *dependencies,
	planID string,
	orgID int64,
	jobs []planTaskJob,
	workers int,
	verbose bool,
	reservedOpenTask *atomic.Bool,
	skippedCount *atomic.Int64,
	expiredCount *atomic.Int64,
	recoveredCount *atomic.Int64,
	failedTaskExecutionCount *atomic.Int64,
	dashboard *planSeedDashboard,
) error {
	if len(jobs) == 0 {
		return nil
	}
	if workers <= 0 {
		workers = 1
	}
	if workers > len(jobs) {
		workers = len(jobs)
	}

	jobCh := make(chan planTaskJob, workers)
	var workerWG sync.WaitGroup
	for i := 0; i < workers; i++ {
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case job, ok := <-jobCh:
					if !ok {
						return
					}
					handlePlanTaskWithoutSubmission(
						ctx,
						gateway,
						deps,
						planID,
						orgID,
						job,
						1,
						verbose,
						reservedOpenTask,
						skippedCount,
						expiredCount,
						recoveredCount,
						failedTaskExecutionCount,
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
			workerWG.Wait()
			return ctx.Err()
		case jobCh <- job:
		}
	}
	close(jobCh)
	workerWG.Wait()

	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

func handlePlanTaskWithoutSubmission(
	ctx context.Context,
	gateway planProcessGateway,
	deps *dependencies,
	planID string,
	orgID int64,
	job planTaskJob,
	planExpireRate float64,
	verbose bool,
	reservedOpenTask *atomic.Bool,
	skippedCount *atomic.Int64,
	expiredCount *atomic.Int64,
	recoveredCount *atomic.Int64,
	failedTaskExecutionCount *atomic.Int64,
	dashboard *planSeedDashboard,
) bool {
	if strings.TrimSpace(job.testeeID) == "" || strings.TrimSpace(job.task.ID) == "" {
		dashboard.AdvanceTask()
		return true
	}

	if reservedOpenTask != nil && reservedOpenTask.CompareAndSwap(false, true) {
		skippedCount.Add(1)
		if verbose {
			deps.Logger.Infow("Leaving one opened plan task unprocessed to keep plan active",
				"plan_id", planID,
				"testee_id", job.testeeID,
				"task_id", job.task.ID,
				"seq", job.task.Seq,
			)
		}
		dashboard.AdvanceTask()
		return true
	}

	if !shouldExpirePlanTask(job.task, planExpireRate) {
		return false
	}

	err := runSeedPlanOperationWithRecovery(ctx, deps.Logger, verbose, "expire_plan_task", job.task.ID, func() error {
		finalTask, recovered, err := expirePlanTaskWithRecovery(ctx, gateway, orgID, job.task)
		if err != nil {
			return fmt.Errorf("expire task %s for testee %s: %w", job.task.ID, job.testeeID, err)
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
				"testee_id", job.testeeID,
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
				"testee_id", job.testeeID,
				"task_id", job.task.ID,
				"error", err.Error(),
			)
		}
	}
	dashboard.AdvanceTask()
	return true
}

func processPlanTaskSubmitStage(
	ctx context.Context,
	gateway planProcessGateway,
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
	submitController *seedPlanSubmitController,
	inflightSlots chan struct{},
	waitCh chan<- planTaskWaitJob,
	inflightCount *atomic.Int64,
	maxInflightObserved *atomic.Int64,
	dashboard *planSeedDashboard,
) {
	if handlePlanTaskWithoutSubmission(
		ctx,
		gateway,
		deps,
		planID,
		orgID,
		job,
		planExpireRate,
		verbose,
		reservedOpenTask,
		skippedCount,
		expiredCount,
		recoveredCount,
		failedTaskExecutionCount,
		dashboard,
	) {
		return
	}

	req, err := buildPlanSubmissionRequest(detail, questionnaireVersion, job.testeeID, job.task, verbose, deps.Logger)
	if err != nil {
		failedTaskExecutionCount.Add(1)
		if verbose {
			deps.Logger.Warnw("Skipping task because answersheet request build failed",
				"plan_id", planID,
				"org_id", orgID,
				"testee_id", job.testeeID,
				"task_id", job.task.ID,
				"error", err.Error(),
			)
		}
		dashboard.AdvanceTask()
		return
	}

	if verbose {
		logSubmitRequest(deps.Logger, *req, job.testeeID)
	}

	select {
	case <-ctx.Done():
		dashboard.AdvanceTask()
		return
	case inflightSlots <- struct{}{}:
	}

	currentInflight := inflightCount.Add(1)
	updateMaxInFlightCounter(maxInflightObserved, currentInflight)

	releaseSlot := func() {
		<-inflightSlots
		inflightCount.Add(-1)
	}

	attempts, err := submitPlanAnswerSheetWithControlledRetry(ctx, deps.APIClient, *req, planSubmitMaxAttempts)
	if err != nil {
		if isSeedPlanRecoverableError(err) {
			submitController.OnRecoverableError(deps.Logger, planID, orgID, job.task.ID, err)
		}
		releaseSlot()
		failedTaskExecutionCount.Add(1)
		if verbose {
			deps.Logger.Warnw("Skipping task after recovery attempts failed",
				"plan_id", planID,
				"org_id", orgID,
				"testee_id", job.testeeID,
				"task_id", job.task.ID,
				"error", err.Error(),
			)
		}
		dashboard.AdvanceTask()
		return
	}
	submitController.OnSuccess()

	submittedCount.Add(1)

	waitJob := planTaskWaitJob{
		testeeID: job.testeeID,
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
	gateway planProcessGateway,
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

	err := waitForTaskCompletion(ctx, deps.Logger, gateway, orgID, waitJob.task.ID, verbose)
	if err != nil {
		failedTaskExecutionCount.Add(1)
		if verbose {
			deps.Logger.Warnw("Skipping task after completion wait failed",
				"plan_id", planID,
				"org_id", orgID,
				"testee_id", waitJob.testeeID,
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
			"testee_id", waitJob.testeeID,
			"task_id", waitJob.task.ID,
			"seq", waitJob.task.Seq,
			"attempts", waitJob.attempts,
		)
	}
}

func collectPlanTaskJobWindowByPlan(
	ctx context.Context,
	gateway planProcessGateway,
	deps *dependencies,
	planID string,
	discoveryLimit int,
	verbose bool,
	skippedCount *atomic.Int64,
	failedTaskListCount *atomic.Int64,
) ([]planTaskJob, bool, error) {
	return collectPlanTaskJobWindow(
		ctx,
		gateway,
		deps,
		ListPlanTaskWindowRequest{
			PlanID: planID,
			Status: "opened",
		},
		planID,
		discoveryLimit,
		verbose,
		skippedCount,
		failedTaskListCount,
	)
}

func collectPlanTaskJobWindowForTesteeIDs(
	ctx context.Context,
	gateway planProcessGateway,
	deps *dependencies,
	planID string,
	testeeIDs []string,
	discoveryLimit int,
	verbose bool,
	skippedCount *atomic.Int64,
	failedTaskListCount *atomic.Int64,
) ([]planTaskJob, bool, error) {
	if discoveryLimit <= 0 {
		discoveryLimit = planProcessTaskPageSize
	}

	taskJobs := make([]planTaskJob, 0, min(discoveryLimit, planProcessTaskPageSize))
	for _, testeeID := range testeeIDs {
		testeeID = strings.TrimSpace(testeeID)
		if testeeID == "" {
			continue
		}

		remaining := discoveryLimit - len(taskJobs)
		if remaining <= 0 {
			return taskJobs[:discoveryLimit], true, nil
		}

		testeeJobs, more, err := collectPlanTaskJobWindow(
			ctx,
			gateway,
			deps,
			ListPlanTaskWindowRequest{
				PlanID:    planID,
				TesteeIDs: []string{testeeID},
				Status:    "opened",
			},
			planID,
			remaining,
			verbose,
			skippedCount,
			failedTaskListCount,
		)
		if err != nil {
			return nil, false, err
		}
		taskJobs = append(taskJobs, testeeJobs...)
		if len(taskJobs) >= discoveryLimit {
			return taskJobs[:discoveryLimit], true, nil
		}
		if more {
			return taskJobs, true, nil
		}
	}

	return taskJobs, false, nil
}

func collectSchedulablePendingTesteeWindowByPlan(
	ctx context.Context,
	gateway planProcessGateway,
	deps *dependencies,
	planID string,
	scopeLimit int,
	verbose bool,
	skippedCount *atomic.Int64,
	failedTaskListCount *atomic.Int64,
) ([]string, bool, error) {
	if scopeLimit <= 0 {
		scopeLimit = 1
	}

	before := time.Now()
	beforeStr := before.Format("2006-01-02 15:04:05")
	selectedIDs := make([]string, 0, scopeLimit)
	seen := make(map[string]struct{}, scopeLimit)
	resourceID := strings.TrimSpace(planID)
	if resourceID == "" {
		resourceID = "schedulable_pending_plan_tasks"
	}

	for page := 1; len(selectedIDs) < scopeLimit; page++ {
		pageSize := min(planProcessTaskPageSize, scopeLimit-len(selectedIDs))
		if pageSize <= 0 {
			pageSize = 1
		}

		var taskList *PlanTaskWindowResponse
		err := runSeedPlanOperationWithRecovery(ctx, deps.Logger, verbose, "list_schedulable_pending_plan_tasks", resourceID, func() error {
			if err := waitForSeedPlanPacer(ctx, "list_schedulable_pending_plan_tasks"); err != nil {
				return err
			}
			resp, err := gateway.ListPlanTaskWindow(ctx, ListPlanTaskWindowRequest{
				PlanID:        planID,
				Status:        "pending",
				PlannedBefore: beforeStr,
				Page:          page,
				PageSize:      pageSize,
			})
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
			return nil, false, err
		}
		if taskList == nil {
			taskList = &PlanTaskWindowResponse{}
		}

		for _, task := range taskList.Tasks {
			testeeID := strings.TrimSpace(task.TesteeID)
			if testeeID == "" {
				if skippedCount != nil {
					skippedCount.Add(1)
				}
				if verbose {
					deps.Logger.Warnw("Skipping schedulable pending task without testee_id",
						"plan_id", planID,
						"task_id", task.ID,
					)
				}
				continue
			}
			if _, ok := seen[testeeID]; ok {
				continue
			}
			seen[testeeID] = struct{}{}
			selectedIDs = append(selectedIDs, testeeID)
			if len(selectedIDs) >= scopeLimit {
				return selectedIDs[:scopeLimit], true, nil
			}
		}

		if !hasMorePlanTaskWindow(taskList, page, pageSize) {
			return selectedIDs, false, nil
		}
	}

	return selectedIDs, false, nil
}

func collectPlanTaskJobWindow(
	ctx context.Context,
	gateway planProcessGateway,
	deps *dependencies,
	req ListPlanTaskWindowRequest,
	planID string,
	discoveryLimit int,
	verbose bool,
	skippedCount *atomic.Int64,
	failedTaskListCount *atomic.Int64,
) ([]planTaskJob, bool, error) {
	if discoveryLimit <= 0 {
		discoveryLimit = planProcessTaskPageSize
	}

	taskJobs := make([]planTaskJob, 0, min(discoveryLimit, planProcessTaskPageSize))
	resourceID := strings.TrimSpace(req.PlanID)
	if len(req.TesteeIDs) > 0 && strings.TrimSpace(req.TesteeIDs[0]) != "" {
		resourceID = strings.TrimSpace(req.TesteeIDs[0])
	}
	if resourceID == "" {
		resourceID = "opened_plan_tasks"
	}

	for page := 1; len(taskJobs) < discoveryLimit; page++ {
		pageReq := req
		pageReq.Page = page
		pageReq.PageSize = min(planProcessTaskPageSize, discoveryLimit-len(taskJobs))
		if pageReq.PageSize <= 0 {
			pageReq.PageSize = 1
		}

		var taskList *PlanTaskWindowResponse
		err := runSeedPlanOperationWithRecovery(ctx, deps.Logger, verbose, "list_opened_plan_tasks", resourceID, func() error {
			if err := waitForSeedPlanPacer(ctx, "list_opened_plan_tasks"); err != nil {
				return err
			}
			resp, err := gateway.ListPlanTaskWindow(ctx, pageReq)
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
			return nil, false, err
		}
		if taskList == nil {
			taskList = &PlanTaskWindowResponse{}
		}

		fallbackTesteeID := ""
		if len(pageReq.TesteeIDs) == 1 {
			fallbackTesteeID = pageReq.TesteeIDs[0]
		}
		taskJobs = appendPlanTaskJobsFromTasks(taskJobs, taskList.Tasks, fallbackTesteeID, planID, deps, verbose, skippedCount)
		if len(taskJobs) >= discoveryLimit {
			return taskJobs[:discoveryLimit], true, nil
		}
		if !hasMorePlanTaskWindow(taskList, pageReq.Page, pageReq.PageSize) {
			return taskJobs, false, nil
		}
	}

	return taskJobs, false, nil
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

func appendPlanTaskJobsFromTasks(
	taskJobs []planTaskJob,
	tasks []TaskResponse,
	fallbackTesteeID string,
	planID string,
	deps *dependencies,
	verbose bool,
	skippedCount *atomic.Int64,
) []planTaskJob {
	sortedTasks := append([]TaskResponse(nil), tasks...)
	sortTasksBySeq(sortedTasks)

	for _, task := range sortedTasks {
		taskTesteeID := strings.TrimSpace(task.TesteeID)
		if taskTesteeID == "" {
			taskTesteeID = strings.TrimSpace(fallbackTesteeID)
		}

		switch normalizeTaskStatus(task.Status) {
		case "completed", "canceled":
			if skippedCount != nil {
				skippedCount.Add(1)
			}
			if verbose {
				deps.Logger.Debugw("Skipping terminal plan task",
					"plan_id", planID,
					"testee_id", taskTesteeID,
					"task_id", task.ID,
					"status", task.Status,
				)
			}
		case "pending", "expired":
			if skippedCount != nil {
				skippedCount.Add(1)
			}
			if verbose {
				deps.Logger.Debugw("Skipping non-open plan task",
					"plan_id", planID,
					"testee_id", taskTesteeID,
					"task_id", task.ID,
					"status", task.Status,
				)
			}
		case "opened":
			if taskTesteeID == "" {
				if skippedCount != nil {
					skippedCount.Add(1)
				}
				deps.Logger.Warnw("Skipping opened task without testee_id",
					"plan_id", planID,
					"task_id", task.ID,
				)
				continue
			}
			taskJobs = append(taskJobs, planTaskJob{testeeID: taskTesteeID, task: task})
		default:
			if skippedCount != nil {
				skippedCount.Add(1)
			}
			if verbose {
				deps.Logger.Warnw("Skipping task with unsupported status",
					"plan_id", planID,
					"testee_id", taskTesteeID,
					"task_id", task.ID,
					"status", task.Status,
				)
			}
		}
	}
	return taskJobs
}

func expirePlanTaskWithRecovery(ctx context.Context, gateway planProcessGateway, orgID int64, task TaskResponse) (*TaskResponse, bool, error) {
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

func submitPlanAnswerSheetWithControlledRetry(
	ctx context.Context,
	client *APIClient,
	req SubmitAnswerSheetRequest,
	maxAttempts int,
) (int, error) {
	return submitAdminAnswerSheet(ctx, client, req, adminAnswerSheetSubmitPolicy{
		Timeout:      planSubmitRequestTimeout,
		HTTPRetryMax: planSubmitHTTPRetryMax,
		MaxAttempts:  maxAttempts,
		RetryBackoff: planSubmitRetryBackoff,
		Retryable:    isSeedPlanRecoverableError,
	})
}

func buildPlanSubmissionRequest(
	detail *QuestionnaireDetailResponse,
	questionnaireVersion string,
	testeeID string,
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

	testeeID = strings.TrimSpace(testeeID)
	if testeeID == "" {
		testeeID = strings.TrimSpace(task.TesteeID)
	}
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
		logger.Warnw("Invalid answers detected for plan submission",
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

	req := &SubmitAnswerSheetRequest{
		QuestionnaireCode:    detail.Code,
		QuestionnaireVersion: questionnaireVersion,
		Title:                detail.Title,
		TesteeID:             testeeIDUint,
		TaskID:               task.ID,
		Answers:              answers,
	}
	return req, nil
}

func waitForTaskCompletion(
	ctx context.Context,
	logger interface{ Warnw(string, ...interface{}) },
	client interface {
		GetTask(context.Context, string) (*TaskResponse, error)
	},
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
