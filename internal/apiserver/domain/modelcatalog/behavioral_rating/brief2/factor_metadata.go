package brief2

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	factornorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor_norm"
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
	return factornorm.ApplyNormMetadata(factors, factornorm.MetadataContext{
		NormTableVersion: ctx.NormTableVersion,
		IndexCodes:       ctx.IndexCodes,
		ValidityCodes:    ctx.ValidityCodes,
		NormFactorCodes:  ctx.NormFactorCodes,
	})
}
