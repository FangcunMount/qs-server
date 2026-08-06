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
		fmt.Fprintf(opts.Stdout, "run_id=%s plan=%s output=%s\n", runID, opts.Plan, runDir)
		for _, phase := range phases {
			fmt.Fprintf(opts.Stdout, "%s profile=%s target=%d duration=%s tier=%s\n", phase.ID, phase.Profile, phase.TargetQPS, phase.Duration, phase.ThresholdTier)
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
	for _, spec := range phases {
		if stop {
			break
		}
		if spec.ID == "admission_300" {
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
		fmt.Fprint(opts.Stdout, renderPhaseConsole(phase))
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
	fmt.Fprintf(opts.Stdout, "\nK6 admission result: %s\nreport: %s\n", summary.Verdict.Status, filepath.Join(runDir, "report.md"))
	return summary, exitCodeForVerdict(summary.Verdict.Status), nil
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
	fmt.Fprintf(opts.Stdout, "\nK6 admission result: %s\nreport: %s\n", summary.Verdict.Status, filepath.Join(runDir, "report.md"))
	return summary, 4, cause
}

func renderPhaseConsole(phase PhaseSummary) string {
	worstP95Operation, worstP95 := worstLatencyP95(phase.Latency)
	minSuccessOperation, minSuccess := correctnessExtreme(phase.Correctness, func(item CorrectnessMetric) Measurement { return item.SuccessRate }, false)
	maxErrorOperation, maxError := correctnessExtreme(phase.Correctness, func(item CorrectnessMetric) Measurement { return item.ErrorRate }, true)
	maxTimeoutOperation, maxTimeout := correctnessExtreme(phase.Correctness, func(item CorrectnessMetric) Measurement { return item.TimeoutRate }, true)
	maxRetryLayer, maxRetry := retryExtreme(phase.Retry)
	return fmt.Sprintf("\n[%s] %s\n  throughput: target=%d actual=%s http=%s accepted=%s completed=%s\n  latency: worst_p95=%s:%s\n  correctness: min_success=%s:%s max_error=%s:%s max_timeout=%s:%s\n  retry: max_amplification=%s:%s\n",
		phase.ID, phase.Verdict.Status, phase.TargetQPS,
		formatMeasurement(phase.Throughput.BusinessQPS.Actual), formatMeasurement(phase.Throughput.HTTPRPS), formatMeasurement(phase.Throughput.AcceptedTPS), formatMeasurement(phase.Throughput.CompletedTPS),
		worstP95Operation, formatMeasurement(worstP95),
		minSuccessOperation, formatPercent(minSuccess), maxErrorOperation, formatPercent(maxError), maxTimeoutOperation, formatPercent(maxTimeout),
		maxRetryLayer, formatPercent(maxRetry))
}

func worstLatencyP95(items []LatencyMetric) (string, Measurement) {
	name := "N/A"
	result := naMeasurement("ms", "console", "no latency samples")
	for _, item := range items {
		if item.P95.Value != nil && (result.Value == nil || *item.P95.Value > *result.Value) {
			name, result = item.Operation, item.P95
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
	args := []string{"run", "--summary-export", rawPath,
		"-e", "PERF_CONFIG_FILE=" + configFile,
		"-e", "PERF_ROOT_DIR=" + opts.Root,
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
	fmt.Fprintf(opts.Stderr, "k6 execution error: %v\n", err)
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
		fmt.Fprintf(opts.Stderr, "CASE must be one of:\n  %s\n", strings.Join(keys, "\n  "))
		return RunSummary{}, 4, nil
	}
	if opts.DryRun {
		fmt.Fprintf(opts.Stdout, "%s %s %s\n", strings.Join(caseSpec.Env, " "), caseSpec.Command, strings.Join(caseSpec.Args, " "))
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
	defer logFile.Close()
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
	fmt.Fprintf(opts.Stdout, "\nK6 diagnostic result: %s\nreport: %s\n", summary.Verdict.Status, filepath.Join(runDir, "report.md"))
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
