package cachegovernance

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachemodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Coordinator interface {
	WarmStartup(ctx context.Context) error
	HandleScalePublished(ctx context.Context, code string) error
	HandleQuestionnairePublished(ctx context.Context, code, version string) error
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

func (c *coordinator) WarmStartup(ctx context.Context) error {
	if c == nil || !c.cfg.Enable {
		return nil
	}
	targets := make([]cachetarget.WarmupTarget, 0)
	if c.cfg.StartupStatic {
		targets = append(targets, c.startupStaticTargets(ctx)...)
	}
	if c.cfg.StartupQuery {
		targets = append(targets, c.mergeQueryTargets(ctx, nil, nil)...)
	}
	_, err := c.executeTargets(ctx, "startup", targets)
	return err
}

func (c *coordinator) HandleScalePublished(ctx context.Context, code string) error {
	if c == nil || !c.cfg.Enable {
		return nil
	}
	targets := []cachetarget.WarmupTarget{
		cachetarget.NewStaticScaleWarmupTarget(code),
		cachetarget.NewStaticScaleListWarmupTarget(),
	}
	if c.deps.LookupScaleQuestionnaireCode != nil {
		if questionnaireCode, err := c.deps.LookupScaleQuestionnaireCode(ctx, code); err != nil {
			logger.L(ctx).Warnw("failed to resolve questionnaire linked to scale during publish warmup",
				"scale_code", code,
				"error", err,
			)
		} else if questionnaireCode != "" {
			targets = append(targets, cachetarget.NewStaticQuestionnaireWarmupTarget(questionnaireCode))
		}
	}
	_, err := c.executeTargets(ctx, "publish", targets)
	return err
}

func (c *coordinator) HandleQuestionnairePublished(ctx context.Context, code, _ string) error {
	if c == nil || !c.cfg.Enable {
		return nil
	}
	_, err := c.executeTargets(ctx, "publish", []cachetarget.WarmupTarget{
		cachetarget.NewStaticQuestionnaireWarmupTarget(code),
	})
	return err
}

func (c *coordinator) HandleStatisticsSync(ctx context.Context, orgID int64) error {
	if c == nil || !c.cfg.Enable || orgID <= 0 {
		return nil
	}
	targets := []cachetarget.WarmupTarget{
		cachetarget.NewQueryStatsOverviewWarmupTarget(orgID, "today"),
		cachetarget.NewQueryStatsOverviewWarmupTarget(orgID, "7d"),
		cachetarget.NewQueryStatsOverviewWarmupTarget(orgID, "30d"),
		cachetarget.NewQueryStatsSystemWarmupTarget(orgID),
	}
	_, err := c.executeTargets(ctx, "statistics_sync", append(targets, c.mergeQueryTargets(ctx, []int64{orgID}, nil)...))
	return err
}

func (c *coordinator) HandleRepairComplete(ctx context.Context, req RepairCompleteRequest) error {
	if c == nil || !c.cfg.Enable {
		return nil
	}

	var targets []cachetarget.WarmupTarget
	switch strings.TrimSpace(req.RepairKind) {
	case "statistics_backfill":
		targets = append(targets, c.repairQueryTargets(req)...)
		targets = append(targets, c.mergeQueryTargets(ctx, req.OrgIDs, &req)...)
	case "journey_rebuild_history":
		targets = append(targets, c.repairQueryTargets(req)...)
	default:
		targets = append(targets, c.repairQueryTargets(req)...)
	}
	_, err := c.executeTargets(ctx, "repair", targets)
	return err
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

func (c *coordinator) startupStaticTargets(ctx context.Context) []cachetarget.WarmupTarget {
	targets := make([]cachetarget.WarmupTarget, 0)
	if c.deps.ListPublishedScaleCodes != nil {
		if codes, err := c.deps.ListPublishedScaleCodes(ctx); err != nil {
			logger.L(ctx).Warnw("failed to load published scales for startup warmup", "error", err)
		} else {
			for _, code := range codes {
				targets = append(targets, cachetarget.NewStaticScaleWarmupTarget(code))
			}
		}
	}
	if c.deps.ListPublishedQuestionnaireCodes != nil {
		if codes, err := c.deps.ListPublishedQuestionnaireCodes(ctx); err != nil {
			logger.L(ctx).Warnw("failed to load published questionnaires for startup warmup", "error", err)
		} else {
			for _, code := range codes {
				targets = append(targets, cachetarget.NewStaticQuestionnaireWarmupTarget(code))
			}
		}
	}
	if c.deps.WarmScaleList != nil {
		targets = append(targets, cachetarget.NewStaticScaleListWarmupTarget())
	}
	return dedupeTargets(targets)
}

func (c *coordinator) mergeQueryTargets(ctx context.Context, orgFilter []int64, repair *RepairCompleteRequest) []cachetarget.WarmupTarget {
	targets := make([]cachetarget.WarmupTarget, 0)
	targets = append(targets, c.querySeedTargets(orgFilter)...)
	targets = append(targets, c.queryHotTargets(ctx, orgFilter, repair)...)
	return dedupeTargets(targets)
}

func (c *coordinator) querySeedTargets(orgFilter []int64) []cachetarget.WarmupTarget {
	if c.deps.StatisticsSeeds == nil {
		return nil
	}
	filter := make(map[int64]struct{}, len(orgFilter))
	for _, orgID := range orgFilter {
		if orgID > 0 {
			filter[orgID] = struct{}{}
		}
	}
	targets := make([]cachetarget.WarmupTarget, 0)
	for _, orgID := range c.deps.StatisticsSeeds.OrgIDs {
		if len(filter) > 0 {
			if _, ok := filter[orgID]; !ok {
				continue
			}
		}
		for _, preset := range overviewSeedPresets(c.deps.StatisticsSeeds.OverviewPresets) {
			targets = append(targets, cachetarget.NewQueryStatsOverviewWarmupTarget(orgID, preset))
		}
		targets = append(targets, cachetarget.NewQueryStatsSystemWarmupTarget(orgID))
		for _, code := range c.deps.StatisticsSeeds.QuestionnaireCodes {
			targets = append(targets, cachetarget.NewQueryStatsQuestionnaireWarmupTarget(orgID, code))
		}
		for _, planID := range c.deps.StatisticsSeeds.PlanIDs {
			targets = append(targets, cachetarget.NewQueryStatsPlanWarmupTarget(orgID, planID))
		}
	}
	return targets
}

func overviewSeedPresets(configured []string) []string {
	if len(configured) == 0 {
		return []string{"today", "7d", "30d"}
	}
	result := make([]string, 0, len(configured))
	seen := map[string]struct{}{}
	for _, preset := range configured {
		preset = strings.ToLower(strings.TrimSpace(preset))
		switch preset {
		case "today", "7d", "30d":
			if _, ok := seen[preset]; ok {
				continue
			}
			seen[preset] = struct{}{}
			result = append(result, preset)
		}
	}
	if len(result) == 0 {
		return []string{"today", "7d", "30d"}
	}
	return result
}

func (c *coordinator) queryHotTargets(ctx context.Context, orgFilter []int64, repair *RepairCompleteRequest) []cachetarget.WarmupTarget {
	if !c.cfg.HotsetEnable || c.deps.Hotset == nil {
		return nil
	}
	filter := make(map[int64]struct{}, len(orgFilter))
	for _, orgID := range orgFilter {
		if orgID > 0 {
			filter[orgID] = struct{}{}
		}
	}
	targets := make([]cachetarget.WarmupTarget, 0)
	for _, kind := range []cachetarget.WarmupKind{
		cachetarget.WarmupKindQueryStatsOverview,
		cachetarget.WarmupKindQueryStatsSystem,
		cachetarget.WarmupKindQueryStatsQuestionnaire,
		cachetarget.WarmupKindQueryStatsPlan,
	} {
		items, err := c.deps.Hotset.Top(ctx, cachemodel.FamilyQuery, kind, c.cfg.HotsetTopN)
		if err != nil {
			logger.L(ctx).Warnw("failed to load warmup hotset", "family", cachemodel.FamilyQuery, "kind", kind, "error", err)
			continue
		}
		for _, item := range items {
			if !allowQueryTarget(item, filter, repair) {
				continue
			}
			targets = append(targets, item)
		}
	}
	return targets
}

func (c *coordinator) repairQueryTargets(req RepairCompleteRequest) []cachetarget.WarmupTarget {
	if len(req.OrgIDs) == 0 {
		return nil
	}
	targets := make([]cachetarget.WarmupTarget, 0)
	for _, orgID := range req.OrgIDs {
		if orgID <= 0 {
			continue
		}
		if strings.TrimSpace(req.RepairKind) == "statistics_backfill" {
			for _, preset := range overviewSeedPresets(nil) {
				targets = append(targets, cachetarget.NewQueryStatsOverviewWarmupTarget(orgID, preset))
			}
			targets = append(targets, cachetarget.NewQueryStatsSystemWarmupTarget(orgID))
		}
		for _, code := range req.QuestionnaireCodes {
			targets = append(targets, cachetarget.NewQueryStatsQuestionnaireWarmupTarget(orgID, code))
		}
		for _, planID := range req.PlanIDs {
			targets = append(targets, cachetarget.NewQueryStatsPlanWarmupTarget(orgID, planID))
		}
	}
	return dedupeTargets(targets)
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

func allowQueryTarget(target cachetarget.WarmupTarget, orgFilter map[int64]struct{}, repair *RepairCompleteRequest) bool {
	if len(orgFilter) == 0 && repair == nil {
		return true
	}
	switch target.Kind {
	case cachetarget.WarmupKindQueryStatsOverview:
		orgID, _, ok := cachetarget.ParseQueryStatsOverviewScope(target.Scope)
		return ok && allowOrg(orgFilter, orgID)
	case cachetarget.WarmupKindQueryStatsSystem:
		orgID, ok := cachetarget.ParseQueryStatsSystemScope(target.Scope)
		return ok && allowOrg(orgFilter, orgID)
	case cachetarget.WarmupKindQueryStatsQuestionnaire:
		orgID, code, ok := cachetarget.ParseQueryStatsQuestionnaireScope(target.Scope)
		if !ok || !allowOrg(orgFilter, orgID) {
			return false
		}
		return repair == nil || len(repair.QuestionnaireCodes) == 0 || containsFold(repair.QuestionnaireCodes, code)
	case cachetarget.WarmupKindQueryStatsPlan:
		orgID, planID, ok := cachetarget.ParseQueryStatsPlanScope(target.Scope)
		if !ok || !allowOrg(orgFilter, orgID) {
			return false
		}
		return repair == nil || len(repair.PlanIDs) == 0 || containsUint64(repair.PlanIDs, planID)
	default:
		return true
	}
}

func allowOrg(filter map[int64]struct{}, orgID int64) bool {
	if len(filter) == 0 {
		return true
	}
	_, ok := filter[orgID]
	return ok
}

func dedupeTargets(targets []cachetarget.WarmupTarget) []cachetarget.WarmupTarget {
	if len(targets) == 0 {
		return nil
	}
	seen := make(map[string]cachetarget.WarmupTarget, len(targets))
	for _, target := range targets {
		if target.Scope == "" {
			continue
		}
		seen[target.Key()] = target
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]cachetarget.WarmupTarget, 0, len(keys))
	for _, key := range keys {
		result = append(result, seen[key])
	}
	return result
}

func containsFold(items []string, want string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}

func containsUint64(items []uint64, want uint64) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
