package plansubmit

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	toolprogress "github.com/FangcunMount/qs-server/tools/seeddata-runner/internal/progress"
)

const (
	planSubmitOpenTasksDaemonIdleSleep   = 30 * time.Second
	planSubmitOpenTasksDaemonActiveSleep = 5 * time.Second
)

type planTaskSubmitRunner struct {
	session   *planTaskSubmitSession
	detail    *QuestionnaireDetailResponse
	tracker   *recentPlanTaskTracker
	scaleResp *ScaleResponse
}

func seedPlanSubmitOpenTasksDaemon(
	ctx context.Context,
	deps *dependencies,
	opts planOpenTaskSubmitOptions,
) (*planOpenTaskSubmitStats, error) {
	if deps == nil {
		return nil, fmt.Errorf("dependencies are nil")
	}
	if deps.APIClient == nil {
		return nil, fmt.Errorf("api client is not initialized")
	}

	planIDs := normalizePlanIDs(opts.PlanIDs)
	if len(planIDs) == 0 {
		return nil, fmt.Errorf("plan-ids are required for the open-task submit daemon")
	}

	logger := deps.Logger
	runners := make([]planTaskSubmitRunner, 0, len(planIDs))
	for _, planID := range planIDs {
		session, err := openPlanTaskSubmitSession(ctx, deps, planID, opts.Verbose)
		if err != nil {
			return nil, err
		}
		scaleResp, detail, err := loadPlanTaskSubmitQuestionnaire(ctx, session, opts.Verbose)
		if err != nil {
			return nil, err
		}
		logger.Infow("Plan opened-task answersheet daemon started",
			"plan_id", session.planID,
			"org_id", session.orgID,
			"scale_code", session.plan.ScaleCode,
			"questionnaire_code", scaleResp.QuestionnaireCode,
			"questionnaire_version", scaleResp.QuestionnaireVersion,
			"workers", opts.Workers,
			"continuous", opts.Continuous,
		)
		runners = append(runners, planTaskSubmitRunner{
			session:   session,
			detail:    detail,
			tracker:   newRecentPlanTaskTracker(planOpenTaskRecentSubmitTTL),
			scaleResp: scaleResp,
		})
	}

	aggregate := &planOpenTaskSubmitStats{}
	for cycle := 1; ; cycle++ {
		cycleStats := &planOpenTaskSubmitStats{}
		for _, runner := range runners {
			runnerStats, err := runPlanSubmitOpenTasksCycle(
				ctx,
				runner.session.gateway,
				deps.APIClient,
				logger,
				runner.session.planID,
				runner.scaleResp.QuestionnaireVersion,
				runner.detail,
				opts.Workers,
				runner.tracker,
				opts.Verbose,
			)
			if runnerStats != nil {
				mergePlanOpenTaskSubmitStats(cycleStats, runnerStats)
				mergePlanOpenTaskSubmitStats(aggregate, runnerStats)
			}
			if err != nil {
				return aggregate, err
			}
			if runnerStats == nil {
				runnerStats = &planOpenTaskSubmitStats{}
			}

			logger.Infow("Plan opened-task answersheet cycle completed",
				"plan_id", runner.session.planID,
				"cycle", cycle,
				"opened_tasks", runnerStats.OpenedCount,
				"submitted_answersheets", runnerStats.SubmittedCount,
				"skipped_tasks", runnerStats.SkippedCount,
				"failed_task_list_loads", runnerStats.FailedTaskListLoads,
				"failed_task_executions", runnerStats.FailedTaskExecutions,
			)
		}

		if !opts.Continuous {
			return aggregate, nil
		}

		sleepDuration := planSubmitOpenTasksDaemonIdleSleep
		if cycleStats.OpenedCount > 0 || cycleStats.SubmittedCount > 0 {
			sleepDuration = planSubmitOpenTasksDaemonActiveSleep
		}
		if err := sleepWithContext(ctx, sleepDuration); err != nil {
			return aggregate, err
		}
	}
}

func runPlanSubmitOpenTasksCycle(
	ctx context.Context,
	gateway planTaskSubmitGateway,
	submitClient adminAnswerSheetSubmitClient,
	logger planTaskLogger,
	planID string,
	questionnaireVersion string,
	detail *QuestionnaireDetailResponse,
	workers int,
	tracker *recentPlanTaskTracker,
	verbose bool,
) (*planOpenTaskSubmitStats, error) {
	stats := &planOpenTaskSubmitStats{}
	jobs, err := listOpenPlanTaskJobs(ctx, gateway, logger, planID, verbose)
	if err != nil {
		stats.FailedTaskListLoads = 1
		return stats, err
	}

	stats.OpenedCount = len(jobs)
	if len(jobs) == 0 {
		return stats, nil
	}

	workers = normalizePlanWorkers(workers, len(jobs))
	if workers <= 0 {
		workers = 1
	}

	progress := toolprogress.New("plan_submit_open_tasks_daemon tasks", len(jobs))
	defer progress.Close()

	jobCh := make(chan planTaskJob, len(jobs))
	var submittedCount atomic.Int64
	var skippedCount atomic.Int64
	var failedTaskExecutionCount atomic.Int64
	var workerWG sync.WaitGroup

	for i := 0; i < workers; i++ {
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			for job := range jobCh {
				processOpenPlanTaskSubmission(
					ctx,
					submitClient,
					logger,
					planID,
					questionnaireVersion,
					detail,
					job,
					tracker,
					verbose,
					&submittedCount,
					&skippedCount,
					&failedTaskExecutionCount,
					progress,
				)
			}
		}()
	}

	for _, job := range jobs {
		select {
		case <-ctx.Done():
			close(jobCh)
			workerWG.Wait()
			return stats, ctx.Err()
		case jobCh <- job:
		}
	}

	close(jobCh)
	workerWG.Wait()
	progress.Complete()

	stats.SubmittedCount = int(submittedCount.Load())
	stats.SkippedCount = int(skippedCount.Load())
	stats.FailedTaskExecutions = int(failedTaskExecutionCount.Load())
	return stats, nil
}

func processOpenPlanTaskSubmission(
	ctx context.Context,
	submitClient adminAnswerSheetSubmitClient,
	logger planTaskLogger,
	planID string,
	questionnaireVersion string,
	detail *QuestionnaireDetailResponse,
	job planTaskJob,
	tracker *recentPlanTaskTracker,
	verbose bool,
	submittedCount *atomic.Int64,
	skippedCount *atomic.Int64,
	failedTaskExecutionCount *atomic.Int64,
	progress *toolprogress.Bar,
) {
	defer progress.Increment()

	select {
	case <-ctx.Done():
		return
	default:
	}

	if tracker != nil && tracker.Seen(job.task.ID) {
		skippedCount.Add(1)
		if verbose {
			logger.Debugw("Skipping recently submitted opened task",
				"plan_id", planID,
				"task_id", job.task.ID,
				"testee_id", job.testeeID,
			)
		}
		return
	}

	req, err := buildPlanTaskSubmitRequest(detail, questionnaireVersion, job.task, verbose, logger)
	if err != nil {
		failedTaskExecutionCount.Add(1)
		logger.Warnw("Skipping opened task because answersheet request build failed",
			"plan_id", planID,
			"task_id", job.task.ID,
			"testee_id", job.testeeID,
			"error", err.Error(),
		)
		return
	}

	if verbose {
		logSubmitRequest(logger, *req, job.testeeID)
	}

	attempts, err := submitPlanTaskAnswerSheet(ctx, submitClient, *req)
	if err != nil {
		failedTaskExecutionCount.Add(1)
		logger.Warnw("Opened plan task answersheet submit failed",
			"plan_id", planID,
			"task_id", job.task.ID,
			"testee_id", job.testeeID,
			"error", err.Error(),
		)
		return
	}

	if tracker != nil {
		tracker.Remember(job.task.ID)
	}
	submittedCount.Add(1)
	if verbose {
		logger.Infow("Opened plan task answersheet submitted",
			"plan_id", planID,
			"task_id", job.task.ID,
			"testee_id", job.testeeID,
			"attempts", attempts,
		)
	}
}

func mergePlanOpenTaskSubmitStats(dst *planOpenTaskSubmitStats, src *planOpenTaskSubmitStats) {
	if dst == nil || src == nil {
		return
	}
	dst.OpenedCount += src.OpenedCount
	dst.SubmittedCount += src.SubmittedCount
	dst.SkippedCount += src.SkippedCount
	dst.FailedTaskListLoads += src.FailedTaskListLoads
	dst.FailedTaskExecutions += src.FailedTaskExecutions
}
