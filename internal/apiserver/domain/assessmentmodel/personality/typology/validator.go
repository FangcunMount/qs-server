package typology

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
)

// QuestionnaireSnapshot is the minimal questionnaire shape needed to validate a runtime spec.
type QuestionnaireSnapshot struct {
	Code      string
	Version   string
	Questions []QuestionSnapshot
}

// QuestionSnapshot is the minimal question shape needed to validate references.
type QuestionSnapshot struct {
	Code        string
	OptionCodes []string
}

// ValidateRuntimeSpecForPublish performs the strong validation gate used before publishing.
func ValidateRuntimeSpecForPublish(spec *RuntimeSpec, questionnaire QuestionnaireSnapshot) []assessmentmodel.DomainValidationIssue {
	return ValidateRuntimeSpecForPublishWithContext(spec, questionnaire, RuntimeSpecValidationContext{})
}

// RuntimeSpecValidationContext carries payload-level metadata needed by publish validation.
type RuntimeSpecValidationContext struct {
	Algorithm assessmentmodel.Algorithm
	Outcomes  []Outcome
}

// ValidateRuntimeSpecForPublishWithContext performs the strong validation gate used before publishing.
func ValidateRuntimeSpecForPublishWithContext(spec *RuntimeSpec, questionnaire QuestionnaireSnapshot, validationContext RuntimeSpecValidationContext) []assessmentmodel.DomainValidationIssue {
	validator := runtimeSpecValidator{
		questions: map[string]map[string]struct{}{},
		algorithm: validationContext.Algorithm,
		outcomes:  map[string]Outcome{},
	}
	for _, question := range questionnaire.Questions {
		options := make(map[string]struct{}, len(question.OptionCodes))
		for _, optionCode := range question.OptionCodes {
			options[optionCode] = struct{}{}
		}
		if question.Code != "" {
			validator.questions[question.Code] = options
		}
	}
	validator.loadOutcomes(validationContext.Outcomes)
	validator.validate(spec)
	return validator.issues
}

type runtimeSpecValidator struct {
	questions map[string]map[string]struct{}
	algorithm assessmentmodel.Algorithm
	outcomes  map[string]Outcome
	issues    []assessmentmodel.DomainValidationIssue
}

func (v *runtimeSpecValidator) loadOutcomes(outcomes []Outcome) {
	for _, outcome := range outcomes {
		if outcome.Code == "" {
			v.add("outcomes.code", "outcome.code.required", "outcome code 不能为空")
			continue
		}
		if _, exists := v.outcomes[outcome.Code]; exists {
			v.add("outcomes."+outcome.Code, "outcome.code.duplicated", fmt.Sprintf("outcome code %s 重复", outcome.Code))
			continue
		}
		v.outcomes[outcome.Code] = outcome
		if outcome.Name == "" {
			v.add("outcomes."+outcome.Code+".title", "outcome.title.required", fmt.Sprintf("outcome %s 标题不能为空", outcome.Code))
		}
	}
}

func (v *runtimeSpecValidator) validate(spec *RuntimeSpec) {
	if spec == nil {
		v.add("definition.payload", "definition.payload.required", "runtime spec is required")
		return
	}
	v.validateFactorGraph(spec.FactorGraph)
	v.validateDecision(*spec)
	v.validateOutcomeMapping(spec.OutcomeMapping, spec.Decision.Kind)
	v.validateSpecialRules(spec.SpecialRules)
	v.validateReport(spec.Report, spec.OutcomeMapping, spec.Decision.Kind)
}

func (v *runtimeSpecValidator) validateFactorGraph(graph FactorGraphSpec) {
	if !graph.HasExplicitFactorGraph() {
		v.add("factor_graph", "factor_graph.explicit.required", "人格测评发布必须使用 explicit factor graph")
		return
	}
	for _, root := range graph.Roots {
		if _, ok := graph.Factors[root]; !ok {
			v.add("factor_graph.roots", "factor_graph.root.not_found", fmt.Sprintf("root factor %s 不存在", root))
		}
	}
	for key, factor := range graph.Factors {
		v.validateFactor(key, factor, graph.Factors)
	}
	v.detectCycles(graph)
}

func (v *runtimeSpecValidator) validateFactor(key string, factor FactorSpec, factors map[string]FactorSpec) {
	if factor.ID == "" {
		v.add("factor_graph.factors."+key+".id", "factor_graph.factor.id.required", "factor id 不能为空")
	}
	if factor.Code == "" {
		v.add("factor_graph.factors."+key+".code", "factor_graph.factor.code.required", "factor code 不能为空")
	}
	switch factor.Kind {
	case FactorSpecKindLeaf:
		if len(factor.Contributions) == 0 {
			v.add("factor_graph.factors."+key+".contributions", "factor_graph.leaf.contributions.required", "leaf factor 必须配置题目贡献")
		}
		for _, contribution := range factor.Contributions {
			v.validateContribution(key, contribution)
		}
	case FactorSpecKindComposite:
		if len(factor.Children) == 0 {
			v.add("factor_graph.factors."+key+".children", "factor_graph.composite.children.required", "composite factor 必须配置 children")
		}
		for _, child := range factor.Children {
			if _, ok := factors[child]; !ok {
				v.add("factor_graph.factors."+key+".children", "factor_graph.factor.not_found", fmt.Sprintf("child factor %s 不存在", child))
			}
			if factor.Aggregation == FactorAggregationWeightedAvg {
				if _, ok := factor.Weights[child]; !ok {
					v.add("factor_graph.factors."+key+".weights", "factor_graph.weight.required", fmt.Sprintf("weighted_avg 缺少 child %s 的权重", child))
				}
			}
		}
	default:
		v.add("factor_graph.factors."+key+".kind", "factor_graph.factor.kind.unsupported", "factor kind 必须是 leaf 或 composite")
	}
}

func (v *runtimeSpecValidator) validateContribution(factorKey string, contribution FactorContributionSpec) {
	if contribution.QuestionCode == "" {
		v.add("factor_graph.factors."+factorKey+".contributions.question_code", "question_mapping.question_code.required", "question_code 不能为空")
		return
	}
	options, ok := v.questions[contribution.QuestionCode]
	if !ok {
		v.add("factor_graph.factors."+factorKey+".contributions.question_code", "question_mapping.question_not_found", fmt.Sprintf("题目 %s 不存在", contribution.QuestionCode))
		return
	}
	for optionCode := range contribution.OptionScores {
		if _, ok := options[optionCode]; !ok {
			v.add("factor_graph.factors."+factorKey+".contributions.option_scores", "question_mapping.option_not_found", fmt.Sprintf("题目 %s 的选项 %s 不存在", contribution.QuestionCode, optionCode))
		}
	}
}

func (v *runtimeSpecValidator) detectCycles(graph FactorGraphSpec) {
	const (
		visiting = 1
		visited  = 2
	)
	state := map[string]int{}
	var walk func(string) bool
	walk = func(id string) bool {
		switch state[id] {
		case visiting:
			v.add("factor_graph.factors."+id+".children", "factor_graph.cycle_detected", fmt.Sprintf("factor graph 存在循环依赖：%s", id))
			return true
		case visited:
			return false
		}
		state[id] = visiting
		for _, child := range graph.Factors[id].Children {
			if _, ok := graph.Factors[child]; ok && walk(child) {
				return true
			}
		}
		state[id] = visited
		return false
	}
	for id := range graph.Factors {
		if walk(id) {
			return
		}
	}
}

func (v *runtimeSpecValidator) validateDecision(spec RuntimeSpec) {
	if spec.Decision.Kind == "" {
		v.add("decision.kind", "decision.kind.required", "decision kind 不能为空")
		return
	}
	if !isSupportedDecisionKind(spec.Decision.Kind) {
		v.add("decision.kind", "decision.kind.unsupported", fmt.Sprintf("decision kind %s 不支持", spec.Decision.Kind))
		return
	}
	if expected, ok := expectedDecisionKindForAlgorithm(v.algorithm); ok && spec.Decision.Kind != expected {
		v.add("decision.kind", "decision.kind.incompatible", fmt.Sprintf("algorithm %s 必须使用 decision kind %s", v.algorithm, expected))
	}
	if spec.Decision.FallbackCode != "" {
		v.validateOutcomeCode("decision.fallback_code", "decision.fallback_code.not_found", spec.Decision.FallbackCode)
	}
	if spec.Decision.LevelRule != nil && spec.Decision.LevelRule.LowMax >= spec.Decision.LevelRule.HighMin {
		v.add("decision.level_rule", "decision.level_rule.invalid", "level_rule low_max 必须小于 high_min")
	}
}

func (v *runtimeSpecValidator) validateOutcomeMapping(mapping OutcomeMappingSpec, decisionKind assessmentmodel.DecisionKind) {
	if mapping.DetailKind == "" {
		v.add("outcome_mapping.detail_kind", "outcome_mapping.detail_kind.required", "outcome mapping detail_kind 不能为空")
	} else if !isSupportedOutcomeDetailKind(mapping.DetailKind) {
		v.add("outcome_mapping.detail_kind", "outcome_mapping.detail_kind.unsupported", fmt.Sprintf("outcome detail kind %s 不支持", mapping.DetailKind))
	}
	adapterKey := mapping.ResolvedDetailAdapterKey(decisionKind)
	if adapterKey == "" {
		v.add("outcome_mapping.detail_adapter_key", "outcome_mapping.detail_adapter.required", "outcome detail adapter 不能为空")
		return
	}
	if !isSupportedDetailAdapter(adapterKey) {
		v.add("outcome_mapping.detail_adapter_key", "outcome_mapping.detail_adapter.unsupported", fmt.Sprintf("outcome detail adapter %s 不支持", adapterKey))
		return
	}
	if !isDetailAdapterCompatible(v.algorithm, adapterKey) {
		v.add("outcome_mapping.detail_adapter_key", "outcome_mapping.detail_adapter.incompatible", fmt.Sprintf("algorithm %s 不兼容 outcome detail adapter %s", v.algorithm, adapterKey))
	}
}

func (v *runtimeSpecValidator) validateSpecialRules(rules []SpecialRuleSpec) {
	for _, rule := range rules {
		switch rule.Phase {
		case "", SpecialRuleBeforeScore, SpecialRuleAfterDecision:
		case SpecialRuleBeforeDecision:
			v.add("special_rules."+rule.Code+".phase", "special_rule.phase.unsupported", fmt.Sprintf("special rule phase %s 暂不支持", rule.Phase))
		default:
			v.add("special_rules."+rule.Code+".phase", "special_rule.phase.unsupported", fmt.Sprintf("special rule phase %s 不支持", rule.Phase))
		}
		switch rule.ResolvedKind() {
		case "", SpecialRuleKindAnswerMatch, SpecialRuleKindFallbackThreshold:
		default:
			v.add("special_rules."+rule.Code+".kind", "special_rule.kind.unsupported", fmt.Sprintf("special rule kind %s 不支持", rule.ResolvedKind()))
		}
		if outcomeCode := firstNonEmpty(rule.OutcomeCode, rule.Code); outcomeCode != "" {
			v.validateOutcomeCode("special_rules."+rule.Code+".outcome_code", "special_rule.outcome.not_found", outcomeCode)
		}
		if rule.ResolvedKind() == SpecialRuleKindAnswerMatch {
			v.validateSpecialRuleQuestionRefs(rule)
		}
	}
}

func (v *runtimeSpecValidator) validateSpecialRuleQuestionRefs(rule SpecialRuleSpec) {
	questionCodes := rule.ResolvedQuestionCodes()
	optionValues := rule.ResolvedOptionValues()
	for _, questionCode := range questionCodes {
		options, ok := v.questions[questionCode]
		if !ok {
			v.add("special_rules."+rule.Code+".condition.question_codes", "question_mapping.question_not_found", fmt.Sprintf("题目 %s 不存在", questionCode))
			continue
		}
		for _, optionValue := range optionValues {
			if _, ok := options[optionValue]; !ok {
				v.add("special_rules."+rule.Code+".condition.option_values", "question_mapping.option_not_found", fmt.Sprintf("题目 %s 的选项 %s 不存在", questionCode, optionValue))
			}
		}
	}
}

func (v *runtimeSpecValidator) validateReport(report ReportSpec, mapping OutcomeMappingSpec, decisionKind assessmentmodel.DecisionKind) {
	if report.Kind == "" {
		v.add("report.kind", "report.kind.required", "report kind 不能为空")
	}
	if report.Kind != "" && !isSupportedReportKind(report.Kind) {
		v.add("report.kind", "report.kind.unsupported", fmt.Sprintf("report kind %s 不支持", report.Kind))
		return
	}
	if report.Kind == ReportKindTemplate && report.AdapterKey == "" {
		v.add("report.adapter_key", "report.adapter.required", "template report adapter_key 不能为空")
		return
	}
	adapterKey := report.ResolvedAdapterKey(mapping, decisionKind)
	if adapterKey == "" {
		return
	}
	if !isSupportedReportAdapter(adapterKey) {
		v.add("report.adapter_key", "report.adapter.unsupported", fmt.Sprintf("report adapter %s 不支持", adapterKey))
		return
	}
	if !isReportAdapterCompatible(v.algorithm, adapterKey) {
		v.add("report.adapter_key", "report.adapter.incompatible", fmt.Sprintf("algorithm %s 不兼容 report adapter %s", v.algorithm, adapterKey))
	}
}

func (v *runtimeSpecValidator) validateOutcomeCode(field, issueCode, outcomeCode string) {
	if len(v.outcomes) == 0 {
		return
	}
	if _, ok := v.outcomes[outcomeCode]; !ok {
		v.add(field, issueCode, fmt.Sprintf("outcome %s 不存在", outcomeCode))
	}
}

func (v *runtimeSpecValidator) add(field, code, message string) {
	v.issues = append(v.issues, assessmentmodel.DomainValidationIssue{
		Field:   field,
		Code:    code,
		Message: message,
		Level:   assessmentmodel.ValidationLevelError,
	})
}

func isSupportedDecisionKind(kind assessmentmodel.DecisionKind) bool {
	switch kind {
	case assessmentmodel.DecisionKindPoleComposition,
		assessmentmodel.DecisionKindNearestPattern,
		assessmentmodel.DecisionKindTraitProfile:
		return true
	default:
		return false
	}
}

func expectedDecisionKindForAlgorithm(algorithm assessmentmodel.Algorithm) (assessmentmodel.DecisionKind, bool) {
	switch algorithm {
	case assessmentmodel.AlgorithmMBTI:
		return assessmentmodel.DecisionKindPoleComposition, true
	case assessmentmodel.AlgorithmSBTI:
		return assessmentmodel.DecisionKindNearestPattern, true
	case assessmentmodel.AlgorithmBigFive:
		return assessmentmodel.DecisionKindTraitProfile, true
	default:
		return "", false
	}
}

func isSupportedOutcomeDetailKind(kind OutcomeDetailKind) bool {
	switch kind {
	case OutcomeDetailPersonalityType, OutcomeDetailTraitProfile:
		return true
	default:
		return false
	}
}

func isSupportedDetailAdapter(adapter DetailAdapterKey) bool {
	switch adapter {
	case DetailAdapterPersonalityType,
		DetailAdapterTraitProfile,
		DetailAdapterMBTI,
		DetailAdapterSBTI,
		DetailAdapterBigFive:
		return true
	default:
		return false
	}
}

func isSupportedReportKind(kind ReportKind) bool {
	switch kind {
	case ReportKindPersonalityType, ReportKindTraitProfile, ReportKindTemplate:
		return true
	default:
		return false
	}
}

func isSupportedReportAdapter(adapter ReportAdapterKey) bool {
	switch adapter {
	case ReportAdapterPersonalityType,
		ReportAdapterTraitProfile,
		ReportAdapterMBTI,
		ReportAdapterSBTI,
		ReportAdapterBigFive:
		return true
	default:
		return false
	}
}

func isDetailAdapterCompatible(algorithm assessmentmodel.Algorithm, adapter DetailAdapterKey) bool {
	if algorithm == "" || algorithm == assessmentmodel.AlgorithmPersonalityTypology {
		return true
	}
	switch algorithm {
	case assessmentmodel.AlgorithmMBTI:
		return adapter == DetailAdapterMBTI || adapter == DetailAdapterPersonalityType
	case assessmentmodel.AlgorithmSBTI:
		return adapter == DetailAdapterSBTI || adapter == DetailAdapterPersonalityType
	case assessmentmodel.AlgorithmBigFive:
		return adapter == DetailAdapterBigFive || adapter == DetailAdapterTraitProfile
	default:
		return true
	}
}

func isReportAdapterCompatible(algorithm assessmentmodel.Algorithm, adapter ReportAdapterKey) bool {
	if algorithm == "" || algorithm == assessmentmodel.AlgorithmPersonalityTypology {
		return true
	}
	switch algorithm {
	case assessmentmodel.AlgorithmMBTI:
		return adapter == ReportAdapterMBTI || adapter == ReportAdapterPersonalityType
	case assessmentmodel.AlgorithmSBTI:
		return adapter == ReportAdapterSBTI || adapter == ReportAdapterPersonalityType
	case assessmentmodel.AlgorithmBigFive:
		return adapter == ReportAdapterBigFive || adapter == ReportAdapterTraitProfile
	default:
		return true
	}
}
