// Package eventcodec centralizes event JSON payload and messaging metadata rules.
package eventcodec

import (
	basecodec "github.com/FangcunMount/component-base/pkg/eventcodec"
	"github.com/FangcunMount/component-base/pkg/eventmessaging"
	"github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/qs-server/pkg/event"
)

const occurredAtLayout = basecodec.OccurredAtLayout

type Envelope = basecodec.Envelope

func EncodeDomainEvent(evt event.DomainEvent) ([]byte, error) {
	return basecodec.EncodeDomainEvent(evt)
}

func DecodeEnvelope(payload []byte) (*Envelope, error) {
	return basecodec.DecodeEnvelope(payload)
}

func DecodeDomainEvent(payload []byte) (event.DomainEvent, error) {
	return basecodec.DecodeDomainEvent(payload)
}

func MetadataFromEvent(evt event.DomainEvent, source string) map[string]string {
	return basecodec.MetadataFromEvent(evt, source)
}

func BuildMessage(evt event.DomainEvent, source string) (*messaging.Message, error) {
	return eventmessaging.BuildMessage(evt, source, basecodec.EncodeDomainEvent)
}
