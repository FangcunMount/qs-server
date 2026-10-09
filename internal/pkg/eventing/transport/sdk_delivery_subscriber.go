package transport

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/FangcunMount/qs-server/internal/pkg/runtimefacts"
	rmtransport "github.com/FangcunMount/reliable-messaging/transport"
	rmnsq "github.com/FangcunMount/reliable-messaging/transport/nsq"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
)

// SDKDeliverySubscriber keeps QS process lifecycle ownership while passing
// native SDK deliveries to Worker handlers. The SDK owns NSQ retry, receipt
// settlement and terminal handoff; QS owns durable failure audit.
type SDKDeliverySubscriber struct {
	inner       sdkDeliverySource
	failed      func(context.Context, legacy.FailedHandoff) error
	stopOnce    sync.Once
	stopDone    chan struct{}
	stopErr     error
	facts       *runtimefacts.Owner
	factsID     string
	failedGroup string
}

type sdkDeliverySource interface {
	Subscribe(context.Context, string, string, rmtransport.Handler, func(context.Context, legacy.FailedHandoff) error) error
	Close(context.Context) error
}

func NewSDKDeliverySubscriber(
	config SubscriberConfig,
	maxInFlight, maxAttempts int,
	failed func(context.Context, legacy.FailedHandoff) error,
) (*SDKDeliverySubscriber, error) {
	return NewSDKDeliverySubscriberWithFacts(config, maxInFlight, maxAttempts, failed, nil, "")
}

// NewSDKDeliverySubscriberWithFacts observes the exact driver configuration.
// Observation errors do not replace the subscriber's original errors or work.
func NewSDKDeliverySubscriberWithFacts(
	config SubscriberConfig, maxInFlight, maxAttempts int,
	failed func(context.Context, legacy.FailedHandoff) error,
	facts *runtimefacts.Owner, id string,
) (result *SDKDeliverySubscriber, resultErr error) {
	defer func() {
		if resultErr != nil && facts != nil {
			facts.MarkIncomplete(id)
		}
	}()
	if config.Provider != "nsq" || config.NSQLookupdAddr == "" {
		return nil, errors.New("NSQ lookupd is required for SDK delivery subscriber")
	}
	if maxAttempts < 1 || maxAttempts > retrygovernance.HardMaxDeliveryAttempts || failed == nil {
		return nil, fmt.Errorf("bounded attempts and durable failed-message handler are required")
	}
	driverConfig, err := newNSQConfig(config.NSQMessageTimeout)
	if err != nil {
		return nil, err
	}
	if facts != nil {
		driverConfig.ClientID = facts.ClientID(id)
		if err := facts.Declare(runtimefacts.Transport{
			ID: id, Provider: "nsq", Direction: "consumer",
			LookupdAddresses:  []string{config.NSQLookupdAddr},
			NSQDHTTPAddresses: config.NSQDHTTPEndpoints,
			ClientID:          driverConfig.ClientID, Hostname: driverConfig.Hostname,
		}); err != nil {
			facts.MarkIncomplete(id)
		}
	}
	inner, err := rmnsq.NewSubscriber(rmnsq.SubscriberConfig{
		LookupdAddresses: []string{config.NSQLookupdAddr}, Driver: driverConfig,
		MaxInFlight: maxInFlight, MaxAttempts: uint16(maxAttempts),
		FailedHandoffGroup: config.FailedHandoffGroup,
		Retry: rmnsq.Backoff{
			BaseDelay: DefaultRetryBaseDelay, MaxDelay: DefaultRetryMaxDelay,
			JitterFraction: DefaultRetryJitter,
		},
	})
	if err != nil {
		if facts != nil {
			facts.MarkIncomplete(id)
		}
		return nil, err
	}
	return &SDKDeliverySubscriber{inner: inner, failed: failed, stopDone: make(chan struct{}), facts: facts, factsID: id, failedGroup: config.FailedHandoffGroup}, nil
}

func (s *SDKDeliverySubscriber) Subscribe(topic, channel string, handler rmtransport.Handler) error {
	if s == nil || s.inner == nil || handler == nil {
		if s != nil && s.facts != nil {
			s.facts.MarkIncomplete(s.factsID)
		}
		return errors.New("SDK delivery subscriber and handler are required")
	}
	err := s.inner.Subscribe(context.Background(), topic, channel, handler, s.failed)
	if s.facts != nil {
		if err != nil {
			s.facts.MarkIncomplete(s.factsID)
		} else {
			failureTopic := legacy.FailedHandoffTopic(topic, channel)
			if s.failedGroup != "" {
				failureTopic = legacy.FailedHandoffTopicForGroup(topic, s.failedGroup)
			}
			if e := s.facts.MarkSubscribed(s.factsID, runtimefacts.Subscription{Topic: topic, Channel: channel, FailureTopic: failureTopic, FailureChannel: legacy.FailedHandoffChannel}); e != nil {
				s.facts.MarkIncomplete(s.factsID)
			} else if e := s.facts.MarkStarted(s.factsID); e != nil {
				s.facts.MarkIncomplete(s.factsID)
			}
		}
	}
	return err
}

func (s *SDKDeliverySubscriber) Stop() {
	if s == nil || s.inner == nil {
		return
	}
	s.stopOnce.Do(func() {
		if s.facts != nil {
			s.facts.MarkStopped(s.factsID)
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
			defer cancel()
			s.stopErr = s.inner.Close(ctx)
			close(s.stopDone)
		}()
	})
}

func (s *SDKDeliverySubscriber) Close() error {
	if s == nil || s.inner == nil {
		return nil
	}
	s.Stop()
	<-s.stopDone
	if s.stopErr == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	return s.inner.Close(ctx)
}
