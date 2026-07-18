package grpcclient

import (
	"context"
	"testing"

	pkgmiddleware "github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/admission"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestNewManagerRequiresInjectedInflightSemaphore(t *testing.T) {
	if _, err := NewManager(&ManagerConfig{Endpoint: "passthrough:///unused", Insecure: true}); err == nil {
		t.Fatal("NewManager() error = nil, want missing inflight semaphore failure")
	}
}

func TestNewManagerUsesInjectedInflightSemaphore(t *testing.T) {
	sem := admission.NewChannelSemaphore(3)
	manager, err := NewManager(&ManagerConfig{
		Endpoint:          "passthrough:///unused",
		Insecure:          true,
		InflightSemaphore: sem,
	})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	if manager.inflightSem != sem {
		t.Fatal("manager did not retain the injected process semaphore")
	}
}

func TestUnaryInterceptorPropagatesRequestIDMetadata(t *testing.T) {
	manager := &Manager{
		config:      &ManagerConfig{},
		inflightSem: admission.NewChannelSemaphore(1),
	}
	ctx := pkgmiddleware.WithRequestID(context.Background(), "req-collection-1")
	err := manager.unaryInterceptor(ctx, "/test.Service/Call", nil, nil, nil, func(callCtx context.Context, _ string, _, _ interface{}, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		md, _ := metadata.FromOutgoingContext(callCtx)
		values := md.Get("x-request-id")
		if len(values) != 1 || values[0] != "req-collection-1" {
			t.Fatalf("x-request-id metadata = %v", values)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unaryInterceptor() error = %v", err)
	}
}
