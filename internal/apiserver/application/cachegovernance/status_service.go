package cachegovernance

import (
	"context"
	"time"

	cacheinfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
)

type StatusService interface {
	GetRuntime(context.Context) (*cacheobservability.RuntimeSnapshot, error)
	GetStatus(context.Context) (*StatusSnapshot, error)
	GetHotset(context.Context, cacheinfra.WarmupKind, int64) (*HotsetSnapshot, error)
}

type StatusSnapshot struct {
	cacheobservability.RuntimeSnapshot
	Warmup WarmupStatusSnapshot `json:"warmup"`
}

type HotsetSnapshot struct {
	Family    redisplane.Family       `json:"family"`
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

func (s *governanceStatusService) GetRuntime(ctx context.Context) (*cacheobservability.RuntimeSnapshot, error) {
	_ = ctx
	component := ""
	if s != nil {
		component = s.component
	}
	result := cacheobservability.RuntimeSnapshot{
		GeneratedAt: time.Now(),
		Component:   component,
		Families:    []cacheobservability.FamilyStatus{},
		Summary: cacheobservability.RuntimeSummary{
			Ready: true,
		},
	}
	if s == nil {
		return &result, nil
	}
	snapshot := cacheobservability.SnapshotForComponent(s.component, s.status)
	return &snapshot, nil
}

func (s *governanceStatusService) GetStatus(ctx context.Context) (*StatusSnapshot, error) {
	runtimeSnapshot, err := s.GetRuntime(ctx)
	if err != nil {
		return nil, err
	}
	result := &StatusSnapshot{RuntimeSnapshot: *runtimeSnapshot}
	if s.coord != nil {
		result.Warmup = s.coord.Snapshot()
	}
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

func (s *governanceStatusService) familyStatus(family redisplane.Family) *cacheobservability.FamilyStatus {
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

func warmupKindFamily(kind cacheinfra.WarmupKind) redisplane.Family {
	switch kind {
	case cacheinfra.WarmupKindStaticScale, cacheinfra.WarmupKindStaticQuestionnaire, cacheinfra.WarmupKindStaticScaleList:
		return redisplane.FamilyStatic
	case cacheinfra.WarmupKindQueryStatsSystem, cacheinfra.WarmupKindQueryStatsQuestionnaire, cacheinfra.WarmupKindQueryStatsPlan:
		return redisplane.FamilyQuery
	default:
		return redisplane.FamilyDefault
	}
}
