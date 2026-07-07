package option

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/capability"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

// ModelCatalogOption is the application-facing catalog presentation contract.
type ModelCatalogOption = capability.CatalogOption

// DefaultOptions returns catalog presentation options for REST/BFF surfaces.
func DefaultOptions() []ModelCatalogOption {
	return capability.DefaultCatalogOptions()
}

// ByKind resolves presentation options for a model family kind.
func ByKind(kind identity.Kind) (ModelCatalogOption, bool) {
	return capability.CatalogOptionByKind(kind)
}
