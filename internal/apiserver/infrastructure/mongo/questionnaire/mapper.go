package questionnaire

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/calculation"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/option"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/types"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/validation"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
)

// QuestionnaireMapper 问卷映射器
type QuestionnaireMapper struct{}

// NewQuestionnaireMapper 创建问卷映射器
func NewQuestionnaireMapper() *QuestionnaireMapper {
	return &QuestionnaireMapper{}
}

// ToPO 将领域模型转换为MongoDB持久化对象
func (m *QuestionnaireMapper) ToPO(bo *questionnaire.Questionnaire) *QuestionnairePO {
	po := &QuestionnairePO{
		Code:        bo.GetCode().Value(),
		DomainID:    bo.GetID().Value(),
		Title:       bo.GetTitle(),
		Description: bo.GetDescription(),
		ImgUrl:      bo.GetImgUrl(),
		Version:     bo.GetVersion().Value(),
		Status:      bo.GetStatus().Value(),
	}

	for _, questionBO := range bo.GetQuestions() {
		questionPO := QuestionPO{
			Code:            questionBO.GetCode().Value(),
			Title:           questionBO.GetTitle(),
			QuestionType:    string(questionBO.GetType()),
			Tips:            questionBO.GetTips(),
			Placeholder:     questionBO.GetPlaceholder(),
			Options:         m.mapOptions(questionBO.GetOptions()),
			ValidationRules: m.mapValidationRules(questionBO.GetValidationRules()),
			CalculationRule: m.mapCalculationRule(questionBO.GetCalculationRule()),
		}

		// 处理计算规则（可能为nil）
		if rule := questionBO.GetCalculationRule(); rule != nil {
			questionPO.CalculationRule = CalculationRulePO{
				Formula: string(rule.GetFormulaType()),
			}
		}

		po.Questions = append(po.Questions, questionPO)
	}

	return po
}

// mapOptions 转换选项
func (m *QuestionnaireMapper) mapOptions(options []option.Option) []OptionPO {
	if options == nil {
		return []OptionPO{} // 返回空切片而不是nil
	}

	var optionsPO []OptionPO
	for _, opt := range options {
		optionsPO = append(optionsPO, OptionPO{
			Code:    opt.GetCode(),
			Content: opt.GetContent(),
			Score:   opt.GetScore(),
		})
	}
	return optionsPO
}

// mapValidationRules 转换校验规则
func (m *QuestionnaireMapper) mapValidationRules(rules []validation.ValidationRule) []ValidationRulePO {
	if rules == nil {
		return []ValidationRulePO{} // 返回空切片而不是nil
	}

	var rulesPO []ValidationRulePO
	for _, rule := range rules {
		rulesPO = append(rulesPO, ValidationRulePO{
			RuleType:    string(rule.GetRuleType()),
			TargetValue: rule.GetTargetValue(),
		})
	}
	return rulesPO
}

// mapCalculationRule 转换计算规则
func (m *QuestionnaireMapper) mapCalculationRule(rule *calculation.CalculationRule) CalculationRulePO {
	if rule == nil {
		return CalculationRulePO{}
	}
	return CalculationRulePO{
		Formula: string(rule.GetFormulaType()),
	}
}

// ToBO 将MongoDB持久化对象转换为业务对象
func (m *QuestionnaireMapper) ToBO(po *QuestionnairePO) *questionnaire.Questionnaire {
	// 创建问卷对象
	q := questionnaire.NewQuestionnaire(
		questionnaire.NewQuestionnaireCode(po.Code),
		po.Title,
		questionnaire.WithID(questionnaire.NewQuestionnaireID(po.DomainID)),
		questionnaire.WithDescription(po.Description),
		questionnaire.WithImgUrl(po.ImgUrl),
		questionnaire.WithVersion(questionnaire.NewQuestionnaireVersion(po.Version)),
		questionnaire.WithStatus(questionnaire.QuestionnaireStatus(po.Status)),
		questionnaire.WithQuestions(m.mapQuestions(po.Questions)),
	)

	return q
}

// mapQuestions 将问题PO转换为问题BO - 使用重构后的Builder和Factory
func (m *QuestionnaireMapper) mapQuestions(questionsPO []QuestionPO) []question.Question {
	if questionsPO == nil {
		return []question.Question{}
	}

	var questions []question.Question
	factory := types.NewQuestionFactory()

	for _, questionPO := range questionsPO {
		// 构建配置选项列表
		opts := []types.BuilderOption{
			types.WithCode(question.NewQuestionCode(questionPO.Code)),
			types.WithTitle(questionPO.Title),
			types.WithTips(questionPO.Tips),
			types.WithQuestionType(question.QuestionType(questionPO.QuestionType)),
			types.WithPlaceholder(questionPO.Placeholder),
			types.WithOptions(m.mapOptionsPOToBO(questionPO.Options)),
			types.WithValidationRules(m.mapValidationRulesPOToBO(questionPO.ValidationRules)),
		}

		// 添加计算规则（如果有的话）
		if questionPO.CalculationRule.Formula != "" {
			opts = append(opts, types.WithCalculationRule(calculation.FormulaType(questionPO.CalculationRule.Formula)))
		}

		// 1. 创建配置
		builder := types.BuildQuestionConfig(opts...)

		// 2. 创建对象
		questionBO := factory.CreateFromBuilder(builder)
		if questionBO != nil {
			questions = append(questions, questionBO)
		}
	}

	return questions
}

// mapOptionsPOToBO 将选项PO转换为选项BO
func (m *QuestionnaireMapper) mapOptionsPOToBO(optionsPO []OptionPO) []option.Option {
	if optionsPO == nil {
		return []option.Option{}
	}

	var options []option.Option
	for _, optionPO := range optionsPO {
		optionBO := option.NewOption(optionPO.Code, optionPO.Content, optionPO.Score)
		options = append(options, optionBO)
	}
	return options
}

// mapValidationRulesPOToBO 将校验规则PO转换为校验规则BO
func (m *QuestionnaireMapper) mapValidationRulesPOToBO(rulesPO []ValidationRulePO) []validation.ValidationRule {
	if rulesPO == nil {
		return []validation.ValidationRule{}
	}

	var rules []validation.ValidationRule
	for _, rulePO := range rulesPO {
		ruleType := validation.RuleType(rulePO.RuleType)
		rule := validation.NewValidationRule(ruleType, rulePO.TargetValue)
		rules = append(rules, rule)
	}
	return rules
}

// mapCalculationRulePOToBO 将计算规则PO转换为计算规则BO
func (m *QuestionnaireMapper) mapCalculationRulePOToBO(rulePO CalculationRulePO) *calculation.CalculationRule {
	if rulePO.Formula == "" {
		return nil
	}

	formulaType := calculation.FormulaType(rulePO.Formula)
	return calculation.NewCalculationRule(formulaType)
}
