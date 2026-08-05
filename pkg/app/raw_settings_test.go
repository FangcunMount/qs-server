package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
)

func TestRawSettingsSourcePrecedenceFileEnvExplicitFlag(t *testing.T) {
	path := filepath.Join(t.TempDir(), "apiserver.yaml")
	if err := os.WriteFile(path, []byte("cache:\n  defaults:\n    ttl_jitter_ratio: 0.1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	flags := pflag.NewFlagSet("reload", pflag.ContinueOnError)
	flags.Float64("cache.defaults.ttl_jitter_ratio", 0.9, "")
	source := newRawSettingsSource(path, "QS_APISERVER", flags)

	t.Setenv("QS_APISERVER_CACHE_DEFAULTS_TTL_JITTER_RATIO", "0.2")
	settings, err := source.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := nestedFloat(t, settings.Values, "cache", "defaults", "ttl_jitter_ratio"); got != 0.2 {
		t.Fatalf("env value = %v, want 0.2", got)
	}
	if err := flags.Set("cache.defaults.ttl_jitter_ratio", "0.3"); err != nil {
		t.Fatal(err)
	}
	settings, err = source.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := nestedFloat(t, settings.Values, "cache", "defaults", "ttl_jitter_ratio"); got != 0.3 {
		t.Fatalf("explicit flag value = %v, want 0.3", got)
	}
}

func TestRawSettingsSourceUsesCanonicalDeploymentEnvironmentKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "apiserver.yaml")
	if err := os.WriteFile(path, []byte("redis:\n  database: 0\nmessaging:\n  nsq-addr: config-nsqd:4150\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	source := newRawSettingsSource(path, "QS_APISERVER", nil)

	t.Setenv("QS_APISERVER_REDIS_DATABASE", "6")
	t.Setenv("QS_APISERVER_MESSAGING_NSQ_ADDR", "nsqd:4150")
	t.Setenv("QS_APISERVER_REDIS_DB", "9")
	t.Setenv("QS_APISERVER_NSQ_NSQD_HOST", "legacy-nsqd")

	settings, err := source.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := nestedString(t, settings.Values, "redis", "database"); got != "6" {
		t.Fatalf("redis.database = %q, want canonical env value 6", got)
	}
	if got := nestedString(t, settings.Values, "messaging", "nsq-addr"); got != "nsqd:4150" {
		t.Fatalf("messaging.nsq-addr = %q, want canonical env value", got)
	}
}

func nestedFloat(t *testing.T, values map[string]any, path ...string) float64 {
	t.Helper()
	var current any = values
	for _, key := range path {
		mapping, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("%s is %T, want map", key, current)
		}
		current = mapping[key]
	}
	switch value := current.(type) {
	case float64:
		return value
	case string:
		var result float64
		if _, err := fmt.Sscan(value, &result); err != nil {
			t.Fatal(err)
		}
		return result
	default:
		t.Fatalf("value is %T", current)
		return 0
	}
}

func nestedString(t *testing.T, values map[string]any, path ...string) string {
	t.Helper()
	var current any = values
	for _, key := range path {
		mapping, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("%s is %T, want map", key, current)
		}
		current = mapping[key]
	}
	return fmt.Sprint(current)
}
