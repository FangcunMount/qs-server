package modelcatalog

import (
	"context"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func (s *service) listBehaviorAbilityChannel(ctx context.Context, dto ListModelsDTO) (*ModelListResult, error) {
	if family, ok := domain.ResolveBehaviorAbilityChannelFamily(dto.ModelFamily); ok {
		return s.listBehaviorAbilityChannelFamily(ctx, dto, family)
	}

	result := &ModelListResult{Page: dto.Page, PageSize: dto.PageSize}
	legacy, err := s.listBehaviorAbility(ctx, dto)
	if err != nil {
		return nil, err
	}
	result.Items = append(result.Items, legacy.Items...)
	result.Total += legacy.Total

	rating, err := s.listBehavioralRating(ctx, dto)
	if err != nil {
		return nil, err
	}
	result.Items = append(result.Items, rating.Items...)
	result.Total += rating.Total

	cognitive, err := s.listCognitive(ctx, dto)
	if err != nil {
		return nil, err
	}
	result.Items = append(result.Items, cognitive.Items...)
	result.Total += cognitive.Total
	return result, nil
}

func (s *service) listBehaviorAbilityChannelFamily(ctx context.Context, dto ListModelsDTO, family domain.Kind) (*ModelListResult, error) {
	switch family {
	case domain.KindBehaviorAbility: //nolint:staticcheck // SA1019: behavior_ability legacy product-channel compatibility
		return s.listBehaviorAbility(ctx, dto)
	case domain.KindBehavioralRating:
		return s.listBehavioralRating(ctx, dto)
	case domain.KindCognitive:
		return s.listCognitive(ctx, dto)
	default:
		return nil, invalidArgument("行为能力频道模型族无效")
	}
}

func behaviorAbilityChannelModelFamilyOptions() []Option {
	return []Option{
		{Label: "行为评定", Value: string(domain.KindBehavioralRating)},
		{Label: "认知能力", Value: string(domain.KindCognitive)},
		{Label: "legacy scale adapter", Value: string(domain.KindBehaviorAbility)}, //nolint:staticcheck // SA1019: behavior_ability legacy product-channel compatibility
	}
}
