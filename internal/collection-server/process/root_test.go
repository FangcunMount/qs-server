package process

import (
	"testing"

	collectionconfig "github.com/FangcunMount/qs-server/internal/collection-server/config"
)

func TestCreateCollectionServerKeepsRootState(t *testing.T) {
	t.Parallel()

	cfg := &collectionconfig.Config{}
	server, err := createServer(cfg)
	if err != nil {
		t.Fatalf("createServer() error = %v", err)
	}
	if server.gs == nil {
		t.Fatal("gs = nil, want value")
	}
	if server.config != cfg {
		t.Fatalf("config = %#v, want %#v", server.config, cfg)
	}
}
