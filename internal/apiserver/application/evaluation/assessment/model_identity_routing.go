package assessment

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	identitypkg "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

func EnrichModelIdentityResult(model ModelIdentityResult, explicitProductChannel string) ModelIdentityResult {
	kind := binding.Kind(model.Kind)
	model.ProductChannel = identitypkg.ProductChannelForIdentity(kind, firstNonEmpty(explicitProductChannel, model.ProductChannel))
	model.AlgorithmFamily = identitypkg.AlgorithmFamilyStringFromIdentity(kind, binding.SubKind(model.SubKind), binding.Algorithm(model.Algorithm))
	return model
}
