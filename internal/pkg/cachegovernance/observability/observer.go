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
}

func NewComponentObserver(component string) *ComponentObserver {
	return &ComponentObserver{component: component}
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
	ObserveFamilySuccess(o.component, family)
}

func (o *ComponentObserver) ObserveFamilyFailure(family string, err error) {
	if o == nil {
		return
	}
	ObserveFamilyFailure(o.component, family, err)
}
