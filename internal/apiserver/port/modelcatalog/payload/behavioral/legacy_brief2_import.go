// Legacy BRIEF-2 payload semantics are isolated here for legacy import and
// decode adapters. Publishing and V2 runtime paths consume DefinitionV2.
package behavioral

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
)

// brief2CompositeIndexSpec describes a BRIEF-2 composite index in its wire payload.
type brief2CompositeIndexSpec struct {
	Code       string
	Strategy   factor.ChildrenAggregationStrategy
	Children   []string
	ParentCode string
}

// brief2MetadataContext carries BRIEF-2 metadata without embedding norm tables.
type brief2MetadataContext struct {
	NormTableVersion string
	IndexCodes       []string
	ValidityCodes    []string
	NormFactorCodes  []string
}

func applyBrief2CompositeMetadata(measure definition.MeasureSpec, specs []brief2CompositeIndexSpec) definition.MeasureSpec {
	if len(measure.Factors) == 0 || len(specs) == 0 {
		return measure
	}
	out := cloneBrief2MeasureSpec(measure)
	indexPos := make(map[string]int, len(out.Factors))
	for i, item := range out.Factors {
		indexPos[item.Code] = i
	}
	out.FactorGraph.Edges = filterBrief2CompositeEdges(out.FactorGraph.Edges, specs)
	out.Scoring = filterBrief2CompositeScoring(out.Scoring, specs)
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
			out.FactorGraph.Edges = appendBrief2Edge(out.FactorGraph.Edges, factor.FactorEdge{ParentCode: spec.Code, ChildCode: childCode})
		}
		if spec.ParentCode != "" {
			out.FactorGraph.Edges = appendBrief2Edge(out.FactorGraph.Edges, factor.FactorEdge{ParentCode: spec.ParentCode, ChildCode: spec.Code})
		}
		out.Scoring = append(out.Scoring, factor.Scoring{
			FactorCode: spec.Code,
			Sources:    sources,
			Strategy:   factor.ScoringStrategy(strategy),
		})
	}
	out.FactorGraph.Roots = brief2Roots(out.Factors, out.FactorGraph.Edges)
	return out
}

func applyBrief2NormMetadata(measure definition.MeasureSpec, ctx brief2MetadataContext) (definition.MeasureSpec, definition.Calibration) {
	if len(measure.Factors) == 0 {
		return measure, definition.Calibration{}
	}
	indexCodes := stringSet(ctx.IndexCodes)
	validityCodes := stringSet(ctx.ValidityCodes)
	out := cloneBrief2MeasureSpec(measure)
	for i, item := range out.Factors {
		switch {
		case indexCodes[item.Code]:
			out.Factors[i].Role = factor.FactorRoleIndex
		case validityCodes[item.Code]:
			out.Factors[i].Role = factor.FactorRoleValidity
		}
	}
	return out, definition.Calibration{NormRefs: brief2NormRefsFromMetadata(ctx)}
}

func brief2NormRefsFromMetadata(ctx brief2MetadataContext) []norm.Ref {
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

func cloneBrief2MeasureSpec(measure definition.MeasureSpec) definition.MeasureSpec {
	return definition.MeasureSpec{
		Factors: append([]factor.Factor(nil), measure.Factors...),
		FactorGraph: factor.FactorGraph{
			Roots:      append([]string(nil), measure.FactorGraph.Roots...),
			Edges:      append([]factor.FactorEdge(nil), measure.FactorGraph.Edges...),
			SortOrders: cloneBrief2SortOrders(measure.FactorGraph.SortOrders),
		},
		Scoring: cloneBrief2Scoring(measure.Scoring),
	}
}

func filterBrief2CompositeEdges(edges []factor.FactorEdge, specs []brief2CompositeIndexSpec) []factor.FactorEdge {
	if len(edges) == 0 || len(specs) == 0 {
		return edges
	}
	specCodes := brief2CompositeCodes(specs)
	out := edges[:0]
	for _, edge := range edges {
		if specCodes[edge.ParentCode] || specCodes[edge.ChildCode] {
			continue
		}
		out = append(out, edge)
	}
	return out
}

func filterBrief2CompositeScoring(scoring []factor.Scoring, specs []brief2CompositeIndexSpec) []factor.Scoring {
	if len(scoring) == 0 || len(specs) == 0 {
		return scoring
	}
	specCodes := brief2CompositeCodes(specs)
	out := scoring[:0]
	for _, rule := range scoring {
		if specCodes[rule.FactorCode] {
			continue
		}
		out = append(out, rule)
	}
	return out
}

func brief2CompositeCodes(specs []brief2CompositeIndexSpec) map[string]bool {
	result := make(map[string]bool, len(specs))
	for _, spec := range specs {
		result[spec.Code] = true
	}
	return result
}

func appendBrief2Edge(edges []factor.FactorEdge, edge factor.FactorEdge) []factor.FactorEdge {
	for _, existing := range edges {
		if existing == edge {
			return edges
		}
	}
	return append(edges, edge)
}

func brief2Roots(factors []factor.Factor, edges []factor.FactorEdge) []string {
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

func cloneBrief2SortOrders(items map[string]int) map[string]int {
	if items == nil {
		return nil
	}
	out := make(map[string]int, len(items))
	for key, value := range items {
		out[key] = value
	}
	return out
}

func cloneBrief2Scoring(scoring []factor.Scoring) []factor.Scoring {
	if scoring == nil {
		return nil
	}
	out := make([]factor.Scoring, 0, len(scoring))
	for _, rule := range scoring {
		copied := rule
		copied.Sources = cloneBrief2Sources(rule.Sources)
		if rule.Params != nil {
			copied.Params = &factor.ScoringParams{CntOptionContents: append([]string(nil), rule.Params.CntOptionContents...)}
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

func cloneBrief2Sources(sources []factor.ScoringSource) []factor.ScoringSource {
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
