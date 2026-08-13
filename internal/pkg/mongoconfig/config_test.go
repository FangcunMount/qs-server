package mongoconfig

import (
	"net/url"
	"testing"
	"time"

	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
)

func TestBuildAppliesPoolOptionsToSeparatedConnectionFields(t *testing.T) {
	opts := genericoptions.NewMongoDBOptions()
	opts.Host = "mongo:27017"
	opts.Username = "app user"
	opts.Password = "secret/value"
	opts.Database = "qs"
	opts.ReplicaSet = "rs0"
	opts.DirectConnection = true
	opts.MinPoolSize = 16
	opts.MaxPoolSize = 64
	opts.MaxConnecting = 8
	opts.MaxConnIdleTime = 10 * time.Minute

	config, err := Build(opts)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	parsed, err := url.Parse(config.URL)
	if err != nil {
		t.Fatalf("parse built URL: %v", err)
	}
	if parsed.Host != "mongo:27017" || parsed.Path != "/qs" {
		t.Fatalf("built URL target = %s%s, want mongo:27017/qs", parsed.Host, parsed.Path)
	}
	username := parsed.User.Username()
	password, _ := parsed.User.Password()
	if username != opts.Username || password != opts.Password {
		t.Fatal("built URL must preserve encoded credentials")
	}
	want := map[string]string{
		"replicaSet": "rs0", "directConnection": "true",
		"minPoolSize": "16", "maxPoolSize": "64", "maxConnecting": "8", "maxIdleTimeMS": "600000",
	}
	for key, value := range want {
		if got := parsed.Query().Get(key); got != value {
			t.Fatalf("query %s = %q, want %q", key, got, value)
		}
	}
}

func TestBuildOverridesOnlyExplicitPoolOptionsInDirectURL(t *testing.T) {
	opts := genericoptions.NewMongoDBOptions()
	opts.URL = "mongodb://mongo:27017/qs?replicaSet=rs0&maxPoolSize=90&minPoolSize=7"
	opts.Host = "ignored:27017"
	opts.MaxPoolSize = 64

	config, err := Build(opts)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	parsed, err := url.Parse(config.URL)
	if err != nil {
		t.Fatalf("parse built URL: %v", err)
	}
	if got := parsed.Query().Get("maxPoolSize"); got != "64" {
		t.Fatalf("maxPoolSize = %q, want 64", got)
	}
	if got := parsed.Query().Get("minPoolSize"); got != "7" {
		t.Fatalf("minPoolSize = %q, want existing direct-URL value", got)
	}
	if got := parsed.Query().Get("replicaSet"); got != "rs0" {
		t.Fatalf("replicaSet = %q, want rs0", got)
	}
}

func TestBuildRejectsInvalidPoolBudget(t *testing.T) {
	opts := genericoptions.NewMongoDBOptions()
	opts.MaxPoolSize = 8
	opts.MinPoolSize = 9

	if _, err := Build(opts); err == nil {
		t.Fatal("Build() error = nil, want invalid pool budget")
	}
}
