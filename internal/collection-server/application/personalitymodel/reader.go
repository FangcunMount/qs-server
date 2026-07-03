package personalitymodel

import "context"

// CatalogReader 人格模型目录读端口（application-owned DTO）。
type CatalogReader interface {
	GetPersonalityModel(ctx context.Context, code string) (*PersonalityModelResponse, error)
	ListPersonalityModels(ctx context.Context, page, pageSize int32, algorithm string) (*ListPersonalityModelsResponse, error)
	GetPersonalityModelCategories(ctx context.Context) (*PersonalityModelCategoriesResponse, error)
}
