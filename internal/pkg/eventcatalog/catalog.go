package eventcatalog

// Catalog is an immutable query view over the event contract.
type Catalog struct {
	config        *Config
	eventToTopic  map[string]string
	topicToEvents map[string][]string
}

// NewCatalog builds a query catalog from a validated config.
func NewCatalog(cfg *Config) *Catalog {
	c := &Catalog{
		config:        cfg,
		eventToTopic:  make(map[string]string),
		topicToEvents: make(map[string][]string),
	}
	if cfg == nil {
		return c
	}
	for eventType, eventCfg := range cfg.Events {
		if topicCfg, ok := cfg.Topics[eventCfg.Topic]; ok {
			topicName := topicCfg.Name
			c.eventToTopic[eventType] = topicName
			c.topicToEvents[topicName] = append(c.topicToEvents[topicName], eventType)
		}
	}
	return c
}

// TopicSubscription describes one topic subscription derived from the catalog.
type TopicSubscription struct {
	TopicName  string
	TopicKey   string
	EventTypes []string
}

// Config returns the catalog config.
func (c *Catalog) Config() *Config {
	if c == nil {
		return nil
	}
	return c.config
}

// GetTopicForEvent returns the physical topic name for an event type.
func (c *Catalog) GetTopicForEvent(eventType string) (string, bool) {
	if c == nil {
		return "", false
	}
	topic, ok := c.eventToTopic[eventType]
	return topic, ok
}

// GetEventsForTopic returns event types bound to a physical topic name.
func (c *Catalog) GetEventsForTopic(topicName string) []string {
	if c == nil {
		return nil
	}
	return append([]string(nil), c.topicToEvents[topicName]...)
}

// GetTopicConfig returns a logical topic config.
func (c *Catalog) GetTopicConfig(topicKey string) (TopicConfig, bool) {
	if c == nil || c.config == nil {
		return TopicConfig{}, false
	}
	cfg, ok := c.config.Topics[topicKey]
	return cfg, ok
}

// GetEventConfig returns an event config.
func (c *Catalog) GetEventConfig(eventType string) (EventConfig, bool) {
	if c == nil || c.config == nil {
		return EventConfig{}, false
	}
	cfg, ok := c.config.Events[eventType]
	return cfg, ok
}

// AllTopicNames returns all physical topic names that have events.
func (c *Catalog) AllTopicNames() []string {
	if c == nil {
		return nil
	}
	names := make([]string, 0, len(c.topicToEvents))
	for name := range c.topicToEvents {
		names = append(names, name)
	}
	return names
}

// IsEventRegistered reports whether the event type exists in the catalog.
func (c *Catalog) IsEventRegistered(eventType string) bool {
	if c == nil {
		return false
	}
	_, ok := c.eventToTopic[eventType]
	return ok
}

// TopicSubscriptions returns all topic subscriptions with at least one event.
func (c *Catalog) TopicSubscriptions() []TopicSubscription {
	if c == nil || c.config == nil {
		return nil
	}
	subs := make([]TopicSubscription, 0, len(c.config.Topics))
	for topicKey, topicCfg := range c.config.Topics {
		events := c.config.GetEventsByTopic(topicKey)
		if len(events) == 0 {
			continue
		}
		subs = append(subs, TopicSubscription{
			TopicName:  topicCfg.Name,
			TopicKey:   topicKey,
			EventTypes: events,
		})
	}
	return subs
}
