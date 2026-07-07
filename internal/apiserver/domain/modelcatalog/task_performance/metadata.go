package task_performance

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

// MetadataContext carries task_performance metadata without embedding norm table bodies.
// Execution-layer scoring (answer key, ability level) awaits a second task_performance model.
type MetadataContext struct {
	NormTableVersion string
	ItemSetCodes     []string
}

// ApplyNormMetadata annotates canonical factors with task-set roles and norm references.
func ApplyNormMetadata(factors []factor.FactorSnapshot, ctx MetadataContext) []factor.FactorSnapshot {
	if len(factors) == 0 {
		return factors
	}
	itemSetCodes := stringSet(ctx.ItemSetCodes)
	out := make([]factor.FactorSnapshot, len(factors))
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
