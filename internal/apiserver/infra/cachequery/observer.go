package cachequery

// FamilyObserver is the narrow family-health port used by query cache runtime.
type FamilyObserver interface {
	ObserveFamilySuccess(family string)
	ObserveFamilyFailure(family string, err error)
}
