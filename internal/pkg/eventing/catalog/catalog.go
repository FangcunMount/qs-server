package eventcatalog

import base "github.com/FangcunMount/component-base/pkg/eventcatalog"

type Catalog = base.Catalog
type TopicResolver = base.TopicResolver
type DeliveryClassResolver = base.DeliveryClassResolver
type TopicSubscription = base.TopicSubscription

func NewCatalog(cfg *Config) *Catalog {
	return base.NewCatalog(cfg)
}
