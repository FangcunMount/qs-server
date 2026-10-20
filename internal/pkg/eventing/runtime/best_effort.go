package eventruntime

import (
	"errors"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
)

// NewBestEffortPublisher selects the existing best_effort contracts for the
// final NORMAL client. Sharing a logical topic never authorizes a durable
// event to bypass its transaction sender.
func NewBestEffortPublisher(catalog *eventcatalog.Catalog, wire WirePublisher) (*RoutingPublisher, error) {
	if catalog == nil || wire == nil {
		return nil, errors.New("explicit best-effort catalog and NORMAL wire publisher required")
	}
	return NewRoutingPublisher(RoutingPublisherOptions{TopicResolver: bestEffortRoutes{catalog}, WirePublisher: wire, Mode: PublishModeMQ}), nil
}

type bestEffortRoutes struct{ catalog *eventcatalog.Catalog }

func (r bestEffortRoutes) GetTopicForEvent(kind string) (string, bool) {
	delivery, ok := r.catalog.GetDeliveryClass(kind)
	if !ok || delivery != eventcatalog.DeliveryClassBestEffort {
		return "", false
	}
	return r.catalog.GetTopicForEvent(kind)
}
