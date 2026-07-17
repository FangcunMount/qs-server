package systemgovernance

import (
	"context"
	"sync"
	"testing"
	"time"

	govprom "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance/prometheus"
	"github.com/FangcunMount/qs-server/internal/pkg/resilienceplane"
)

func TestMetricEvidenceReaderBuildsSortedEscapedCounterQuery(t *testing.T) {
	metrics := &recordingMetricsReader{}
	reader := NewMetricEvidenceReader(metrics)

	evidence, ok := reader.CounterIncrease(context.Background(), "queue_full", "qs_resilience_decision_total", "5m", map[string]string{
		"strategy": "memory_channel",
		"resource": "submit\"queue",
		"scope":    "submit\nqueue",
	}, time.Now())

	if !ok {
		t.Fatal("CounterIncrease() ok = false, want true")
	}
	if evidence.Name != "queue_full" || !evidence.Available {
		t.Fatalf("evidence = %#v, want available queue_full evidence", evidence)
	}
	want := `sum(increase(qs_resilience_decision_total{resource="submit\"queue",scope="submit\nqueue",strategy="memory_channel"}[5m]))`
	if len(metrics.specs) != 1 || metrics.specs[0].Query != want {
		t.Fatalf("query = %#v, want %q", metrics.specs, want)
	}
}

func TestMetricEvidenceReaderScopesResilienceDecisionLabels(t *testing.T) {
	metrics := &recordingMetricsReader{}
	reader := NewMetricEvidenceReader(metrics)
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)

	reader.ResilienceQueueFull(context.Background(), "collection-server", resilienceplane.QueueSnapshot{
		Component: "collection-server",
		Name:      "answersheet_submit",
		Strategy:  "memory_channel",
	}, "5m", now)
	reader.ResilienceBackpressureTimeout(context.Background(), "apiserver", resilienceplane.BackpressureSnapshot{
		Component:  "apiserver",
		Name:       "mysql",
		Dependency: "mysql",
		Strategy:   "semaphore",
	}, "5m", now)

	wantQueries := []string{
		`sum(increase(qs_resilience_decision_total{component="collection-server",kind="queue",outcome="queue_full",resource="submit_queue",scope="answersheet_submit",strategy="memory_channel"}[5m]))`,
		`sum(increase(qs_resilience_decision_total{component="apiserver",kind="backpressure",outcome="backpressure_timeout",resource="downstream",scope="mysql",strategy="semaphore"}[5m]))`,
	}
	if len(metrics.specs) != len(wantQueries) {
		t.Fatalf("metrics specs = %#v, want %d specs", metrics.specs, len(wantQueries))
	}
	if metrics.specs[0].Name != "queue_full_collection-server_answersheet_submit" {
		t.Fatalf("queue evidence name = %q, want legacy display name", metrics.specs[0].Name)
	}
	for i, want := range wantQueries {
		if metrics.specs[i].Query != want {
			t.Fatalf("query[%d] = %q, want %q", i, metrics.specs[i].Query, want)
		}
	}
}

func TestMetricEvidenceReaderBuildsEventQueries(t *testing.T) {
	metrics := &recordingMetricsReader{}
	reader := NewMetricEvidenceReader(metrics)
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)

	reader.EventOutboxPendingBacklog(context.Background(), "mysql", "5m", now)
	reader.EventOutboxPendingOldestAge(context.Background(), "mysql", "5m", now)
	reader.EventOutboxStatusScrapeFailure(context.Background(), "mysql", "5m", now)
	reader.EventTypePendingBacklog(context.Background(), "mysql", "evaluation.requested", "5m", now)
	reader.EventTypePendingOldestAge(context.Background(), "mysql", "evaluation.requested", "5m", now)

	want := []string{
		`sum(qs_event_outbox_backlog{status="pending",store="mysql"})`,
		`sum(qs_event_outbox_oldest_age_seconds{status="pending",store="mysql"})`,
		`sum(increase(qs_event_outbox_status_scrape_total{outcome="failure",store="mysql"}[5m]))`,
		`sum(qs_event_outbox_backlog_by_type{event_type="evaluation.requested",status="pending",store="mysql"})`,
		`sum(qs_event_outbox_oldest_age_by_type_seconds{event_type="evaluation.requested",status="pending",store="mysql"})`,
	}
	if len(metrics.specs) != len(want) {
		t.Fatalf("metric specs = %#v, want %d specs", metrics.specs, len(want))
	}
	for i, query := range want {
		if metrics.specs[i].Query != query {
			t.Fatalf("query[%d] = %q, want %q", i, metrics.specs[i].Query, query)
		}
	}
	if metrics.specs[0].Name != "outbox_pending_backlog_mysql" || metrics.specs[0].Unit != "count" {
		t.Fatalf("outbox backlog spec = %#v, want legacy name and count unit", metrics.specs[0])
	}
	if metrics.specs[1].Unit != "seconds" {
		t.Fatalf("oldest age unit = %q, want seconds", metrics.specs[1].Unit)
	}
}

func TestMetricEvidenceReaderBuildsCacheQueries(t *testing.T) {
	metrics := &recordingMetricsReader{}
	reader := NewMetricEvidenceReader(metrics)
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)

	reader.CacheFamilyAvailable(context.Background(), "apiserver", "query_result", "query_cache", "5m", now)
	reader.CacheFamilyDegraded(context.Background(), "apiserver", "query_result", "query_cache", "5m", now)
	reader.CacheWarmupRunsError(context.Background(), "5m", now)
	reader.CacheHotsetSize(context.Background(), "query_result", "query.stats_overview", "5m", now)

	want := []string{
		`sum(qs_cache_family_available{component="apiserver",family="query_result",profile="query_cache"})`,
		`sum(increase(qs_cache_family_degraded_total{component="apiserver",family="query_result",profile="query_cache"}[5m]))`,
		`sum(increase(qs_cache_warmup_runs_total{result="error"}[5m]))`,
		`sum(qs_cache_hotset_size{family="query_result",kind="query.stats_overview"})`,
	}
	if len(metrics.specs) != len(want) {
		t.Fatalf("metric specs = %#v, want %d specs", metrics.specs, len(want))
	}
	for i, query := range want {
		if metrics.specs[i].Query != query {
			t.Fatalf("query[%d] = %q, want %q", i, metrics.specs[i].Query, query)
		}
	}
	if metrics.specs[0].Name != "cache_family_available_apiserver_query_result" || metrics.specs[0].Unit != "bool" {
		t.Fatalf("cache available spec = %#v, want legacy name and bool unit", metrics.specs[0])
	}
	if metrics.specs[3].Name != "cache_hotset_size_query_stats_overview" || metrics.specs[3].Unit != "count" {
		t.Fatalf("hotset size spec = %#v, want sanitized legacy name and count unit", metrics.specs[3])
	}
}

func TestMetricEvidenceReaderBuildsCanonicalCapabilityWorkloadQueries(t *testing.T) {
	metrics := &recordingMetricsReader{}
	reader := NewMetricEvidenceReader(metrics)
	now := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)

	reader.CacheCapabilityHitRate(context.Background(), "statistics.query", "query_result", "stats_query", "5m", now)
	reader.CacheCapabilityErrorCount(context.Background(), "statistics.query", "query_result", "stats_query", "5m", now)
	reader.CacheCapabilityGetP95(context.Background(), "statistics.query", "query_result", "stats_query", "5m", now)

	want := []string{
		`(sum(increase(qs_cache_get_total{family="query_result",policy="stats_query",result="hit"}[5m]))) / clamp_min((sum(increase(qs_cache_get_total{family="query_result",policy="stats_query",result="hit"}[5m])) + sum(increase(qs_cache_get_total{family="query_result",policy="stats_query",result="miss"}[5m]))), 1)`,
		`sum(increase(qs_cache_get_total{family="query_result",policy="stats_query",result="error"}[5m])) + sum(increase(qs_cache_write_total{family="query_result",policy="stats_query",result="error"}[5m]))`,
		`histogram_quantile(0.95, sum by (le) (rate(qs_cache_operation_duration_seconds_bucket{family="query_result",op="get",policy="stats_query"}[5m])))`,
	}
	if len(metrics.specs) != len(want) {
		t.Fatalf("metric specs = %#v, want %d", metrics.specs, len(want))
	}
	for index, query := range want {
		if metrics.specs[index].Query != query {
			t.Fatalf("query[%d] = %q, want %q", index, metrics.specs[index].Query, query)
		}
	}
	if metrics.specs[0].Name != "cache_hit_rate_statistics_query" || metrics.specs[0].Unit != "ratio" {
		t.Fatalf("hit rate spec = %#v", metrics.specs[0])
	}
	if metrics.specs[1].Name != "cache_error_count_statistics_query" || metrics.specs[1].Unit != "count" {
		t.Fatalf("error count spec = %#v", metrics.specs[1])
	}
	if metrics.specs[2].Name != "cache_get_p95_statistics_query" || metrics.specs[2].Unit != "seconds" {
		t.Fatalf("latency spec = %#v", metrics.specs[2])
	}
}

type unavailableMetricsReader struct{}

func (unavailableMetricsReader) Query(_ context.Context, spec govprom.QuerySpec, _ time.Time) govprom.MetricResult {
	return govprom.MetricResult{
		Name:      spec.Name,
		Window:    spec.Window,
		Unit:      spec.Unit,
		Available: false,
		Reason:    "prometheus unavailable",
	}
}

type recordingMetricsReader struct {
	mu    sync.Mutex
	specs []govprom.QuerySpec
}

func (r *recordingMetricsReader) Query(_ context.Context, spec govprom.QuerySpec, _ time.Time) govprom.MetricResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.specs = append(r.specs, spec)
	value := 1.0
	return govprom.MetricResult{
		Name:      spec.Name,
		Window:    spec.Window,
		Unit:      spec.Unit,
		Value:     &value,
		Available: true,
	}
}
