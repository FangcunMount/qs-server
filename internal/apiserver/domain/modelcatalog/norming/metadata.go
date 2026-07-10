package norming

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
)

// CompositeIndexSpec declares 如何复合 index 推导自 子节点 因子。
type CompositeIndexSpec struct {
	Code       string
	Strategy   factor.ChildrenAggregationStrategy
	Children   []string
	ParentCode string
}

// ApplyCompositeMetadata projects composite index metadata into the target
// measure layer.
func ApplyCompositeMetadata(measure definition.MeasureSpec, specs []CompositeIndexSpec) definition.MeasureSpec {
	if len(measure.Factors) == 0 || len(specs) == 0 {
		return measure
	}
	out := cloneMeasureSpec(measure)
	indexPos := make(map[string]int, len(out.Factors))
	for i, item := range out.Factors {
		indexPos[item.Code] = i
	}
	out.FactorGraph.Edges = filterCompositeEdges(out.FactorGraph.Edges, specs)
	out.Scoring = filterCompositeScoring(out.Scoring, specs)
	for _, spec := range specs {
		if _, ok := indexPos[spec.Code]; !ok || len(spec.Children) == 0 {
			continue
		}
		strategy := spec.Strategy
		if strategy == "" {
			strategy = factor.ChildrenAggregationSum
		}
		sources := make([]factor.ScoringSource, 0, len(spec.Children))
		for _, childCode := range spec.Children {
			sources = append(sources, factor.ScoringSource{Kind: factor.ScoringSourceFactor, Code: childCode})
			out.FactorGraph.Edges = appendEdge(out.FactorGraph.Edges, factor.FactorEdge{ParentCode: spec.Code, ChildCode: childCode})
		}
		if spec.ParentCode != "" {
			out.FactorGraph.Edges = appendEdge(out.FactorGraph.Edges, factor.FactorEdge{ParentCode: spec.ParentCode, ChildCode: spec.Code})
		}
		out.Scoring = append(out.Scoring, factor.Scoring{
			FactorCode: spec.Code,
			Sources:    sources,
			Strategy:   factor.ScoringStrategy(strategy),
		})
	}
	out.FactorGraph.Roots = deriveRoots(out.Factors, out.FactorGraph.Edges)
	return out
}

// MeasureSpecWithCompositeMetadata projects composite metadata into the target measure layer.
func MeasureSpecWithCompositeMetadata(factors []factor.Factor, specs []CompositeIndexSpec) definition.MeasureSpec {
	return ApplyCompositeMetadata(definition.MeasureSpec{Factors: factors}, specs)
}

// MetadataContext 携带常模ing 元数据 不使用 embedding 常模表 bodies。
type MetadataContext struct {
	NormTableVersion string
	IndexCodes       []string
	ValidityCodes    []string
	NormFactorCodes  []string
}

// ApplyNormMetadata projects role and norm metadata into the target definition
// layers.
func ApplyNormMetadata(measure definition.MeasureSpec, ctx MetadataContext) (definition.MeasureSpec, definition.Calibration) {
	if len(measure.Factors) == 0 {
		return measure, definition.Calibration{}
	}
	indexCodes := stringSet(ctx.IndexCodes)
	validityCodes := stringSet(ctx.ValidityCodes)
	out := cloneMeasureSpec(measure)
	for i, item := range out.Factors {
		switch {
		case indexCodes[item.Code]:
			out.Factors[i].Role = factor.FactorRoleIndex
		case validityCodes[item.Code]:
			out.Factors[i].Role = factor.FactorRoleValidity
		}
	}
	return out, definition.Calibration{NormRefs: NormRefsFromMetadata(ctx)}
}

// NormRefsFromMetadata projects norming metadata into the target calibration layer.
func NormRefsFromMetadata(ctx MetadataContext) []norm.Ref {
	if ctx.NormTableVersion == "" || len(ctx.NormFactorCodes) == 0 {
		return nil
	}
	refs := make([]norm.Ref, 0, len(ctx.NormFactorCodes))
	seen := make(map[string]struct{}, len(ctx.NormFactorCodes))
	for _, code := range ctx.NormFactorCodes {
		if code == "" {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		refs = append(refs, norm.Ref{FactorCode: code, NormTableVersion: ctx.NormTableVersion})
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
	out := definition.MeasureSpec{
		Factors: append([]factor.Factor(nil), measure.Factors...),
		FactorGraph: factor.FactorGraph{
			Roots:      append([]string(nil), measure.FactorGraph.Roots...),
			Edges:      append([]factor.FactorEdge(nil), measure.FactorGraph.Edges...),
			SortOrders: cloneSortOrders(measure.FactorGraph.SortOrders),
		},
		Scoring: cloneScoring(measure.Scoring),
	}
	return out
}

func filterCompositeEdges(edges []factor.FactorEdge, specs []CompositeIndexSpec) []factor.FactorEdge {
	if len(edges) == 0 || len(specs) == 0 {
		return edges
	}
	specCodes := make(map[string]bool, len(specs))
	for _, spec := range specs {
		specCodes[spec.Code] = true
	}
	out := edges[:0]
	for _, edge := range edges {
		if specCodes[edge.ParentCode] || specCodes[edge.ChildCode] {
			continue
		}
		out = append(out, edge)
	}
	return out
}

func filterCompositeScoring(scoring []factor.Scoring, specs []CompositeIndexSpec) []factor.Scoring {
	if len(scoring) == 0 || len(specs) == 0 {
		return scoring
	}
	specCodes := make(map[string]bool, len(specs))
	for _, spec := range specs {
		specCodes[spec.Code] = true
	}
	out := scoring[:0]
	for _, rule := range scoring {
		if specCodes[rule.FactorCode] {
			continue
		}
		out = append(out, rule)
	}
	return out
}

func appendEdge(edges []factor.FactorEdge, edge factor.FactorEdge) []factor.FactorEdge {
	for _, existing := range edges {
		if existing == edge {
			return edges
		}
	}
	return append(edges, edge)
}

func deriveRoots(factors []factor.Factor, edges []factor.FactorEdge) []string {
	hasParent := make(map[string]bool, len(edges))
	for _, edge := range edges {
		hasParent[edge.ChildCode] = true
	}
	roots := make([]string, 0, len(factors))
	for _, item := range factors {
		if !hasParent[item.Code] {
			roots = append(roots, item.Code)
		}
	}
	if len(roots) == 0 {
		return nil
	}
	return roots
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
