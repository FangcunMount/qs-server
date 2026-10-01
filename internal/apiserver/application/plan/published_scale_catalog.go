package plan

import (
	"context"
	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

type publishedScaleCatalog struct {
	published modelcatalogport.PublishedModelLister
}

func newPublishedScaleCatalog(published modelcatalogport.PublishedModelLister) ScaleCatalog {
	if published == nil {
		return nil
	}
	return publishedScaleCatalog{published: published}
}

// NewPublishedScaleCatalog adapts the published assessment-model catalog to
// plan scale validation and title projection.
func NewPublishedScaleCatalog(published modelcatalogport.PublishedModelLister) ScaleCatalog {
	return newPublishedScaleCatalog(published)
}

func (c publishedScaleCatalog) ExistsByCode(ctx context.Context, code string) (bool, error) {
	if code == "" || c.published == nil {
		return false, nil
	}
	model, err := c.published.FindPublishedModelByCode(ctx, domain.KindScale, code)
	if err != nil {
		if domain.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return model != nil && model.Kind == domain.KindScale && model.DefinitionV2 != nil, nil
}

func (c publishedScaleCatalog) ResolveTitle(ctx context.Context, code string) string {
	if code == "" || c.published == nil {
		return code
	}
	model, err := c.published.FindPublishedModelByCode(ctx, domain.KindScale, code)
	if err != nil || model == nil || model.Kind != domain.KindScale || model.DefinitionV2 == nil {
		return code
	}
	return model.Title
}

func (c publishedScaleCatalog) ResolveTitles(ctx context.Context, codes []string) map[string]string {
	titles := make(map[string]string, len(codes))
	for _, code := range codes {
		if code == "" {
			continue
		}
		titles[code] = c.ResolveTitle(ctx, code)
	}
	return titles
}

// ResolveTaskContent pins the published model's exact question binding for
// this entry. Start and submit still validate and freeze the authoritative admission.
func (c publishedScaleCatalog) ResolveTaskContent(ctx context.Context, modelCode string) (string, string, string, error) {
	model, err := c.published.FindPublishedModelByCode(ctx, domain.KindScale, modelCode)
	if err != nil {
		return "", "", "", err
	}
	if model == nil || model.QuestionnaireCode == "" || model.QuestionnaireVersion == "" {
		return "", "", "", errors.WithCode(code.ErrPageNotFound, "task questionnaire binding unavailable")
	}
	return model.QuestionnaireCode, model.QuestionnaireVersion, model.Version, nil
}
