package questionnaire

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/validation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
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
		Status:      bo.GetStatus().String(),
		Type:        bo.GetType().String(),
	}
	po.CreatedAt = bo.GetCreatedAt()
	po.CreatedBy = bo.GetCreatedBy().Uint64()
	po.UpdatedAt = bo.GetUpdatedAt()
	po.UpdatedBy = bo.GetUpdatedBy().Uint64()

	for _, questionBO := range bo.GetQuestions() {
		questionPO := QuestionPO{
			Code:            questionBO.GetCode().Value(),
			Title:           questionBO.GetStem(),
			QuestionType:    string(questionBO.GetType()),
			Tips:            questionBO.GetTips(),
			Placeholder:     questionBO.GetPlaceholder(),
			Options:         m.mapOptions(questionBO.GetOptions()),
			ValidationRules: m.mapValidationRules(questionBO.GetValidationRules()),
			CalculationRule: m.mapCalculationRule(questionBO.GetCalculationRule()),
			ShowController:  m.mapShowController(questionBO.GetShowController()),
		}

		po.Questions = append(po.Questions, questionPO)
	}

	return po
}

// mapOptions 转换选项
func (m *QuestionnaireMapper) mapOptions(options []questionnaire.Option) []OptionPO {
	if options == nil {
		return []OptionPO{} // 返回空切片而不是nil
	}

	var optionsPO []OptionPO
	for _, opt := range options {
		optionsPO = append(optionsPO, OptionPO{
			Code:    opt.GetCode().Value(),
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
		Formula: string(rule.GetFormula()),
	}
}

// ToBO 将MongoDB持久化对象转换为业务对象
func (m *QuestionnaireMapper) ToBO(po *QuestionnairePO) *questionnaire.Questionnaire {
	// 创建问卷对象（code 是唯一标识，不再使用 ID）
	opts := []questionnaire.QuestionnaireOption{
		questionnaire.WithDesc(po.Description),
		questionnaire.WithImgUrl(po.ImgUrl),
		questionnaire.WithVersion(questionnaire.NewVersion(po.Version)),
		questionnaire.WithStatus(questionnaire.Status(po.Status)),
		questionnaire.WithType(questionnaire.NormalizeQuestionnaireType(po.Type)),
		questionnaire.WithQuestionCount(po.QuestionCount),
		questionnaire.WithCreatedBy(meta.FromUint64(po.CreatedBy)),
		questionnaire.WithCreatedAt(po.CreatedAt),
		questionnaire.WithUpdatedBy(meta.FromUint64(po.UpdatedBy)),
		questionnaire.WithUpdatedAt(po.UpdatedAt),
	}
	if po.Questions != nil {
		opts = append(opts, questionnaire.WithQuestions(m.mapQuestions(po.Questions)))
	}

	q, _ := questionnaire.NewQuestionnaire(
		meta.NewCode(po.Code),
		po.Title,
		opts...,
	)

	return q
}

// mapQuestions 将问题PO转换为问题BO - 使用重构后的Builder和Factory
func (m *QuestionnaireMapper) mapQuestions(questionsPO []QuestionPO) []questionnaire.Question {
	if questionsPO == nil {
		return []questionnaire.Question{}
	}

	var questions []questionnaire.Question

	for _, questionPO := range questionsPO {
		// 构建配置选项列表
		opts := []questionnaire.QuestionParamsOption{
			questionnaire.WithCode(meta.NewCode(questionPO.Code)),
			questionnaire.WithStem(questionPO.Title),
			questionnaire.WithTips(questionPO.Tips),
			questionnaire.WithQuestionType(questionnaire.QuestionType(questionPO.QuestionType)),
			questionnaire.WithPlaceholder(questionPO.Placeholder),
			questionnaire.WithOptions(m.mapOptionsPOToBO(questionPO.Options)),
			questionnaire.WithValidationRules(m.mapValidationRulesPOToBO(questionPO.ValidationRules)),
		}

		// 添加计算规则（如果有的话）
		if questionPO.CalculationRule.Formula != "" {
			opts = append(opts, questionnaire.WithCalculationRule(calculation.FormulaType(questionPO.CalculationRule.Formula)))
		}

		// 添加显示控制器（如果有的话）
		if showController := m.mapShowControllerPOToBO(questionPO.ShowController); showController != nil {
			opts = append(opts, questionnaire.WithShowController(showController))
		}

		questionBO, err := questionnaire.NewQuestion(opts...)
		if err != nil {
			// 跳过不符合条件的问题
			continue
		}
		questions = append(questions, questionBO)
	}

	return questions
}

// mapOptionsPOToBO 将选项PO转换为选项BO
func (m *QuestionnaireMapper) mapOptionsPOToBO(optionsPO []OptionPO) []questionnaire.Option {
	if optionsPO == nil {
		return []questionnaire.Option{}
	}

	var options []questionnaire.Option
	for _, optionPO := range optionsPO {
		if optionBO, err := questionnaire.NewOptionWithStringCode(optionPO.Code, optionPO.Content, optionPO.Score); err == nil {
			options = append(options, optionBO)
		}
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
	return calculation.NewCalculationRule(formulaType, []string{})
}

// mapShowController 将显示控制器BO转换为PO
func (m *QuestionnaireMapper) mapShowController(sc *questionnaire.ShowController) *ShowControllerPO {
	if sc == nil || sc.IsEmpty() {
		return nil
	}

	conditions := make([]ShowControllerConditionPO, 0, len(sc.GetQuestions()))
	for _, cond := range sc.GetQuestions() {
		optionCodes := make([]string, 0, len(cond.SelectOptionCodes))
		for _, code := range cond.SelectOptionCodes {
			optionCodes = append(optionCodes, code.Value())
		}
		conditions = append(conditions, ShowControllerConditionPO{
			Code:              cond.Code.Value(),
			SelectOptionCodes: optionCodes,
		})
	}

	return &ShowControllerPO{
		Rule:      sc.GetRule(),
		Questions: conditions,
	}
}

// mapShowControllerPOToBO 将显示控制器PO转换为BO
func (m *QuestionnaireMapper) mapShowControllerPOToBO(scPO *ShowControllerPO) *questionnaire.ShowController {
	if scPO == nil || scPO.Rule == "" || len(scPO.Questions) == 0 {
		return nil
	}

	conditions := make([]questionnaire.ShowControllerCondition, 0, len(scPO.Questions))
	for _, condPO := range scPO.Questions {
		optionCodes := make([]meta.Code, 0, len(condPO.SelectOptionCodes))
		for _, codeStr := range condPO.SelectOptionCodes {
			optionCodes = append(optionCodes, meta.NewCode(codeStr))
		}
		conditions = append(conditions, questionnaire.NewShowControllerCondition(
			meta.NewCode(condPO.Code),
			optionCodes,
		))
	}

	return questionnaire.NewShowController(scPO.Rule, conditions)
}
