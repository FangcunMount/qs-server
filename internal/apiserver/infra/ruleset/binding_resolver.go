package ruleset

import (
	"context"

	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// CatalogBindingResolver 从统一规则目录解析建测评绑定。
type CatalogBindingResolver struct {
	catalog port.Catalog
}

var _ port.AssessmentBindingResolver = (*CatalogBindingResolver)(nil)

func NewAssessmentBindingResolver(catalog port.Catalog) *CatalogBindingResolver {
	return &CatalogBindingResolver{catalog: catalog}
}

func (r *CatalogBindingResolver) ResolveByQuestionnaire(
	ctx context.Context,
	questionnaireCode, questionnaireVersion string,
) (port.Ref, bool, error) {
	if r == nil || r.catalog == nil {
		return port.Ref{}, false, nil
	}
	return r.catalog.ResolveByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
}

func (r *CatalogBindingResolver) ResolveAssessmentBinding(
	ctx context.Context,
	questionnaireCode, questionnaireVersion string,
) (port.AssessmentBinding, bool, error) {
	if r == nil || r.catalog == nil || questionnaireCode == "" {
		return port.AssessmentBinding{}, false, nil
	}
	ref, ok, err := r.catalog.ResolveByQuestionnaire(ctx, questionnaireCode, questionnaireVersion)
	if err != nil || !ok || ref.IsEmpty() {
		return port.AssessmentBinding{}, ok, err
	}
	return port.AssessmentBinding{Ref: ref}, true, nil
}
