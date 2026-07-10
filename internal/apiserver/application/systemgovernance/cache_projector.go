package systemgovernance

import (
	"context"
	"sort"
	"time"

	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
)

// CacheWarmupProjection 是diagnostic 缓存 governance 视图。
type CacheWarmupProjection struct {
	FamilyRows  []CacheFamilyRow
	WarmupKinds []CacheWarmupKind
	Hotsets     []CacheHotsetView
	Signals     []Signal
}

// CacheWarmupEvaluator 将缓存 运行时 快照转换为operator-facing 行 和 信号。
type CacheWarmupEvaluator struct {
	evidence MetricEvidenceReader
}

func NewCacheWarmupEvaluator(metrics MetricsReader) *CacheWarmupEvaluator {
	return &CacheWarmupEvaluator{evidence: NewMetricEvidenceReader(metrics)}
}

func (e *CacheWarmupEvaluator) Evaluate(
	ctx context.Context,
	components map[string]ComponentCache,
	hotsets []CacheHotsetView,
	window string,
	evalAt time.Time,
) CacheWarmupProjection {
	return e.evaluate(ctx, components, hotsets, nil, window, evalAt)
}

func (e *CacheWarmupEvaluator) EvaluateWithLatestRun(
	ctx context.Context,
	components map[string]ComponentCache,
	hotsets []CacheHotsetView,
	latest *observabilityWarmupLatestRun,
	window string,
	evalAt time.Time,
) CacheWarmupProjection {
	return e.evaluate(ctx, components, hotsets, latest, window, evalAt)
}

func (e *CacheWarmupEvaluator) evaluate(
	ctx context.Context,
	components map[string]ComponentCache,
	hotsets []CacheHotsetView,
	latest *observabilityWarmupLatestRun,
	window string,
	evalAt time.Time,
) CacheWarmupProjection {
	projection := CacheWarmupProjection{
		FamilyRows:  []CacheFamilyRow{},
		WarmupKinds: DefaultCacheWarmupKinds(),
		Hotsets:     e.withHotsetMetricEvidence(ctx, hotsets, window, evalAt),
		Signals:     []Signal{},
	}
	for name, component := range components {
		if !component.Available {
			projection.Signals = append(projection.Signals, Signal{
				ID:       "cache.component.unavailable." + name,
				Domain:   DomainCache,
				Severity: SeverityWarning,
				Status:   "component_unavailable",
				Title:    "Cache component snapshot unavailable: " + name,
				Evidence: map[string]interface{}{
					"component": name,
					"reason":    component.Reason,
				},
				DashboardKey: "cache_runtime",
			})
			continue
		}
		if component.Snapshot == nil {
			continue
		}
		if !component.Snapshot.Summary.Ready {
			projection.Signals = append(projection.Signals, Signal{
				ID:       cacheRuntimeSignalID(component.Snapshot.Component),
				Domain:   DomainCache,
				Severity: SeverityCritical,
				Status:   "not_ready",
				Title:    "Cache runtime is not ready",
				Evidence: map[string]interface{}{
					"component":      component.Snapshot.Component,
					"degraded_count": component.Snapshot.Summary.DegradedCount,
				},
				ActionIDs:    []string{"cache.repair_complete"},
				DashboardKey: "cache_runtime",
			})
		}
		for _, family := range component.Snapshot.Families {
			row := e.projectFamilyRow(ctx, family, window, evalAt)
			projection.FamilyRows = append(projection.FamilyRows, row)
			projection.Signals = append(projection.Signals, cacheFamilySignals(row)...)
		}
	}
	for _, hotset := range projection.Hotsets {
		if hotset.Degraded {
			projection.Signals = append(projection.Signals, Signal{
				ID:       "cache.hotset.degraded." + string(hotset.Kind),
				Domain:   DomainCache,
				Severity: SeverityWarning,
				Status:   "hotset_degraded",
				Title:    "Cache hotset unavailable: " + string(hotset.Kind),
				Evidence: map[string]interface{}{
					"family":  hotset.Family,
					"kind":    hotset.Kind,
					"message": hotset.Message,
				},
				MetricEvidence: hotset.MetricEvidence,
				ActionIDs:      []string{"cache.manual_warmup"},
				DashboardKey:   "cache_hotset",
			})
		}
	}
	if latest != nil {
		projection.Signals = append(projection.Signals, e.WarmupSignals(ctx, *latest, window, evalAt)...)
	}
	sortCacheFamilyRows(projection.FamilyRows)
	sortCacheHotsets(projection.Hotsets)
	projection.Signals = SortSignals(projection.Signals)
	return projection
}

func (e *CacheWarmupEvaluator) projectFamilyRow(
	ctx context.Context,
	family observability.FamilyStatus,
	window string,
	evalAt time.Time,
) CacheFamilyRow {
	row := CacheFamilyRow{
		Component:           family.Component,
		Family:              family.Family,
		Profile:             family.Profile,
		Namespace:           family.Namespace,
		AllowWarmup:         family.AllowWarmup,
		Configured:          family.Configured,
		Available:           family.Available,
		Degraded:            family.Degraded,
		Mode:                family.Mode,
		LastError:           family.LastError,
		LastSuccessAt:       family.LastSuccessAt,
		LastFailureAt:       family.LastFailureAt,
		ConsecutiveFailures: family.ConsecutiveFailures,
		UpdatedAt:           family.UpdatedAt,
		Severity:            SeverityHealthy,
		Reason:              family.LastError,
		MetricEvidence:      e.familyMetricEvidence(ctx, family, window, evalAt),
	}
	switch {
	case !family.Available:
		row.Severity = SeverityCritical
	case family.Degraded:
		row.Severity = SeverityWarning
	}
	return row
}

func (e *CacheWarmupEvaluator) familyMetricEvidence(
	ctx context.Context,
	family observability.FamilyStatus,
	window string,
	evalAt time.Time,
) []MetricEvidence {
	if e == nil {
		return nil
	}
	items := make([]MetricEvidence, 0, 2)
	if item, ok := e.evidence.CacheFamilyAvailable(ctx, family.Component, family.Family, family.Profile, window, evalAt); ok {
		items = append(items, item)
	}
	if item, ok := e.evidence.CacheFamilyDegraded(ctx, family.Component, family.Family, family.Profile, window, evalAt); ok {
		items = append(items, item)
	}
	return items
}

func cacheFamilySignals(row CacheFamilyRow) []Signal {
	if !row.Available {
		return []Signal{{
			ID:       cacheFamilySignalID("cache.family.unavailable", row),
			Domain:   DomainCache,
			Severity: SeverityCritical,
			Status:   "unavailable",
			Title:    "Cache family unavailable: " + row.Family,
			Evidence: map[string]interface{}{
				"family":     row.Family,
				"component":  row.Component,
				"last_error": row.LastError,
			},
			MetricEvidence: row.MetricEvidence,
			ActionIDs:      []string{"cache.repair_complete", "cache.manual_warmup"},
			DashboardKey:   "cache_runtime",
		}}
	}
	if row.Degraded {
		return []Signal{{
			ID:       cacheFamilySignalID("cache.family.degraded", row),
			Domain:   DomainCache,
			Severity: SeverityWarning,
			Status:   "degraded",
			Title:    "Cache family degraded: " + row.Family,
			Evidence: map[string]interface{}{
				"family":    row.Family,
				"component": row.Component,
			},
			MetricEvidence: row.MetricEvidence,
			ActionIDs:      []string{"cache.manual_warmup"},
			DashboardKey:   "cache_runtime",
		}}
	}
	return nil
}

func (e *CacheWarmupEvaluator) WarmupSignals(ctx context.Context, latest observabilityWarmupLatestRun, window string, evalAt time.Time) []Signal {
	if latest.ErrorCount <= 0 {
		return nil
	}
	metricEvidence := []MetricEvidence{}
	if e != nil {
		if item, ok := e.evidence.CacheWarmupRunsError(ctx, window, evalAt); ok {
			metricEvidence = append(metricEvidence, item)
		}
	}
	return []Signal{{
		ID:       "cache.warmup.error",
		Domain:   DomainCache,
		Severity: SeverityWarning,
		Status:   "warmup_error",
		Title:    "Cache warmup reported errors",
		Evidence: map[string]interface{}{
			"trigger":      latest.Trigger,
			"error_count":  latest.ErrorCount,
			"target_count": latest.TargetCount,
		},
		MetricEvidence: metricEvidence,
		ActionIDs:      []string{"cache.manual_warmup"},
		DashboardKey:   "cache_warmup",
	}}
}

type observabilityWarmupLatestRun struct {
	Trigger     string
	ErrorCount  int
	TargetCount int
}

func (e *CacheWarmupEvaluator) withHotsetMetricEvidence(
	ctx context.Context,
	hotsets []CacheHotsetView,
	window string,
	evalAt time.Time,
) []CacheHotsetView {
	result := make([]CacheHotsetView, 0, len(hotsets))
	for _, hotset := range hotsets {
		if e != nil && hotset.Kind != "" {
			if item, ok := e.evidence.CacheHotsetSize(ctx, string(hotset.Family), string(hotset.Kind), window, evalAt); ok {
				hotset.MetricEvidence = append(hotset.MetricEvidence, item)
			}
		}
		result = append(result, hotset)
	}
	return result
}

func CacheHotsetViewFromResponse(kind cachetarget.WarmupKind, response *statisticsApp.GovernanceHotsetResponse, err error) CacheHotsetView {
	if err != nil {
		return CacheHotsetView{
			Family:    cachetarget.FamilyForKind(kind),
			Kind:      kind,
			Limit:     5,
			Available: false,
			Degraded:  true,
			Message:   err.Error(),
			Items:     []CacheHotsetItem{},
		}
	}
	if response == nil {
		return CacheHotsetView{
			Family:    cachetarget.FamilyForKind(kind),
			Kind:      kind,
			Limit:     5,
			Available: false,
			Degraded:  true,
			Message:   "hotset response unavailable",
			Items:     []CacheHotsetItem{},
		}
	}
	items := make([]CacheHotsetItem, 0, len(response.Items))
	for _, item := range response.Items {
		items = append(items, CacheHotsetItem{
			Family: string(item.Target.Family),
			Kind:   item.Target.Kind,
			Scope:  item.Target.Scope,
			Score:  item.Score,
		})
	}
	family := response.Family
	if family == "" {
		family = cachetarget.FamilyForKind(kind)
	}
	responseKind := response.Kind
	if responseKind == "" {
		responseKind = kind
	}
	limit := response.Limit
	if limit <= 0 {
		limit = 5
	}
	return CacheHotsetView{
		Family:    family,
		Kind:      responseKind,
		Limit:     limit,
		Available: response.Available,
		Degraded:  response.Degraded,
		Message:   response.Message,
		Items:     items,
	}
}

func DefaultCacheWarmupKinds() []CacheWarmupKind {
	kinds := []cachetarget.WarmupKind{
		cachetarget.WarmupKindStaticScale,
		cachetarget.WarmupKindStaticQuestionnaire,
		cachetarget.WarmupKindStaticTypologyModel,
		cachetarget.WarmupKindQueryStatsOverview,
		cachetarget.WarmupKindQueryStatsSystem,
		cachetarget.WarmupKindQueryStatsQuestionnaire,
		cachetarget.WarmupKindQueryStatsPlan,
	}
	result := make([]CacheWarmupKind, 0, len(kinds))
	for _, kind := range kinds {
		result = append(result, CacheWarmupKind{
			Kind:                 kind,
			Family:               cachetarget.FamilyForKind(kind),
			ScopeExample:         cacheWarmupScopeExample(kind),
			SupportsManualWarmup: true,
		})
	}
	return result
}

func cacheWarmupScopeExample(kind cachetarget.WarmupKind) string {
	switch kind {
	case cachetarget.WarmupKindStaticScale:
		return "scale:S-001"
	case cachetarget.WarmupKindStaticQuestionnaire:
		return "questionnaire:Q-001"
	case cachetarget.WarmupKindStaticTypologyModel:
		return "typology_model:M-001"
	case cachetarget.WarmupKindQueryStatsOverview:
		return "org:1:preset:30d"
	case cachetarget.WarmupKindQueryStatsSystem:
		return "org:1"
	case cachetarget.WarmupKindQueryStatsQuestionnaire:
		return "org:1:questionnaire:Q-001"
	case cachetarget.WarmupKindQueryStatsPlan:
		return "org:1:plan:99"
	default:
		return ""
	}
}

func sortCacheFamilyRows(rows []CacheFamilyRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		if severityRank(rows[i].Severity) != severityRank(rows[j].Severity) {
			return severityRank(rows[i].Severity) > severityRank(rows[j].Severity)
		}
		if rows[i].Component != rows[j].Component {
			return rows[i].Component < rows[j].Component
		}
		return rows[i].Family < rows[j].Family
	})
}

func sortCacheHotsets(hotsets []CacheHotsetView) {
	sort.SliceStable(hotsets, func(i, j int) bool {
		if hotsets[i].Degraded != hotsets[j].Degraded {
			return hotsets[i].Degraded
		}
		return string(hotsets[i].Kind) < string(hotsets[j].Kind)
	})
}

func cacheRuntimeSignalID(component string) string {
	if component == "" || component == "apiserver" {
		return "cache.runtime.not_ready"
	}
	return "cache.runtime.not_ready." + component
}

func cacheFamilySignalID(prefix string, row CacheFamilyRow) string {
	if row.Component == "" || row.Component == "apiserver" {
		return prefix + "." + row.Family
	}
	return prefix + "." + row.Component + "." + row.Family
}
