package cache

import (
	"errors"
	"reflect"
	"sort"
	"sync/atomic"
	"time"
)

type Layer string
type CapabilityKind string

const (
	LayerL1      Layer = "L1"
	LayerL2      Layer = "L2"
	LayerL1L2    Layer = "L1+L2"
	LayerRuntime Layer = "runtime"

	KindCache            CapabilityKind = "cache"
	KindOperationalState CapabilityKind = "operational_state"
)

var ErrRegistryVersionConflict = errors.New("cache registry version conflict")

type PolicyLayers struct {
	SpecDefault   Policy `json:"spec_default"`
	GlobalDefault Policy `json:"global_default"`
	FamilyDefault Policy `json:"family_default"`
	Override      Policy `json:"override"`
}

// EffectiveCapability is the process-resolved cache capability contract.
type EffectiveCapability struct {
	Capability     Capability     `json:"capability"`
	Owner          string         `json:"owner"`
	Kind           CapabilityKind `json:"kind"`
	Layer          Layer          `json:"layer"`
	Family         string         `json:"family"`
	Enabled        bool           `json:"enabled"`
	Layers         PolicyLayers   `json:"layers"`
	Policy         Policy         `json:"policy"`
	Source         string         `json:"source"`
	CatalogVersion string         `json:"catalog_version"`
	MetricLabel    string         `json:"metric_label"`
}

type RegistrySnapshot struct {
	Version      uint64                `json:"version"`
	GeneratedAt  time.Time             `json:"generated_at"`
	Capabilities []EffectiveCapability `json:"capabilities"`
}

type PolicyProvider interface {
	Resolve(Capability) (EffectiveCapability, bool)
	Version() uint64
	All() []EffectiveCapability
}

type SnapshotPublisher interface {
	Publish(expectedVersion uint64, capabilities []EffectiveCapability, generatedAt time.Time) (PublishResult, error)
}

type PublishResult struct {
	PreviousVersion uint64
	CurrentVersion  uint64
	Changed         bool
}

// Registry publishes immutable, process-wide effective-policy snapshots.
type Registry struct {
	snapshot atomic.Pointer[RegistrySnapshot]
}

func NewRegistry(entries ...EffectiveCapability) *Registry {
	r := &Registry{}
	r.snapshot.Store(newRegistrySnapshot(1, time.Now(), entries))
	return r
}

func (r *Registry) Resolve(id Capability) (EffectiveCapability, bool) {
	if r == nil {
		return EffectiveCapability{}, false
	}
	snapshot := r.snapshot.Load()
	if snapshot == nil {
		return EffectiveCapability{}, false
	}
	index := sort.Search(len(snapshot.Capabilities), func(i int) bool {
		return snapshot.Capabilities[i].Capability >= id
	})
	if index >= len(snapshot.Capabilities) || snapshot.Capabilities[index].Capability != id {
		return EffectiveCapability{}, false
	}
	return snapshot.Capabilities[index], true
}

func (r *Registry) Version() uint64 {
	if r == nil || r.snapshot.Load() == nil {
		return 0
	}
	return r.snapshot.Load().Version
}

func (r *Registry) All() []EffectiveCapability {
	if r == nil || r.snapshot.Load() == nil {
		return nil
	}
	return append([]EffectiveCapability(nil), r.snapshot.Load().Capabilities...)
}

func (r *Registry) Snapshot() RegistrySnapshot {
	if r == nil || r.snapshot.Load() == nil {
		return RegistrySnapshot{}
	}
	snapshot := r.snapshot.Load()
	return RegistrySnapshot{
		Version: snapshot.Version, GeneratedAt: snapshot.GeneratedAt,
		Capabilities: append([]EffectiveCapability(nil), snapshot.Capabilities...),
	}
}

func (r *Registry) Publish(expectedVersion uint64, capabilities []EffectiveCapability, generatedAt time.Time) (PublishResult, error) {
	if r == nil {
		return PublishResult{}, ErrRegistryVersionConflict
	}
	for {
		current := r.snapshot.Load()
		if current == nil || current.Version != expectedVersion {
			return PublishResult{}, ErrRegistryVersionConflict
		}
		nextCapabilities := normalizeCapabilities(capabilities)
		result := PublishResult{PreviousVersion: current.Version, CurrentVersion: current.Version}
		if reflect.DeepEqual(current.Capabilities, nextCapabilities) {
			return result, nil
		}
		if generatedAt.IsZero() {
			generatedAt = time.Now()
		}
		next := &RegistrySnapshot{
			Version: current.Version + 1, GeneratedAt: generatedAt,
			Capabilities: nextCapabilities,
		}
		if r.snapshot.CompareAndSwap(current, next) {
			result.CurrentVersion = next.Version
			result.Changed = true
			return result, nil
		}
	}
}

func newRegistrySnapshot(version uint64, generatedAt time.Time, capabilities []EffectiveCapability) *RegistrySnapshot {
	if generatedAt.IsZero() {
		generatedAt = time.Now()
	}
	return &RegistrySnapshot{Version: version, GeneratedAt: generatedAt, Capabilities: normalizeCapabilities(capabilities)}
}

func normalizeCapabilities(capabilities []EffectiveCapability) []EffectiveCapability {
	result := append([]EffectiveCapability(nil), capabilities...)
	sort.Slice(result, func(i, j int) bool { return result[i].Capability < result[j].Capability })
	return result
}
