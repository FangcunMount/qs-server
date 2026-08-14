package messagingruntime

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	basemessaging "github.com/FangcunMount/component-base/pkg/messaging"
	"github.com/nsqio/go-nsq"
)

type reconnectPublisherStub struct {
	publishErrors        []error
	publishMessageErrors []error
	publishCalls         int
	publishMessageCalls  int
	closeCalls           int
}

func (s *reconnectPublisherStub) Publish(context.Context, string, []byte) error {
	err := errorAt(s.publishErrors, s.publishCalls)
	s.publishCalls++
	return err
}

func (s *reconnectPublisherStub) PublishMessage(context.Context, string, *basemessaging.Message) error {
	err := errorAt(s.publishMessageErrors, s.publishMessageCalls)
	s.publishMessageCalls++
	return err
}

func (s *reconnectPublisherStub) Close() error {
	s.closeCalls++
	return nil
}

func errorAt(values []error, index int) error {
	if index >= len(values) {
		return nil
	}
	return values[index]
}

func noWait(context.Context, time.Duration) error { return nil }

func TestNSQReconnectPublisherRetriesOnlyNotConnected(t *testing.T) {
	delegate := &reconnectPublisherStub{publishErrors: []error{
		fmt.Errorf("publish failed: %w", nsq.ErrNotConnected),
		nsq.ErrNotConnected,
		nil,
	}}
	publisher := newNSQReconnectPublisher(delegate, []time.Duration{time.Millisecond, time.Millisecond}, noWait)

	if err := publisher.Publish(context.Background(), "topic", []byte("payload")); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if delegate.publishCalls != 3 {
		t.Fatalf("Publish() calls = %d, want 3", delegate.publishCalls)
	}
}

func TestNSQReconnectPublisherDoesNotRetryOtherFailures(t *testing.T) {
	wantErr := errors.New("protocol failure")
	delegate := &reconnectPublisherStub{publishErrors: []error{wantErr}}
	publisher := newNSQReconnectPublisher(delegate, []time.Duration{time.Millisecond}, noWait)

	err := publisher.Publish(context.Background(), "topic", []byte("payload"))
	if !errors.Is(err, wantErr) {
		t.Fatalf("Publish() error = %v, want %v", err, wantErr)
	}
	if delegate.publishCalls != 1 {
		t.Fatalf("Publish() calls = %d, want 1", delegate.publishCalls)
	}
}

func TestNSQReconnectPublisherHonorsContextDuringReconnect(t *testing.T) {
	delegate := &reconnectPublisherStub{publishErrors: []error{nsq.ErrNotConnected}}
	publisher := newNSQReconnectPublisher(delegate, []time.Duration{time.Second}, func(context.Context, time.Duration) error {
		return context.Canceled
	})

	err := publisher.Publish(context.Background(), "topic", []byte("payload"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Publish() error = %v, want context canceled", err)
	}
	if delegate.publishCalls != 1 {
		t.Fatalf("Publish() calls = %d, want 1", delegate.publishCalls)
	}
}

func TestNSQReconnectPublisherCoversMessagesAndDelegatesClose(t *testing.T) {
	delegate := &reconnectPublisherStub{publishMessageErrors: []error{nsq.ErrNotConnected, nil}}
	publisher := newNSQReconnectPublisher(delegate, []time.Duration{time.Millisecond}, noWait)

	if err := publisher.PublishMessage(context.Background(), "topic", &basemessaging.Message{UUID: "event-1"}); err != nil {
		t.Fatalf("PublishMessage() error = %v", err)
	}
	if delegate.publishMessageCalls != 2 {
		t.Fatalf("PublishMessage() calls = %d, want 2", delegate.publishMessageCalls)
	}
	if err := publisher.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if delegate.closeCalls != 1 {
		t.Fatalf("Close() calls = %d, want 1", delegate.closeCalls)
	}
}
