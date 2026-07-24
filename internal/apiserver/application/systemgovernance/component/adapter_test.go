package component

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
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
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, additiveField := range []string{"instances", "discovered_instance_count", "available_instance_count", "partial", "target_errors"} {
		if strings.Contains(string(encoded), `"`+additiveField+`"`) {
			t.Fatalf("single mode JSON = %s, must omit additive DNS field %s", encoded, additiveField)
		}
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

func TestDNSDiscoveryFetchesEveryUniqueIPv4AndSelectsStableSnapshot(t *testing.T) {
	adapter := NewAdapter(map[string]*options.GovernanceComponentOptions{
		"collection-server": {
			Discovery: "dns", MinimumInstances: 2,
			ResilienceURL: "http://collection.test:8080/governance/resilience",
			CacheURL:      "http://collection.test:8080/governance/redis",
			Timeout:       time.Second,
		},
	})
	adapter.resolver = staticResolver{addresses: []net.IPAddr{
		{IP: net.ParseIP("10.0.0.2")},
		{IP: net.ParseIP("10.0.0.1")},
		{IP: net.ParseIP("10.0.0.1")},
		{IP: net.ParseIP("2001:db8::1")},
	}}
	adapter.http = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Host != "collection.test:8080" {
			t.Errorf("Host = %q, want original collection.test:8080", req.Host)
		}
		instanceID := "collection-b"
		if req.URL.Hostname() == "10.0.0.2" {
			instanceID = "collection-a"
		}
		body := `{"component":"collection-server","instance_id":"` + instanceID + `","generation":"g1","summary":{"ready":true},"families":[]}`
		return jsonResponse(http.StatusOK, body), nil
	})}

	resilienceResult := adapter.FetchResilience(context.Background())["collection-server"]
	if !resilienceResult.Available || resilienceResult.Partial {
		t.Fatalf("resilience result = %#v, want complete availability", resilienceResult)
	}
	if resilienceResult.DiscoveredInstanceCount != 2 || resilienceResult.AvailableInstanceCount != 2 {
		t.Fatalf("resilience counts = %d/%d", resilienceResult.AvailableInstanceCount, resilienceResult.DiscoveredInstanceCount)
	}
	if len(resilienceResult.Instances) != 2 || resilienceResult.Snapshot == nil || resilienceResult.Snapshot.InstanceID != "collection-a" {
		t.Fatalf("resilience instances = %#v snapshot=%#v", resilienceResult.Instances, resilienceResult.Snapshot)
	}

	cacheResult := adapter.FetchCache(context.Background())["collection-server"]
	if !cacheResult.Available || cacheResult.Partial || len(cacheResult.Instances) != 2 {
		t.Fatalf("cache result = %#v, want two complete instances", cacheResult)
	}
	if cacheResult.Snapshot == nil || cacheResult.Snapshot.InstanceID != "collection-a" {
		t.Fatalf("cache representative = %#v, want lexicographically smallest instance", cacheResult.Snapshot)
	}
}

func TestDNSDiscoveryReportsPartialWithoutOverwritingDuplicateInstance(t *testing.T) {
	adapter := NewAdapter(map[string]*options.GovernanceComponentOptions{
		"collection-server": {
			Discovery: "dns", MinimumInstances: 2,
			ResilienceURL: "http://collection.test:8080/governance/resilience",
			Timeout:       time.Second,
		},
	})
	adapter.resolver = staticResolver{addresses: []net.IPAddr{
		{IP: net.ParseIP("10.0.0.1")},
		{IP: net.ParseIP("10.0.0.2")},
	}}
	adapter.http = &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, `{
			"component":"collection-server",
			"instance_id":"collection-duplicate",
			"generation":"g1",
			"summary":{"ready":true}
		}`), nil
	})}

	result := adapter.FetchResilience(context.Background())["collection-server"]
	if !result.Available || !result.Partial || result.AvailableInstanceCount != 1 {
		t.Fatalf("result = %#v, want one available partial instance", result)
	}
	if len(result.TargetErrors) != 1 || len(result.Instances) != 1 {
		t.Fatalf("instances=%#v errors=%#v, want duplicate rejected", result.Instances, result.TargetErrors)
	}
}

func TestDNSDiscoveryReportsPartialTargetFailureAndEmptyIdentity(t *testing.T) {
	adapter := NewAdapter(map[string]*options.GovernanceComponentOptions{
		"collection-server": {
			Discovery: "dns", MinimumInstances: 3,
			CacheURL: "http://collection.test:8080/governance/redis", Timeout: time.Second,
		},
	})
	adapter.resolver = staticResolver{addresses: []net.IPAddr{
		{IP: net.ParseIP("10.0.0.1")},
		{IP: net.ParseIP("10.0.0.2")},
		{IP: net.ParseIP("10.0.0.3")},
	}}
	adapter.http = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Hostname() {
		case "10.0.0.1":
			return jsonResponse(http.StatusOK, `{"component":"collection-server","instance_id":"collection-a","summary":{"ready":true}}`), nil
		case "10.0.0.2":
			return nil, errors.New("connection refused")
		default:
			return jsonResponse(http.StatusOK, `{"component":"collection-server","summary":{"ready":true}}`), nil
		}
	})}

	result := adapter.FetchCache(context.Background())["collection-server"]
	if !result.Available || !result.Partial || result.AvailableInstanceCount != 1 ||
		result.DiscoveredInstanceCount != 3 || len(result.TargetErrors) != 2 {
		t.Fatalf("result = %#v, want one success plus transport and identity errors", result)
	}
}

func TestDNSDiscoveryBoundsUniqueIPv4Targets(t *testing.T) {
	addresses := make([]net.IPAddr, 0, maxDNSAddresses+1)
	for index := 1; index <= maxDNSAddresses+1; index++ {
		addresses = append(addresses, net.IPAddr{IP: net.IPv4(10, 0, 0, byte(index))})
	}
	adapter := NewAdapter(map[string]*options.GovernanceComponentOptions{
		"collection-server": {
			Discovery: "dns", MinimumInstances: 1,
			ResilienceURL: "http://collection.test:8080/governance/resilience", Timeout: time.Second,
		},
	})
	adapter.resolver = staticResolver{addresses: addresses}
	adapter.http = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		instanceID := strings.ReplaceAll(req.URL.Hostname(), ".", "-")
		return jsonResponse(http.StatusOK, `{"component":"collection-server","instance_id":"`+instanceID+`","summary":{"ready":true}}`), nil
	})}

	result := adapter.FetchResilience(context.Background())["collection-server"]
	if result.DiscoveredInstanceCount != maxDNSAddresses || result.AvailableInstanceCount != maxDNSAddresses {
		t.Fatalf("counts = %d/%d, want max %d", result.AvailableInstanceCount, result.DiscoveredInstanceCount, maxDNSAddresses)
	}
}

func TestDNSDiscoveryAllTargetsFailedIsUnavailable(t *testing.T) {
	adapter := NewAdapter(map[string]*options.GovernanceComponentOptions{
		"collection-server": {
			Discovery: "dns", MinimumInstances: 2,
			CacheURL: "http://collection.test:8080/governance/redis", Timeout: time.Second,
		},
	})
	adapter.resolver = staticResolver{addresses: []net.IPAddr{
		{IP: net.ParseIP("10.0.0.1")},
		{IP: net.ParseIP("10.0.0.2")},
	}}
	adapter.http = &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return nil, errors.New("dial failed")
	})}

	result := adapter.FetchCache(context.Background())["collection-server"]
	if result.Available || !result.Partial || result.AvailableInstanceCount != 0 || len(result.TargetErrors) != 2 {
		t.Fatalf("result = %#v, want all targets unavailable", result)
	}
}

func TestDNSDiscoveryFailureIsUnavailable(t *testing.T) {
	adapter := NewAdapter(map[string]*options.GovernanceComponentOptions{
		"collection-server": {
			Discovery: "dns", MinimumInstances: 2,
			ResilienceURL: "http://collection.test:8080/governance/resilience", Timeout: time.Second,
		},
	})
	adapter.resolver = staticResolver{err: errors.New("dns unavailable")}

	result := adapter.FetchResilience(context.Background())["collection-server"]
	if result.Available || !strings.Contains(result.Reason, "dns unavailable") {
		t.Fatalf("result = %#v, want DNS failure", result)
	}
}

func TestDNSDiscoveryPreservesRequestCancellation(t *testing.T) {
	adapter := NewAdapter(map[string]*options.GovernanceComponentOptions{
		"collection-server": {
			Discovery: "dns", MinimumInstances: 2,
			ResilienceURL: "http://collection.test:8080/governance/resilience", Timeout: time.Second,
		},
	})
	adapter.resolver = resolverFunc(func(ctx context.Context, _ string) ([]net.IPAddr, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := adapter.FetchResilience(ctx)["collection-server"]
	if result.Available || !strings.Contains(result.Reason, context.Canceled.Error()) {
		t.Fatalf("result = %#v, want canceled DNS discovery", result)
	}
}

type staticResolver struct {
	addresses []net.IPAddr
	err       error
}

func (r staticResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return r.addresses, r.err
}

type resolverFunc func(context.Context, string) ([]net.IPAddr, error)

func (f resolverFunc) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return f(ctx, host)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
