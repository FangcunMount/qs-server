package prometheus

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientQueryInstantParsesVectorResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1,"42"]}]}}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, time.Second)
	value, ok, err := client.QueryInstant(context.Background(), "up", time.Now())
	if err != nil {
		t.Fatalf("QueryInstant() error = %v", err)
	}
	if !ok || value != 42 {
		t.Fatalf("QueryInstant() = (%v, %v), want (42, true)", value, ok)
	}
}

func TestAdapterProbeDegradesWhenPrometheusUnavailable(t *testing.T) {
	adapter := NewAdapter(nil)
	summary := adapter.Probe(context.Background(), time.Now())
	if summary.Available || summary.Reason == "" {
		t.Fatalf("Probe() = %#v, want unavailable summary", summary)
	}
}
