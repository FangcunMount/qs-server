package assessment

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
)

func EnrichModelIdentityResult(model ModelIdentityResult, explicitProductChannel string) ModelIdentityResult {
	kind := binding.Kind(model.Kind)
	model.ProductChannel = binding.ProductChannelForIdentity(kind, firstNonEmpty(explicitProductChannel, model.ProductChannel))
	model.AlgorithmFamily = binding.AlgorithmFamilyStringFromIdentity(kind, binding.SubKind(model.SubKind), binding.Algorithm(model.Algorithm))
	return model
}
