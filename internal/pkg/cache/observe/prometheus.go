package observe

import (
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	legacy "github.com/FangcunMount/qs-server/internal/pkg/cachegovernance/observability"
)

// FamilyObserver is the runtime-health seam used while cache metrics and Redis
// family status are split into their terminal packages.
type FamilyObserver interface {
	ObserveFamilySuccess(family string)
	ObserveFamilyFailure(family string, err error)
}

// Prometheus preserves the existing metric names and label values for one
// family/capability pair.
type Prometheus struct {
	family string
	policy string
	health FamilyObserver
}

func NewPrometheus(family, policy string, health FamilyObserver) *Prometheus {
	return &Prometheus{family: family, policy: policy, health: health}
}

func (o *Prometheus) Observe(event sharedcache.Event) {
	if o == nil {
		return
	}
	switch event.Operation {
	case sharedcache.OperationGet:
		legacy.ObserveCacheGet(o.family, o.policy, string(event.Result))
		legacy.ObserveCacheOperationDuration(o.family, o.policy, "get", event.Duration)
	case sharedcache.OperationSourceLoad:
		legacy.ObserveCacheOperationDuration(o.family, o.policy, "source_load", event.Duration)
	case sharedcache.OperationSet:
		legacy.ObserveCacheWrite(o.family, o.policy, "set", string(event.Result))
		legacy.ObserveCacheOperationDuration(o.family, o.policy, "set", event.Duration)
	case sharedcache.OperationInvalidate:
		legacy.ObserveCacheWrite(o.family, o.policy, "invalidate", string(event.Result))
	case sharedcache.OperationPayloadRaw:
		legacy.ObserveCachePayloadBytes(o.family, o.policy, "raw", event.Size)
	case sharedcache.OperationPayloadSet:
		legacy.ObserveCachePayloadBytes(o.family, o.policy, "stored", event.Size)
	}
	if event.Err != nil {
		if o.health != nil {
			o.health.ObserveFamilyFailure(o.family, event.Err)
		}
		return
	}
	if o.health != nil && (event.Operation == sharedcache.OperationGet || event.Operation == sharedcache.OperationSet) {
		o.health.ObserveFamilySuccess(o.family)
	}
}
