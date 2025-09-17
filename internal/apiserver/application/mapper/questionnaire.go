package mapper

import (
	"errors"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/application/dto"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/question"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/question/ability"
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
		ID:          bo.GetID().Value(),
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
func (m *QuestionnaireMapper) toQuestionDTOs(questions []question.Question) []dto.QuestionDTO {
	if len(questions) == 0 {
		return nil
	}

	dtos := make([]dto.QuestionDTO, 0, len(questions))
	for _, q := range questions {
		dtos = append(dtos, dto.QuestionDTO{
			Code:            string(q.GetCode()),
			Title:           q.GetTitle(),
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
func (m *QuestionnaireMapper) toOptionDTOs(options []question.Option) []dto.OptionDTO {
	if len(options) == 0 {
		return nil
	}

	dtos := make([]dto.OptionDTO, 0, len(options))
	for _, o := range options {
		dtos = append(dtos, dto.OptionDTO{
			Code:    string(o.GetCode()),
			Content: o.GetContent(),
			Score:   o.GetScore(),
		})
	}
	return dtos
}

// toValidationRuleDTOs 将验证规则领域对象转换为 DTO
func (m *QuestionnaireMapper) toValidationRuleDTOs(rules []ability.ValidationRule) []dto.ValidationRuleDTO {
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
func (m *QuestionnaireMapper) toCalculationRuleDTO(rule *ability.CalculationRule) *dto.CalculationRuleDTO {
	if rule == nil {
		return nil
	}

	return &dto.CalculationRuleDTO{
		FormulaType: string(rule.GetFormulaType()),
	}
}

// QuestionFromDTO 将问题 DTO 转换为领域对象
func (m *QuestionnaireMapper) QuestionFromDTO(dto *dto.QuestionDTO) (question.Question, error) {
	if dto == nil {
		return nil, errors.New("问题 DTO 不能为空")
	}

	// 创建问题构建器
	builder := question.NewQuestionBuilder()

	// 设置基本属性
	builder.SetCode(question.QuestionCode(dto.Code))
	builder.SetTitle(dto.Title)
	builder.SetTips(dto.Tips)
	builder.SetQuestionType(question.QuestionType(dto.Type))
	builder.SetPlaceholder(dto.Tips)

	// 设置选项
	if len(dto.Options) > 0 {
		for _, optionDTO := range dto.Options {
			builder.AddOption(optionDTO.Code, optionDTO.Content, optionDTO.Score)
		}
	}

	// 设置验证规则
	if len(dto.ValidationRules) > 0 {
		for _, ruleDTO := range dto.ValidationRules {
			builder.AddValidationRule(ability.RuleType(ruleDTO.RuleType), ruleDTO.TargetValue)
		}
	}

	// 设置计算规则
	if dto.CalculationRule != nil {
		builder.SetCalculationRule(ability.FormulaType(dto.CalculationRule.FormulaType))
	}

	// 使用工厂函数创建问题
	q := question.CreateQuestionFromBuilder(builder)
	if q == nil {
		return nil, errors.New("创建问题失败")
	}

	return q, nil
}

// FromDTO 将 DTO 转换为领域对象
func (m *QuestionnaireMapper) FromDTO(dto *dto.QuestionnaireDTO) (*questionnaire.Questionnaire, error) {
	if dto == nil {
		return nil, errors.New("问卷 DTO 不能为空")
	}

	// 构建选项列表
	opts := []questionnaire.QuestionnaireOption{
		questionnaire.WithID(questionnaire.NewQuestionnaireID(dto.ID)),
		questionnaire.WithDescription(dto.Description),
		questionnaire.WithImgUrl(dto.ImgUrl),
		questionnaire.WithVersion(questionnaire.NewQuestionnaireVersion(dto.Version)),
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
		questions := make([]question.Question, 0, len(dto.Questions))
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
	return questionnaire.NewQuestionnaire(
		questionnaire.NewQuestionnaireCode(dto.Code),
		dto.Title,
		opts...,
	), nil
}
