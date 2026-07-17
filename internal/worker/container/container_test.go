package container

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/worker/options"
)

func TestContainerInitializeRequiresInternalClient(t *testing.T) {
	c := NewContainer(options.NewOptions(), nil, nil, nil, nil)
	err := c.Initialize()
	if err == nil {
		t.Fatal("expected initialize to fail without internal client")
	}
}
