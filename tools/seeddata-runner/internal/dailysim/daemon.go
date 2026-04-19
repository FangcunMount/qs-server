package dailysim

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	toolprogress "github.com/FangcunMount/qs-server/tools/seeddata-runner/internal/progress"
)

const (
	dailySimulationDaemonDefaultRunAt      = "10:00"
	dailySimulationDaemonDefaultRetryDelay = 30 * time.Minute
	dailySimulationDaemonDefaultStateFile  = ".seeddata-cache/daily-simulation-daemon-state.json"
)

type dailySimulationScenario struct {
	ClinicianID string
	Entry       *AssessmentEntryResponse
	Target      *dailySimulationResolvedTarget
}

type dailySimulationDaemonState struct {
	LastSuccessDate string    `json:"lastSuccessDate"`
	LastSuccessAt   time.Time `json:"lastSuccessAt"`
}

type dailySimulationRunClock struct {
	Hour   int
	Minute int
}

/**
 * 模拟每日用户守护进程
 *
 * @param ctx 上下文
 * @param deps 依赖
 */
func seedDailySimulationDaemon(ctx context.Context, deps *dependencies) error {
	cfg := deps.Config.DailySimulation
	if cfg.IsZero() {
		return fmt.Errorf("dailySimulation config is required for daily_simulation_daemon step")
	}

	runClock, err := parseDailySimulationRunClock(cfg.RunAt)
	if err != nil {
		return err
	}
	retryDelay, err := parseDailySimulationRetryDelay(cfg.RetryDelay)
	if err != nil {
		return err
	}
	statePath := normalizeDailySimulationStateFile(cfg.StateFile)

	deps.Logger.Infow("Daily simulation daemon started",
		"run_at", fmt.Sprintf("%02d:%02d", runClock.Hour, runClock.Minute),
		"retry_delay", retryDelay.String(),
		"state_file", statePath,
		"count_min", cfg.CountMin,
		"count_max", cfg.CountMax,
		"count_per_run", cfg.CountPerRun,
		"focus_clinicians_per_run_min", cfg.FocusCliniciansPerRunMin,
		"focus_clinicians_per_run_max", cfg.FocusCliniciansPerRunMax,
	)

	for {
		now := time.Now().In(time.Local)
		state, err := loadDailySimulationDaemonState(statePath)
		if err != nil {
			return err
		}

		runDate, waitDuration := nextDailySimulationDaemonRun(now, runClock, state.LastSuccessDate)
		if waitDuration > 0 {
			deps.Logger.Infow("Daily simulation daemon waiting for next run",
				"next_run_date", runDate.Format("2006-01-02"),
				"next_run_at", time.Date(runDate.Year(), runDate.Month(), runDate.Day(), runClock.Hour, runClock.Minute, 0, 0, time.Local).Format(time.RFC3339),
				"sleep", waitDuration.String(),
			)
			if err := sleepWithContext(ctx, waitDuration); err != nil {
				return err
			}
			continue
		}

		count, err := resolveDailySimulationBatchCount(cfg, runDate)
		if err != nil {
			return err
		}

		if err := runDailySimulationBatch(ctx, deps, cfg, runDate, count, "daily_simulation_daemon"); err != nil {
			deps.Logger.Warnw("Daily simulation daemon batch failed",
				"run_date", runDate.Format("2006-01-02"),
				"count", count,
				"retry_delay", retryDelay.String(),
				"error", err.Error(),
			)
			if err := sleepWithContext(ctx, retryDelay); err != nil {
				return err
			}
			continue
		}

		state.LastSuccessDate = runDate.Format("2006-01-02")
		state.LastSuccessAt = time.Now().In(time.Local)
		if err := saveDailySimulationDaemonState(statePath, state); err != nil {
			return err
		}
	}
}

func runDailySimulationBatch(
	ctx context.Context,
	deps *dependencies,
	cfg DailySimulationConfig,
	runDate time.Time,
	count int,
	progressLabel string,
) error {
	if count <= 0 {
		return fmt.Errorf("%s requires count > 0", progressLabel)
	}

	workers := normalizeDailySimulationWorkers(cfg.Workers, count)

	iamBundle, err := newDailySimulationIAMBundle(ctx, deps.Config.IAM, deps.Config.Global.OrgID)
	if err != nil {
		return err
	}
	defer func() {
		if iamBundle != nil && iamBundle.client != nil {
			_ = iamBundle.client.Close()
		}
	}()

	scenarios, err := resolveDailySimulationScenariosForRun(ctx, deps, cfg, runDate)
	if err != nil {
		return err
	}
	if len(scenarios) == 0 {
		return fmt.Errorf("%s resolved zero scenarios", progressLabel)
	}

	progress := toolprogress.New(progressLabel+" users", count)
	defer progress.Close()

	jobs := make(chan int)
	var wg sync.WaitGroup
	var counters dailySimulationCounters
	var failureMu sync.Mutex
	failures := make([]string, 0, 8)

	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}

				profile := buildDailySimulationProfile(cfg, runDate, idx)
				scenario := scenarios[idx%len(scenarios)]
				outcome, simErr := simulateDailyUser(
					ctx,
					deps,
					iamBundle,
					cfg,
					profile,
					scenario.ClinicianID,
					scenario.Entry,
					scenario.Target,
				)
				if simErr != nil {
					deps.Logger.Warnw("Daily simulation user failed",
						"index", profile.Index,
						"run_date", runDate.Format("2006-01-02"),
						"guardian_phone", profile.GuardianPhone,
						"guardian_email", profile.GuardianEmail,
						"child_name", profile.ChildName,
						"clinician_id", scenario.ClinicianID,
						"entry_id", scenario.Entry.ID,
						"target_code", scenario.Target.TargetCode,
						"journey_target", outcome.JourneyTarget,
						"error", simErr.Error(),
					)
					counters.addFailure()
					failureMu.Lock()
					if len(failures) < 8 {
						failures = append(failures, fmt.Sprintf("idx=%d guardian=%s child=%s clinician=%s journey=%s err=%v", profile.Index, profile.GuardianEmail, profile.ChildName, scenario.ClinicianID, outcome.JourneyTarget, simErr))
					}
					failureMu.Unlock()
				} else {
					counters.add(outcome)
				}
				progress.Increment()
			}
		}()
	}

	for idx := 0; idx < count; idx++ {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return ctx.Err()
		case jobs <- idx:
		}
	}
	close(jobs)
	wg.Wait()
	progress.Complete()

	selectedClinicians := make([]string, 0, len(scenarios))
	for _, scenario := range scenarios {
		selectedClinicians = append(selectedClinicians, strings.TrimSpace(scenario.ClinicianID))
	}

	deps.Logger.Infow("Daily simulation batch completed",
		"label", progressLabel,
		"run_date", runDate.Format("2006-01-02"),
		"count", count,
		"workers", workers,
		"selected_clinicians", selectedClinicians,
		"users_created", atomic.LoadInt64(&counters.userCreated),
		"children_created", atomic.LoadInt64(&counters.childCreated),
		"testees_created", atomic.LoadInt64(&counters.testeeCreated),
		"plan_enrolled", atomic.LoadInt64(&counters.enrolled),
		"entries_resolved", atomic.LoadInt64(&counters.resolved),
		"entries_intaked", atomic.LoadInt64(&counters.intaked),
		"answersheets_submitted", atomic.LoadInt64(&counters.submitted),
		"submissions_skipped", atomic.LoadInt64(&counters.skippedSubmission),
		"assessments_found", atomic.LoadInt64(&counters.assessmentCreated),
		"failed", atomic.LoadInt64(&counters.failed),
	)
	if len(failures) > 0 {
		deps.Logger.Warnw("Daily simulation failure samples", "count", len(failures), "samples", failures)
	}
	if atomic.LoadInt64(&counters.failed) > 0 {
		return fmt.Errorf("%s completed with %d failures", progressLabel, atomic.LoadInt64(&counters.failed))
	}
	return nil
}

func resolveDailySimulationBatchCount(cfg DailySimulationConfig, runDate time.Time) (int, error) {
	minCount := cfg.CountMin
	maxCount := cfg.CountMax
	if minCount == 0 && maxCount == 0 {
		count := cfg.CountPerRun
		if count <= 0 {
			count = dailySimulationDefaultCount
		}
		return count, nil
	}

	switch {
	case minCount <= 0 && maxCount > 0:
		minCount = maxCount
	case maxCount <= 0 && minCount > 0:
		maxCount = minCount
	}
	if minCount <= 0 || maxCount <= 0 {
		return 0, fmt.Errorf("dailySimulation countMin/countMax must be positive")
	}
	if maxCount < minCount {
		return 0, fmt.Errorf("dailySimulation countMax must be >= countMin")
	}
	if minCount == maxCount {
		return minCount, nil
	}
	rng := newDailySimulationRand("daily-count:" + runDate.Format("20060102"))
	return minCount + rng.Intn(maxCount-minCount+1), nil
}

func resolveDailySimulationScenariosForRun(
	ctx context.Context,
	deps *dependencies,
	cfg DailySimulationConfig,
	runDate time.Time,
) ([]dailySimulationScenario, error) {
	if !cfg.EntryID.IsZero() {
		entry, target, clinicianID, err := ensureDailySimulationEntryAndTarget(ctx, deps, cfg)
		if err != nil {
			return nil, err
		}
		return []dailySimulationScenario{{
			ClinicianID: clinicianID,
			Entry:       entry,
			Target:      target,
		}}, nil
	}

	selectedClinicianIDs, err := selectDailySimulationClinicianIDsForRun(collectDailySimulationClinicianIDs(cfg.ClinicianIDs), cfg, runDate)
	if err != nil {
		return nil, err
	}

	scenarios := make([]dailySimulationScenario, 0, len(selectedClinicianIDs))
	for _, clinicianID := range selectedClinicianIDs {
		scenarioCfg := cfg
		scenarioCfg.EntryID = FlexibleID("")
		scenarioCfg.ClinicianIDs = []FlexibleID{FlexibleID(clinicianID)}

		entry, target, resolvedClinicianID, err := ensureDailySimulationEntryAndTarget(ctx, deps, scenarioCfg)
		if err != nil {
			return nil, err
		}
		scenarios = append(scenarios, dailySimulationScenario{
			ClinicianID: resolvedClinicianID,
			Entry:       entry,
			Target:      target,
		})
	}
	return scenarios, nil
}

func selectDailySimulationClinicianIDsForRun(
	clinicianIDs []string,
	cfg DailySimulationConfig,
	runDate time.Time,
) ([]string, error) {
	if len(clinicianIDs) == 0 {
		return nil, fmt.Errorf("dailySimulation clinician scope resolved zero clinicians")
	}
	if len(clinicianIDs) == 1 {
		return clinicianIDs, nil
	}

	minCount := cfg.FocusCliniciansPerRunMin
	maxCount := cfg.FocusCliniciansPerRunMax
	if minCount <= 0 && maxCount <= 0 {
		minCount = len(clinicianIDs)
		maxCount = len(clinicianIDs)
	}
	if minCount <= 0 {
		minCount = 1
	}
	if maxCount <= 0 {
		maxCount = minCount
	}
	if maxCount < minCount {
		return nil, fmt.Errorf("dailySimulation focusCliniciansPerRunMax must be >= focusCliniciansPerRunMin")
	}
	if minCount > len(clinicianIDs) {
		minCount = len(clinicianIDs)
	}
	if maxCount > len(clinicianIDs) {
		maxCount = len(clinicianIDs)
	}

	selectedCount := minCount
	if maxCount > minCount {
		rng := newDailySimulationRand("focus-clinicians:" + runDate.Format("20060102"))
		selectedCount = minCount + rng.Intn(maxCount-minCount+1)
	}

	rng := newDailySimulationRand("focus-clinician-order:" + runDate.Format("20060102"))
	order := rng.Perm(len(clinicianIDs))
	selected := make([]string, 0, selectedCount)
	for _, idx := range order[:selectedCount] {
		selected = append(selected, clinicianIDs[idx])
	}
	return selected, nil
}

func collectDailySimulationClinicianIDs(ids []FlexibleID) []string {
	collected := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		value := strings.TrimSpace(id.String())
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		collected = append(collected, value)
	}
	return collected
}

func collectDailySimulationPlanIDs(ids []FlexibleID) []string {
	collected := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		value := strings.TrimSpace(id.String())
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		collected = append(collected, value)
	}
	return collected
}

func parseDailySimulationRunClock(raw string) (dailySimulationRunClock, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = dailySimulationDaemonDefaultRunAt
	}
	parsed, err := time.ParseInLocation("15:04", raw, time.Local)
	if err != nil {
		return dailySimulationRunClock{}, fmt.Errorf("invalid dailySimulation.runAt %q: %w", raw, err)
	}
	return dailySimulationRunClock{
		Hour:   parsed.Hour(),
		Minute: parsed.Minute(),
	}, nil
}

func nextDailySimulationDaemonRun(
	now time.Time,
	runClock dailySimulationRunClock,
	lastSuccessDate string,
) (time.Time, time.Duration) {
	now = now.In(time.Local)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	todayKey := today.Format("2006-01-02")
	scheduledToday := time.Date(now.Year(), now.Month(), now.Day(), runClock.Hour, runClock.Minute, 0, 0, time.Local)

	if strings.TrimSpace(lastSuccessDate) == todayKey {
		nextRun := scheduledToday.Add(24 * time.Hour)
		return nextRun, nextRun.Sub(now)
	}
	if now.Before(scheduledToday) {
		return today, scheduledToday.Sub(now)
	}
	return today, 0
}

func parseDailySimulationRetryDelay(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return dailySimulationDaemonDefaultRetryDelay, nil
	}
	return parseSeedRelativeDuration(raw)
}

func normalizeDailySimulationStateFile(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = dailySimulationDaemonDefaultStateFile
	}
	return raw
}

func loadDailySimulationDaemonState(path string) (*dailySimulationDaemonState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &dailySimulationDaemonState{}, nil
		}
		return nil, fmt.Errorf("read daily simulation daemon state %s: %w", path, err)
	}
	var state dailySimulationDaemonState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("decode daily simulation daemon state %s: %w", path, err)
	}
	return &state, nil
}

func saveDailySimulationDaemonState(path string, state *dailySimulationDaemonState) error {
	if state == nil {
		return fmt.Errorf("daily simulation daemon state is nil")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create daily simulation daemon state dir for %s: %w", path, err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode daily simulation daemon state %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write daily simulation daemon state %s: %w", path, err)
	}
	return nil
}
