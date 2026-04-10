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
	_ *seedContext,
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
		"plan_mode", opts.PlanMode,
		"org_id", deps.Config.Global.OrgID,
		"plan_workers", opts.PlanWorkers,
		"plan_process_existing_only", opts.PlanProcessExistingOnly,
		"testee_page_size", opts.TesteePageSize,
		"testee_offset", opts.TesteeOffset,
		"testee_limit", opts.TesteeLimit,
		"verbose", opts.Verbose,
	)

	session, err := openPlanSeedSession(ctx, deps, planID, opts.PlanMode, opts.Verbose)
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
		existingStats = selection.ExistingStats
		loadedTesteeCnt = selection.LoadedTesteeCount
	}

	if selection != nil && selection.RecoveryFilterStats != nil {
		filterStats := selection.RecoveryFilterStats
		logger.Infow("Inspected existing plan tasks before task creation",
			"plan_id", session.planID,
			"org_id", session.orgID,
			"selected_testee_count", loadedTesteeCnt,
			"existing_task_stats", existingStats,
		)
		logger.Infow("Filtered recovery-mode plan testees before task creation",
			"plan_id", session.planID,
			"org_id", session.orgID,
			"selected_testee_count", loadedTesteeCnt,
			"retained_testee_count", len(selectedTestees),
			"filtered_completed_plan_testees", filterStats.FilteredCompletedPlanTestees,
			"filtered_no_task_testees", filterStats.FilteredNoTaskTestees,
			"retained_undetermined_testees", filterStats.RetainedUndeterminedTestees,
		)
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
	if len(selectedTestees) == 0 && !opts.PlanProcessExistingOnly {
		logger.Infow("No testees found for plan task creation", "plan_id", session.planID, "org_id", session.orgID)
		return &seedPlanCreateResult{}, nil
	}

	inspectionWorkers := normalizePlanWorkers(opts.PlanWorkers, len(selectedTestees))
	if opts.PlanProcessExistingOnly {
		if existingStats == nil || existingStats.Total == 0 {
			logger.Infow("No existing plan tasks found for recovery mode",
				"plan_id", session.planID,
				"org_id", session.orgID,
				"selected_testee_count", loadedTesteeCnt,
			)
			return &seedPlanCreateResult{}, nil
		}
		if len(selectedTestees) == 0 {
			logger.Infow("All recovery-mode testees were filtered out before scheduling",
				"plan_id", session.planID,
				"org_id", session.orgID,
				"selected_testee_count", loadedTesteeCnt,
			)
			return &seedPlanCreateResult{}, nil
		}
	} else {
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
	}

	enrolledCount := 0
	failedEnrollments := 0
	if opts.PlanProcessExistingOnly {
		logger.Infow("Skipping plan enrollment because recovery mode is enabled",
			"plan_id", session.planID,
			"org_id", session.orgID,
			"selected_testee_count", len(selectedTestees),
		)
	} else {
		enrolledCount, failedEnrollments, err = enrollPlanTesteesConcurrently(session.ctx, session.gateway, deps.Logger, session.planID, selectedTestees, opts.PlanWorkers, opts.Verbose)
		if err != nil {
			return nil, err
		}
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
