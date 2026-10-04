package interpretation

import (
	"context"
	"errors"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"testing"
	"time"
)

type messagingLifecycleProbe struct {
	started, stopped int
	stopError        error
}

func (p *messagingLifecycleProbe) Start(context.Context) error { p.started++; return nil }
func (p *messagingLifecycleProbe) Stop(ctx context.Context) error {
	p.stopped++
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 21*time.Second {
		panic("missing stop budget")
	}
	return p.stopError
}
func TestMQModuleReplacesLegacyRelayAndResultIngressWhileIntakeClosed(t *testing.T) {
	legacy := &relayStore{entered: make(chan struct{})}
	probe := &messagingLifecycleProbe{}
	m := &Module{aiMessagingRuntime: probe, aiBridge: &app.Service{Store: legacy, Sender: unusedSender{}}}
	if err := m.StartAIWorkflowRelay(t.Context()); err != nil {
		t.Fatal(err)
	}
	if probe.started != 1 || legacy.calls.Load() != 0 {
		t.Fatal("second scheduler or intake-dependent recovery")
	}
	if m.ExportGRPCDeps().AIWorkflowResults != nil {
		t.Fatal("legacy result ingress still active")
	}
	closer := &relayCloser{done: make(chan struct{})}
	m.aiManagementConnection = closer
	probe.stopError = errors.New("drain blocked")
	if m.Cleanup() == nil || closer.closed {
		t.Fatal("borrowed RPC closed before drain")
	}
	done := make(chan struct{})
	close(done)
	closer.done = done
	probe.stopError = nil
	if m.Cleanup() != nil || !closer.closed || probe.stopped != 2 {
		t.Fatal("cleanup order")
	}
}
