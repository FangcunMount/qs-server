package brief2

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	factornorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor_norm"
)

// CompositeIndexSpec declares how a Brief-2 composite index derives from child factors.
type CompositeIndexSpec = factornorm.CompositeIndexSpec

// ApplyCompositeMetadata annotates factors with Brief-2 composite index policies.
func ApplyCompositeMetadata(factors []factor.FactorSnapshot, specs []CompositeIndexSpec) []factor.FactorSnapshot {
	return factornorm.ApplyCompositeMetadata(factors, specs)
}
