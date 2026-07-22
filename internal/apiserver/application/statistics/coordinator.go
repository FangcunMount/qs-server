package statistics

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	statisticsDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
)

const (
	maxManualWindowDays = 31
	maxRunReasonRunes   = 500
)

type Run struct {
	ID                        uint64                        `json:"id"`
	OrgID                     int64                         `json:"org_id"`
	BatchKey                  string                        `json:"batch_key"`
	Attempt                   uint32                        `json:"attempt"`
	TriggerType               string                        `json:"trigger_type"`
	Mode                      statisticsDomain.RunMode      `json:"mode"`
	Window                    statisticsDomain.InstantRange `json:"-"`
	AsOfDate                  time.Time                     `json:"as_of_date"`
	CacheGeneration           int64                         `json:"cache_generation"`
	CachePublishedAt          *time.Time                    `json:"cache_published_at,omitempty"`
	Status                    statisticsDomain.RunStatus    `json:"status"`
	Stage                     string                        `json:"stage"`
	Reason                    string                        `json:"reason"`
	OperatorID                uint64                        `json:"operator_id,omitempty"`
	StartedAt                 time.Time                     `json:"started_at"`
	DataCommittedAt           *time.Time                    `json:"data_committed_at,omitempty"`
	FinishedAt                *time.Time                    `json:"finished_at,omitempty"`
	SourceCounts              map[string]int64              `json:"source_counts,omitempty"`
	FactCounts                map[string]int64              `json:"fact_counts,omitempty"`
	ResultCounts              map[string]int64              `json:"result_counts,omitempty"`
	ErrorCode                 string                        `json:"error_code,omitempty"`
	ErrorMessage              string                        `json:"error_message,omitempty"`
	CacheResumeCount          uint32                        `json:"cache_resume_count,omitempty"`
	LastCacheResumeOperatorID uint64                        `json:"last_cache_resume_operator_id,omitempty"`
	LastCacheResumeReason     string                        `json:"last_cache_resume_reason,omitempty"`
	LastCacheResumeAt         *time.Time                    `json:"last_cache_resume_at,omitempty"`
	LastCacheResumeStatus     string                        `json:"last_cache_resume_status,omitempty"`
}

type RunStore interface {
	Create(context.Context, Run) (*Run, error)
	UpdateProgress(context.Context, uint64, string, map[string]int64, map[string]int64, map[string]int64) error
	AssertPublishable(context.Context, int64, time.Time) error
	MarkDataCommitted(context.Context, uint64, time.Time) error
	MarkCachePublished(context.Context, uint64, int64, time.Time) error
	MarkCachePublishFailed(context.Context, uint64, int64, string, time.Time) error
	RecordCacheResume(context.Context, uint64, uint64, string, string, int64, time.Time) error
	MarkSucceeded(context.Context, uint64, time.Time) error
	MarkFailed(context.Context, uint64, string, string, string, time.Time) error
	Get(context.Context, uint64) (*Run, error)
	List(context.Context, int64, int) ([]Run, error)
}

type CachePublisher interface {
	Publish(context.Context, int64, time.Time) (int64, error)
}

type CacheResumeRequest struct {
	OperatorID uint64
	Reason     string
}

type RunRequest struct {
	OrgID               int64
	FromDate, ToDate    time.Time
	Reason, TriggerType string
	OperatorID          uint64
	Mode                statisticsDomain.RunMode
	// ValidateOnly is the compatibility input for the original internal API.
	// New callers must set Mode explicitly.
	ValidateOnly bool
}

type runExecutionError struct {
	stage string
	code  string
	err   error
}

type InvalidRunRequestError struct{ message string }

func (e *InvalidRunRequestError) Error() string { return e.message }

func invalidRunRequest(format string, args ...any) error {
	return &InvalidRunRequestError{message: fmt.Sprintf(format, args...)}
}

func IsInvalidRunRequest(err error) bool {
	var target *InvalidRunRequestError
	return errors.As(err, &target)
}

func (e *runExecutionError) Error() string { return e.err.Error() }
func (e *runExecutionError) Unwrap() error { return e.err }

func executionError(stage, code string, err error) error {
	if err == nil {
		return nil
	}
	return &runExecutionError{stage: stage, code: code, err: err}
}

type Coordinator struct {
	collectors   *statisticsDomain.CollectorSet
	dailyEngine  *statisticsDomain.ProjectionEngine
	globalEngine *statisticsDomain.ProjectionEngine
	store        RunStore
	tx           apptransaction.Runner
	locks        locklease.Runner
	cache        CachePublisher
	now          func() time.Time
}

func NewCoordinator(
	collectors *statisticsDomain.CollectorSet,
	dailyEngine *statisticsDomain.ProjectionEngine,
	globalEngine *statisticsDomain.ProjectionEngine,
	store RunStore,
	tx apptransaction.Runner,
	locks locklease.Runner,
	cache CachePublisher,
) *Coordinator {
	return &Coordinator{
		collectors: collectors, dailyEngine: dailyEngine, globalEngine: globalEngine,
		store: store, tx: tx, locks: locks, cache: cache, now: time.Now,
	}
}

func (c *Coordinator) Run(ctx context.Context, request RunRequest) (resultRun *Run, resultErr error) {
	if request.OrgID <= 0 {
		return nil, invalidRunRequest("org_id is required")
	}
	if utf8.RuneCountInString(request.Reason) > maxRunReasonRunes {
		return nil, invalidRunRequest("reason exceeds %d characters", maxRunReasonRunes)
	}
	mode, err := normalizeRunMode(request.Mode, request.ValidateOnly)
	if err != nil {
		return nil, err
	}
	metricStart := time.Now()
	defer func() { observeStatisticsRun(metricStart, mode, request.TriggerType, resultRun, resultErr) }()
	if c.collectors == nil || c.dailyEngine == nil || c.globalEngine == nil || c.store == nil || c.tx == nil || c.locks == nil {
		return nil, fmt.Errorf("statistics coordinator is not fully configured")
	}

	now := c.now()
	latestCompleteDay := statisticsDomain.BusinessDate(now).AddDate(0, 0, -1)
	if request.FromDate.IsZero() || request.ToDate.IsZero() {
		return nil, invalidRunRequest("from_date and to_date are required")
	}
	fromDate := statisticsDomain.BusinessDate(request.FromDate)
	toDate := statisticsDomain.BusinessDate(request.ToDate)
	if toDate.After(latestCompleteDay) {
		return nil, invalidRunRequest("statistics window cannot exceed latest complete business day %s", latestCompleteDay.Format("2006-01-02"))
	}
	window := statisticsDomain.InstantRange{From: fromDate, To: toDate.AddDate(0, 0, 1)}
	if err := window.Validate(); err != nil {
		return nil, invalidRunRequest("%s", err)
	}
	if daysBetween(window.From, window.To) > maxManualWindowDays {
		return nil, invalidRunRequest("manual statistics window exceeds %d days", maxManualWindowDays)
	}

	run, err := c.store.Create(ctx, Run{
		OrgID:       request.OrgID,
		BatchKey:    fmt.Sprintf("%d:%s:%s:%s:%s", request.OrgID, window.From.Format("20060102"), toDate.Format("20060102"), mode, request.TriggerType),
		TriggerType: request.TriggerType,
		Mode:        mode,
		Window:      window,
		AsOfDate:    latestCompleteDay,
		Status:      statisticsDomain.RunStatusRunning,
		Stage:       "created",
		Reason:      request.Reason,
		OperatorID:  request.OperatorID,
		StartedAt:   now,
	})
	if err != nil {
		return nil, err
	}

	lockKey := fmt.Sprintf("statistics:%d", request.OrgID)
	result, runErr := c.locks.Run(ctx, locklease.WorkloadStatisticsSync, lockKey, 30*time.Minute, func(lockCtx context.Context) error {
		return c.execute(lockCtx, run)
	})
	if runErr == nil && !result.Acquired {
		runErr = executionError("lock", "lock_busy", fmt.Errorf("statistics lock busy"))
	}
	if runErr != nil {
		latest, getErr := c.store.Get(ctx, run.ID)
		if getErr != nil {
			return nil, fmt.Errorf("statistics run failed: %w; reload run: %v", runErr, getErr)
		}
		if latest != nil && latest.Status == statisticsDomain.RunStatusDataCommitted {
			return latest, runErr
		}
		stage, code := "failed", "run_failed"
		var execution *runExecutionError
		if errors.As(runErr, &execution) {
			stage, code = execution.stage, execution.code
		}
		observeStatisticsStageFailure(stage, code)
		if markErr := c.store.MarkFailed(ctx, run.ID, stage, code, runErr.Error(), c.now()); markErr != nil {
			runErr = errors.Join(runErr, fmt.Errorf("persist failed statistics run: %w", markErr))
		}
		latest, getErr = c.store.Get(ctx, run.ID)
		if getErr != nil {
			return nil, fmt.Errorf("statistics run failed: %w; reload failed run: %v", runErr, getErr)
		}
		return latest, runErr
	}
	return c.store.Get(ctx, run.ID)
}

func normalizeRunMode(mode statisticsDomain.RunMode, validateOnly bool) (statisticsDomain.RunMode, error) {
	if mode == "" {
		if validateOnly {
			return statisticsDomain.RunModeValidate, nil
		}
		// Compatibility for the scheduler and the original internal endpoint.
		return statisticsDomain.RunModePublish, nil
	}
	if validateOnly && mode != statisticsDomain.RunModeValidate {
		return "", invalidRunRequest("validate_only conflicts with mode %q", mode)
	}
	if err := mode.Validate(); err != nil {
		return "", invalidRunRequest("%s", err)
	}
	return mode, nil
}

func daysBetween(from, to time.Time) int {
	return int(to.Sub(from).Hours() / 24)
}

func (c *Coordinator) execute(ctx context.Context, run *Run) error {
	collectMode := statisticsDomain.CollectModeNormal
	switch run.Mode {
	case statisticsDomain.RunModeValidate:
		collectMode = statisticsDomain.CollectModeValidate
	case statisticsDomain.RunModeRepair:
		collectMode = statisticsDomain.CollectModeBackfill
	}

	sources, facts := map[string]int64{}, map[string]int64{}
	for _, collector := range c.collectors.Ordered() {
		stage := "collecting_" + collector.Name()
		if err := c.store.UpdateProgress(ctx, run.ID, stage, sources, facts, nil); err != nil {
			return executionError(stage, "run_progress_failed", err)
		}
		item, err := collector.Collect(ctx, statisticsDomain.CollectRequest{
			RunID: run.ID, OrgID: run.OrgID, Window: run.Window, AsOfDate: run.AsOfDate, Mode: collectMode,
		})
		mergeCollectorCounts(sources, facts, item)
		observeCollectorResult(item)
		if progressErr := c.store.UpdateProgress(ctx, run.ID, stage, sources, facts, nil); progressErr != nil {
			return executionError(stage, "run_progress_failed", progressErr)
		}
		if item.ConflictCount > 0 {
			return executionError(stage, "fact_conflict", fmt.Errorf("collector %s found %d conflicts", collector.Name(), item.ConflictCount))
		}
		if err != nil {
			return executionError(stage, "collector_failed", fmt.Errorf("collect %s: %w", collector.Name(), err))
		}
	}
	if err := c.store.UpdateProgress(ctx, run.ID, "facts_ready", sources, facts, nil); err != nil {
		return executionError("facts_ready", "run_progress_failed", err)
	}
	if run.Mode == statisticsDomain.RunModeValidate {
		if err := c.store.MarkSucceeded(ctx, run.ID, c.now()); err != nil {
			return executionError("completed", "run_progress_failed", err)
		}
		return nil
	}

	resultCounts := map[string]int64{}
	snapshotAt := c.now()
	cutoffAt := statisticsDomain.BusinessDate(snapshotAt)
	err := c.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		request := statisticsDomain.ProjectionRequest{
			RunID: run.ID, OrgID: run.OrgID, Window: run.Window,
			AsOfDate: run.AsOfDate, CutoffAt: cutoffAt, SnapshotAt: snapshotAt,
		}
		if err := c.project(txCtx, run.ID, c.dailyEngine, request, resultCounts); err != nil {
			return err
		}
		if run.Mode == statisticsDomain.RunModeRepair {
			if err := c.store.MarkSucceeded(txCtx, run.ID, c.now()); err != nil {
				return executionError("completed", "result_tx_failed", err)
			}
			return nil
		}
		if err := c.store.AssertPublishable(txCtx, run.OrgID, run.AsOfDate); err != nil {
			return executionError("projecting_org_snapshot", "publish_watermark_regression", err)
		}
		if err := c.project(txCtx, run.ID, c.globalEngine, request, resultCounts); err != nil {
			return err
		}
		if err := c.store.MarkDataCommitted(txCtx, run.ID, snapshotAt); err != nil {
			return executionError("data_committed", "result_tx_failed", err)
		}
		return nil
	})
	if err != nil {
		var execution *runExecutionError
		if errors.As(err, &execution) {
			return err
		}
		return executionError("result_transaction", "result_tx_failed", err)
	}
	if run.Mode == statisticsDomain.RunModeRepair {
		return nil
	}
	if c.cache != nil {
		if err := c.store.UpdateProgress(ctx, run.ID, "publishing_cache", nil, nil, nil); err != nil {
			return executionError("publishing_cache", "run_progress_failed", err)
		}
		generation, err := c.cache.Publish(ctx, run.OrgID, run.AsOfDate)
		if err != nil {
			observeCachePublish("publish", err)
			if markErr := c.store.MarkCachePublishFailed(ctx, run.ID, generation, err.Error(), c.now()); markErr != nil {
				err = errors.Join(err, fmt.Errorf("persist cache publication failure: %w", markErr))
			}
			return executionError("publishing_cache", "cache_publish_failed", err)
		}
		if err := c.store.MarkCachePublished(ctx, run.ID, generation, c.now()); err != nil {
			return executionError("publishing_cache", "cache_publication_audit_failed", err)
		}
		observeCachePublish("publish", nil)
	}
	if err := c.store.MarkSucceeded(ctx, run.ID, c.now()); err != nil {
		return executionError("completed", "run_progress_failed", err)
	}
	return nil
}

func mergeCollectorCounts(sources, facts map[string]int64, item statisticsDomain.CollectResult) {
	if item.Collector == "" {
		return
	}
	sources[item.Collector] = item.SourceCount
	facts[item.Collector+".inserted"] = item.InsertedCount
	facts[item.Collector+".existing"] = item.ExistingCount
	facts[item.Collector+".conflict"] = item.ConflictCount
	for factType, count := range item.FactTypeCounts {
		facts[item.Collector+".type."+factType] = count
	}
}

func (c *Coordinator) project(ctx context.Context, runID uint64, engine *statisticsDomain.ProjectionEngine, request statisticsDomain.ProjectionRequest, counts map[string]int64) error {
	for _, projection := range engine.Ordered() {
		stage := "projecting_" + projection.Name()
		if err := c.store.UpdateProgress(ctx, runID, stage, nil, nil, counts); err != nil {
			return executionError(stage, "result_tx_failed", err)
		}
		result, err := projection.Project(ctx, request)
		if err != nil {
			return executionError(stage, "projection_failed", fmt.Errorf("project %s: %w", projection.Name(), err))
		}
		counts[result.Name] = result.Rows
		observeProjectionResult(result)
		if err := c.store.UpdateProgress(ctx, runID, stage, nil, nil, counts); err != nil {
			return executionError(stage, "result_tx_failed", err)
		}
	}
	return nil
}

func (c *Coordinator) ResumeCache(ctx context.Context, id uint64, request ...CacheResumeRequest) (*Run, error) {
	resume := CacheResumeRequest{}
	if len(request) > 0 {
		resume = request[0]
	}
	if resume.OperatorID != 0 && resume.Reason == "" {
		return nil, fmt.Errorf("cache resume reason is required")
	}
	if utf8.RuneCountInString(resume.Reason) > maxRunReasonRunes {
		return nil, invalidRunRequest("cache resume reason exceeds %d characters", maxRunReasonRunes)
	}
	run, err := c.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if run == nil || run.Status != statisticsDomain.RunStatusDataCommitted || run.Mode != statisticsDomain.RunModePublish {
		return nil, fmt.Errorf("run is not a publish run in data_committed state")
	}
	generation := run.CacheGeneration
	if c.cache != nil {
		generation, err = c.cache.Publish(ctx, run.OrgID, run.AsOfDate)
		if err != nil {
			observeCachePublish("resume", err)
			if markErr := c.store.MarkCachePublishFailed(ctx, id, generation, err.Error(), c.now()); markErr != nil {
				err = errors.Join(err, fmt.Errorf("persist cache publication failure: %w", markErr))
			}
			if auditErr := c.store.RecordCacheResume(ctx, id, resume.OperatorID, resume.Reason, "failed", generation, c.now()); auditErr != nil {
				err = errors.Join(err, fmt.Errorf("persist cache resume audit: %w", auditErr))
			}
			latest, _ := c.store.Get(ctx, id)
			return latest, err
		}
		if err := c.store.MarkCachePublished(ctx, id, generation, c.now()); err != nil {
			_ = c.store.RecordCacheResume(ctx, id, resume.OperatorID, resume.Reason, "failed", generation, c.now())
			return nil, fmt.Errorf("persist resumed cache generation: %w", err)
		}
		observeCachePublish("resume", nil)
	}
	if err := c.store.RecordCacheResume(ctx, id, resume.OperatorID, resume.Reason, "succeeded", generation, c.now()); err != nil {
		return nil, err
	}
	if err := c.store.MarkSucceeded(ctx, id, c.now()); err != nil {
		return nil, err
	}
	return c.store.Get(ctx, id)
}
