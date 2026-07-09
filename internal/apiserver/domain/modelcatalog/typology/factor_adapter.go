package typology

import (
	"sort"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

// CanonicalFactorsFromGraph 投影类型学 因子图 配置 为 规范 目录 因子。
// The adapter 是 只读: 类型学 运行时 keeps owning 图执行语义。
func CanonicalFactorsFromGraph(fg FactorGraphSpec) []factor.Factor {
	return CanonicalMeasureSpecFromGraph(fg).Factors
}

// CanonicalMeasureSpecFromGraph 投影类型学 因子图 配置 为测量层配置。
func CanonicalMeasureSpecFromGraph(fg FactorGraphSpec) definition.MeasureSpec {
	if fg.HasExplicitFactorGraph() {
		return canonicalMeasureFromExplicitGraph(fg)
	}
	return canonicalMeasureFromLegacyLayout(fg)
}

// CanonicalFactors 投影运行时规格 为 规范 目录 因子。
func (s *RuntimeSpec) CanonicalFactors() []factor.Factor {
	if s == nil {
		return nil
	}
	return CanonicalFactorsFromGraph(s.FactorGraph)
}

// CanonicalMeasureSpec 投影运行时规格 为测量层配置。
func (s *RuntimeSpec) CanonicalMeasureSpec() definition.MeasureSpec {
	if s == nil {
		return definition.MeasureSpec{}
	}
	return CanonicalMeasureSpecFromGraph(s.FactorGraph)
}

// CanonicalFactors 解析运行时规格 和 投影 类型学 配置 为 规范 因子。
func (p *Payload) CanonicalFactors() ([]factor.Factor, error) {
	spec, err := p.ToRuntimeSpec()
	if err != nil {
		return nil, err
	}
	return spec.CanonicalFactors(), nil
}

// CanonicalMeasureSpec 解析运行时规格 和 投影 类型学 配置 为测量层配置。
func (p *Payload) CanonicalMeasureSpec() (definition.MeasureSpec, error) {
	spec, err := p.ToRuntimeSpec()
	if err != nil {
		return definition.MeasureSpec{}, err
	}
	return spec.CanonicalMeasureSpec(), nil
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
