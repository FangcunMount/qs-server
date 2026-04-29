// Package eventcatalog owns qs-server event names and aliases the generic
// event catalog runtime from component-base.
package eventcatalog

import base "github.com/FangcunMount/component-base/pkg/eventcatalog"

type Config = base.Config
type TopicConfig = base.TopicConfig
type DeliveryClass = base.DeliveryClass
type EventConfig = base.EventConfig
type ValidateOptions = base.ValidateOptions

const (
	DeliveryClassBestEffort    = base.DeliveryClassBestEffort
	DeliveryClassDurableOutbox = base.DeliveryClassDurableOutbox
)

var StrictValidateOptions = base.StrictValidateOptions

func Load(path string) (*Config, error) {
	return base.Load(path)
}

func LoadWithOptions(path string, opts ValidateOptions) (*Config, error) {
	return base.LoadWithOptions(path, opts)
}

func Parse(data []byte) (*Config, error) {
	return base.Parse(data)
}

func ParseWithOptions(data []byte, opts ValidateOptions) (*Config, error) {
	return base.ParseWithOptions(data, opts)
}
