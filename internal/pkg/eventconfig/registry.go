package eventconfig

import (
	"sync"

	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
)

// TopicResolver resolves an event type to its physical topic name.
type TopicResolver = eventcatalog.TopicResolver

// Registry is the legacy mutable global access point for event configuration.
// New process code should prefer explicit eventcatalog.Catalog dependencies.
type Registry struct {
	mu      sync.RWMutex
	catalog *eventcatalog.Catalog
}

var (
	globalRegistry *Registry
	once           sync.Once
)

// Global returns the legacy global registry.
func Global() *Registry {
	once.Do(func() {
		globalRegistry = &Registry{catalog: eventcatalog.NewCatalog(nil)}
	})
	return globalRegistry
}

// Initialize loads the global registry.
func Initialize(configPath string) error {
	cfg, err := Load(configPath)
	if err != nil {
		return err
	}
	Global().SetConfig(cfg)
	return nil
}

// SetConfig replaces the registry catalog.
func (r *Registry) SetConfig(cfg *Config) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.catalog = eventcatalog.NewCatalog(cfg)
}

// Catalog returns the current immutable catalog snapshot.
func (r *Registry) Catalog() *eventcatalog.Catalog {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.catalog == nil {
		return eventcatalog.NewCatalog(nil)
	}
	return r.catalog
}

// Config returns the loaded config.
func (r *Registry) Config() *Config {
	return r.Catalog().Config()
}

// GetTopicForEvent returns the physical topic for an event type.
func (r *Registry) GetTopicForEvent(eventType string) (string, bool) {
	return r.Catalog().GetTopicForEvent(eventType)
}

// GetEventsForTopic returns event types bound to a physical topic.
func (r *Registry) GetEventsForTopic(topicName string) []string {
	return r.Catalog().GetEventsForTopic(topicName)
}

// GetTopicConfig returns a logical topic config.
func (r *Registry) GetTopicConfig(topicKey string) (TopicConfig, bool) {
	return r.Catalog().GetTopicConfig(topicKey)
}

// GetEventConfig returns an event config.
func (r *Registry) GetEventConfig(eventType string) (EventConfig, bool) {
	return r.Catalog().GetEventConfig(eventType)
}

// AllTopicNames returns all physical topic names.
func (r *Registry) AllTopicNames() []string {
	return r.Catalog().AllTopicNames()
}

// IsEventRegistered reports whether the event type exists in the catalog.
func (r *Registry) IsEventRegistered(eventType string) bool {
	return r.Catalog().IsEventRegistered(eventType)
}

// TopicSubscriptions returns all topic subscriptions with events.
func (r *Registry) TopicSubscriptions() []eventcatalog.TopicSubscription {
	return r.Catalog().TopicSubscriptions()
}
