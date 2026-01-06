package main

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	questionTypeRadio    = "Radio"
	questionTypeCheckbox = "Checkbox"
	questionTypeText     = "Text"
	questionTypeTextarea = "Textarea"
	questionTypeNumber   = "Number"
	questionTypeSection  = "Section"

	questionnaireTypeMedicalScale = "MedicalScale"

	submitMaxRetry     = 3
	submitRetryBackoff = 200 * time.Millisecond

	testeeQueueSize = 100
	submitQueueSize = 200
)

type taskOutcome int

const (
	taskReady taskOutcome = iota
	taskSkipped
	taskFailed
)

type assessmentCounters struct {
	errorCount           int64
	submittedCount       int64
	skippedCount         int64
	processedTesteeCount int64
	successTesteeCount   int64
	failedTesteeCount    int64
	enqueuedTesteeCount  int64
	submitInFlight       int64
	submitMaxInFlight    int64
	errorLogCount        int64
}

type assessmentSnapshot struct {
	errorCount           int64
	submittedCount       int64
	skippedCount         int64
	processedTesteeCount int64
	successTesteeCount   int64
	failedTesteeCount    int64
	enqueuedTesteeCount  int64
	submitMaxInFlight    int64
}

func newAssessmentCounters() *assessmentCounters {
	return &assessmentCounters{}
}

func (c *assessmentCounters) Snapshot() assessmentSnapshot {
	return assessmentSnapshot{
		errorCount:           atomic.LoadInt64(&c.errorCount),
		submittedCount:       atomic.LoadInt64(&c.submittedCount),
		skippedCount:         atomic.LoadInt64(&c.skippedCount),
		processedTesteeCount: atomic.LoadInt64(&c.processedTesteeCount),
		successTesteeCount:   atomic.LoadInt64(&c.successTesteeCount),
		failedTesteeCount:    atomic.LoadInt64(&c.failedTesteeCount),
		enqueuedTesteeCount:  atomic.LoadInt64(&c.enqueuedTesteeCount),
		submitMaxInFlight:    atomic.LoadInt64(&c.submitMaxInFlight),
	}
}

func (c *assessmentCounters) AddErrors(count int64) {
	atomic.AddInt64(&c.errorCount, count)
}

func (c *assessmentCounters) AddSkipped(count int64) {
	atomic.AddInt64(&c.skippedCount, count)
}

func (c *assessmentCounters) AddSubmitted(count int64) {
	atomic.AddInt64(&c.submittedCount, count)
}

func (c *assessmentCounters) AddEnqueued(count int64) {
	atomic.AddInt64(&c.enqueuedTesteeCount, count)
}

func (c *assessmentCounters) MarkTesteeProcessed(failed bool) {
	atomic.AddInt64(&c.processedTesteeCount, 1)
	if failed {
		atomic.AddInt64(&c.failedTesteeCount, 1)
	} else {
		atomic.AddInt64(&c.successTesteeCount, 1)
	}
}

func (c *assessmentCounters) StartSubmit() int64 {
	inFlight := atomic.AddInt64(&c.submitInFlight, 1)
	updateMaxInFlight(&c.submitMaxInFlight, inFlight)
	return inFlight
}

func (c *assessmentCounters) EndSubmit() {
	atomic.AddInt64(&c.submitInFlight, -1)
}

func (c *assessmentCounters) NextErrorLogIndex() int64 {
	return atomic.AddInt64(&c.errorLogCount, 1)
}

func (c *assessmentCounters) ErrorCount() int64 {
	return atomic.LoadInt64(&c.errorCount)
}

type failureSamples struct {
	mu      sync.Mutex
	limit   int
	samples []string
}

func newFailureSamples(limit int) *failureSamples {
	return &failureSamples{limit: limit, samples: make([]string, 0, 16)}
}

func (f *failureSamples) Add(testeeID, questionnaireCode string, err error) {
	if f == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.samples) >= f.limit {
		return
	}
	entry := fmt.Sprintf("testee_id=%s questionnaire=%s error=%v", testeeID, questionnaireCode, err)
	f.samples = append(f.samples, entry)
}

func (f *failureSamples) Log(logger interface{ Warnw(string, ...interface{}) }) {
	if f == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.samples) == 0 {
		return
	}
	logger.Warnw("Assessment seeding failed samples",
		"sample_count", len(f.samples),
		"samples", f.samples,
	)
}

type assessmentDiagnostics struct {
	logger interface {
		Debugw(string, ...interface{})
		Infow(string, ...interface{})
		Warnw(string, ...interface{})
	}
	verbose bool
}

type submitLogPayload struct {
	testeeID      string
	questionnaire string
	attempts      int
	duration      time.Duration
	inFlight      int64
	maxInFlight   int64
	answerCount   int
}

type testeeLogPayload struct {
	testeeID           string
	questionnaireCount int
	duration           time.Duration
	success            bool
}

func newAssessmentDiagnostics(logger interface {
	Debugw(string, ...interface{})
	Infow(string, ...interface{})
	Warnw(string, ...interface{})
}, verbose bool) *assessmentDiagnostics {
	return &assessmentDiagnostics{logger: logger, verbose: verbose}
}

func (d *assessmentDiagnostics) LogScaleTargetsLoaded(count int, duration time.Duration) {
	d.logger.Infow("Loaded scale targets",
		"count", count,
		"duration_ms", duration.Milliseconds(),
	)
}

func (d *assessmentDiagnostics) LogNoScaleTargets(categories []string) {
	d.logger.Warnw("No medical scales found for assessment seeding", "categories", categories)
}

func (d *assessmentDiagnostics) LogProducerFinished(enqueued int64, duration time.Duration) {
	d.logger.Infow("Testee producer finished",
		"enqueued_testees", enqueued,
		"duration_ms", duration.Milliseconds(),
	)
}

func (d *assessmentDiagnostics) LogTesteeStart(testeeID string, count int) {
	if !d.verbose {
		return
	}
	d.logger.Debugw("Seeding assessments for testee", "testee_id", testeeID, "count", count)
}

func (d *assessmentDiagnostics) LogTesteeProcessed(payload testeeLogPayload) {
	if !d.verbose {
		return
	}
	d.logger.Infow("Testee processed",
		"testee_id", payload.testeeID,
		"questionnaire_count", payload.questionnaireCount,
		"duration_ms", payload.duration.Milliseconds(),
		"success", payload.success,
	)
}

func (d *assessmentDiagnostics) LogSubmitCompleted(payload submitLogPayload) {
	if !d.verbose {
		return
	}
	d.logger.Infow("Submit answersheet completed",
		"testee_id", payload.testeeID,
		"questionnaire", payload.questionnaire,
		"attempts", payload.attempts,
		"duration_ms", payload.duration.Milliseconds(),
		"in_flight", payload.inFlight,
		"max_in_flight", payload.maxInFlight,
		"answer_count", payload.answerCount,
	)
}

// seedAssessments runs a two-stage pipeline:
// 1) load testees -> build answersheets
// 2) submit answersheets -> write assessments
func seedAssessments(ctx context.Context, deps *dependencies, seedCtx *seedContext, minPerTestee, maxPerTestee, workerCount, submitWorkerCount, testeePageSize, testeeOffset, testeeLimit int, categoryFilter string, verbose bool) error {
	logger := deps.Logger
	client := deps.CollectionClient
	if client == nil {
		return fmt.Errorf("collection client is not initialized")
	}
	if deps.APIClient == nil {
		return fmt.Errorf("api client is not initialized")
	}
	orgID := deps.Config.Global.OrgID
	if orgID <= 0 {
		return fmt.Errorf("global.orgId must be set in seeddata config")
	}

	diag := newAssessmentDiagnostics(logger, verbose)
	categories := parseCategories(categoryFilter)
	targets, err := loadScaleTargets(ctx, client, categories, diag)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return nil
	}

	workerCount = normalizeWorkerCount(workerCount)
	submitWorkerCount = normalizeSubmitWorkerCount(submitWorkerCount)
	testeePageSize = normalizeTesteePageSize(testeePageSize)
	testeeOffset = normalizeTesteeOffset(testeeOffset)
	testeeLimit = normalizeTesteeLimit(testeeLimit)

	questionnaireCache := make(map[string]*QuestionnaireDetailResponse)
	var cacheMu sync.RWMutex
	var targetTestees int64
	counters := newAssessmentCounters()
	failures := newFailureSamples(100)

	// Two-stage pipeline:
	// 1) testee -> assessment task
	// 2) answersheet -> submit task
	taskCh := make(chan *TesteeResponse, testeeQueueSize)
	submitCh := make(chan submissionTask, submitQueueSize)
	var wg sync.WaitGroup
	var submitWG sync.WaitGroup

	logger.Infow("Assessment seeding started",
		"categories", categories,
		"scale_count", len(targets),
		"worker_count", workerCount,
		"submit_worker_count", submitWorkerCount,
		"testee_page_size", testeePageSize,
		"testee_offset", testeeOffset,
		"testee_limit", testeeLimit,
		"org_id", orgID,
		"min_per_testee", minPerTestee,
		"max_per_testee", maxPerTestee,
	)

	if testeeLimit > 0 {
		targetTestees = int64(testeeLimit)
	}

	prewarmAPIToken(ctx, deps.APIClient, orgID, logger)
	seedStart := time.Now()

	startSubmitWorkers(ctx, &submitWG, submitCh, submitWorkerCount, submitWorkerConfig{
		logger:      logger,
		diagnostics: diag,
		client:      deps.APIClient,
		counters:    counters,
		failures:    failures,
	})

	startAssessmentWorkers(ctx, &wg, taskCh, workerCount, assessmentWorkerConfig{
		logger:             logger,
		diagnostics:        diag,
		client:             deps.APIClient,
		minPerTestee:       minPerTestee,
		maxPerTestee:       maxPerTestee,
		targets:            targets,
		questionnaireCache: questionnaireCache,
		cacheMu:            &cacheMu,
		submitCh:           submitCh,
		counters:           counters,
		totalTestees:       targetTestees,
		failures:           failures,
	})

	if targetTestees > 0 {
		printAssessmentProgress(0, targetTestees, 0)
	}

	producerErrCh := startTesteeProducer(
		ctx,
		deps.APIClient,
		taskCh,
		diag,
		orgID,
		testeePageSize,
		testeeOffset,
		testeeLimit,
		counters,
	)

	wg.Wait()
	close(submitCh)
	submitWG.Wait()
	if err := <-producerErrCh; err != nil {
		return err
	}
	snapshot := counters.Snapshot()
	if snapshot.successTesteeCount+snapshot.failedTesteeCount > 0 {
		fmt.Println()
	}

	if counters.ErrorCount() > 0 {
		failures.Log(logger)
		return fmt.Errorf("assessment seeding completed with %d errors", counters.ErrorCount())
	}

	snapshot = counters.Snapshot()
	logger.Infow("Assessment seeding completed",
		"processed_testees", snapshot.processedTesteeCount,
		"success_testees", snapshot.successTesteeCount,
		"failed_testees", snapshot.failedTesteeCount,
		"submitted_answersheets", snapshot.submittedCount,
		"skipped_items", snapshot.skippedCount,
		"enqueued_testees", snapshot.enqueuedTesteeCount,
		"submit_peak_in_flight", snapshot.submitMaxInFlight,
		"duration_ms", time.Since(seedStart).Milliseconds(),
	)
	return nil
}

type assessmentWorkerConfig struct {
	logger interface {
		Debugw(string, ...interface{})
		Warnw(string, ...interface{})
		Infow(string, ...interface{})
	}
	diagnostics        *assessmentDiagnostics
	client             *APIClient
	minPerTestee       int
	maxPerTestee       int
	targets            []scaleTarget
	questionnaireCache map[string]*QuestionnaireDetailResponse
	cacheMu            *sync.RWMutex
	submitCh           chan<- submissionTask
	counters           *assessmentCounters
	totalTestees       int64
	failures           *failureSamples
}

type submissionTask struct {
	testeeID          string
	questionnaireCode string
	req               SubmitAnswerSheetRequest
	resultCh          chan error
}

type submitWorkerConfig struct {
	logger interface {
		Debugw(string, ...interface{})
		Warnw(string, ...interface{})
		Infow(string, ...interface{})
	}
	diagnostics *assessmentDiagnostics
	client      *APIClient
	counters    *assessmentCounters
	failures    *failureSamples
}

func startAssessmentWorkers(
	ctx context.Context,
	wg *sync.WaitGroup,
	taskCh <-chan *TesteeResponse,
	workerCount int,
	cfg assessmentWorkerConfig,
) {
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		workerID := i
		go func() {
			defer wg.Done()
			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(workerID)))
			for testee := range taskCh {
				if ctx.Err() != nil {
					return
				}
				processTestee(ctx, cfg, testee, rng)
			}
		}()
	}
}

func startSubmitWorkers(
	ctx context.Context,
	wg *sync.WaitGroup,
	taskCh <-chan submissionTask,
	workerCount int,
	cfg submitWorkerConfig,
) {
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range taskCh {
				if ctx.Err() != nil {
					task.resultCh <- ctx.Err()
					close(task.resultCh)
					continue
				}
				submitStart := time.Now()
				inFlight := cfg.counters.StartSubmit()
				attempts, err := submitAnswerSheetWithRetry(ctx, cfg.client, task.req, submitMaxRetry)
				cfg.counters.EndSubmit()
				duration := time.Since(submitStart)
				if err != nil {
					cfg.logger.Warnw("Submit answersheet failed",
						"testee_id", task.testeeID,
						"questionnaire", task.questionnaireCode,
						"attempts", attempts,
						"duration_ms", duration.Milliseconds(),
						"in_flight", inFlight,
						"max_in_flight", cfg.counters.Snapshot().submitMaxInFlight,
						"error", err,
					)
					if cfg.counters.NextErrorLogIndex() <= 3 {
						cfg.logger.Warnw("Submit payload preview",
							"testee_id", task.testeeID,
							"questionnaire", task.questionnaireCode,
							"questionnaire_version", task.req.QuestionnaireVersion,
							"answer_count", len(task.req.Answers),
							"answers", previewAnswers(task.req.Answers),
						)
					}
					cfg.counters.AddErrors(1)
					cfg.failures.Add(task.testeeID, task.questionnaireCode, err)
				} else {
					cfg.diagnostics.LogSubmitCompleted(submitLogPayload{
						testeeID:      task.testeeID,
						questionnaire: task.questionnaireCode,
						attempts:      attempts,
						duration:      duration,
						inFlight:      inFlight,
						maxInFlight:   cfg.counters.Snapshot().submitMaxInFlight,
						answerCount:   len(task.req.Answers),
					})
					cfg.counters.AddSubmitted(1)
				}
				task.resultCh <- err
				close(task.resultCh)
			}
		}()
	}
}

func processTestee(ctx context.Context, cfg assessmentWorkerConfig, testee *TesteeResponse, rng *rand.Rand) {
	perTestee := rng.Intn(cfg.maxPerTestee-cfg.minPerTestee+1) + cfg.minPerTestee
	testeeStart := time.Now()
	cfg.diagnostics.LogTesteeStart(testee.ID, perTestee)

	picks := pickScaleTargets(cfg.targets, perTestee, rng)
	var testeeFailed int32
	resultChans := make([]chan error, 0, len(picks))
	for _, target := range picks {
		if ctx.Err() != nil {
			atomic.StoreInt32(&testeeFailed, 1)
			break
		}

		task, outcome := buildSubmissionTask(ctx, cfg, testee, target, rng)
		if outcome == taskFailed {
			atomic.StoreInt32(&testeeFailed, 1)
			continue
		}
		if outcome != taskReady || task == nil {
			continue
		}

		resultCh := make(chan error, 1)
		select {
		case <-ctx.Done():
			atomic.StoreInt32(&testeeFailed, 1)
			return
		case cfg.submitCh <- submissionTask{
			testeeID:          task.testeeID,
			questionnaireCode: task.questionnaireCode,
			req:               task.req,
			resultCh:          resultCh,
		}:
			resultChans = append(resultChans, resultCh)
		}
	}

	for _, ch := range resultChans {
		select {
		case <-ctx.Done():
			atomic.StoreInt32(&testeeFailed, 1)
			return
		case err := <-ch:
			if err != nil {
				atomic.StoreInt32(&testeeFailed, 1)
			}
		}
	}
	cfg.counters.MarkTesteeProcessed(atomic.LoadInt32(&testeeFailed) == 1)
	snapshot := cfg.counters.Snapshot()
	printAssessmentProgress(
		snapshot.successTesteeCount+snapshot.failedTesteeCount,
		cfg.totalTestees,
		snapshot.failedTesteeCount,
	)
	cfg.diagnostics.LogTesteeProcessed(testeeLogPayload{
		testeeID:           testee.ID,
		questionnaireCount: len(picks),
		duration:           time.Since(testeeStart),
		success:            atomic.LoadInt32(&testeeFailed) == 0,
	})
}

// buildSubmissionTask prepares one answersheet submission task for a testee.
func buildSubmissionTask(ctx context.Context, cfg assessmentWorkerConfig, testee *TesteeResponse, target scaleTarget, rng *rand.Rand) (*submissionTask, taskOutcome) {
	if ctx.Err() != nil {
		return nil, taskFailed
	}

	detail := getQuestionnaireDetail(ctx, cfg.client, target.QuestionnaireCode, cfg.questionnaireCache, cfg.cacheMu, cfg.logger)
	if detail == nil {
		cfg.counters.AddErrors(1)
		cfg.counters.AddSkipped(1)
		cfg.failures.Add(testee.ID, target.QuestionnaireCode, fmt.Errorf("questionnaire detail missing"))
		return nil, taskFailed
	}
	debugLogQuestionnaire(detail, cfg.logger)
	if detail.Type != questionnaireTypeMedicalScale {
		cfg.logger.Warnw("Questionnaire is not medical scale, skipping", "code", target.QuestionnaireCode, "type", detail.Type)
		cfg.counters.AddSkipped(1)
		return nil, taskSkipped
	}
	if target.QuestionnaireVersion != "" && detail.Version != target.QuestionnaireVersion {
		cfg.logger.Warnw("Questionnaire version mismatch, skipping",
			"code", target.QuestionnaireCode,
			"expected", target.QuestionnaireVersion,
			"actual", detail.Version,
		)
		cfg.counters.AddSkipped(1)
		return nil, taskSkipped
	}

	answers := buildAnswers(detail, rng)
	if len(answers) == 0 {
		cfg.logger.Warnw("No supported answers generated, skipping questionnaire",
			"code", target.QuestionnaireCode,
			"testee_id", testee.ID,
			"question_types", collectQuestionTypes(detail),
		)
		cfg.counters.AddSkipped(1)
		return nil, taskSkipped
	}

	if invalidAnswers := validateAnswers(detail, answers); len(invalidAnswers) > 0 {
		cfg.logger.Warnw("Invalid answers detected",
			"questionnaire_code", target.QuestionnaireCode,
			"testee_id", testee.ID,
			"invalid_count", len(invalidAnswers),
			"invalid_answers", invalidAnswers,
		)
	}

	testeeID := parseID(testee.ID)
	if testeeID == 0 {
		cfg.logger.Warnw("Invalid testee ID, skipping", "testee_id", testee.ID)
		cfg.counters.AddErrors(1)
		cfg.counters.AddSkipped(1)
		cfg.failures.Add(testee.ID, target.QuestionnaireCode, fmt.Errorf("invalid testee id"))
		return nil, taskFailed
	}

	version := detail.Version
	if target.QuestionnaireVersion != "" {
		version = target.QuestionnaireVersion
	}

	req := SubmitAnswerSheetRequest{
		QuestionnaireCode:    target.QuestionnaireCode,
		QuestionnaireVersion: version,
		Title:                detail.Title,
		TesteeID:             testeeID,
		Answers:              answers,
	}

	return &submissionTask{
		testeeID:          testee.ID,
		questionnaireCode: target.QuestionnaireCode,
		req:               req,
	}, taskReady
}

func submitAnswerSheet(ctx context.Context, client *APIClient, req SubmitAnswerSheetRequest) error {
	adminReq := AdminSubmitAnswerSheetRequest{
		QuestionnaireCode:    req.QuestionnaireCode,
		QuestionnaireVersion: req.QuestionnaireVersion,
		Title:                req.Title,
		TesteeID:             req.TesteeID,
		Answers:              req.Answers,
	}

	_, err := client.SubmitAnswerSheetAdmin(ctx, adminReq)
	return err
}

func submitAnswerSheetWithRetry(ctx context.Context, client *APIClient, req SubmitAnswerSheetRequest, maxRetry int) (int, error) {
	if maxRetry <= 0 {
		return 1, submitAnswerSheet(ctx, client, req)
	}

	var lastErr error
	attempts := 0
	for attempt := 0; attempt < maxRetry; attempt++ {
		if ctx.Err() != nil {
			return attempts, ctx.Err()
		}
		attempts++
		if err := submitAnswerSheet(ctx, client, req); err == nil {
			return attempts, nil
		} else {
			lastErr = err
		}
		if attempt < maxRetry-1 {
			time.Sleep(submitRetryBackoff * time.Duration(attempt+1))
		}
	}
	return attempts, lastErr
}

// logQuestionnaireDetail 打印问卷详细信息
func logQuestionnaireDetail(logger interface{ Infow(string, ...interface{}) }, detail *QuestionnaireDetailResponse, target scaleTarget) {
	questions := make([]map[string]interface{}, 0, len(detail.Questions))
	for _, q := range detail.Questions {
		options := make([]map[string]string, 0, len(q.Options))
		for _, opt := range q.Options {
			options = append(options, map[string]string{
				"code":    opt.Code,
				"content": opt.Content,
				"score":   fmt.Sprintf("%d", opt.Score),
			})
		}
		questions = append(questions, map[string]interface{}{
			"code":    q.Code,
			"type":    q.Type,
			"title":   truncateString(q.Title, 50),
			"options": options,
		})
	}

	logger.Infow("Questionnaire detail",
		"questionnaire_code", detail.Code,
		"questionnaire_version", detail.Version,
		"title", detail.Title,
		"type", detail.Type,
		"question_count", len(detail.Questions),
		"questions", questions,
	)
}

// logBuiltAnswers 打印组织好的答卷信息
func logBuiltAnswers(logger interface{ Infow(string, ...interface{}) }, answers []Answer, questionnaireCode, testeeID string) {
	answerDetails := make([]map[string]interface{}, 0, len(answers))
	for _, a := range answers {
		valueStr := formatAnswerValue(a.Value)
		answerDetails = append(answerDetails, map[string]interface{}{
			"question_code": a.QuestionCode,
			"question_type": a.QuestionType,
			"value":         valueStr,
			"value_type":    fmt.Sprintf("%T", a.Value),
			"score":         a.Score,
		})
	}

	logger.Infow("Built answers",
		"questionnaire_code", questionnaireCode,
		"testee_id", testeeID,
		"answer_count", len(answers),
		"answers", answerDetails,
	)
}

// logSubmitRequest 打印提交的答卷信息
func logSubmitRequest(logger interface{ Infow(string, ...interface{}) }, req SubmitAnswerSheetRequest, testeeIDStr string) {
	answerDetails := make([]map[string]interface{}, 0, len(req.Answers))
	for _, a := range req.Answers {
		valueStr := formatAnswerValue(a.Value)
		answerDetails = append(answerDetails, map[string]interface{}{
			"question_code": a.QuestionCode,
			"question_type": a.QuestionType,
			"value":         valueStr,
			"value_type":    fmt.Sprintf("%T", a.Value),
			"score":         a.Score,
		})
	}

	logger.Infow("Submit answer sheet request",
		"testee_id", testeeIDStr,
		"testee_id_uint64", req.TesteeID,
		"questionnaire_code", req.QuestionnaireCode,
		"questionnaire_version", req.QuestionnaireVersion,
		"title", req.Title,
		"answer_count", len(req.Answers),
		"answers", answerDetails,
	)
}

// validateAnswers 验证答案是否在问卷的选项中存在
func validateAnswers(detail *QuestionnaireDetailResponse, answers []Answer) []map[string]interface{} {
	// 构建问题编码到选项的映射
	questionMap := make(map[string]map[string]bool) // question_code -> {option_code: true, ...}
	for _, q := range detail.Questions {
		optionSet := make(map[string]bool)
		for _, opt := range q.Options {
			if opt.Code != "" {
				optionSet[opt.Code] = true
			}
			if opt.Content != "" {
				optionSet[opt.Content] = true
			}
		}
		questionMap[q.Code] = optionSet
	}

	invalidAnswers := make([]map[string]interface{}, 0)
	for _, answer := range answers {
		optionSet, exists := questionMap[answer.QuestionCode]
		if !exists {
			invalidAnswers = append(invalidAnswers, map[string]interface{}{
				"question_code": answer.QuestionCode,
				"reason":        "question not found in questionnaire",
			})
			continue
		}

		// 检查答案值是否在选项中
		var valueStr string
		switch v := answer.Value.(type) {
		case string:
			valueStr = v
		case []string:
			// 对于多选题，检查所有选项
			for _, val := range v {
				if !optionSet[val] {
					invalidAnswers = append(invalidAnswers, map[string]interface{}{
						"question_code": answer.QuestionCode,
						"value":         val,
						"reason":        "option not found in question",
					})
				}
			}
			continue
		default:
			valueStr = formatAnswerValue(v)
		}

		if !optionSet[valueStr] {
			invalidAnswers = append(invalidAnswers, map[string]interface{}{
				"question_code":     answer.QuestionCode,
				"value":             valueStr,
				"reason":            "option not found in question",
				"available_options": getQuestionOptions(detail, answer.QuestionCode),
			})
		}
	}

	return invalidAnswers
}

// getQuestionOptions 获取问题的所有选项
func getQuestionOptions(detail *QuestionnaireDetailResponse, questionCode string) []string {
	for _, q := range detail.Questions {
		if q.Code == questionCode {
			options := make([]string, 0, len(q.Options))
			for _, opt := range q.Options {
				if opt.Code != "" {
					options = append(options, opt.Code)
				} else if opt.Content != "" {
					options = append(options, opt.Content)
				}
			}
			return options
		}
	}
	return nil
}

// formatAnswerValue 格式化答案值用于日志输出
func formatAnswerValue(value interface{}) string {
	if value == nil {
		return "<nil>"
	}
	switch v := value.(type) {
	case string:
		return v
	case []string:
		return fmt.Sprintf("%v", v)
	case float64:
		return fmt.Sprintf("%.0f", v)
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	default:
		return fmt.Sprintf("%v (type: %T)", v, v)
	}
}

func enqueueTestees(ctx context.Context, taskCh chan<- *TesteeResponse, testees []*TesteeResponse) error {
	for _, testee := range testees {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case taskCh <- testee:
		}
	}
	return nil
}

func normalizeWorkerCount(workerCount int) int {
	if workerCount <= 0 {
		return 8
	}
	return workerCount
}

func normalizeSubmitWorkerCount(workerCount int) int {
	if workerCount <= 0 {
		return 10
	}
	return workerCount
}

func normalizeTesteePageSize(pageSize int) int {
	if pageSize <= 0 {
		return 100
	}
	if pageSize > 100 {
		return 100
	}
	return pageSize
}

func normalizeTesteeOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}

func normalizeTesteeLimit(limit int) int {
	if limit < 0 {
		return 0
	}
	return limit
}

func loadScaleTargets(ctx context.Context, client *APIClient, categories []string, diag *assessmentDiagnostics) ([]scaleTarget, error) {
	start := time.Now()
	targets, err := listAllScaleTargets(ctx, client, categories)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		diag.LogNoScaleTargets(categories)
		return nil, nil
	}
	diag.LogScaleTargetsLoaded(len(targets), time.Since(start))
	return targets, nil
}

func startTesteeProducer(
	ctx context.Context,
	client *APIClient,
	taskCh chan<- *TesteeResponse,
	diag *assessmentDiagnostics,
	orgID int64,
	pageSize, offset, limit int,
	counters *assessmentCounters,
) <-chan error {
	errCh := make(chan error, 1)
	go func() {
		defer close(taskCh)
		start := time.Now()
		err := iterateTesteesFromApiserver(ctx, client, orgID, pageSize, offset, limit, func(testees []*TesteeResponse) error {
			counters.AddEnqueued(int64(len(testees)))
			return enqueueTestees(ctx, taskCh, testees)
		})
		diag.LogProducerFinished(counters.Snapshot().enqueuedTesteeCount, time.Since(start))
		errCh <- err
	}()
	return errCh
}

func iterateTesteesFromApiserver(
	ctx context.Context,
	client *APIClient,
	orgID int64,
	pageSize, offset, limit int,
	fn func([]*TesteeResponse) error,
) error {
	if pageSize <= 0 {
		pageSize = 200
	}
	if offset < 0 {
		offset = 0
	}
	if limit < 0 {
		limit = 0
	}

	page := offset/pageSize + 1
	skip := offset % pageSize
	processed := 0

	for {
		resp, err := client.ListTesteesByOrg(ctx, orgID, page, pageSize)
		if err != nil {
			return err
		}
		if resp == nil || len(resp.Items) == 0 {
			return nil
		}

		items := resp.Items
		if skip > 0 {
			if skip >= len(items) {
				skip -= len(items)
				page++
				continue
			}
			items = items[skip:]
			skip = 0
		}

		if limit > 0 {
			remaining := limit - processed
			if remaining <= 0 {
				return nil
			}
			if len(items) > remaining {
				items = items[:remaining]
			}
		}

		mapped := make([]*TesteeResponse, len(items))
		for i, item := range items {
			mapped[i] = &TesteeResponse{ID: item.ID}
		}

		if err := fn(mapped); err != nil {
			return err
		}

		processed += len(items)
		if limit > 0 && processed >= limit {
			return nil
		}
		if resp.TotalPages > 0 && resp.Page >= resp.TotalPages {
			return nil
		}
		page++
	}
}

type scaleTarget struct {
	ScaleCode            string
	Category             string
	QuestionnaireCode    string
	QuestionnaireVersion string
}

func listAllScaleTargets(ctx context.Context, client *APIClient, categories []string) ([]scaleTarget, error) {
	const pageSize = 100
	targets := make([]scaleTarget, 0, 64)
	seen := make(map[string]struct{})

	if len(categories) == 0 {
		scales, err := listScalesByCategory(ctx, client, "", pageSize)
		if err != nil {
			return nil, err
		}
		return appendUniqueScaleTargets(targets, scales, seen), nil
	}

	for _, category := range categories {
		scales, err := listScalesByCategory(ctx, client, category, pageSize)
		if err != nil {
			return nil, err
		}
		targets = appendUniqueScaleTargets(targets, scales, seen)
	}

	return targets, nil
}

func listScalesByCategory(ctx context.Context, client *APIClient, category string, pageSize int) ([]CollectionScaleSummary, error) {
	page := 1
	scales := make([]CollectionScaleSummary, 0, 64)

	for {
		resp, err := client.ListScales(ctx, page, pageSize, "published", category)
		if err != nil {
			return nil, err
		}
		if resp == nil || len(resp.Scales) == 0 {
			break
		}
		scales = append(scales, resp.Scales...)
		page++
		if int64(len(scales)) >= resp.Total {
			break
		}
	}

	return scales, nil
}

func appendUniqueScaleTargets(targets []scaleTarget, scales []CollectionScaleSummary, seen map[string]struct{}) []scaleTarget {
	for _, scale := range scales {
		if scale.QuestionnaireCode == "" {
			continue
		}
		key := scale.QuestionnaireCode + ":" + scale.QuestionnaireVersion
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		targets = append(targets, scaleTarget{
			ScaleCode:            scale.Code,
			Category:             scale.Category,
			QuestionnaireCode:    scale.QuestionnaireCode,
			QuestionnaireVersion: scale.QuestionnaireVersion,
		})
	}
	return targets
}

func getQuestionnaireDetail(
	ctx context.Context,
	client *APIClient,
	code string,
	cache map[string]*QuestionnaireDetailResponse,
	cacheMu *sync.RWMutex,
	logger interface{ Warnw(string, ...interface{}) },
) *QuestionnaireDetailResponse {
	cacheMu.RLock()
	detail, ok := cache[code]
	cacheMu.RUnlock()
	if ok {
		return detail
	}

	detail, err := client.GetQuestionnaireDetail(ctx, code)
	if err != nil {
		logger.Warnw("Failed to load questionnaire detail", "code", code, "error", err)
		return nil
	}
	if detail == nil {
		logger.Warnw("Questionnaire detail not found", "code", code)
		return nil
	}

	cacheMu.Lock()
	cache[code] = detail
	cacheMu.Unlock()
	return detail
}

func pickScaleTargets(source []scaleTarget, count int, rng *rand.Rand) []scaleTarget {
	if count <= 0 {
		return nil
	}
	if count >= len(source) {
		shuffled := append([]scaleTarget(nil), source...)
		rng.Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})
		return shuffled
	}

	shuffled := append([]scaleTarget(nil), source...)
	rng.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})
	return shuffled[:count]
}

func buildAnswers(q *QuestionnaireDetailResponse, rng *rand.Rand) []Answer {
	answers := make([]Answer, 0, len(q.Questions))
	for _, question := range q.Questions {
		answer, ok := buildAnswerForQuestion(question, rng)
		if !ok {
			continue
		}
		answers = append(answers, answer)
	}
	return answers
}

func buildAnswerForQuestion(question QuestionResponse, rng *rand.Rand) (Answer, bool) {
	resolvedType := resolveQuestionType(question)
	normalizedType := normalizeQuestionType(resolvedType)

	switch normalizedType {
	case strings.ToLower(questionTypeRadio):
		// 单选题：随机选择一个选项
		if len(question.Options) == 0 {
			return Answer{}, false
		}
		opt := question.Options[rng.Intn(len(question.Options))]
		value := opt.Code
		if value == "" {
			value = opt.Content
		}
		if value == "" {
			return Answer{}, false
		}
		return Answer{
			QuestionCode: question.Code,
			QuestionType: questionTypeRadio,
			Score:        0,
			Value:        value,
		}, true

	case strings.ToLower(questionTypeCheckbox):
		// 多选题：随机选择 1-3 个选项
		if len(question.Options) == 0 {
			return Answer{}, false
		}
		count := rng.Intn(3) + 1 // 1-3 个选项
		if count > len(question.Options) {
			count = len(question.Options)
		}

		// 随机选择选项
		selectedIndices := make(map[int]bool)
		selectedValues := make([]string, 0, count)
		for len(selectedValues) < count {
			idx := rng.Intn(len(question.Options))
			if !selectedIndices[idx] {
				selectedIndices[idx] = true
				opt := question.Options[idx]
				value := opt.Code
				if value == "" {
					value = opt.Content
				}
				if value != "" {
					selectedValues = append(selectedValues, value)
				}
			}
		}

		if len(selectedValues) == 0 {
			return Answer{}, false
		}

		// 直接使用数组，JSON 序列化时会自动处理
		return Answer{
			QuestionCode: question.Code,
			QuestionType: questionTypeCheckbox,
			Score:        0,
			Value:        selectedValues, // []string 类型
		}, true

	case strings.ToLower(questionTypeText), strings.ToLower(questionTypeTextarea):
		// 文本题：生成随机文本
		texts := []string{
			"正常",
			"良好",
			"一般",
			"需要关注",
			"测试答案",
		}
		value := texts[rng.Intn(len(texts))]
		return Answer{
			QuestionCode: question.Code,
			QuestionType: resolvedType, // 保持原始类型
			Score:        0,
			Value:        value,
		}, true

	case strings.ToLower(questionTypeNumber):
		// 数字题：生成 1-100 的随机数字
		// 使用 float64 类型，因为 JSON 中的数字会被解析为 float64
		value := float64(rng.Intn(100) + 1)
		return Answer{
			QuestionCode: question.Code,
			QuestionType: questionTypeNumber,
			Score:        0,
			Value:        value, // float64 类型
		}, true

	case strings.ToLower(questionTypeSection):
		// 段落题：不需要答案，跳过
		return Answer{}, false

	default:
		// 不支持的类型，跳过
		return Answer{}, false
	}
}

func parseID(raw string) uint64 {
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0
	}
	return id
}

func parseCategories(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	items := make([]string, 0, 4)
	for _, item := range strings.Split(raw, ",") {
		val := strings.TrimSpace(item)
		if val == "" {
			continue
		}
		items = append(items, val)
	}
	return items
}

func normalizeQuestionType(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func collectQuestionTypes(q *QuestionnaireDetailResponse) []string {
	if q == nil || len(q.Questions) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(q.Questions))
	out := make([]string, 0, len(q.Questions))
	for _, question := range q.Questions {
		typ := strings.TrimSpace(question.Type)
		if typ == "" {
			typ = fmt.Sprintf("<empty:%s>", resolveQuestionType(question))
		}
		if _, exists := seen[typ]; exists {
			continue
		}
		seen[typ] = struct{}{}
		out = append(out, typ)
	}
	return out
}

func resolveQuestionType(question QuestionResponse) string {
	raw := normalizeQuestionType(question.Type)
	switch raw {
	case strings.ToLower(questionTypeRadio):
		return questionTypeRadio
	case strings.ToLower(questionTypeCheckbox):
		return questionTypeCheckbox
	case strings.ToLower(questionTypeText):
		return questionTypeText
	case strings.ToLower(questionTypeTextarea):
		return questionTypeTextarea
	case strings.ToLower(questionTypeNumber):
		return questionTypeNumber
	case strings.ToLower(questionTypeSection):
		return questionTypeSection
	}
	// 如果没有明确类型，根据是否有选项推断
	if len(question.Options) > 0 {
		// 有选项，默认为单选题
		return questionTypeRadio
	}
	// 无选项，默认为段落题
	return questionTypeSection
}
func previewAnswers(answers []Answer) []map[string]string {
	const maxPreview = 3
	if len(answers) == 0 {
		return nil
	}
	n := len(answers)
	if n > maxPreview {
		n = maxPreview
	}
	out := make([]map[string]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, map[string]string{
			"question_code": answers[i].QuestionCode,
			"value":         formatAnswerValue(answers[i].Value),
		})
	}
	return out
}

func debugLogQuestionnaire(q *QuestionnaireDetailResponse, logger interface{ Debugw(string, ...interface{}) }) {
	if q == nil || len(q.Questions) == 0 {
		return
	}
	preview := make([]map[string]string, 0, 3)
	for i, question := range q.Questions {
		if i >= 3 {
			break
		}
		preview = append(preview, map[string]string{
			"code":          question.Code,
			"type":          question.Type,
			"resolved_type": resolveQuestionType(question),
			"option_count":  strconv.Itoa(len(question.Options)),
			"title_preview": truncateString(question.Title, 30),
		})
	}
	logger.Debugw("Questionnaire detail preview",
		"code", q.Code,
		"title", q.Title,
		"type", q.Type,
		"question_count", len(q.Questions),
		"questions", preview,
	)
}

func truncateString(value string, max int) string {
	if max <= 0 || value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max]) + "..."
}

func printAssessmentProgress(current, total, failed int64) {
	const barWidth = 40
	if total > 0 {
		percent := float64(current) / float64(total)
		if percent > 1 {
			percent = 1
		}
		filled := int(percent * barWidth)
		bar := strings.Repeat("#", filled) + strings.Repeat("-", barWidth-filled)
		if failed > 0 {
			fmt.Printf("\rAssessment: [%s] %d/%d (%.1f%%) failed:%d", bar, current, total, percent*100, failed)
		} else {
			fmt.Printf("\rAssessment: [%s] %d/%d (%.1f%%)", bar, current, total, percent*100)
		}
		return
	}

	if failed > 0 {
		fmt.Printf("\rAssessment: processed %d failed:%d", current, failed)
	} else {
		fmt.Printf("\rAssessment: processed %d", current)
	}
}

func updateMaxInFlight(max *int64, current int64) {
	if max == nil {
		return
	}
	for {
		prev := atomic.LoadInt64(max)
		if current <= prev {
			return
		}
		if atomic.CompareAndSwapInt64(max, prev, current) {
			return
		}
	}
}

func prewarmAPIToken(ctx context.Context, client *APIClient, orgID int64, logger interface{ Warnw(string, ...interface{}) }) {
	if client == nil || orgID <= 0 {
		return
	}
	_, err := client.ListTesteesByOrg(ctx, orgID, 1, 1)
	if err != nil {
		logger.Warnw("Prewarm API token failed", "error", err)
	}
}
