package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"
)

type operationSpec struct {
	ID            string
	QPSKey        string
	TrendMetric   string
	SuccessMetric string
	ErrorMetric   string
	TimeoutMetric string
	TrendTags     []string
	ResultTags    []string
	TimeoutTags   []string
}

var operationSpecs = []operationSpec{
	{ID: "catalog_query", QPSKey: "query", TrendMetric: "questionnaire_query_duration", ErrorMetric: "http_req_failed", TimeoutMetric: "questionnaire_query_timeout", ResultTags: []string{"endpoint:questionnaire_query"}},
	{ID: "medical_model_query", QPSKey: "medicalQuery", TrendMetric: "medical_model_query_duration", ErrorMetric: "http_req_failed", TimeoutMetric: "medical_model_query_timeout", ResultTags: []string{"endpoint:medical_model_query"}},
	{ID: "personality_model_query", QPSKey: "personalityQuery", TrendMetric: "personality_model_query_duration", ErrorMetric: "http_req_failed", TimeoutMetric: "personality_model_query_timeout", ResultTags: []string{"endpoint:personality_model_query"}},
	{ID: "questionnaire_query", QPSKey: "questionnaireQuery", TrendMetric: "questionnaire_query_duration", ErrorMetric: "http_req_failed", TimeoutMetric: "questionnaire_query_timeout", ResultTags: []string{"endpoint:questionnaire_query"}},
	{ID: "personality_questionnaire_query", QPSKey: "personalityQuestionnaireQuery", TrendMetric: "personality_questionnaire_query_duration", ErrorMetric: "http_req_failed", TimeoutMetric: "personality_questionnaire_query_timeout", ResultTags: []string{"endpoint:personality_questionnaire_query"}},
	{ID: "personality_session", QPSKey: "personalitySession", TrendMetric: "personality_session_duration", ErrorMetric: "http_req_failed", TimeoutMetric: "personality_session_timeout", ResultTags: []string{"endpoint:personality_session"}},
	{ID: "answersheet_submit", QPSKey: "submit", TrendMetric: "answer_submit_duration", SuccessMetric: "answer_submit_success_rate", TimeoutMetric: "answer_submit_timeout"},
	{ID: "medical_submit", QPSKey: "medicalSubmit", TrendMetric: "medical_answer_submit_duration", SuccessMetric: "medical_answer_submit_success_rate", TimeoutMetric: "answer_submit_timeout", TimeoutTags: []string{"model_type:medical"}},
	{ID: "personality_submit", QPSKey: "personalitySubmit", TrendMetric: "personality_answer_submit_duration", SuccessMetric: "personality_answer_submit_success_rate", TimeoutMetric: "answer_submit_timeout", TimeoutTags: []string{"model_type:personality"}},
	{ID: "report_ws_connect", QPSKey: "report", TrendMetric: "report_ws_connect_duration", SuccessMetric: "report_ws_connect_success_rate", TimeoutMetric: "report_ws_timeout_total"},
	{ID: "report_ws_message", QPSKey: "report", TrendMetric: "report_ws_first_message_latency", SuccessMetric: "report_ws_message_success_rate", TimeoutMetric: "report_ws_timeout_total"},
	{ID: "medical_report_ws_connect", QPSKey: "medicalWaitReport", TrendMetric: "report_ws_connect_duration", SuccessMetric: "report_ws_connect_success_rate", TimeoutMetric: "report_ws_timeout_total", TrendTags: []string{"model_type:medical"}, ResultTags: []string{"model_type:medical"}, TimeoutTags: []string{"model_type:medical"}},
	{ID: "medical_report_ws_message", QPSKey: "medicalWaitReport", TrendMetric: "report_ws_first_message_latency", SuccessMetric: "report_ws_message_success_rate", TimeoutMetric: "report_ws_timeout_total", TrendTags: []string{"model_type:medical"}, ResultTags: []string{"model_type:medical"}, TimeoutTags: []string{"model_type:medical"}},
	{ID: "behavior_report_ws_connect", QPSKey: "behaviorWaitReport", TrendMetric: "report_ws_connect_duration", SuccessMetric: "report_ws_connect_success_rate", TimeoutMetric: "report_ws_timeout_total", TrendTags: []string{"model_type:behavior"}, ResultTags: []string{"model_type:behavior"}, TimeoutTags: []string{"model_type:behavior"}},
	{ID: "behavior_report_ws_message", QPSKey: "behaviorWaitReport", TrendMetric: "report_ws_first_message_latency", SuccessMetric: "report_ws_message_success_rate", TimeoutMetric: "report_ws_timeout_total", TrendTags: []string{"model_type:behavior"}, ResultTags: []string{"model_type:behavior"}, TimeoutTags: []string{"model_type:behavior"}},
	{ID: "personality_report_ws_connect", QPSKey: "personalityWaitReport", TrendMetric: "report_ws_connect_duration", SuccessMetric: "report_ws_connect_success_rate", TimeoutMetric: "report_ws_timeout_total", TrendTags: []string{"model_type:personality"}, ResultTags: []string{"model_type:personality"}, TimeoutTags: []string{"model_type:personality"}},
	{ID: "personality_report_ws_message", QPSKey: "personalityWaitReport", TrendMetric: "report_ws_first_message_latency", SuccessMetric: "report_ws_message_success_rate", TimeoutMetric: "report_ws_timeout_total", TrendTags: []string{"model_type:personality"}, ResultTags: []string{"model_type:personality"}, TimeoutTags: []string{"model_type:personality"}},
	{ID: "statistics_overview", QPSKey: "stats", TrendMetric: "statistics_overview_duration", SuccessMetric: "statistics_overview_success_rate", TimeoutMetric: "statistics_overview_timeout"},
	{ID: "statistics_content_batch", QPSKey: "stats", TrendMetric: "statistics_content_batch_duration", SuccessMetric: "statistics_content_batch_success_rate", TimeoutMetric: "statistics_content_batch_timeout"},
}

func readRawSummary(path string) (rawSummary, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return rawSummary{}, err
	}
	var result rawSummary
	if err := json.Unmarshal(raw, &result); err != nil {
		return rawSummary{}, err
	}
	if result.Metrics == nil {
		return rawSummary{}, fmt.Errorf("summary has no metrics")
	}
	return result, nil
}

func buildPhaseSummary(spec phaseSpec, qps map[string]float64, raw rawSummary, evidence PhaseEvidence, startedAt, finishedAt time.Time, k6Exit int) PhaseSummary {
	phase := PhaseSummary{
		ID: spec.ID, Profile: spec.Profile, TargetQPS: spec.TargetQPS, Duration: spec.Duration,
		ActualDuration: measured(floatPtr(finishedAt.Sub(startedAt).Seconds()), "seconds", "orchestrator:phase_wall_time"),
		ThresholdTier:  spec.ThresholdTier, StartedAt: startedAt, FinishedAt: finishedAt,
		Evidence: evidence,
	}
	phase.Thresholds = thresholdResults(raw)
	phase.Latency, phase.Correctness = operationResults(qps, raw)
	phase.Throughput = throughputResults(spec, raw, evidence)
	phase.Retry = retryResults(raw, evidence)
	phase.QueueWait = append(make([]QueueWaitMetric, 0, len(evidence.QueueWait)+1), evidence.QueueWait...)
	if metric := findMetric(raw, "submit_to_assessment_latency", nil); metric != nil {
		phase.QueueWait = append(phase.QueueWait, QueueWaitMetric{
			Layer: "submit_to_assessment",
			Wait:  Measurement{Value: metricNumberPtr(metric, "p(95)"), Unit: "ms", Source: "k6:submit_to_assessment_latency", Note: "端到端等待代理值，不等同于纯队列驻留时间"},
		})
	}
	phase.Verdict = evaluatePhase(spec, phase, raw, k6Exit)
	if phase.Verdict.Status == VerdictPass && !evidence.Complete {
		phase.Verdict = classifyEvidence(evidence, "service evidence")
	}
	return phase
}

func throughputResults(spec phaseSpec, raw rawSummary, evidence PhaseEvidence) Throughput {
	iterations := findMetric(raw, "iterations", nil)
	loadWindowSeconds := metricWindowSeconds(iterations)
	httpReqs := findMetric(raw, "http_reqs", nil)
	wsSessions := findMetric(raw, "ws_sessions", nil)
	accepted := findMetric(raw, "answer_submit_accepted", nil)
	chainAccepted := findMetric(raw, "chain_probe_accepted", nil)
	chainStarted := findMetric(raw, "chain_probe_started", nil)
	chainPolls := findMetric(raw, "chain_probe_poll_requests", nil)
	dropped := findMetric(raw, "dropped_iterations", nil)
	actualQPS := metricNumberPtr(iterations, "rate")
	httpRPS := metricNumberPtr(httpReqs, "rate")
	acceptedCount := metricInt64(accepted, "count") + metricInt64(chainAccepted, "count")
	var acceptedTPS *float64
	if loadWindowSeconds > 0 {
		acceptedTPS = floatPtr(float64(acceptedCount) / loadWindowSeconds)
	}
	droppedCount := metricNumberPtr(dropped, "count")
	droppedNote := ""
	if droppedCount == nil && iterations != nil {
		droppedCount = floatPtr(0)
		droppedNote = "k6 omits dropped_iterations when no iteration was dropped"
	}
	target := float64(spec.TargetQPS)
	result := Throughput{
		BusinessQPS: BusinessQPS{
			Target:  measured(&target, "qps", "plan"),
			Actual:  Measurement{Value: actualQPS, Unit: "qps", Samples: metricInt64(iterations, "count"), Source: "k6:iterations"},
			Dropped: Measurement{Value: droppedCount, Unit: "iterations", Samples: metricInt64(dropped, "count"), Source: "k6:dropped_iterations", Note: droppedNote},
		},
		HTTPRPS:             Measurement{Value: httpRPS, Unit: "rps", Samples: metricInt64(httpReqs, "count"), Source: "k6:http_reqs"},
		WSSessionsPerSecond: Measurement{Value: metricNumberPtr(wsSessions, "rate"), Unit: "sessions/s", Samples: metricInt64(wsSessions, "count"), Source: "k6:ws_sessions"},
		AcceptedTPS:         Measurement{Value: acceptedTPS, Unit: "tps", Samples: acceptedCount, Source: "derived:(answer_submit_accepted+chain_probe_accepted)/k6_metric_window"},
		AcceptedTPSByModel:  map[string]Measurement{},
		CompletedTPSByModel: map[string]Measurement{},
	}
	for _, model := range []string{"medical", "personality"} {
		metric := findMetric(raw, model+"_answer_submit_success_rate", nil)
		modelAcceptedCount := metricInt64(metric, "passes")
		if metric != nil && loadWindowSeconds > 0 {
			result.AcceptedTPSByModel[model] = Measurement{Value: floatPtr(float64(modelAcceptedCount) / loadWindowSeconds), Unit: "tps", Samples: modelAcceptedCount, Source: "k6:" + model + "_answer_submit_success_rate.passes"}
		} else {
			result.AcceptedTPSByModel[model] = naMeasurement("tps", "k6:"+model+"_answer_submit_success_rate.passes", "model submit evidence unavailable")
		}
	}
	result.AcceptedTPSByModel["behavior"] = naMeasurement("tps", "not_applicable", "behavior submissions are not active in the canonical workload")
	if actualQPS != nil && target > 0 {
		result.BusinessQPS.TargetAttainment = measured(floatPtr(*actualQPS/target), "ratio", "derived:actual_qps/target_qps")
	} else {
		result.BusinessQPS.TargetAttainment = naMeasurement("ratio", "derived:actual_qps/target_qps", "actual QPS unavailable")
	}
	if httpRPS != nil && actualQPS != nil && *actualQPS > 0 {
		result.RequestAmplification = measured(floatPtr(*httpRPS / *actualQPS), "ratio", "derived:http_rps/business_qps")
	} else {
		result.RequestAmplification = naMeasurement("ratio", "derived:http_rps/business_qps", "RPS or QPS unavailable")
	}
	chainStartedCount := metricInt64(chainStarted, "count")
	if chainStartedCount > 0 {
		result.PollingAmplification = measured(floatPtr(float64(metricInt64(chainPolls, "count"))/float64(chainStartedCount)), "ratio", "derived:chain_probe_poll_requests/chain_probe_started")
	} else {
		result.PollingAmplification = naMeasurement("ratio", "derived:chain_probe_poll_requests/chain_probe_started", "chain probe is inactive")
	}
	completionWindowSeconds := 0.0
	if evidence.CompletionWindow.Value != nil {
		completionWindowSeconds = *evidence.CompletionWindow.Value
	}
	if evidence.CompletedCountDelta != nil && completionWindowSeconds > 0 {
		result.CompletedTPS = Measurement{
			Value: floatPtr(*evidence.CompletedCountDelta / completionWindowSeconds), Unit: "tps",
			Samples: int64(math.Round(*evidence.CompletedCountDelta)), Source: "prometheus:qs_interpretation_run_duration_seconds_count/snapshot_window",
		}
		if evidence.TrafficIsolated == nil {
			result.CompletedTPS.Note = "traffic isolation was not declared; not valid admission evidence"
		} else if !*evidence.TrafficIsolated {
			result.CompletedTPS.Note = "contains non-isolated concurrent traffic; not valid admission evidence"
		}
	} else {
		result.CompletedTPS = naMeasurement("tps", "prometheus:qs_interpretation_run_duration_seconds_count", "server completion evidence unavailable")
	}
	for _, model := range []string{"medical", "personality", "behavior"} {
		if count, ok := evidence.CompletedCountDeltaByModel[model]; ok && completionWindowSeconds > 0 {
			result.CompletedTPSByModel[model] = Measurement{
				Value: floatPtr(count / completionWindowSeconds), Unit: "tps", Samples: int64(math.Round(count)),
				Source: "prometheus:qs_interpretation_run_duration_seconds_count{builder_identity}/snapshot_window",
			}
		} else {
			result.CompletedTPSByModel[model] = naMeasurement("tps", "prometheus:qs_interpretation_run_duration_seconds_count{builder_identity}", "model completion evidence unavailable")
		}
	}
	if evidence.CompletedCountDelta != nil && acceptedCount > 0 {
		result.FinalCompletionRate = measured(floatPtr(*evidence.CompletedCountDelta/float64(acceptedCount)), "ratio", "derived:completed/accepted")
		if evidence.TrafficIsolated == nil {
			result.FinalCompletionRate.Note = "traffic isolation was not declared; not valid admission evidence"
		} else if !*evidence.TrafficIsolated {
			result.FinalCompletionRate.Note = "contains non-isolated concurrent traffic; not valid admission evidence"
		}
		if *result.FinalCompletionRate.Value > 1 {
			result.FinalCompletionRate.Note = "完成量包含窗口边界任务或并发流量；共享环境不得据此验收"
		}
	} else {
		result.FinalCompletionRate = naMeasurement("ratio", "derived:completed/accepted", "accepted or completed count unavailable")
	}
	return result
}

func metricWindowSeconds(metric map[string]any) float64 {
	count := metricNumber(metric, "count")
	rate := metricNumber(metric, "rate")
	if count <= 0 || rate <= 0 {
		return 0
	}
	return count / rate
}

func operationResults(qps map[string]float64, raw rawSummary) ([]LatencyMetric, []CorrectnessMetric) {
	latencies := make([]LatencyMetric, 0)
	correctness := make([]CorrectnessMetric, 0)
	for _, spec := range operationSpecs {
		if qps[spec.QPSKey] <= 0 {
			continue
		}
		trend := findMetric(raw, spec.TrendMetric, spec.TrendTags)
		rateMetricName := spec.SuccessMetric
		if rateMetricName == "" {
			rateMetricName = spec.ErrorMetric
		}
		rateMetric := findMetric(raw, rateMetricName, spec.ResultTags)
		attempts := metricInt64(rateMetric, "passes") + metricInt64(rateMetric, "fails")
		latencies = append(latencies, LatencyMetric{
			Operation: spec.ID,
			Samples:   attempts,
			P50:       trendMeasurement(trend, "med", spec.TrendMetric),
			P95:       trendMeasurement(trend, "p(95)", spec.TrendMetric),
			P99:       trendMeasurement(trend, "p(99)", spec.TrendMetric),
			Max:       trendMeasurement(trend, "max", spec.TrendMetric),
			Average:   trendMeasurement(trend, "avg", spec.TrendMetric),
		})
		correctness = append(correctness, correctnessForOperation(spec, attempts, rateMetric, raw))
	}
	if started := findMetric(raw, "chain_probe_started", nil); started != nil {
		attempts := metricInt64(started, "count")
		interpreted := metricInt64(findMetric(raw, "chain_probe_completed", nil), "count")
		failed := metricInt64(findMetric(raw, "chain_probe_final_failed", nil), "count")
		timeouts := metricInt64(findMetric(raw, "chain_probe_timeout", nil), "count")
		otherErrors := metricInt64(findMetric(raw, "chain_probe_failed", nil), "count")
		errorCount := maxInt64(failed+timeouts, otherErrors)
		correctness = append(correctness, correctnessFromCounts("async_chain_probe", attempts, interpreted, errorCount, timeouts, failed, "k6:chain_probe_*"))
		trend := findMetric(raw, "report_generated_latency", nil)
		latencies = append(latencies, LatencyMetric{
			Operation: "async_chain_probe", Samples: attempts,
			P50:     trendMeasurement(trend, "med", "report_generated_latency"),
			P95:     trendMeasurement(trend, "p(95)", "report_generated_latency"),
			P99:     trendMeasurement(trend, "p(99)", "report_generated_latency"),
			Max:     trendMeasurement(trend, "max", "report_generated_latency"),
			Average: trendMeasurement(trend, "avg", "report_generated_latency"),
		})
	}
	if global := globalHTTPCorrectness(raw); global != nil {
		correctness = append(correctness, *global)
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i].Operation < latencies[j].Operation })
	sort.Slice(correctness, func(i, j int) bool { return correctness[i].Operation < correctness[j].Operation })
	return latencies, correctness
}

func globalHTTPCorrectness(raw rawSummary) *CorrectnessMetric {
	httpReqs := findMetric(raw, "http_reqs", nil)
	failed := findMetric(raw, "http_req_failed", nil)
	if httpReqs == nil || failed == nil {
		return nil
	}
	attempts := metricInt64(httpReqs, "count")
	errors := metricInt64(failed, "passes")
	if errors == 0 && metricNumber(failed, "value") > 0 {
		errors = int64(math.Round(metricNumber(failed, "value") * float64(attempts)))
	}
	timeouts := metricInt64(findMetric(raw, "http_timeout_total", nil), "count")
	result := correctnessFromCounts("global_http", attempts, attempts-errors, errors, timeouts, 0, "k6:http_req_failed+http_timeout_total")
	result.FinalFailRate = naMeasurement("ratio", "not_applicable", "not an asynchronous terminal operation")
	return &result
}

func correctnessForOperation(spec operationSpec, attempts int64, rateMetric map[string]any, raw rawSummary) CorrectnessMetric {
	if attempts == 0 || rateMetric == nil {
		return CorrectnessMetric{
			Operation:     spec.ID,
			SuccessRate:   naMeasurement("ratio", "k6:"+spec.SuccessMetric, "no samples"),
			ErrorRate:     naMeasurement("ratio", "k6:"+spec.ErrorMetric, "no samples"),
			TimeoutRate:   naMeasurement("ratio", "k6:"+spec.TimeoutMetric, "no samples"),
			FinalFailRate: naMeasurement("ratio", "not_applicable", "not an asynchronous terminal operation"),
			Idempotency:   naMeasurement("ratio", "diagnose:submit-coalescing", "requires the idempotency diagnostic case"),
		}
	}
	value := metricNumber(rateMetric, "value")
	successRate := value
	errorRate := 1 - value
	if spec.SuccessMetric == "" {
		errorRate = value
		successRate = 1 - value
	}
	timeoutMetric := findMetric(raw, spec.TimeoutMetric, spec.TimeoutTags)
	timeoutCount := metricInt64(timeoutMetric, "count")
	errorCount := int64(math.Round(errorRate * float64(attempts)))
	successCount := attempts - errorCount
	return CorrectnessMetric{
		Operation: spec.ID, Attempts: attempts,
		SuccessCount: int64Ptr(successCount), ErrorCount: int64Ptr(errorCount), TimeoutCount: int64Ptr(timeoutCount),
		SuccessRate:   measured(floatPtr(successRate), "ratio", "k6:"+metricNameForSource(spec)),
		ErrorRate:     measured(floatPtr(errorRate), "ratio", "derived:1-success_rate"),
		TimeoutRate:   measured(floatPtr(float64(timeoutCount)/float64(attempts)), "ratio", "k6:"+spec.TimeoutMetric),
		FinalFailRate: naMeasurement("ratio", "not_applicable", "not an asynchronous terminal operation"),
		Idempotency:   naMeasurement("ratio", "diagnose:submit-coalescing", "requires the idempotency diagnostic case"),
	}
}

func correctnessFromCounts(operation string, attempts, success, errors, timeouts, finalFailures int64, source string) CorrectnessMetric {
	result := CorrectnessMetric{Operation: operation, Attempts: attempts, SuccessCount: int64Ptr(success), ErrorCount: int64Ptr(errors), TimeoutCount: int64Ptr(timeouts)}
	if attempts <= 0 {
		result.SuccessRate = naMeasurement("ratio", source, "no samples")
		result.ErrorRate = naMeasurement("ratio", source, "no samples")
		result.TimeoutRate = naMeasurement("ratio", source, "no samples")
		result.FinalFailRate = naMeasurement("ratio", source, "no samples")
	} else {
		result.SuccessRate = measured(floatPtr(float64(success)/float64(attempts)), "ratio", source)
		result.ErrorRate = measured(floatPtr(float64(errors)/float64(attempts)), "ratio", source)
		result.TimeoutRate = measured(floatPtr(float64(timeouts)/float64(attempts)), "ratio", source)
		result.FinalFailRate = measured(floatPtr(float64(finalFailures)/float64(attempts)), "ratio", source)
	}
	result.Idempotency = naMeasurement("ratio", "diagnose:submit-coalescing", "requires the idempotency diagnostic case")
	return result
}

func retryResults(raw rawSummary, evidence PhaseEvidence) []RetryMetric {
	httpAttempts := metricInt64(findMetric(raw, "http_reqs", nil), "count")
	client := RetryMetric{Layer: "client", InitialAttempts: httpAttempts, RetryAttempts: 0}
	if httpAttempts > 0 {
		client.RetryRate = measured(floatPtr(0), "ratio", "k6:retry_policy=none")
	} else {
		client.RetryRate = naMeasurement("ratio", "k6:retry_policy=none", "no HTTP attempts")
	}
	result := []RetryMetric{client, {
		Layer: "api_entry", RetryRate: naMeasurement("ratio", "server", "API entry retry is not instrumented; 429 is reported as an error, not a retry"),
	}}
	result = append(result, evidence.Retry...)
	result = append(result, RetryMetric{
		Layer: "downstream", RetryRate: naMeasurement("ratio", "server", "downstream client retry attempts are not yet exposed"),
	})
	return result
}

func evaluatePhase(spec phaseSpec, phase PhaseSummary, raw rawSummary, k6Exit int) Verdict {
	reasons := make([]string, 0)
	for _, threshold := range phase.Thresholds {
		if !threshold.Passed {
			reasons = append(reasons, fmt.Sprintf("threshold failed: %s %s", threshold.Metric, threshold.Expression))
		}
	}
	if spec.ThresholdTier != "none" {
		for _, item := range phase.Correctness {
			if item.SuccessRate.Value == nil || item.ErrorRate.Value == nil {
				reasons = append(reasons, fmt.Sprintf("%s has no correctness samples", item.Operation))
				continue
			}
			if *item.SuccessRate.Value <= 0.99 {
				reasons = append(reasons, fmt.Sprintf("%s success rate %.4f <= 0.99", item.Operation, *item.SuccessRate.Value))
			}
			if *item.ErrorRate.Value >= 0.01 {
				reasons = append(reasons, fmt.Sprintf("%s error rate %.4f >= 0.01", item.Operation, *item.ErrorRate.Value))
			}
		}
		httpReqs := metricInt64(findMetric(raw, "http_reqs", nil), "count")
		timeouts := metricInt64(findMetric(raw, "http_timeout_total", nil), "count")
		if httpReqs == 0 {
			reasons = append(reasons, "no HTTP request samples")
		} else if float64(timeouts)/float64(httpReqs) >= 0.001 {
			reasons = append(reasons, fmt.Sprintf("global timeout rate %.6f >= 0.001", float64(timeouts)/float64(httpReqs)))
		}
		if metricInt64(findMetric(raw, "answer_submit_timeout", nil), "count") > 0 {
			reasons = append(reasons, "answersheet submit timeout count is not zero")
		}
		if metricInt64(findMetric(raw, "chain_probe_timeout", nil), "count") > 0 {
			reasons = append(reasons, "chain probe timeout count is not zero")
		}
		if phase.Throughput.BusinessQPS.TargetAttainment.Value == nil || *phase.Throughput.BusinessQPS.TargetAttainment.Value < 0.99 {
			reasons = append(reasons, "actual business QPS did not reach 99% of target")
		}
		if phase.Throughput.BusinessQPS.Dropped.Value == nil {
			reasons = append(reasons, "dropped iteration evidence is unavailable")
		} else if *phase.Throughput.BusinessQPS.Dropped.Value != 0 {
			reasons = append(reasons, fmt.Sprintf("dropped iterations %.0f is not zero", *phase.Throughput.BusinessQPS.Dropped.Value))
		}
	}
	if len(reasons) > 0 {
		return Verdict{Status: VerdictFail, Reasons: uniqueStrings(reasons)}
	}
	if k6Exit != 0 {
		return Verdict{Status: VerdictError, Reasons: []string{fmt.Sprintf("k6 exited with code %d without an exported failed threshold", k6Exit)}}
	}
	return Verdict{Status: VerdictPass, Reasons: []string{"load, latency, and correctness gates passed"}}
}

func thresholdResults(raw rawSummary) []ThresholdResult {
	result := make([]ThresholdResult, 0)
	for name, metric := range raw.Metrics {
		thresholds, _ := metric["thresholds"].(map[string]any)
		for expression, failedValue := range thresholds {
			failed, _ := failedValue.(bool)
			result = append(result, ThresholdResult{Metric: name, Expression: expression, Passed: !failed})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Metric == result[j].Metric {
			return result[i].Expression < result[j].Expression
		}
		return result[i].Metric < result[j].Metric
	})
	return result
}

func findMetric(raw rawSummary, base string, tags []string) map[string]any {
	if len(tags) == 0 {
		return raw.Metrics[base]
	}
	for name, metric := range raw.Metrics {
		if name != base && !strings.HasPrefix(name, base+"{") {
			continue
		}
		matched := true
		for _, tag := range tags {
			if !strings.Contains(name, tag) {
				matched = false
				break
			}
		}
		if matched {
			return metric
		}
	}
	return nil
}

func metricNumber(metric map[string]any, key string) float64 {
	if metric == nil {
		return 0
	}
	value, _ := metric[key].(float64)
	return value
}

func metricNumberPtr(metric map[string]any, key string) *float64 {
	if metric == nil {
		return nil
	}
	value, ok := metric[key].(float64)
	if !ok {
		return nil
	}
	return floatPtr(value)
}

func metricInt64(metric map[string]any, key string) int64 {
	return int64(math.Round(metricNumber(metric, key)))
}

func trendMeasurement(metric map[string]any, key, name string) Measurement {
	if value := metricNumberPtr(metric, key); value != nil {
		return measured(value, "ms", "k6:"+name)
	}
	return naMeasurement("ms", "k6:"+name, "trend sample unavailable")
}

func metricNameForSource(spec operationSpec) string {
	if spec.SuccessMetric != "" {
		return spec.SuccessMetric
	}
	return spec.ErrorMetric
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func writeJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o600)
}

func renderRunMarkdown(summary RunSummary) string {
	var output bytes.Buffer
	fmt.Fprintf(&output, "# K6 压测报告：%s\n\n", summary.Run.ID)
	fmt.Fprintf(&output, "**结论：%s**\n\n", summary.Verdict.Status)
	for _, reason := range summary.Verdict.Reasons {
		fmt.Fprintf(&output, "- %s\n", reason)
	}
	fmt.Fprintf(&output, "\n## 运行信息\n\n- Plan: `%s`\n- Git SHA: `%s`\n- k6: `%s`\n- 时间: %s ～ %s\n\n", summary.Run.Plan, summary.Run.GitSHA, summary.Run.K6Version, summary.Run.StartedAt.Format(time.RFC3339), summary.Run.FinishedAt.Format(time.RFC3339))
	fmt.Fprintln(&output, "## 阶段结论")
	fmt.Fprintln(&output)
	fmt.Fprintln(&output, "| 阶段 | 结论 | 目标时长 | 实际时长 | 目标 QPS | 实际 QPS | HTTP RPS | 受理 TPS | 完成 TPS |")
	fmt.Fprintln(&output, "| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |")
	for _, phase := range summary.Phases {
		fmt.Fprintf(&output, "| %s | %s | %s | %s | %d | %s | %s | %s | %s |\n", phase.ID, phase.Verdict.Status, phase.Duration, formatMeasurement(phase.ActualDuration), phase.TargetQPS,
			formatMeasurement(phase.Throughput.BusinessQPS.Actual), formatMeasurement(phase.Throughput.HTTPRPS), formatMeasurement(phase.Throughput.AcceptedTPS), formatMeasurement(phase.Throughput.CompletedTPS))
	}
	for _, phase := range summary.Phases {
		fmt.Fprintf(&output, "\n## %s\n\n", phase.ID)
		fmt.Fprintf(&output, "结论：**%s**\n\n", phase.Verdict.Status)
		for _, reason := range phase.Verdict.Reasons {
			fmt.Fprintf(&output, "- %s\n", reason)
		}
		fmt.Fprintln(&output, "### 吞吐与处理能力")
		fmt.Fprintln(&output)
		fmt.Fprintf(&output, "- QPS 达成率：%s\n- HTTP RPS：%s\n- WebSocket 会话率：%s\n- 受理 TPS：%s\n- 完成 TPS：%s\n- 最终完成率：%s\n- 请求放大率：%s\n- 轮询放大率：%s\n",
			formatPercent(phase.Throughput.BusinessQPS.TargetAttainment), formatMeasurement(phase.Throughput.HTTPRPS), formatMeasurement(phase.Throughput.WSSessionsPerSecond),
			formatMeasurement(phase.Throughput.AcceptedTPS), formatMeasurement(phase.Throughput.CompletedTPS), formatPercent(phase.Throughput.FinalCompletionRate), formatRatio(phase.Throughput.RequestAmplification), formatRatio(phase.Throughput.PollingAmplification))
		fmt.Fprintln(&output, "\n| 模型类型 | 受理 TPS | 完成 TPS |")
		fmt.Fprintln(&output, "| --- | ---: | ---: |")
		for _, model := range []string{"medical", "personality", "behavior"} {
			fmt.Fprintf(&output, "| %s | %s | %s |\n", model, formatMeasurement(phase.Throughput.AcceptedTPSByModel[model]), formatMeasurement(phase.Throughput.CompletedTPSByModel[model]))
		}
		fmt.Fprintln(&output, "\n### 时延与响应体验")
		fmt.Fprintln(&output)
		fmt.Fprintln(&output, "| 操作 | 样本 | P50 | P95 | P99 | 最大耗时 | 平均耗时 |")
		fmt.Fprintln(&output, "| --- | ---: | ---: | ---: | ---: | ---: | ---: |")
		for _, item := range phase.Latency {
			fmt.Fprintf(&output, "| %s | %d | %s | %s | %s | %s | %s |\n", item.Operation, item.Samples, formatMeasurement(item.P50), formatMeasurement(item.P95), formatMeasurement(item.P99), formatMeasurement(item.Max), formatMeasurement(item.Average))
		}
		fmt.Fprintln(&output, "\n### 可靠性与正确性")
		fmt.Fprintln(&output)
		fmt.Fprintln(&output, "| 操作 | 初始操作 | 成功数 | 错误数 | 超时数 | 成功率 | 错误率 | 超时率 | 最终失败率 | 幂等成功率 |")
		fmt.Fprintln(&output, "| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |")
		for _, item := range phase.Correctness {
			fmt.Fprintf(&output, "| %s | %d | %s | %s | %s | %s | %s | %s | %s | %s |\n", item.Operation, item.Attempts, formatCount(item.SuccessCount), formatCount(item.ErrorCount), formatCount(item.TimeoutCount), formatPercent(item.SuccessRate), formatPercent(item.ErrorRate), formatPercent(item.TimeoutRate), formatPercent(item.FinalFailRate), formatPercent(item.Idempotency))
		}
		fmt.Fprintln(&output, "\n### 分层重试")
		fmt.Fprintln(&output)
		fmt.Fprintln(&output, "| 层级 | 初始尝试 | 重试尝试 | 重试率 |")
		fmt.Fprintln(&output, "| --- | ---: | ---: | ---: |")
		for _, item := range phase.Retry {
			fmt.Fprintf(&output, "| %s | %d | %d | %s |\n", item.Layer, item.InitialAttempts, item.RetryAttempts, formatPercent(item.RetryRate))
		}
		fmt.Fprintln(&output, "\n### 排队与服务端证据")
		fmt.Fprintln(&output)
		fmt.Fprintln(&output, "| 项目 | 状态 / 数值 | 来源 |")
		fmt.Fprintln(&output, "| --- | --- | --- |")
		fmt.Fprintf(&output, "| 完成 TPS 观测窗口 | %s | %s |\n", formatMeasurement(phase.Evidence.CompletionWindow), phase.Evidence.CompletionWindow.Source)
		for _, item := range phase.QueueWait {
			fmt.Fprintf(&output, "| %s | %s | %s |\n", item.Layer, formatMeasurement(item.Wait), item.Wait.Source)
		}
		for _, check := range phase.Evidence.Checks {
			fmt.Fprintf(&output, "| %s | %s | %s |\n", check.Name, check.Status, check.Source)
		}
		fmt.Fprintln(&output, "\n### 阈值结果")
		fmt.Fprintln(&output)
		fmt.Fprintln(&output, "| 指标 | 表达式 | 结果 |")
		fmt.Fprintln(&output, "| --- | --- | --- |")
		for _, threshold := range phase.Thresholds {
			status := "PASS"
			if !threshold.Passed {
				status = "FAIL"
			}
			fmt.Fprintf(&output, "| %s | `%s` | %s |\n", threshold.Metric, threshold.Expression, status)
		}
	}
	if len(summary.Recovery) > 0 {
		fmt.Fprintln(&output, "\n## 排空与恢复验收")
		fmt.Fprintln(&output)
		fmt.Fprintln(&output, "| 恢复门 | 结论 | 恢复耗时 | 尝试次数 | 成功完成增量 | 失败增量 | Outbox 残留 | NSQ 残留 | 最老任务 |")
		fmt.Fprintln(&output, "| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |")
		for _, recovery := range summary.Recovery {
			fmt.Fprintf(&output, "| %s | %s | %s | %d | %s | %s | %s | %s | %s |\n", recovery.ID, recovery.Verdict.Status, formatDuration(recovery.StartedAt, recovery.FinishedAt), recovery.Attempts,
				formatFloatCount(recovery.Evidence.CompletedCountDelta), formatFloatCount(recovery.Evidence.FailedCountDelta), formatFloatCount(recovery.Evidence.OutboxBacklog), formatFloatCount(recovery.Evidence.NSQDepth), formatSeconds(recovery.Evidence.OutboxOldestAge))
		}
	}
	return output.String()
}

func formatCount(value *int64) string {
	if value == nil {
		return "N/A"
	}
	return fmt.Sprintf("%d", *value)
}

func formatFloatCount(value *float64) string {
	if value == nil {
		return "N/A"
	}
	return fmt.Sprintf("%.0f", *value)
}

func formatSeconds(value *float64) string {
	if value == nil {
		return "N/A"
	}
	return fmt.Sprintf("%.2fs", *value)
}

func formatDuration(startedAt, finishedAt time.Time) string {
	if startedAt.IsZero() || finishedAt.IsZero() || finishedAt.Before(startedAt) {
		return "N/A"
	}
	return finishedAt.Sub(startedAt).Round(time.Second).String()
}

func formatMeasurement(value Measurement) string {
	if value.Value == nil {
		return "N/A"
	}
	switch value.Unit {
	case "ms":
		return fmt.Sprintf("%.2f ms", *value.Value)
	case "tps", "qps", "rps", "sessions/s":
		return fmt.Sprintf("%.2f %s", *value.Value, value.Unit)
	default:
		return fmt.Sprintf("%.4f", *value.Value)
	}
}

func formatPercent(value Measurement) string {
	if value.Value == nil {
		return "N/A"
	}
	return fmt.Sprintf("%.2f%%", *value.Value*100)
}

func formatRatio(value Measurement) string {
	if value.Value == nil {
		return "N/A"
	}
	return fmt.Sprintf("%.2fx", *value.Value)
}
