package eventcatalog

import base "github.com/FangcunMount/reliable-messaging/catalog"

type Catalog = base.Catalog
type TopicResolver = base.TopicResolver
type DeliveryClassResolver = base.DeliveryClassResolver
type TopicSubscription = base.TopicSubscription

func NewCatalog(cfg *Config) *Catalog {
	return base.NewCatalog(cfg)
}
