package reporttemplate

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
)

// Repository persists Interpretation-owned report template releases.
type Repository interface {
	Save(ctx context.Context, template *ReportTemplate) error
	FindByKey(ctx context.Context, templateID string, version policy.TemplateVersion) (*ReportTemplate, error)
	FindPublished(ctx context.Context, templateID string, version policy.TemplateVersion) (*ReportTemplate, error)
	ListByTemplateID(ctx context.Context, templateID string, limit int) ([]*ReportTemplate, error)
}

// Catalog answers whether a template release is selectable at publish/freeze time.
type Catalog interface {
	IsPublished(templateID string, version string) bool
}
