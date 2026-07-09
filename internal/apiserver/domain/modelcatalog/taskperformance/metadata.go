package taskperformance

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
)

// MetadataContext 携带task_performance 元数据 不使用 embedding 常模表 bodies。
// 执行层 计分 (answer 键, ability 等级) 等待 second task_performance model。
type MetadataContext struct {
	NormTableVersion string
	ItemSetCodes     []string
}

// ApplyNormMetadataToLegacyFactors 标注 legacy flat 因子 使用 task-set 角色 和 常模 references。
func ApplyNormMetadataToLegacyFactors(factors []factor.LegacyFactor, ctx MetadataContext) []factor.LegacyFactor {
	if len(factors) == 0 {
		return factors
	}
	itemSetCodes := stringSet(ctx.ItemSetCodes)
	out := make([]factor.LegacyFactor, len(factors))
	for i, item := range factors {
		out[i] = item
		if itemSetCodes[item.Code] {
			out[i].Role = factor.FactorRoleTaskSet
		}
		if ctx.NormTableVersion != "" && (item.IsTotalScore || itemSetCodes[item.Code]) {
			out[i].Norm = &factor.NormRef{
				FactorCode:       item.Code,
				NormTableVersion: ctx.NormTableVersion,
			}
		}
	}
	return out
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
