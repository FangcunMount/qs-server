package cachegovernance

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/model"
	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
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
	registry  *sharedcache.Registry
	reloader  interface {
		ReloadStatus() cachemodel.PolicyReloadStatus
	}
}

func NewStatusService(component string, status *observability.FamilyStatusRegistry, hotset cachetarget.HotsetInspector, coord Coordinator, optional ...interface{}) StatusService {
	var registry *sharedcache.Registry
	var reloader interface {
		ReloadStatus() cachemodel.PolicyReloadStatus
	}
	for _, dependency := range optional {
		switch value := dependency.(type) {
		case *sharedcache.Registry:
			registry = value
		case interface {
			ReloadStatus() cachemodel.PolicyReloadStatus
		}:
			reloader = value
		}
	}
	return &governanceStatusService{
		component: component,
		status:    status,
		hotset:    hotset,
		coord:     coord,
		registry:  registry,
		reloader:  reloader,
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
	if s.registry != nil {
		result.EffectiveRegistry = projectEffectiveRegistry(s.registry, s.reloader)
	}
	return result, nil
}

func projectEffectiveRegistry(registry *sharedcache.Registry, reloader interface {
	ReloadStatus() cachemodel.PolicyReloadStatus
}) *cachemodel.EffectiveRegistrySnapshot {
	snapshot := registry.Snapshot()
	result := &cachemodel.EffectiveRegistrySnapshot{
		SnapshotVersion: snapshot.Version, GeneratedAt: snapshot.GeneratedAt,
		Capabilities: make([]cachemodel.CapabilityPolicyView, 0, len(snapshot.Capabilities)),
	}
	if reloader != nil {
		result.Reload = reloader.ReloadStatus()
	}
	for _, item := range snapshot.Capabilities {
		if result.CatalogVersion == "" {
			result.CatalogVersion = item.CatalogVersion
		}
		result.Capabilities = append(result.Capabilities, cachemodel.CapabilityPolicyView{
			Capability: string(item.Capability), Owner: item.Owner, Kind: string(item.Kind), Layer: string(item.Layer),
			Family: item.Family, Enabled: item.Enabled, SpecDefault: policyView(item.Layers.SpecDefault),
			GlobalDefault: policyView(item.Layers.GlobalDefault), FamilyDefault: policyView(item.Layers.FamilyDefault),
			Override: policyView(item.Layers.Override), Effective: policyView(item.Policy), Source: item.Source, MetricLabel: item.MetricLabel,
		})
	}
	return result
}

func policyView(policy sharedcache.Policy) cachemodel.PolicyView {
	return cachemodel.PolicyView{
		TTL: policy.TTL.String(), NegativeTTL: policy.NegativeTTL.String(), TTLJitterRatio: policy.JitterRatio,
		Compress: policySwitchView(policy.Compress), Singleflight: policySwitchView(policy.Singleflight), Negative: policySwitchView(policy.Negative),
	}
}

func policySwitchView(value sharedcache.PolicySwitch) string {
	switch value {
	case sharedcache.PolicySwitchEnabled:
		return "enabled"
	case sharedcache.PolicySwitchDisabled:
		return "disabled"
	default:
		return "inherit"
	}
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
