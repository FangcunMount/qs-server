package modelcatalog

import (
	"context"

	appCognitive "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/cognitive"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

type taskPerformanceKindGateway struct {
	cmd appCognitive.Service
}

func (g taskPerformanceKindGateway) require() (appCognitive.Service, error) {
	if g.cmd == nil {
		return nil, invalidArgument("认知模型服务未配置")
	}
	return g.cmd, nil
}

func (s *service) createCognitive(ctx context.Context, dto CreateModelDTO) (*ModelSummary, error) {
	cmd, err := s.taskPerformanceKind.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.Create(ctx, appCognitive.CreateInput{
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
	return cognitiveSummaryFromResult(result), nil
}

func (g taskPerformanceKindGateway) updateBasicInfo(ctx context.Context, dto UpdateBasicInfoDTO) (*ModelSummary, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.UpdateBasicInfo(ctx, appCognitive.UpdateBasicInfoInput{
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
	return cognitiveSummaryFromResult(result), nil
}

func (g taskPerformanceKindGateway) delete(ctx context.Context, modelCode string) error {
	cmd, err := g.require()
	if err != nil {
		return err
	}
	return cmd.Delete(ctx, modelCode)
}

func (g taskPerformanceKindGateway) publish(ctx context.Context, modelCode string) (*ModelSummary, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.Publish(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return cognitiveSummaryFromResult(result), nil
}

func (g taskPerformanceKindGateway) unpublish(ctx context.Context, modelCode string) (*ModelSummary, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.Unpublish(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return cognitiveSummaryFromResult(result), nil
}

func (g taskPerformanceKindGateway) archive(ctx context.Context, modelCode string) (*ModelSummary, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.Archive(ctx, modelCode)
	if err != nil {
		return nil, err
	}
	return cognitiveSummaryFromResult(result), nil
}

func (g taskPerformanceKindGateway) bindQuestionnaire(ctx context.Context, dto BindQuestionnaireDTO) (*QuestionnaireBindingResult, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.BindQuestionnaire(ctx, appCognitive.BindQuestionnaireInput{
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

func (g taskPerformanceKindGateway) getDefinition(ctx context.Context, modelCode string) (*DefinitionDTO, error) {
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
	populateDefinitionIdentity(dto, domain.KindCognitive, domain.SubKindEmpty, domain.Algorithm(result.Algorithm), domain.ProductChannel(result.ProductChannel))
	return dto, nil
}

func (g taskPerformanceKindGateway) updateDefinition(ctx context.Context, modelCode string, dto DefinitionDTO) (*DefinitionDTO, error) {
	cmd, err := g.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.UpdateDefinition(ctx, modelCode, appCognitive.DefinitionInput{Payload: dto.Payload})
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
	populateDefinitionIdentity(out, domain.KindCognitive, domain.SubKindEmpty, domain.Algorithm(result.Algorithm), domain.ProductChannel(result.ProductChannel))
	return out, nil
}

func cognitiveSummaryFromResult(result *appCognitive.ModelSummary) *ModelSummary {
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
	populateModelSummaryIdentity(summary, domain.KindCognitive, domain.SubKindEmpty, domain.Algorithm(result.Algorithm), domain.ProductChannel(result.ProductChannel))
	return summary
}

func (s *service) listCognitive(ctx context.Context, dto ListModelsDTO) (*ModelListResult, error) {
	cmd, err := s.taskPerformanceKind.require()
	if err != nil {
		return &ModelListResult{Page: dto.Page, PageSize: dto.PageSize}, nil
	}
	result, err := cmd.List(ctx, appCognitive.ListInput{
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
		out.Items = append(out.Items, *cognitiveSummaryFromResult(&item))
	}
	return out, nil
}
