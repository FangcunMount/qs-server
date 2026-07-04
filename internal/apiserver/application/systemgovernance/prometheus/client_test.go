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

func TestCounterIncreaseQuerySortsAndEscapesLabels(t *testing.T) {
	spec := CounterIncreaseQuery("queue_full", "qs_resilience_decision_total", "5m", map[string]string{
		"strategy": "memory_channel",
		"scope":    "submit\nqueue",
		"resource": `submit"queue`,
	})
	want := `sum(increase(qs_resilience_decision_total{resource="submit\"queue",scope="submit\nqueue",strategy="memory_channel"}[5m]))`
	if spec.Query != want {
		t.Fatalf("query = %q, want %q", spec.Query, want)
	}
	if spec.Name != "queue_full" || spec.Window != "5m" || spec.Unit != "count" {
		t.Fatalf("spec metadata = %#v", spec)
	}
}

func TestInstantGaugeQuerySortsAndEscapesLabels(t *testing.T) {
	spec := InstantGaugeQuery("outbox_pending", "qs_event_outbox_backlog", "15m", "count", map[string]string{
		"status":     "pending",
		"event_type": `assessment"submitted`,
		"store":      "mysql",
	})
	want := `sum(qs_event_outbox_backlog{event_type="assessment\"submitted",status="pending",store="mysql"})`
	if spec.Query != want {
		t.Fatalf("query = %q, want %q", spec.Query, want)
	}
	if spec.Name != "outbox_pending" || spec.Window != "15m" || spec.Unit != "count" {
		t.Fatalf("spec metadata = %#v", spec)
	}
}
