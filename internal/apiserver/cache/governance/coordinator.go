package cachegovernance

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/model"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"
	cacheobserve "github.com/FangcunMount/qs-server/internal/pkg/cache/observe"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const manualWarmupTrigger = "manual"

type Coordinator interface {
	WarmStartup(ctx context.Context) error
	HandleScalePublished(ctx context.Context, code string) error
	HandleQuestionnairePublished(ctx context.Context, code, version string) error
	HandleTypologyModelPublished(ctx context.Context, code string) error
	HandleStatisticsSync(ctx context.Context, orgID int64) error
	HandleRepairComplete(ctx context.Context, req cachetarget.RepairCompleteRequest) error
	HandleManualWarmup(ctx context.Context, req cachetarget.ManualWarmupRequest) (*cachemodel.ManualWarmupResult, error)
	Snapshot() cachemodel.WarmupStatusSnapshot
}

type WarmFunc func(context.Context, cachetarget.WarmupTarget) error

type WarmupRegistry struct {
	executors map[cachetarget.WarmupKind]WarmFunc
}

func NewWarmupRegistry() *WarmupRegistry {
	return &WarmupRegistry{executors: make(map[cachetarget.WarmupKind]WarmFunc)}
}

func (r *WarmupRegistry) Register(kind cachetarget.WarmupKind, fn WarmFunc) {
	if r == nil || fn == nil {
		return
	}
	r.executors[kind] = fn
}

func (r *WarmupRegistry) Execute(ctx context.Context, target cachetarget.WarmupTarget) error {
	if r == nil {
		return fmt.Errorf("warmup registry is nil")
	}
	fn, ok := r.executors[target.Kind]
	if !ok || fn == nil {
		return fmt.Errorf("warmup executor for %s is not registered", target.Kind)
	}
	return fn(ctx, target)
}

type Config struct {
	Enable          bool
	StartupStatic   bool
	StartupQuery    bool
	HotsetEnable    bool
	HotsetTopN      int64
	MaxItemsPerKind int64
}

type Dependencies struct {
	Runtime                         FamilyRuntime
	StatisticsSeeds                 *StatisticsWarmupConfig
	Hotset                          cachetarget.HotsetRecorder
	ListPublishedScaleCodes         func(context.Context) ([]string, error)
	ListPublishedQuestionnaireCodes func(context.Context) ([]string, error)
	LookupScaleQuestionnaireCode    func(context.Context, string) (string, error)
	WarmScale                       func(context.Context, string) error
	WarmQuestionnaire               func(context.Context, string) error
	WarmPublishedTypologyModel      func(context.Context, string) error
	WarmStatsOverview               func(context.Context, int64, string) error
}

type coordinator struct {
	cfg      Config
	deps     Dependencies
	registry *WarmupRegistry
	mu       sync.RWMutex
	runs     map[string]cachemodel.WarmupRunSnapshot
}

var (
	warmupRunTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "qs_cache_warmup_runs_total",
			Help: "Total number of cache governance warmup runs grouped by trigger and result.",
		},
		[]string{"trigger", "result"},
	)
	warmupItemTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "qs_cache_warmup_items_total",
			Help: "Total number of cache governance warmup items grouped by trigger, family, kind and result.",
		},
		[]string{"trigger", "family", "kind", "result"},
	)
)

func NewCoordinator(cfg Config, deps Dependencies) Coordinator {
	if !cfg.Enable {
		return nil
	}
	if cfg.HotsetTopN <= 0 {
		cfg.HotsetTopN = 20
	}
	c := &coordinator{
		cfg:      cfg,
		deps:     deps,
		registry: ExecutorRegistryBuilder{}.Build(deps),
		runs:     make(map[string]cachemodel.WarmupRunSnapshot),
	}
	return c
}

func (c *coordinator) Snapshot() cachemodel.WarmupStatusSnapshot {
	if c == nil {
		return cachemodel.WarmupStatusSnapshot{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()

	runs := make([]cachemodel.WarmupRunSnapshot, 0, len(c.runs))
	for _, run := range c.runs {
		runs = append(runs, run)
	}
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].StartedAt.After(runs[j].StartedAt)
	})
	return cachemodel.WarmupStatusSnapshot{
		Enabled: c.cfg.Enable,
		Startup: cachemodel.WarmupStartupStatus{
			Static: c.cfg.StartupStatic,
			Query:  c.cfg.StartupQuery,
		},
		Hotset: cachemodel.WarmupHotsetStatus{
			Enable:          c.cfg.HotsetEnable,
			TopN:            c.cfg.HotsetTopN,
			MaxItemsPerKind: c.cfg.MaxItemsPerKind,
		},
		LatestRuns: runs,
	}
}

func (c *coordinator) HandleManualWarmup(ctx context.Context, req cachetarget.ManualWarmupRequest) (*cachemodel.ManualWarmupResult, error) {
	if c == nil || !c.cfg.Enable {
		return &cachemodel.ManualWarmupResult{
			Trigger:    manualWarmupTrigger,
			StartedAt:  time.Now(),
			FinishedAt: time.Now(),
			Summary: cachemodel.ManualWarmupSummary{
				Result: "skipped",
			},
			Items: []cachemodel.ManualWarmupItemResult{},
		}, nil
	}
	if len(req.Targets) == 0 {
		return nil, fmt.Errorf("warmup targets cannot be empty")
	}

	targets := make([]cachetarget.WarmupTarget, 0, len(req.Targets))
	for _, item := range req.Targets {
		target, err := cachetarget.ParseManualWarmupTarget(item)
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	return c.executeTargets(ctx, manualWarmupTrigger, targets)
}

func (c *coordinator) planner() *TargetPlanner {
	return NewTargetPlanner(c.cfg, c.deps)
}

func (c *coordinator) mergeQueryTargets(ctx context.Context, orgFilter []int64, repair *cachetarget.RepairCompleteRequest) []cachetarget.WarmupTarget {
	if c == nil {
		return nil
	}
	return c.planner().MergeQueryTargets(ctx, orgFilter, repair)
}

func (c *coordinator) executeTargets(ctx context.Context, trigger string, targets []cachetarget.WarmupTarget) (*cachemodel.ManualWarmupResult, error) {
	if c == nil || !c.cfg.Enable {
		return &cachemodel.ManualWarmupResult{
			Trigger:    trigger,
			StartedAt:  time.Now(),
			FinishedAt: time.Now(),
			Summary: cachemodel.ManualWarmupSummary{
				Result: "skipped",
			},
			Items: []cachemodel.ManualWarmupItemResult{},
		}, nil
	}
	startedAt := time.Now()
	targets = dedupeTargets(targets)
	if len(targets) == 0 {
		warmupRunTotal.WithLabelValues(trigger, "skipped").Inc()
		finishedAt := time.Now()
		cacheobserve.ObserveWarmupDuration(trigger, "skipped", finishedAt.Sub(startedAt))
		run := cachemodel.WarmupRunSnapshot{
			Trigger:      trigger,
			StartedAt:    startedAt,
			FinishedAt:   finishedAt,
			Result:       "skipped",
			TargetCount:  0,
			SkippedCount: 0,
		}
		c.recordRun(run)
		return &cachemodel.ManualWarmupResult{
			Trigger:    trigger,
			StartedAt:  startedAt,
			FinishedAt: finishedAt,
			Summary: cachemodel.ManualWarmupSummary{
				Result: "skipped",
			},
			Items: []cachemodel.ManualWarmupItemResult{},
		}, nil
	}

	runCtx := cachetarget.SuppressHotsetRecording(ctx)
	warmupRunTotal.WithLabelValues(trigger, "started").Inc()
	okCount := 0
	errorCount := 0
	skippedCount := 0
	items := make([]cachemodel.ManualWarmupItemResult, 0, len(targets))
	for _, target := range targets {
		if c.deps.Runtime != nil && !c.deps.Runtime.AllowWarmup(target.Family) {
			skippedCount++
			items = append(items, cachemodel.ManualWarmupItemResult{
				Family:  string(target.Family),
				Kind:    string(target.Kind),
				Scope:   target.Scope,
				Status:  cachemodel.ManualWarmupItemStatusSkipped,
				Message: "该缓存族未开启预热",
			})
			continue
		}
		if err := c.registry.Execute(runCtx, target); err != nil {
			if errors.Is(err, cachetarget.ErrWarmupSkipped) {
				warmupItemTotal.WithLabelValues(trigger, string(target.Family), string(target.Kind), "skipped").Inc()
				skippedCount++
				items = append(items, cachemodel.ManualWarmupItemResult{
					Family: string(target.Family), Kind: string(target.Kind), Scope: target.Scope,
					Status: cachemodel.ManualWarmupItemStatusSkipped, Message: err.Error(),
				})
				continue
			}
			warmupItemTotal.WithLabelValues(trigger, string(target.Family), string(target.Kind), "error").Inc()
			errorCount++
			items = append(items, cachemodel.ManualWarmupItemResult{
				Family:  string(target.Family),
				Kind:    string(target.Kind),
				Scope:   target.Scope,
				Status:  cachemodel.ManualWarmupItemStatusError,
				Message: err.Error(),
			})
			logger.L(ctx).Warnw("cache governance warmup target failed",
				"trigger", trigger,
				"family", target.Family,
				"kind", target.Kind,
				"scope", target.Scope,
				"error", err,
			)
			continue
		}
		warmupItemTotal.WithLabelValues(trigger, string(target.Family), string(target.Kind), "ok").Inc()
		okCount++
		items = append(items, cachemodel.ManualWarmupItemResult{
			Family: string(target.Family),
			Kind:   string(target.Kind),
			Scope:  target.Scope,
			Status: cachemodel.ManualWarmupItemStatusOK,
		})
	}
	result := "ok"
	switch {
	case okCount == 0 && errorCount > 0:
		result = "error"
	case errorCount > 0:
		result = "partial"
	case okCount == 0 && skippedCount > 0:
		result = "skipped"
	}
	finishedAt := time.Now()
	warmupRunTotal.WithLabelValues(trigger, result).Inc()
	cacheobserve.ObserveWarmupDuration(trigger, result, finishedAt.Sub(startedAt))
	c.recordRun(cachemodel.WarmupRunSnapshot{
		Trigger:      trigger,
		StartedAt:    startedAt,
		FinishedAt:   finishedAt,
		Result:       result,
		TargetCount:  len(targets),
		OkCount:      okCount,
		ErrorCount:   errorCount,
		SkippedCount: skippedCount,
	})
	return &cachemodel.ManualWarmupResult{
		Trigger:    trigger,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		Summary: cachemodel.ManualWarmupSummary{
			TargetCount:  len(targets),
			OkCount:      okCount,
			SkippedCount: skippedCount,
			ErrorCount:   errorCount,
			Result:       result,
		},
		Items: items,
	}, nil
}

func (c *coordinator) recordRun(run cachemodel.WarmupRunSnapshot) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.runs[run.Trigger] = run
}
