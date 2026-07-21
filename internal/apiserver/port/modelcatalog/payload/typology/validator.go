package typology

import (
	"fmt"
	"math"
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
)

// QuestionnaireSnapshot 是minimal 问卷 结构 needed 到 有效ate 运行时规格。
type QuestionnaireSnapshot struct {
	Code      string
	Version   string
	Questions []QuestionSnapshot
}

// QuestionSnapshot 是minimal question 结构 needed 到 有效ate references。
type QuestionSnapshot struct {
	Code        string
	Type        string
	OptionCodes []string
}

// ValidateRuntimeSpecForPublish performs strong 校验 gate 用于之前 发布。
func ValidateRuntimeSpecForPublish(spec *RuntimeSpec, questionnaire QuestionnaireSnapshot) []binding.DomainValidationIssue {
	return ValidateRuntimeSpecForPublishWithContext(spec, questionnaire, RuntimeSpecValidationContext{})
}

// RuntimeSpecValidationContext 携带载荷-等级 元数据 needed 按 publish 校验。
type RuntimeSpecValidationContext struct {
	Algorithm          binding.Algorithm
	Outcomes           []Outcome
	PublishedTemplates PublishedTemplateLookup
}

// PublishedTemplateLookup validates frozen TemplateID+TemplateVersion references.
// ModelCatalog publish UI/API wiring is a follow-up item (IR-R013).
type PublishedTemplateLookup interface {
	IsPublished(templateID string, version string) bool
}

// ValidateRuntimeSpecForPublishWithContext performs strong 校验 gate 用于之前 发布。
func ValidateRuntimeSpecForPublishWithContext(spec *RuntimeSpec, questionnaire QuestionnaireSnapshot, validationContext RuntimeSpecValidationContext) []binding.DomainValidationIssue {
	validator := runtimeSpecValidator{
		questions:          map[string]map[string]struct{}{},
		questionTypes:      map[string]string{},
		algorithm:          validationContext.Algorithm,
		outcomes:           map[string]Outcome{},
		publishedTemplates: validationContext.PublishedTemplates,
	}
	for _, question := range questionnaire.Questions {
		options := make(map[string]struct{}, len(question.OptionCodes))
		for _, optionCode := range question.OptionCodes {
			options[optionCode] = struct{}{}
		}
		if question.Code != "" {
			validator.questions[question.Code] = options
			validator.questionTypes[question.Code] = question.Type
		}
	}
	validator.loadOutcomes(validationContext.Outcomes)
	validator.validate(spec)
	return validator.issues
}

type runtimeSpecValidator struct {
	questions          map[string]map[string]struct{}
	questionTypes      map[string]string
	algorithm          binding.Algorithm
	outcomes           map[string]Outcome
	publishedTemplates PublishedTemplateLookup
	issues             []binding.DomainValidationIssue
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
		if v.algorithm == binding.AlgorithmMBTI && strings.TrimSpace(outcome.ImageURL) == "" {
			v.add("outcomes."+outcome.Code+".image_url", "outcome.image_url.required", fmt.Sprintf("MBTI 结果 %s 必须配置人物图片", outcome.Code))
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
		seen := make(map[string]struct{}, len(factor.Contributions))
		for _, contribution := range factor.Contributions {
			if _, duplicate := seen[contribution.QuestionCode]; duplicate && contribution.QuestionCode != "" {
				v.add("factor_graph.factors."+key+".contributions", "question_contribution.duplicate", fmt.Sprintf("题目 %s 对 factor %s 的贡献重复", contribution.QuestionCode, key))
			}
			seen[contribution.QuestionCode] = struct{}{}
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
	field := "factor_graph.factors." + factorKey + ".contributions"
	if contribution.ScoringMode == "" {
		v.addWarning(field+".scoring_mode", "question_contribution.legacy_implicit", fmt.Sprintf("题目 %s 使用旧版隐式计分规则", contribution.QuestionCode))
		for optionCode := range contribution.OptionScores {
			if _, exists := options[optionCode]; !exists {
				v.add(field+".option_scores", "question_mapping.option_not_found", fmt.Sprintf("题目 %s 的选项 %s 不存在", contribution.QuestionCode, optionCode))
			}
		}
		return
	}
	if contribution.ScoringMode != QuestionScoringModeQuestionScore && contribution.ScoringMode != QuestionScoringModeOptionOverride {
		v.add(field+".scoring_mode", "scoring_mode.invalid", fmt.Sprintf("scoring_mode %s 不支持", contribution.ScoringMode))
	}
	if contribution.Sign != 1 && contribution.Sign != -1 {
		v.add(field+".sign", "sign.invalid", "sign 必须是 1 或 -1")
	}
	if math.IsNaN(contribution.Weight) || math.IsInf(contribution.Weight, 0) || contribution.Weight <= 0 {
		v.add(field+".weight", "weight.invalid", "weight 必须是大于 0 的有限数字")
	}
	if contribution.ScoringMode == QuestionScoringModeQuestionScore {
		if contribution.OptionScores != nil {
			v.add(field+".option_scores", "option_scores.forbidden", "question_score 不能配置 option_scores")
		}
		return
	}
	if contribution.ScoringMode != QuestionScoringModeOptionOverride {
		return
	}
	if v.questionTypes[contribution.QuestionCode] != "Radio" {
		v.add(field+".scoring_mode", "scoring_mode.invalid", "option_override 仅支持单选题")
	}
	if len(contribution.OptionScores) == 0 {
		v.add(field+".option_scores", "option_scores.required", "option_override 必须配置 option_scores")
		return
	}
	for optionCode := range options {
		if _, exists := contribution.OptionScores[optionCode]; !exists {
			v.add(field+".option_scores", "option_scores.missing_option", fmt.Sprintf("题目 %s 缺少选项 %s 的覆盖分值", contribution.QuestionCode, optionCode))
		}
	}
	for optionCode, score := range contribution.OptionScores {
		if _, exists := options[optionCode]; !exists {
			v.add(field+".option_scores", "option_scores.unknown_option", fmt.Sprintf("题目 %s 的选项 %s 不存在", contribution.QuestionCode, optionCode))
		}
		if math.IsNaN(score) || math.IsInf(score, 0) {
			v.add(field+".option_scores", "option_scores.invalid", fmt.Sprintf("题目 %s 的选项 %s 分值必须是有限数字", contribution.QuestionCode, optionCode))
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
	if spec.Decision.FallbackCode != "" {
		v.validateOutcomeCode("decision.fallback_code", "decision.fallback_code.not_found", spec.Decision.FallbackCode)
	}
	if spec.Decision.LevelRule != nil && spec.Decision.LevelRule.LowMax >= spec.Decision.LevelRule.HighMin {
		v.add("decision.level_rule", "decision.level_rule.invalid", "level_rule low_max 必须小于 high_min")
	}
	switch spec.Decision.Kind {
	case binding.DecisionKindPoleComposition:
		for _, factorCode := range spec.FactorGraph.DecisionFactorOrder() {
			meta, ok := dimensionForValidation(spec.FactorGraph, factorCode)
			if !ok || meta.LeftPole == "" || meta.RightPole == "" {
				v.add("decision.poles."+factorCode, "decision.poles.required", fmt.Sprintf("factor %s 必须配置左右极", factorCode))
			}
		}
	case binding.DecisionKindNearestPattern:
		hasPattern := false
		for _, outcome := range v.outcomes {
			hasPattern = hasPattern || (!outcome.IsSpecial && outcome.Pattern != "")
		}
		if !hasPattern {
			v.add("outcomes.pattern", "decision.patterns.required", "nearest_pattern 至少需要一个普通结果配置 pattern")
		}
	case binding.DecisionKindDominantFactor:
		topK := spec.Decision.TopK
		if topK <= 0 {
			topK = 1
		}
		factorOrder := spec.FactorGraph.DecisionFactorOrder()
		if topK > len(factorOrder) {
			v.add("decision.top_k", "decision.top_k.invalid", fmt.Sprintf("top_k %d 不能超过决策因子数 %d", topK, len(factorOrder)))
		}
		for _, factorCode := range factorOrder {
			if _, ok := v.outcomes[factorCode]; !ok {
				v.add("outcomes."+factorCode, "decision.dominant_factor.outcome.required", fmt.Sprintf("dominant factor %s 必须有同 code 的结果", factorCode))
			}
		}
	}
}

func dimensionForValidation(graph FactorGraphSpec, factorCode string) (Dimension, bool) {
	if meta, ok := graph.Dimensions[factorCode]; ok {
		return meta, true
	}
	if factor, ok := graph.Factors[factorCode]; ok {
		if meta, ok := graph.Dimensions[factor.Code]; ok {
			return meta, true
		}
	}
	return Dimension{}, false
}

func (v *runtimeSpecValidator) validateOutcomeMapping(mapping OutcomeMappingSpec, decisionKind binding.DecisionKind) {
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
	if mapping.DetailAdapterKey != "" && isLegacyDetailAdapter(mapping.DetailAdapterKey) {
		v.add("outcome_mapping.detail_adapter_key", "outcome_mapping.detail_adapter.deprecated",
			fmt.Sprintf("outcome detail adapter %s 已废弃，请使用 personality_type 或 trait_profile", mapping.DetailAdapterKey))
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

func (v *runtimeSpecValidator) validateReport(report ReportSpec, mapping OutcomeMappingSpec, decisionKind binding.DecisionKind) {
	if report.Kind == "" {
		v.add("report.kind", "report.kind.required", "report kind 不能为空")
	}
	if report.TemplateID != "" && !IsRegisteredReportTemplateID(report.TemplateID) {
		v.add("report.template_id", "report.template_id.unknown", fmt.Sprintf("report template_id %s 未注册", report.TemplateID))
	}
	if report.TemplateVersion != "" {
		if report.TemplateID == "" {
			v.add("report.template_id", "report.template_id.required", "template_version 需要同时声明 template_id")
		} else if v.publishedTemplates != nil && !v.publishedTemplates.IsPublished(report.TemplateID, report.TemplateVersion) {
			v.add("report.template_version", "report.template_version.unpublished", fmt.Sprintf("report template %s@%s 未发布", report.TemplateID, report.TemplateVersion))
		}
	}
	if report.Kind != "" && !isSupportedReportKind(report.Kind) {
		v.add("report.kind", "report.kind.unsupported", fmt.Sprintf("report kind %s 不支持", report.Kind))
		return
	}
	if report.Kind == ReportKindTemplate && report.AdapterKey == "" {
		v.add("report.adapter_key", "report.adapter.required", "template report adapter_key 不能为空")
		return
	}
	if report.AdapterKey != "" && isLegacyReportAdapter(report.AdapterKey) {
		v.add("report.adapter_key", "report.adapter.deprecated",
			fmt.Sprintf("report adapter %s 已废弃，请使用 personality_type 或 trait_profile", report.AdapterKey))
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
	v.issues = append(v.issues, binding.DomainValidationIssue{
		Field:   field,
		Code:    code,
		Message: message,
		Level:   binding.ValidationLevelError,
	})
}

func (v *runtimeSpecValidator) addWarning(field, code, message string) {
	v.issues = append(v.issues, binding.DomainValidationIssue{
		Field: field, Code: code, Message: message, Level: binding.ValidationLevelWarning,
	})
}

func isSupportedDecisionKind(kind binding.DecisionKind) bool {
	switch kind {
	case binding.DecisionKindPoleComposition,
		binding.DecisionKindNearestPattern,
		binding.DecisionKindTraitProfile,
		binding.DecisionKindDominantFactor:
		return true
	default:
		return false
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

func isLegacyDetailAdapter(adapter DetailAdapterKey) bool {
	switch adapter {
	case DetailAdapterMBTI, DetailAdapterSBTI, DetailAdapterBigFive:
		return true
	default:
		return false
	}
}

func isLegacyReportAdapter(adapter ReportAdapterKey) bool {
	switch adapter {
	case ReportAdapterMBTI, ReportAdapterSBTI, ReportAdapterBigFive:
		return true
	default:
		return false
	}
}

func isDetailAdapterCompatible(algorithm binding.Algorithm, adapter DetailAdapterKey) bool {
	if algorithm == "" || algorithm == binding.AlgorithmPersonalityTypology {
		return true
	}
	switch algorithm {
	case binding.AlgorithmMBTI, binding.AlgorithmSBTI:
		return adapter == DetailAdapterPersonalityType
	case binding.AlgorithmBigFive:
		return adapter == DetailAdapterTraitProfile
	default:
		return true
	}
}

func isReportAdapterCompatible(algorithm binding.Algorithm, adapter ReportAdapterKey) bool {
	if algorithm == "" || algorithm == binding.AlgorithmPersonalityTypology {
		return true
	}
	switch algorithm {
	case binding.AlgorithmMBTI, binding.AlgorithmSBTI:
		return adapter == ReportAdapterPersonalityType
	case binding.AlgorithmBigFive:
		return adapter == ReportAdapterTraitProfile
	default:
		return true
	}
}
