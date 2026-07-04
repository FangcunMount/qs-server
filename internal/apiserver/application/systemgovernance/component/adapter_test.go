package component

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/options"
)

func TestFetchCacheLoadsConfiguredGovernanceRedisEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/governance/redis" {
			t.Fatalf("path = %q, want /governance/redis", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"component": "worker",
			"summary": {"family_total": 0, "ready": true},
			"families": []
		}`))
	}))
	defer server.Close()

	adapter := NewAdapter(map[string]*options.GovernanceComponentOptions{
		"worker": {
			CacheURL: server.URL + "/governance/redis",
			Timeout:  500 * time.Millisecond,
		},
	})

	result := adapter.FetchCache(context.Background())["worker"]
	if !result.Available {
		t.Fatalf("FetchCache() available = false, reason = %q", result.Reason)
	}
	if result.Snapshot == nil || result.Snapshot.Component != "worker" {
		t.Fatalf("snapshot = %#v, want worker snapshot", result.Snapshot)
	}
}

func TestFetchCacheUsesConfiguredTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
		_, _ = w.Write([]byte(`{"component":"worker"}`))
	}))
	defer server.Close()

	adapter := NewAdapter(map[string]*options.GovernanceComponentOptions{
		"worker": {
			CacheURL: server.URL,
			Timeout:  time.Millisecond,
		},
	})

	result := adapter.FetchCache(context.Background())["worker"]
	if result.Available {
		t.Fatalf("FetchCache() available = true, want timeout degradation")
	}
	if !strings.Contains(result.Reason, "context deadline exceeded") {
		t.Fatalf("reason = %q, want context deadline exceeded", result.Reason)
	}
}
