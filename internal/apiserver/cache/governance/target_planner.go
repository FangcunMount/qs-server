package cachegovernance

import (
	"context"
	"sort"
	"strings"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/model"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"
)

// TargetPlanner 负责组装 governance 预热目标。
type TargetPlanner struct {
	cfg  Config
	deps Dependencies
}

func NewTargetPlanner(cfg Config, deps Dependencies) *TargetPlanner {
	return &TargetPlanner{cfg: cfg, deps: deps}
}

func (p *TargetPlanner) MergeQueryTargets(ctx context.Context, orgFilter []int64, repair *cachetarget.RepairCompleteRequest) []cachetarget.WarmupTarget {
	if p == nil {
		return nil
	}
	targets := make([]cachetarget.WarmupTarget, 0)
	targets = append(targets, p.querySeedTargets(orgFilter)...)
	targets = append(targets, p.queryHotTargets(ctx, orgFilter, repair)...)
	return dedupeTargets(targets)
}

func (p *TargetPlanner) querySeedTargets(orgFilter []int64) []cachetarget.WarmupTarget {
	if p.deps.StatisticsSeeds == nil {
		return nil
	}
	filter := make(map[int64]struct{}, len(orgFilter))
	for _, orgID := range orgFilter {
		if orgID > 0 {
			filter[orgID] = struct{}{}
		}
	}
	targets := make([]cachetarget.WarmupTarget, 0)
	for _, orgID := range p.deps.StatisticsSeeds.OrgIDs {
		if len(filter) > 0 {
			if _, ok := filter[orgID]; !ok {
				continue
			}
		}
		for _, preset := range overviewSeedPresets(p.deps.StatisticsSeeds.OverviewPresets) {
			targets = append(targets, cachetarget.NewQueryStatsOverviewWarmupTarget(orgID, preset))
		}
	}
	return targets
}

func overviewSeedPresets(configured []string) []string {
	if len(configured) == 0 {
		return []string{"latest_complete_day", "7d", "30d"}
	}
	result := make([]string, 0, len(configured))
	seen := map[string]struct{}{}
	for _, preset := range configured {
		preset = strings.ToLower(strings.TrimSpace(preset))
		switch preset {
		case "latest_complete_day", "7d", "30d":
			if _, ok := seen[preset]; ok {
				continue
			}
			seen[preset] = struct{}{}
			result = append(result, preset)
		}
	}
	if len(result) == 0 {
		return []string{"latest_complete_day", "7d", "30d"}
	}
	return result
}

func (p *TargetPlanner) queryHotTargets(ctx context.Context, orgFilter []int64, repair *cachetarget.RepairCompleteRequest) []cachetarget.WarmupTarget {
	if !p.cfg.HotsetEnable || p.deps.Hotset == nil {
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
	} {
		items, err := p.deps.Hotset.Top(ctx, cachemodel.FamilyQuery, kind, p.cfg.HotsetTopN)
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

func allowQueryTarget(target cachetarget.WarmupTarget, orgFilter map[int64]struct{}, repair *cachetarget.RepairCompleteRequest) bool {
	if len(orgFilter) == 0 && repair == nil {
		return true
	}
	switch target.Kind {
	case cachetarget.WarmupKindQueryStatsOverview:
		orgID, _, ok := cachetarget.ParseQueryStatsOverviewScope(target.Scope)
		return ok && allowOrg(orgFilter, orgID)
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
