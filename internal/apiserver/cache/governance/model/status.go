package cachemodel

import (
	"time"

	sharedgovernance "github.com/FangcunMount/qs-server/internal/pkg/cache/governance"
)

// RuntimeSnapshot is the Redis-free cache runtime projection exposed to
// application consumers. Infrastructure converts redisruntime state into this
// contract at the governance boundary.
type RuntimeSnapshot struct {
	GeneratedAt time.Time      `json:"generated_at"`
	Component   string         `json:"component"`
	InstanceID  string         `json:"instance_id,omitempty"`
	Generation  string         `json:"generation,omitempty"`
	Summary     RuntimeSummary `json:"summary"`
	Families    []FamilyStatus `json:"families"`
}

type RuntimeSummary struct {
	FamilyTotal      int  `json:"family_total"`
	AvailableCount   int  `json:"available_count"`
	DegradedCount    int  `json:"degraded_count"`
	UnavailableCount int  `json:"unavailable_count"`
	Ready            bool `json:"ready"`
}

type FamilyStatus struct {
	Component           string    `json:"component"`
	Family              string    `json:"family"`
	Profile             string    `json:"profile"`
	Namespace           string    `json:"namespace"`
	AllowWarmup         bool      `json:"allow_warmup"`
	Configured          bool      `json:"configured"`
	Available           bool      `json:"available"`
	Degraded            bool      `json:"degraded"`
	Mode                string    `json:"mode"`
	LastError           string    `json:"last_error,omitempty"`
	LastSuccessAt       time.Time `json:"last_success_at,omitempty"`
	LastFailureAt       time.Time `json:"last_failure_at,omitempty"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
	UpdatedAt           time.Time `json:"updated_at"`
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

type StatusSnapshot struct {
	RuntimeSnapshot
	Warmup            WarmupStatusSnapshot       `json:"warmup"`
	EffectiveRegistry *EffectiveRegistrySnapshot `json:"effective_registry,omitempty"`
}

// The following transport projections intentionally remain concrete rather
// than aliases. swag cannot resolve cross-package aliases reliably, while the
// conversion helpers keep the shared governance contract as the runtime truth.
type PolicyView struct {
	TTL            string  `json:"ttl"`
	NegativeTTL    string  `json:"negative_ttl"`
	TTLJitterRatio float64 `json:"ttl_jitter_ratio"`
	Compress       string  `json:"compress"`
	Singleflight   string  `json:"singleflight"`
	Negative       string  `json:"negative"`
}

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

// ComponentCacheGovernanceSnapshot is the concrete Swagger projection for
// the shared /governance/cache wire contract.
type ComponentCacheGovernanceSnapshot struct {
	GeneratedAt       time.Time                 `json:"generated_at"`
	Component         string                    `json:"component"`
	InstanceID        string                    `json:"instance_id"`
	Generation        string                    `json:"generation"`
	RedisRuntime      RuntimeSnapshot           `json:"redis_runtime"`
	EffectiveRegistry EffectiveRegistrySnapshot `json:"effective_registry"`
	L1Runtime         []L1CapabilityRuntime     `json:"l1_runtime,omitempty"`
}

func FromSharedEffectiveRegistry(value sharedgovernance.EffectiveRegistrySnapshot) EffectiveRegistrySnapshot {
	result := EffectiveRegistrySnapshot{
		SnapshotVersion: value.SnapshotVersion,
		CatalogVersion:  value.CatalogVersion,
		GeneratedAt:     value.GeneratedAt,
		Reload: PolicyReloadStatus{
			LastAttemptAt: value.Reload.LastAttemptAt, LastSuccessAt: value.Reload.LastSuccessAt,
			LastFailureAt: value.Reload.LastFailureAt, LastError: value.Reload.LastError,
		},
		Capabilities: make([]CapabilityPolicyView, 0, len(value.Capabilities)),
	}
	if value.PolicySource != nil {
		result.PolicySource = &PolicySourceView{
			Component: value.PolicySource.Component, SchemaVersion: value.PolicySource.SchemaVersion,
			Path: value.PolicySource.Path, PolicySHA256: value.PolicySource.PolicySHA256,
		}
	}
	for _, capability := range value.Capabilities {
		result.Capabilities = append(result.Capabilities, CapabilityPolicyView{
			Capability: capability.Capability, Owner: capability.Owner, Kind: capability.Kind, Layer: capability.Layer,
			Family: capability.Family, Enabled: capability.Enabled, SpecDefault: fromSharedPolicy(capability.SpecDefault),
			GlobalDefault: fromSharedPolicy(capability.GlobalDefault), FamilyDefault: fromSharedPolicy(capability.FamilyDefault),
			Override: fromSharedPolicy(capability.Override), Effective: fromSharedPolicy(capability.Effective),
			Source: capability.Source, MetricLabel: capability.MetricLabel, TopologyGroup: capability.TopologyGroup,
			TopologyOrder: capability.TopologyOrder, ReadModel: capability.ReadModel,
		})
	}
	return result
}

func (value EffectiveRegistrySnapshot) Shared() sharedgovernance.EffectiveRegistrySnapshot {
	result := sharedgovernance.EffectiveRegistrySnapshot{
		SnapshotVersion: value.SnapshotVersion, CatalogVersion: value.CatalogVersion, GeneratedAt: value.GeneratedAt,
		Reload: sharedgovernance.PolicyReloadStatus{
			LastAttemptAt: value.Reload.LastAttemptAt, LastSuccessAt: value.Reload.LastSuccessAt,
			LastFailureAt: value.Reload.LastFailureAt, LastError: value.Reload.LastError,
		},
		Capabilities: make([]sharedgovernance.CapabilityPolicyView, 0, len(value.Capabilities)),
	}
	if value.PolicySource != nil {
		result.PolicySource = &sharedgovernance.PolicySourceView{
			Component: value.PolicySource.Component, SchemaVersion: value.PolicySource.SchemaVersion,
			Path: value.PolicySource.Path, PolicySHA256: value.PolicySource.PolicySHA256,
		}
	}
	for _, capability := range value.Capabilities {
		result.Capabilities = append(result.Capabilities, sharedgovernance.CapabilityPolicyView{
			Capability: capability.Capability, Owner: capability.Owner, Kind: capability.Kind, Layer: capability.Layer,
			Family: capability.Family, Enabled: capability.Enabled, SpecDefault: capability.SpecDefault.shared(),
			GlobalDefault: capability.GlobalDefault.shared(), FamilyDefault: capability.FamilyDefault.shared(),
			Override: capability.Override.shared(), Effective: capability.Effective.shared(),
			Source: capability.Source, MetricLabel: capability.MetricLabel, TopologyGroup: capability.TopologyGroup,
			TopologyOrder: capability.TopologyOrder, ReadModel: capability.ReadModel,
		})
	}
	return result
}

func FromSharedL1Runtime(values []sharedgovernance.L1CapabilityRuntime) []L1CapabilityRuntime {
	result := make([]L1CapabilityRuntime, 0, len(values))
	for _, value := range values {
		result = append(result, FromSharedL1CapabilityRuntime(value))
	}
	return result
}

func FromSharedL1CapabilityRuntime(value sharedgovernance.L1CapabilityRuntime) L1CapabilityRuntime {
	buckets := make([]L1BucketRuntime, 0, len(value.Buckets))
	for _, bucket := range value.Buckets {
		buckets = append(buckets, L1BucketRuntime{
			Bucket: bucket.Bucket, Entries: bucket.Entries, MaxEntries: bucket.MaxEntries,
			Hits: bucket.Hits, Misses: bucket.Misses, FIFOEvictions: bucket.FIFOEvictions,
			TTLExpirations: bucket.TTLExpirations, ExplicitDeletions: bucket.ExplicitDeletions,
			SignalDeletions: bucket.SignalDeletions,
		})
	}
	return L1CapabilityRuntime{
		Capability: value.Capability, Enabled: value.Enabled, Buckets: buckets,
		SignalWatcher: SignalWatcherStatus{
			Configured: value.SignalWatcher.Configured, Status: value.SignalWatcher.Status,
			LastSignalAt: value.SignalWatcher.LastSignalAt, LastEvictionAt: value.SignalWatcher.LastEvictionAt,
			LastErrorAt: value.SignalWatcher.LastErrorAt, LastError: value.SignalWatcher.LastError,
			ReconnectCount: value.SignalWatcher.ReconnectCount,
		},
	}
}

func fromSharedPolicy(value sharedgovernance.PolicyView) PolicyView {
	return PolicyView{
		TTL: value.TTL, NegativeTTL: value.NegativeTTL, TTLJitterRatio: value.TTLJitterRatio,
		Compress: value.Compress, Singleflight: value.Singleflight, Negative: value.Negative,
	}
}

func (value PolicyView) shared() sharedgovernance.PolicyView {
	return sharedgovernance.PolicyView{
		TTL: value.TTL, NegativeTTL: value.NegativeTTL, TTLJitterRatio: value.TTLJitterRatio,
		Compress: value.Compress, Singleflight: value.Singleflight, Negative: value.Negative,
	}
}

type CachePolicyReloadRequest struct {
	ExpectedVersion uint64 `json:"expected_version"`
	ActorUserID     uint64 `json:"-"`
}

type CachePolicyReloadResult struct {
	PreviousVersion     uint64   `json:"previous_version"`
	CurrentVersion      uint64   `json:"current_version"`
	Changed             bool     `json:"changed"`
	Source              string   `json:"source"`
	ChangedCapabilities []string `json:"changed_capabilities"`
}

type ManualWarmupItemStatus string

const (
	ManualWarmupItemStatusOK      ManualWarmupItemStatus = "ok"
	ManualWarmupItemStatusSkipped ManualWarmupItemStatus = "skipped"
	ManualWarmupItemStatusError   ManualWarmupItemStatus = "error"
)

type ManualWarmupSummary struct {
	TargetCount  int    `json:"target_count"`
	OkCount      int    `json:"ok_count"`
	SkippedCount int    `json:"skipped_count"`
	ErrorCount   int    `json:"error_count"`
	Result       string `json:"result"`
}

type ManualWarmupItemResult struct {
	Family  string                 `json:"family"`
	Kind    string                 `json:"kind"`
	Scope   string                 `json:"scope"`
	Status  ManualWarmupItemStatus `json:"status"`
	Message string                 `json:"message,omitempty"`
}

type ManualWarmupResult struct {
	Trigger    string                   `json:"trigger"`
	StartedAt  time.Time                `json:"started_at"`
	FinishedAt time.Time                `json:"finished_at"`
	Summary    ManualWarmupSummary      `json:"summary"`
	Items      []ManualWarmupItemResult `json:"items"`
}
