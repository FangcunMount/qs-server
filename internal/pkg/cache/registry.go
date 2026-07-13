package cache

import "sync"

type Layer string

const (
	LayerL1 Layer = "L1"
	LayerL2 Layer = "L2"
)

// EffectiveCapability is the process-resolved cache capability contract.
type EffectiveCapability struct {
	Capability Capability `json:"capability"`
	Layer      Layer      `json:"layer"`
	Family     string     `json:"family"`
	Policy     Policy     `json:"policy"`
	Source     string     `json:"source"`
	Version    string     `json:"version"`
}

type Registry struct {
	mu      sync.RWMutex
	entries []EffectiveCapability
}

func NewRegistry(entries ...EffectiveCapability) *Registry {
	r := &Registry{}
	r.Replace(entries)
	return r
}

func (r *Registry) Replace(entries []EffectiveCapability) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.entries = append([]EffectiveCapability(nil), entries...)
	r.mu.Unlock()
}

func (r *Registry) Snapshot() []EffectiveCapability {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]EffectiveCapability(nil), r.entries...)
}
