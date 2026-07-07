package norming

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

// CompositeIndexSpec declares 如何复合 index 推导自 子节点 因子。
type CompositeIndexSpec struct {
	Code       string
	Strategy   factor.ChildrenAggregationStrategy
	Children   []string
	ParentCode string
}

// ApplyCompositeMetadata 标注因子 使用 复合 index 策略。
func ApplyCompositeMetadata(factors []factor.FactorSnapshot, specs []CompositeIndexSpec) []factor.FactorSnapshot {
	if len(factors) == 0 || len(specs) == 0 {
		return factors
	}
	out := make([]factor.FactorSnapshot, len(factors))
	copy(out, factors)
	indexPos := make(map[string]int, len(out))
	for i, item := range out {
		indexPos[item.Code] = i
	}
	for _, spec := range specs {
		pos, ok := indexPos[spec.Code]
		if !ok || len(spec.Children) == 0 {
			continue
		}
		strategy := spec.Strategy
		if strategy == "" {
			strategy = factor.ChildrenAggregationSum
		}
		out[pos].ChildrenPolicy = &factor.ChildrenPolicy{
			Strategy: strategy,
			Children: append([]string(nil), spec.Children...),
		}
		if spec.ParentCode != "" {
			out[pos].ParentCode = spec.ParentCode
		}
		for _, childCode := range spec.Children {
			childPos, ok := indexPos[childCode]
			if !ok || out[childPos].ParentCode != "" {
				continue
			}
			out[childPos].ParentCode = spec.Code
		}
	}
	return factor.DeriveLevels(out)
}

// MetadataContext 携带常模ing 元数据 不使用 embedding 常模表 bodies。
type MetadataContext struct {
	NormTableVersion string
	IndexCodes       []string
	ValidityCodes    []string
	NormFactorCodes  []string
}

// ApplyNormMetadata 标注规范 因子 使用 index/有效ity 角色 和 常模 references。
func ApplyNormMetadata(factors []factor.FactorSnapshot, ctx MetadataContext) []factor.FactorSnapshot {
	if len(factors) == 0 {
		return factors
	}
	indexCodes := stringSet(ctx.IndexCodes)
	validityCodes := stringSet(ctx.ValidityCodes)
	normFactorCodes := stringSet(ctx.NormFactorCodes)
	out := make([]factor.FactorSnapshot, len(factors))
	for i, item := range factors {
		out[i] = item
		switch {
		case indexCodes[item.Code]:
			out[i].Role = factor.FactorRoleIndex
		case validityCodes[item.Code]:
			out[i].Role = factor.FactorRoleValidity
		}
		if normFactorCodes[item.Code] && ctx.NormTableVersion != "" {
			out[i].Norm = &factor.NormRef{
				FactorCode:       item.Code,
				NormTableVersion: ctx.NormTableVersion,
			}
		}
	}
	return out
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
