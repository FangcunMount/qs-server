package cachegovernance

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Coordinator interface {
	WarmStartup(ctx context.Context) error
	HandleScalePublished(ctx context.Context, code string) error
	HandleQuestionnairePublished(ctx context.Context, code, version string) error
	HandlePersonalityModelPublished(ctx context.Context, code string) error
	HandleStatisticsSync(ctx context.Context, orgID int64) error
	HandleRepairComplete(ctx context.Context, req RepairCompleteRequest) error
	HandleManualWarmup(ctx context.Context, req ManualWarmupRequest) (*ManualWarmupResult, error)
	Snapshot() WarmupStatusSnapshot
}

type RepairCompleteRequest struct {
	RepairKind         string
	OrgIDs             []int64
	QuestionnaireCodes []string
	PlanIDs            []uint64
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
	WarmScaleList                   func(context.Context) error
	WarmPublishedPersonalityModel   func(context.Context, string) error
	WarmStatsOverview               func(context.Context, int64, string) error
	WarmStatsSystem                 func(context.Context, int64) error
	WarmStatsQuestionnaire          func(context.Context, int64, string) error
	WarmStatsPlan                   func(context.Context, int64, uint64) error
}

type WarmupRunSnapshot struct {
	Trigger      string    `json:"trigger"`
	StartedAt    time.Time `json:"started_at"`
	FinishedAt   time.Time `json:"finished_at"`
	Result       string    `json:"result"`
	TargetCount  int       `json:"target_count"`
	OkCount      int       `json:"ok_count"`
	ErrorCount   int       `json:"error_count"`
	SkippedCount int       `json:"skipped_count"`
}

type WarmupStatusSnapshot struct {
	Enabled    bool                `json:"enabled"`
	Startup    WarmupStartupStatus `json:"startup"`
	Hotset     WarmupHotsetStatus  `json:"hotset"`
	LatestRuns []WarmupRunSnapshot `json:"latest_runs"`
}

type WarmupStartupStatus struct {
	Static bool `json:"static"`
	Query  bool `json:"query"`
}

type WarmupHotsetStatus struct {
	Enable          bool  `json:"enable"`
	TopN            int64 `json:"top_n"`
	MaxItemsPerKind int64 `json:"max_items_per_kind"`
}

type coordinator struct {
	cfg      Config
	deps     Dependencies
	registry *WarmupRegistry
	mu       sync.RWMutex
	runs     map[string]WarmupRunSnapshot
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
		registry: NewWarmupRegistry(),
		runs:     make(map[string]WarmupRunSnapshot),
	}
	c.registerExecutors()
	return c
}

func (c *coordinator) Snapshot() WarmupStatusSnapshot {
	if c == nil {
		return WarmupStatusSnapshot{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()

	runs := make([]WarmupRunSnapshot, 0, len(c.runs))
	for _, run := range c.runs {
		runs = append(runs, run)
	}
	sort.Slice(runs, func(i, j int) bool {
		return runs[i].StartedAt.After(runs[j].StartedAt)
	})
	return WarmupStatusSnapshot{
		Enabled: c.cfg.Enable,
		Startup: WarmupStartupStatus{
			Static: c.cfg.StartupStatic,
			Query:  c.cfg.StartupQuery,
		},
		Hotset: WarmupHotsetStatus{
			Enable:          c.cfg.HotsetEnable,
			TopN:            c.cfg.HotsetTopN,
			MaxItemsPerKind: c.cfg.MaxItemsPerKind,
		},
		LatestRuns: runs,
	}
}

func (c *coordinator) HandleManualWarmup(ctx context.Context, req ManualWarmupRequest) (*ManualWarmupResult, error) {
	if c == nil || !c.cfg.Enable {
		return &ManualWarmupResult{
			Trigger:    manualWarmupTrigger,
			StartedAt:  time.Now(),
			FinishedAt: time.Now(),
			Summary: ManualWarmupSummary{
				Result: "skipped",
			},
			Items: []ManualWarmupItemResult{},
		}, nil
	}
	if len(req.Targets) == 0 {
		return nil, fmt.Errorf("warmup targets cannot be empty")
	}

	targets := make([]cachetarget.WarmupTarget, 0, len(req.Targets))
	for _, item := range req.Targets {
		target, err := ParseManualWarmupTarget(item)
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	return c.executeTargets(ctx, manualWarmupTrigger, targets)
}

func (c *coordinator) registerExecutors() {
	c.registry.Register(cachetarget.WarmupKindStaticScale, func(ctx context.Context, target cachetarget.WarmupTarget) error {
		if c.deps.WarmScale == nil {
			return nil
		}
		code, ok := cachetarget.ParseStaticScaleScope(target.Scope)
		if !ok {
			return fmt.Errorf("invalid static scale warmup scope: %s", target.Scope)
		}
		return c.deps.WarmScale(ctx, code)
	})
	c.registry.Register(cachetarget.WarmupKindStaticQuestionnaire, func(ctx context.Context, target cachetarget.WarmupTarget) error {
		if c.deps.WarmQuestionnaire == nil {
			return nil
		}
		code, ok := cachetarget.ParseStaticQuestionnaireScope(target.Scope)
		if !ok {
			return fmt.Errorf("invalid static questionnaire warmup scope: %s", target.Scope)
		}
		return c.deps.WarmQuestionnaire(ctx, code)
	})
	c.registry.Register(cachetarget.WarmupKindStaticScaleList, func(ctx context.Context, _ cachetarget.WarmupTarget) error {
		if c.deps.WarmScaleList == nil {
			return nil
		}
		return c.deps.WarmScaleList(ctx)
	})
	c.registry.Register(cachetarget.WarmupKindQueryStatsOverview, func(ctx context.Context, target cachetarget.WarmupTarget) error {
		if c.deps.WarmStatsOverview == nil {
			return nil
		}
		orgID, preset, ok := cachetarget.ParseQueryStatsOverviewScope(target.Scope)
		if !ok {
			return fmt.Errorf("invalid stats overview warmup scope: %s", target.Scope)
		}
		return c.deps.WarmStatsOverview(ctx, orgID, preset)
	})
	c.registry.Register(cachetarget.WarmupKindQueryStatsSystem, func(ctx context.Context, target cachetarget.WarmupTarget) error {
		if c.deps.WarmStatsSystem == nil {
			return nil
		}
		orgID, ok := cachetarget.ParseQueryStatsSystemScope(target.Scope)
		if !ok {
			return fmt.Errorf("invalid stats system warmup scope: %s", target.Scope)
		}
		return c.deps.WarmStatsSystem(ctx, orgID)
	})
	c.registry.Register(cachetarget.WarmupKindQueryStatsQuestionnaire, func(ctx context.Context, target cachetarget.WarmupTarget) error {
		if c.deps.WarmStatsQuestionnaire == nil {
			return nil
		}
		orgID, code, ok := cachetarget.ParseQueryStatsQuestionnaireScope(target.Scope)
		if !ok {
			return fmt.Errorf("invalid stats questionnaire warmup scope: %s", target.Scope)
		}
		return c.deps.WarmStatsQuestionnaire(ctx, orgID, code)
	})
	c.registry.Register(cachetarget.WarmupKindQueryStatsPlan, func(ctx context.Context, target cachetarget.WarmupTarget) error {
		if c.deps.WarmStatsPlan == nil {
			return nil
		}
		orgID, planID, ok := cachetarget.ParseQueryStatsPlanScope(target.Scope)
		if !ok {
			return fmt.Errorf("invalid stats plan warmup scope: %s", target.Scope)
		}
		return c.deps.WarmStatsPlan(ctx, orgID, planID)
	})
}

func (c *coordinator) executeTargets(ctx context.Context, trigger string, targets []cachetarget.WarmupTarget) (*ManualWarmupResult, error) {
	if c == nil || !c.cfg.Enable {
		return &ManualWarmupResult{
			Trigger:    trigger,
			StartedAt:  time.Now(),
			FinishedAt: time.Now(),
			Summary: ManualWarmupSummary{
				Result: "skipped",
			},
			Items: []ManualWarmupItemResult{},
		}, nil
	}
	startedAt := time.Now()
	targets = dedupeTargets(targets)
	if len(targets) == 0 {
		warmupRunTotal.WithLabelValues(trigger, "skipped").Inc()
		finishedAt := time.Now()
		observability.ObserveWarmupDuration(trigger, "skipped", finishedAt.Sub(startedAt))
		run := WarmupRunSnapshot{
			Trigger:      trigger,
			StartedAt:    startedAt,
			FinishedAt:   finishedAt,
			Result:       "skipped",
			TargetCount:  0,
			SkippedCount: 0,
		}
		c.recordRun(run)
		return &ManualWarmupResult{
			Trigger:    trigger,
			StartedAt:  startedAt,
			FinishedAt: finishedAt,
			Summary: ManualWarmupSummary{
				Result: "skipped",
			},
			Items: []ManualWarmupItemResult{},
		}, nil
	}

	runCtx := cachetarget.SuppressHotsetRecording(ctx)
	warmupRunTotal.WithLabelValues(trigger, "started").Inc()
	okCount := 0
	errorCount := 0
	skippedCount := 0
	items := make([]ManualWarmupItemResult, 0, len(targets))
	for _, target := range targets {
		if c.deps.Runtime != nil && !c.deps.Runtime.AllowWarmup(target.Family) {
			skippedCount++
			items = append(items, ManualWarmupItemResult{
				Family:  string(target.Family),
				Kind:    target.Kind,
				Scope:   target.Scope,
				Status:  ManualWarmupItemStatusSkipped,
				Message: "该缓存族未开启预热",
			})
			continue
		}
		if err := c.registry.Execute(runCtx, target); err != nil {
			warmupItemTotal.WithLabelValues(trigger, string(target.Family), string(target.Kind), "error").Inc()
			errorCount++
			items = append(items, ManualWarmupItemResult{
				Family:  string(target.Family),
				Kind:    target.Kind,
				Scope:   target.Scope,
				Status:  ManualWarmupItemStatusError,
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
		items = append(items, ManualWarmupItemResult{
			Family: string(target.Family),
			Kind:   target.Kind,
			Scope:  target.Scope,
			Status: ManualWarmupItemStatusOK,
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
	observability.ObserveWarmupDuration(trigger, result, finishedAt.Sub(startedAt))
	c.recordRun(WarmupRunSnapshot{
		Trigger:      trigger,
		StartedAt:    startedAt,
		FinishedAt:   finishedAt,
		Result:       result,
		TargetCount:  len(targets),
		OkCount:      okCount,
		ErrorCount:   errorCount,
		SkippedCount: skippedCount,
	})
	return &ManualWarmupResult{
		Trigger:    trigger,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		Summary: ManualWarmupSummary{
			TargetCount:  len(targets),
			OkCount:      okCount,
			SkippedCount: skippedCount,
			ErrorCount:   errorCount,
			Result:       result,
		},
		Items: items,
	}, nil
}

func (c *coordinator) recordRun(run WarmupRunSnapshot) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.runs[run.Trigger] = run
}
