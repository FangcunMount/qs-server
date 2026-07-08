package assessment

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/publishing"
)

func EnrichModelIdentityResult(model ModelIdentityResult, explicitProductChannel string) ModelIdentityResult {
	kind := binding.Kind(model.Kind)
	model.ProductChannel = publishing.ProductChannelForIdentity(kind, firstNonEmpty(explicitProductChannel, model.ProductChannel))
	model.AlgorithmFamily = publishing.AlgorithmFamilyStringFromIdentity(kind, binding.SubKind(model.SubKind), binding.Algorithm(model.Algorithm))
	return model
}
