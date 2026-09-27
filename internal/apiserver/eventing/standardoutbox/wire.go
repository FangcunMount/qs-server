package standardoutbox

import (
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
)

// EncodeWire preserves the complete NSQ transport envelope consumed by the
// current QS Worker. The SDK NSQ publisher sends Message.Payload verbatim, so
// its durable payload must be this wire value, not just the domain event JSON.
func EncodeWire(evt event.DomainEvent, source string) ([]byte, error) {
	payload, err := domainwire.EncodeEvent(evt)
	if err != nil {
		return nil, err
	}
	return legacy.Encode(legacy.Envelope{
		UUID:     evt.EventID(),
		Metadata: domainwire.MetadataFromEvent(evt, source),
		Payload:  payload,
	}, legacy.Revision2)
}
