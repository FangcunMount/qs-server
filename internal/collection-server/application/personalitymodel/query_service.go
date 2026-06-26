package personalitymodel

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/log"
	"github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
	"github.com/FangcunMount/qs-server/internal/pkg/cancelerr"
)

// QueryService is the BFF layer for personality model catalog reads.
type QueryService struct {
	client *grpcclient.PersonalityModelClient
}

func NewQueryService(client *grpcclient.PersonalityModelClient) *QueryService {
	return &QueryService{client: client}
}

func (s *QueryService) Get(ctx context.Context, code string) (*PersonalityModelResponse, error) {
	log.Infof("Getting personality model: code=%s", code)
	result, err := s.client.GetPersonalityModel(ctx, code)
	if err != nil {
		logPersonalityGRPCError("Failed to get personality model via gRPC", err)
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return convertDetail(result), nil
}

func (s *QueryService) List(ctx context.Context, req *ListPersonalityModelsRequest) (*ListPersonalityModelsResponse, error) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 20
	}
	if req.PageSize > 100 {
		req.PageSize = 100
	}
	result, err := s.client.ListPersonalityModels(ctx, req.Page, req.PageSize, req.Algorithm)
	if err != nil {
		logPersonalityGRPCError("Failed to list personality models via gRPC", err)
		return nil, err
	}
	models := make([]PersonalityModelSummaryResponse, 0, len(result.Models))
	for _, model := range result.Models {
		models = append(models, convertSummary(model))
	}
	return &ListPersonalityModelsResponse{
		Models:     models,
		Total:      result.Total,
		Page:       result.Page,
		PageSize:   result.PageSize,
		TotalPages: result.TotalPages,
	}, nil
}

func (s *QueryService) GetCategories(ctx context.Context) (*PersonalityModelCategoriesResponse, error) {
	result, err := s.client.GetPersonalityModelCategories(ctx)
	if err != nil {
		logPersonalityGRPCError("Failed to get personality model categories via gRPC", err)
		return nil, err
	}
	categories := make([]CategoryResponse, 0, len(result.Categories))
	for _, item := range result.Categories {
		categories = append(categories, CategoryResponse{Value: item.Value, Label: item.Label})
	}
	return &PersonalityModelCategoriesResponse{Categories: categories}, nil
}

func convertSummary(model grpcclient.PersonalityModelSummaryOutput) PersonalityModelSummaryResponse {
	return PersonalityModelSummaryResponse{
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

func convertDetail(model *grpcclient.PersonalityModelOutput) *PersonalityModelResponse {
	dimensions := make([]PersonalityDimensionResponse, 0, len(model.Dimensions))
	for _, dim := range model.Dimensions {
		dimensions = append(dimensions, PersonalityDimensionResponse{
			Code:      dim.Code,
			Name:      dim.Name,
			LeftPole:  dim.LeftPole,
			RightPole: dim.RightPole,
		})
	}
	outcomes := make([]PersonalityOutcomeResponse, 0, len(model.Outcomes))
	for _, outcome := range model.Outcomes {
		outcomes = append(outcomes, PersonalityOutcomeResponse{
			Code:     outcome.Code,
			Name:     outcome.Name,
			OneLiner: outcome.OneLiner,
			ImageURL: outcome.ImageURL,
		})
	}
	summary := convertSummary(model.Summary)
	return &PersonalityModelResponse{
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

func logPersonalityGRPCError(message string, err error) {
	if cancelerr.Is(err) {
		log.Debugf("%s: %v", message, err)
		return
	}
	log.Errorf("%s: %v", message, err)
}
