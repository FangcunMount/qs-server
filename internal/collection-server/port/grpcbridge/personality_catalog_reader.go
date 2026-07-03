package grpcbridge

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/personalitymodel"
)

// PersonalityCatalogReader 将 infra gRPC 输出转换为 application DTO。
type PersonalityCatalogReader struct {
	inner PersonalityModelReader
}

func NewPersonalityCatalogReader(inner PersonalityModelReader) *PersonalityCatalogReader {
	return &PersonalityCatalogReader{inner: inner}
}

func (r *PersonalityCatalogReader) GetPersonalityModel(ctx context.Context, code string) (*personalitymodel.PersonalityModelResponse, error) {
	if r == nil {
		return nil, nil
	}
	return CallBridge(r.inner,
		func() (*PersonalityModelOutput, error) { return r.inner.GetPersonalityModel(ctx, code) },
		toPersonalityModelResponse,
	)
}

func (r *PersonalityCatalogReader) ListPersonalityModels(ctx context.Context, page, pageSize int32, algorithm string) (*personalitymodel.ListPersonalityModelsResponse, error) {
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

func (r *PersonalityCatalogReader) GetPersonalityModelCategories(ctx context.Context) (*personalitymodel.PersonalityModelCategoriesResponse, error) {
	if r == nil {
		return nil, nil
	}
	return CallBridge(r.inner,
		func() (*PersonalityModelCategoriesOutput, error) { return r.inner.GetPersonalityModelCategories(ctx) },
		toPersonalityModelCategoriesResponse,
	)
}

func toListPersonalityModelsResponse(out *ListPersonalityModelsOutput) *personalitymodel.ListPersonalityModelsResponse {
	models := make([]personalitymodel.PersonalityModelSummaryResponse, 0, len(out.Models))
	for _, model := range out.Models {
		models = append(models, toPersonalitySummary(model))
	}
	return &personalitymodel.ListPersonalityModelsResponse{
		Models:     models,
		Total:      out.Total,
		Page:       out.Page,
		PageSize:   out.PageSize,
		TotalPages: out.TotalPages,
	}
}

func toPersonalityModelCategoriesResponse(out *PersonalityModelCategoriesOutput) *personalitymodel.PersonalityModelCategoriesResponse {
	categories := make([]personalitymodel.CategoryResponse, 0, len(out.Categories))
	for _, item := range out.Categories {
		categories = append(categories, personalitymodel.CategoryResponse{Value: item.Value, Label: item.Label})
	}
	return &personalitymodel.PersonalityModelCategoriesResponse{Categories: categories}
}

func toPersonalitySummary(model PersonalityModelSummaryOutput) personalitymodel.PersonalityModelSummaryResponse {
	return personalitymodel.PersonalityModelSummaryResponse{
		Code:                 model.Code,
		Version:              model.Version,
		Title:                model.Title,
		Algorithm:            model.Algorithm,
		Description:          model.Description,
		QuestionnaireCode:    model.QuestionnaireCode,
		QuestionnaireVersion: model.QuestionnaireVersion,
		Status:               model.Status,
		QuestionCount:        model.QuestionCount,
	}
}

func toPersonalityModelResponse(model *PersonalityModelOutput) *personalitymodel.PersonalityModelResponse {
	dimensions := make([]personalitymodel.PersonalityDimensionResponse, 0, len(model.Dimensions))
	for _, dim := range model.Dimensions {
		dimensions = append(dimensions, personalitymodel.PersonalityDimensionResponse{
			Code:      dim.Code,
			Name:      dim.Name,
			LeftPole:  dim.LeftPole,
			RightPole: dim.RightPole,
		})
	}
	outcomes := make([]personalitymodel.PersonalityOutcomeResponse, 0, len(model.Outcomes))
	for _, outcome := range model.Outcomes {
		outcomes = append(outcomes, personalitymodel.PersonalityOutcomeResponse{
			Code:     outcome.Code,
			Name:     outcome.Name,
			OneLiner: outcome.OneLiner,
			ImageURL: outcome.ImageURL,
		})
	}
	summary := toPersonalitySummary(model.Summary)
	return &personalitymodel.PersonalityModelResponse{
		Code:                 summary.Code,
		Version:              summary.Version,
		Title:                summary.Title,
		Algorithm:            summary.Algorithm,
		Description:          summary.Description,
		QuestionnaireCode:    summary.QuestionnaireCode,
		QuestionnaireVersion: summary.QuestionnaireVersion,
		Status:               summary.Status,
		QuestionCount:        summary.QuestionCount,
		DimensionOrder:       append([]string(nil), model.DimensionOrder...),
		Dimensions:           dimensions,
		Outcomes:             outcomes,
	}
}
