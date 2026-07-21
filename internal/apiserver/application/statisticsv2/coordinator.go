package statisticsv2

import (
	"context"
	"fmt"
	"time"

	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	domainv2 "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics/v2"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
)

type Run struct {
	ID                                     uint64                `json:"id"`
	OrgID                                  int64                 `json:"org_id"`
	BatchKey                               string                `json:"batch_key"`
	Attempt                                uint32                `json:"attempt"`
	TriggerType                            string                `json:"trigger_type"`
	Window                                 domainv2.InstantRange `json:"-"`
	AsOfDate                               time.Time             `json:"as_of_date"`
	Status                                 domainv2.RunStatus    `json:"status"`
	Stage                                  string                `json:"stage"`
	Reason                                 string                `json:"reason"`
	OperatorID                             uint64                `json:"operator_id,omitempty"`
	StartedAt                              time.Time             `json:"started_at"`
	DataCommittedAt                        *time.Time            `json:"data_committed_at,omitempty"`
	FinishedAt                             *time.Time            `json:"finished_at,omitempty"`
	SourceCounts, FactCounts, ResultCounts map[string]int64      `json:"-"`
	ErrorCode, ErrorMessage                string                `json:"error_code,omitempty"`
}

type RunStore interface {
	Create(context.Context, Run) (*Run, error)
	UpdateProgress(context.Context, uint64, string, map[string]int64, map[string]int64, map[string]int64) error
	MarkDataCommitted(context.Context, uint64, time.Time) error
	MarkSucceeded(context.Context, uint64, time.Time) error
	MarkFailed(context.Context, uint64, string, string, string, time.Time) error
	Get(context.Context, uint64) (*Run, error)
	List(context.Context, int64, int) ([]Run, error)
}

type CachePublisher interface {
	Publish(context.Context, int64, time.Time) error
}

type RunRequest struct {
	OrgID               int64
	FromDate, ToDate    time.Time
	Reason, TriggerType string
	OperatorID          uint64
	ValidateOnly        bool
}

type Coordinator struct {
	collectors *domainv2.CollectorSet
	engine     *domainv2.ProjectionEngine
	store      RunStore
	tx         apptransaction.Runner
	locks      locklease.Runner
	cache      CachePublisher
	now        func() time.Time
}

func NewCoordinator(collectors *domainv2.CollectorSet, engine *domainv2.ProjectionEngine, store RunStore, tx apptransaction.Runner, locks locklease.Runner, cache CachePublisher) *Coordinator {
	return &Coordinator{collectors: collectors, engine: engine, store: store, tx: tx, locks: locks, cache: cache, now: time.Now}
}

func (c *Coordinator) Run(ctx context.Context, request RunRequest) (*Run, error) {
	if request.OrgID <= 0 {
		return nil, fmt.Errorf("org_id is required")
	}
	window := domainv2.InstantRange{From: domainv2.BusinessDate(request.FromDate), To: domainv2.BusinessDate(request.ToDate).AddDate(0, 0, 1)}
	if err := window.Validate(); err != nil {
		return nil, err
	}
	if int(window.To.Sub(window.From).Hours()/24) > 31 {
		return nil, fmt.Errorf("manual statistics window exceeds 31 days")
	}
	if c.collectors == nil || c.engine == nil || c.store == nil || c.tx == nil || c.locks == nil {
		return nil, fmt.Errorf("statistics v2 coordinator is not fully configured")
	}
	asOf := window.To.AddDate(0, 0, -1)
	now := c.now()
	run, err := c.store.Create(ctx, Run{OrgID: request.OrgID, BatchKey: fmt.Sprintf("%d:%s:%s:%s", request.OrgID, window.From.Format("20060102"), asOf.Format("20060102"), request.TriggerType), TriggerType: request.TriggerType, Window: window, AsOfDate: asOf, Status: domainv2.RunStatusRunning, Stage: "created", Reason: request.Reason, OperatorID: request.OperatorID, StartedAt: now})
	if err != nil {
		return nil, err
	}
	lockKey := fmt.Sprintf("statistics:v2:%d:%s", request.OrgID, asOf.Format("2006-01-02"))
	result, runErr := c.locks.Run(ctx, locklease.WorkloadStatisticsSync, lockKey, 30*time.Minute, func(lockCtx context.Context) error { return c.execute(lockCtx, run, request.ValidateOnly) })
	if runErr == nil && !result.Acquired {
		runErr = fmt.Errorf("statistics v2 lock busy")
	}
	if runErr != nil {
		latest, getErr := c.store.Get(ctx, run.ID)
		if getErr != nil {
			return nil, fmt.Errorf("statistics run failed: %w; reload run: %v", runErr, getErr)
		}
		// Result data and data_committed are committed in one transaction. Cache
		// publication happens afterwards, so a publication failure must preserve
		// the resumable data_committed state instead of rewriting it to failed.
		if latest != nil && latest.Status == domainv2.RunStatusDataCommitted {
			return latest, runErr
		}
		_ = c.store.MarkFailed(ctx, run.ID, "failed", "run_failed", runErr.Error(), c.now())
		latest, getErr = c.store.Get(ctx, run.ID)
		if getErr != nil {
			return nil, fmt.Errorf("statistics run failed: %w; reload failed run: %v", runErr, getErr)
		}
		return latest, runErr
	}
	return c.store.Get(ctx, run.ID)
}

func (c *Coordinator) execute(ctx context.Context, run *Run, validateOnly bool) error {
	mode := domainv2.CollectModeNormal
	if validateOnly {
		mode = domainv2.CollectModeValidate
	}
	collected, err := c.collectors.Collect(ctx, domainv2.CollectRequest{RunID: run.ID, OrgID: run.OrgID, Window: run.Window, AsOfDate: run.AsOfDate, Mode: mode})
	if err != nil {
		return err
	}
	sources, facts := map[string]int64{}, map[string]int64{}
	for _, item := range collected {
		sources[item.Collector] = item.SourceCount
		facts[item.Collector+".inserted"] = item.InsertedCount
		facts[item.Collector+".existing"] = item.ExistingCount
		facts[item.Collector+".conflict"] = item.ConflictCount
		if item.ConflictCount > 0 {
			return fmt.Errorf("collector %s found %d conflicts", item.Collector, item.ConflictCount)
		}
	}
	if err := c.store.UpdateProgress(ctx, run.ID, "collected", sources, facts, nil); err != nil {
		return err
	}
	if validateOnly {
		if err := c.store.MarkSucceeded(ctx, run.ID, c.now()); err != nil {
			return err
		}
		return nil
	}
	projectionCounts := map[string]int64{}
	snapshotAt := c.now()
	err = c.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		results, err := c.engine.Project(txCtx, domainv2.ProjectionRequest{RunID: run.ID, OrgID: run.OrgID, Window: run.Window, AsOfDate: run.AsOfDate, CutoffAt: run.Window.To, SnapshotAt: snapshotAt})
		if err != nil {
			return err
		}
		for _, item := range results {
			projectionCounts[item.Name] = item.Rows
		}
		if err := c.store.UpdateProgress(txCtx, run.ID, "projected", nil, nil, projectionCounts); err != nil {
			return err
		}
		return c.store.MarkDataCommitted(txCtx, run.ID, c.now())
	})
	if err != nil {
		return err
	}
	if c.cache != nil {
		if err := c.cache.Publish(ctx, run.OrgID, run.AsOfDate); err != nil {
			return err
		}
	}
	return c.store.MarkSucceeded(ctx, run.ID, c.now())
}

func (c *Coordinator) ResumeCache(ctx context.Context, id uint64) (*Run, error) {
	run, err := c.store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if run == nil || run.Status != domainv2.RunStatusDataCommitted {
		return nil, fmt.Errorf("run is not data_committed")
	}
	if c.cache != nil {
		if err := c.cache.Publish(ctx, run.OrgID, run.AsOfDate); err != nil {
			return nil, err
		}
	}
	if err := c.store.MarkSucceeded(ctx, id, c.now()); err != nil {
		return nil, err
	}
	return c.store.Get(ctx, id)
}
