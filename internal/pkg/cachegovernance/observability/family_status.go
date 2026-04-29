package observability

import (
	"sort"
	"sync"
	"time"
)

const (
	FamilyModeDefault         = "default"
	FamilyModeFallbackDefault = "fallback_default"
	FamilyModeNamedProfile    = "named_profile"
	FamilyModeDegraded        = "degraded"
	FamilyModeDisabled        = "disabled"
)

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

type FamilyStatusRegistry struct {
	component string
	mu        sync.RWMutex
	status    map[string]FamilyStatus
	baseMode  map[string]string
}

func NewFamilyStatusRegistry(component string) *FamilyStatusRegistry {
	registry := &FamilyStatusRegistry{
		component: component,
		status:    make(map[string]FamilyStatus),
		baseMode:  make(map[string]string),
	}
	RegisterFamilyStatusRegistry(registry)
	return registry
}

func familyStatusKey(component, family string) string {
	return component + ":" + family
}

func (r *FamilyStatusRegistry) Update(status FamilyStatus) {
	if r == nil {
		return
	}
	if status.Component == "" {
		status.Component = r.component
	}
	if status.Mode == "" {
		status.Mode = FamilyModeDisabled
	}
	status.UpdatedAt = time.Now()

	r.mu.Lock()
	prev, hadPrev := r.status[familyStatusKey(status.Component, status.Family)]
	if status.LastSuccessAt.IsZero() {
		status.LastSuccessAt = prev.LastSuccessAt
	}
	if status.LastFailureAt.IsZero() {
		status.LastFailureAt = prev.LastFailureAt
	}
	if status.ConsecutiveFailures == 0 && prev.ConsecutiveFailures > 0 {
		status.ConsecutiveFailures = prev.ConsecutiveFailures
	}
	if status.Mode != FamilyModeDegraded && status.Mode != FamilyModeDisabled {
		r.baseMode[familyStatusKey(status.Component, status.Family)] = status.Mode
	}
	r.status[familyStatusKey(status.Component, status.Family)] = status
	r.mu.Unlock()

	SetCacheFamilyAvailable(status.Component, status.Family, status.Profile, status.Available)
	if status.Degraded {
		if !hadPrev || !prev.Degraded || prev.Mode != status.Mode || prev.LastError != status.LastError {
			reason := status.LastError
			if reason == "" {
				reason = status.Mode
			}
			IncCacheFamilyDegraded(status.Component, status.Family, status.Profile, reason)
		}
	}
	SetRuntimeComponentReady(status.Component, SnapshotForComponent(status.Component, r).Summary.Ready)
}

func (r *FamilyStatusRegistry) RecordSuccess(family string) {
	if r == nil || family == "" {
		return
	}
	now := time.Now()
	key := familyStatusKey(r.component, family)

	r.mu.Lock()
	status, ok := r.status[key]
	if !ok {
		r.mu.Unlock()
		return
	}
	status.Available = true
	status.Degraded = false
	status.LastError = ""
	status.LastSuccessAt = now
	status.ConsecutiveFailures = 0
	if baseMode := r.baseMode[key]; baseMode != "" {
		status.Mode = baseMode
	}
	status.UpdatedAt = now
	r.status[key] = status
	r.mu.Unlock()

	SetCacheFamilyAvailable(status.Component, status.Family, status.Profile, true)
	SetRuntimeComponentReady(status.Component, SnapshotForComponent(status.Component, r).Summary.Ready)
}

func (r *FamilyStatusRegistry) RecordFailure(family string, err error) {
	if r == nil || family == "" || err == nil {
		return
	}
	now := time.Now()
	key := familyStatusKey(r.component, family)

	r.mu.Lock()
	status, ok := r.status[key]
	if !ok {
		r.mu.Unlock()
		return
	}
	prev := status
	status.Available = false
	status.Degraded = true
	status.Mode = FamilyModeDegraded
	status.LastError = err.Error()
	status.LastFailureAt = now
	status.ConsecutiveFailures++
	status.UpdatedAt = now
	r.status[key] = status
	r.mu.Unlock()

	SetCacheFamilyAvailable(status.Component, status.Family, status.Profile, false)
	if !prev.Degraded || prev.LastError != status.LastError {
		IncCacheFamilyDegraded(status.Component, status.Family, status.Profile, status.LastError)
	}
	SetRuntimeComponentReady(status.Component, SnapshotForComponent(status.Component, r).Summary.Ready)
}

func (r *FamilyStatusRegistry) Snapshot() []FamilyStatus {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]FamilyStatus, 0, len(r.status))
	for _, status := range r.status {
		result = append(result, status)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Component == result[j].Component {
			return result[i].Family < result[j].Family
		}
		return result[i].Component < result[j].Component
	})
	return result
}

var activeFamilyStatusRegistries sync.Map

func RegisterFamilyStatusRegistry(registry *FamilyStatusRegistry) {
	if registry == nil || registry.component == "" {
		return
	}
	activeFamilyStatusRegistries.Store(registry.component, registry)
}

func ObserveFamilySuccess(component, family string) {
	if component == "" || family == "" {
		return
	}
	if registry, ok := activeFamilyStatusRegistries.Load(component); ok {
		if typed, ok := registry.(*FamilyStatusRegistry); ok {
			typed.RecordSuccess(family)
		}
	}
}

func ObserveFamilyFailure(component, family string, err error) {
	if component == "" || family == "" || err == nil {
		return
	}
	if registry, ok := activeFamilyStatusRegistries.Load(component); ok {
		if typed, ok := registry.(*FamilyStatusRegistry); ok {
			typed.RecordFailure(family, err)
		}
	}
}
