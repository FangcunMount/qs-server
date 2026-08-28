package grpcclient

import (
	"testing"
	"time"
)

func TestManagerKeepsSeparateOrdinaryAndAIExplanationTimeouts(t *testing.T) {
	manager, err := NewManager(&ManagerConfig{
		Endpoint:             "localhost:9090",
		Timeout:              17 * time.Second,
		AIExplanationTimeout: 2*time.Minute + 45*time.Second,
		Insecure:             true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })

	if manager.Timeout() != 17*time.Second {
		t.Fatalf("ordinary timeout = %s", manager.Timeout())
	}
	if manager.AIExplanationTimeout() != 2*time.Minute+45*time.Second {
		t.Fatalf("AI explanation timeout = %s", manager.AIExplanationTimeout())
	}
}

func TestManagerDefaultsAIExplanationTimeoutWithoutChangingOrdinaryTimeout(t *testing.T) {
	manager, err := NewManager(&ManagerConfig{Endpoint: "localhost:9090", Insecure: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })

	if manager.Timeout() != 30*time.Second || manager.AIExplanationTimeout() != 3*time.Minute {
		t.Fatalf("timeouts = %s/%s", manager.Timeout(), manager.AIExplanationTimeout())
	}
}
