package brief2

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

// NormContext carries Brief-2 norm metadata without embedding norm table bodies.
type NormContext struct {
	NormTableVersion string
	IndexCodes       []string
	ValidityCodes    []string
	NormFactorCodes  []string
}

// ApplyNormMetadata annotates canonical factors with Brief-2 roles and norm references.
func ApplyNormMetadata(factors []factor.FactorSnapshot, ctx NormContext) []factor.FactorSnapshot {
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
