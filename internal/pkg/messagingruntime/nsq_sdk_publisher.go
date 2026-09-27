package messagingruntime

import (
	"context"
	"errors"

	basemessaging "github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
)

// sdkNSQPublisher is retained for callers that still use the old publisher
// interface. The API's native NSQ route uses NewSDKNSQWirePublisher instead.
type sdkNSQPublisher struct{ *sdkNSQWirePublisher }

func NewSDKNSQPublisher(address string) (basemessaging.Publisher, error) {
	wire, err := NewSDKNSQWirePublisher(address)
	if err != nil {
		return nil, err
	}
	return &sdkNSQPublisher{sdkNSQWirePublisher: wire}, nil
}

func (p *sdkNSQPublisher) Publish(ctx context.Context, topic string, body []byte) error {
	return p.PublishWire(ctx, topic, body)
}

func (p *sdkNSQPublisher) PublishMessage(ctx context.Context, topic string, msg *basemessaging.Message) error {
	if msg == nil {
		return errors.New("message is nil")
	}
	body, err := legacy.Encode(legacy.Envelope{UUID: msg.UUID, Metadata: msg.Metadata, Payload: msg.Payload}, legacy.Revision2)
	if err != nil {
		return err
	}
	return p.PublishWire(ctx, topic, body)
}
