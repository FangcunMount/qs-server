package typologymodel

import "context"

// CatalogReader 类型学模型目录读端口（application-owned DTO）。
type CatalogReader interface {
	GetTypologyModel(ctx context.Context, code string) (*TypologyModelResponse, error)
	ListTypologyModels(ctx context.Context, page, pageSize int32, algorithm string) (*ListTypologyModelsResponse, error)
	GetTypologyModelCategories(ctx context.Context) (*TypologyModelCategoriesResponse, error)
}
