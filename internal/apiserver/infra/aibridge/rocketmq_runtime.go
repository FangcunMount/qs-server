package aibridge

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
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
	var physical string
	config.Routes, physical, err = transactionalEventRoutes(config.Routes)
	if err != nil {
		return err
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
	observations := newMessagingCollector(r.DB)
	if err = prometheus.DefaultRegisterer.Register(observations); err != nil {
		return errors.New("AI MQ observation registration unavailable")
	}
	r.observations = observations
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
