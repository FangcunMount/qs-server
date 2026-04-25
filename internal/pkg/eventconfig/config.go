// Package eventconfig provides the legacy facade over the event catalog.
//
// New code should depend on internal/pkg/eventcatalog for contract queries and
// use eventconfig only for backward-compatible publisher/subscriber adapters.
package eventconfig

import "github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"

// Config is kept as a compatibility alias for eventcatalog.Config.
type Config = eventcatalog.Config

// TopicConfig is kept as a compatibility alias for eventcatalog.TopicConfig.
type TopicConfig = eventcatalog.TopicConfig

// EventConfig is kept as a compatibility alias for eventcatalog.EventConfig.
type EventConfig = eventcatalog.EventConfig

// Load loads an event catalog from disk.
func Load(path string) (*Config, error) {
	return eventcatalog.Load(path)
}

// Parse decodes an event catalog.
func Parse(data []byte) (*Config, error) {
	return eventcatalog.Parse(data)
}
