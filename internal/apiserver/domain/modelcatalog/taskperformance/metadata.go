package taskperformance

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
)

// MetadataContext 携带task_performance 元数据 不使用 embedding 常模表 bodies。
// 执行层 计分 (answer 键, ability 等级) 等待 second task_performance model。
type MetadataContext struct {
	NormTableVersion string
	ItemSetCodes     []string
}

// ApplyNormMetadata projects task-set role and norm metadata into the target
// definition layers.
func ApplyNormMetadata(measure definition.MeasureSpec, ctx MetadataContext) (definition.MeasureSpec, definition.Calibration) {
	if len(measure.Factors) == 0 {
		return measure, definition.Calibration{}
	}
	itemSetCodes := stringSet(ctx.ItemSetCodes)
	out := cloneMeasureSpec(measure)
	for i, item := range out.Factors {
		if itemSetCodes[item.Code] {
			out.Factors[i].Role = factor.FactorRoleTaskSet
		}
	}
	return out, definition.Calibration{NormRefs: NormRefsFromMetadata(out.Factors, ctx)}
}

// NormRefsFromMetadata projects cognitive metadata into the target calibration layer.
func NormRefsFromMetadata(factors []factor.Factor, ctx MetadataContext) []norm.Ref {
	if ctx.NormTableVersion == "" || len(factors) == 0 {
		return nil
	}
	itemSetCodes := stringSet(ctx.ItemSetCodes)
	refs := make([]norm.Ref, 0, len(factors))
	for _, item := range factors {
		if item.ResolvedRole() == factor.FactorRoleTotal || itemSetCodes[item.Code] {
			refs = append(refs, norm.Ref{FactorCode: item.Code, NormTableVersion: ctx.NormTableVersion})
		}
	}
	return refs
}

func stringSet(values []string) map[string]bool {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]bool, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		set[value] = true
	}
	return set
}

func cloneMeasureSpec(measure definition.MeasureSpec) definition.MeasureSpec {
	return definition.MeasureSpec{
		Factors: append([]factor.Factor(nil), measure.Factors...),
		FactorGraph: factor.FactorGraph{
			Roots:      append([]string(nil), measure.FactorGraph.Roots...),
			Edges:      append([]factor.FactorEdge(nil), measure.FactorGraph.Edges...),
			SortOrders: cloneSortOrders(measure.FactorGraph.SortOrders),
		},
		Scoring: cloneScoring(measure.Scoring),
	}
}

func cloneSortOrders(items map[string]int) map[string]int {
	if items == nil {
		return nil
	}
	out := make(map[string]int, len(items))
	for key, value := range items {
		out[key] = value
	}
	return out
}

func cloneScoring(scoring []factor.Scoring) []factor.Scoring {
	if scoring == nil {
		return nil
	}
	out := make([]factor.Scoring, 0, len(scoring))
	for _, rule := range scoring {
		copied := rule
		copied.Sources = cloneSources(rule.Sources)
		if rule.Params != nil {
			copied.Params = &factor.ScoringParams{
				CntOptionContents: append([]string(nil), rule.Params.CntOptionContents...),
			}
		}
		if rule.MaxScore != nil {
			maxScore := *rule.MaxScore
			copied.MaxScore = &maxScore
		}
		copied.Weights = cloneWeights(rule.Weights)
		out = append(out, copied)
	}
	return out
}

func cloneSources(sources []factor.ScoringSource) []factor.ScoringSource {
	if sources == nil {
		return nil
	}
	out := make([]factor.ScoringSource, 0, len(sources))
	for _, source := range sources {
		copied := source
		copied.OptionScores = cloneWeights(source.OptionScores)
		out = append(out, copied)
	}
	return out
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
