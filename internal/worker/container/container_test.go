package container

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/worker/options"
)

func TestContainerInitializeRequiresInternalClient(t *testing.T) {
	c, err := NewContainer(options.NewOptions(), nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = c.Initialize()
	if err == nil {
		t.Fatal("expected initialize to fail without internal client")
	}
}
