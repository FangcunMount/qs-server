package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type metricSample struct {
	Name   string
	Labels map[string]string
	Value  float64
}

func collectPhaseEvidence(dir string) PhaseEvidence {
	checks := []EvidenceCheck{
		checkReadyFile(dir, "after-collection-readyz.json", "collection readyz"),
		checkReadyFile(dir, "after-apiserver-readyz.json", "apiserver readyz"),
		checkReadyFile(dir, "after-worker-readyz.json", "worker readyz"),
		checkFile(dir, "after-collection-metrics.txt", "collection metrics"),
		checkFile(dir, "after-apiserver-metrics.txt", "apiserver metrics"),
		checkFile(dir, "after-worker-metrics.txt", "worker metrics"),
		checkJSONFile(dir, "after-nsqd-stats.json", "NSQD stats"),
	}
	var trafficIsolated *bool
	if isolated, declared := os.LookupEnv("PERF_ISOLATED_ENV"); declared {
		if strings.EqualFold(strings.TrimSpace(isolated), "true") {
			value := true
			trafficIsolated = &value
			checks = append(checks, EvidenceCheck{Name: "traffic isolation", Status: "PASS", Source: "env:PERF_ISOLATED_ENV"})
		} else {
			value := false
			trafficIsolated = &value
			checks = append(checks, EvidenceCheck{Name: "traffic isolation", Status: "MISSING", Source: "env:PERF_ISOLATED_ENV", Message: "concurrent business traffic cannot be isolated; completed TPS is not admission evidence"})
		}
	}
	before := readMetricSnapshots(dir, "before")
	after := readMetricSnapshots(dir, "after")
	completedDelta, failedDelta, completedByModel, hasCompletionEvidence := interpretationDeltas(before, after)
	if hasCompletionEvidence {
		checks = append(checks, EvidenceCheck{Name: "completed transaction metric", Status: "PASS", Source: "qs_interpretation_run_duration_seconds_count"})
	} else {
		checks = append(checks, EvidenceCheck{Name: "completed transaction metric", Status: "MISSING", Source: "qs_interpretation_run_duration_seconds_count", Message: "cannot calculate completed TPS"})
	}
	retry, retryComplete := retryEvidence(before, after)
	if retryComplete {
		checks = append(checks, EvidenceCheck{Name: "layered retry metric", Status: "PASS", Source: "qs_retry_layer_attempt_total"})
	} else {
		checks = append(checks, EvidenceCheck{Name: "layered retry metric", Status: "MISSING", Source: "qs_retry_layer_attempt_total", Message: "one or more server processes do not expose the retry contract"})
	}
	outboxBacklog, hasOutbox := sumOutboxBacklog(after)
	outboxOldest, hasOldest := maxOutboxAge(after)
	nsqDepth, hasNSQ := readNSQDepth(filepath.Join(dir, "after-nsqd-stats.json"))
	queueWait := make([]QueueWaitMetric, 0, 1)
	if hasOldest {
		queueWait = append(queueWait, QueueWaitMetric{Layer: "outbox_oldest_pending", Wait: measured(&outboxOldest, "seconds", "prometheus:qs_event_outbox_oldest_age_seconds")})
	}
	complete := retryComplete && hasCompletionEvidence && hasOutbox && hasOldest && hasNSQ
	for _, check := range checks {
		if check.Status != "PASS" {
			complete = false
		}
	}
	evidence := PhaseEvidence{
		Complete: complete, TrafficIsolated: trafficIsolated, Checks: checks, CompletedCountDelta: completedDelta, FailedCountDelta: failedDelta,
		CompletedCountDeltaByModel: completedByModel, Retry: retry, QueueWait: queueWait,
	}
	if hasOutbox {
		evidence.OutboxBacklog = &outboxBacklog
	}
	if hasOldest {
		evidence.OutboxOldestAge = &outboxOldest
	}
	if hasNSQ {
		evidence.NSQDepth = &nsqDepth
	}
	return evidence
}

func interpretationDeltas(before, after []metricSample) (*float64, *float64, map[string]float64, bool) {
	completedBefore, hasCompletedBefore := sumMetric(before, "qs_interpretation_run_duration_seconds_count", map[string]string{"result": "success"})
	completedAfter, hasCompletedAfter := sumMetric(after, "qs_interpretation_run_duration_seconds_count", map[string]string{"result": "success"})
	if !hasCompletedBefore || !hasCompletedAfter {
		return nil, nil, map[string]float64{}, false
	}
	completed := nonNegativeDelta(completedBefore, completedAfter)
	failedBefore, hasFailedBefore := sumMetric(before, "qs_interpretation_run_duration_seconds_count", map[string]string{"result": "error"})
	failedAfter, hasFailedAfter := sumMetric(after, "qs_interpretation_run_duration_seconds_count", map[string]string{"result": "error"})
	failed := 0.0
	if hasFailedBefore && hasFailedAfter {
		failed = nonNegativeDelta(failedBefore, failedAfter)
	} else if hasFailedAfter {
		failed = failedAfter
	}
	return &completed, &failed, completedModelDeltas(before, after), true
}

func completedModelDeltas(before, after []metricSample) map[string]float64 {
	builders := map[string][]string{
		"medical":     {"factor-scoring", "norm-profile"},
		"personality": {"typology"},
		"behavior":    {"task-performance"},
	}
	result := make(map[string]float64, len(builders))
	for model, identities := range builders {
		var beforeValue, afterValue float64
		var beforeFound, afterFound bool
		for _, identity := range identities {
			value, found := sumMetric(before, "qs_interpretation_run_duration_seconds_count", map[string]string{"result": "success", "builder_identity": identity})
			beforeValue += value
			beforeFound = beforeFound || found
			value, found = sumMetric(after, "qs_interpretation_run_duration_seconds_count", map[string]string{"result": "success", "builder_identity": identity})
			afterValue += value
			afterFound = afterFound || found
		}
		if beforeFound && afterFound {
			result[model] = nonNegativeDelta(beforeValue, afterValue)
		}
	}
	return result
}

func retryEvidence(before, after []metricSample) ([]RetryMetric, bool) {
	result := make([]RetryMetric, 0, 4)
	complete := true
	for _, layer := range []string{"business", "outbox", "hold", "transport"} {
		initialBefore, hasInitialBefore := sumMetric(before, "qs_retry_layer_attempt_total", map[string]string{"layer": layer, "attempt_class": "initial"})
		initialAfter, hasInitialAfter := sumMetric(after, "qs_retry_layer_attempt_total", map[string]string{"layer": layer, "attempt_class": "initial"})
		retryBefore, hasRetryBefore := sumMetric(before, "qs_retry_layer_attempt_total", map[string]string{"layer": layer, "attempt_class": "retry"})
		retryAfter, hasRetryAfter := sumMetric(after, "qs_retry_layer_attempt_total", map[string]string{"layer": layer, "attempt_class": "retry"})
		initial := int64(math.Round(nonNegativeDelta(initialBefore, initialAfter)))
		retries := int64(math.Round(nonNegativeDelta(retryBefore, retryAfter)))
		metric := RetryMetric{Layer: layer, InitialAttempts: initial, RetryAttempts: retries}
		if hasInitialBefore && hasInitialAfter && hasRetryBefore && hasRetryAfter {
			if initial > 0 {
				metric.RetryRate = measured(floatPtr(float64(retries)/float64(initial)), "ratio", "prometheus:qs_retry_layer_attempt_total")
			} else {
				metric.RetryRate = naMeasurement("ratio", "prometheus:qs_retry_layer_attempt_total", "initial attempt denominator is zero")
			}
		} else {
			complete = false
			metric.RetryRate = naMeasurement("ratio", "prometheus:qs_retry_layer_attempt_total", "layer metric unavailable")
		}
		result = append(result, metric)
	}
	return result, complete
}

func checkReadyFile(dir, name, label string) EvidenceCheck {
	path := filepath.Join(dir, name)
	raw, err := os.ReadFile(path)
	if err != nil {
		return EvidenceCheck{Name: label, Status: "MISSING", Source: path, Message: err.Error()}
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return EvidenceCheck{Name: label, Status: "INVALID", Source: path, Message: err.Error()}
	}
	status, _ := payload["status"].(string)
	if status == "" {
		if data, ok := payload["data"].(map[string]any); ok {
			status, _ = data["status"].(string)
		}
	}
	if status != "ready" {
		return EvidenceCheck{Name: label, Status: "FAIL", Source: path, Message: fmt.Sprintf("status=%q", status)}
	}
	return EvidenceCheck{Name: label, Status: "PASS", Source: path}
}

func checkFile(dir, name, label string) EvidenceCheck {
	path := filepath.Join(dir, name)
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 {
		message := "file is empty"
		if err != nil {
			message = err.Error()
		}
		return EvidenceCheck{Name: label, Status: "MISSING", Source: path, Message: message}
	}
	return EvidenceCheck{Name: label, Status: "PASS", Source: path}
}

func checkJSONFile(dir, name, label string) EvidenceCheck {
	result := checkFile(dir, name, label)
	if result.Status != "PASS" {
		return result
	}
	raw, err := os.ReadFile(result.Source)
	if err != nil {
		result.Status, result.Message = "MISSING", err.Error()
		return result
	}
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		result.Status, result.Message = "INVALID", err.Error()
	}
	return result
}

func readMetricSnapshots(dir, label string) []metricSample {
	result := make([]metricSample, 0)
	for _, component := range []string{"collection", "apiserver", "worker"} {
		path := filepath.Join(dir, fmt.Sprintf("%s-%s-metrics.txt", label, component))
		samples, err := parsePrometheusFile(path)
		if err == nil {
			result = append(result, samples...)
		}
	}
	return result
}

func parsePrometheusFile(path string) ([]metricSample, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	result := make([]metricSample, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lastSpace := strings.LastIndexAny(line, " \t")
		if lastSpace < 0 {
			continue
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(line[lastSpace+1:]), 64)
		if err != nil {
			continue
		}
		identity := strings.TrimSpace(line[:lastSpace])
		name := identity
		labels := map[string]string{}
		if brace := strings.IndexByte(identity, '{'); brace >= 0 && strings.HasSuffix(identity, "}") {
			name = identity[:brace]
			labels = parseLabels(identity[brace+1 : len(identity)-1])
		}
		result = append(result, metricSample{Name: name, Labels: labels, Value: value})
	}
	return result, scanner.Err()
}

func parseLabels(raw string) map[string]string {
	result := map[string]string{}
	parts := strings.Split(raw, ",")
	for _, part := range parts {
		key, value, found := strings.Cut(part, "=")
		if !found {
			continue
		}
		result[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), "\"")
	}
	return result
}

func sumMetric(samples []metricSample, name string, labels map[string]string) (float64, bool) {
	total := 0.0
	found := false
	for _, sample := range samples {
		if sample.Name != name || !labelsMatch(sample.Labels, labels) {
			continue
		}
		total += sample.Value
		found = true
	}
	return total, found
}

func labelsMatch(actual, expected map[string]string) bool {
	for key, value := range expected {
		if actual[key] != value {
			return false
		}
	}
	return true
}

func sumOutboxBacklog(samples []metricSample) (float64, bool) {
	return sumMetric(samples, "qs_event_outbox_backlog", map[string]string{})
}

func maxOutboxAge(samples []metricSample) (float64, bool) {
	maximum := 0.0
	found := false
	for _, sample := range samples {
		if sample.Name != "qs_event_outbox_oldest_age_seconds" {
			continue
		}
		if sample.Value > maximum {
			maximum = sample.Value
		}
		found = true
	}
	return maximum, found
}

func readNSQDepth(path string) (float64, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return 0, false
	}
	total, count := sumJSONKey(payload, "depth")
	return total, count > 0
}

func sumJSONKey(value any, key string) (float64, int) {
	switch typed := value.(type) {
	case map[string]any:
		total, count := 0.0, 0
		for childKey, childValue := range typed {
			if childKey == key {
				if number, ok := childValue.(float64); ok {
					total += number
					count++
				}
			}
			childTotal, childCount := sumJSONKey(childValue, key)
			total, count = total+childTotal, count+childCount
		}
		return total, count
	case []any:
		total, count := 0.0, 0
		for _, child := range typed {
			childTotal, childCount := sumJSONKey(child, key)
			total, count = total+childTotal, count+childCount
		}
		return total, count
	default:
		return 0, 0
	}
}

func nonNegativeDelta(before, after float64) float64 {
	if after < before {
		return after
	}
	return after - before
}

func recoveryVerdict(baseline, current PhaseEvidence) Verdict {
	reasons := make([]string, 0)
	if !baseline.Complete {
		reasons = append(reasons, "baseline recovery evidence is incomplete")
	}
	if !current.Complete {
		reasons = append(reasons, "recovery evidence is incomplete")
	}
	if baseline.OutboxBacklog == nil || current.OutboxBacklog == nil {
		reasons = append(reasons, "outbox backlog baseline is unavailable")
	} else if *current.OutboxBacklog > *baseline.OutboxBacklog {
		reasons = append(reasons, fmt.Sprintf("outbox backlog %.0f has not returned to baseline %.0f", *current.OutboxBacklog, *baseline.OutboxBacklog))
	}
	if baseline.NSQDepth == nil || current.NSQDepth == nil {
		reasons = append(reasons, "NSQ depth baseline is unavailable")
	} else if *current.NSQDepth > *baseline.NSQDepth {
		reasons = append(reasons, fmt.Sprintf("NSQ depth %.0f has not returned to baseline %.0f", *current.NSQDepth, *baseline.NSQDepth))
	}
	if current.OutboxOldestAge != nil {
		baselineAge := 5.0
		if baseline.OutboxOldestAge != nil && *baseline.OutboxOldestAge > baselineAge {
			baselineAge = *baseline.OutboxOldestAge
		}
		if *current.OutboxOldestAge > baselineAge {
			reasons = append(reasons, fmt.Sprintf("oldest outbox age %.2fs remains above baseline allowance %.2fs", *current.OutboxOldestAge, baselineAge))
		}
	}
	if len(reasons) == 0 {
		return Verdict{Status: VerdictPass, Reasons: []string{"readiness is healthy and asynchronous backlog returned to baseline"}}
	}
	status := VerdictFail
	if classifyEvidence(baseline, "baseline recovery evidence").Status == VerdictIncomplete ||
		classifyEvidence(current, "recovery evidence").Status == VerdictIncomplete ||
		baseline.OutboxBacklog == nil || baseline.NSQDepth == nil {
		status = VerdictIncomplete
	}
	return Verdict{Status: status, Reasons: uniqueStrings(reasons)}
}

func classifyEvidence(evidence PhaseEvidence, subject string) Verdict {
	failed := make([]string, 0)
	for _, check := range evidence.Checks {
		if check.Status == "FAIL" {
			failed = append(failed, fmt.Sprintf("%s failed: %s", subject, check.Name))
		}
	}
	if len(failed) > 0 {
		return Verdict{Status: VerdictFail, Reasons: failed}
	}
	if !evidence.Complete {
		return Verdict{Status: VerdictIncomplete, Reasons: []string{subject + " is incomplete"}}
	}
	return Verdict{Status: VerdictPass, Reasons: []string{subject + " is complete"}}
}

func sortChecks(checks []EvidenceCheck) {
	sort.Slice(checks, func(i, j int) bool { return checks[i].Name < checks[j].Name })
}
