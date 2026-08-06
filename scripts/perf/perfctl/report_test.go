package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPhaseReportIncludesThreeDimensions(t *testing.T) {
	raw := rawSummary{Metrics: map[string]map[string]any{
		"iterations":                         {"count": float64(100), "rate": float64(10)},
		"http_reqs":                          {"count": float64(120), "rate": float64(12)},
		"dropped_iterations":                 {"count": float64(0), "rate": float64(0), "thresholds": map[string]any{"count==0": false}},
		"answer_submit_accepted":             {"count": float64(100), "rate": float64(10)},
		"medical_answer_submit_success_rate": {"passes": float64(99), "fails": float64(1), "value": float64(0.99)},
		"medical_answer_submit_duration":     {"med": float64(20), "p(95)": float64(40), "p(99)": float64(70), "max": float64(90), "avg": float64(25)},
		"answer_submit_timeout":              {"count": float64(1), "rate": float64(0.1)},
		"http_timeout_total":                 {"count": float64(1), "rate": float64(0.1)},
	}}
	completed := 100.0
	evidence := PhaseEvidence{Complete: true, CompletedCountDelta: &completed, CompletedCountDeltaByModel: map[string]float64{"medical": 100}}
	spec := phaseSpec{ID: "test", Profile: "test", TargetQPS: 10, Duration: "10s", ThresholdTier: "none"}
	phase := buildPhaseSummary(spec, map[string]float64{"medicalSubmit": 10}, raw, evidence, time.Now(), time.Now(), 0)
	if phase.Throughput.AcceptedTPS.Value == nil || *phase.Throughput.AcceptedTPS.Value != 10 {
		t.Fatalf("accepted TPS = %#v", phase.Throughput.AcceptedTPS)
	}
	if phase.Throughput.CompletedTPS.Value == nil || *phase.Throughput.CompletedTPS.Value != 10 {
		t.Fatalf("completed TPS = %#v", phase.Throughput.CompletedTPS)
	}
	if value := phase.Throughput.AcceptedTPSByModel["medical"].Value; value == nil || *value != 9.9 {
		t.Fatalf("medical accepted TPS = %#v", phase.Throughput.AcceptedTPSByModel)
	}
	if value := phase.Throughput.CompletedTPSByModel["medical"].Value; value == nil || *value != 10 {
		t.Fatalf("medical completed TPS = %#v", phase.Throughput.CompletedTPSByModel)
	}
	if len(phase.Latency) != 1 || phase.Latency[0].P50.Value == nil || *phase.Latency[0].P50.Value != 20 || *phase.Latency[0].Max.Value != 90 {
		t.Fatalf("latency = %#v", phase.Latency)
	}
	if len(phase.Correctness) != 1 || phase.Correctness[0].TimeoutRate.Value == nil || *phase.Correctness[0].TimeoutRate.Value != 0.01 {
		t.Fatalf("correctness = %#v", phase.Correctness)
	}
	if *phase.Correctness[0].ErrorRate.Value < *phase.Correctness[0].TimeoutRate.Value {
		t.Fatalf("timeout must be a subset of error: %#v", phase.Correctness[0])
	}
	run := RunSummary{SchemaVersion: reportSchemaVersion, Run: RunMetadata{ID: "test"}, Verdict: Verdict{Status: VerdictPass}, Phases: []PhaseSummary{phase}}
	populateRunViews(&run)
	if run.Throughput["test"].AcceptedTPS.Value == nil || len(run.Latency["test"]) != 1 || len(run.Correctness["test"]) != 1 {
		t.Fatalf("root report views are incomplete: %#v", run)
	}
	markdown := renderRunMarkdown(run)
	for _, want := range []string{"吞吐与处理能力", "P50", "P95", "P99", "最大耗时", "最终失败率", "分层重试"} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown missing %q\n%s", want, markdown)
		}
	}
}

func TestRawFixtureKeepsConsoleMarkdownAndJSONConsistent(t *testing.T) {
	rawBytes, err := os.ReadFile("testdata/raw-summary.json")
	if err != nil {
		t.Fatal(err)
	}
	var raw rawSummary
	if err := json.Unmarshal(rawBytes, &raw); err != nil {
		t.Fatal(err)
	}
	completed := 40.0
	phase := buildPhaseSummary(
		phaseSpec{ID: "fixture", Profile: "fixture", TargetQPS: 10, Duration: "10s", ThresholdTier: "none"},
		map[string]float64{"medicalQuery": 6, "medicalSubmit": 4}, raw,
		PhaseEvidence{Complete: true, CompletedCountDelta: &completed, CompletedCountDeltaByModel: map[string]float64{"medical": 40}},
		time.Now(), time.Now(), 0,
	)
	if len(phase.Latency) != 2 || phase.Latency[0].Operation != "medical_model_query" || phase.Latency[1].Operation != "medical_submit" {
		t.Fatalf("operation ordering drifted: %#v", phase.Latency)
	}
	run := RunSummary{SchemaVersion: reportSchemaVersion, Run: RunMetadata{ID: "fixture"}, Verdict: phase.Verdict, Phases: []PhaseSummary{phase}}
	populateRunViews(&run)
	jsonBytes, err := json.Marshal(run)
	if err != nil {
		t.Fatal(err)
	}
	markdown := renderRunMarkdown(run)
	console := renderPhaseConsole(phase)
	for _, want := range []string{"throughput:", "latency:", "correctness:", "retry:"} {
		if !strings.Contains(console, want) {
			t.Fatalf("console missing %q: %s", want, console)
		}
	}
	for label, output := range map[string]string{"json": string(jsonBytes), "markdown": markdown, "console": console} {
		want := "10.00"
		if label == "json" {
			want = `"value":10`
		}
		if !strings.Contains(output, want) {
			t.Fatalf("%s output is missing actual 10 QPS: %s", label, output)
		}
	}
}

func TestRetryRateIsNAWhenInitialDenominatorIsZero(t *testing.T) {
	before := []metricSample{
		{Name: "qs_retry_layer_attempt_total", Labels: map[string]string{"layer": "business", "attempt_class": "initial"}, Value: 0},
		{Name: "qs_retry_layer_attempt_total", Labels: map[string]string{"layer": "business", "attempt_class": "retry"}, Value: 0},
	}
	after := append([]metricSample(nil), before...)
	metrics, _ := retryEvidence(before, after)
	if metrics[0].RetryRate.Value != nil {
		t.Fatalf("retry rate = %#v, want N/A", metrics[0].RetryRate)
	}
}

func TestMissingDroppedIterationsMetricMeansZeroWhenIterationsExist(t *testing.T) {
	raw := rawSummary{Metrics: map[string]map[string]any{
		"iterations": {"count": float64(100), "rate": float64(10)},
		"http_reqs":  {"count": float64(100), "rate": float64(10)},
	}}
	phase := buildPhaseSummary(
		phaseSpec{ID: "no-drops", Profile: "test", TargetQPS: 10, Duration: "10s", ThresholdTier: "protection"},
		map[string]float64{}, raw, PhaseEvidence{Complete: true}, time.Now(), time.Now(), 0,
	)
	if phase.Throughput.BusinessQPS.Dropped.Value == nil || *phase.Throughput.BusinessQPS.Dropped.Value != 0 {
		t.Fatalf("dropped iterations = %#v, want observed zero", phase.Throughput.BusinessQPS.Dropped)
	}
	if phase.Verdict.Status != VerdictPass {
		t.Fatalf("verdict = %#v, want PASS", phase.Verdict)
	}
}
