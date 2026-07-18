package grpcclient

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/resilience/admission"
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
