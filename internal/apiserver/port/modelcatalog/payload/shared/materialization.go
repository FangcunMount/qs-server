package shared

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
)

// DefinitionMaterialization is the semantic result of decoding a wire payload.
// Payload adapters may additionally extract immutable norm reference material.
type DefinitionMaterialization struct {
	Definition *definition.Definition
	Norms      []*norm.Norm
}
