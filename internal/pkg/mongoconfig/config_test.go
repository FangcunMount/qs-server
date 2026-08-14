package mongoconfig

import (
	"net/url"
	"reflect"
	"testing"
	"time"

	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	mongooptions "go.mongodb.org/mongo-driver/mongo/options"
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
	opts.Compressors = []string{"zstd", "snappy"}
	opts.ZstdCompressionLevel = 1

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
		"compressors": "zstd,snappy", "zstdCompressionLevel": "1",
	}
	for key, value := range want {
		if got := parsed.Query().Get(key); got != value {
			t.Fatalf("query %s = %q, want %q", key, got, value)
		}
	}
	driverOptions := mongooptions.Client().ApplyURI(config.URL)
	if err := driverOptions.Validate(); err != nil {
		t.Fatalf("MongoDB driver rejected built URL: %v", err)
	}
	if !reflect.DeepEqual(driverOptions.Compressors, opts.Compressors) {
		t.Fatalf("driver compressors = %v, want %v", driverOptions.Compressors, opts.Compressors)
	}
	if driverOptions.ZstdLevel == nil || *driverOptions.ZstdLevel != opts.ZstdCompressionLevel {
		t.Fatalf("driver zstd level = %v, want %d", driverOptions.ZstdLevel, opts.ZstdCompressionLevel)
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

func TestBuildPreservesCompressionOptionsFromDirectURLWhenTypedOptionsAreEmpty(t *testing.T) {
	opts := genericoptions.NewMongoDBOptions()
	opts.URL = "mongodb://mongo:27017/qs?compressors=zstd&zstdCompressionLevel=3"
	opts.MaxPoolSize = 64

	config, err := Build(opts)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	parsed, err := url.Parse(config.URL)
	if err != nil {
		t.Fatalf("parse built URL: %v", err)
	}
	if got := parsed.Query().Get("compressors"); got != "zstd" {
		t.Fatalf("compressors = %q, want direct-URL value", got)
	}
	if got := parsed.Query().Get("zstdCompressionLevel"); got != "3" {
		t.Fatalf("zstdCompressionLevel = %q, want direct-URL value", got)
	}
}

func TestBuildRejectsInvalidCompressionOptions(t *testing.T) {
	tests := []struct {
		name        string
		compressors []string
		zstdLevel   int
	}{
		{name: "unknown compressor", compressors: []string{"brotli"}},
		{name: "duplicate compressor", compressors: []string{"zstd", "zstd"}},
		{name: "level without zstd", compressors: []string{"snappy"}, zstdLevel: 1},
		{name: "level too high", compressors: []string{"zstd"}, zstdLevel: 21},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := genericoptions.NewMongoDBOptions()
			opts.Compressors = tt.compressors
			opts.ZstdCompressionLevel = tt.zstdLevel
			if _, err := Build(opts); err == nil {
				t.Fatal("Build() error = nil, want invalid compression options")
			}
		})
	}
}
