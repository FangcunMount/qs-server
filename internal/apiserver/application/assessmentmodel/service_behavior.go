package assessmentmodel

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/assessmentmodel/behavior"
)

func (s *service) createBehaviorAbility(ctx context.Context, dto CreateModelDTO) (*ModelSummary, error) {
	cmd, err := s.behavior.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.Create(ctx, behavior.CreateInput{
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
	return summaryFromBehavior(result), nil
}

func (s *service) updateBehaviorBasicInfo(ctx context.Context, dto UpdateBasicInfoDTO) (*ModelSummary, error) {
	cmd, err := s.behavior.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.UpdateBasicInfo(ctx, behavior.UpdateBasicInfoInput{
		Code:        dto.Code,
		Title:       dto.Title,
		Description: dto.Description,
		Category:    dto.Category,
		Tags:        dto.Tags,
	})
	if err != nil {
		return nil, err
	}
	return summaryFromBehavior(result), nil
}

func (s *service) updateBehaviorDefinition(ctx context.Context, modelCode string, dto DefinitionDTO) (*DefinitionDTO, error) {
	cmd, err := s.behavior.require()
	if err != nil {
		return nil, err
	}
	result, err := cmd.UpdateDefinition(ctx, modelCode, behavior.DefinitionInput{Payload: dto.Payload})
	if err != nil {
		return nil, err
	}
	return definitionFromBehavior(result), nil
}

func (s *service) listBehaviorAbility(ctx context.Context, dto ListModelsDTO) (*ModelListResult, error) {
	if s.behavior.cmd == nil {
		return &ModelListResult{Page: dto.Page, PageSize: dto.PageSize}, nil
	}
	result, err := s.behavior.cmd.List(ctx, behavior.ListInput{
		Page:     dto.Page,
		PageSize: dto.PageSize,
		Status:   dto.Status,
		Keyword:  dto.Keyword,
		Category: dto.Category,
	})
	if err != nil {
		return nil, err
	}
	out := &ModelListResult{Page: dto.Page, PageSize: dto.PageSize}
	if result == nil {
		return out, nil
	}
	out.Total = result.Total
	for _, item := range result.Items {
		out.Items = append(out.Items, summaryFromBehaviorValue(item))
	}
	return out, nil
}

func (s *service) loadBehaviorAbility(ctx context.Context, modelCode string) (*behavior.Model, error) {
	if modelCode == "" {
		return nil, invalidArgument("模型编码不能为空")
	}
	cmd, err := s.behavior.require()
	if err != nil {
		return nil, err
	}
	return cmd.Get(ctx, modelCode)
}
