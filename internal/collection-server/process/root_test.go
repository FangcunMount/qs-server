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
	if server.runtimeFacts == nil {
		t.Fatal("Collection runtime facts owner missing")
	}
	snapshot := server.runtimeFacts.Snapshot()
	if len(snapshot.Transports) != 1 || snapshot.Transports[0].Direction != "no_local_mq" || snapshot.BrokerConnectionsVerified || snapshot.ObservationComplete {
		t.Fatal("Collection fabricated a local MQ consumer or live observation")
	}
	if err := server.runtimeFacts.Close(); err != nil {
		t.Fatal(err)
	}
}
