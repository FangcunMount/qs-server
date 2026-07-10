package grpcbridge

import (
	"context"
	"encoding/json"

	appmodelcatalog "github.com/FangcunMount/qs-server/internal/collection-server/application/modelcatalog"
	grpcclient "github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
)

// AssessmentModelCatalogAdapter isolates collection application services from
// the gRPC client implementation while preserving the published-only contract.
type AssessmentModelCatalogAdapter struct {
	inner *grpcclient.AssessmentModelCatalogClient
}

func NewAssessmentModelCatalogReader(inner *grpcclient.AssessmentModelCatalogClient) *AssessmentModelCatalogAdapter {
	return &AssessmentModelCatalogAdapter{inner: inner}
}

func (r *AssessmentModelCatalogAdapter) GetPublishedModel(ctx context.Context, code, version string) (*appmodelcatalog.CatalogModel, error) {
	if r == nil || r.inner == nil {
		return nil, nil
	}
	value, err := r.inner.GetPublishedModel(ctx, code, version)
	return catalogModelOutput(value), err
}

func (r *AssessmentModelCatalogAdapter) ListPublishedModels(ctx context.Context, kind, subKind, algorithm, category, keyword, questionnaireCode, questionnaireVersion string, page, pageSize int32) (*appmodelcatalog.CatalogList, error) {
	if r == nil || r.inner == nil {
		return nil, nil
	}
	value, err := r.inner.ListPublishedModels(ctx, kind, subKind, algorithm, category, keyword, questionnaireCode, questionnaireVersion, page, pageSize)
	if err != nil || value == nil {
		return nil, err
	}
	result := &appmodelcatalog.CatalogList{Models: make([]appmodelcatalog.CatalogModel, 0, len(value.Models)), Total: value.Total, Page: value.Page, PageSize: value.PageSize}
	for index := range value.Models {
		result.Models = append(result.Models, *catalogModelOutput(&value.Models[index]))
	}
	return result, nil
}

func (r *AssessmentModelCatalogAdapter) ListHotPublishedModels(ctx context.Context, kind string, limit, windowDays int32) (*appmodelcatalog.HotCatalogList, error) {
	if r == nil || r.inner == nil {
		return nil, nil
	}
	value, err := r.inner.ListHotPublishedModels(ctx, kind, limit, windowDays)
	if err != nil || value == nil {
		return nil, err
	}
	result := &appmodelcatalog.HotCatalogList{Models: make([]appmodelcatalog.HotCatalogModel, 0, len(value.Models)), Total: value.Total, Limit: value.Limit, WindowDays: value.WindowDays}
	for _, item := range value.Models {
		result.Models = append(result.Models, appmodelcatalog.HotCatalogModel{Model: *catalogModelOutput(&item.Model), Rank: item.Rank, SubmissionCount: item.SubmissionCount, HeatScore: item.HeatScore})
	}
	return result, nil
}

func (r *AssessmentModelCatalogAdapter) GetCatalogOptions(ctx context.Context, kind string) (*appmodelcatalog.CatalogOptions, error) {
	if r == nil || r.inner == nil {
		return nil, nil
	}
	value, err := r.inner.GetCatalogOptions(ctx, kind)
	if err != nil || value == nil {
		return nil, err
	}
	return &appmodelcatalog.CatalogOptions{
		Kinds:             catalogOptionOutputs(value.Kinds),
		ProductChannels:   catalogOptionOutputs(value.ProductChannels),
		AlgorithmFamilies: catalogOptionOutputs(value.AlgorithmFamilies),
		Algorithms:        catalogOptionOutputs(value.Algorithms),
		SubKinds:          catalogOptionOutputs(value.SubKinds),
		Categories:        catalogOptionOutputs(value.Categories),
		Stages:            catalogOptionOutputs(value.Stages),
		ApplicableAges:    catalogOptionOutputs(value.ApplicableAges),
		Reporters:         catalogOptionOutputs(value.Reporters),
	}, nil
}

func catalogModelOutput(value *grpcclient.CatalogModelOutput) *appmodelcatalog.CatalogModel {
	if value == nil {
		return nil
	}
	return &appmodelcatalog.CatalogModel{
		Code: value.Code, Kind: value.Kind, SubKind: value.SubKind, Algorithm: value.Algorithm, ProductChannel: value.ProductChannel,
		Version: value.Version, Title: value.Title, Description: value.Description, Status: value.Status, Category: value.Category,
		Stages: append([]string(nil), value.Stages...), ApplicableAges: append([]string(nil), value.ApplicableAges...), Reporters: append([]string(nil), value.Reporters...), Tags: append([]string(nil), value.Tags...),
		QuestionnaireCode: value.QuestionnaireCode, QuestionnaireVersion: value.QuestionnaireVersion, Definition: append(json.RawMessage(nil), value.Definition...),
	}
}

func catalogOptionOutputs(values []grpcclient.CatalogOptionOutput) []appmodelcatalog.CatalogOption {
	result := make([]appmodelcatalog.CatalogOption, 0, len(values))
	for _, value := range values {
		result = append(result, appmodelcatalog.CatalogOption{Label: value.Label, Value: value.Value, Disabled: value.Disabled})
	}
	return result
}

var _ appmodelcatalog.CatalogReader = (*AssessmentModelCatalogAdapter)(nil)
