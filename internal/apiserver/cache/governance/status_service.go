package cachegovernance

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/model"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
)

type StatusService interface {
	GetRuntime(context.Context) (*cachemodel.RuntimeSnapshot, error)
	GetStatus(context.Context) (*cachemodel.StatusSnapshot, error)
	GetHotset(context.Context, cachetarget.WarmupKind, int64) (*cachetarget.HotsetSnapshot, error)
}

type governanceStatusService struct {
	component string
	status    *observability.FamilyStatusRegistry
	hotset    cachetarget.HotsetInspector
	coord     Coordinator
}

func NewStatusService(component string, status *observability.FamilyStatusRegistry, hotset cachetarget.HotsetInspector, coord Coordinator) StatusService {
	return &governanceStatusService{
		component: component,
		status:    status,
		hotset:    hotset,
		coord:     coord,
	}
}

func (s *governanceStatusService) GetRuntime(ctx context.Context) (*cachemodel.RuntimeSnapshot, error) {
	_ = ctx
	component := ""
	if s != nil {
		component = s.component
	}
	result := cachemodel.RuntimeSnapshot{
		GeneratedAt: time.Now(),
		Component:   component,
		Families:    []cachemodel.FamilyStatus{},
		Summary: cachemodel.RuntimeSummary{
			Ready: true,
		},
	}
	if s == nil {
		return &result, nil
	}
	snapshot := projectRuntimeSnapshot(observability.SnapshotForComponent(s.component, s.status))
	return &snapshot, nil
}

func (s *governanceStatusService) GetStatus(ctx context.Context) (*cachemodel.StatusSnapshot, error) {
	runtimeSnapshot, err := s.GetRuntime(ctx)
	if err != nil {
		return nil, err
	}
	result := &cachemodel.StatusSnapshot{RuntimeSnapshot: *runtimeSnapshot}
	if s.coord != nil {
		result.Warmup = s.coord.Snapshot()
	}
	return result, nil
}

func (s *governanceStatusService) GetHotset(ctx context.Context, kind cachetarget.WarmupKind, limit int64) (*cachetarget.HotsetSnapshot, error) {
	family := cachetarget.FamilyForKind(kind)
	result := &cachetarget.HotsetSnapshot{
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

func projectRuntimeSnapshot(in observability.RuntimeSnapshot) cachemodel.RuntimeSnapshot {
	out := cachemodel.RuntimeSnapshot{
		GeneratedAt: in.GeneratedAt,
		Component:   in.Component,
		Summary: cachemodel.RuntimeSummary{
			FamilyTotal:      in.Summary.FamilyTotal,
			AvailableCount:   in.Summary.AvailableCount,
			DegradedCount:    in.Summary.DegradedCount,
			UnavailableCount: in.Summary.UnavailableCount,
			Ready:            in.Summary.Ready,
		},
		Families: make([]cachemodel.FamilyStatus, 0, len(in.Families)),
	}
	for _, family := range in.Families {
		out.Families = append(out.Families, cachemodel.FamilyStatus{
			Component: family.Component, Family: family.Family, Profile: family.Profile,
			Namespace: family.Namespace, AllowWarmup: family.AllowWarmup,
			Configured: family.Configured, Available: family.Available,
			Degraded: family.Degraded, Mode: family.Mode, LastError: family.LastError,
			LastSuccessAt: family.LastSuccessAt, LastFailureAt: family.LastFailureAt,
			ConsecutiveFailures: family.ConsecutiveFailures, UpdatedAt: family.UpdatedAt,
		})
	}
	return out
}

func (s *governanceStatusService) familyStatus(family cachemodel.Family) *observability.FamilyStatus {
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
