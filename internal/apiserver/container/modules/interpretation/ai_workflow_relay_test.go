package interpretation

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	bridge "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
)

type relayStore struct {
	bridge.Store
	entered chan struct{}
	calls   atomic.Int32
}

func (s *relayStore) Pending(ctx context.Context, _ int) ([]bridge.Command, error) {
	s.calls.Add(1)
	close(s.entered)
	<-ctx.Done()
	return nil, ctx.Err()
}

type unusedSender struct{}

func (unusedSender) Send(context.Context, bridge.Command) (bridge.Receipt, error) {
	panic("no commands")
}

type relayCloser struct {
	done   <-chan struct{}
	closed bool
}

func (c *relayCloser) Close() error {
	select {
	case <-c.done:
		c.closed = true
	default:
		panic("connection closed before relay stopped")
	}
	return nil
}

func TestAIWorkflowRelayRequiresConfiguredTransportOnlyWhenEnabled(t *testing.T) {
	m := &Module{}
	if err := m.StartAIWorkflowRelay(t.Context()); err != nil || m.aiRelayDone != nil {
		t.Fatal("disabled relay started")
	}
	m.aiWorkflowEnabled = true
	if err := m.StartAIWorkflowRelay(t.Context()); err == nil {
		t.Fatal("missing relay accepted")
	}
}

func TestAIWorkflowRelayStartsOnceAndJoinsBeforeClosingConnection(t *testing.T) {
	s := &relayStore{entered: make(chan struct{})}
	m := &Module{aiWorkflowEnabled: true, aiBridge: &bridge.Service{Store: s, Sender: unusedSender{}}}
	if err := m.StartAIWorkflowRelay(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Cleanup() })
	select {
	case <-s.entered:
	case <-time.After(time.Second):
		t.Fatal("relay did not start")
	}
	if err := m.StartAIWorkflowRelay(t.Context()); err != nil {
		t.Fatal(err)
	}
	closer := &relayCloser{done: m.aiRelayDone}
	m.aiManagementConnection = closer
	if err := m.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if !closer.closed || s.calls.Load() != 1 {
		t.Fatal("duplicate relay or unclosed connection")
	}
	// Closing intake does not disable receipt of already accepted AI results.
	m.aiWorkflowEnabled = false
	deps := m.ExportGRPCDeps()
	if deps.AIWorkflowResults != m.aiBridge {
		t.Fatal("result receiver was tied to intake")
	}
}
