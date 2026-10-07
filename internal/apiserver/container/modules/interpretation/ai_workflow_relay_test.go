package interpretation

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	bridge "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	grpctransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc"
	servergrpc "github.com/FangcunMount/qs-server/internal/pkg/grpc"
	"google.golang.org/grpc"
)

type relayStore struct {
	bridge.Store
	entered chan struct{}
	calls   atomic.Int32
}

func (s *relayStore) Pending(ctx context.Context, _ int) ([]struct{}, error) {
	s.calls.Add(1)
	close(s.entered)
	<-ctx.Done()
	return nil, ctx.Err()
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
	if err := m.StartAIWorkflowRelay(t.Context()); err != nil {
		t.Fatal("disabled relay started")
	}
	m.aiWorkflowEnabled = true
	if err := m.StartAIWorkflowRelay(t.Context()); err == nil {
		t.Fatal("missing relay accepted")
	}
}

func TestMQRetirementNeverStartsLegacyTransportOrExportsResultIngress(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprint(enabled), func(t *testing.T) {
			store := &relayStore{entered: make(chan struct{})}
			m := &Module{aiWorkflowEnabled: enabled, aiBridge: &bridge.Service{Store: store}}
			t.Cleanup(func() { _ = m.Cleanup() })
			err := m.StartAIWorkflowRelay(t.Context())
			if enabled && err == nil {
				t.Fatal("legacy transport accepted without MQ runtime")
			}
			if !enabled && err != nil {
				t.Fatal("query-only configuration cannot start", err)
			}
			if store.calls.Load() != 0 {
				t.Fatal("legacy sender was scheduled")
			}
			assertRetiredResultIngressNotRegistered(t, m)
		})
	}
}

func assertRetiredResultIngressNotRegistered(t *testing.T, module *Module) {
	t.Helper()
	server := &servergrpc.Server{Server: grpc.NewServer()}
	t.Cleanup(server.Stop)
	registry := grpctransport.NewRegistry(grpctransport.Deps{Server: server, Interpretation: module.ExportGRPCDeps()})
	if err := registry.RegisterServices(); err != nil {
		t.Fatal(err)
	}
	if _, registered := server.GetServiceInfo()["qsai.workflow.v1.Results"]; registered {
		t.Fatal("retired result write ingress registered in runtime")
	}
}
