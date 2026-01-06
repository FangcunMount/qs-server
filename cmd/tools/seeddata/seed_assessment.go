package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
)

const (
	questionTypeRadio    = "Radio"
	questionTypeCheckbox = "Checkbox"
	questionTypeText     = "Text"
	questionTypeTextarea = "Textarea"
	questionTypeNumber   = "Number"
	questionTypeSection  = "Section"

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
	var errorLogCount int64

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
		client:             deps.APIClient,
		minPerTestee:       minPerTestee,
		maxPerTestee:       maxPerTestee,
		targets:            targets,
		questionnaireCache: questionnaireCache,
		cacheMu:            &cacheMu,
		errorCount:         &errorCount,
		submittedCount:     &submittedCount,
		skippedCount:       &skippedCount,
		errorLogCount:      &errorLogCount,
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
		Infow(string, ...interface{})
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
	errorLogCount      *int64
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
		debugLogQuestionnaire(detail, cfg.logger)
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

		// 打印问卷详细信息
		// logQuestionnaireDetail(cfg.logger, detail, target)

		answers := buildAnswers(detail, rng)
		if len(answers) == 0 {
			cfg.logger.Warnw("No supported answers generated, skipping questionnaire",
				"code", target.QuestionnaireCode,
				"testee_id", testee.ID,
				"question_types", collectQuestionTypes(detail),
			)
			atomic.AddInt64(cfg.skippedCount, 1)
			continue
		}

		// 打印组织好的答卷信息
		logBuiltAnswers(cfg.logger, answers, target.QuestionnaireCode, testee.ID)

		// 验证答案是否有效
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

		// 打印提交的答卷信息
		logSubmitRequest(cfg.logger, req, testee.ID)

		if err := submitAnswerSheet(ctx, cfg.client, req); err != nil {
			cfg.logger.Warnw("Failed to submit answer sheet", "testee_id", testee.ID, "questionnaire", target.QuestionnaireCode, "error", err)
			if atomic.AddInt64(cfg.errorLogCount, 1) <= 3 {
				cfg.logger.Warnw("Submit payload preview",
					"testee_id", testee.ID,
					"questionnaire", target.QuestionnaireCode,
					"questionnaire_version", req.QuestionnaireVersion,
					"answer_count", len(req.Answers),
					"answers", previewAnswers(req.Answers),
				)
			}
			atomic.AddInt64(cfg.errorCount, 1)
			continue
		}
		// 提交成功后，系统会自动通过事件创建 Assessment
		atomic.AddInt64(cfg.submittedCount, 1)
	}
}

func submitAnswerSheet(ctx context.Context, client *APIClient, req SubmitAnswerSheetRequest) error {
	adminReq := AdminSubmitAnswerSheetRequest{
		QuestionnaireCode:    req.QuestionnaireCode,
		QuestionnaireVersion: req.QuestionnaireVersion,
		Title:                req.Title,
		TesteeID:             req.TesteeID,
		Answers:              req.Answers,
	}

	// 打印实际发送的 JSON 数据用于调试
	if jsonData, err := json.Marshal(adminReq); err == nil {
		logger := log.L(ctx)
		logger.Infow("Actual JSON payload being sent",
			"questionnaire_code", req.QuestionnaireCode,
			"json_payload", string(jsonData),
		)
	}

	_, err := client.SubmitAnswerSheetAdmin(ctx, adminReq)
	return err
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
