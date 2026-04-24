package cachegovernance

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
)

type StatusService interface {
	GetRuntime(context.Context) (*cacheobservability.RuntimeSnapshot, error)
	GetStatus(context.Context) (*StatusSnapshot, error)
	GetHotset(context.Context, cachetarget.WarmupKind, int64) (*HotsetSnapshot, error)
}

type StatusSnapshot struct {
	cacheobservability.RuntimeSnapshot
	Warmup WarmupStatusSnapshot `json:"warmup"`
}

type HotsetSnapshot struct {
	Family    redisplane.Family        `json:"family"`
	Kind      cachetarget.WarmupKind   `json:"kind"`
	Limit     int64                    `json:"limit"`
	Available bool                     `json:"available"`
	Degraded  bool                     `json:"degraded"`
	Message   string                   `json:"message,omitempty"`
	Items     []cachetarget.HotsetItem `json:"items"`
}

type governanceStatusService struct {
	component string
	status    *cacheobservability.FamilyStatusRegistry
	hotset    cachetarget.HotsetInspector
	coord     Coordinator
}

func NewStatusService(component string, status *cacheobservability.FamilyStatusRegistry, hotset cachetarget.HotsetInspector, coord Coordinator) StatusService {
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

func (s *governanceStatusService) GetHotset(ctx context.Context, kind cachetarget.WarmupKind, limit int64) (*HotsetSnapshot, error) {
	family := warmupKindFamily(kind)
	result := &HotsetSnapshot{
		Family: family,
		Kind:   kind,
		Limit:  limit,
		Items:  []cachetarget.HotsetItem{},
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

func warmupKindFamily(kind cachetarget.WarmupKind) redisplane.Family {
	switch kind {
	case cachetarget.WarmupKindStaticScale, cachetarget.WarmupKindStaticQuestionnaire, cachetarget.WarmupKindStaticScaleList:
		return redisplane.FamilyStatic
	case cachetarget.WarmupKindQueryStatsSystem, cachetarget.WarmupKindQueryStatsQuestionnaire, cachetarget.WarmupKindQueryStatsPlan:
		return redisplane.FamilyQuery
	default:
		return redisplane.FamilyDefault
	}
}
