package plan

import (
	"context"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

type assessmentModelScaleCatalog struct {
	repo modelcatalogport.ModelRepository
}

func newAssessmentModelScaleCatalog(repo modelcatalogport.ModelRepository) ScaleCatalog {
	if repo == nil {
		return nil
	}
	return assessmentModelScaleCatalog{repo: repo}
}

// NewAssessmentModelScaleCatalog adapts assessment model repository to plan scale catalog.
func NewAssessmentModelScaleCatalog(repo modelcatalogport.ModelRepository) ScaleCatalog {
	return newAssessmentModelScaleCatalog(repo)
}

func (c assessmentModelScaleCatalog) ExistsByCode(ctx context.Context, code string) (bool, error) {
	if code == "" || c.repo == nil {
		return false, nil
	}
	model, err := c.repo.FindByCode(ctx, code)
	if err != nil {
		if domain.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return model != nil && model.Kind == domain.KindScale, nil
}

func (c assessmentModelScaleCatalog) ResolveTitle(ctx context.Context, code string) string {
	if code == "" || c.repo == nil {
		return code
	}
	model, err := c.repo.FindByCode(ctx, code)
	if err != nil || model == nil || model.Kind != domain.KindScale {
		return code
	}
	return model.Title
}

func (c assessmentModelScaleCatalog) ResolveTitles(ctx context.Context, codes []string) map[string]string {
	titles := make(map[string]string, len(codes))
	for _, code := range codes {
		if code == "" {
			continue
		}
		titles[code] = c.ResolveTitle(ctx, code)
	}
	return titles
}
