package cachegovernance

import (
	"context"
	"sort"
	"time"

	cacheinfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
)

type StatusService interface {
	GetStatus(context.Context) (*StatusSnapshot, error)
	GetHotset(context.Context, cacheinfra.WarmupKind, int64) (*HotsetSnapshot, error)
}

type StatusSnapshot struct {
	GeneratedAt time.Time                         `json:"generated_at"`
	Summary     StatusSummary                     `json:"summary"`
	Families    []cacheobservability.FamilyStatus `json:"families"`
	Warmup      WarmupStatusSnapshot              `json:"warmup"`
}

type StatusSummary struct {
	FamilyTotal      int  `json:"family_total"`
	AvailableCount   int  `json:"available_count"`
	DegradedCount    int  `json:"degraded_count"`
	UnavailableCount int  `json:"unavailable_count"`
	WarmupEnabled    bool `json:"warmup_enabled"`
	HotsetEnabled    bool `json:"hotset_enabled"`
}

type HotsetSnapshot struct {
	Family    cacheinfra.CacheFamily  `json:"family"`
	Kind      cacheinfra.WarmupKind   `json:"kind"`
	Limit     int64                   `json:"limit"`
	Available bool                    `json:"available"`
	Degraded  bool                    `json:"degraded"`
	Message   string                  `json:"message,omitempty"`
	Items     []cacheinfra.HotsetItem `json:"items"`
}

type governanceStatusService struct {
	component string
	status    *cacheobservability.FamilyStatusRegistry
	hotset    cacheinfra.HotsetInspector
	coord     Coordinator
}

func NewStatusService(component string, status *cacheobservability.FamilyStatusRegistry, hotset cacheinfra.HotsetInspector, coord Coordinator) StatusService {
	return &governanceStatusService{
		component: component,
		status:    status,
		hotset:    hotset,
		coord:     coord,
	}
}

func (s *governanceStatusService) GetStatus(ctx context.Context) (*StatusSnapshot, error) {
	_ = ctx
	result := &StatusSnapshot{
		GeneratedAt: time.Now(),
	}
	if s == nil {
		return result, nil
	}
	if s.status != nil {
		all := s.status.Snapshot()
		result.Families = make([]cacheobservability.FamilyStatus, 0, len(all))
		for _, item := range all {
			if s.component != "" && item.Component != s.component {
				continue
			}
			result.Families = append(result.Families, item)
		}
		sort.SliceStable(result.Families, func(i, j int) bool {
			left := familyDisplayOrder(result.Families[i].Family)
			right := familyDisplayOrder(result.Families[j].Family)
			if left == right {
				return result.Families[i].Family < result.Families[j].Family
			}
			return left < right
		})
	}
	if s.coord != nil {
		result.Warmup = s.coord.Snapshot()
	}
	result.Summary = summarizeStatus(result.Families, result.Warmup)
	return result, nil
}

func (s *governanceStatusService) GetHotset(ctx context.Context, kind cacheinfra.WarmupKind, limit int64) (*HotsetSnapshot, error) {
	family := warmupKindFamily(kind)
	result := &HotsetSnapshot{
		Family: family,
		Kind:   kind,
		Limit:  limit,
		Items:  []cacheinfra.HotsetItem{},
	}
	if limit <= 0 {
		result.Limit = 20
	}
	status := s.familyStatus(family)
	if status != nil {
		result.Available = status.Available
		result.Degraded = status.Degraded
		if status.LastError != "" {
			result.Message = status.LastError
		}
	}
	if s == nil || s.hotset == nil {
		if result.Message == "" {
			result.Message = "hotset inspector unavailable"
		}
		return result, nil
	}
	items, err := s.hotset.TopWithScores(ctx, family, kind, result.Limit)
	if err != nil {
		result.Degraded = true
		result.Available = false
		result.Message = err.Error()
		return result, nil
	}
	result.Available = true
	result.Items = items
	return result, nil
}

func (s *governanceStatusService) familyStatus(family cacheinfra.CacheFamily) *cacheobservability.FamilyStatus {
	if s == nil || s.status == nil {
		return nil
	}
	for _, item := range s.status.Snapshot() {
		if item.Component == s.component && item.Family == string(family) {
			value := item
			return &value
		}
	}
	return nil
}

func warmupKindFamily(kind cacheinfra.WarmupKind) cacheinfra.CacheFamily {
	switch kind {
	case cacheinfra.WarmupKindStaticScale, cacheinfra.WarmupKindStaticQuestionnaire, cacheinfra.WarmupKindStaticScaleList:
		return cacheinfra.CacheFamilyStatic
	case cacheinfra.WarmupKindQueryStatsSystem, cacheinfra.WarmupKindQueryStatsQuestionnaire, cacheinfra.WarmupKindQueryStatsPlan:
		return cacheinfra.CacheFamilyQuery
	default:
		return cacheinfra.CacheFamilyDefault
	}
}

func summarizeStatus(families []cacheobservability.FamilyStatus, warmup WarmupStatusSnapshot) StatusSummary {
	summary := StatusSummary{
		FamilyTotal:   len(families),
		WarmupEnabled: warmup.Enabled,
		HotsetEnabled: warmup.Hotset.Enable,
	}
	for _, family := range families {
		if family.Available && !family.Degraded {
			summary.AvailableCount++
		}
		if family.Degraded {
			summary.DegradedCount++
		}
		if !family.Available {
			summary.UnavailableCount++
		}
	}
	return summary
}

func familyDisplayOrder(family string) int {
	switch family {
	case string(cacheinfra.CacheFamilyStatic):
		return 0
	case string(cacheinfra.CacheFamilyObject):
		return 1
	case string(cacheinfra.CacheFamilyQuery):
		return 2
	case string(cacheinfra.CacheFamilyMeta):
		return 3
	case string(cacheinfra.CacheFamilySDK):
		return 4
	case string(cacheinfra.CacheFamilyLock):
		return 5
	default:
		return 99
	}
}
