package questionnaire

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
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
	// 1. 验证输入参数
	if dto.QuestionnaireCode == "" {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问卷编码不能为空")
	}
	if dto.Code == "" {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问题编码不能为空")
	}
	if dto.Stem == "" {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问题题干不能为空")
	}
	if dto.Type == "" {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问题类型不能为空")
	}

	// 2. 获取问卷
	q, err := s.repo.FindByCode(ctx, dto.QuestionnaireCode)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取问卷失败")
	}

	// 3. 判断问卷状态（已归档的问卷不能编辑）
	if q.IsArchived() {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireArchived, "问卷已归档，不能编辑")
	}

	// 4. 构建问题领域对象
	question, err := buildQuestionFromDTO(dto.Code, dto.Stem, dto.Type, dto.Options, dto.Required, dto.Description, nil)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireInvalidQuestion, "创建问题失败")
	}

	// 5. 添加问题到问卷
	if err := s.questionMgr.AddQuestion(q, question); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireInvalidQuestion, "添加问题失败")
	}

	// 6. 持久化
	if err := s.repo.Update(ctx, q); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存问卷失败")
	}

	return toQuestionnaireResult(q), nil
}

// UpdateQuestion 更新问题
func (s *contentService) UpdateQuestion(ctx context.Context, dto UpdateQuestionDTO) (*QuestionnaireResult, error) {
	// 1. 验证输入参数
	if dto.QuestionnaireCode == "" {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问卷编码不能为空")
	}
	if dto.Code == "" {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问题编码不能为空")
	}
	if dto.Stem == "" {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问题题干不能为空")
	}

	// 2. 获取问卷
	q, err := s.repo.FindByCode(ctx, dto.QuestionnaireCode)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取问卷失败")
	}

	// 3. 判断问卷状态
	if q.IsArchived() {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireArchived, "问卷已归档，不能编辑")
	}

	// 4. 构建新的问题对象
	newQuestion, err := buildQuestionFromDTO(dto.Code, dto.Stem, dto.Type, dto.Options, dto.Required, dto.Description, nil)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireInvalidQuestion, "创建问题失败")
	}

	// 5. 更新问题
	if err := s.questionMgr.UpdateQuestion(q, newQuestion); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireInvalidQuestion, "更新问题失败")
	}

	// 6. 持久化
	if err := s.repo.Update(ctx, q); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存问卷失败")
	}

	return toQuestionnaireResult(q), nil
}

// RemoveQuestion 删除问题
func (s *contentService) RemoveQuestion(ctx context.Context, questionnaireCode, questionCode string) (*QuestionnaireResult, error) {
	// 1. 验证输入参数
	if questionnaireCode == "" {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问卷编码不能为空")
	}
	if questionCode == "" {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问题编码不能为空")
	}

	// 2. 获取问卷
	q, err := s.repo.FindByCode(ctx, questionnaireCode)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取问卷失败")
	}

	// 3. 判断问卷状态
	if q.IsArchived() {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireArchived, "问卷已归档，不能编辑")
	}

	// 4. 删除问题
	if err := s.questionMgr.RemoveQuestion(q, meta.NewCode(questionCode)); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireInvalidQuestion, "删除问题失败")
	}

	// 5. 持久化
	if err := s.repo.Update(ctx, q); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存问卷失败")
	}

	return toQuestionnaireResult(q), nil
}

// ReorderQuestions 重排问题顺序
func (s *contentService) ReorderQuestions(ctx context.Context, questionnaireCode string, orderedCodes []string) (*QuestionnaireResult, error) {
	// 1. 验证输入参数
	if questionnaireCode == "" {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问卷编码不能为空")
	}
	if len(orderedCodes) == 0 {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问题编码列表不能为空")
	}

	// 2. 获取问卷
	q, err := s.repo.FindByCode(ctx, questionnaireCode)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取问卷失败")
	}

	// 3. 判断问卷状态
	if q.IsArchived() {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireArchived, "问卷已归档，不能编辑")
	}

	// 4. 转换 string 编码为 meta.Code
	metaCodes := make([]meta.Code, 0, len(orderedCodes))
	for _, code := range orderedCodes {
		metaCodes = append(metaCodes, meta.NewCode(code))
	}

	// 5. 重排问题顺序
	if err := s.questionMgr.ReorderQuestions(q, metaCodes); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireInvalidQuestion, "重排问题顺序失败")
	}

	// 6. 持久化
	if err := s.repo.Update(ctx, q); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存问卷失败")
	}

	return toQuestionnaireResult(q), nil
}

// BatchUpdateQuestions 批量更新问题
func (s *contentService) BatchUpdateQuestions(ctx context.Context, questionnaireCode string, questions []QuestionDTO) (*QuestionnaireResult, error) {
	// 1. 验证输入参数
	if questionnaireCode == "" {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问卷编码不能为空")
	}
	if len(questions) == 0 {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireInvalidInput, "问题列表不能为空")
	}

	// 2. 获取问卷
	q, err := s.repo.FindByCode(ctx, questionnaireCode)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireNotFound, "获取问卷失败")
	}

	// 3. 判断问卷状态
	if q.IsArchived() {
		return nil, errors.WithCode(errorCode.ErrQuestionnaireArchived, "问卷已归档，不能编辑")
	}

	// 4. 转换 DTO 为领域对象
	domainQuestions := make([]questionnaire.Question, 0, len(questions))
	for i, qDTO := range questions {
		question, err := buildQuestionFromDTO(qDTO.Code, qDTO.Stem, qDTO.Type, qDTO.Options, qDTO.Required, qDTO.Description, qDTO.ValidationRules)
		if err != nil {
			return nil, errors.WrapC(err, errorCode.ErrQuestionnaireInvalidQuestion, "第 %d 个问题创建失败", i+1)
		}
		domainQuestions = append(domainQuestions, question)
	}

	// 5. 批量更新问题（使用 ReplaceQuestions）
	if err := s.questionMgr.ReplaceQuestions(q, domainQuestions); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrQuestionnaireInvalidQuestion, "批量更新问题失败")
	}

	// 6. 持久化
	if err := s.repo.Update(ctx, q); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存问卷失败")
	}

	return toQuestionnaireResult(q), nil
}

// buildQuestionFromDTO 从 DTO 构建问题领域对象
func buildQuestionFromDTO(code, stem, qType string, options []OptionDTO, required bool, description string, validationRules []validation.ValidationRule) (questionnaire.Question, error) {
	// 构建选项列表
	opts := make([]questionnaire.Option, 0, len(options))
	for _, optDTO := range options {
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
			return nil, err
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

	// 使用领域层工厂方法创建问题
	return questionnaire.NewQuestion(qOptions...)
}
