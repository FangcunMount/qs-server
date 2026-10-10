package messagingruntime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/runtimefacts"
	"github.com/nsqio/go-nsq"

	rmtransport "github.com/FangcunMount/reliable-messaging/transport"
	rmnsq "github.com/FangcunMount/reliable-messaging/transport/nsq"
)

var (
	ErrNSQPublishUnknown  = errors.New("NSQ publish outcome unknown; retain original message identity")
	ErrNSQPublishRejected = errors.New("NSQ publish rejected before delivery")
)

// sdkNSQWirePublisher owns the SDK producer without exposing a legacy Message.
type sdkNSQWirePublisher struct {
	publisher *rmnsq.ManagedPublisher
	closeOnce sync.Once
	closed    chan struct{}
	closeErr  error
	facts     *runtimefacts.Owner
	factsID   string
}

// NewSDKNSQWirePublisher exposes complete wire bytes to the API's NSQ path.
func NewSDKNSQWirePublisher(address string) (*sdkNSQWirePublisher, error) {
	return NewSDKNSQWirePublisherWithFacts(address, nil, "", nil)
}

func NewSDKNSQWirePublisherWithFacts(address string, facts *runtimefacts.Owner, id string, httpEndpoints []string) (*sdkNSQWirePublisher, error) {
	driverConfig := nsq.NewConfig()
	if facts != nil {
		driverConfig.ClientID = facts.ClientID(id)
		if err := facts.Declare(runtimefacts.Transport{ID: id, Provider: "nsq", Direction: "publisher", NSQDTCPAddresses: []string{address}, NSQDHTTPAddresses: httpEndpoints, ClientID: driverConfig.ClientID, Hostname: driverConfig.Hostname}); err != nil {
			facts.MarkIncomplete(id)
		}
	}
	publisher, err := rmnsq.NewManagedPublisher(rmnsq.ManagedPublisherConfig{Address: address, Driver: driverConfig, MaxInFlight: 64})
	if err != nil {
		if facts != nil {
			facts.MarkIncomplete(id)
		}
		return nil, err
	}
	if facts != nil {
		if err := facts.MarkStarted(id); err != nil {
			facts.MarkIncomplete(id)
		}
	}
	return &sdkNSQWirePublisher{publisher: publisher, closed: make(chan struct{}), facts: facts, factsID: id}, nil
}

// PublishWire never rebuilds or mutates the caller's envelope. Unknown keeps
// the original application identity and bytes under the caller's recovery.
func (p *sdkNSQWirePublisher) PublishWire(ctx context.Context, topic string, body []byte) error {
	return classifyNSQPublish(topic, p.publisher.PublishRaw(ctx, topic, body))
}

func classifyNSQPublish(topic string, result rmtransport.Result) error {
	switch result.Outcome {
	case rmtransport.Confirmed:
		return nil
	case rmtransport.Rejected:
		return fmt.Errorf("%w: topic %s", ErrNSQPublishRejected, topic)
	default:
		return fmt.Errorf("%w: topic %s", ErrNSQPublishUnknown, topic)
	}
}

func (p *sdkNSQWirePublisher) Close() error {
	p.closeOnce.Do(func() {
		if p.facts != nil {
			p.facts.MarkStopped(p.factsID)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		p.closeErr = p.publisher.Close(ctx)
		cancel()
		if p.closeErr != nil {
			p.publisher.Interrupt()
			ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
			p.closeErr = errors.Join(p.closeErr, p.publisher.Close(ctx))
			cancel()
		}
		close(p.closed)
	})
	<-p.closed
	return p.closeErr
}
