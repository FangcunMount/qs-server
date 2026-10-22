package eventruntime

import (
	"errors"
	"sort"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
)

// NewBestEffortPublisher selects the existing best_effort contracts for the
// final NORMAL client. Sharing a logical topic never authorizes a durable
// event to bypass its transaction sender.
func NewBestEffortPublisher(catalog *eventcatalog.Catalog, wire WirePublisher) (*RoutingPublisher, error) {
	return NewBestEffortPublisherWithOptions(RoutingPublisherOptions{Catalog: catalog, WirePublisher: wire})
}

func NewBestEffortPublisherWithOptions(opts RoutingPublisherOptions) (*RoutingPublisher, error) {
	if opts.Catalog == nil || opts.WirePublisher == nil {
		return nil, errors.New("explicit best-effort catalog and NORMAL wire publisher required")
	}
	opts.TopicResolver = bestEffortRoutes{opts.Catalog}
	opts.Mode = PublishModeMQ
	return NewRoutingPublisher(opts), nil
}

// BestEffortTopics freezes the logical topic set from the actual host catalog.
// A shared logical topic still grants no permission to publish durable events.
func BestEffortTopics(catalog *eventcatalog.Catalog) ([]string, error) {
	if catalog == nil || catalog.Config() == nil {
		return nil, errors.New("explicit event catalog required")
	}
	wanted := map[string]bool{}
	for kind := range catalog.Config().Events {
		if delivery, ok := catalog.GetDeliveryClass(kind); !ok || delivery != eventcatalog.DeliveryClassBestEffort {
			continue
		}
		topic, ok := catalog.GetTopicForEvent(kind)
		if !ok || topic == "" {
			return nil, errors.New("best-effort event route absent")
		}
		wanted[topic] = true
	}
	topics := make([]string, 0, len(wanted))
	for topic := range wanted {
		topics = append(topics, topic)
	}
	sort.Strings(topics)
	if len(topics) == 0 {
		return nil, errors.New("explicit best-effort topics required")
	}
	return topics, nil
}

type bestEffortRoutes struct{ catalog *eventcatalog.Catalog }

func (r bestEffortRoutes) GetTopicForEvent(kind string) (string, bool) {
	delivery, ok := r.catalog.GetDeliveryClass(kind)
	if !ok || delivery != eventcatalog.DeliveryClassBestEffort {
		return "", false
	}
	return r.catalog.GetTopicForEvent(kind)
}
