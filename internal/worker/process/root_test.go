package process

import (
	"testing"

	workerconfig "github.com/FangcunMount/qs-server/internal/worker/config"
)

func TestCreateWorkerServerKeepsRootState(t *testing.T) {
	t.Parallel()

	cfg := &workerconfig.Config{
		Log: &workerconfig.LogConfig{},
	}
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
	if server.logger == nil {
		t.Fatal("logger = nil, want value")
	}
}
