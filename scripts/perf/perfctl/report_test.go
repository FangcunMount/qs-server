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
		"iterations":                                {"count": float64(100), "rate": float64(10)},
		"http_reqs":                                 {"count": float64(120), "rate": float64(12)},
		"dropped_iterations":                        {"count": float64(0), "rate": float64(0), "thresholds": map[string]any{"count==0": false}},
		"answer_submit_accepted":                    {"count": float64(100), "rate": float64(10)},
		"chain_probe_accepted":                      {"count": float64(10), "rate": float64(1)},
		"medical_answer_submit_success_rate":        {"passes": float64(99), "fails": float64(1), "value": float64(0.99)},
		"medical_answer_submit_duration":            {"med": float64(20), "p(90)": float64(30), "p(95)": float64(40), "p(99)": float64(70), "max": float64(90), "avg": float64(25)},
		"answer_submit_timeout":                     {"count": float64(1), "rate": float64(0.1)},
		"answer_submit_timeout{model_type:medical}": {"count": float64(1), "rate": float64(0.1)},
		"http_timeout_total":                        {"count": float64(1), "rate": float64(0.1)},
	}}
	completed, expected, noAssessmentRequired := 100.0, 100.0, 10.0
	evidence := PhaseEvidence{
		Complete: true, CompletionWindow: measured(floatPtr(20), "seconds", "test"),
		CompletedCountDelta: &completed, CompletedCountDeltaByModel: map[string]float64{"medical": 100},
		ExpectedCompletionCountDelta: &expected, NoAssessmentRequiredCountDelta: &noAssessmentRequired,
	}
	spec := phaseSpec{ID: "test", Profile: "test", TargetQPS: 10, Duration: "10s", ThresholdTier: "none"}
	phase := buildPhaseSummary(spec, map[string]float64{"medicalSubmit": 10}, raw, evidence, time.Now(), time.Now(), 0)
	if phase.Throughput.AcceptedTPS.Value == nil || *phase.Throughput.AcceptedTPS.Value != 11 {
		t.Fatalf("accepted TPS = %#v", phase.Throughput.AcceptedTPS)
	}
	if phase.Throughput.CompletedTPS.Value == nil || *phase.Throughput.CompletedTPS.Value != 5 {
		t.Fatalf("completed TPS = %#v", phase.Throughput.CompletedTPS)
	}
	if value := phase.Throughput.AcceptedTPSByModel["medical"].Value; value == nil || *value != 9.9 {
		t.Fatalf("medical accepted TPS = %#v", phase.Throughput.AcceptedTPSByModel)
	}
	if value := phase.Throughput.CompletedTPSByModel["medical"].Value; value == nil || *value != 5 {
		t.Fatalf("medical completed TPS = %#v", phase.Throughput.CompletedTPSByModel)
	}
	if value := phase.Throughput.FinalCompletionRate.Value; value == nil || *value != 1 {
		t.Fatalf("final completion rate = %#v, want completed/assessment_created = 1", phase.Throughput.FinalCompletionRate)
	}
	if value := phase.Throughput.NoAssessmentRequired.Value; value == nil || *value != 10 {
		t.Fatalf("no assessment required = %#v", phase.Throughput.NoAssessmentRequired)
	}
	if len(phase.Latency) != 1 || phase.Latency[0].P50.Value == nil || *phase.Latency[0].P50.Value != 20 || phase.Latency[0].P90.Value == nil || *phase.Latency[0].P90.Value != 30 || *phase.Latency[0].Max.Value != 90 {
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
	for _, want := range []string{"三维结果总览", "1. 吞吐与处理能力", "目标 QPS", "受理 TPS", "2. 时延与响应体验", "P50", "P90", "P95", "P99", "3. 可靠性与正确性", "成功（数量 / 比例）", "重试率（按层级）"} {
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
	completed, expected := 40.0, 40.0
	phase := buildPhaseSummary(
		phaseSpec{ID: "fixture", Profile: "fixture", TargetQPS: 10, Duration: "10s", ThresholdTier: "none"},
		map[string]float64{"medicalQuery": 6, "medicalSubmit": 4}, raw,
		PhaseEvidence{Complete: true, CompletedCountDelta: &completed, CompletedCountDeltaByModel: map[string]float64{"medical": 40}, ExpectedCompletionCountDelta: &expected},
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
	if !strings.Contains(string(jsonBytes), `"p90":{"value":25`) {
		t.Fatalf("JSON output is missing normalized P90: %s", jsonBytes)
	}
	markdown := renderRunMarkdown(run)
	console := renderPhaseConsole(phase)
	for _, want := range []string{"结论依据", "1. 吞吐与处理能力", "| 类别", "QPS", "TPS", "2. 时延与响应体验", "| 操作", "P50", "P90", "P95", "P99", "3. 可靠性与正确性", "成功率", "错误率", "超时率", "| 重试层级", "重试率"} {
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

func TestConsoleTableAlignsChineseHeadersAndNumericColumns(t *testing.T) {
	var output strings.Builder
	writeConsoleTable(&output, "  ",
		[]string{"操作", "样本", "成功率"},
		[][]string{{"medical_submit", "1797", "100.00%"}, {"人格报告", "9", "99.67%"}},
		[]bool{false, true, true},
	)
	lines := strings.Split(strings.TrimSuffix(output.String(), "\n"), "\n")
	if len(lines) != 6 {
		t.Fatalf("table lines = %d, want 6:\n%s", len(lines), output.String())
	}
	wantWidth := consoleDisplayWidth(lines[0])
	for _, line := range lines[1:] {
		if got := consoleDisplayWidth(line); got != wantWidth {
			t.Fatalf("table width = %d, want %d for %q:\n%s", got, wantWidth, line, output.String())
		}
	}
	if !strings.Contains(output.String(), "| 操作") || !strings.Contains(output.String(), "|    9 |  99.67% |") {
		t.Fatalf("table is not readable or right-aligned:\n%s", output.String())
	}
}

func TestNativeDiagnosticsAppendWebSocketExecutionAndScenarios(t *testing.T) {
	raw := rawSummary{
		Metrics: map[string]map[string]any{
			"ws_connecting": {
				"avg": float64(164.11), "min": float64(81.41), "med": float64(147.72),
				"p(90)": float64(219.26), "p(95)": float64(236.98), "p(99)": float64(390.09), "max": float64(757.02),
			},
			"ws_msgs_received": {"count": float64(5403), "rate": float64(17.492972)},
			"ws_msgs_sent":     {"count": float64(2702), "rate": float64(8.748105)},
			"ws_session_duration": {
				"avg": float64(243.43), "min": float64(137.24), "med": float64(236.06),
				"p(90)": float64(302.65), "p(95)": float64(327.12), "p(99)": float64(479.22), "max": float64(832.62),
			},
			"ws_sessions":                       {"count": float64(2702), "rate": float64(8.748105)},
			"report_status_failed":              {"count": float64(9), "rate": float64(0.03)},
			"report_ws_capacity_rejected_total": {"count": float64(9), "rate": float64(0.03)},
			"iterations":                        {"count": float64(18005), "rate": float64(58.29)},
			"dropped_iterations":                {"count": float64(1)},
			"dropped_iterations{scenario:medical_model_query}":     {"count": float64(1)},
			"dropped_iterations{scenario:medical_report_ws_query}": {"count": float64(0)},
			"vus":     {"value": float64(0), "max": float64(42)},
			"vus_max": {"max": float64(106)},
		},
		State: &rawSummaryState{TestRunDurationMS: 308900},
		Scenarios: map[string]rawScenario{
			"medical_model_query":     {Executor: "constant-arrival-rate", Rate: 12, TimeUnit: "1s", Duration: "5m0s", PreAllocatedVUs: 9, MaxVUs: 450},
			"medical_report_ws_query": {Executor: "constant-arrival-rate", Rate: 5, TimeUnit: "1s", Duration: "5m0s", PreAllocatedVUs: 20, MaxVUs: 200},
		},
	}
	console := renderK6NativeDiagnostics(PhaseSummary{}, raw)
	for _, want := range []string{
		"K6 原生运行诊断", "WEBSOCKET", "ws_connecting", "avg=164.11ms", "p(99)=390.09ms",
		"WEBSOCKET / 失败分类", "report_status_failed", "report_ws_capacity_rejected_total",
		"ws_msgs_received", "5403  17.492972/s", "running (5m8.9s)", "0/106 VUs (peak=42)",
		"18005 complete", "1 dropped", "interrupted=N/A", "medical_model_query", "pre/max VUs=9/450", "12.00 iters/s", "dropped=1",
	} {
		if !strings.Contains(console, want) {
			t.Fatalf("native diagnostics missing %q:\n%s", want, console)
		}
	}
}

func TestNativeDiagnosticsGroupEachActiveInterface(t *testing.T) {
	latency := func(operation string, samples int64) LatencyMetric {
		return LatencyMetric{Operation: operation, Samples: samples}
	}
	correctness := func(operation string, attempts, success, failures, timeouts int64) CorrectnessMetric {
		return CorrectnessMetric{
			Operation: operation, Attempts: attempts,
			SuccessCount: int64Ptr(success), ErrorCount: int64Ptr(failures), TimeoutCount: int64Ptr(timeouts),
			SuccessRate: measured(floatPtr(float64(success)/float64(attempts)), "ratio", "test"),
			ErrorRate:   measured(floatPtr(float64(failures)/float64(attempts)), "ratio", "test"),
			TimeoutRate: measured(floatPtr(float64(timeouts)/float64(attempts)), "ratio", "test"),
		}
	}
	trend := func(median float64) map[string]any {
		return map[string]any{
			"avg": median + 1, "min": median - 1, "med": median, "p(90)": median + 2,
			"p(95)": median + 3, "p(99)": median + 4, "max": median + 5,
		}
	}
	phase := PhaseSummary{
		Duration: "6s",
		Latency: []LatencyMetric{
			latency("medical_model_query", 12),
			latency("medical_submit", 6),
			latency("personality_session", 3),
			latency("medical_report_ws_connect", 5),
			latency("statistics_overview", 4),
			latency("async_chain_probe", 1),
		},
		Correctness: []CorrectnessMetric{
			correctness("medical_model_query", 12, 11, 1, 0),
			correctness("medical_submit", 6, 6, 0, 0),
			correctness("personality_session", 3, 3, 0, 0),
			correctness("medical_report_ws_connect", 5, 5, 0, 0),
			correctness("statistics_overview", 4, 4, 0, 0),
			correctness("async_chain_probe", 1, 1, 0, 0),
		},
	}
	raw := rawSummary{Metrics: map[string]map[string]any{
		"medical_model_query_duration":                   trend(20),
		"medical_model_query_5xx":                        {"count": float64(1), "rate": float64(1.0 / 6)},
		"medical_answer_submit_duration":                 trend(30),
		"personality_session_duration":                   trend(40),
		"report_ws_connect_duration{model_type:medical}": trend(50),
		"statistics_overview_duration":                   trend(60),
		"report_generated_latency":                       trend(70),
		"chain_probe_started":                            {"count": float64(1), "rate": float64(1.0 / 6)},
		"chain_probe_accepted":                           {"count": float64(1), "rate": float64(1.0 / 6)},
		"chain_probe_completed":                          {"count": float64(1), "rate": float64(1.0 / 6)},
		"chain_probe_failed":                             {"count": float64(0), "rate": float64(0)},
		"chain_probe_timeout":                            {"count": float64(0), "rate": float64(0)},
		"chain_probe_final_failed":                       {"count": float64(0), "rate": float64(0)},
		"chain_probe_poll_requests":                      {"count": float64(2), "rate": float64(2.0 / 6)},
	}}

	console := renderK6NativeDiagnostics(phase, raw)
	ordered := []string{
		"QUERY / 查询", "SUBMIT / 提交", "SESSION / 会话", "WEBSOCKET / 报告订阅",
		"STATISTICS / 统计", "ASYNC CHAIN / 异步链路", "WEBSOCKET / K6 内置", "EXECUTION",
	}
	last := -1
	for _, title := range ordered {
		index := strings.Index(console, title)
		if index <= last {
			t.Fatalf("section %q is missing or out of order:\n%s", title, console)
		}
		last = index
	}
	for _, want := range []string{
		"medical_model_query_duration", "12  2.000000/s", "success=11 error=1 timeout=0",
		"failure_breakdown", "4xx=0 5xx=1 transport=0", "medical_answer_submit_duration",
		"report_ws_connect_duration{model_type:medical}", "statistics_overview_duration",
		"report_generated_latency", "chain_probe_poll_requests", "2  0.333333/s",
	} {
		if !strings.Contains(console, want) {
			t.Fatalf("interface diagnostics missing %q:\n%s", want, console)
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

func TestThroughputUsesPlannedTrafficWindowInsteadOfSetupInclusiveK6Rate(t *testing.T) {
	raw := rawSummary{Metrics: map[string]map[string]any{
		// The k6 rate covers a 60s process lifetime: 30s setup + 30s traffic.
		// Throughput must instead use the 30s planned traffic window.
		"iterations":             {"count": float64(120), "rate": float64(2)},
		"ws_sessions":            {"count": float64(30), "rate": float64(0.5)},
		"answer_submit_accepted": {"count": float64(30), "rate": float64(0.5)},
	}}

	throughput := throughputResults(
		phaseSpec{TargetQPS: 4, Duration: "30s"},
		raw,
		PhaseEvidence{},
	)

	if got := throughput.BusinessQPS.Actual.Value; got == nil || *got != 4 {
		t.Fatalf("actual QPS = %#v, want 4", throughput.BusinessQPS.Actual)
	}
	if got := throughput.BusinessQPS.TargetAttainment.Value; got == nil || *got != 1 {
		t.Fatalf("target attainment = %#v, want 1", throughput.BusinessQPS.TargetAttainment)
	}
	if got := throughput.WSSessionsPerSecond.Value; got == nil || *got != 1 {
		t.Fatalf("WS sessions/s = %#v, want 1", throughput.WSSessionsPerSecond)
	}
	if got := throughput.AcceptedTPS.Value; got == nil || *got != 1 {
		t.Fatalf("accepted TPS = %#v, want 1", throughput.AcceptedTPS)
	}
}

func TestOperationResultsIncludeGlobalHTTPTimeoutAndErrorRates(t *testing.T) {
	raw := rawSummary{Metrics: map[string]map[string]any{
		"http_reqs":          {"count": float64(100), "rate": float64(10)},
		"http_req_failed":    {"passes": float64(2), "fails": float64(98), "value": float64(0.02)},
		"http_timeout_total": {"count": float64(1)},
	}}

	_, correctness := operationResults(map[string]float64{}, raw)
	if len(correctness) != 1 || correctness[0].Operation != "global_http" {
		t.Fatalf("correctness = %#v, want global_http", correctness)
	}
	global := correctness[0]
	if global.SuccessRate.Value == nil || *global.SuccessRate.Value != 0.98 || global.ErrorRate.Value == nil || *global.ErrorRate.Value != 0.02 {
		t.Fatalf("global correctness = %#v", global)
	}
	if global.TimeoutCount == nil || *global.TimeoutCount != 1 || global.TimeoutRate.Value == nil || *global.TimeoutRate.Value != 0.01 {
		t.Fatalf("global timeout = %#v", global)
	}
}

func TestTaggedMetricLookupDoesNotFallbackToGlobalMetric(t *testing.T) {
	raw := rawSummary{Metrics: map[string]map[string]any{
		"report_ws_message_success_rate": {"passes": float64(100), "fails": float64(0), "value": float64(1)},
	}}
	if metric := findMetric(raw, "report_ws_message_success_rate", []string{"model_type:medical"}); metric != nil {
		t.Fatalf("tagged metric = %#v, want nil when the tagged evidence is missing", metric)
	}
}

func TestWebSocketSubscribeLatencyIsDiagnosticOnly(t *testing.T) {
	raw := rawSummary{Metrics: map[string]map[string]any{
		"report_ws_connect_duration":                   {"med": float64(100), "p(90)": float64(180), "p(95)": float64(200), "p(99)": float64(300), "max": float64(400), "avg": float64(120)},
		"report_ws_first_message_latency":              {"med": float64(180), "p(90)": float64(250), "p(95)": float64(280), "p(99)": float64(380), "max": float64(480), "avg": float64(200)},
		"report_ws_subscribe_to_first_message_latency": {"med": float64(80), "p(90)": float64(90), "p(95)": float64(100), "p(99)": float64(120), "max": float64(180), "avg": float64(85)},
		"report_ws_connect_success_rate":               {"passes": float64(10), "fails": float64(0), "value": float64(1)},
		"report_ws_message_success_rate":               {"passes": float64(10), "fails": float64(0), "value": float64(1)},
	}}
	latencies, correctness := operationResults(map[string]float64{"report": 1}, raw)
	if len(latencies) != 3 {
		t.Fatalf("WS latencies = %#v, want connect, end-to-end first message and subscribe-to-message", latencies)
	}
	for _, item := range correctness {
		if item.Operation == "report_ws_subscribe_to_message" {
			t.Fatalf("diagnostic-only WS stage unexpectedly emitted correctness row: %#v", item)
		}
	}
}

func TestModelWebSocketSubscribeLatencyUsesDedicatedSummaryMetric(t *testing.T) {
	raw := rawSummary{Metrics: map[string]map[string]any{
		"medical_report_ws_subscribe_to_first_message_latency": {"med": float64(70), "p(90)": float64(90), "p(95)": float64(110), "p(99)": float64(140)},
		"report_ws_message_success_rate{model_type:medical}":   {"passes": float64(10), "fails": float64(0), "value": float64(1)},
	}}

	latencies, correctness := operationResults(map[string]float64{"medicalWaitReport": 1}, raw)
	var found *LatencyMetric
	for index := range latencies {
		if latencies[index].Operation == "medical_report_ws_subscribe_to_message" {
			found = &latencies[index]
			break
		}
	}
	if found == nil || found.P95.Value == nil || *found.P95.Value != 110 {
		t.Fatalf("medical subscribe latency = %#v, want dedicated model metric", found)
	}
	for _, item := range correctness {
		if item.Operation == "medical_report_ws_subscribe_to_message" {
			t.Fatalf("diagnostic model latency unexpectedly emitted correctness row: %#v", item)
		}
	}
}
