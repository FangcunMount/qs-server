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
	questionTypeRadio   = "Radio"
	questionTypeSection = "Section"

	questionnaireTypeMedicalScale = "MedicalScale"
)

func seedAssessments(ctx context.Context, deps *dependencies, seedCtx *seedContext, minPerTestee, maxPerTestee, workerCount, testeePageSize, testeeOffset, testeeLimit int, categoryFilter string) error {
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

	categories := parseCategories(categoryFilter)
	targets, err := listAllScaleTargets(ctx, client, categories)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		logger.Warnw("No medical scales found for assessment seeding", "categories", categories)
		return nil
	}

	workerCount = normalizeWorkerCount(workerCount)
	testeePageSize = normalizeTesteePageSize(testeePageSize)
	testeeOffset = normalizeTesteeOffset(testeeOffset)
	testeeLimit = normalizeTesteeLimit(testeeLimit)

	questionnaireCache := make(map[string]*QuestionnaireDetailResponse)
	var cacheMu sync.RWMutex
	var errorCount int64
	var submittedCount int64
	var skippedCount int64
	var processedTesteeCount int64

	taskCh := make(chan *TesteeResponse, workerCount*4)
	var wg sync.WaitGroup

	logger.Infow("Assessment seeding started",
		"categories", categories,
		"scale_count", len(targets),
		"worker_count", workerCount,
		"testee_page_size", testeePageSize,
		"testee_offset", testeeOffset,
		"testee_limit", testeeLimit,
		"org_id", orgID,
		"min_per_testee", minPerTestee,
		"max_per_testee", maxPerTestee,
	)

	startAssessmentWorkers(ctx, &wg, taskCh, workerCount, assessmentWorkerConfig{
		logger:             logger,
		client:             client,
		minPerTestee:       minPerTestee,
		maxPerTestee:       maxPerTestee,
		targets:            targets,
		questionnaireCache: questionnaireCache,
		cacheMu:            &cacheMu,
		errorCount:         &errorCount,
		submittedCount:     &submittedCount,
		skippedCount:       &skippedCount,
	})

	err = iterateTesteesFromApiserver(ctx, deps.APIClient, orgID, testeePageSize, testeeOffset, testeeLimit, func(testees []*TesteeResponse) error {
		atomic.AddInt64(&processedTesteeCount, int64(len(testees)))
		return enqueueTestees(ctx, taskCh, testees)
	})
	close(taskCh)
	wg.Wait()
	if err != nil {
		return err
	}

	if errorCount > 0 {
		return fmt.Errorf("assessment seeding completed with %d errors", errorCount)
	}

	logger.Infow("Assessment seeding completed",
		"processed_testees", processedTesteeCount,
		"submitted_answersheets", submittedCount,
		"skipped_items", skippedCount,
	)
	return nil
}

type assessmentWorkerConfig struct {
	logger interface {
		Debugw(string, ...interface{})
		Warnw(string, ...interface{})
	}
	client             *APIClient
	minPerTestee       int
	maxPerTestee       int
	targets            []scaleTarget
	questionnaireCache map[string]*QuestionnaireDetailResponse
	cacheMu            *sync.RWMutex
	errorCount         *int64
	submittedCount     *int64
	skippedCount       *int64
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

func processTestee(ctx context.Context, cfg assessmentWorkerConfig, testee *TesteeResponse, rng *rand.Rand) {
	perTestee := rng.Intn(cfg.maxPerTestee-cfg.minPerTestee+1) + cfg.minPerTestee
	cfg.logger.Debugw("Seeding assessments for testee", "testee_id", testee.ID, "count", perTestee)

	picks := pickScaleTargets(cfg.targets, perTestee, rng)
	for _, target := range picks {
		detail := getQuestionnaireDetail(ctx, cfg.client, target.QuestionnaireCode, cfg.questionnaireCache, cfg.cacheMu, cfg.logger)
		if detail == nil {
			atomic.AddInt64(cfg.errorCount, 1)
			atomic.AddInt64(cfg.skippedCount, 1)
			continue
		}
		if detail.Type != questionnaireTypeMedicalScale {
			cfg.logger.Warnw("Questionnaire is not medical scale, skipping", "code", target.QuestionnaireCode, "type", detail.Type)
			atomic.AddInt64(cfg.skippedCount, 1)
			continue
		}
		if target.QuestionnaireVersion != "" && detail.Version != target.QuestionnaireVersion {
			cfg.logger.Warnw("Questionnaire version mismatch, skipping",
				"code", target.QuestionnaireCode,
				"expected", target.QuestionnaireVersion,
				"actual", detail.Version,
			)
			atomic.AddInt64(cfg.skippedCount, 1)
			continue
		}

		answers := buildAnswers(detail, rng)
		if len(answers) == 0 {
			cfg.logger.Warnw("No supported answers generated, skipping questionnaire", "code", target.QuestionnaireCode, "testee_id", testee.ID)
			atomic.AddInt64(cfg.skippedCount, 1)
			continue
		}

		testeeID := parseID(testee.ID)
		if testeeID == 0 {
			cfg.logger.Warnw("Invalid testee ID, skipping", "testee_id", testee.ID)
			atomic.AddInt64(cfg.errorCount, 1)
			atomic.AddInt64(cfg.skippedCount, 1)
			continue
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

		if err := submitAnswerSheet(ctx, cfg.client, req); err != nil {
			cfg.logger.Warnw("Failed to submit answer sheet", "testee_id", testee.ID, "questionnaire", target.QuestionnaireCode, "error", err)
			atomic.AddInt64(cfg.errorCount, 1)
			continue
		}
		atomic.AddInt64(cfg.submittedCount, 1)
	}
}

func submitAnswerSheet(ctx context.Context, client *APIClient, req SubmitAnswerSheetRequest) error {
	_, err := client.SubmitAnswerSheet(ctx, req)
	return err
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

func normalizeTesteePageSize(pageSize int) int {
	if pageSize <= 0 {
		return 200
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
	switch question.Type {
	case questionTypeRadio:
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
			QuestionType: question.Type,
			Score:        0,
			Value:        value,
		}, true
	case questionTypeSection:
		return Answer{
			QuestionCode: question.Code,
			QuestionType: question.Type,
			Score:        0,
			Value:        fmt.Sprintf("section-%s", question.Code),
		}, true
	default:
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
