package main

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
)

func seedPlanBackfill(
	ctx context.Context,
	deps *dependencies,
	createOpts planCreateOptions,
	processOpts planProcessOptions,
) error {
	createResult, err := seedPlanCreateTasks(ctx, deps, createOpts)
	if err != nil {
		return err
	}
	if createResult == nil || !createResult.ShouldProcess {
		return nil
	}

	_, err = seedPlanProcessTasks(ctx, deps, processOpts.withScope(createResult.ScopeTesteeIDs, false))
	return err
}

func seedPlanCreateTasks(
	ctx context.Context,
	deps *dependencies,
	opts planCreateOptions,
) (*seedPlanCreateResult, error) {
	logger := deps.Logger
	planID := normalizePlanID(opts.PlanID)
	logger.Infow("Plan task creation started",
		"plan_id", planID,
		"org_id", deps.Config.Global.OrgID,
		"plan_workers", opts.PlanWorkers,
		"testee_page_size", opts.TesteePageSize,
		"testee_offset", opts.TesteeOffset,
		"testee_limit", opts.TesteeLimit,
		"verbose", opts.Verbose,
	)

	session, err := openPlanSeedSession(ctx, deps, planID, opts.Verbose)
	if err != nil {
		return nil, err
	}
	defer session.Close()

	explicitPlanTesteeIDs := applyTesteeLimitToIDs(parsePlanTesteeIDs(opts.PlanTesteeIDsRaw), opts.TesteeLimit)
	selector := newPlanTesteeSelector(opts, explicitPlanTesteeIDs)
	if selector == nil {
		return nil, fmt.Errorf("plan testee selector is nil")
	}
	selection, err := selector.Select(session.ctx, session)
	if err != nil {
		return nil, err
	}

	var (
		selectedTestees []*TesteeResponse
		selectionMode   string
		sampleRate      string
		existingStats   *planTaskStatusStats
		loadedTesteeCnt int
	)
	if selection != nil {
		selectedTestees = selection.SelectedTestees
		selectionMode = selection.SelectionMode
		sampleRate = selection.SampleRate
		loadedTesteeCnt = selection.LoadedTesteeCount
	}

	logger.Infow("Loaded testees for plan task creation",
		"plan_id", session.planID,
		"org_id", session.orgID,
		"loaded_testee_count", loadedTesteeCnt,
		"selected_testee_count", len(selectedTestees),
		"selection_mode", selectionMode,
		"sample_rate", sampleRate,
		"explicit_testee_ids", explicitPlanTesteeIDs,
	)
	if len(selectedTestees) == 0 {
		logger.Infow("No testees found for plan task creation", "plan_id", session.planID, "org_id", session.orgID)
		return &seedPlanCreateResult{}, nil
	}

	inspectionWorkers := normalizePlanWorkers(opts.PlanWorkers, len(selectedTestees))
	existingStats, err = inspectExistingPlanTasks(session.ctx, session.gateway, deps.Logger, session.planID, selectedTestees, inspectionWorkers, opts.Verbose)
	if err != nil {
		return nil, err
	}
	logger.Infow("Inspected existing plan tasks before task creation",
		"plan_id", session.planID,
		"org_id", session.orgID,
		"selected_testee_count", len(selectedTestees),
		"existing_task_stats", existingStats,
	)

	enrolledCount, failedEnrollments, err := enrollPlanTesteesConcurrently(session.ctx, session.gateway, deps.Logger, session.planID, selectedTestees, opts.PlanWorkers, opts.Verbose)
	if err != nil {
		return nil, err
	}

	logger.Infow("Plan task creation completed",
		"plan_id", session.planID,
		"org_id", session.orgID,
		"enrolled_testees", enrolledCount,
		"failed_enrollments", failedEnrollments,
		"selected_testee_count", len(selectedTestees),
		"existing_task_stats", existingStats,
	)

	return &seedPlanCreateResult{
		ShouldProcess:  len(selectedTestees) > 0,
		ScopeTesteeIDs: collectPlanTesteeIDs(selectedTestees),
	}, nil
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
