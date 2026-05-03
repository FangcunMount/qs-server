package plan

import "context"

// ScaleCatalog is the narrow scale lookup surface consumed by plan use cases.
type ScaleCatalog interface {
	ExistsByCode(ctx context.Context, code string) (bool, error)
	ResolveTitle(ctx context.Context, code string) string
	ResolveTitles(ctx context.Context, codes []string) map[string]string
}
