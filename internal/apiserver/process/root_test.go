package process

import (
	"reflect"
	"testing"

	apiserverconfig "github.com/FangcunMount/qs-server/internal/apiserver/config"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
)

func TestCreateAPIServerKeepsOnlyRootState(t *testing.T) {
	t.Parallel()

	cfg, err := apiserverconfig.CreateConfigFromOptions(apiserveroptions.NewOptions())
	if err != nil {
		t.Fatalf("CreateConfigFromOptions() error = %v", err)
	}

	server, err := createServer(cfg)
	if err != nil {
		t.Fatalf("createServer() error = %v", err)
	}

	if server.gs == nil {
		t.Fatal("gs = nil, want graceful shutdown manager")
	}
	if server.config != cfg {
		t.Fatalf("config = %#v, want %#v", server.config, cfg)
	}

	typ := reflect.TypeOf(*server)
	if typ.NumField() != 2 {
		t.Fatalf("server field count = %d, want 2", typ.NumField())
	}
	if got := typ.Field(0).Name; got != "gs" {
		t.Fatalf("field[0] = %q, want gs", got)
	}
	if got := typ.Field(1).Name; got != "config" {
		t.Fatalf("field[1] = %q, want config", got)
	}
}
