package questionnaire

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
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
}

// NewContentService 创建问卷内容编辑服务
func NewContentService(
	repo questionnaire.Repository,
	questionMgr questionnaire.QuestionManager,
) QuestionnaireContentService {
	return &contentService{
		repo:        repo,
		questionMgr: questionMgr,
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
	if dto.QuestionnaireCode == "" {
		l.Warnw("问卷编码为空",
			"action", "add_question",
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问卷编码不能为空")
	}
	if dto.Code == "" {
		l.Warnw("问题编码为空",
			"action", "add_question",
			"questionnaire_code", dto.QuestionnaireCode,
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问题编码不能为空")
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
	l.Debugw("查询问卷",
		"action", "add_question",
		"questionnaire_code", dto.QuestionnaireCode,
	)
	q, err := s.repo.FindByCode(ctx, dto.QuestionnaireCode)
	if err != nil {
		l.Errorw("获取问卷失败",
			"action", "add_question",
			"questionnaire_code", dto.QuestionnaireCode,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取问卷失败")
	}

	// 3. 判断问卷状态（已归档的问卷不能编辑）
	if q.IsArchived() {
		l.Warnw("问卷已归档，不能编辑",
			"action", "add_question",
			"questionnaire_code", dto.QuestionnaireCode,
			"status", q.GetStatus().String(),
			"result", "invalid_status",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireArchived, "问卷已归档，不能编辑")
	}

	// 4. 构建问题领域对象
	l.Debugw("构建问题领域对象",
		"action", "add_question",
		"questionnaire_code", dto.QuestionnaireCode,
		"question_code", dto.Code,
		"question_type", dto.Type,
	)
	question, err := buildQuestionFromDTO(dto.Code, dto.Stem, dto.Type, dto.Options, dto.Required, dto.Description, nil, nil)
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
	l.Debugw("添加问题到问卷",
		"action", "add_question",
		"questionnaire_code", dto.QuestionnaireCode,
		"question_code", dto.Code,
	)
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
	l.Debugw("保存问卷",
		"action", "add_question",
		"questionnaire_code", dto.QuestionnaireCode,
	)
	if err := s.repo.Update(ctx, q); err != nil {
		l.Errorw("保存问卷失败",
			"action", "add_question",
			"questionnaire_code", dto.QuestionnaireCode,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存问卷失败")
	}

	duration := time.Since(startTime)
	l.Debugw("添加问题成功",
		"action", "add_question",
		"questionnaire_code", dto.QuestionnaireCode,
		"question_code", dto.Code,
		"questions_count", len(q.GetQuestions()),
		"duration_ms", duration.Milliseconds(),
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
	if dto.QuestionnaireCode == "" {
		l.Warnw("问卷编码为空",
			"action", "update_question",
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问卷编码不能为空")
	}
	if dto.Code == "" {
		l.Warnw("问题编码为空",
			"action", "update_question",
			"questionnaire_code", dto.QuestionnaireCode,
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问题编码不能为空")
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
	l.Debugw("查询问卷",
		"action", "update_question",
		"questionnaire_code", dto.QuestionnaireCode,
	)
	q, err := s.repo.FindByCode(ctx, dto.QuestionnaireCode)
	if err != nil {
		l.Errorw("获取问卷失败",
			"action", "update_question",
			"questionnaire_code", dto.QuestionnaireCode,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取问卷失败")
	}

	// 3. 判断问卷状态
	if q.IsArchived() {
		l.Warnw("问卷已归档，不能编辑",
			"action", "update_question",
			"questionnaire_code", dto.QuestionnaireCode,
			"status", q.GetStatus().String(),
			"result", "invalid_status",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireArchived, "问卷已归档，不能编辑")
	}

	// 4. 构建新的问题对象
	l.Debugw("构建问题领域对象",
		"action", "update_question",
		"questionnaire_code", dto.QuestionnaireCode,
		"question_code", dto.Code,
	)
	newQuestion, err := buildQuestionFromDTO(dto.Code, dto.Stem, dto.Type, dto.Options, dto.Required, dto.Description, nil, nil)
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
	l.Debugw("更新问题",
		"action", "update_question",
		"questionnaire_code", dto.QuestionnaireCode,
		"question_code", dto.Code,
	)
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
	l.Debugw("保存问卷",
		"action", "update_question",
		"questionnaire_code", dto.QuestionnaireCode,
	)
	if err := s.repo.Update(ctx, q); err != nil {
		l.Errorw("保存问卷失败",
			"action", "update_question",
			"questionnaire_code", dto.QuestionnaireCode,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存问卷失败")
	}

	duration := time.Since(startTime)
	l.Debugw("更新问题成功",
		"action", "update_question",
		"questionnaire_code", dto.QuestionnaireCode,
		"question_code", dto.Code,
		"duration_ms", duration.Milliseconds(),
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
	if questionnaireCode == "" {
		l.Warnw("问卷编码为空",
			"action", "remove_question",
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问卷编码不能为空")
	}
	if questionCode == "" {
		l.Warnw("问题编码为空",
			"action", "remove_question",
			"questionnaire_code", questionnaireCode,
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问题编码不能为空")
	}

	// 2. 获取问卷
	l.Debugw("查询问卷",
		"action", "remove_question",
		"questionnaire_code", questionnaireCode,
	)
	q, err := s.repo.FindByCode(ctx, questionnaireCode)
	if err != nil {
		l.Errorw("获取问卷失败",
			"action", "remove_question",
			"questionnaire_code", questionnaireCode,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取问卷失败")
	}

	// 3. 判断问卷状态
	if q.IsArchived() {
		l.Warnw("问卷已归档，不能编辑",
			"action", "remove_question",
			"questionnaire_code", questionnaireCode,
			"status", q.GetStatus().String(),
			"result", "invalid_status",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireArchived, "问卷已归档，不能编辑")
	}

	// 4. 删除问题
	l.Debugw("删除问题",
		"action", "remove_question",
		"questionnaire_code", questionnaireCode,
		"question_code", questionCode,
	)
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
	l.Debugw("保存问卷",
		"action", "remove_question",
		"questionnaire_code", questionnaireCode,
	)
	if err := s.repo.Update(ctx, q); err != nil {
		l.Errorw("保存问卷失败",
			"action", "remove_question",
			"questionnaire_code", questionnaireCode,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存问卷失败")
	}

	duration := time.Since(startTime)
	l.Debugw("删除问题成功",
		"action", "remove_question",
		"questionnaire_code", questionnaireCode,
		"question_code", questionCode,
		"questions_count", len(q.GetQuestions()),
		"duration_ms", duration.Milliseconds(),
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
	if questionnaireCode == "" {
		l.Warnw("问卷编码为空",
			"action", "reorder_questions",
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问卷编码不能为空")
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
	l.Debugw("查询问卷",
		"action", "reorder_questions",
		"questionnaire_code", questionnaireCode,
	)
	q, err := s.repo.FindByCode(ctx, questionnaireCode)
	if err != nil {
		l.Errorw("获取问卷失败",
			"action", "reorder_questions",
			"questionnaire_code", questionnaireCode,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取问卷失败")
	}

	// 3. 判断问卷状态
	if q.IsArchived() {
		l.Warnw("问卷已归档，不能编辑",
			"action", "reorder_questions",
			"questionnaire_code", questionnaireCode,
			"status", q.GetStatus().String(),
			"result", "invalid_status",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireArchived, "问卷已归档，不能编辑")
	}

	// 4. 转换 string 编码为 meta.Code
	metaCodes := make([]meta.Code, 0, len(orderedCodes))
	for _, code := range orderedCodes {
		metaCodes = append(metaCodes, meta.NewCode(code))
	}

	// 5. 重排问题顺序
	l.Debugw("执行重排问题顺序",
		"action", "reorder_questions",
		"questionnaire_code", questionnaireCode,
		"ordered_codes_count", len(metaCodes),
	)
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
	l.Debugw("保存问卷",
		"action", "reorder_questions",
		"questionnaire_code", questionnaireCode,
	)
	if err := s.repo.Update(ctx, q); err != nil {
		l.Errorw("保存问卷失败",
			"action", "reorder_questions",
			"questionnaire_code", questionnaireCode,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存问卷失败")
	}

	duration := time.Since(startTime)
	l.Debugw("重排问题顺序成功",
		"action", "reorder_questions",
		"questionnaire_code", questionnaireCode,
		"questions_count", len(q.GetQuestions()),
		"duration_ms", duration.Milliseconds(),
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
	if questionnaireCode == "" {
		l.Warnw("问卷编码为空",
			"action", "batch_update_questions",
			"result", "invalid_params",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问卷编码不能为空")
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
	l.Debugw("查询问卷",
		"action", "batch_update_questions",
		"questionnaire_code", questionnaireCode,
	)
	q, err := s.repo.FindByCode(ctx, questionnaireCode)
	if err != nil {
		l.Errorw("获取问卷失败",
			"action", "batch_update_questions",
			"questionnaire_code", questionnaireCode,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取问卷失败")
	}

	// 3. 判断问卷状态
	if q.IsArchived() {
		l.Warnw("问卷已归档，不能编辑",
			"action", "batch_update_questions",
			"questionnaire_code", questionnaireCode,
			"status", q.GetStatus().String(),
			"result", "invalid_status",
		)
		return nil, errors.WithCode(errorCode.ErrQuestionnaireArchived, "问卷已归档，不能编辑")
	}

	// 4. 转换 DTO 为领域对象
	l.Debugw("转换问题 DTO 为领域对象",
		"action", "batch_update_questions",
		"questionnaire_code", questionnaireCode,
		"questions_count", len(questions),
	)
	domainQuestions := make([]questionnaire.Question, 0, len(questions))
	for i, qDTO := range questions {
		l.Debugw("构建问题领域对象",
			"action", "batch_update_questions",
			"questionnaire_code", questionnaireCode,
			"question_index", i+1,
			"question_code", qDTO.Code,
			"question_type", qDTO.Type,
			"options_count", len(qDTO.Options),
			"validation_rules_count", len(qDTO.ValidationRules),
		)
		question, err := buildQuestionFromDTO(qDTO.Code, qDTO.Stem, qDTO.Type, qDTO.Options, qDTO.Required, qDTO.Description, qDTO.ValidationRules, qDTO.ShowController)
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
	l.Debugw("批量替换问题",
		"action", "batch_update_questions",
		"questionnaire_code", questionnaireCode,
		"old_questions_count", len(q.GetQuestions()),
		"new_questions_count", len(domainQuestions),
	)
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
	l.Debugw("保存问卷",
		"action", "batch_update_questions",
		"questionnaire_code", questionnaireCode,
	)
	if err := s.repo.Update(ctx, q); err != nil {
		l.Errorw("保存问卷失败",
			"action", "batch_update_questions",
			"questionnaire_code", questionnaireCode,
			"result", "failed",
			"error", err.Error(),
		)
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存问卷失败")
	}

	duration := time.Since(startTime)
	l.Debugw("批量更新问题成功",
		"action", "batch_update_questions",
		"questionnaire_code", questionnaireCode,
		"questions_count", len(domainQuestions),
		"duration_ms", duration.Milliseconds(),
	)

	return toQuestionnaireResult(q), nil
}

// buildQuestionFromDTO 从 DTO 构建问题领域对象
func buildQuestionFromDTO(code, stem, qType string, options []OptionDTO, required bool, description string, validationRules []validation.ValidationRule, showController *questionnaire.ShowController) (questionnaire.Question, error) {

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

	// 添加显示控制器
	if showController != nil {
		qOptions = append(qOptions, questionnaire.WithShowController(showController))
	}

	// 使用领域层工厂方法创建问题
	return questionnaire.NewQuestion(qOptions...)
}
