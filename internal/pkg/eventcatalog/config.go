// Package eventcatalog owns the event contract model loaded from configs/events.yaml.
package eventcatalog

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the root event contract loaded from configs/events.yaml.
type Config struct {
	Version string                 `yaml:"version"`
	Topics  map[string]TopicConfig `yaml:"topics"`
	Events  map[string]EventConfig `yaml:"events"`
}

// TopicConfig describes a logical event topic.
type TopicConfig struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// DeliveryClass describes the durability contract for one event type.
type DeliveryClass string

const (
	DeliveryClassBestEffort    DeliveryClass = "best_effort"
	DeliveryClassDurableOutbox DeliveryClass = "durable_outbox"
)

// Valid reports whether the delivery class is part of the supported contract.
func (c DeliveryClass) Valid() bool {
	switch c {
	case DeliveryClassBestEffort, DeliveryClassDurableOutbox:
		return true
	default:
		return false
	}
}

func (c DeliveryClass) String() string {
	return string(c)
}

// EventConfig describes one event type and its runtime routing contract.
type EventConfig struct {
	Topic       string        `yaml:"topic"`
	Delivery    DeliveryClass `yaml:"delivery"`
	Aggregate   string        `yaml:"aggregate"`
	Domain      string        `yaml:"domain"`
	Description string        `yaml:"description"`
	Handler     string        `yaml:"handler"`
}

// Load reads and validates an event catalog from disk.
func Load(path string) (*Config, error) {
	// #nosec G304 -- config path is provided by trusted service startup options.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	return Parse(data)
}

// Parse decodes and validates an event catalog.
func Parse(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}
	return &cfg, nil
}

// Validate verifies topic and handler references.
func (c *Config) Validate() error {
	referencedTopics := make(map[string]struct{}, len(c.Topics))

	for eventType, eventCfg := range c.Events {
		if _, ok := c.Topics[eventCfg.Topic]; !ok {
			return fmt.Errorf("event %q references unknown topic %q", eventType, eventCfg.Topic)
		}
		if eventCfg.Handler == "" {
			return fmt.Errorf("event %q has empty handler", eventType)
		}
		if eventCfg.Delivery == "" {
			return fmt.Errorf("event %q has empty delivery", eventType)
		}
		if !eventCfg.Delivery.Valid() {
			return fmt.Errorf("event %q has invalid delivery %q", eventType, eventCfg.Delivery)
		}
		referencedTopics[eventCfg.Topic] = struct{}{}
	}

	for topicKey := range c.Topics {
		if _, ok := referencedTopics[topicKey]; !ok {
			return fmt.Errorf("topic %q has no events", topicKey)
		}
	}
	return nil
}

// GetTopicName returns the physical topic name for an event type.
func (c *Config) GetTopicName(eventType string) (string, bool) {
	eventCfg, ok := c.Events[eventType]
	if !ok {
		return "", false
	}
	topicCfg, ok := c.Topics[eventCfg.Topic]
	if !ok {
		return "", false
	}
	return topicCfg.Name, true
}

// GetEventsByTopic returns event types bound to the logical topic key.
func (c *Config) GetEventsByTopic(topicKey string) []string {
	var events []string
	for eventType, eventCfg := range c.Events {
		if eventCfg.Topic == topicKey {
			events = append(events, eventType)
		}
	}
	return events
}

// GetTopicKeys returns all logical topic keys.
func (c *Config) GetTopicKeys() []string {
	keys := make([]string, 0, len(c.Topics))
	for k := range c.Topics {
		keys = append(keys, k)
	}
	return keys
}

// GetHandlerName returns the handler configured for an event type.
func (c *Config) GetHandlerName(eventType string) (string, bool) {
	eventCfg, ok := c.Events[eventType]
	if !ok {
		return "", false
	}
	return eventCfg.Handler, eventCfg.Handler != ""
}

// GetDeliveryClass returns the configured delivery class for an event type.
func (c *Config) GetDeliveryClass(eventType string) (DeliveryClass, bool) {
	eventCfg, ok := c.Events[eventType]
	if !ok || eventCfg.Delivery == "" {
		return "", false
	}
	return eventCfg.Delivery, true
}

// ListEventTypes returns all event types present in the catalog.
func (c *Config) ListEventTypes() []string {
	types := make([]string, 0, len(c.Events))
	for t := range c.Events {
		types = append(types, t)
	}
	return types
}
