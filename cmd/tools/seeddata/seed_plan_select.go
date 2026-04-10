package main

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

type planTesteeTaskPriority struct {
	TotalTaskCount       int
	CurrentPlanTaskCount int
}

type planTesteeSelectionResult struct {
	SelectedTestees   []*TesteeResponse
	LoadedTesteeCount int
	SelectionMode     string
	SampleRate        string
	ExistingStats     *planTaskStatusStats
}

type planTesteeSelector interface {
	Select(context.Context, *planSeedSession) (*planTesteeSelectionResult, error)
}

type explicitPlanTesteeSelector struct {
	opts      planCreateOptions
	testeeIDs []string
}

type sampledPriorityPlanTesteeSelector struct {
	opts planCreateOptions
}

func newPlanTesteeSelector(opts planCreateOptions, explicitPlanTesteeIDs []string) planTesteeSelector {
	if len(explicitPlanTesteeIDs) > 0 {
		return explicitPlanTesteeSelector{
			opts:      opts,
			testeeIDs: append([]string(nil), explicitPlanTesteeIDs...),
		}
	}
	return sampledPriorityPlanTesteeSelector{opts: opts}
}

func (s explicitPlanTesteeSelector) Select(ctx context.Context, session *planSeedSession) (*planTesteeSelectionResult, error) {
	selected, err := loadExplicitPlanTestees(ctx, session.gateway, s.testeeIDs)
	if err != nil {
		return nil, err
	}
	return &planTesteeSelectionResult{
		SelectedTestees:   selected,
		LoadedTesteeCount: len(selected),
		SelectionMode:     "explicit",
		SampleRate:        "all",
	}, nil
}

func (s sampledPriorityPlanTesteeSelector) Select(ctx context.Context, session *planSeedSession) (*planTesteeSelectionResult, error) {
	pageSize := s.opts.TesteePageSize
	if pageSize < 100 {
		pageSize = 100
	}
	sampled, loadedCount, err := streamSamplePlanEnrollmentTestees(
		ctx,
		session.gateway,
		session.orgID,
		pageSize,
		s.opts.TesteeOffset,
		s.opts.TesteeLimit,
		session.planID,
	)
	if err != nil {
		return nil, err
	}
	prioritized, err := prioritizePlanEnrollmentTestees(
		ctx,
		session.gateway,
		session.logger,
		session.planID,
		sampled,
		s.opts.PlanWorkers,
		s.opts.Verbose,
	)
	if err != nil {
		return nil, err
	}
	return &planTesteeSelectionResult{
		SelectedTestees:   prioritized,
		LoadedTesteeCount: loadedCount,
		SelectionMode:     "priority_stream_sample",
		SampleRate:        fmt.Sprintf("1/%d", planEnrollmentSampleRate),
	}, nil
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

func prioritizePlanEnrollmentTestees(
	ctx context.Context,
	gateway PlanSeedGateway,
	logger interface {
		Warnw(string, ...interface{})
		Infow(string, ...interface{})
	},
	planID string,
	testees []*TesteeResponse,
	workers int,
	verbose bool,
) ([]*TesteeResponse, error) {
	if len(testees) == 0 {
		return nil, nil
	}

	taskPriorities, err := loadPlanTaskPriorityForTestees(ctx, gateway, logger, planID, testees, workers, verbose)
	if err != nil {
		return nil, err
	}

	prioritized := append([]*TesteeResponse(nil), testees...)
	sort.SliceStable(prioritized, func(i, j int) bool {
		left := prioritized[i]
		right := prioritized[j]
		if left == nil {
			return false
		}
		if right == nil {
			return true
		}

		leftPriority := planTaskPriorityForSort(taskPriorities, left.ID)
		rightPriority := planTaskPriorityForSort(taskPriorities, right.ID)

		leftHasNoTasks := leftPriority.TotalTaskCount == 0
		rightHasNoTasks := rightPriority.TotalTaskCount == 0
		if leftHasNoTasks != rightHasNoTasks {
			return leftHasNoTasks
		}

		leftHasNoCurrentPlanTask := leftPriority.CurrentPlanTaskCount == 0
		rightHasNoCurrentPlanTask := rightPriority.CurrentPlanTaskCount == 0
		if leftHasNoCurrentPlanTask != rightHasNoCurrentPlanTask {
			return leftHasNoCurrentPlanTask
		}

		if leftPriority.TotalTaskCount != rightPriority.TotalTaskCount {
			return leftPriority.TotalTaskCount < rightPriority.TotalTaskCount
		}
		if leftPriority.CurrentPlanTaskCount != rightPriority.CurrentPlanTaskCount {
			return leftPriority.CurrentPlanTaskCount < rightPriority.CurrentPlanTaskCount
		}
		if left.CreatedAt.Equal(right.CreatedAt) {
			return parseID(left.ID) < parseID(right.ID)
		}
		return left.CreatedAt.Before(right.CreatedAt)
	})
	return prioritized, nil
}

func loadPlanTaskPriorityForTestees(
	ctx context.Context,
	gateway PlanSeedGateway,
	logger interface {
		Warnw(string, ...interface{})
		Infow(string, ...interface{})
	},
	planID string,
	testees []*TesteeResponse,
	workers int,
	verbose bool,
) (map[string]planTesteeTaskPriority, error) {
	if len(testees) == 0 {
		return map[string]planTesteeTaskPriority{}, nil
	}

	if provider, ok := gateway.(planTaskPriorityProvider); ok {
		if err := waitForSeedPlanPacer(ctx, "load_plan_task_priority_for_testees"); err != nil {
			return nil, err
		}
		stats, err := provider.GetPlanTaskPriorityByTesteeIDs(ctx, planID, collectPlanTesteeIDs(testees))
		if err == nil {
			return stats, nil
		}
		if verbose {
			logger.Warnw("Falling back to per-testee task priority counting after batch counting failed",
				"plan_id", planID,
				"error", err.Error(),
			)
		}
	}

	stats := make(map[string]planTesteeTaskPriority, len(testees))
	var mu sync.Mutex
	workers = normalizePlanWorkers(workers, len(testees))
	err := runPlanTesteeWorkerPool(ctx, testees, workers, func(ctx context.Context, testee *TesteeResponse) error {
		if testee == nil || strings.TrimSpace(testee.ID) == "" {
			return nil
		}
		var taskList *TaskListResponse
		err := runSeedPlanOperationWithRecovery(ctx, logger, verbose, "load_task_priority_for_testee", testee.ID, func() error {
			if err := waitForSeedPlanPacer(ctx, "load_task_priority_for_testee"); err != nil {
				return err
			}
			resp, err := gateway.ListTasksByTestee(ctx, testee.ID)
			if err != nil {
				return err
			}
			taskList = resp
			return nil
		})
		if err != nil {
			if verbose {
				logger.Warnw("Unable to determine task priority for testee, treating as lowest priority",
					"plan_id", planID,
					"testee_id", testee.ID,
					"error", err.Error(),
				)
			}
			return nil
		}

		if taskList == nil {
			taskList = &TaskListResponse{}
		}
		priority := summarizePlanTesteeTaskPriority(planID, taskList.Tasks)
		mu.Lock()
		stats[testee.ID] = priority
		mu.Unlock()
		return nil
	})
	if err != nil {
		return nil, err
	}

	return stats, nil
}

func summarizePlanTesteeTaskPriority(planID string, tasks []TaskResponse) planTesteeTaskPriority {
	priority := planTesteeTaskPriority{
		TotalTaskCount:       0,
		CurrentPlanTaskCount: 0,
	}
	planID = strings.TrimSpace(planID)
	for _, task := range tasks {
		priority.TotalTaskCount++
		if strings.TrimSpace(task.PlanID) == planID {
			priority.CurrentPlanTaskCount++
		}
	}
	return priority
}

func planTaskPriorityForSort(stats map[string]planTesteeTaskPriority, testeeID string) planTesteeTaskPriority {
	if stats == nil {
		return planTesteeTaskPriority{
			TotalTaskCount:       planEnrollmentUnknownCount,
			CurrentPlanTaskCount: planEnrollmentUnknownCount,
		}
	}
	if stat, ok := stats[strings.TrimSpace(testeeID)]; ok {
		return stat
	}
	return planTesteeTaskPriority{
		TotalTaskCount:       planEnrollmentUnknownCount,
		CurrentPlanTaskCount: planEnrollmentUnknownCount,
	}
}

func selectPlanEnrollmentTestees(prioritized []*TesteeResponse) []*TesteeResponse {
	if len(prioritized) == 0 {
		return nil
	}

	targetCount := len(prioritized) / planEnrollmentSampleRate
	if targetCount <= 0 {
		targetCount = 1
	}
	if targetCount > len(prioritized) {
		targetCount = len(prioritized)
	}
	return append([]*TesteeResponse(nil), prioritized[:targetCount]...)
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

func chunkPlanTesteeIDs(testeeIDs []string, batchSize int) [][]string {
	if len(testeeIDs) == 0 {
		return nil
	}
	if batchSize <= 0 {
		batchSize = 1
	}

	batches := make([][]string, 0, (len(testeeIDs)+batchSize-1)/batchSize)
	for start := 0; start < len(testeeIDs); start += batchSize {
		end := start + batchSize
		if end > len(testeeIDs) {
			end = len(testeeIDs)
		}
		batches = append(batches, append([]string(nil), testeeIDs[start:end]...))
	}
	return batches
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
