package grpcbridge

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/typologymodel"
)

// PersonalityCatalogReader 将 infra gRPC 输出转换为 application DTO。
type PersonalityCatalogReader struct {
	inner PersonalityModelReader
}

func NewPersonalityCatalogReader(inner PersonalityModelReader) *PersonalityCatalogReader {
	return &PersonalityCatalogReader{inner: inner}
}

func (r *PersonalityCatalogReader) GetPersonalityModel(ctx context.Context, code string) (*typologymodel.PersonalityModelResponse, error) {
	if r == nil {
		return nil, nil
	}
	return CallBridge(r.inner,
		func() (*PersonalityModelOutput, error) { return r.inner.GetPersonalityModel(ctx, code) },
		toPersonalityModelResponse,
	)
}

func (r *PersonalityCatalogReader) ListPersonalityModels(ctx context.Context, page, pageSize int32, algorithm string) (*typologymodel.ListPersonalityModelsResponse, error) {
	if r == nil {
		return nil, nil
	}
	return CallBridge(r.inner,
		func() (*ListPersonalityModelsOutput, error) {
			return r.inner.ListPersonalityModels(ctx, page, pageSize, algorithm)
		},
		toListPersonalityModelsResponse,
	)
}

func (r *PersonalityCatalogReader) GetPersonalityModelCategories(ctx context.Context) (*typologymodel.PersonalityModelCategoriesResponse, error) {
	if r == nil {
		return nil, nil
	}
	return CallBridge(r.inner,
		func() (*PersonalityModelCategoriesOutput, error) { return r.inner.GetPersonalityModelCategories(ctx) },
		toPersonalityModelCategoriesResponse,
	)
}

func toListPersonalityModelsResponse(out *ListPersonalityModelsOutput) *typologymodel.ListPersonalityModelsResponse {
	models := make([]typologymodel.PersonalityModelSummaryResponse, 0, len(out.Models))
	for _, model := range out.Models {
		models = append(models, toPersonalitySummary(model))
	}
	return &typologymodel.ListPersonalityModelsResponse{
		Models:     models,
		Total:      out.Total,
		Page:       out.Page,
		PageSize:   out.PageSize,
		TotalPages: out.TotalPages,
	}
}

func toPersonalityModelCategoriesResponse(out *PersonalityModelCategoriesOutput) *typologymodel.PersonalityModelCategoriesResponse {
	categories := make([]typologymodel.CategoryResponse, 0, len(out.Categories))
	for _, item := range out.Categories {
		categories = append(categories, typologymodel.CategoryResponse{Value: item.Value, Label: item.Label})
	}
	return &typologymodel.PersonalityModelCategoriesResponse{Categories: categories}
}

func toPersonalitySummary(model PersonalityModelSummaryOutput) typologymodel.PersonalityModelSummaryResponse {
	return typologymodel.PersonalityModelSummaryResponse{
		Code:                 model.Code,
		Version:              model.Version,
		Title:                model.Title,
		Algorithm:            model.Algorithm,
		Description:          model.Description,
		QuestionnaireCode:    model.QuestionnaireCode,
		QuestionnaireVersion: model.QuestionnaireVersion,
		Status:               model.Status,
		QuestionCount:        model.QuestionCount,
		Kind:                 model.Kind,
		SubKind:              model.SubKind,
		ProductChannel:       model.ProductChannel,
		AlgorithmFamily:      model.AlgorithmFamily,
		PayloadFormat:        model.PayloadFormat,
		DecisionKind:         model.DecisionKind,
	}
}

func toPersonalityModelResponse(model *PersonalityModelOutput) *typologymodel.PersonalityModelResponse {
	dimensions := make([]typologymodel.PersonalityDimensionResponse, 0, len(model.Dimensions))
	for _, dim := range model.Dimensions {
		dimensions = append(dimensions, typologymodel.PersonalityDimensionResponse{
			Code:      dim.Code,
			Name:      dim.Name,
			LeftPole:  dim.LeftPole,
			RightPole: dim.RightPole,
		})
	}
	outcomes := make([]typologymodel.PersonalityOutcomeResponse, 0, len(model.Outcomes))
	for _, outcome := range model.Outcomes {
		outcomes = append(outcomes, typologymodel.PersonalityOutcomeResponse{
			Code:     outcome.Code,
			Name:     outcome.Name,
			OneLiner: outcome.OneLiner,
			ImageURL: outcome.ImageURL,
		})
	}
	summary := toPersonalitySummary(model.Summary)
	return &typologymodel.PersonalityModelResponse{
		Code:                 summary.Code,
		Version:              summary.Version,
		Title:                summary.Title,
		Algorithm:            summary.Algorithm,
		Description:          summary.Description,
		QuestionnaireCode:    summary.QuestionnaireCode,
		QuestionnaireVersion: summary.QuestionnaireVersion,
		Status:               summary.Status,
		QuestionCount:        summary.QuestionCount,
		Kind:                 summary.Kind,
		SubKind:              summary.SubKind,
		ProductChannel:       summary.ProductChannel,
		AlgorithmFamily:      summary.AlgorithmFamily,
		PayloadFormat:        summary.PayloadFormat,
		DecisionKind:         summary.DecisionKind,
		DimensionOrder:       append([]string(nil), model.DimensionOrder...),
		Dimensions:           dimensions,
		Outcomes:             outcomes,
	}
}
