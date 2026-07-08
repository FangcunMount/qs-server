package grpcbridge

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/typologymodel"
)

// TypologyCatalogReader 将 infra gRPC 输出转换为 application DTO。
type TypologyCatalogReader struct {
	inner TypologyModelReader
}

func NewTypologyCatalogReader(inner TypologyModelReader) *TypologyCatalogReader {
	return &TypologyCatalogReader{inner: inner}
}

func (r *TypologyCatalogReader) GetTypologyModel(ctx context.Context, code string) (*typologymodel.TypologyModelResponse, error) {
	if r == nil {
		return nil, nil
	}
	return CallBridge(r.inner,
		func() (*TypologyModelOutput, error) { return r.inner.GetTypologyModel(ctx, code) },
		toTypologyModelResponse,
	)
}

func (r *TypologyCatalogReader) ListTypologyModels(ctx context.Context, page, pageSize int32, algorithm string) (*typologymodel.ListTypologyModelsResponse, error) {
	if r == nil {
		return nil, nil
	}
	return CallBridge(r.inner,
		func() (*ListTypologyModelsOutput, error) {
			return r.inner.ListTypologyModels(ctx, page, pageSize, algorithm)
		},
		toListTypologyModelsResponse,
	)
}

func (r *TypologyCatalogReader) GetTypologyModelCategories(ctx context.Context) (*typologymodel.TypologyModelCategoriesResponse, error) {
	if r == nil {
		return nil, nil
	}
	return CallBridge(r.inner,
		func() (*TypologyModelCategoriesOutput, error) { return r.inner.GetTypologyModelCategories(ctx) },
		toTypologyModelCategoriesResponse,
	)
}

func toListTypologyModelsResponse(out *ListTypologyModelsOutput) *typologymodel.ListTypologyModelsResponse {
	models := make([]typologymodel.TypologyModelSummaryResponse, 0, len(out.Models))
	for _, model := range out.Models {
		models = append(models, toTypologySummary(model))
	}
	return &typologymodel.ListTypologyModelsResponse{
		Models:     models,
		Total:      out.Total,
		Page:       out.Page,
		PageSize:   out.PageSize,
		TotalPages: out.TotalPages,
	}
}

func toTypologyModelCategoriesResponse(out *TypologyModelCategoriesOutput) *typologymodel.TypologyModelCategoriesResponse {
	categories := make([]typologymodel.TypologyCategoryResponse, 0, len(out.Categories))
	for _, item := range out.Categories {
		categories = append(categories, typologymodel.TypologyCategoryResponse{Value: item.Value, Label: item.Label})
	}
	return &typologymodel.TypologyModelCategoriesResponse{Categories: categories}
}

func toTypologySummary(model TypologyModelSummaryOutput) typologymodel.TypologyModelSummaryResponse {
	return typologymodel.TypologyModelSummaryResponse{
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

func toTypologyModelResponse(model *TypologyModelOutput) *typologymodel.TypologyModelResponse {
	dimensions := make([]typologymodel.TypologyDimensionResponse, 0, len(model.Dimensions))
	for _, dim := range model.Dimensions {
		dimensions = append(dimensions, typologymodel.TypologyDimensionResponse{
			Code:      dim.Code,
			Name:      dim.Name,
			LeftPole:  dim.LeftPole,
			RightPole: dim.RightPole,
		})
	}
	outcomes := make([]typologymodel.TypologyOutcomeResponse, 0, len(model.Outcomes))
	for _, outcome := range model.Outcomes {
		outcomes = append(outcomes, typologymodel.TypologyOutcomeResponse{
			Code:     outcome.Code,
			Name:     outcome.Name,
			OneLiner: outcome.OneLiner,
			ImageURL: outcome.ImageURL,
		})
	}
	summary := toTypologySummary(model.Summary)
	return &typologymodel.TypologyModelResponse{
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
