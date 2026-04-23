package cache

import "github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"

// Observer 收口 infra/cache 对组件级 family observability 的访问。
type Observer struct {
	component string
}

func NewObserver(component string) *Observer {
	return &Observer{component: component}
}

func (o *Observer) Component() string {
	if o == nil {
		return ""
	}
	return o.component
}

func (o *Observer) ObserveFamilySuccess(family string) {
	if o == nil {
		return
	}
	cacheobservability.ObserveFamilySuccess(o.component, family)
}

func (o *Observer) ObserveFamilyFailure(family string, err error) {
	if o == nil {
		return
	}
	cacheobservability.ObserveFamilyFailure(o.component, family, err)
}
