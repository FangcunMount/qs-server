package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type runOptions struct {
	Plan       string
	Case       string
	Root       string
	ConfigFile string
	OutputRoot string
	K6Script   string
	DryRun     bool
	Stdout     io.Writer
	Stderr     io.Writer
}

type diagnosticCase struct {
	Command string
	Args    []string
	Env     []string
}

var diagnosticCases = map[string]diagnosticCase{
	"submit-coalescing-healthy":              {Command: "scripts/perf/run-submit-coalescing.sh", Env: []string{"COALESCING_SCENARIO=healthy"}},
	"submit-coalescing-conflict":             {Command: "scripts/perf/run-submit-coalescing.sh", Env: []string{"COALESCING_SCENARIO=conflict"}},
	"submit-coalescing-redis-lock-failure":   {Command: "scripts/perf/run-submit-coalescing.sh", Env: []string{"COALESCING_SCENARIO=redis_lock_failure"}},
	"submit-coalescing-redis-signal-failure": {Command: "scripts/perf/run-submit-coalescing.sh", Env: []string{"COALESCING_SCENARIO=redis_signal_failure"}},
	"submit-coalescing-redis-unavailable":    {Command: "scripts/perf/run-submit-coalescing.sh", Env: []string{"COALESCING_SCENARIO=redis_unavailable"}},
	"submit-redis-degraded-low":              {Command: "scripts/perf/run-submit-redis-degraded.sh", Env: []string{"DEGRADED_SUBMIT_MODE=low"}},
	"submit-redis-degraded-global":           {Command: "scripts/perf/run-submit-redis-degraded.sh", Env: []string{"DEGRADED_SUBMIT_MODE=global_overload"}},
	"submit-redis-degraded-user":             {Command: "scripts/perf/run-submit-redis-degraded.sh", Env: []string{"DEGRADED_SUBMIT_MODE=user_overload"}},
	"collection-runtime-status":              {Command: "scripts/perf/run-collection-runtime-acceptance.sh", Args: []string{"status"}},
	"collection-runtime-healthy-smoke":       {Command: "scripts/perf/run-collection-runtime-acceptance.sh", Args: []string{"healthy-smoke"}},
	"collection-runtime-healthy":             {Command: "scripts/perf/run-collection-runtime-acceptance.sh", Args: []string{"healthy"}},
	"collection-runtime-degraded-low":        {Command: "scripts/perf/run-collection-runtime-acceptance.sh", Args: []string{"degraded-low"}},
	"collection-runtime-degraded-global":     {Command: "scripts/perf/run-collection-runtime-acceptance.sh", Args: []string{"degraded-global"}},
	"collection-runtime-degraded-user":       {Command: "scripts/perf/run-collection-runtime-acceptance.sh", Args: []string{"degraded-user"}},
	"collection-runtime-recovery":            {Command: "scripts/perf/run-collection-runtime-acceptance.sh", Args: []string{"recovery"}},
	"grpc":                                   {Command: "scripts/perf/ghz-qs-grpc.sh"},
}

func execute(ctx context.Context, opts runOptions) (RunSummary, int, error) {
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	if opts.Stderr == nil {
		opts.Stderr = os.Stderr
	}
	if opts.Plan == "diagnose" {
		return runDiagnostic(ctx, opts)
	}
	startedAt := time.Now()
	gitSHA, gitDirty := gitIdentity(ctx, opts.Root)
	runID := fmt.Sprintf("%s-%s", startedAt.Format("20060102-150405.000000000"), shortSHA(gitSHA))
	runDir := filepath.Join(opts.OutputRoot, runID)
	if err := os.MkdirAll(runDir, 0o750); err != nil {
		return RunSummary{}, 4, err
	}
	phases, err := phasesForPlan(opts.Plan)
	if err != nil {
		return finishSetupError(opts, runDir, runID, gitSHA, gitDirty, startedAt, opts.ConfigFile, "", "", err)
	}
	effectiveConfig := filepath.Join(runDir, "effective-config.json")
	config, err := loadAndPrepareConfig(opts.ConfigFile, effectiveConfig, phases)
	if err != nil {
		return finishSetupError(opts, runDir, runID, gitSHA, gitDirty, startedAt, opts.ConfigFile, "", "", err)
	}
	k6Version, err := checkK6Version(ctx)
	if err != nil && !opts.DryRun {
		return finishSetupError(opts, runDir, runID, gitSHA, gitDirty, startedAt, effectiveConfig, environmentLabel(config), k6Version, err)
	}
	if opts.DryRun {
		_, _ = fmt.Fprintf(opts.Stdout, "run_id=%s plan=%s output=%s\n", runID, opts.Plan, runDir)
		for _, phase := range phases {
			_, _ = fmt.Fprintf(opts.Stdout, "%s profile=%s target=%d duration=%s tier=%s\n", phase.ID, phase.Profile, phase.TargetQPS, phase.Duration, phase.ThresholdTier)
		}
		return RunSummary{SchemaVersion: reportSchemaVersion, Run: RunMetadata{ID: runID, Plan: opts.Plan, GitSHA: gitSHA, GitDirty: gitDirty, K6Version: k6Version, StartedAt: startedAt, FinishedAt: time.Now(), ConfigFile: effectiveConfig}}, 0, nil
	}
	summary := RunSummary{
		SchemaVersion: reportSchemaVersion,
		Run: RunMetadata{
			ID: runID, Plan: opts.Plan, GitSHA: gitSHA, GitDirty: gitDirty,
			Environment: environmentLabel(config), K6Version: k6Version,
			StartedAt: startedAt, ConfigFile: effectiveConfig,
		},
		Recovery: make([]RecoverySummary, 0),
	}
	rootEvidenceDir := filepath.Join(runDir, "run-baseline")
	_ = os.MkdirAll(rootEvidenceDir, 0o750)
	_ = takeSnapshot(ctx, opts.Root, rootEvidenceDir, "before", opts.Stdout, opts.Stderr)
	_ = takeSnapshot(ctx, opts.Root, rootEvidenceDir, "after", opts.Stdout, opts.Stderr)
	baselineEvidence := collectPhaseEvidence(rootEvidenceDir)
	rawByPhase := map[string]any{}
	stop := false
	for phaseIndex, spec := range phases {
		if stop {
			break
		}
		if spec.ID == "admission_300" {
			prerequisite := admissionPrerequisiteVerdict(phases[:phaseIndex], summary.Phases)
			if prerequisite.Status != VerdictPass {
				now := time.Now()
				summary.Recovery = append(summary.Recovery, RecoverySummary{
					ID: "pre-admission-300", Verdict: prerequisite, StartedAt: now, FinishedAt: now,
				})
				_, _ = fmt.Fprintf(opts.Stdout, "\nadmission_300 skipped: %s\n", strings.Join(prerequisite.Reasons, "; "))
				break
			}
			recovery := waitForRecovery(ctx, opts, runDir, "pre-admission-300", baselineEvidence, filepath.Join(runDir, "capacity_280"))
			summary.Recovery = append(summary.Recovery, recovery)
			if recovery.Verdict.Status != VerdictPass {
				break
			}
		}
		phaseDir := filepath.Join(runDir, spec.ID)
		if err := os.MkdirAll(phaseDir, 0o750); err != nil {
			return summary, 4, err
		}
		_ = takeSnapshot(ctx, opts.Root, phaseDir, "before", opts.Stdout, opts.Stderr)
		phaseStarted := time.Now()
		rawPath := filepath.Join(phaseDir, "raw-k6-summary.json")
		exitCode := runK6(ctx, opts, effectiveConfig, runID, spec, rawPath)
		phaseFinished := time.Now()
		_ = takeSnapshot(ctx, opts.Root, phaseDir, "after", opts.Stdout, opts.Stderr)
		raw, readErr := readRawSummary(rawPath)
		if readErr != nil {
			phase := PhaseSummary{ID: spec.ID, Profile: spec.Profile, TargetQPS: spec.TargetQPS, Duration: spec.Duration, ActualDuration: measured(floatPtr(phaseFinished.Sub(phaseStarted).Seconds()), "seconds", "orchestrator:phase_wall_time"), ThresholdTier: spec.ThresholdTier, StartedAt: phaseStarted, FinishedAt: phaseFinished, Verdict: Verdict{Status: VerdictError, Reasons: []string{"raw k6 summary unavailable: " + readErr.Error()}}, Evidence: collectPhaseEvidence(phaseDir)}
			summary.Phases = append(summary.Phases, phase)
			_ = writeJSON(filepath.Join(phaseDir, "summary.json"), phase)
			_ = writeJSON(filepath.Join(phaseDir, "evidence.json"), phase.Evidence)
			phaseRun := summary
			phaseRun.Phases = []PhaseSummary{phase}
			phaseRun.Verdict = phase.Verdict
			populateRunViews(&phaseRun)
			_ = os.WriteFile(filepath.Join(phaseDir, "report.md"), []byte(renderRunMarkdown(phaseRun)), 0o600)
			if spec.ID == "admission_300" {
				summary.Recovery = append(summary.Recovery, waitForRecovery(ctx, opts, runDir, "final-recovery", baselineEvidence, phaseDir))
			}
			stop = true
			continue
		}
		qps := phaseQPS(config, spec.Profile)
		evidence := collectPhaseEvidence(phaseDir)
		phase := buildPhaseSummary(spec, qps, raw, evidence, phaseStarted, phaseFinished, exitCode)
		summary.Phases = append(summary.Phases, phase)
		_ = writeJSON(filepath.Join(phaseDir, "summary.json"), phase)
		_ = writeJSON(filepath.Join(phaseDir, "evidence.json"), evidence)
		phaseRun := summary
		phaseRun.Phases = []PhaseSummary{phase}
		phaseRun.Verdict = phase.Verdict
		_ = os.WriteFile(filepath.Join(phaseDir, "report.md"), []byte(renderRunMarkdown(phaseRun)), 0o600)
		if rawBytes, err := os.ReadFile(rawPath); err == nil {
			var decoded any
			if json.Unmarshal(rawBytes, &decoded) == nil {
				rawByPhase[spec.ID] = decoded
			}
		}
		_, _ = fmt.Fprint(opts.Stdout, renderPhaseConsole(phase))
		_, _ = fmt.Fprint(opts.Stdout, renderK6NativeDiagnostics(phase, raw))
		if phase.Verdict.Status == VerdictFail || phase.Verdict.Status == VerdictError {
			stop = true
		}
		if spec.ID == "admission_300" {
			recovery := waitForRecovery(ctx, opts, runDir, "final-recovery", baselineEvidence, phaseDir)
			summary.Recovery = append(summary.Recovery, recovery)
		}
	}
	summary.Run.FinishedAt = time.Now()
	summary.Verdict = aggregateVerdict(summary)
	populateRunViews(&summary)
	if err := writeRunArtifacts(runDir, summary, rawByPhase, baselineEvidence); err != nil {
		return summary, 4, err
	}
	_, _ = fmt.Fprintf(opts.Stdout, "\nK6 admission result: %s\nreport: %s\n", summary.Verdict.Status, filepath.Join(runDir, "report.md"))
	return summary, exitCodeForVerdict(summary.Verdict.Status), nil
}

func admissionPrerequisiteVerdict(expected []phaseSpec, actual []PhaseSummary) Verdict {
	byID := make(map[string]PhaseSummary, len(actual))
	for _, phase := range actual {
		byID[phase.ID] = phase
	}
	status := VerdictPass
	reasons := make([]string, 0)
	for _, spec := range expected {
		phase, found := byID[spec.ID]
		if !found {
			status = worseVerdict(status, VerdictIncomplete)
			reasons = append(reasons, spec.ID+" was not executed")
			continue
		}
		if phase.Verdict.Status == VerdictPass {
			continue
		}
		status = worseVerdict(status, phase.Verdict.Status)
		reasons = append(reasons, fmt.Sprintf("%s verdict is %s", spec.ID, phase.Verdict.Status))
	}
	if len(reasons) == 0 {
		return Verdict{Status: VerdictPass, Reasons: []string{"all pre-admission phases passed"}}
	}
	return Verdict{Status: status, Reasons: uniqueStrings(reasons)}
}

func finishSetupError(opts runOptions, runDir, runID, gitSHA string, gitDirty bool, startedAt time.Time, configFile, environment, k6Version string, cause error) (RunSummary, int, error) {
	summary := RunSummary{
		SchemaVersion: reportSchemaVersion,
		Run: RunMetadata{
			ID: runID, Plan: opts.Plan, GitSHA: gitSHA, GitDirty: gitDirty,
			Environment: environment, K6Version: k6Version, StartedAt: startedAt,
			FinishedAt: time.Now(), ConfigFile: configFile,
		},
		Verdict:  Verdict{Status: VerdictError, Reasons: []string{cause.Error()}},
		Phases:   []PhaseSummary{},
		Recovery: []RecoverySummary{},
	}
	populateRunViews(&summary)
	if err := writeRunArtifacts(runDir, summary, map[string]any{}, PhaseEvidence{}); err != nil {
		return summary, 4, errors.Join(cause, err)
	}
	_, _ = fmt.Fprintf(opts.Stdout, "\nK6 admission result: %s\nreport: %s\n", summary.Verdict.Status, filepath.Join(runDir, "report.md"))
	return summary, 4, cause
}

func renderPhaseConsole(phase PhaseSummary) string {
	var output strings.Builder
	fmt.Fprintf(&output, "\n[%s] %s\n", phase.ID, phase.Verdict.Status)
	for _, reason := range phase.Verdict.Reasons {
		fmt.Fprintf(&output, "  结论依据: %s\n", reason)
	}
	fmt.Fprintln(&output, "  1. 吞吐与处理能力")
	writeConsoleTable(&output, "     ",
		[]string{"类别", "目标", "主要值", "次要值", "比率", "补充"},
		[][]string{
			{"QPS", formatMeasurement(phase.Throughput.BusinessQPS.Target), "实际 " + formatMeasurement(phase.Throughput.BusinessQPS.Actual), "N/A", "达成率 " + formatPercent(phase.Throughput.BusinessQPS.TargetAttainment), "Dropped " + formatMeasurement(phase.Throughput.BusinessQPS.Dropped)},
			{"TPS", "N/A", "受理 " + formatMeasurement(phase.Throughput.AcceptedTPS), "完成 " + formatMeasurement(phase.Throughput.CompletedTPS), "完成率 " + formatPercent(phase.Throughput.FinalCompletionRate), "N/A"},
			{"请求", "N/A", "HTTP " + formatMeasurement(phase.Throughput.HTTPRPS), "WebSocket " + formatMeasurement(phase.Throughput.WSSessionsPerSecond), "N/A", "N/A"},
		},
		[]bool{false, true, true, true, true, true},
	)

	fmt.Fprintln(&output, "\n  2. 时延与响应体验")
	latencyRows := make([][]string, 0, len(phase.Latency))
	for _, item := range phase.Latency {
		latencyRows = append(latencyRows, []string{
			item.Operation, strconv.FormatInt(item.Samples, 10),
			formatMeasurement(item.P50), formatMeasurement(item.P90), formatMeasurement(item.P95), formatMeasurement(item.P99),
		})
	}
	writeConsoleTable(&output, "     ",
		[]string{"操作", "样本", "P50", "P90", "P95", "P99"}, latencyRows,
		[]bool{false, true, true, true, true, true},
	)

	fmt.Fprintln(&output, "\n  3. 可靠性与正确性")
	correctnessRows := make([][]string, 0, len(phase.Correctness))
	for _, item := range phase.Correctness {
		correctnessRows = append(correctnessRows, []string{
			item.Operation, strconv.FormatInt(item.Attempts, 10),
			formatPercent(item.SuccessRate), formatPercent(item.ErrorRate), formatPercent(item.TimeoutRate),
		})
	}
	writeConsoleTable(&output, "     ",
		[]string{"操作", "初始操作", "成功率", "错误率", "超时率"}, correctnessRows,
		[]bool{false, true, true, true, true},
	)

	fmt.Fprintln(&output, "\n     重试")
	retryRows := make([][]string, 0, len(phase.Retry))
	for _, item := range phase.Retry {
		retryRows = append(retryRows, []string{
			item.Layer, strconv.FormatInt(item.InitialAttempts, 10), strconv.FormatInt(item.RetryAttempts, 10), formatPercent(item.RetryRate),
		})
	}
	writeConsoleTable(&output, "     ",
		[]string{"重试层级", "初始尝试", "重试尝试", "重试率"}, retryRows,
		[]bool{false, true, true, true},
	)
	return output.String()
}

func writeConsoleTable(output *strings.Builder, indent string, headers []string, rows [][]string, rightAlign []bool) {
	if len(headers) == 0 {
		return
	}
	widths := make([]int, len(headers))
	for index, header := range headers {
		widths[index] = consoleDisplayWidth(header)
	}
	for _, row := range rows {
		for index := 0; index < len(headers) && index < len(row); index++ {
			if width := consoleDisplayWidth(row[index]); width > widths[index] {
				widths[index] = width
			}
		}
	}

	writeConsoleBorder(output, indent, widths)
	writeConsoleRow(output, indent, headers, widths, nil)
	writeConsoleBorder(output, indent, widths)
	for _, row := range rows {
		writeConsoleRow(output, indent, row, widths, rightAlign)
	}
	writeConsoleBorder(output, indent, widths)
}

func writeConsoleBorder(output *strings.Builder, indent string, widths []int) {
	output.WriteString(indent)
	output.WriteByte('+')
	for _, width := range widths {
		output.WriteString(strings.Repeat("-", width+2))
		output.WriteByte('+')
	}
	output.WriteByte('\n')
}

func writeConsoleRow(output *strings.Builder, indent string, row []string, widths []int, rightAlign []bool) {
	output.WriteString(indent)
	output.WriteByte('|')
	for index, width := range widths {
		value := ""
		if index < len(row) {
			value = strings.ReplaceAll(row[index], "\n", " ")
		}
		paddingWidth := width - consoleDisplayWidth(value)
		if paddingWidth < 0 {
			paddingWidth = 0
		}
		padding := strings.Repeat(" ", paddingWidth)
		alignRight := index < len(rightAlign) && rightAlign[index]
		if alignRight {
			fmt.Fprintf(output, " %s%s |", padding, value)
		} else {
			fmt.Fprintf(output, " %s%s |", value, padding)
		}
	}
	output.WriteByte('\n')
}

func consoleDisplayWidth(value string) int {
	width := 0
	for _, char := range value {
		switch {
		case unicode.Is(unicode.Mn, char), unicode.Is(unicode.Me, char), unicode.IsControl(char):
			continue
		case char > unicode.MaxASCII:
			width += 2
		default:
			width++
		}
	}
	return width
}

func renderK6NativeDiagnostics(phase PhaseSummary, raw rawSummary) string {
	var output strings.Builder
	fmt.Fprintln(&output, "\n  K6 原生运行诊断")
	writeNativeOperationDiagnostics(&output, phase, raw)
	writeNativeWebSocketFailureBreakdown(&output, raw)

	fmt.Fprintln(&output, "  WEBSOCKET / K6 内置")
	writeNativeTrend(&output, "ws_connecting", findMetric(raw, "ws_connecting", nil))
	writeNativeCounter(&output, "ws_msgs_received", findMetric(raw, "ws_msgs_received", nil))
	writeNativeCounter(&output, "ws_msgs_sent", findMetric(raw, "ws_msgs_sent", nil))
	writeNativeTrend(&output, "ws_session_duration", findMetric(raw, "ws_session_duration", nil))
	writeNativeCounter(&output, "ws_sessions", findMetric(raw, "ws_sessions", nil))

	fmt.Fprintln(&output, "\n  EXECUTION")
	duration := phase.ActualDuration.Value
	if raw.State != nil && raw.State.TestRunDurationMS > 0 {
		duration = floatPtr(raw.State.TestRunDurationMS / 1000)
	}
	vus := findMetric(raw, "vus", nil)
	currentVUs := metricNumber(vus, "value")
	observedVUs := metricNumber(vus, "max")
	configuredVUs := metricNumber(findMetric(raw, "vus_max", nil), "max")
	iterations := metricNumber(findMetric(raw, "iterations", nil), "count")
	dropped := metricNumber(findMetric(raw, "dropped_iterations", nil), "count")
	fmt.Fprintf(&output, "  running (%s), %s/%s VUs (peak=%s), %s complete, %s dropped, interrupted=N/A\n",
		formatNativeDuration(duration), formatNativeInteger(currentVUs), formatNativeInteger(configuredVUs), formatNativeInteger(observedVUs),
		formatNativeInteger(iterations), formatNativeInteger(dropped))

	names := make([]string, 0, len(raw.Scenarios))
	for name := range raw.Scenarios {
		names = append(names, name)
	}
	sort.Strings(names)
	globalDroppedKnown := findMetric(raw, "dropped_iterations", nil) != nil
	for _, name := range names {
		scenario := raw.Scenarios[name]
		scenarioDropped := findMetric(raw, "dropped_iterations", []string{"scenario:" + name})
		droppedText := "N/A"
		if scenarioDropped != nil {
			droppedText = formatNativeInteger(metricNumber(scenarioDropped, "count"))
		} else if globalDroppedKnown && dropped == 0 {
			droppedText = "0"
		}
		fmt.Fprintf(&output, "  %-34s ✓ [======================================] pre/max VUs=%s/%s  %s  %s  dropped=%s\n",
			truncateNativeName(name, 34), formatNativeInteger(float64(scenario.PreAllocatedVUs)), formatNativeInteger(float64(scenario.MaxVUs)),
			scenario.Duration, formatScenarioRate(scenario), droppedText)
	}
	return output.String()
}

func writeNativeWebSocketFailureBreakdown(output *strings.Builder, raw rawSummary) {
	names := []string{
		"report_status_failed",
		"report_ws_capacity_rejected_total",
		"report_ws_rate_limited_total",
		"report_ws_protocol_error_total",
		"report_ws_transport_error_total",
		"report_ws_connect_failed_total",
		"report_ws_message_missing_total",
		"report_ws_server_rejected_total",
		"report_ws_timeout_total",
	}
	found := false
	for _, name := range names {
		if findMetric(raw, name, nil) != nil {
			found = true
			break
		}
	}
	if !found {
		return
	}
	fmt.Fprintln(output, "  WEBSOCKET / 失败分类")
	for _, name := range names {
		if metric := findMetric(raw, name, nil); metric != nil {
			writeNativeCounterWithIndent(output, "    ", name, metric)
		}
	}
	fmt.Fprintln(output)
}

var nativeOperationGroupOrder = []string{
	"QUERY / 查询",
	"SUBMIT / 提交",
	"SESSION / 会话",
	"WEBSOCKET / 报告订阅",
	"STATISTICS / 统计",
	"ASYNC CHAIN / 异步链路",
}

func writeNativeOperationDiagnostics(output *strings.Builder, phase PhaseSummary, raw rawSummary) {
	groups := make(map[string][]LatencyMetric)
	for _, latency := range phase.Latency {
		group := nativeOperationGroupFor(latency.Operation)
		if group == "" {
			continue
		}
		groups[group] = append(groups[group], latency)
	}
	correctness := make(map[string]CorrectnessMetric, len(phase.Correctness))
	for _, item := range phase.Correctness {
		correctness[item.Operation] = item
	}

	for _, title := range nativeOperationGroupOrder {
		operations := groups[title]
		if len(operations) == 0 {
			continue
		}
		fmt.Fprintf(output, "  %s\n", title)
		for _, latency := range operations {
			writeNativeOperation(output, phase, raw, latency, correctness)
		}
		fmt.Fprintln(output)
	}
}

func nativeOperationGroupFor(operation string) string {
	switch {
	case operation == "async_chain_probe":
		return "ASYNC CHAIN / 异步链路"
	case strings.Contains(operation, "report_ws_"):
		return "WEBSOCKET / 报告订阅"
	case strings.Contains(operation, "submit"):
		return "SUBMIT / 提交"
	case operation == "personality_session":
		return "SESSION / 会话"
	case strings.HasPrefix(operation, "statistics_"):
		return "STATISTICS / 统计"
	case strings.Contains(operation, "query"):
		return "QUERY / 查询"
	default:
		return ""
	}
}

func writeNativeOperation(output *strings.Builder, phase PhaseSummary, raw rawSummary, latency LatencyMetric, correctness map[string]CorrectnessMetric) {
	fmt.Fprintf(output, "    %s\n", latency.Operation)
	if latency.Operation == "async_chain_probe" {
		writeNativeTrendWithIndent(output, "      ", "report_generated_latency", findMetric(raw, "report_generated_latency", nil))
		writeNativeSamples(output, "      ", latency.Samples, phase.Duration)
		if item, ok := correctness[latency.Operation]; ok {
			writeNativeOutcome(output, "      ", item)
		}
		writeNativeChainCounters(output, raw)
		return
	}

	spec, ok := nativeOperationSpec(latency.Operation)
	if !ok {
		writeNativeTrendFromLatency(output, "      ", latency.Operation+"_duration", latency)
		writeNativeSamples(output, "      ", latency.Samples, phase.Duration)
		if item, found := correctness[latency.Operation]; found {
			writeNativeOutcome(output, "      ", item)
		}
		return
	}
	metricName := nativeTaggedMetricName(spec.TrendMetric, spec.TrendTags)
	writeNativeTrendWithIndent(output, "      ", metricName, findMetric(raw, spec.TrendMetric, spec.TrendTags))
	writeNativeSamples(output, "      ", latency.Samples, phase.Duration)
	if item, found := correctness[latency.Operation]; found {
		writeNativeOutcome(output, "      ", item)
		writeNativeFailureBreakdown(output, raw, spec)
	}
}

func nativeOperationSpec(operation string) (operationSpec, bool) {
	for _, spec := range operationSpecs {
		if spec.ID == operation {
			return spec, true
		}
	}
	return operationSpec{}, false
}

func nativeTaggedMetricName(name string, tags []string) string {
	if len(tags) == 0 {
		return name
	}
	return name + "{" + strings.Join(tags, ",") + "}"
}

func writeNativeSamples(output *strings.Builder, indent string, samples int64, plannedDuration string) {
	rate := "N/A"
	if duration, err := time.ParseDuration(strings.TrimSpace(plannedDuration)); err == nil && duration > 0 {
		rate = fmt.Sprintf("%.6f/s", float64(samples)/duration.Seconds())
	}
	fmt.Fprintf(output, "%s%-58s: %d  %s\n", indent, nativeLeader("samples", 58), samples, rate)
}

func writeNativeOutcome(output *strings.Builder, indent string, item CorrectnessMetric) {
	fmt.Fprintf(output, "%s%-58s: success=%s error=%s timeout=%s | success_rate=%s error_rate=%s timeout_rate=%s\n",
		indent, nativeLeader("outcome", 58), formatNativeCount(item.SuccessCount), formatNativeCount(item.ErrorCount),
		formatNativeCount(item.TimeoutCount), formatPercent(item.SuccessRate), formatPercent(item.ErrorRate), formatPercent(item.TimeoutRate))
}

func writeNativeFailureBreakdown(output *strings.Builder, raw rawSummary, spec operationSpec) {
	if !strings.HasSuffix(spec.TimeoutMetric, "_timeout") {
		return
	}
	prefix := strings.TrimSuffix(spec.TimeoutMetric, "_timeout")
	status4xx := findMetric(raw, prefix+"_4xx", spec.TimeoutTags)
	status5xx := findMetric(raw, prefix+"_5xx", spec.TimeoutTags)
	transport := findMetric(raw, prefix+"_transport_error", spec.TimeoutTags)
	if status4xx == nil && status5xx == nil && transport == nil {
		return
	}
	fmt.Fprintf(output, "      %-58s: 4xx=%s 5xx=%s transport=%s\n", nativeLeader("failure_breakdown", 58),
		formatNativeInteger(metricNumber(status4xx, "count")), formatNativeInteger(metricNumber(status5xx, "count")),
		formatNativeInteger(metricNumber(transport, "count")))
}

func writeNativeChainCounters(output *strings.Builder, raw rawSummary) {
	names := []string{
		"chain_probe_started", "chain_probe_accepted", "chain_probe_completed", "chain_probe_failed",
		"chain_probe_timeout", "chain_probe_final_failed", "chain_probe_poll_requests",
		"chain_probe_timeout{stage:assessment_readiness}", "chain_probe_timeout{stage:report_terminal}",
		"chain_probe_failed{reason:assessment_no_assessment_required}", "chain_probe_failed{reason:assessment_failed}",
	}
	for _, name := range names {
		writeNativeCounterWithIndent(output, "      ", name, findMetric(raw, name, nil))
	}
	for _, modelType := range []string{"medical", "behavior", "personality"} {
		for _, stage := range []string{"assessment_readiness", "report_terminal"} {
			name := fmt.Sprintf("chain_probe_timeout{stage:%s,model_type:%s}", stage, modelType)
			writeNativeCounterWithIndent(output, "      ", name, findMetric(raw, name, nil))
		}
	}
}

func writeNativeTrendFromLatency(output *strings.Builder, indent, name string, latency LatencyMetric) {
	metric := map[string]any{}
	for key, measurement := range map[string]Measurement{
		"avg": latency.Average, "med": latency.P50, "p(90)": latency.P90,
		"p(95)": latency.P95, "p(99)": latency.P99, "max": latency.Max,
	} {
		if measurement.Value != nil {
			metric[key] = *measurement.Value
		}
	}
	writeNativeTrendWithIndent(output, indent, name, metric)
}

func formatNativeCount(value *int64) string {
	if value == nil {
		return "N/A"
	}
	return strconv.FormatInt(*value, 10)
}

func writeNativeTrend(output *strings.Builder, name string, metric map[string]any) {
	writeNativeTrendWithIndent(output, "  ", name, metric)
}

func writeNativeTrendWithIndent(output *strings.Builder, indent, name string, metric map[string]any) {
	if metric == nil {
		fmt.Fprintf(output, "%s%-58s: N/A\n", indent, nativeLeader(name, 58))
		return
	}
	fmt.Fprintf(output, "%s%-58s: avg=%s min=%s med=%s p(90)=%s p(95)=%s p(99)=%s max=%s\n", indent,
		nativeLeader(name, 58), formatNativeMS(metric, "avg"), formatNativeMS(metric, "min"),
		formatNativeMS(metric, "med"), formatNativeMS(metric, "p(90)"), formatNativeMS(metric, "p(95)"),
		formatNativeMS(metric, "p(99)"), formatNativeMS(metric, "max"))
}

func writeNativeCounter(output *strings.Builder, name string, metric map[string]any) {
	writeNativeCounterWithIndent(output, "  ", name, metric)
}

func writeNativeCounterWithIndent(output *strings.Builder, indent, name string, metric map[string]any) {
	if metric == nil {
		fmt.Fprintf(output, "%s%-58s: N/A\n", indent, nativeLeader(name, 58))
		return
	}
	fmt.Fprintf(output, "%s%-58s: %s  %.6f/s\n", indent, nativeLeader(name, 58),
		formatNativeInteger(metricNumber(metric, "count")), metricNumber(metric, "rate"))
}

func nativeLeader(name string, width int) string {
	if len(name) >= width {
		return name
	}
	return name + strings.Repeat(".", width-len(name))
}

func formatNativeMS(metric map[string]any, key string) string {
	value := metricNumberPtr(metric, key)
	if value == nil {
		return "N/A"
	}
	return fmt.Sprintf("%.2fms", *value)
}

func formatNativeInteger(value float64) string {
	return strconv.FormatInt(int64(value+0.5), 10)
}

func formatNativeDuration(seconds *float64) string {
	if seconds == nil || *seconds <= 0 {
		return "N/A"
	}
	return (time.Duration(*seconds * float64(time.Second))).Round(100 * time.Millisecond).String()
}

func formatScenarioRate(scenario rawScenario) string {
	unit := strings.TrimSpace(scenario.TimeUnit)
	if unit == "" {
		unit = "1s"
	}
	parsed, err := time.ParseDuration(unit)
	if err != nil || parsed <= 0 {
		return fmt.Sprintf("%.2f iters/%s", scenario.Rate, unit)
	}
	return fmt.Sprintf("%.2f iters/s", scenario.Rate/parsed.Seconds())
}

func truncateNativeName(value string, width int) string {
	if len(value) <= width {
		return value
	}
	if width <= 3 {
		return value[:width]
	}
	return value[:width-3] + "..."
}

func latencyExtreme(items []LatencyMetric, selectValue func(LatencyMetric) Measurement) (string, Measurement) {
	name := "N/A"
	result := naMeasurement("ms", "console", "no latency samples")
	for _, item := range items {
		value := selectValue(item)
		if value.Value != nil && (result.Value == nil || *value.Value > *result.Value) {
			name, result = item.Operation, value
		}
	}
	return name, result
}

func correctnessExtreme(items []CorrectnessMetric, selectValue func(CorrectnessMetric) Measurement, highest bool) (string, Measurement) {
	name := "N/A"
	result := naMeasurement("ratio", "console", "no correctness samples")
	for _, item := range items {
		value := selectValue(item)
		if value.Value == nil || (result.Value != nil && ((highest && *value.Value <= *result.Value) || (!highest && *value.Value >= *result.Value))) {
			continue
		}
		name, result = item.Operation, value
	}
	return name, result
}

func retryExtreme(items []RetryMetric) (string, Measurement) {
	name := "N/A"
	result := naMeasurement("ratio", "console", "no retry samples")
	for _, item := range items {
		if item.RetryRate.Value != nil && (result.Value == nil || *item.RetryRate.Value > *result.Value) {
			name, result = item.Layer, item.RetryRate
		}
	}
	return name, result
}

func populateRunViews(summary *RunSummary) {
	summary.Throughput = make(map[string]Throughput, len(summary.Phases))
	summary.Latency = make(map[string][]LatencyMetric, len(summary.Phases))
	summary.Correctness = make(map[string][]CorrectnessMetric, len(summary.Phases))
	summary.Retry = make(map[string][]RetryMetric, len(summary.Phases))
	for _, phase := range summary.Phases {
		summary.Throughput[phase.ID] = phase.Throughput
		summary.Latency[phase.ID] = phase.Latency
		summary.Correctness[phase.ID] = phase.Correctness
		summary.Retry[phase.ID] = phase.Retry
	}
}

func runK6(ctx context.Context, opts runOptions, configFile, runID string, spec phaseSpec, rawPath string) int {
	args := []string{"run",
		"-e", "PERF_CONFIG_FILE=" + configFile,
		"-e", "PERF_ROOT_DIR=" + opts.Root,
		"-e", "PERF_RAW_SUMMARY_FILE=" + rawPath,
		"-e", "QPS_PROFILE=" + spec.Profile,
		"-e", "RUN_ID=" + runID + "-" + spec.ID,
		opts.K6Script,
	}
	command := exec.CommandContext(ctx, "k6", args...)
	command.Dir = opts.Root
	command.Stdout, command.Stderr = opts.Stdout, opts.Stderr
	command.Env = os.Environ()
	err := command.Run()
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	_, _ = fmt.Fprintf(opts.Stderr, "k6 execution error: %v\n", err)
	return 127
}

func takeSnapshot(ctx context.Context, root, outDir, label string, stdout, stderr io.Writer) error {
	command := exec.CommandContext(ctx, filepath.Join(root, "scripts/perf/snapshot-observability.sh"), label)
	command.Dir = root
	command.Env = append(os.Environ(), "OUT_DIR="+outDir)
	command.Stdout, command.Stderr = stdout, stderr
	return command.Run()
}

func waitForRecovery(ctx context.Context, opts runOptions, runDir, id string, baseline PhaseEvidence, drainStartDir string) RecoverySummary {
	started := time.Now()
	if !baseline.Complete {
		verdict := classifyEvidence(baseline, "baseline recovery evidence")
		return RecoverySummary{
			ID: id, StartedAt: started, FinishedAt: time.Now(), Evidence: baseline,
			Verdict: verdict,
		}
	}
	timeout := envDuration("PERF_RECOVERY_TIMEOUT", 5*time.Minute)
	poll := envDuration("PERF_RECOVERY_POLL", 10*time.Second)
	result := RecoverySummary{ID: id, StartedAt: started}
	for attempt := 1; time.Since(started) <= timeout; attempt++ {
		attemptDir := filepath.Join(runDir, id, fmt.Sprintf("attempt-%02d", attempt))
		_ = os.MkdirAll(attemptDir, 0o750)
		_ = takeSnapshot(ctx, opts.Root, attemptDir, "before", opts.Stdout, opts.Stderr)
		_ = takeSnapshot(ctx, opts.Root, attemptDir, "after", opts.Stdout, opts.Stderr)
		current := collectPhaseEvidence(attemptDir)
		if drainStartDir != "" {
			completed, failed, byModel, ok := interpretationDeltas(readMetricSnapshots(drainStartDir, "after"), readMetricSnapshots(attemptDir, "after"))
			if ok {
				current.CompletedCountDelta = completed
				current.FailedCountDelta = failed
				current.CompletedCountDeltaByModel = byModel
				if seconds, windowOK := prometheusObservationWindow(drainStartDir, "after", attemptDir, "after"); windowOK {
					current.CompletionWindow = measured(&seconds, "seconds", "prometheus:snapshot_file_mtime")
				} else {
					current.Complete = false
					current.Checks = append(current.Checks, EvidenceCheck{Name: "drain completion observation window", Status: "MISSING", Source: "after/after Prometheus snapshot timestamps", Message: "cannot calculate drain-window duration"})
				}
			} else {
				current.Complete = false
				current.Checks = append(current.Checks, EvidenceCheck{Name: "drain completion metric", Status: "MISSING", Source: "qs_interpretation_run_duration_seconds_count", Message: "cannot calculate drain-window completion delta"})
			}
		}
		result.Attempts, result.Evidence = attempt, current
		result.Verdict = recoveryVerdict(baseline, current)
		if result.Verdict.Status == VerdictPass {
			break
		}
		select {
		case <-ctx.Done():
			result.Verdict = Verdict{Status: VerdictError, Reasons: []string{ctx.Err().Error()}}
			result.FinishedAt = time.Now()
			return result
		case <-time.After(poll):
		}
	}
	result.FinishedAt = time.Now()
	return result
}

func aggregateVerdict(summary RunSummary) Verdict {
	reasons := make([]string, 0)
	status := VerdictPass
	for _, phase := range summary.Phases {
		status = worseVerdict(status, phase.Verdict.Status)
		if !phase.Evidence.Complete && phase.Verdict.Status == VerdictPass {
			status = worseVerdict(status, VerdictIncomplete)
			reasons = append(reasons, phase.ID+": service evidence is incomplete")
		}
		for _, reason := range phase.Verdict.Reasons {
			if phase.Verdict.Status != VerdictPass {
				reasons = append(reasons, phase.ID+": "+reason)
			}
		}
	}
	for _, recovery := range summary.Recovery {
		status = worseVerdict(status, recovery.Verdict.Status)
		for _, reason := range recovery.Verdict.Reasons {
			if recovery.Verdict.Status != VerdictPass {
				reasons = append(reasons, recovery.ID+": "+reason)
			}
		}
	}
	if len(summary.Phases) == 0 {
		status = VerdictError
		reasons = append(reasons, "no load phase completed")
	}
	if len(reasons) == 0 {
		reasons = []string{"all executed phases and recovery gates passed"}
	}
	return Verdict{Status: status, Reasons: uniqueStrings(reasons)}
}

func worseVerdict(left, right VerdictStatus) VerdictStatus {
	rank := map[VerdictStatus]int{VerdictPass: 0, VerdictIncomplete: 1, VerdictFail: 2, VerdictError: 3}
	if rank[right] > rank[left] {
		return right
	}
	return left
}

func exitCodeForVerdict(status VerdictStatus) int {
	switch status {
	case VerdictPass:
		return 0
	case VerdictFail:
		return 2
	case VerdictIncomplete:
		return 3
	default:
		return 4
	}
}

func writeRunArtifacts(dir string, summary RunSummary, rawByPhase map[string]any, baseline PhaseEvidence) error {
	if err := writeJSON(filepath.Join(dir, "summary.json"), summary); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "report.md"), []byte(renderRunMarkdown(summary)), 0o600); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, "raw-k6-summary.json"), rawByPhase); err != nil {
		return err
	}
	evidence := map[string]any{"baseline": baseline, "phases": map[string]PhaseEvidence{}, "recovery": summary.Recovery}
	phases := evidence["phases"].(map[string]PhaseEvidence)
	for _, phase := range summary.Phases {
		phases[phase.ID] = phase.Evidence
	}
	return writeJSON(filepath.Join(dir, "evidence.json"), evidence)
}

func runDiagnostic(ctx context.Context, opts runOptions) (RunSummary, int, error) {
	caseSpec, ok := diagnosticCases[opts.Case]
	if !ok {
		keys := make([]string, 0, len(diagnosticCases))
		for key := range diagnosticCases {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		_, _ = fmt.Fprintf(opts.Stderr, "CASE must be one of:\n  %s\n", strings.Join(keys, "\n  "))
		return RunSummary{}, 4, nil
	}
	if opts.DryRun {
		_, _ = fmt.Fprintf(opts.Stdout, "%s %s %s\n", strings.Join(caseSpec.Env, " "), caseSpec.Command, strings.Join(caseSpec.Args, " "))
		return RunSummary{}, 0, nil
	}
	startedAt := time.Now()
	gitSHA, gitDirty := gitIdentity(ctx, opts.Root)
	runID := fmt.Sprintf("%s-%s-diagnose", startedAt.Format("20060102-150405.000000000"), shortSHA(gitSHA))
	runDir := filepath.Join(opts.OutputRoot, runID)
	if err := os.MkdirAll(runDir, 0o750); err != nil {
		return RunSummary{}, 4, err
	}
	logFile, err := os.OpenFile(filepath.Join(runDir, "diagnostic.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return RunSummary{}, 4, err
	}
	defer func() { _ = logFile.Close() }()
	command := exec.CommandContext(ctx, filepath.Join(opts.Root, caseSpec.Command), caseSpec.Args...)
	command.Dir = opts.Root
	command.Env = append(os.Environ(), caseSpec.Env...)
	command.Stdout = io.MultiWriter(opts.Stdout, logFile)
	command.Stderr = io.MultiWriter(opts.Stderr, logFile)
	verdict := Verdict{Status: VerdictPass, Reasons: []string{"registered diagnostic completed successfully"}}
	if err := command.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			verdict = Verdict{Status: VerdictFail, Reasons: []string{fmt.Sprintf("diagnostic exited with code %d", exitErr.ExitCode())}}
		} else {
			verdict = Verdict{Status: VerdictError, Reasons: []string{err.Error()}}
		}
	}
	summary := RunSummary{
		SchemaVersion: reportSchemaVersion,
		Run:           RunMetadata{ID: runID, Plan: opts.Plan, GitSHA: gitSHA, GitDirty: gitDirty, Environment: "diagnostic", K6Version: "N/A", StartedAt: startedAt, FinishedAt: time.Now(), ConfigFile: opts.ConfigFile},
		Verdict:       verdict,
		Phases:        []PhaseSummary{},
		Recovery:      []RecoverySummary{},
	}
	populateRunViews(&summary)
	raw := map[string]any{"diagnostic": map[string]any{"case": opts.Case, "command": caseSpec.Command, "args": caseSpec.Args, "env_names": diagnosticEnvNames(caseSpec.Env), "log": "diagnostic.log"}}
	if err := writeRunArtifacts(runDir, summary, raw, PhaseEvidence{}); err != nil {
		return summary, 4, err
	}
	_, _ = fmt.Fprintf(opts.Stdout, "\nK6 diagnostic result: %s\nreport: %s\n", summary.Verdict.Status, filepath.Join(runDir, "report.md"))
	return summary, exitCodeForVerdict(summary.Verdict.Status), nil
}

func diagnosticEnvNames(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		name, _, _ := strings.Cut(value, "=")
		result = append(result, name)
	}
	return result
}

var k6VersionPattern = regexp.MustCompile(`(?i)k6\s+v?(\d+)\.(\d+)\.(\d+)`)

func checkK6Version(ctx context.Context) (string, error) {
	output, err := exec.CommandContext(ctx, "k6", "version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("k6 version: %w", err)
	}
	version := strings.TrimSpace(string(output))
	match := k6VersionPattern.FindStringSubmatch(version)
	if len(match) != 4 {
		return version, fmt.Errorf("cannot parse k6 version %q", version)
	}
	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])
	patch, _ := strconv.Atoi(match[3])
	if major < 1 || (major == 1 && minor < 5) {
		return version, fmt.Errorf("k6 %d.%d.%d is unsupported; minimum is 1.5.0", major, minor, patch)
	}
	return version, nil
}

func gitIdentity(ctx context.Context, root string) (string, bool) {
	shaOutput, _ := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "HEAD").Output()
	statusOutput, _ := exec.CommandContext(ctx, "git", "-C", root, "status", "--porcelain").Output()
	return strings.TrimSpace(string(shaOutput)), len(bytesTrimSpace(statusOutput)) > 0
}

func bytesTrimSpace(value []byte) []byte { return []byte(strings.TrimSpace(string(value))) }

func shortSHA(value string) string {
	if len(value) >= 8 {
		return value[:8]
	}
	if value == "" {
		return "unknown"
	}
	return value
}

func phaseQPS(config perfConfig, profile string) map[string]float64 {
	profiles, _ := config["qpsProfiles"].(map[string]any)
	value, _ := profiles[profile].(map[string]any)
	return profileQPS(value)
}

func environmentLabel(config perfConfig) string {
	collection, _ := config["collectionBaseUrl"].(string)
	apiserver, _ := config["apiserverBaseUrl"].(string)
	return fmt.Sprintf("collection=%s; apiserver=%s", collection, apiserver)
}

func envDuration(name string, fallback time.Duration) time.Duration {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
