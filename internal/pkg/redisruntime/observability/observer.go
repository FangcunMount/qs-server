package observability

// FamilyObserver exposes the narrow family-health reporting port used by cache
// infrastructure adapters.
type FamilyObserver interface {
	ObserveFamilySuccess(family string)
	ObserveFamilyFailure(family string, err error)
}

// ComponentObserver reports cache family health for one runtime component.
type ComponentObserver struct {
	component string
	registry  *FamilyStatusRegistry
}

func NewComponentObserver(component string, registry ...*FamilyStatusRegistry) *ComponentObserver {
	observer := &ComponentObserver{component: component}
	if len(registry) > 0 {
		observer.registry = registry[0]
	}
	return observer
}

func (o *ComponentObserver) Component() string {
	if o == nil {
		return ""
	}
	return o.component
}

func (o *ComponentObserver) ObserveFamilySuccess(family string) {
	if o == nil {
		return
	}
	if o.registry != nil {
		o.registry.RecordSuccess(family)
	}
}

func (o *ComponentObserver) ObserveFamilyFailure(family string, err error) {
	if o == nil {
		return
	}
	if o.registry != nil {
		o.registry.RecordFailure(family, err)
	}
}
