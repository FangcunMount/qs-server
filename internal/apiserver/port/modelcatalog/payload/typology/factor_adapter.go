package typology

import (
	"sort"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

func canonicalMeasureSpecFromGraph(fg FactorGraphSpec) definition.MeasureSpec {
	if fg.HasExplicitFactorGraph() {
		return canonicalMeasureFromExplicitGraph(fg)
	}
	return canonicalMeasureFromLegacyLayout(fg)
}

// CanonicalMeasureSpec 投影运行时规格 为测量层配置。
func (s *RuntimeSpec) CanonicalMeasureSpec() definition.MeasureSpec {
	if s == nil {
		return definition.MeasureSpec{}
	}
	return canonicalMeasureSpecFromGraph(s.FactorGraph)
}

func canonicalMeasureFromLegacyLayout(fg FactorGraphSpec) definition.MeasureSpec {
	if len(fg.DimensionOrder) == 0 {
		return definition.MeasureSpec{}
	}
	factors := make([]factor.Factor, 0, len(fg.DimensionOrder))
	scoring := make([]factor.Scoring, 0, len(fg.DimensionOrder))
	for _, code := range fg.DimensionOrder {
		dim, ok := fg.Dimensions[code]
		if !ok {
			continue
		}
		factors = append(factors, legacyDimensionToCanonical(dim))
		scoring = appendQuestionScoring(scoring, dim.Code, questionCodesForDimension(dim.Code, fg.QuestionMappings))
	}
	return definition.MeasureSpec{Factors: factors, Scoring: scoring}
}

func canonicalMeasureFromExplicitGraph(fg FactorGraphSpec) definition.MeasureSpec {
	factors := make([]factor.Factor, 0, len(fg.Factors))
	scoring := make([]factor.Scoring, 0, len(fg.Factors))
	for _, spec := range fg.Factors {
		if spec.Kind != FactorSpecKindLeaf {
			continue
		}
		factors = append(factors, leafFactorToCanonical(spec))
		code := spec.Code
		if code == "" {
			code = spec.ID
		}
		scoring = appendQuestionScoring(scoring, code, questionCodesFromContributions(spec.Contributions))
	}
	sort.Slice(factors, func(i, j int) bool {
		return factors[i].Code < factors[j].Code
	})
	sort.Slice(scoring, func(i, j int) bool {
		return scoring[i].FactorCode < scoring[j].FactorCode
	})
	return definition.MeasureSpec{Factors: factors, Scoring: scoring}
}

func legacyDimensionToCanonical(dim Dimension) factor.Factor {
	return factor.Factor{
		Code:  dim.Code,
		Title: dim.Name,
		Role:  factor.FactorRoleDimension,
	}
}

func leafFactorToCanonical(spec FactorSpec) factor.Factor {
	code := spec.Code
	if code == "" {
		code = spec.ID
	}
	return factor.Factor{
		Code:  code,
		Title: spec.Name,
		Role:  factor.FactorRoleDimension,
	}
}

func appendQuestionScoring(scoring []factor.Scoring, factorCode string, questionCodes []string) []factor.Scoring {
	if factorCode == "" || len(questionCodes) == 0 {
		return scoring
	}
	sources := make([]factor.ScoringSource, 0, len(questionCodes))
	for _, code := range questionCodes {
		sources = append(sources, factor.ScoringSource{Kind: factor.ScoringSourceQuestion, Code: code})
	}
	return append(scoring, factor.Scoring{FactorCode: factorCode, Sources: sources})
}

func questionCodesFromContributions(contributions []FactorContributionSpec) []string {
	codes := make([]string, 0, len(contributions))
	for _, contribution := range contributions {
		codes = append(codes, contribution.QuestionCode)
	}
	return codes
}

func questionCodesForDimension(dimensionCode string, mappings []QuestionMapping) []string {
	codes := make([]string, 0, len(mappings))
	for _, mapping := range mappings {
		if mapping.Dimension == dimensionCode {
			codes = append(codes, mapping.QuestionCode)
		}
	}
	return codes
}
