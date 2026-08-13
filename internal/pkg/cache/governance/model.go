// Package governance defines the component-neutral cache governance contract.
// It intentionally contains no transport or component-specific dependencies.
package governance

import (
	"time"

	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/observability"
)

// PolicyView is the JSON-safe projection of one effective cache policy layer.
type PolicyView struct {
	TTL            string  `json:"ttl"`
	NegativeTTL    string  `json:"negative_ttl"`
	TTLJitterRatio float64 `json:"ttl_jitter_ratio"`
	Compress       string  `json:"compress"`
	Singleflight   string  `json:"singleflight"`
	Negative       string  `json:"negative"`
}

// CapabilityPolicyView describes one process-local capability and its merged policy.
type CapabilityPolicyView struct {
	Capability    string     `json:"capability"`
	Owner         string     `json:"owner"`
	Kind          string     `json:"kind"`
	Layer         string     `json:"layer"`
	Family        string     `json:"family"`
	Enabled       bool       `json:"enabled"`
	SpecDefault   PolicyView `json:"spec_default"`
	GlobalDefault PolicyView `json:"global_default"`
	FamilyDefault PolicyView `json:"family_default"`
	Override      PolicyView `json:"override"`
	Effective     PolicyView `json:"effective"`
	Source        string     `json:"source"`
	MetricLabel   string     `json:"metric_label"`
	TopologyGroup string     `json:"topology_group,omitempty"`
	TopologyOrder int        `json:"topology_order,omitempty"`
	ReadModel     string     `json:"read_model,omitempty"`
}

type PolicyReloadStatus struct {
	LastAttemptAt time.Time `json:"last_attempt_at,omitempty"`
	LastSuccessAt time.Time `json:"last_success_at,omitempty"`
	LastFailureAt time.Time `json:"last_failure_at,omitempty"`
	LastError     string    `json:"last_error,omitempty"`
}

type PolicySourceView struct {
	Component     string `json:"component"`
	SchemaVersion string `json:"schema_version"`
	Path          string `json:"path"`
	PolicySHA256  string `json:"policy_sha256"`
}

type EffectiveRegistrySnapshot struct {
	SnapshotVersion uint64                 `json:"snapshot_version"`
	CatalogVersion  string                 `json:"catalog_version"`
	GeneratedAt     time.Time              `json:"generated_at"`
	Capabilities    []CapabilityPolicyView `json:"capabilities"`
	Reload          PolicyReloadStatus     `json:"reload"`
	PolicySource    *PolicySourceView      `json:"policy_source,omitempty"`
}

// ComponentCacheGovernanceSnapshot is the read-only process contract exposed
// by /governance/cache. It never reloads policy or reads business storage.
type ComponentCacheGovernanceSnapshot struct {
	GeneratedAt       time.Time                     `json:"generated_at"`
	Component         string                        `json:"component"`
	InstanceID        string                        `json:"instance_id"`
	Generation        string                        `json:"generation"`
	RedisRuntime      observability.RuntimeSnapshot `json:"redis_runtime"`
	EffectiveRegistry EffectiveRegistrySnapshot     `json:"effective_registry"`
	L1Runtime         []L1CapabilityRuntime         `json:"l1_runtime,omitempty"`
}

type L1CapabilityRuntime struct {
	Capability    string              `json:"capability"`
	Enabled       bool                `json:"enabled"`
	Buckets       []L1BucketRuntime   `json:"buckets"`
	SignalWatcher SignalWatcherStatus `json:"signal_watcher"`
}

type L1BucketRuntime struct {
	Bucket            string `json:"bucket"`
	Entries           int    `json:"entries"`
	MaxEntries        int    `json:"max_entries"`
	Hits              uint64 `json:"hits"`
	Misses            uint64 `json:"misses"`
	FIFOEvictions     uint64 `json:"fifo_evictions"`
	TTLExpirations    uint64 `json:"ttl_expirations"`
	ExplicitDeletions uint64 `json:"explicit_deletions"`
	SignalDeletions   uint64 `json:"signal_deletions"`
}

type SignalWatcherStatus struct {
	Configured     bool      `json:"configured"`
	Status         string    `json:"status"`
	LastSignalAt   time.Time `json:"last_signal_at,omitempty"`
	LastEvictionAt time.Time `json:"last_eviction_at,omitempty"`
	LastErrorAt    time.Time `json:"last_error_at,omitempty"`
	LastError      string    `json:"last_error,omitempty"`
	ReconnectCount uint64    `json:"reconnect_count"`
}

// ProjectRegistry creates a stable JSON projection from the immutable Registry.
func ProjectRegistry(registry *sharedcache.Registry, reload PolicyReloadStatus) EffectiveRegistrySnapshot {
	result := EffectiveRegistrySnapshot{Capabilities: []CapabilityPolicyView{}, Reload: reload}
	if registry == nil {
		return result
	}
	snapshot := registry.Snapshot()
	result.SnapshotVersion = snapshot.Version
	result.GeneratedAt = snapshot.GeneratedAt
	if snapshot.PolicySource != nil {
		result.PolicySource = &PolicySourceView{
			Component: snapshot.PolicySource.Component, SchemaVersion: snapshot.PolicySource.SchemaVersion,
			Path: snapshot.PolicySource.Path, PolicySHA256: snapshot.PolicySource.PolicySHA256,
		}
	}
	for _, item := range snapshot.Capabilities {
		if result.CatalogVersion == "" {
			result.CatalogVersion = item.CatalogVersion
		}
		result.Capabilities = append(result.Capabilities, CapabilityPolicyView{
			Capability: string(item.Capability), Owner: item.Owner, Kind: string(item.Kind), Layer: string(item.Layer),
			Family: item.Family, Enabled: item.Enabled, SpecDefault: projectPolicy(item.Layers.SpecDefault),
			GlobalDefault: projectPolicy(item.Layers.GlobalDefault), FamilyDefault: projectPolicy(item.Layers.FamilyDefault),
			Override: projectPolicy(item.Layers.Override), Effective: projectPolicy(item.Policy),
			Source: item.Source, MetricLabel: item.MetricLabel, TopologyGroup: item.TopologyGroup,
			TopologyOrder: item.TopologyOrder, ReadModel: item.ReadModel,
		})
	}
	return result
}

func projectPolicy(policy sharedcache.Policy) PolicyView {
	return PolicyView{
		TTL: policy.TTL.String(), NegativeTTL: policy.NegativeTTL.String(), TTLJitterRatio: policy.JitterRatio,
		Compress: projectSwitch(policy.Compress), Singleflight: projectSwitch(policy.Singleflight), Negative: projectSwitch(policy.Negative),
	}
}

func projectSwitch(value sharedcache.PolicySwitch) string {
	switch value {
	case sharedcache.PolicySwitchEnabled:
		return "enabled"
	case sharedcache.PolicySwitchDisabled:
		return "disabled"
	default:
		return "inherit"
	}
}
