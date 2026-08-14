package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type metricSample struct {
	Name   string
	Labels map[string]string
	Value  float64
}

const snapshotComponentLabel = "__snapshot_component"

func collectPhaseEvidence(dir string) PhaseEvidence {
	checks := []EvidenceCheck{
		checkReplicaReadyFile(dir, "after-collection-readyz.json", "collection readyz", "EXPECTED_COLLECTION_REPLICAS", 2),
		checkReadyFile(dir, "after-apiserver-readyz.json", "apiserver readyz"),
		checkReplicaReadyFile(dir, "after-worker-readyz.json", "worker readyz", "EXPECTED_WORKER_REPLICAS", 3),
		checkFederatedMetricsReady(dir, "after-collection-metrics.txt", "collection metrics replicas", "collection-server", "EXPECTED_COLLECTION_REPLICAS", 2),
		checkFile(dir, "after-apiserver-metrics.txt", "apiserver metrics"),
		checkFederatedMetricsReady(dir, "after-worker-metrics.txt", "worker metrics replicas", "worker", "EXPECTED_WORKER_REPLICAS", 3),
		checkJSONFile(dir, "after-nsqd-stats.json", "NSQD stats"),
	}
	before := readMetricSnapshots(dir, "before")
	after := readMetricSnapshots(dir, "after")
	checks = append(checks, processContinuityEvidence(before, after))
	trafficIsolated, isolationCheck := trafficIsolationEvidence(before, after)
	checks = append(checks, isolationCheck)
	completedDelta, failedDelta, completedByModel, hasCompletionEvidence := interpretationDeltas(before, after)
	expectedCompletionDelta, noAssessmentRequiredDelta, hasIntakeOutcomeEvidence := assessmentIntakeOutcomeDeltas(before, after)
	completionWindow := naMeasurement("seconds", "prometheus:snapshot_window", "completion metric capture window unavailable")
	windowSeconds, hasCompletionWindow := prometheusObservationWindow(dir, "before", dir, "after")
	if hasCompletionWindow {
		completionWindow = measured(&windowSeconds, "seconds", "prometheus:snapshot_file_mtime")
		checks = append(checks, EvidenceCheck{Name: "completion observation window", Status: "PASS", Source: "before/after Prometheus snapshot timestamps"})
	} else {
		checks = append(checks, EvidenceCheck{Name: "completion observation window", Status: "MISSING", Source: "before/after Prometheus snapshot timestamps", Message: "cannot calculate completed TPS denominator"})
	}
	if hasCompletionEvidence {
		checks = append(checks, EvidenceCheck{Name: "completed transaction metric", Status: "PASS", Source: "qs_interpretation_run_duration_seconds_count"})
	} else {
		checks = append(checks, EvidenceCheck{Name: "completed transaction metric", Status: "MISSING", Source: "qs_interpretation_run_duration_seconds_count", Message: "cannot calculate completed TPS"})
	}
	if hasIntakeOutcomeEvidence {
		checks = append(checks, EvidenceCheck{Name: "assessment intake outcome metric", Status: "PASS", Source: "qs_evaluation_assessment_intake_outcome_total"})
	} else {
		checks = append(checks, EvidenceCheck{Name: "assessment intake outcome metric", Status: "MISSING", Source: "qs_evaluation_assessment_intake_outcome_total", Message: "cannot distinguish newly created Assessments from independent questionnaires"})
	}
	retry, retryComplete := retryEvidence(before, after)
	if retryComplete {
		checks = append(checks, EvidenceCheck{Name: "layered retry metric", Status: "PASS", Source: "qs_retry_layer_attempt_total"})
	} else {
		checks = append(checks, EvidenceCheck{Name: "layered retry metric", Status: "MISSING", Source: "qs_retry_layer_attempt_total", Message: "one or more server processes do not expose the retry contract"})
	}
	outboxBacklogBaseline, hasOutboxBaseline := sumOutboxBacklog(before)
	outboxBacklog, hasOutbox := sumOutboxBacklog(after)
	outboxOldest, hasOldest := maxOutboxAge(after)
	nsqDepthBaseline, hasNSQBaseline := readNSQDepth(filepath.Join(dir, "before-nsqd-stats.json"))
	nsqDepth, hasNSQ := readNSQDepth(filepath.Join(dir, "after-nsqd-stats.json"))
	if hasOutboxBaseline && hasOutbox {
		checks = append(checks, EvidenceCheck{Name: "outbox backlog window", Status: "PASS", Source: "before/after:qs_event_outbox_backlog"})
	} else {
		checks = append(checks, EvidenceCheck{Name: "outbox backlog window", Status: "MISSING", Source: "before/after:qs_event_outbox_backlog", Message: "cannot determine whether the load window accumulated outbox work"})
	}
	if hasNSQBaseline && hasNSQ {
		checks = append(checks, EvidenceCheck{Name: "NSQ depth window", Status: "PASS", Source: "before/after:NSQD stats"})
	} else {
		checks = append(checks, EvidenceCheck{Name: "NSQ depth window", Status: "MISSING", Source: "before/after:NSQD stats", Message: "cannot determine whether the load window accumulated NSQ work"})
	}
	queueWait := make([]QueueWaitMetric, 0, 1)
	if hasOldest {
		queueWait = append(queueWait, QueueWaitMetric{Layer: "outbox_oldest_pending", Wait: measured(&outboxOldest, "seconds", "prometheus:qs_event_outbox_oldest_age_seconds")})
	}
	complete := retryComplete && hasCompletionEvidence && hasIntakeOutcomeEvidence && hasCompletionWindow && hasOutboxBaseline && hasOutbox && hasOldest && hasNSQBaseline && hasNSQ
	for _, check := range checks {
		if check.Status != "PASS" {
			complete = false
		}
	}
	evidence := PhaseEvidence{
		Complete: complete, TrafficIsolated: trafficIsolated, CompletionWindow: completionWindow, Checks: checks, CompletedCountDelta: completedDelta, FailedCountDelta: failedDelta,
		CompletedCountDeltaByModel: completedByModel, ExpectedCompletionCountDelta: expectedCompletionDelta, NoAssessmentRequiredCountDelta: noAssessmentRequiredDelta,
		Retry: retry, QueueWait: queueWait,
	}
	if hasOutboxBaseline {
		evidence.OutboxBacklogBaseline = &outboxBacklogBaseline
	}
	if hasOutbox {
		evidence.OutboxBacklog = &outboxBacklog
	}
	if hasOutboxBaseline && hasOutbox {
		delta := outboxBacklog - outboxBacklogBaseline
		evidence.OutboxBacklogDelta = &delta
	}
	if hasOldest {
		evidence.OutboxOldestAge = &outboxOldest
	}
	if hasNSQBaseline {
		evidence.NSQDepthBaseline = &nsqDepthBaseline
	}
	if hasNSQ {
		evidence.NSQDepth = &nsqDepth
	}
	if hasNSQBaseline && hasNSQ {
		delta := nsqDepth - nsqDepthBaseline
		evidence.NSQDepthDelta = &delta
	}
	return evidence
}

func processContinuityEvidence(before, after []metricSample) EvidenceCheck {
	const source = "before/after:process_start_time_seconds"
	beforeProcesses, beforeIssue := processStartTimes(before)
	afterProcesses, afterIssue := processStartTimes(after)
	if beforeIssue != "" || afterIssue != "" {
		issues := make([]string, 0, 2)
		if beforeIssue != "" {
			issues = append(issues, beforeIssue)
		}
		if afterIssue != "" {
			issues = append(issues, afterIssue)
		}
		return EvidenceCheck{Name: "component process continuity", Status: "INVALID", Source: source, Message: strings.Join(issues, "; ")}
	}
	if len(beforeProcesses) == 0 || len(afterProcesses) == 0 {
		return EvidenceCheck{Name: "component process continuity", Status: "MISSING", Source: source, Message: "process start metrics are unavailable"}
	}
	for _, component := range []string{"collection", "apiserver", "worker"} {
		if !hasProcessComponent(beforeProcesses, component) || !hasProcessComponent(afterProcesses, component) {
			return EvidenceCheck{Name: "component process continuity", Status: "MISSING", Source: source, Message: component + " process start metric is unavailable"}
		}
	}
	if len(beforeProcesses) != len(afterProcesses) {
		return EvidenceCheck{Name: "component process continuity", Status: "INVALID", Source: source, Message: fmt.Sprintf("process set changed from %d to %d instances", len(beforeProcesses), len(afterProcesses))}
	}
	for identity, beforeStart := range beforeProcesses {
		afterStart, exists := afterProcesses[identity]
		if !exists {
			return EvidenceCheck{Name: "component process continuity", Status: "INVALID", Source: source, Message: identity + " disappeared or changed identity during the load window"}
		}
		if afterStart != beforeStart {
			return EvidenceCheck{Name: "component process continuity", Status: "INVALID", Source: source, Message: fmt.Sprintf("%s process start changed from %.3f to %.3f during the load window", identity, beforeStart, afterStart)}
		}
	}
	return EvidenceCheck{Name: "component process continuity", Status: "PASS", Source: source, Message: fmt.Sprintf("%d process identities remained stable", len(beforeProcesses))}
}

func processStartTimes(samples []metricSample) (map[string]float64, string) {
	result := map[string]float64{}
	for _, sample := range samples {
		if sample.Name != "process_start_time_seconds" {
			continue
		}
		component := strings.TrimSpace(sample.Labels[snapshotComponentLabel])
		if component == "" {
			continue
		}
		instance := strings.TrimSpace(sample.Labels["instance"])
		if instance == "" {
			instance = strings.TrimSpace(sample.Labels["exported_instance"])
		}
		if instance == "" {
			instance = "direct"
		}
		identity := component + "/" + instance
		if previous, exists := result[identity]; exists && previous != sample.Value {
			return nil, identity + " exposes conflicting process start times"
		}
		result[identity] = sample.Value
	}
	return result, ""
}

func hasProcessComponent(processes map[string]float64, component string) bool {
	prefix := component + "/"
	for identity := range processes {
		if strings.HasPrefix(identity, prefix) {
			return true
		}
	}
	return false
}

func assessmentIntakeOutcomeDeltas(before, after []metricSample) (*float64, *float64, bool) {
	const metric = "qs_evaluation_assessment_intake_outcome_total"
	expectedBefore, _ := sumMetric(before, metric, map[string]string{"result": "assessment_created"})
	expectedAfter, hasExpectedAfter := sumMetric(after, metric, map[string]string{"result": "assessment_created"})
	noAssessmentBefore, _ := sumMetric(before, metric, map[string]string{"result": "no_assessment_required"})
	noAssessmentAfter, hasNoAssessmentAfter := sumMetric(after, metric, map[string]string{"result": "no_assessment_required"})
	if !hasExpectedAfter || !hasNoAssessmentAfter {
		return nil, nil, false
	}
	expected := nonNegativeDelta(expectedBefore, expectedAfter)
	noAssessment := nonNegativeDelta(noAssessmentBefore, noAssessmentAfter)
	return &expected, &noAssessment, true
}

func trafficIsolationEvidence(before, after []metricSample) (*bool, EvidenceCheck) {
	raw, declared := os.LookupEnv("PERF_ISOLATED_ENV")
	normalized := strings.ToLower(strings.TrimSpace(raw))
	if !declared || normalized == "" {
		return nil, EvidenceCheck{Name: "traffic isolation", Status: "MISSING", Source: "env:PERF_ISOLATED_ENV", Message: "traffic isolation was not declared; set PERF_ISOLATED_ENV=true only for a controlled window"}
	}
	if normalized != "true" {
		value := false
		return &value, EvidenceCheck{Name: "traffic isolation", Status: "MISSING", Source: "env:PERF_ISOLATED_ENV", Message: "concurrent business traffic cannot be isolated; completed TPS is not admission evidence"}
	}

	const metric = "qs_perf_traffic_requests_total"
	source := "env:PERF_ISOLATED_ENV+prometheus:" + metric + `{origin="other"}`
	delta := 0.0
	for _, component := range []string{"collection", "apiserver"} {
		labels := map[string]string{"origin": "other", snapshotComponentLabel: component}
		otherBefore, hasBefore := sumMetric(before, metric, labels)
		otherAfter, hasAfter := sumMetric(after, metric, labels)
		if !hasBefore || !hasAfter {
			return nil, EvidenceCheck{
				Name: "traffic isolation", Status: "MISSING", Source: source,
				Message: fmt.Sprintf("PERF_ISOLATED_ENV=true was declared but %s does not expose bounded perf/other traffic evidence", component),
			}
		}
		if otherAfter < otherBefore {
			return nil, EvidenceCheck{
				Name: "traffic isolation", Status: "INVALID", Source: source,
				Message: fmt.Sprintf("%s other-traffic counter decreased from %.0f to %.0f; a process restart or counter reset invalidated the window", component, otherBefore, otherAfter),
			}
		}
		delta += otherAfter - otherBefore
	}
	if delta > 0 {
		value := false
		return &value, EvidenceCheck{
			Name: "traffic isolation", Status: "FAIL", Source: source,
			Message: fmt.Sprintf("observed %.0f business requests without X-Perf-Run-ID during the declared isolated window", delta),
		}
	}
	value := true
	return &value, EvidenceCheck{Name: "traffic isolation", Status: "PASS", Source: source, Message: "observed other business requests=0"}
}

func prometheusObservationWindow(startDir, startLabel, endDir, endLabel string) (float64, bool) {
	var earliest time.Time
	var latest time.Time
	for _, component := range []string{"collection", "apiserver", "worker"} {
		startPath := filepath.Join(startDir, fmt.Sprintf("%s-%s-metrics.txt", startLabel, component))
		endPath := filepath.Join(endDir, fmt.Sprintf("%s-%s-metrics.txt", endLabel, component))
		// A Prometheus counter may not exist until its first observation. The
		// before snapshot still defines the start of the measurement window when
		// it is a valid metrics document and the counter appears after the run.
		if !metricFileHasSamples(startPath) || !metricFileContains(endPath, "qs_interpretation_run_duration_seconds_count") {
			continue
		}
		startInfo, startErr := os.Stat(startPath)
		endInfo, endErr := os.Stat(endPath)
		if startErr != nil || endErr != nil {
			continue
		}
		if earliest.IsZero() || startInfo.ModTime().Before(earliest) {
			earliest = startInfo.ModTime()
		}
		if latest.IsZero() || endInfo.ModTime().After(latest) {
			latest = endInfo.ModTime()
		}
	}
	if earliest.IsZero() || latest.IsZero() || !latest.After(earliest) {
		return 0, false
	}
	return latest.Sub(earliest).Seconds(), true
}

func metricFileHasSamples(path string) bool {
	samples, err := parsePrometheusFile(path)
	return err == nil && len(samples) > 0
}

func metricFileContains(path, name string) bool {
	samples, err := parsePrometheusFile(path)
	if err != nil {
		return false
	}
	for _, sample := range samples {
		if sample.Name == name {
			return true
		}
	}
	return false
}

func interpretationDeltas(before, after []metricSample) (*float64, *float64, map[string]float64, bool) {
	completedBefore, _ := sumMetric(before, "qs_interpretation_run_duration_seconds_count", map[string]string{"result": "success"})
	completedAfter, hasCompletedAfter := sumMetric(after, "qs_interpretation_run_duration_seconds_count", map[string]string{"result": "success"})
	if !hasCompletedAfter {
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
		if afterFound {
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

func checkReplicaReadyFile(dir, name, label, expectedEnv string, defaultExpected int) EvidenceCheck {
	path := filepath.Join(dir, name)
	raw, err := os.ReadFile(path)
	if err != nil {
		return EvidenceCheck{Name: label, Status: "MISSING", Source: path, Message: err.Error()}
	}
	expected, err := expectedReplicaCount(expectedEnv, defaultExpected)
	if err != nil {
		return EvidenceCheck{Name: label, Status: "INVALID", Source: path, Message: err.Error()}
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return EvidenceCheck{Name: label, Status: "INVALID", Source: path, Message: err.Error()}
	}
	if count, ok := prometheusReadyCount(payload); ok {
		if count != float64(expected) {
			return EvidenceCheck{Name: label, Status: "FAIL", Source: path, Message: fmt.Sprintf("ready replicas=%.0f, expected=%d", count, expected)}
		}
		return EvidenceCheck{Name: label, Status: "PASS", Source: path, Message: fmt.Sprintf("ready replicas=%d", expected)}
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
	if expected > 1 {
		return EvidenceCheck{
			Name: label, Status: "INVALID", Source: path,
			Message: fmt.Sprintf("single load-balanced readyz response cannot prove %d replicas; use the aggregate Prometheus readiness endpoint", expected),
		}
	}
	return EvidenceCheck{Name: label, Status: "PASS", Source: path}
}

func prometheusReadyCount(payload map[string]any) (float64, bool) {
	if status, _ := payload["status"].(string); status != "success" {
		return 0, false
	}
	data, _ := payload["data"].(map[string]any)
	results, _ := data["result"].([]any)
	if len(results) != 1 {
		return 0, true
	}
	result, _ := results[0].(map[string]any)
	value, _ := result["value"].([]any)
	if len(value) != 2 {
		return 0, true
	}
	switch raw := value[1].(type) {
	case string:
		parsed, err := strconv.ParseFloat(raw, 64)
		return parsed, err == nil
	case float64:
		return raw, true
	default:
		return 0, true
	}
}

func checkFederatedMetricsReady(dir, name, label, component, expectedEnv string, defaultExpected int) EvidenceCheck {
	path := filepath.Join(dir, name)
	expected, err := expectedReplicaCount(expectedEnv, defaultExpected)
	if err != nil {
		return EvidenceCheck{Name: label, Status: "INVALID", Source: path, Message: err.Error()}
	}
	samples, err := parsePrometheusFile(path)
	if err != nil {
		return EvidenceCheck{Name: label, Status: "MISSING", Source: path, Message: err.Error()}
	}
	discoveredInstances := map[string]float64{}
	fallbackInstances := map[string]float64{}
	for _, sample := range samples {
		if sample.Name != "qs_runtime_component_ready" {
			continue
		}
		instances := fallbackInstances
		if sample.Labels["exported_component"] == component {
			instances = discoveredInstances
		} else if sample.Labels["component"] != component {
			continue
		}
		instance := strings.TrimSpace(sample.Labels["instance"])
		if instance == "" {
			return EvidenceCheck{
				Name: label, Status: "INVALID", Source: path,
				Message: "metrics have no instance label; a direct load-balanced /metrics endpoint is not valid multi-replica evidence",
			}
		}
		if previous, exists := instances[instance]; exists && previous != sample.Value {
			return EvidenceCheck{Name: label, Status: "INVALID", Source: path, Message: "conflicting readiness samples for instance " + instance}
		}
		instances[instance] = sample.Value
	}
	instances := discoveredInstances
	if len(instances) == 0 {
		instances = fallbackInstances
	}
	if len(instances) != expected {
		return EvidenceCheck{Name: label, Status: "FAIL", Source: path, Message: fmt.Sprintf("observed replicas=%d, expected=%d", len(instances), expected)}
	}
	for instance, ready := range instances {
		if ready != 1 {
			return EvidenceCheck{Name: label, Status: "FAIL", Source: path, Message: fmt.Sprintf("instance %s runtime ready=%g", instance, ready)}
		}
	}
	return EvidenceCheck{Name: label, Status: "PASS", Source: path, Message: fmt.Sprintf("ready replicas=%d", expected)}
}

func expectedReplicaCount(envName string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(envName))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 {
		return 0, fmt.Errorf("%s must be a positive integer, got %q", envName, raw)
	}
	return value, nil
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
			for index := range samples {
				if samples[index].Labels == nil {
					samples[index].Labels = map[string]string{}
				}
				samples[index].Labels[snapshotComponentLabel] = component
			}
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
	defer func() { _ = file.Close() }()
	result := make([]metricSample, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		identity, value, ok := parsePrometheusSampleLine(line)
		if !ok {
			continue
		}
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

func parsePrometheusSampleLine(line string) (string, float64, bool) {
	lastSpace := strings.LastIndexAny(line, " \t")
	if lastSpace < 0 {
		return "", 0, false
	}
	lastValue, err := strconv.ParseFloat(strings.TrimSpace(line[lastSpace+1:]), 64)
	if err != nil {
		return "", 0, false
	}
	identity := strings.TrimSpace(line[:lastSpace])
	value := lastValue
	// Prometheus federation appends a millisecond timestamp after the sample
	// value. Direct /metrics output has no timestamp. Detect the optional
	// penultimate numeric field without breaking label values that contain spaces.
	if valueSpace := strings.LastIndexAny(identity, " \t"); valueSpace >= 0 {
		if sampleValue, parseErr := strconv.ParseFloat(strings.TrimSpace(identity[valueSpace+1:]), 64); parseErr == nil {
			identity = strings.TrimSpace(identity[:valueSpace])
			value = sampleValue
		}
	}
	return identity, value, identity != ""
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
	topics := nsqTopics(payload)
	if len(topics) == 0 {
		return 0, false
	}
	allowed := configuredNSQTopics()
	total := 0.0
	matched := 0
	for _, value := range topics {
		topic, ok := value.(map[string]any)
		if !ok {
			continue
		}
		name, _ := topic["topic_name"].(string)
		if name == "" {
			name, _ = topic["name"].(string)
		}
		if len(allowed) > 0 {
			if _, ok := allowed[name]; !ok {
				continue
			}
		}
		matched++
		channels, _ := topic["channels"].([]any)
		if len(channels) == 0 {
			total += jsonNumber(topic["depth"])
			continue
		}
		for _, channelValue := range channels {
			channel, _ := channelValue.(map[string]any)
			total += jsonNumber(channel["depth"])
		}
	}
	return total, matched > 0
}

func nsqTopics(payload any) []any {
	root, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	if topics, ok := root["topics"].([]any); ok {
		return topics
	}
	data, _ := root["data"].(map[string]any)
	topics, _ := data["topics"].([]any)
	return topics
}

func configuredNSQTopics() map[string]struct{} {
	result := map[string]struct{}{}
	for _, value := range strings.Split(os.Getenv("PERF_NSQ_TOPICS"), ",") {
		value = strings.TrimSpace(value)
		if value != "" {
			result[value] = struct{}{}
		}
	}
	return result
}

func jsonNumber(value any) float64 {
	number, _ := value.(float64)
	return number
}

func nonNegativeDelta(before, after float64) float64 {
	if after < before {
		return after
	}
	return after - before
}

func recoveryVerdict(baseline, current PhaseEvidence) Verdict {
	reasons := make([]string, 0)
	baselineVerdict := classifyRecoveryEvidence(baseline, "baseline recovery evidence")
	currentVerdict := classifyRecoveryEvidence(current, "recovery evidence")
	if baselineVerdict.Status != VerdictPass {
		reasons = append(reasons, baselineVerdict.Reasons...)
	}
	if currentVerdict.Status != VerdictPass {
		reasons = append(reasons, currentVerdict.Reasons...)
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
	status := worseVerdict(baselineVerdict.Status, currentVerdict.Status)
	if status == VerdictPass {
		status = VerdictFail
	}
	return Verdict{Status: status, Reasons: uniqueStrings(reasons)}
}

var recoveryEvidenceChecks = []string{
	"collection readyz",
	"apiserver readyz",
	"worker readyz",
	"collection metrics replicas",
	"apiserver metrics",
	"worker metrics replicas",
	"NSQD stats",
	"component process continuity",
	"traffic isolation",
	"outbox backlog window",
	"NSQ depth window",
}

func classifyRecoveryEvidence(evidence PhaseEvidence, subject string) Verdict {
	checks := make(map[string]EvidenceCheck, len(evidence.Checks))
	for _, check := range evidence.Checks {
		checks[check.Name] = check
	}
	failures := make([]string, 0)
	incomplete := make([]string, 0)
	for _, name := range recoveryEvidenceChecks {
		check, found := checks[name]
		if !found {
			incomplete = append(incomplete, fmt.Sprintf("%s is missing %s", subject, name))
			continue
		}
		switch check.Status {
		case "PASS":
		case "FAIL":
			failures = append(failures, fmt.Sprintf("%s failed: %s", subject, name))
		default:
			incomplete = append(incomplete, fmt.Sprintf("%s is incomplete: %s", subject, name))
		}
	}
	fields := []struct {
		name  string
		value *float64
	}{
		{name: "outbox backlog", value: evidence.OutboxBacklog},
		{name: "outbox oldest age", value: evidence.OutboxOldestAge},
		{name: "NSQ depth", value: evidence.NSQDepth},
	}
	for _, field := range fields {
		if field.value == nil {
			incomplete = append(incomplete, fmt.Sprintf("%s is missing %s", subject, field.name))
		}
	}
	if len(failures) > 0 {
		return Verdict{Status: VerdictFail, Reasons: uniqueStrings(failures)}
	}
	if len(incomplete) > 0 {
		return Verdict{Status: VerdictIncomplete, Reasons: uniqueStrings(incomplete)}
	}
	return Verdict{Status: VerdictPass, Reasons: []string{subject + " is complete"}}
}

func classifyEvidence(evidence PhaseEvidence, subject string) Verdict {
	failed := make([]string, 0)
	incomplete := make([]string, 0)
	for _, check := range evidence.Checks {
		switch check.Status {
		case "PASS":
		case "FAIL":
			failed = append(failed, fmt.Sprintf("%s failed: %s", subject, check.Name))
		default:
			reason := fmt.Sprintf("%s is incomplete: %s (%s)", subject, check.Name, check.Status)
			if check.Message != "" {
				reason += ": " + check.Message
			}
			incomplete = append(incomplete, reason)
		}
	}
	if len(failed) > 0 {
		return Verdict{Status: VerdictFail, Reasons: uniqueStrings(failed)}
	}
	if len(incomplete) > 0 {
		return Verdict{Status: VerdictIncomplete, Reasons: uniqueStrings(incomplete)}
	}
	if !evidence.Complete {
		return Verdict{Status: VerdictIncomplete, Reasons: []string{subject + " is incomplete"}}
	}
	return Verdict{Status: VerdictPass, Reasons: []string{subject + " is complete"}}
}
