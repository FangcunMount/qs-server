package eventruntime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/FangcunMount/component-base/pkg/eventcodec"
	"github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/observe"
)

// ErrAutomaticRetryPaused marks a business retry event that must be durably
// held instead of consuming transport retry budget.
var ErrAutomaticRetryPaused = errors.New("automatic business retry paused by emergency switch")

// MessageEventExtractor resolves event type from metadata first and falls back
// to the canonical envelope.
type MessageEventExtractor struct{}

func (MessageEventExtractor) Extract(msg *messaging.Message) (string, error) {
	if msg.Metadata == nil {
		msg.Metadata = map[string]string{}
	}
	if eventType := msg.Metadata["event_type"]; eventType != "" {
		return eventType, nil
	}
	env, err := eventcodec.DecodeEnvelope(msg.Payload)
	if err != nil {
		return "", err
	}
	if env.EventType == "" {
		return "", fmt.Errorf("event envelope has no event_type")
	}
	msg.Metadata["event_type"] = env.EventType
	return env.EventType, nil
}

// MessageSettlementPolicy is the shared poison/unknown/handler-failure
// transport mapping used by worker and apiserver projection consumers.
type MessageSettlementPolicy struct {
	logger   *slog.Logger
	service  string
	topic    string
	observer eventobservability.Observer
}

func NewMessageSettlementPolicy(logger *slog.Logger, service, topic string, observer eventobservability.Observer) MessageSettlementPolicy {
	if logger == nil {
		logger = slog.Default()
	}
	if observer == nil {
		observer = eventobservability.DefaultObserver()
	}
	return MessageSettlementPolicy{logger: logger, service: service, topic: topic, observer: observer}
}

func (p MessageSettlementPolicy) NackInvalid(msg *messaging.Message, parseErr error) (eventobservability.ConsumeOutcome, error) {
	p.logger.Warn("message missing event_type and payload parse failed",
		slog.String("channel", p.service), slog.String("topic", p.topic), slog.String("msg_id", msg.UUID),
		slog.Int("payload_bytes", len(msg.Payload)), slog.String("error", parseErr.Error()))
	if nackErr := msg.Nack(); nackErr != nil {
		p.observe(msg, "", eventobservability.ConsumeOutcomeDecodeNackFailed)
		return eventobservability.ConsumeOutcomeDecodeNackFailed, nackErr
	}
	p.observe(msg, "", eventobservability.ConsumeOutcomeDecodeNacked)
	return eventobservability.ConsumeOutcomeDecodeNacked, parseErr
}

func (p MessageSettlementPolicy) AckHeld(msg *messaging.Message) (eventobservability.ConsumeOutcome, error) {
	return p.ack(msg, eventobservability.ConsumeOutcomeHeld, eventobservability.ConsumeOutcomeHoldFailed)
}

func (p MessageSettlementPolicy) NackHoldFailed(msg *messaging.Message, eventType string, holdErr error) eventobservability.ConsumeOutcome {
	p.logger.Error("failed to persist paused retry event hold",
		slog.String("channel", p.service), slog.String("topic", p.topic), slog.String("event_type", eventType),
		slog.String("msg_id", msg.UUID), slog.String("error", holdErr.Error()))
	_ = msg.Nack()
	p.observe(msg, eventType, eventobservability.ConsumeOutcomeHoldFailed)
	return eventobservability.ConsumeOutcomeHoldFailed
}

func (p MessageSettlementPolicy) NackFailed(msg *messaging.Message, eventType string, dispatchErr error) eventobservability.ConsumeOutcome {
	p.logger.Error("failed to dispatch event", slog.String("channel", p.service), slog.String("topic", p.topic), slog.String("event_type", eventType), slog.String("msg_id", msg.UUID), slog.String("error", dispatchErr.Error()))
	if nackErr := msg.Nack(); nackErr != nil {
		outcome := eventobservability.ConsumeOutcomeNackFailed
		p.observe(msg, eventType, outcome)
		p.logger.Warn("failed to nack message", slog.String("channel", p.service), slog.String("topic", p.topic), slog.String("msg_id", msg.UUID), slog.String("error", nackErr.Error()))
		return outcome
	}
	outcome := eventobservability.ConsumeOutcomeNacked
	p.observe(msg, eventType, outcome)
	return outcome
}

func (p MessageSettlementPolicy) AckSuccess(msg *messaging.Message) (eventobservability.ConsumeOutcome, error) {
	return p.ack(msg, eventobservability.ConsumeOutcomeAcked, eventobservability.ConsumeOutcomeAckFailed)
}

func (p MessageSettlementPolicy) AckUnknown(msg *messaging.Message) (eventobservability.ConsumeOutcome, error) {
	return p.ack(msg, eventobservability.ConsumeOutcomeUnknownAcked, eventobservability.ConsumeOutcomeUnknownAckFailed)
}

func (p MessageSettlementPolicy) ack(msg *messaging.Message, successOutcome, failedOutcome eventobservability.ConsumeOutcome) (eventobservability.ConsumeOutcome, error) {
	if ackErr := msg.Ack(); ackErr != nil {
		eventType := eventTypeFromMessage(msg)
		p.observe(msg, eventType, failedOutcome)
		p.logger.Warn("failed to ack message", slog.String("channel", p.service), slog.String("topic", p.topic), slog.String("msg_id", msg.UUID), slog.String("error", ackErr.Error()))
		return failedOutcome, ackErr
	}
	eventType := eventTypeFromMessage(msg)
	p.observe(msg, eventType, successOutcome)
	return successOutcome, nil
}

func (p MessageSettlementPolicy) observe(msg *messaging.Message, eventType string, outcome eventobservability.ConsumeOutcome) {
	if p.observer == nil {
		return
	}
	p.observer.ObserveConsume(context.Background(), eventobservability.ConsumeEvent{Service: p.service, Topic: p.topic, EventType: eventType, Outcome: outcome})
}

func eventTypeFromMessage(msg *messaging.Message) string {
	if msg != nil && msg.Metadata != nil {
		return msg.Metadata["event_type"]
	}
	return ""
}
