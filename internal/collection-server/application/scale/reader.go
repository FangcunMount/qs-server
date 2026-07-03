package scale

import "context"

// CatalogReader 量表目录读端口（application-owned DTO）。
type CatalogReader interface {
	GetScale(ctx context.Context, code string) (*ScaleResponse, error)
	ListScales(ctx context.Context, page, pageSize int32, status, title, category string, stages, applicableAges, reporters, tags []string) (*ListScalesResponse, error)
	ListHotScales(ctx context.Context, limit, windowDays int32) (*ListHotScalesResponse, error)
	GetScaleCategories(ctx context.Context) (*ScaleCategoriesResponse, error)
}
