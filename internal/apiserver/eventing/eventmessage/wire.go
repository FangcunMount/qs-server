package eventmessage

import (
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
)

// EncodeWire preserves the Revision2 envelope used by frozen transaction plans
// and historical references. The transport provider does not change its bytes.
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
