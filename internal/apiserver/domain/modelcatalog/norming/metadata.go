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

// ApplyCompositeMetadataToLegacyFactors 标注 legacy flat 因子 使用 复合 index 策略。
func ApplyCompositeMetadataToLegacyFactors(factors []factor.LegacyFactor, specs []CompositeIndexSpec) []factor.LegacyFactor {
	if len(factors) == 0 || len(specs) == 0 {
		return factors
	}
	out := make([]factor.LegacyFactor, len(factors))
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
	return factor.DeriveFactorLevels(out)
}

// MeasureSpecWithCompositeMetadata projects composite metadata into the target measure layer.
func MeasureSpecWithCompositeMetadata(factors []factor.Factor, specs []CompositeIndexSpec) definition.MeasureSpec {
	legacy := make([]factor.LegacyFactor, 0, len(factors))
	for _, item := range factors {
		legacy = append(legacy, factor.LegacyFactor{
			Code:  item.Code,
			Title: item.Title,
			Role:  item.Role,
		})
	}
	return definition.MeasureSpecFromLegacyFactors(ApplyCompositeMetadataToLegacyFactors(legacy, specs))
}

// MetadataContext 携带常模ing 元数据 不使用 embedding 常模表 bodies。
type MetadataContext struct {
	NormTableVersion string
	IndexCodes       []string
	ValidityCodes    []string
	NormFactorCodes  []string
}

// ApplyNormMetadataToLegacyFactors 标注 legacy flat 因子 使用 index/有效ity 角色 和 常模 references。
func ApplyNormMetadataToLegacyFactors(factors []factor.LegacyFactor, ctx MetadataContext) []factor.LegacyFactor {
	if len(factors) == 0 {
		return factors
	}
	indexCodes := stringSet(ctx.IndexCodes)
	validityCodes := stringSet(ctx.ValidityCodes)
	normFactorCodes := stringSet(ctx.NormFactorCodes)
	out := make([]factor.LegacyFactor, len(factors))
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
