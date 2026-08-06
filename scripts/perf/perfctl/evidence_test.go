package main

import (
	"os"
	"path/filepath"
	"testing"
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
