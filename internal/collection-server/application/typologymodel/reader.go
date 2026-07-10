package typologymodel

import "context"

// CatalogReader is the typology presentation projection consumed by the BFF.
// Its port adapter reads the generic published-model catalog and projects
// DefinitionV2 without calling a typology-specific gRPC service.
type CatalogReader interface {
	GetTypologyModel(context.Context, string) (*TypologyModelResponse, error)
	ListTypologyModels(context.Context, int32, int32) (*ListTypologyModelsResponse, error)
	GetTypologyModelCategories(context.Context) (*TypologyModelCategoriesResponse, error)
}
