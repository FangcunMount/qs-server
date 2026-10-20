package aibridge

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	store "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/aibridge"
	"github.com/FangcunMount/qs-server/internal/pkg/messagingruntime"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/reliable-messaging/transport"
	"github.com/FangcunMount/reliable-messaging/transport/rocketmq"
	"github.com/prometheus/client_golang/prometheus"
)

func (r *MessagingRuntime) startRocketMQ(ctx context.Context) (resultErr error) {
	defer func() {
		if resultErr != nil {
			stopCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			resultErr = errors.Join(resultErr, r.stop(stopCtx))
		}
	}()
	config, err := r.Options.RocketMQ.Load([]string{app.EventsTopic}, 1, 5*time.Second)
	if err != nil {
		return err
	}
	physical := transport.PhysicalTopic(app.EventsTopic)
	if r.Receiver.Transactions != nil {
		config.Routes, physical, err = transactionalEventRoutes(config.Routes)
		if err != nil {
			return err
		}
	}
	handlerCtx, cancel := context.WithCancel(context.Background())
	r.handlerCancel = cancel
	r.rocket, err = rocketmq.NewSubscriber(rocketmq.SubscriberConfig{Config: config, MaxAttempts: 8, InvisibleDuration: 60 * time.Second, HandlerTimeout: 30 * time.Second, RetryDelay: 30 * time.Second, DeliveryContext: handlerCtx})
	if err != nil {
		return err
	}
	if err = r.rocket.Subscribe(ctx, app.EventsTopic, "qs-server.ai-events.v1", func(ctx context.Context, d transport.Delivery) error {
		m := d.Message()
		if m.Topic != app.EventsTopic {
			return errors.New("unconfigured AI event route")
		}
		return r.Receiver.Receive(ctx, m.Wire)
	}, func(ctx context.Context, f transport.Failure) error {
		audit, err := json.Marshal(f)
		if err != nil {
			return err
		}
		if f.Provider != "rocketmq" || f.Message.Topic != app.EventsTopic || f.Message.PhysicalTopic != physical {
			return r.Receiver.Isolate(ctx, audit, "authentication_failed")
		}
		if _, err = app.AuthenticateMessaging(f.Message.Wire, app.EventsTopic, r.Receiver.Keys); err != nil {
			return r.Receiver.Isolate(ctx, audit, "authentication_failed")
		}
		return r.Receiver.FailedHandoff(ctx, f.Message.Wire, audit)
	}); err != nil {
		return err
	}
	r.handlersReady = true
	if r.Receiver.Transactions != nil {
		// The shared qs.mysql owner sends commands and ACKs transactionally.
		// New events never enter the historical Outbox/Relay scan.
		observations := newMessagingCollector(r.DB)
		if err = prometheus.DefaultRegisterer.Register(observations); err != nil {
			return errors.New("AI MQ observation registration unavailable")
		}
		r.observations = observations
		r.started = true
		return nil
	}
	r.publisher, err = messagingruntime.NewPublisher(&genericoptions.MessagingOptions{Provider: "rocketmq", RocketMQ: r.Options.RocketMQ}, map[string]string{app.CommandsTopic: app.CommandsTopic, app.AcksTopic: app.AcksTopic}, 1)
	if err != nil {
		return err
	}
	relay, err := store.NewMessagingRelay(r.DB, r.Store.Outbox, r.publisher)
	if err != nil {
		return err
	}
	observations := newMessagingCollector(r.DB)
	if err = prometheus.DefaultRegisterer.Register(observations); err != nil {
		return errors.New("AI MQ observation registration unavailable")
	}
	r.observations = observations
	relayCtx, relayCancel := context.WithCancel(ctx)
	r.relayCancel = relayCancel
	r.relayDone = make(chan struct{})
	go func() {
		defer close(r.relayDone)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			if r.Options.RocketMQ.PublishingEnabled() {
				if err := relay.Step(relayCtx); err != nil && relayCtx.Err() == nil {
					log.Print("AI MQ relay storage unavailable")
				}
			}
			select {
			case <-relayCtx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	r.started = true
	return nil
}

func transactionalEventRoutes(routes []transport.RouteBinding) ([]transport.RouteBinding, string, error) {
	physical, err := transport.PhysicalTopicV2(app.EventsTopic, transport.TransactionMessageType, "ai.mysql", "m3-1")
	if err != nil {
		return nil, "", err
	}
	if len(routes) != 1 {
		return nil, "", errors.New("one exact AI transaction event input required")
	}
	r := routes[0]
	if r.Topic != app.EventsTopic || r.PhysicalTopic != physical || r.MessageType != transport.TransactionMessageType || r.Owner != "ai.mysql" || r.RouteRevision != "m3-1" || r.Subscription != "qs-server.ai-events.v1" {
		return nil, "", errors.New("AI transaction event binding mismatch")
	}
	return append([]transport.RouteBinding(nil), routes...), physical, nil
}
