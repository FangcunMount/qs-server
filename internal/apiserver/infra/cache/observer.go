package cache

import "github.com/FangcunMount/qs-server/internal/pkg/cacheobservability"

// Observer is kept as the object-cache compatibility name for the shared
// component-level family observer.
type Observer = cacheobservability.ComponentObserver

func NewObserver(component string) *Observer {
	return cacheobservability.NewComponentObserver(component)
}
