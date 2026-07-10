package modelcatalog

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// PublishedModelTitleResolver is a narrow trusted-runtime read service for
// integrations that only need display metadata from an immutable model.
type PublishedModelTitleResolver interface {
	ResolvePublishedTitle(ctx context.Context, kind domain.Kind, codeValue string) (string, error)
}

type publishedModelTitleResolver struct {
	lister modelcatalogport.PublishedModelLister
}

func NewPublishedModelTitleResolver(lister modelcatalogport.PublishedModelLister) PublishedModelTitleResolver {
	return &publishedModelTitleResolver{lister: lister}
}

func (r *publishedModelTitleResolver) ResolvePublishedTitle(ctx context.Context, kind domain.Kind, codeValue string) (string, error) {
	if r == nil || r.lister == nil {
		return "", errors.WithCode(code.ErrInternalServerError, "published model title resolver is not configured")
	}
	if kind == "" || codeValue == "" {
		return "", errors.WithCode(code.ErrInvalidArgument, "published model kind and code are required")
	}
	model, err := r.lister.FindPublishedModelByCode(ctx, kind, codeValue)
	if err != nil {
		return "", err
	}
	if _, err := requireRuntimeDefinition(model); err != nil {
		return "", err
	}
	return model.Title, nil
}

var _ PublishedModelTitleResolver = (*publishedModelTitleResolver)(nil)
