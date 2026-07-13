package observe

import "time"

type QueryVersion struct {
	kind   string
	family string
	health FamilyObserver
}

func NewQueryVersion(kind, family string, health FamilyObserver) *QueryVersion {
	return &QueryVersion{kind: kind, family: family, health: health}
}

func (o *QueryVersion) ObserveVersion(operation, result string, duration time.Duration) {
	if o != nil {
		ObserveQueryCacheVersion(o.kind, operation, result, duration)
	}
}

func (o *QueryVersion) ObserveSuccess() {
	if o != nil && o.health != nil {
		o.health.ObserveFamilySuccess(o.family)
	}
}

func (o *QueryVersion) ObserveFailure(err error) {
	if o != nil && o.health != nil && err != nil {
		o.health.ObserveFamilyFailure(o.family, err)
	}
}
