package shared

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

func MeasureSpecFromDefinitionBody(body DefinitionBody) definition.MeasureSpec {
	return definition.MeasureSpec{
		Factors:     FactorsFromDefinitionDimensions(body.Dimensions),
		FactorGraph: FactorGraphFromDefinitionDimensions(body.Dimensions),
		Scoring:     ScoringFromDefinitionDimensions(body.Dimensions),
	}
}

func FactorsFromDefinitionDimensions(dimensions []DimensionRule) []factor.Factor {
	if dimensions == nil {
		return nil
	}
	out := make([]factor.Factor, 0, len(dimensions))
	for _, item := range dimensions {
		role := factor.FactorRole(item.Role)
		if role != "" && !role.IsValid() {
			role = ""
		}
		if role == "" {
			if item.IsTotalScore {
				role = factor.FactorRoleTotal
			} else {
				role = factor.FactorRoleDimension
			}
		}
		out = append(out, factor.Factor{Code: item.Code, Title: item.Title, Role: role})
	}
	return out
}

func FactorGraphFromDefinitionDimensions(dimensions []DimensionRule) factor.FactorGraph {
	if len(dimensions) == 0 {
		return factor.FactorGraph{}
	}
	graph := factor.FactorGraph{
		Roots:      make([]string, 0, len(dimensions)),
		Edges:      make([]factor.FactorEdge, 0, len(dimensions)),
		SortOrders: make(map[string]int),
	}
	seenEdges := make(map[factor.FactorEdge]struct{})
	hasParent := make(map[string]bool, len(dimensions))
	for _, item := range dimensions {
		if item.SortOrder != 0 {
			graph.SortOrders[item.Code] = item.SortOrder
		}
		if item.ParentCode != "" {
			edge := factor.FactorEdge{ParentCode: item.ParentCode, ChildCode: item.Code}
			if _, ok := seenEdges[edge]; !ok {
				graph.Edges = append(graph.Edges, edge)
				seenEdges[edge] = struct{}{}
			}
			hasParent[item.Code] = true
		}
		if item.ChildrenPolicy == nil {
			continue
		}
		for _, childCode := range item.ChildrenPolicy.Children {
			edge := factor.FactorEdge{ParentCode: item.Code, ChildCode: childCode}
			if _, ok := seenEdges[edge]; ok {
				continue
			}
			graph.Edges = append(graph.Edges, edge)
			seenEdges[edge] = struct{}{}
			hasParent[childCode] = true
		}
	}
	for _, item := range dimensions {
		if !hasParent[item.Code] {
			graph.Roots = append(graph.Roots, item.Code)
		}
	}
	if len(graph.SortOrders) == 0 {
		graph.SortOrders = nil
	}
	return graph
}

func ScoringFromDefinitionDimensions(dimensions []DimensionRule) []factor.Scoring {
	if len(dimensions) == 0 {
		return nil
	}
	out := make([]factor.Scoring, 0, len(dimensions))
	for _, item := range dimensions {
		switch {
		case item.ChildrenPolicy != nil && len(item.ChildrenPolicy.Children) > 0:
			sources := make([]factor.ScoringSource, 0, len(item.ChildrenPolicy.Children))
			for _, childCode := range item.ChildrenPolicy.Children {
				sources = append(sources, factor.ScoringSource{Kind: factor.ScoringSourceFactor, Code: childCode})
			}
			out = append(out, factor.Scoring{
				FactorCode: item.Code,
				Sources:    sources,
				Strategy:   factor.ScoringStrategy(item.ChildrenPolicy.Strategy),
				Params:     scoringParamsFromPayload(item.ScoringParams),
				MaxScore:   cloneFloat64(item.MaxScore),
				Weights:    cloneWeights(item.ChildrenPolicy.Weights),
			})
		case len(item.QuestionCodes) > 0 || item.ScoringStrategy != "" || item.ScoringParams != nil || item.MaxScore != nil:
			sources := make([]factor.ScoringSource, 0, len(item.QuestionCodes))
			for _, questionCode := range item.QuestionCodes {
				sources = append(sources, factor.ScoringSource{Kind: factor.ScoringSourceQuestion, Code: questionCode})
			}
			out = append(out, factor.Scoring{
				FactorCode: item.Code,
				Sources:    sources,
				Strategy:   factor.ScoringStrategy(item.ScoringStrategy),
				Params:     scoringParamsFromPayload(item.ScoringParams),
				MaxScore:   cloneFloat64(item.MaxScore),
			})
		}
	}
	return out
}

func ValidateDefinitionBodyForPublish(body DefinitionBody) []factor.HierarchyIssue {
	if len(body.Dimensions) == 0 {
		return []factor.HierarchyIssue{{
			Field: "dimensions", Code: "dimensions.required", Message: "dimensions 不能为空",
		}}
	}
	measure := MeasureSpecFromDefinitionBody(body)
	issues := factor.ValidateMeasureSpecParts(measure.Factors, measure.FactorGraph, measure.Scoring)
	return append(issues, validateInterpretRuleRefs(body.InterpretRules, body.Dimensions)...)
}

func ValidateDefinitionBodyJSONForPublish(payload []byte) ([]factor.HierarchyIssue, error) {
	body, err := ParseDefinitionBodyJSON(payload)
	if err != nil {
		return nil, fmt.Errorf("decode factor definition body: %w", err)
	}
	return ValidateDefinitionBodyForPublish(body), nil
}

func validateInterpretRuleRefs(rules []InterpretRule, dimensions []DimensionRule) []factor.HierarchyIssue {
	if len(rules) == 0 {
		return nil
	}
	byCode := make(map[string]struct{}, len(dimensions))
	for _, dimension := range dimensions {
		byCode[dimension.Code] = struct{}{}
	}
	issues := make([]factor.HierarchyIssue, 0, len(rules))
	for _, rule := range rules {
		field := "interpret_rules"
		if rule.DimensionCode != "" {
			field = fmt.Sprintf("interpret_rules[%s]", rule.DimensionCode)
		}
		if rule.DimensionCode == "" {
			issues = append(issues, factor.HierarchyIssue{
				Field: field + ".dimension_code", Code: "interpret_rules.dimension_code.required", Message: "interpret_rules.dimension_code 不能为空",
			})
			continue
		}
		if _, ok := byCode[rule.DimensionCode]; !ok {
			issues = append(issues, factor.HierarchyIssue{
				Field: field + ".dimension_code", Code: "interpret_rules.dimension_code.not_found",
				Message: fmt.Sprintf("interpret_rules 引用了不存在的维度 %s", rule.DimensionCode),
			})
		}
	}
	return issues
}

func scoringParamsFromPayload(payload *ScoringParamsPayload) *factor.ScoringParams {
	if payload == nil || len(payload.CntOptionContents) == 0 {
		return nil
	}
	return &factor.ScoringParams{CntOptionContents: append([]string(nil), payload.CntOptionContents...)}
}

func cloneFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	out := *value
	return &out
}

func cloneWeights(weights map[string]float64) map[string]float64 {
	if weights == nil {
		return nil
	}
	out := make(map[string]float64, len(weights))
	for key, value := range weights {
		out[key] = value
	}
	return out
}
