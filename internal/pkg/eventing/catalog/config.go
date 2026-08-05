// Package eventcatalog owns qs-server event names and aliases the generic
// event catalog runtime from component-base.
package eventcatalog

import base "github.com/FangcunMount/component-base/pkg/eventcatalog"

type Config = base.Config
type TopicConfig = base.TopicConfig
type DeliveryClass = base.DeliveryClass
type EventConfig = base.EventConfig

const (
	DeliveryClassBestEffort    = base.DeliveryClassBestEffort
	DeliveryClassDurableOutbox = base.DeliveryClassDurableOutbox
)

func Load(path string) (*Config, error) {
	return base.Load(path)
}

func Parse(data []byte) (*Config, error) {
	return base.Parse(data)
}
