package questionnaire

import (
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/question/vo"
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
		Title:       bo.GetTitle(),
		Description: bo.GetDescription(),
		ImgUrl:      bo.GetImgUrl(),
		Version:     bo.GetVersion().Value(),
		Status:      bo.GetStatus().Value(),
	}

	for _, questionBO := range bo.GetQuestions() {
		questionPO := QuestionPO{
			Code:            questionBO.GetCode(),
			Title:           questionBO.GetTitle(),
			QuestionType:    string(questionBO.GetType()),
			Tip:             questionBO.GetTips(),
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
func (m *QuestionnaireMapper) mapOptions(options []vo.Option) []OptionPO {
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
func (m *QuestionnaireMapper) mapValidationRules(rules []vo.ValidationRule) []ValidationRulePO {
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
func (m *QuestionnaireMapper) mapCalculationRule(rule *vo.CalculationRule) CalculationRulePO {
	if rule == nil {
		return CalculationRulePO{}
	}
	return CalculationRulePO{
		Formula: string(rule.GetFormulaType()),
	}
}

// ToBO 将MongoDB持久化对象转换为业务对象
func (m *QuestionnaireMapper) ToBO(po *QuestionnairePO) *questionnaire.Questionnaire {
	// 使用构造函数和选项模式创建问卷
	domain := questionnaire.NewQuestionnaire(
		questionnaire.NewQuestionnaireCode(po.Code),
		questionnaire.WithDescription(po.Description),
		questionnaire.WithImgUrl(po.ImgUrl),
		questionnaire.WithVersion(questionnaire.NewQuestionnaireVersion(po.Version)),
		questionnaire.WithStatus(questionnaire.QuestionnaireStatus(po.Status)),
	)

	// 添加问题（这里简化处理，实际项目中需要根据具体需求转换问题）
	// TODO: 需要实现从QuestionPO到具体Question类型的转换

	return domain
}
