package interpretationmodel

import (
	"context"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretationmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/interpretationmodel/codec"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationmodel"
)

// CatalogBindingResolver 从统一规则目录解析建测评绑定。
type CatalogBindingResolver struct {
	catalog port.ModelCatalog
}

var _ port.AssessmentBindingResolver = (*CatalogBindingResolver)(nil)

func NewAssessmentBindingResolver(catalog port.ModelCatalog) *CatalogBindingResolver {
	return &CatalogBindingResolver{catalog: catalog}
}

func (r *CatalogBindingResolver) ResolveByQuestionnaire(
	ctx context.Context,
	questionnaireCode, questionnaireVersion string,
) (port.ModelRef, bool, error) {
	if r == nil || r.catalog == nil {
		return port.ModelRef{}, false, nil
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
	if ref.Kind != domain.ModelKindScale {
		return port.InterpretationAssessmentBinding(ref), true, nil
	}
	snapshot, err := r.catalog.GetPublishedByRef(ctx, ref)
	if err != nil {
		return port.AssessmentBinding{}, false, err
	}
	scale, err := codec.DecodeScale(snapshot)
	if err != nil {
		return port.AssessmentBinding{}, false, err
	}
	version := scale.ScaleVersion
	if version == "" {
		version = ref.Version
	}
	return port.ScaleAssessmentBinding(ref, scale.ID, scale.Code, scale.Title, version), true, nil
}
