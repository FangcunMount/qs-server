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
	validator := runtimeSpecValidator{
		questions: map[string]map[string]struct{}{},
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
	validator.validate(spec)
	return validator.issues
}

type runtimeSpecValidator struct {
	questions map[string]map[string]struct{}
	issues    []assessmentmodel.DomainValidationIssue
}

func (v *runtimeSpecValidator) validate(spec *RuntimeSpec) {
	if spec == nil {
		v.add("definition.payload", "definition.payload.required", "runtime spec is required")
		return
	}
	v.validateFactorGraph(spec.FactorGraph)
	v.validateDecision(*spec)
	v.validateReport(spec.Report)
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
	}
}

func (v *runtimeSpecValidator) validateReport(report ReportSpec) {
	if report.Kind == "" {
		v.add("report.kind", "report.kind.required", "report kind 不能为空")
	}
	if report.Kind == ReportKindTemplate && report.AdapterKey == "" {
		v.add("report.adapter_key", "report.adapter.required", "template report adapter_key 不能为空")
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
