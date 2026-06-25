package plan

import (
	"context"

	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/scale/definition"
)

type repositoryScaleCatalog struct {
	repo scaledefinition.Repository
}

func newRepositoryScaleCatalog(repo scaledefinition.Repository) ScaleCatalog {
	if repo == nil {
		return nil
	}
	return repositoryScaleCatalog{repo: repo}
}

// NewRepositoryScaleCatalog adapts the legacy scale repository to the narrow plan catalog.
func NewRepositoryScaleCatalog(repo scaledefinition.Repository) ScaleCatalog {
	return newRepositoryScaleCatalog(repo)
}

func (c repositoryScaleCatalog) ExistsByCode(ctx context.Context, code string) (bool, error) {
	return c.repo.ExistsByCode(ctx, code)
}

func (c repositoryScaleCatalog) ResolveTitle(ctx context.Context, code string) string {
	if code == "" || c.repo == nil {
		return code
	}
	scale, err := c.repo.FindByCode(ctx, code)
	if err != nil || scale == nil {
		return code
	}
	return scale.GetTitle()
}

func (c repositoryScaleCatalog) ResolveTitles(ctx context.Context, codes []string) map[string]string {
	titles := make(map[string]string, len(codes))
	for _, code := range codes {
		if code == "" {
			continue
		}
		titles[code] = c.ResolveTitle(ctx, code)
	}
	return titles
}
