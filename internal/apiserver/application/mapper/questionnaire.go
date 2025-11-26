package mapper

import (
	"errors"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/dto"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/validation"
	"github.com/FangcunMount/qs-server/internal/pkg/calculation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// QuestionnaireMapper DTO 与领域对象转换器
type QuestionnaireMapper struct{}

// NewQuestionnaireMapper 创建问卷映射器
func NewQuestionnaireMapper() QuestionnaireMapper {
	return QuestionnaireMapper{}
}

// ToDTO 将领域对象转换为 DTO
func (m *QuestionnaireMapper) ToDTO(bo *questionnaire.Questionnaire) *dto.QuestionnaireDTO {
	if bo == nil {
		return nil
	}

	return &dto.QuestionnaireDTO{
		ID:          bo.GetID(),
		Code:        bo.GetCode().Value(),
		Version:     bo.GetVersion().Value(),
		Title:       bo.GetTitle(),
		Description: bo.GetDescription(),
		ImgUrl:      bo.GetImgUrl(),
		Status:      bo.GetStatus().String(),
		Questions:   m.toQuestionDTOs(bo.GetQuestions()),
	}
}

// toQuestionDTOs 将问题领域对象转换为 DTO
func (m *QuestionnaireMapper) toQuestionDTOs(questions []questionnaire.Question) []dto.QuestionDTO {
	if len(questions) == 0 {
		return nil
	}

	dtos := make([]dto.QuestionDTO, 0, len(questions))
	for _, q := range questions {
		dtos = append(dtos, dto.QuestionDTO{
			Code:            q.GetCode().Value(),
			Stem:            q.GetStem(),
			Type:            string(q.GetType()),
			Tips:            q.GetTips(),
			Options:         m.toOptionDTOs(q.GetOptions()),
			Placeholder:     q.GetPlaceholder(),
			ValidationRules: m.toValidationRuleDTOs(q.GetValidationRules()),
			CalculationRule: m.toCalculationRuleDTO(q.GetCalculationRule()),
		})
	}
	return dtos
}

// toOptionDTOs 将选项领域对象转换为 DTO
func (m *QuestionnaireMapper) toOptionDTOs(options []questionnaire.Option) []dto.OptionDTO {
	if len(options) == 0 {
		return nil
	}

	dtos := make([]dto.OptionDTO, 0, len(options))
	for _, o := range options {
		dtos = append(dtos, dto.OptionDTO{
			Code:    o.GetCode().Value(),
			Content: o.GetContent(),
			Score:   o.GetScore(),
		})
	}
	return dtos
}

// toValidationRuleDTOs 将验证规则领域对象转换为 DTO
func (m *QuestionnaireMapper) toValidationRuleDTOs(rules []validation.ValidationRule) []dto.ValidationRuleDTO {
	if len(rules) == 0 {
		return nil
	}

	dtos := make([]dto.ValidationRuleDTO, 0, len(rules))
	for _, r := range rules {
		dtos = append(dtos, dto.ValidationRuleDTO{
			RuleType:    string(r.GetRuleType()),
			TargetValue: r.GetTargetValue(),
		})
	}
	return dtos
}

// toCalculationRuleDTO 将计算规则领域对象转换为 DTO
func (m *QuestionnaireMapper) toCalculationRuleDTO(rule *calculation.CalculationRule) *dto.CalculationRuleDTO {
	if rule == nil {
		return nil
	}

	return &dto.CalculationRuleDTO{
		FormulaType: string(rule.GetFormula()),
	}
}

// QuestionFromDTO 将问题 DTO 转换为领域对象
func (m *QuestionnaireMapper) QuestionFromDTO(dto *dto.QuestionDTO) (questionnaire.Question, error) {
	if dto == nil {
		return nil, errors.New("问题 DTO 不能为空")
	}

	opts := []questionnaire.QuestionParamsOption{
		questionnaire.WithCode(meta.NewCode(dto.Code)),
		questionnaire.WithStem(dto.Stem),
		questionnaire.WithTips(dto.Tips),
		questionnaire.WithQuestionType(questionnaire.QuestionType(dto.Type)),
		questionnaire.WithPlaceholder(dto.Placeholder),
	}

	// 设置选项
	if len(dto.Options) > 0 {
		options := make([]questionnaire.Option, 0, len(dto.Options))
		for _, optionDTO := range dto.Options {
			if opt, err := questionnaire.NewOptionWithStringCode(optionDTO.Code, optionDTO.Content, optionDTO.Score); err == nil {
				options = append(options, opt)
			}
		}
		opts = append(opts, questionnaire.WithOptions(options))
	}

	// 设置验证规则
	if len(dto.ValidationRules) > 0 {
		for _, ruleDTO := range dto.ValidationRules {
			opts = append(opts, questionnaire.WithValidationRule(validation.RuleType(ruleDTO.RuleType), ruleDTO.TargetValue))
		}
	}

	// 设置计算规则
	if dto.CalculationRule != nil {
		opts = append(opts, questionnaire.WithCalculationRule(calculation.FormulaType(dto.CalculationRule.FormulaType)))
	}

	questionBO, err := questionnaire.NewQuestion(opts...)
	if err != nil {
		return nil, err
	}

	return questionBO, nil
}

// FromDTO 将 DTO 转换为领域对象
func (m *QuestionnaireMapper) FromDTO(dto *dto.QuestionnaireDTO) (*questionnaire.Questionnaire, error) {
	if dto == nil {
		return nil, errors.New("问卷 DTO 不能为空")
	}

	// 构建选项列表
	opts := []questionnaire.QuestionnaireOption{
		questionnaire.WithID(dto.ID),
		questionnaire.WithDesc(dto.Description),
		questionnaire.WithImgUrl(dto.ImgUrl),
		questionnaire.WithVersion(questionnaire.NewVersion(dto.Version)),
	}

	// 设置状态
	switch dto.Status {
	case "draft":
		opts = append(opts, questionnaire.WithStatus(questionnaire.STATUS_DRAFT))
	case "published":
		opts = append(opts, questionnaire.WithStatus(questionnaire.STATUS_PUBLISHED))
	case "unpublished":
		opts = append(opts, questionnaire.WithStatus(questionnaire.STATUS_ARCHIVED))
	default:
		return nil, errors.New("无效的问卷状态")
	}

	// 如果有问题列表，则转换问题
	if len(dto.Questions) > 0 {
		questions := make([]questionnaire.Question, 0, len(dto.Questions))
		for _, qDTO := range dto.Questions {
			q, err := m.QuestionFromDTO(&qDTO)
			if err != nil {
				return nil, err
			}
			questions = append(questions, q)
		}
		opts = append(opts, questionnaire.WithQuestions(questions))
	}

	// 创建问卷对象
	q, err := questionnaire.NewQuestionnaire(
		meta.NewCode(dto.Code),
		dto.Title,
		opts...,
	)
	if err != nil {
		return nil, err
	}
	return q, nil
}
