package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParsePrometheusAndRetryRateCanExceedOne(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metrics.txt")
	content := "qs_retry_layer_attempt_total{layer=\"business\",attempt_class=\"initial\",component=\"evaluation\",origin=\"initial\",outcome=\"success\"} 2\n" +
		"qs_retry_layer_attempt_total{layer=\"business\",attempt_class=\"retry\",component=\"evaluation\",origin=\"automatic\",outcome=\"failure\"} 5\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	samples, err := parsePrometheusFile(path)
	if err != nil {
		t.Fatal(err)
	}
	initial, ok := sumMetric(samples, "qs_retry_layer_attempt_total", map[string]string{"layer": "business", "attempt_class": "initial"})
	if !ok || initial != 2 {
		t.Fatalf("initial = %v, %v", initial, ok)
	}
	retries, ok := sumMetric(samples, "qs_retry_layer_attempt_total", map[string]string{"layer": "business", "attempt_class": "retry"})
	if !ok || retries/initial != 2.5 {
		t.Fatalf("retry amplification = %v, %v", retries/initial, ok)
	}
}

func TestRecoveryRequiresBacklogToReturnToBaseline(t *testing.T) {
	baselineBacklog, currentBacklog := 1.0, 3.0
	baselineDepth, currentDepth := 0.0, 0.0
	baseline := PhaseEvidence{Complete: true, OutboxBacklog: &baselineBacklog, NSQDepth: &baselineDepth}
	current := PhaseEvidence{Complete: true, OutboxBacklog: &currentBacklog, NSQDepth: &currentDepth}
	verdict := recoveryVerdict(baseline, current)
	if verdict.Status != VerdictFail {
		t.Fatalf("verdict = %s, want FAIL", verdict.Status)
	}
}

func TestCompletedModelDeltasUseBoundedBuilderMapping(t *testing.T) {
	before := []metricSample{
		{Name: "qs_interpretation_run_duration_seconds_count", Labels: map[string]string{"result": "success", "builder_identity": "factor-scoring"}, Value: 10},
		{Name: "qs_interpretation_run_duration_seconds_count", Labels: map[string]string{"result": "success", "builder_identity": "norm-profile"}, Value: 2},
		{Name: "qs_interpretation_run_duration_seconds_count", Labels: map[string]string{"result": "success", "builder_identity": "typology"}, Value: 5},
	}
	after := []metricSample{
		{Name: "qs_interpretation_run_duration_seconds_count", Labels: map[string]string{"result": "success", "builder_identity": "factor-scoring"}, Value: 14},
		{Name: "qs_interpretation_run_duration_seconds_count", Labels: map[string]string{"result": "success", "builder_identity": "norm-profile"}, Value: 3},
		{Name: "qs_interpretation_run_duration_seconds_count", Labels: map[string]string{"result": "success", "builder_identity": "typology"}, Value: 8},
	}
	got := completedModelDeltas(before, after)
	if got["medical"] != 5 || got["personality"] != 3 {
		t.Fatalf("completed model deltas = %#v", got)
	}
	completed, failed, _, ok := interpretationDeltas(before, after)
	if !ok || completed == nil || *completed != 8 || failed == nil || *failed != 0 {
		t.Fatalf("interpretation deltas = completed=%v failed=%v ok=%v", completed, failed, ok)
	}
}

func TestReadyEvidenceAcceptsDirectAndCollectionEnvelope(t *testing.T) {
	dir := t.TempDir()
	for name, payload := range map[string]string{
		"direct.json":     `{"status":"ready"}`,
		"collection.json": `{"code":0,"data":{"status":"ready"}}`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(payload), 0o600); err != nil {
			t.Fatal(err)
		}
		if check := checkReadyFile(dir, name, name); check.Status != "PASS" {
			t.Fatalf("%s check = %#v", name, check)
		}
	}
}

func TestReplicaReadyEvidenceRequiresAggregatePrometheusResult(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "direct.json"), []byte(`{"code":0,"data":{"status":"ready"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if check := checkReplicaReadyFile(dir, "direct.json", "collection readyz", "EXPECTED_COLLECTION_REPLICAS", 2); check.Status != "INVALID" {
		t.Fatalf("direct load-balanced readyz check = %#v, want INVALID", check)
	}

	prometheusReady := `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1776030000,"2"]}]}}`
	if err := os.WriteFile(filepath.Join(dir, "aggregate.json"), []byte(prometheusReady), 0o600); err != nil {
		t.Fatal(err)
	}
	if check := checkReplicaReadyFile(dir, "aggregate.json", "collection readyz", "EXPECTED_COLLECTION_REPLICAS", 2); check.Status != "PASS" {
		t.Fatalf("aggregate readyz check = %#v, want PASS", check)
	}

	t.Setenv("EXPECTED_COLLECTION_REPLICAS", "3")
	if check := checkReplicaReadyFile(dir, "aggregate.json", "collection readyz", "EXPECTED_COLLECTION_REPLICAS", 2); check.Status != "FAIL" {
		t.Fatalf("replica mismatch check = %#v, want FAIL", check)
	}
}

func TestFederatedMetricsEvidenceRequiresEveryReplicaReady(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "collection-metrics.txt")
	metrics := "qs_runtime_component_ready{component=\"collection-server\",instance=\"collection-1:8080\",job=\"qs-collection-server\"} 1 1776030000000\n" +
		"qs_runtime_component_ready{component=\"collection-server\",instance=\"collection-2:8080\",job=\"qs-collection-server\"} 1 1776030000000\n"
	if err := os.WriteFile(path, []byte(metrics), 0o600); err != nil {
		t.Fatal(err)
	}
	if check := checkFederatedMetricsReady(dir, "collection-metrics.txt", "collection metrics replicas", "collection-server", "EXPECTED_COLLECTION_REPLICAS", 2); check.Status != "PASS" {
		t.Fatalf("federated metrics check = %#v, want PASS", check)
	}

	direct := `qs_runtime_component_ready{component="collection-server"} 1` + "\n"
	if err := os.WriteFile(path, []byte(direct), 0o600); err != nil {
		t.Fatal(err)
	}
	if check := checkFederatedMetricsReady(dir, "collection-metrics.txt", "collection metrics replicas", "collection-server", "EXPECTED_COLLECTION_REPLICAS", 2); check.Status != "INVALID" {
		t.Fatalf("direct metrics check = %#v, want INVALID", check)
	}
}

func TestFederatedMetricsEvidencePrefersDockerDiscoveredSamples(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "worker-metrics.txt")
	metrics := "qs_runtime_component_ready{component=\"qs-worker\",exported_component=\"worker\",instance=\"qs-worker-runtime-1\",job=\"qs-worker\"} 1 1776030000000\n" +
		"qs_runtime_component_ready{component=\"qs-worker\",exported_component=\"worker\",instance=\"qs-worker-runtime-2\",job=\"qs-worker\"} 1 1776030000000\n" +
		"qs_runtime_component_ready{component=\"qs-worker\",exported_component=\"worker\",instance=\"qs-worker-runtime-3\",job=\"qs-worker\"} 1 1776030000000\n" +
		"qs_runtime_component_ready{component=\"worker\",instance=\"qs-worker:9092\",job=\"qs-worker\"} 1 1776030000000\n"
	if err := os.WriteFile(path, []byte(metrics), 0o600); err != nil {
		t.Fatal(err)
	}
	if check := checkFederatedMetricsReady(dir, "worker-metrics.txt", "worker metrics replicas", "worker", "EXPECTED_WORKER_REPLICAS", 3); check.Status != "PASS" {
		t.Fatalf("Docker-discovered worker metrics check = %#v, want PASS", check)
	}
}

func TestInterpretationDeltasTreatNewCounterAsStartingAtZero(t *testing.T) {
	after := []metricSample{
		{Name: "qs_interpretation_run_duration_seconds_count", Labels: map[string]string{"result": "success", "builder_identity": "factor-scoring"}, Value: 4},
	}
	completed, failed, byModel, ok := interpretationDeltas(nil, after)
	if !ok || completed == nil || *completed != 4 || failed == nil || *failed != 0 {
		t.Fatalf("interpretation deltas = completed=%v failed=%v ok=%v", completed, failed, ok)
	}
	if byModel["medical"] != 4 {
		t.Fatalf("completed model deltas = %#v, want medical=4", byModel)
	}
}

func TestEvidenceClassificationSeparatesUnhealthyFromMissing(t *testing.T) {
	unhealthy := PhaseEvidence{Checks: []EvidenceCheck{{Name: "worker readyz", Status: "FAIL"}}}
	if verdict := classifyEvidence(unhealthy, "service evidence"); verdict.Status != VerdictFail {
		t.Fatalf("unhealthy verdict = %#v, want FAIL", verdict)
	}
	missing := PhaseEvidence{Checks: []EvidenceCheck{{Name: "worker metrics", Status: "MISSING"}}}
	if verdict := classifyEvidence(missing, "service evidence"); verdict.Status != VerdictIncomplete {
		t.Fatalf("missing verdict = %#v, want INCOMPLETE", verdict)
	}
}

func TestTrafficIsolationEvidenceIsFailClosed(t *testing.T) {
	t.Setenv("PERF_ISOLATED_ENV", "")
	isolated, check := trafficIsolationEvidence()
	if isolated != nil || check.Status != "MISSING" {
		t.Fatalf("isolation = %v, check = %#v, want unknown/MISSING", isolated, check)
	}

	t.Setenv("PERF_ISOLATED_ENV", "true")
	isolated, check = trafficIsolationEvidence()
	if isolated == nil || !*isolated || check.Status != "PASS" {
		t.Fatalf("isolation = %v, check = %#v, want true/PASS", isolated, check)
	}
}

func TestPrometheusObservationWindowUsesMetricCaptureTimes(t *testing.T) {
	dir := t.TempDir()
	before := filepath.Join(dir, "before-apiserver-metrics.txt")
	after := filepath.Join(dir, "after-apiserver-metrics.txt")
	if err := os.WriteFile(before, []byte("qs_runtime_component_ready{component=\"apiserver\"} 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(after, []byte("qs_interpretation_run_duration_seconds_count{result=\"success\"} 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	started := time.Unix(1_700_000_000, 0)
	if err := os.Chtimes(before, started, started); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(after, started.Add(12*time.Second), started.Add(12*time.Second)); err != nil {
		t.Fatal(err)
	}

	window, ok := prometheusObservationWindow(dir, "before", dir, "after")
	if !ok || window != 12 {
		t.Fatalf("window = %v, %v, want 12s", window, ok)
	}
}

func TestNSQDepthUsesChannelWorkWithoutDoubleCountingTopicDepth(t *testing.T) {
	t.Setenv("PERF_NSQ_TOPICS", "")
	dir := t.TempDir()
	path := filepath.Join(dir, "nsqd.json")
	payload := map[string]any{"topics": []any{
		map[string]any{
			"topic_name": "assessment-events", "depth": 10.0,
			"channels": []any{
				map[string]any{"channel_name": "worker", "depth": 3.0},
				map[string]any{"channel_name": "projection", "depth": 2.0},
			},
		},
		map[string]any{"topic_name": "unbound-events", "depth": 4.0, "channels": []any{}},
	}}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	depth, ok := readNSQDepth(path)
	if !ok || depth != 9 {
		t.Fatalf("depth = %v, %v, want channel depths 3+2 plus unbound topic depth 4", depth, ok)
	}
}

func TestNSQDepthCanBeScopedToPerfTopics(t *testing.T) {
	t.Setenv("PERF_NSQ_TOPICS", "assessment-events")
	dir := t.TempDir()
	path := filepath.Join(dir, "nsqd.json")
	payload := `{"topics":[{"topic_name":"assessment-events","depth":2,"channels":[]},{"topic_name":"other","depth":7,"channels":[]}]}`
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}

	depth, ok := readNSQDepth(path)
	if !ok || depth != 2 {
		t.Fatalf("depth = %v, %v, want scoped depth 2", depth, ok)
	}
}
