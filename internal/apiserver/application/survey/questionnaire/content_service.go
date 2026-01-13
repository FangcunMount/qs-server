package questionnaire

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/validation"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// contentService 问卷内容编辑服务实现
// 行为者：问卷内容编辑者
type contentService struct {
	repo        questionnaire.Repository
	questionMgr questionnaire.QuestionManager
	listCache   *QuestionnaireListCache
}

// NewContentService 创建问卷内容编辑服务
func NewContentService(
	repo questionnaire.Repository,
	questionMgr questionnaire.QuestionManager,
	listCache *QuestionnaireListCache,
) QuestionnaireContentService {
	return &contentService{
		repo:        repo,
		questionMgr: questionMgr,
		listCache:   listCache,
	}
}

// AddQuestion 添加问题
func (s *contentService) AddQuestion(ctx context.Context, dto AddQuestionDTO) (*QuestionnaireResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("添加问题",
		"action", "add_question",
		"questionnaire_code", dto.QuestionnaireCode,
		"question_code", dto.Code,
		"question_type", dto.Type,
		"options_count", len(dto.Options),
	)

	// 1. 验证输入参数
	if err := s.validateQuestionnaireCode(ctx, dto.QuestionnaireCode, "add_question"); err != nil {
		return nil, err
	}
	if err := s.validateQuestionCode(ctx, dto.Code, dto.QuestionnaireCode, "add_question"); err != nil {
		return nil, err
	}
	if dto.Stem == "" {
		l.Warnw("问题题干为空",
			"action", "add_question",
			"questionnaire_code", dto.QuestionnaireCode,
			"question_code", dto.Code,
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问题题干不能为空")
	}
	if dto.Type == "" {
		l.Warnw("问题类型为空",
			"action", "add_question",
			"questionnaire_code", dto.QuestionnaireCode,
			"question_code", dto.Code,
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问题类型不能为空")
	}

	// 2. 获取问卷
	q, err := s.findQuestionnaireByCode(ctx, dto.QuestionnaireCode, "add_question")
	if err != nil {
		return nil, err
	}

	// 3. 判断问卷状态（已归档的问卷不能编辑）
	if err := s.checkArchivedStatus(ctx, q, dto.QuestionnaireCode, "add_question"); err != nil {
		return nil, err
	}

	// 4. 构建问题领域对象
	question, err := buildQuestionFromDTO(dto.Code, dto.Stem, dto.Type, dto.Options, dto.Required, dto.Description, nil, nil, nil)
	if err != nil {
		l.Errorw("创建问题失败",
			"action", "add_question",
			"questionnaire_code", dto.QuestionnaireCode,
			"question_code", dto.Code,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireInvalidQuestion, "创建问题失败")
	}

	// 5. 添加问题到问卷
	if err := s.questionMgr.AddQuestion(q, question); err != nil {
		l.Errorw("添加问题失败",
			"action", "add_question",
			"questionnaire_code", dto.QuestionnaireCode,
			"question_code", dto.Code,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireInvalidQuestion, "添加问题失败")
	}

	// 6. 持久化
	if err := s.persistQuestionnaire(ctx, q, dto.QuestionnaireCode, "add_question"); err != nil {
		return nil, err
	}

	s.logSuccess(ctx, "add_question", dto.QuestionnaireCode, startTime,
		"question_code", dto.Code,
		"questions_count", len(q.GetQuestions()),
	)

	return toQuestionnaireResult(q), nil
}

// UpdateQuestion 更新问题
func (s *contentService) UpdateQuestion(ctx context.Context, dto UpdateQuestionDTO) (*QuestionnaireResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("更新问题",
		"action", "update_question",
		"questionnaire_code", dto.QuestionnaireCode,
		"question_code", dto.Code,
		"question_type", dto.Type,
		"options_count", len(dto.Options),
	)

	// 1. 验证输入参数
	if err := s.validateQuestionnaireCode(ctx, dto.QuestionnaireCode, "update_question"); err != nil {
		return nil, err
	}
	if err := s.validateQuestionCode(ctx, dto.Code, dto.QuestionnaireCode, "update_question"); err != nil {
		return nil, err
	}
	if dto.Stem == "" {
		l.Warnw("问题题干为空",
			"action", "update_question",
			"questionnaire_code", dto.QuestionnaireCode,
			"question_code", dto.Code,
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问题题干不能为空")
	}

	// 2. 获取问卷
	q, err := s.findQuestionnaireByCode(ctx, dto.QuestionnaireCode, "update_question")
	if err != nil {
		return nil, err
	}

	// 3. 判断问卷状态
	if err := s.checkArchivedStatus(ctx, q, dto.QuestionnaireCode, "update_question"); err != nil {
		return nil, err
	}

	// 4. 构建新的问题对象
	newQuestion, err := buildQuestionFromDTO(dto.Code, dto.Stem, dto.Type, dto.Options, dto.Required, dto.Description, nil, nil, nil)
	if err != nil {
		l.Errorw("创建问题失败",
			"action", "update_question",
			"questionnaire_code", dto.QuestionnaireCode,
			"question_code", dto.Code,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireInvalidQuestion, "创建问题失败")
	}

	// 5. 更新问题
	if err := s.questionMgr.UpdateQuestion(q, newQuestion); err != nil {
		l.Errorw("更新问题失败",
			"action", "update_question",
			"questionnaire_code", dto.QuestionnaireCode,
			"question_code", dto.Code,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireInvalidQuestion, "更新问题失败")
	}

	// 6. 持久化
	if err := s.persistQuestionnaire(ctx, q, dto.QuestionnaireCode, "update_question"); err != nil {
		return nil, err
	}

	s.logSuccess(ctx, "update_question", dto.QuestionnaireCode, startTime,
		"question_code", dto.Code,
	)

	return toQuestionnaireResult(q), nil
}

// RemoveQuestion 删除问题
func (s *contentService) RemoveQuestion(ctx context.Context, questionnaireCode, questionCode string) (*QuestionnaireResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("删除问题",
		"action", "remove_question",
		"questionnaire_code", questionnaireCode,
		"question_code", questionCode,
	)

	// 1. 验证输入参数
	if err := s.validateQuestionnaireCode(ctx, questionnaireCode, "remove_question"); err != nil {
		return nil, err
	}
	if err := s.validateQuestionCode(ctx, questionCode, questionnaireCode, "remove_question"); err != nil {
		return nil, err
	}

	// 2. 获取问卷
	q, err := s.findQuestionnaireByCode(ctx, questionnaireCode, "remove_question")
	if err != nil {
		return nil, err
	}

	// 3. 判断问卷状态
	if err := s.checkArchivedStatus(ctx, q, questionnaireCode, "remove_question"); err != nil {
		return nil, err
	}

	// 4. 删除问题
	if err := s.questionMgr.RemoveQuestion(q, meta.NewCode(questionCode)); err != nil {
		l.Errorw("删除问题失败",
			"action", "remove_question",
			"questionnaire_code", questionnaireCode,
			"question_code", questionCode,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireInvalidQuestion, "删除问题失败")
	}

	// 5. 持久化
	if err := s.persistQuestionnaire(ctx, q, questionnaireCode, "remove_question"); err != nil {
		return nil, err
	}

	s.logSuccess(ctx, "remove_question", questionnaireCode, startTime,
		"question_code", questionCode,
		"questions_count", len(q.GetQuestions()),
	)

	return toQuestionnaireResult(q), nil
}

// ReorderQuestions 重排问题顺序
func (s *contentService) ReorderQuestions(ctx context.Context, questionnaireCode string, orderedCodes []string) (*QuestionnaireResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("重排问题顺序",
		"action", "reorder_questions",
		"questionnaire_code", questionnaireCode,
		"ordered_codes_count", len(orderedCodes),
	)

	// 1. 验证输入参数
	if err := s.validateQuestionnaireCode(ctx, questionnaireCode, "reorder_questions"); err != nil {
		return nil, err
	}
	if len(orderedCodes) == 0 {
		l.Warnw("问题编码列表为空",
			"action", "reorder_questions",
			"questionnaire_code", questionnaireCode,
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问题编码列表不能为空")
	}

	// 2. 获取问卷
	q, err := s.findQuestionnaireByCode(ctx, questionnaireCode, "reorder_questions")
	if err != nil {
		return nil, err
	}

	// 3. 判断问卷状态
	if err := s.checkArchivedStatus(ctx, q, questionnaireCode, "reorder_questions"); err != nil {
		return nil, err
	}

	// 4. 转换 string 编码为 meta.Code
	metaCodes := make([]meta.Code, 0, len(orderedCodes))
	for _, code := range orderedCodes {
		metaCodes = append(metaCodes, meta.NewCode(code))
	}

	// 5. 重排问题顺序
	if err := s.questionMgr.ReorderQuestions(q, metaCodes); err != nil {
		l.Errorw("重排问题顺序失败",
			"action", "reorder_questions",
			"questionnaire_code", questionnaireCode,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireInvalidQuestion, "重排问题顺序失败")
	}

	// 6. 持久化
	if err := s.persistQuestionnaire(ctx, q, questionnaireCode, "reorder_questions"); err != nil {
		return nil, err
	}

	s.logSuccess(ctx, "reorder_questions", questionnaireCode, startTime,
		"questions_count", len(q.GetQuestions()),
	)

	return toQuestionnaireResult(q), nil
}

// BatchUpdateQuestions 批量更新问题
func (s *contentService) BatchUpdateQuestions(ctx context.Context, questionnaireCode string, questions []QuestionDTO) (*QuestionnaireResult, error) {
	l := logger.L(ctx)
	startTime := time.Now()

	l.Debugw("批量更新问题",
		"action", "batch_update_questions",
		"questionnaire_code", questionnaireCode,
		"questions_count", len(questions),
	)

	// 1. 验证输入参数
	if err := s.validateQuestionnaireCode(ctx, questionnaireCode, "batch_update_questions"); err != nil {
		return nil, err
	}
	if len(questions) == 0 {
		l.Warnw("问题列表为空",
			"action", "batch_update_questions",
			"questionnaire_code", questionnaireCode,
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问题列表不能为空")
	}

	// 2. 获取问卷
	q, err := s.findQuestionnaireByCode(ctx, questionnaireCode, "batch_update_questions")
	if err != nil {
		return nil, err
	}

	// 3. 判断问卷状态
	if err := s.checkArchivedStatus(ctx, q, questionnaireCode, "batch_update_questions"); err != nil {
		return nil, err
	}

	// 4. 转换 DTO 为领域对象
	domainQuestions := make([]questionnaire.Question, 0, len(questions))
	for i, qDTO := range questions {
		question, err := buildQuestionFromDTO(qDTO.Code, qDTO.Stem, qDTO.Type, qDTO.Options, qDTO.Required, qDTO.Description, qDTO.ValidationRules, qDTO.CalculationRule, qDTO.ShowController)
		if err != nil {
			l.Errorw("创建问题失败",
				"action", "batch_update_questions",
				"questionnaire_code", questionnaireCode,
				"question_index", i+1,
				"question_code", qDTO.Code,
				"result", "failed",
				"error", err.Error(),
			)
			return nil, errors.WrapC(err, errorCode.ErrQuestionnaireInvalidQuestion, "第 %d 个问题创建失败", i+1)
		}
		domainQuestions = append(domainQuestions, question)
	}

	// 5. 批量更新问题（使用 ReplaceQuestions）
	if err := s.questionMgr.ReplaceQuestions(q, domainQuestions); err != nil {
		l.Errorw("批量更新问题失败",
			"action", "batch_update_questions",
			"questionnaire_code", questionnaireCode,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireInvalidQuestion, "批量更新问题失败")
	}

	// 6. 持久化
	if err := s.persistQuestionnaire(ctx, q, questionnaireCode, "batch_update_questions"); err != nil {
		return nil, err
	}

	s.logSuccess(ctx, "batch_update_questions", questionnaireCode, startTime,
		"questions_count", len(domainQuestions),
	)

	return toQuestionnaireResult(q), nil
}

// validateQuestionnaireCode 验证问卷编码
func (s *contentService) validateQuestionnaireCode(ctx context.Context, code string, action string) error {
	if code == "" {
		logger.L(ctx).Warnw("问卷编码为空",
			"action", action,
			"result", "invalid_params",
		)
		return errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问卷编码不能为空")
	}
	return nil
}

// validateQuestionCode 验证问题编码
func (s *contentService) validateQuestionCode(ctx context.Context, code string, questionnaireCode string, action string) error {
	if code == "" {
		logger.L(ctx).Warnw("问题编码为空",
			"action", action,
			"questionnaire_code", questionnaireCode,
			"result", "invalid_params",
		)
		return errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问题编码不能为空")
	}
	return nil
}

// findQuestionnaireByCode 根据编码查找问卷
func (s *contentService) findQuestionnaireByCode(ctx context.Context, code string, action string) (*questionnaire.Questionnaire, error) {
	q, err := s.repo.FindByCode(ctx, code)
	if err != nil {
		logger.L(ctx).Errorw("获取问卷失败",
			"action", action,
			"questionnaire_code", code,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取问卷失败")
	}
	return q, nil
}

// checkArchivedStatus 检查问卷是否已归档
func (s *contentService) checkArchivedStatus(ctx context.Context, q *questionnaire.Questionnaire, code string, action string) error {
	if q.IsArchived() {
		logger.L(ctx).Warnw("问卷已归档，不能编辑",
			"action", action,
			"questionnaire_code", code,
			"status", q.GetStatus().String(),
			"result", "invalid_status",
		)
		return errors.WithCode(errorCode.ErrQuestionnaireArchived, "问卷已归档，不能编辑")
	}
	return nil
}

func (s *contentService) refreshListCache(ctx context.Context) {
	if s.listCache == nil {
		return
	}
	logQuestionnaireListCacheError(ctx, s.listCache.Rebuild(ctx))
}

// persistQuestionnaire 持久化问卷
func (s *contentService) persistQuestionnaire(ctx context.Context, q *questionnaire.Questionnaire, code string, action string) error {
	if err := s.repo.Update(ctx, q); err != nil {
		logger.L(ctx).Errorw("保存问卷失败",
			"action", action,
			"questionnaire_code", code,
			"result", "failed",
			"error", err.Error(),
		)
		return errors.WrapC(err, errorCode.ErrDatabase, "保存问卷失败")
	}
	s.refreshListCache(ctx)
	return nil
}

// logSuccess 记录成功日志
func (s *contentService) logSuccess(ctx context.Context, action string, questionnaireCode string, startTime time.Time, extraFields ...interface{}) {
	duration := time.Since(startTime)
	fields := []interface{}{
		"action", action,
		"questionnaire_code", questionnaireCode,
		"duration_ms", duration.Milliseconds(),
	}
	fields = append(fields, extraFields...)
	logger.L(ctx).Debugw("操作成功", fields...)
}

// buildQuestionFromDTO 从 DTO 构建问题领域对象
func buildQuestionFromDTO(code, stem, qType string, options []OptionDTO, required bool, description string, validationRules []validation.ValidationRule, calculationRule *calculation.CalculationRule, showController *questionnaire.ShowController) (questionnaire.Question, error) {

	// 构建选项列表
	opts := make([]questionnaire.Option, 0, len(options))
	for i, optDTO := range options {
		// 如果选项 code 为空（新增选项），自动生成一个
		optionCode := optDTO.Value
		if optionCode == "" {
			generatedCode, err := meta.GenerateCode()
			if err != nil {
				return nil, errors.WrapC(err, errorCode.ErrQuestionnaireInvalidQuestion, "生成选项编码失败")
			}
			optionCode = generatedCode.String()
		}

		opt, err := questionnaire.NewOptionWithStringCode(optionCode, optDTO.Label, float64(optDTO.Score))
		if err != nil {
			return nil, errors.WrapC(err, errorCode.ErrQuestionnaireInvalidQuestion, "第 %d 个选项创建失败: %v", i+1, err)
		}
		opts = append(opts, opt)
	}

	qOptions := []questionnaire.QuestionParamsOption{
		questionnaire.WithCode(meta.NewCode(code)),
		questionnaire.WithStem(stem),
		questionnaire.WithQuestionType(questionnaire.QuestionType(qType)),
		questionnaire.WithOptions(opts),
		questionnaire.WithTips(description),
	}

	if required {
		qOptions = append(qOptions, questionnaire.WithRequired())
	}

	// 添加校验规则
	if len(validationRules) > 0 {
		qOptions = append(qOptions, questionnaire.WithValidationRules(validationRules))
	}

	// 添加计算规则
	if calculationRule != nil {
		qOptions = append(qOptions, questionnaire.WithCalculationRule(calculationRule.GetFormula()))
	}

	// 添加显示控制器
	if showController != nil {
		qOptions = append(qOptions, questionnaire.WithShowController(showController))
	}

	// 使用领域层工厂方法创建问题
	return questionnaire.NewQuestion(qOptions...)
}
