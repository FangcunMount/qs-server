package modelcatalog

import (
	"context"

	appNorming "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/norming"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

type normingKindGateway struct {
	cmd appNorming.Service
}

func (g normingKindGateway) require() (appNorming.Service, error) {
	if g.cmd == nil {
		return nil, invalidArgument("行为评定模型服务未配置")
	}
	return g.cmd, nil
}

func (s *service) createNormingModel(ctx context.Context, dto CreateModelDTO) (*ModelSummary, error) {
	cmd, err := s.normingKind.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.Create(ctx, appNorming.CreateInput{
		Code:                 dto.Code,
		Title:                dto.Title,
		Description:          dto.Description,
		ProductChannel:       dto.ProductChannel,
		Category:             dto.Category,
		Tags:                 dto.Tags,
		QuestionnaireCode:    dto.QuestionnaireCode,
		QuestionnaireVersion: dto.QuestionnaireVersion,
	})
	if err != nil {
		return nil, err
	}
	return normingSummaryFromResult(result), nil
}

func (g normingKindGateway) updateBasicInfo(ctx context.Context, dto UpdateBasicInfoDTO) (*ModelSummary, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.UpdateBasicInfo(ctx, appNorming.UpdateBasicInfoInput{
		Code:           dto.Code,
		Title:          dto.Title,
		Description:    dto.Description,
		ProductChannel: dto.ProductChannel,
		Category:       dto.Category,
		Tags:           dto.Tags,
	})
	if err != nil {
		return nil, err
	}
	return normingSummaryFromResult(result), nil
}

func (g normingKindGateway) delete(ctx context.Context, modelCode string) error {
	cmd, err := g.require()
	if err != nil {
		return err
	}
	return cmd.Delete(ctx, modelCode)
}

func (g normingKindGateway) publish(ctx context.Context, modelCode string) (*ModelSummary, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.Publish(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return normingSummaryFromResult(result), nil
}

func (g normingKindGateway) unpublish(ctx context.Context, modelCode string) (*ModelSummary, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.Unpublish(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return normingSummaryFromResult(result), nil
}

func (g normingKindGateway) archive(ctx context.Context, modelCode string) (*ModelSummary, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.Archive(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return normingSummaryFromResult(result), nil
}

func (g normingKindGateway) bindQuestionnaire(ctx context.Context, dto BindQuestionnaireDTO) (*QuestionnaireBindingResult, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.BindQuestionnaire(ctx, appNorming.BindQuestionnaireInput{
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

func (g normingKindGateway) getDefinition(ctx context.Context, modelCode string) (*DefinitionDTO, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.GetDefinition(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	dto := &DefinitionDTO{
		Kind:           result.Kind,
		Algorithm:      result.Algorithm,
		ProductChannel: result.ProductChannel,
		PayloadFormat:  result.PayloadFormat,
		Payload:        result.Payload,
	}
	populateDefinitionIdentity(dto, domain.KindBehavioralRating, domain.SubKindEmpty, domain.Algorithm(result.Algorithm), domain.ProductChannel(result.ProductChannel))
	return dto, nil
}

func (g normingKindGateway) updateDefinition(ctx context.Context, modelCode string, dto DefinitionDTO) (*DefinitionDTO, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.UpdateDefinition(ctx, modelCode, appNorming.DefinitionInput{Payload: dto.Payload})
	if err != nil {
		return nil, err
	}
	out := &DefinitionDTO{
		Kind:           result.Kind,
		Algorithm:      result.Algorithm,
		ProductChannel: result.ProductChannel,
		PayloadFormat:  result.PayloadFormat,
		Payload:        result.Payload,
	}
	populateDefinitionIdentity(out, domain.KindBehavioralRating, domain.SubKindEmpty, domain.Algorithm(result.Algorithm), domain.ProductChannel(result.ProductChannel))
	return out, nil
}

func normingSummaryFromResult(result *appNorming.ModelSummary) *ModelSummary {
	if result == nil {
		return nil
	}
	summary := &ModelSummary{
		Code:                 result.Code,
		Kind:                 result.Kind,
		Algorithm:            result.Algorithm,
		ProductChannel:       result.ProductChannel,
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
	populateModelSummaryIdentity(summary, domain.KindBehavioralRating, domain.SubKindEmpty, domain.Algorithm(result.Algorithm), domain.ProductChannel(result.ProductChannel))
	return summary
}

func (s *service) listNormingModels(ctx context.Context, dto ListModelsDTO) (*ModelListResult, error) {
	cmd, err := s.normingKind.require()
	if err != nil {
		return &ModelListResult{Page: dto.Page, PageSize: dto.PageSize}, nil
	}
	result, err := cmd.List(ctx, appNorming.ListInput{
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
		out.Items = append(out.Items, *normingSummaryFromResult(&item))
	}
	return out, nil
}
