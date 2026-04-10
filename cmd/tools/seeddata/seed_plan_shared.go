package main

import (
	"context"
	"fmt"
	"hash/fnv"
	"math/rand"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mattn/go-isatty"
)

const (
	defaultPlanID              = "614186929759466030"
	planEnrollmentSampleRate   = 5
	planEnrollmentUnknownCount = int(^uint(0) >> 2)
	planTaskCompletionTimeout  = 5 * time.Minute
	planTaskCompletionInterval = 2 * time.Second
	planTaskCompletedOffset    = 2 * time.Hour
	planTaskTimeLayout         = "2006-01-02 15:04:05"
	planScheduleBatchFactor    = 4
	planTaskBufferFactor       = 4
	planMaxInFlightFactor      = 8
	planMinMaxInFlightTasks    = 20
	planSubmitRequestTimeout   = 15 * time.Second
	planSubmitHTTPRetryMax     = 0
	planSubmitMaxAttempts      = 2
	planSubmitRetryBackoff     = 2 * time.Second
	planSubmitCooldownBase     = 30 * time.Second
	planSubmitCooldownMax      = 2 * time.Minute
	seedPlanPaceInterval       = 3 * time.Minute
	seedPlanPaceSleep          = 15 * time.Second
	seedPlanRecoverableRetries = 3
	seedPlanRecoverableMinWait = 30 * time.Second
	seedPlanRecoverableMaxWait = 120 * time.Second
	planProcessTaskPageSize    = 100
	planProcessIdleSleep       = 30 * time.Second
	planProcessActiveSleep     = 5 * time.Second
)

type seedPlanPacerCtxKey struct{}

type seedPlanPacer struct {
	mu          sync.Mutex
	startedAt   time.Time
	interval    time.Duration
	pause       time.Duration
	nextPauseAt time.Time
	sleepUntil  time.Time
	logger      interface{ Infow(string, ...interface{}) }
	verbose     bool
}

func newSeedPlanPacer(
	startedAt time.Time,
	interval time.Duration,
	pause time.Duration,
	logger interface{ Infow(string, ...interface{}) },
	verbose bool,
) *seedPlanPacer {
	if interval <= 0 || pause <= 0 {
		return nil
	}
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	return &seedPlanPacer{
		startedAt: startedAt,
		interval:  interval,
		pause:     pause,
		logger:    logger,
		verbose:   verbose,
	}
}

func withSeedPlanPacer(ctx context.Context, pacer *seedPlanPacer) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if pacer == nil {
		return ctx
	}
	return context.WithValue(ctx, seedPlanPacerCtxKey{}, pacer)
}

func seedPlanPacerFromContext(ctx context.Context) *seedPlanPacer {
	if ctx == nil {
		return nil
	}
	pacer, _ := ctx.Value(seedPlanPacerCtxKey{}).(*seedPlanPacer)
	return pacer
}

func (p *seedPlanPacer) nextDelay(now time.Time) (time.Duration, bool) {
	if p == nil || p.interval <= 0 || p.pause <= 0 {
		return 0, false
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.startedAt.IsZero() {
		p.startedAt = now
	}
	if p.nextPauseAt.IsZero() {
		p.nextPauseAt = p.startedAt.Add(p.interval)
	}

	if !p.sleepUntil.IsZero() && now.Before(p.sleepUntil) {
		return p.sleepUntil.Sub(now), false
	}
	if now.Before(p.nextPauseAt) {
		return 0, false
	}

	p.sleepUntil = now.Add(p.pause)
	for !p.nextPauseAt.After(now) {
		p.nextPauseAt = p.nextPauseAt.Add(p.interval)
	}
	return p.pause, true
}

func (p *seedPlanPacer) Wait(ctx context.Context, reason string) error {
	if p == nil {
		return nil
	}
	delay, freshPause := p.nextDelay(time.Now())
	if delay <= 0 {
		return nil
	}

	if freshPause && p.verbose && p.logger != nil {
		p.logger.Infow("Seed plan pacing pause",
			"reason", reason,
			"pause_seconds", int(delay.Seconds()),
			"interval_seconds", int(p.interval.Seconds()),
		)
	}
	return sleepWithContext(ctx, delay)
}

func waitForSeedPlanPacer(ctx context.Context, reason string) error {
	return seedPlanPacerFromContext(ctx).Wait(ctx, reason)
}

type seedPlanCreateResult struct {
	ShouldProcess  bool
	ScopeTesteeIDs []string
}

type planTaskJob struct {
	testeeID string
	task     TaskResponse
}

type planTaskStatusStats struct {
	Total     int `json:"total"`
	Pending   int `json:"pending"`
	Opened    int `json:"opened"`
	Completed int `json:"completed"`
	Expired   int `json:"expired"`
	Canceled  int `json:"canceled"`
	Unknown   int `json:"unknown"`
}

type recoveryPlanTesteeFilterStats struct {
	ExistingTaskStats            *planTaskStatusStats `json:"existing_task_stats"`
	FilteredCompletedPlanTestees int                  `json:"filtered_completed_plan_testees"`
	FilteredNoTaskTestees        int                  `json:"filtered_no_task_testees"`
	RetainedUndeterminedTestees  int                  `json:"retained_undetermined_testees"`
}

type seedPlanExecutionStats struct {
	OpenedCount           int
	ScheduleStats         *TaskScheduleStatsResponse
	SubmittedCount        int
	SkippedCount          int
	CompletedCount        int
	ExpiredCount          int
	RecoveredCount        int
	MaxInFlightObserved   int
	FailedEnrollments     int
	FailedScheduleBatches int
	FailedTaskListLoads   int
	FailedTaskExecutions  int
}

type planTaskWaitJob struct {
	testeeID string
	task     TaskResponse
	attempts int
}

type seedPlanSubmitController struct {
	pauseUntilNanos         atomic.Int64
	consecutiveRecoverables atomic.Int64
}

func newSeedPlanSubmitController() *seedPlanSubmitController {
	return &seedPlanSubmitController{}
}

func (c *seedPlanSubmitController) Wait(ctx context.Context) error {
	if c == nil {
		return nil
	}
	for {
		untilNanos := c.pauseUntilNanos.Load()
		if untilNanos <= 0 {
			return nil
		}
		until := time.Unix(0, untilNanos)
		delay := time.Until(until)
		if delay <= 0 {
			c.pauseUntilNanos.CompareAndSwap(untilNanos, 0)
			return nil
		}
		if err := sleepWithContext(ctx, delay); err != nil {
			return err
		}
	}
}

func (c *seedPlanSubmitController) OnSuccess() {
	if c == nil {
		return
	}
	c.consecutiveRecoverables.Store(0)
}

func (c *seedPlanSubmitController) OnRecoverableError(
	logger interface{ Warnw(string, ...interface{}) },
	planID string,
	orgID int64,
	taskID string,
	err error,
) {
	if c == nil {
		return
	}
	streak := c.consecutiveRecoverables.Add(1)
	delay := planSubmitCooldownBase
	for i := int64(1); i < streak; i++ {
		delay *= 2
		if delay >= planSubmitCooldownMax {
			delay = planSubmitCooldownMax
			break
		}
	}
	until := time.Now().Add(delay)
	for {
		current := c.pauseUntilNanos.Load()
		if current >= until.UnixNano() {
			break
		}
		if c.pauseUntilNanos.CompareAndSwap(current, until.UnixNano()) {
			break
		}
	}
	logger.Warnw("Seed plan submit cooldown activated",
		"plan_id", planID,
		"org_id", orgID,
		"task_id", taskID,
		"cooldown_seconds", int(delay.Seconds()),
		"recoverable_submit_failures", streak,
		"error", err.Error(),
	)
}

func normalizePlanTaskExecutionConcurrency(workers, submitWorkers, waitWorkers, maxInFlightTasks int) (int, int, int) {
	baseWorkers := workers
	if baseWorkers <= 0 {
		baseWorkers = 1
	}

	if submitWorkers <= 0 {
		submitWorkers = baseWorkers
	}
	if waitWorkers <= 0 {
		waitWorkers = baseWorkers
	}
	if submitWorkers <= 0 {
		submitWorkers = 1
	}
	if waitWorkers <= 0 {
		waitWorkers = 1
	}

	if maxInFlightTasks <= 0 {
		maxInFlightTasks = max(submitWorkers, waitWorkers) * planMaxInFlightFactor
		if maxInFlightTasks < planMinMaxInFlightTasks {
			maxInFlightTasks = planMinMaxInFlightTasks
		}
	}
	if maxInFlightTasks < submitWorkers {
		maxInFlightTasks = submitWorkers
	}
	if maxInFlightTasks < waitWorkers {
		maxInFlightTasks = waitWorkers
	}
	return submitWorkers, waitWorkers, maxInFlightTasks
}

func updateMaxInFlightCounter(counter *atomic.Int64, current int64) {
	if counter == nil {
		return
	}
	for {
		existing := counter.Load()
		if current <= existing {
			return
		}
		if counter.CompareAndSwap(existing, current) {
			return
		}
	}
}

func mergePlanTaskStatusStats(dst *planTaskStatusStats, src *planTaskStatusStats) {
	if dst == nil || src == nil {
		return
	}
	dst.Total += src.Total
	dst.Pending += src.Pending
	dst.Opened += src.Opened
	dst.Completed += src.Completed
	dst.Expired += src.Expired
	dst.Canceled += src.Canceled
	dst.Unknown += src.Unknown
}

func mergeRecoveryPlanTesteeFilterStats(dst *recoveryPlanTesteeFilterStats, src *recoveryPlanTesteeFilterStats) {
	if dst == nil || src == nil {
		return
	}
	if dst.ExistingTaskStats == nil {
		dst.ExistingTaskStats = &planTaskStatusStats{}
	}
	mergePlanTaskStatusStats(dst.ExistingTaskStats, src.ExistingTaskStats)
	dst.FilteredCompletedPlanTestees += src.FilteredCompletedPlanTestees
	dst.FilteredNoTaskTestees += src.FilteredNoTaskTestees
	dst.RetainedUndeterminedTestees += src.RetainedUndeterminedTestees
}

func mergeTaskScheduleStats(dst *TaskScheduleStatsResponse, src *TaskScheduleStatsResponse) {
	if dst == nil || src == nil {
		return
	}
	dst.PendingCount += src.PendingCount
	dst.OpenedCount += src.OpenedCount
	dst.FailedCount += src.FailedCount
	dst.ExpiredCount += src.ExpiredCount
	dst.ExpireFailedCount += src.ExpireFailedCount
}

func runSeedPlanOperationWithRecovery(
	ctx context.Context,
	logger interface{ Warnw(string, ...interface{}) },
	verbose bool,
	operation string,
	resourceID string,
	fn func() error,
) error {
	if fn == nil {
		return fmt.Errorf("seed plan operation %s is nil", operation)
	}
	var lastErr error
	for attempt := 0; attempt <= seedPlanRecoverableRetries; attempt++ {
		if attempt > 0 {
			delay := seedPlanRecoverableDelay()
			if verbose {
				logger.Warnw("Seed plan recoverable error, waiting before retry",
					"operation", operation,
					"resource_id", resourceID,
					"attempt", attempt,
					"max_attempts", seedPlanRecoverableRetries,
					"delay_seconds", int(delay.Seconds()),
					"error", lastErr.Error(),
				)
			}
			if err := sleepWithContext(ctx, delay); err != nil {
				return err
			}
		}

		if err := fn(); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			lastErr = err
			if !isSeedPlanRecoverableError(err) || attempt == seedPlanRecoverableRetries {
				return err
			}
			continue
		}
		return nil
	}
	return lastErr
}

func isSeedPlanRecoverableError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	recoverablePatterns := []string{
		"context deadline exceeded",
		"client.timeout exceeded",
		"http_status=500",
		"http_status=502",
		"http_status=503",
		"http_status=504",
		"http error: status=500",
		"http error: status=502",
		"http error: status=503",
		"http error: status=504",
		"connection reset by peer",
		"broken pipe",
		"tls handshake timeout",
		"timeout awaiting headers",
		"i/o timeout",
	}
	for _, pattern := range recoverablePatterns {
		if strings.Contains(msg, pattern) {
			return true
		}
	}
	return false
}

func seedPlanRecoverableDelay() time.Duration {
	if seedPlanRecoverableMaxWait <= seedPlanRecoverableMinWait {
		return seedPlanRecoverableMinWait
	}
	span := seedPlanRecoverableMaxWait - seedPlanRecoverableMinWait
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	return seedPlanRecoverableMinWait + time.Duration(rng.Int63n(int64(span)+1))
}

func normalizePlanScheduleBatchSize(workers int) int {
	if workers <= 0 {
		return 1
	}
	size := workers * planScheduleBatchFactor
	if size < workers {
		size = workers
	}
	return size
}

func normalizePlanTaskBufferSize(submitWorkers int, maxInFlightTasks int) int {
	if submitWorkers <= 0 {
		submitWorkers = 1
	}
	size := submitWorkers * planTaskBufferFactor
	if size < submitWorkers {
		size = submitWorkers
	}
	if maxInFlightTasks > size {
		size = maxInFlightTasks
	}
	return size
}

func normalizePlanExpireRate(rate float64) float64 {
	switch {
	case rate < 0:
		return 0
	case rate > 1:
		return 1
	default:
		return rate
	}
}

func shouldExpirePlanTask(task TaskResponse, expireRate float64) bool {
	if expireRate <= 0 {
		return false
	}
	if expireRate >= 1 {
		return true
	}

	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.TrimSpace(task.ID)))
	threshold := uint32(expireRate * 10000)
	return h.Sum32()%10000 < threshold
}

func normalizeTaskStatus(status string) string {
	return strings.ToLower(strings.TrimSpace(status))
}

func seedPlanTaskCompletedAt(task TaskResponse) string {
	plannedAt, err := time.ParseInLocation(planTaskTimeLayout, strings.TrimSpace(task.PlannedAt), time.Local)
	if err != nil || plannedAt.IsZero() {
		return ""
	}
	return plannedAt.Add(planTaskCompletedOffset).Format(time.RFC3339)
}

type planSeedDashboard struct {
	mu                   sync.Mutex
	enabled              bool
	finished             bool
	startedAt            time.Time
	stopCh               chan struct{}
	planMode             string
	totalBatches         int
	currentBatch         int
	openedTasks          int
	discoveredTasks      int
	processedTasks       int
	scheduleFailureCount int
	submitted            *atomic.Int64
	completed            *atomic.Int64
	expired              *atomic.Int64
	skipped              *atomic.Int64
	recovered            *atomic.Int64
	inflight             *atomic.Int64
	maxInflight          *atomic.Int64
	failedExecutions     *atomic.Int64
}

const planSeedDashboardRenderInterval = 5 * time.Second

func newPlanSeedDashboard(
	planMode string,
	totalBatches int,
	submitted *atomic.Int64,
	completed *atomic.Int64,
	expired *atomic.Int64,
	skipped *atomic.Int64,
	recovered *atomic.Int64,
	inflight *atomic.Int64,
	maxInflight *atomic.Int64,
	failedExecutions *atomic.Int64,
) *planSeedDashboard {
	enabled := isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd())
	dashboard := &planSeedDashboard{
		enabled:          enabled,
		startedAt:        time.Now(),
		stopCh:           make(chan struct{}),
		planMode:         strings.TrimSpace(planMode),
		totalBatches:     totalBatches,
		submitted:        submitted,
		completed:        completed,
		expired:          expired,
		skipped:          skipped,
		recovered:        recovered,
		inflight:         inflight,
		maxInflight:      maxInflight,
		failedExecutions: failedExecutions,
	}
	if dashboard.enabled {
		dashboard.renderLocked()
		dashboard.start()
	}
	return dashboard
}

func (d *planSeedDashboard) start() {
	if d == nil || !d.enabled {
		return
	}
	go func() {
		ticker := time.NewTicker(planSeedDashboardRenderInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				d.mu.Lock()
				if d.finished {
					d.mu.Unlock()
					return
				}
				d.renderLocked()
				d.mu.Unlock()
			case <-d.stopCh:
				return
			}
		}
	}()
}

func (d *planSeedDashboard) SetCurrentBatch(batch int) {
	if d == nil || !d.enabled {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.finished {
		return
	}
	if batch < 0 {
		batch = 0
	}
	if d.totalBatches > 0 && batch > d.totalBatches {
		batch = d.totalBatches
	}
	d.currentBatch = batch
}

func (d *planSeedDashboard) IncrementScheduleFailures() {
	if d == nil || !d.enabled {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.finished {
		return
	}
	d.scheduleFailureCount++
}

func (d *planSeedDashboard) AddOpenedTasks(delta int) {
	if d == nil || !d.enabled || delta <= 0 {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.finished {
		return
	}
	d.openedTasks += delta
}

func (d *planSeedDashboard) AddDiscoveredTasks(delta int) {
	if d == nil || !d.enabled || delta <= 0 {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.finished {
		return
	}
	d.discoveredTasks += delta
}

func (d *planSeedDashboard) AdvanceTask() {
	if d == nil || !d.enabled {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.finished {
		return
	}
	if d.processedTasks < d.discoveredTasks {
		d.processedTasks++
	}
}

func (d *planSeedDashboard) Refresh() {
	if d == nil || !d.enabled {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.finished {
		return
	}
}

func (d *planSeedDashboard) Finish() {
	if d == nil || !d.enabled {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.finished {
		return
	}
	d.currentBatch = d.totalBatches
	d.processedTasks = d.discoveredTasks
	d.renderLocked()
	d.finished = true
	close(d.stopCh)
}

func (d *planSeedDashboard) renderLocked() {
	if !d.enabled {
		return
	}

	elapsed := time.Since(d.startedAt).Round(time.Second)
	planLine := fmt.Sprintf(
		"plan(%s)      [%s] %d/%d batches elapsed=%s opened=%d schedule_failures=%d",
		d.planModeLabel(),
		renderDashboardBar(d.currentBatch, d.totalBatches, 24),
		d.currentBatch,
		max(d.totalBatches, 0),
		elapsed,
		d.openedTasks,
		d.scheduleFailureCount,
	)

	inflight := atomicLoadInt64(d.inflight)
	maxInflight := atomicLoadInt64(d.maxInflight)
	taskLine := fmt.Sprintf(
		"task-flow(remote) [%s] %d/%d tasks inflight=%d max=%d",
		renderDashboardBar(d.processedTasks, d.discoveredTasks, 24),
		d.processedTasks,
		d.discoveredTasks,
		inflight,
		maxInflight,
	)

	statsLine := fmt.Sprintf(
		"stats          submitted=%d completed=%d expired=%d skipped=%d recovered=%d failed=%d",
		atomicLoadInt64(d.submitted),
		atomicLoadInt64(d.completed),
		atomicLoadInt64(d.expired),
		atomicLoadInt64(d.skipped),
		atomicLoadInt64(d.recovered),
		atomicLoadInt64(d.failedExecutions),
	)

	fmt.Fprintf(os.Stderr, "%s\n%s\n%s\n", planLine, taskLine, statsLine)
}

func (d *planSeedDashboard) planModeLabel() string {
	mode := strings.ToLower(strings.TrimSpace(d.planMode))
	if mode == "" {
		return "unknown"
	}
	return mode
}

func renderDashboardBar(current, total, width int) string {
	if width <= 0 {
		width = 24
	}
	if total <= 0 {
		total = max(current, 1)
	}
	if current < 0 {
		current = 0
	}
	if current > total {
		current = total
	}
	progressRatio := float64(current) / float64(total)
	filled := int(progressRatio * float64(width))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	return strings.Repeat("=", filled) + strings.Repeat(" ", width-filled)
}

func atomicLoadInt64(counter *atomic.Int64) int64 {
	if counter == nil {
		return 0
	}
	return counter.Load()
}

func planStartDateFromAuditTimes(createdAt, updatedAt, now time.Time) (string, string, error) {
	switch {
	case !createdAt.IsZero():
		return createdAt.In(time.Local).Format("2006-01-02"), "created_at", nil
	case !updatedAt.IsZero():
		return updatedAt.In(time.Local).Format("2006-01-02"), "updated_at", nil
	case !now.IsZero():
		return now.In(time.Local).Format("2006-01-02"), "now", nil
	default:
		return "", "", fmt.Errorf("created_at and updated_at are both zero")
	}
}

func newPlanQuestionnaireVersionMismatchError(
	scaleCode string,
	questionnaireCode string,
	scaleQuestionnaireVersion string,
	loadedQuestionnaireVersion string,
) error {
	normalizedScaleCode := strings.ToLower(strings.TrimSpace(scaleCode))
	return fmt.Errorf(
		"questionnaire version mismatch for plan backfill: scale_code=%s questionnaire_code=%s scale_questionnaire_version=%s loaded_questionnaire_version=%s; seeddata loads questionnaire detail by code only, so this usually means the scale still comes from apiserver Redis cache or the scale is bound to a different questionnaire version; if you changed scale.questionnaire_version directly in MongoDB, delete Redis key scale:%s (or <cache.namespace>:scale:%s) and retry",
		scaleCode,
		questionnaireCode,
		scaleQuestionnaireVersion,
		loadedQuestionnaireVersion,
		normalizedScaleCode,
		normalizedScaleCode,
	)
}

func newExplicitPlanZeroCreatedAtError(testeeID string) error {
	return fmt.Errorf(
		"explicit plan backfill requires non-zero created_at: testee_id=%s; seeddata refuses to fall back to updated_at/now when --plan-testee-ids is used; if the database already has created_at, refresh /api/v1/testees/%s or delete Redis key testee:info:%s (or <cache.namespace>:testee:info:%s) and retry",
		testeeID,
		testeeID,
		testeeID,
		testeeID,
	)
}
