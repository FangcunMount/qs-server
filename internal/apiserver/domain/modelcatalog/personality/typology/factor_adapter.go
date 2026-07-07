package typology

import (
	"sort"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

// CanonicalFactorsFromGraph 投影类型学 因子图 配置 为 规范 目录 因子。
// The adapter 是 只读: 类型学 运行时 keeps owning 图执行语义。
func CanonicalFactorsFromGraph(fg FactorGraphSpec) []factor.FactorSnapshot {
	if fg.HasExplicitFactorGraph() {
		return canonicalFactorsFromExplicitGraph(fg)
	}
	return canonicalFactorsFromLegacyLayout(fg)
}

// CanonicalFactors 投影运行时规格 为 规范 目录 因子。
func (s *RuntimeSpec) CanonicalFactors() []factor.FactorSnapshot {
	if s == nil {
		return nil
	}
	return CanonicalFactorsFromGraph(s.FactorGraph)
}

// CanonicalFactors 解析运行时规格 和 投影 类型学 配置 为 规范 因子。
func (p *Payload) CanonicalFactors() ([]factor.FactorSnapshot, error) {
	spec, err := p.ToRuntimeSpec()
	if err != nil {
		return nil, err
	}
	return spec.CanonicalFactors(), nil
}

func canonicalFactorsFromLegacyLayout(fg FactorGraphSpec) []factor.FactorSnapshot {
	if len(fg.DimensionOrder) == 0 {
		return nil
	}
	factors := make([]factor.FactorSnapshot, 0, len(fg.DimensionOrder))
	for _, code := range fg.DimensionOrder {
		dim, ok := fg.Dimensions[code]
		if !ok {
			continue
		}
		factors = append(factors, legacyDimensionToCanonical(dim, fg.QuestionMappings))
	}
	return factors
}

func canonicalFactorsFromExplicitGraph(fg FactorGraphSpec) []factor.FactorSnapshot {
	factors := make([]factor.FactorSnapshot, 0, len(fg.Factors))
	for _, spec := range fg.Factors {
		if spec.Kind != FactorSpecKindLeaf {
			continue
		}
		factors = append(factors, leafFactorToCanonical(spec))
	}
	sort.Slice(factors, func(i, j int) bool {
		return factors[i].Code < factors[j].Code
	})
	return factors
}

func legacyDimensionToCanonical(dim Dimension, mappings []QuestionMapping) factor.FactorSnapshot {
	return factor.FactorSnapshot{
		Code:           dim.Code,
		Title:          dim.Name,
		Role:           factor.FactorRoleDimension,
		QuestionCodes:  questionCodesForDimension(dim.Code, mappings),
		Classification: classificationFromDimension(dim),
	}
}

func leafFactorToCanonical(spec FactorSpec) factor.FactorSnapshot {
	code := spec.Code
	if code == "" {
		code = spec.ID
	}
	questionCodes := make([]string, 0, len(spec.Contributions))
	for _, contribution := range spec.Contributions {
		questionCodes = append(questionCodes, contribution.QuestionCode)
	}
	return factor.FactorSnapshot{
		Code:          code,
		Title:         spec.Name,
		Role:          factor.FactorRoleDimension,
		QuestionCodes: questionCodes,
	}
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

func classificationFromDimension(dim Dimension) *factor.ClassificationSpec {
	if dim.LeftPole == "" && dim.RightPole == "" {
		return nil
	}
	return &factor.ClassificationSpec{
		NegativePole: dim.LeftPole,
		PositivePole: dim.RightPole,
	}
}
