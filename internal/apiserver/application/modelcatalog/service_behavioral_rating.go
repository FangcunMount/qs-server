package modelcatalog

import (
	"context"

	appBehavioralRating "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/behavioral_rating"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

type behavioralRatingGateway struct {
	cmd appBehavioralRating.Service
}

func (g behavioralRatingGateway) require() (appBehavioralRating.Service, error) {
	if g.cmd == nil {
		return nil, invalidArgument("行为评定模型服务未配置")
	}
	return g.cmd, nil
}

func (s *service) createBehavioralRating(ctx context.Context, dto CreateModelDTO) (*ModelSummary, error) {
	cmd, err := s.behavioralRating.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.Create(ctx, appBehavioralRating.CreateInput{
		Code:                 dto.Code,
		Title:                dto.Title,
		Description:          dto.Description,
		Category:             dto.Category,
		Tags:                 dto.Tags,
		QuestionnaireCode:    dto.QuestionnaireCode,
		QuestionnaireVersion: dto.QuestionnaireVersion,
	})
	if err != nil {
		return nil, err
	}
	return behavioralRatingSummaryFromResult(result), nil
}

func (g behavioralRatingGateway) updateBasicInfo(ctx context.Context, dto UpdateBasicInfoDTO) (*ModelSummary, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.UpdateBasicInfo(ctx, appBehavioralRating.UpdateBasicInfoInput{
		Code:        dto.Code,
		Title:       dto.Title,
		Description: dto.Description,
		Category:    dto.Category,
		Tags:        dto.Tags,
	})
	if err != nil {
		return nil, err
	}
	return behavioralRatingSummaryFromResult(result), nil
}

func (g behavioralRatingGateway) delete(ctx context.Context, modelCode string) error {
	cmd, err := g.require()
	if err != nil {
		return err
	}
	return cmd.Delete(ctx, modelCode)
}

func (g behavioralRatingGateway) publish(ctx context.Context, modelCode string) (*ModelSummary, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.Publish(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return behavioralRatingSummaryFromResult(result), nil
}

func (g behavioralRatingGateway) unpublish(ctx context.Context, modelCode string) (*ModelSummary, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.Unpublish(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return behavioralRatingSummaryFromResult(result), nil
}

func (g behavioralRatingGateway) archive(ctx context.Context, modelCode string) (*ModelSummary, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.Archive(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return behavioralRatingSummaryFromResult(result), nil
}

func (g behavioralRatingGateway) bindQuestionnaire(ctx context.Context, dto BindQuestionnaireDTO) (*QuestionnaireBindingResult, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.BindQuestionnaire(ctx, appBehavioralRating.BindQuestionnaireInput{
		Code:                 dto.Code,
		QuestionnaireCode:    dto.QuestionnaireCode,
		QuestionnaireVersion: dto.QuestionnaireVersion,
	})
	if err != nil {
		return nil, err
	}
	return &QuestionnaireBindingResult{
		QuestionnaireCode:    result.QuestionnaireCode,
		QuestionnaireVersion: result.QuestionnaireVersion,
	}, nil
}

func (g behavioralRatingGateway) getDefinition(ctx context.Context, modelCode string) (*DefinitionDTO, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.GetDefinition(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return &DefinitionDTO{
		Kind:          KindBehavioralRating,
		Algorithm:     string(domain.AlgorithmBrief2),
		PayloadFormat: result.PayloadFormat,
		Payload:       result.Payload,
	}, nil
}

func (g behavioralRatingGateway) updateDefinition(ctx context.Context, modelCode string, dto DefinitionDTO) (*DefinitionDTO, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.UpdateDefinition(ctx, modelCode, appBehavioralRating.DefinitionInput{Payload: dto.Payload})
	if err != nil {
		return nil, err
	}
	return &DefinitionDTO{
		Kind:          KindBehavioralRating,
		Algorithm:     string(domain.AlgorithmBrief2),
		PayloadFormat: result.PayloadFormat,
		Payload:       result.Payload,
	}, nil
}

func behavioralRatingSummaryFromResult(result *appBehavioralRating.ModelSummary) *ModelSummary {
	if result == nil {
		return nil
	}
	return &ModelSummary{
		Code:                 result.Code,
		Kind:                 result.Kind,
		Algorithm:            result.Algorithm,
		Title:                result.Title,
		Description:          result.Description,
		Status:               result.Status,
		Category:             result.Category,
		Tags:                 result.Tags,
		QuestionnaireCode:    result.QuestionnaireCode,
		QuestionnaireVersion: result.QuestionnaireVersion,
		CreatedAt:            result.CreatedAt,
		UpdatedAt:            result.UpdatedAt,
	}
}

func (s *service) listBehavioralRating(ctx context.Context, dto ListModelsDTO) (*ModelListResult, error) {
	cmd, err := s.behavioralRating.require()
	if err != nil {
		return &ModelListResult{Page: dto.Page, PageSize: dto.PageSize}, nil
	}
	result, err := cmd.List(ctx, appBehavioralRating.ListInput{
		Status:   dto.Status,
		Keyword:  dto.Keyword,
		Page:     dto.Page,
		PageSize: dto.PageSize,
	})
	if err != nil {
		return nil, err
	}
	out := &ModelListResult{Page: dto.Page, PageSize: dto.PageSize, Total: result.Total}
	for _, item := range result.Items {
		out.Items = append(out.Items, *behavioralRatingSummaryFromResult(&item))
	}
	return out, nil
}
