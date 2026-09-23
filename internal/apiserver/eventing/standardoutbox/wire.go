package standardoutbox

import (
	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/component-base/pkg/eventmessaging"
	"github.com/FangcunMount/component-base/pkg/messaging"
)

// EncodeWire preserves the complete NSQ transport envelope consumed by the
// current QS Worker. The SDK NSQ publisher sends Message.Payload verbatim, so
// its durable payload must be this wire value, not just the domain event JSON.
func EncodeWire(evt event.DomainEvent, source string) ([]byte, error) {
	msg, err := eventmessaging.BuildMessage(evt, source)
	if err != nil {
		return nil, err
	}
	return messaging.EncodeMessagePayload(msg)
}
