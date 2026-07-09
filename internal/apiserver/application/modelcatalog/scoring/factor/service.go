package factor

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/assessmentstore"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/ports"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalelistcache"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// factorService 量表因子编辑服务实现
// 行为者：量表因子编辑者
type factorService struct {
	modelRepo      modelcatalogport.ModelRepository
	listCache      scalelistcache.PublishedListCache
	eventPublisher event.EventPublisher
}

// NewService 创建量表因子编辑应用服务。
func NewService(modelRepo modelcatalogport.ModelRepository, listCache scalelistcache.PublishedListCache, eventPublisher event.EventPublisher) ports.ScaleFactorService {
	if modelRepo == nil {
		panic("factor: assessment model repository is required")
	}
	return &factorService{
		modelRepo:      modelRepo,
		listCache:      listCache,
		eventPublisher: eventPublisher,
	}
}

// AddFactor 添加因子
func (s *factorService) AddFactor(ctx context.Context, dto shared.AddFactorDTO) (*shared.ScaleResult, error) {
	if dto.ScaleCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if dto.Code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "因子编码不能为空")
	}
	if dto.Title == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "因子标题不能为空")
	}

	factor, err := toFactorDomain(dto.Code, dto.Title, dto.FactorType, dto.IsTotalScore, dto.IsShow,
		dto.QuestionCodes, dto.ScoringStrategy, dto.ScoringParams, dto.MaxScore, dto.InterpretRules)
	if err != nil {
		return nil, err
	}

	return s.mutateDefinition(ctx, dto.ScaleCode, "添加因子失败", func(model *domain.AssessmentModel) error {
		return assessmentstore.AddFactorSnapshot(model, factor)
	}, scaledefinition.ChangeActionUpdated)
}

// UpdateFactor 更新因子
func (s *factorService) UpdateFactor(ctx context.Context, dto shared.UpdateFactorDTO) (*shared.ScaleResult, error) {
	if dto.ScaleCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if dto.Code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "因子编码不能为空")
	}

	factor, err := toFactorDomain(dto.Code, dto.Title, dto.FactorType, dto.IsTotalScore, dto.IsShow,
		dto.QuestionCodes, dto.ScoringStrategy, dto.ScoringParams, dto.MaxScore, dto.InterpretRules)
	if err != nil {
		return nil, err
	}

	return s.mutateDefinition(ctx, dto.ScaleCode, "更新因子失败", func(model *domain.AssessmentModel) error {
		return assessmentstore.UpdateFactorSnapshot(model, factor)
	}, scaledefinition.ChangeActionUpdated)
}

// RemoveFactor 删除因子
func (s *factorService) RemoveFactor(ctx context.Context, scaleCode, factorCode string) (*shared.ScaleResult, error) {
	if scaleCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if factorCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "因子编码不能为空")
	}

	return s.mutateDefinition(ctx, scaleCode, "删除因子失败", func(model *domain.AssessmentModel) error {
		return assessmentstore.RemoveFactorSnapshot(model, factorCode)
	}, scaledefinition.ChangeActionUpdated)
}

// ReplaceFactors 替换所有因子
func (s *factorService) ReplaceFactors(ctx context.Context, scaleCode string, factorDTOs []shared.FactorDTO) (*shared.ScaleResult, error) {
	if scaleCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if len(factorDTOs) == 0 {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "因子列表不能为空")
	}

	factors := make([]*scaledefinition.Factor, 0, len(factorDTOs))
	var allValidationErrors []scaledefinition.ValidationError

	for _, dto := range factorDTOs {
		factor, err := toFactorDomain(dto.Code, dto.Title, dto.FactorType, dto.IsTotalScore, dto.IsShow,
			dto.QuestionCodes, dto.ScoringStrategy, dto.ScoringParams, dto.MaxScore, dto.InterpretRules)
		if err != nil {
			return nil, err
		}

		factorErrs := scaledefinition.ValidateFactor(factor)
		if len(factorErrs) > 0 {
			allValidationErrors = append(allValidationErrors, factorErrs...)
		}

		factors = append(factors, factor)
	}

	if len(allValidationErrors) > 0 {
		return nil, shared.WrapScaleDomainError(scaledefinition.ToError(allValidationErrors), errorCode.ErrInvalidArgument, "验证因子失败")
	}

	return s.mutateDefinition(ctx, scaleCode, "替换因子失败", func(model *domain.AssessmentModel) error {
		return assessmentstore.ReplaceFactorSnapshots(model, factors)
	}, scaledefinition.ChangeActionUpdated)
}

// UpdateFactorInterpretRules 更新因子解读规则
func (s *factorService) UpdateFactorInterpretRules(ctx context.Context, dto shared.UpdateFactorInterpretRulesDTO) (*shared.ScaleResult, error) {
	if dto.ScaleCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if dto.FactorCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "因子编码不能为空")
	}

	rules := shared.InterpretRulesFromDTOs(dto.InterpretRules)

	return s.mutateDefinition(ctx, dto.ScaleCode, "更新解读规则失败", func(model *domain.AssessmentModel) error {
		return assessmentstore.UpdateFactorInterpretRulesSnapshot(model, dto.FactorCode, rules)
	}, scaledefinition.ChangeActionUpdated)
}

// ReplaceInterpretRules 批量设置所有因子的解读规则
func (s *factorService) ReplaceInterpretRules(ctx context.Context, scaleCode string, dtos []shared.UpdateFactorInterpretRulesDTO) (*shared.ScaleResult, error) {
	if scaleCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if len(dtos) == 0 {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "因子解读规则列表不能为空")
	}

	for _, dto := range dtos {
		if dto.FactorCode == "" {
			return nil, errors.WithCode(errorCode.ErrInvalidArgument, "因子编码不能为空")
		}
	}

	return s.mutateDefinition(ctx, scaleCode, "更新解读规则失败", func(model *domain.AssessmentModel) error {
		for _, dto := range dtos {
			rules := shared.InterpretRulesFromDTOs(dto.InterpretRules)
			if err := assessmentstore.UpdateFactorInterpretRulesSnapshot(model, dto.FactorCode, rules); err != nil {
				return err
			}
		}
		return nil
	}, scaledefinition.ChangeActionUpdated)
}

type definitionMutation func(model *domain.AssessmentModel) error

func (s *factorService) mutateDefinition(ctx context.Context, scaleCode, failureMessage string, mutate definitionMutation, action scaledefinition.ChangeAction) (*shared.ScaleResult, error) {
	model, err := assessmentstore.LoadScale(ctx, s.modelRepo, scaleCode)
	if err != nil {
		return nil, err
	}
	if err := assessmentstore.EnsureHeadEditable(ctx, s.modelRepo, model); err != nil {
		return nil, err
	}
	if err := mutate(model); err != nil {
		return nil, shared.WrapScaleDomainError(err, errorCode.ErrInvalidArgument, "%s", failureMessage)
	}
	if err := assessmentstore.SaveScale(ctx, s.modelRepo, model); err != nil {
		return nil, err
	}

	if evt, ok := assessmentstore.ScaleChangedEvent(model, action); ok {
		eventing.PublishCollectedEvents(ctx, s.eventPublisher, eventing.Collect(evt), nil, nil)
	}
	s.refreshListCache(ctx)
	return assessmentstore.ScaleResult(model)
}

func (s *factorService) refreshListCache(ctx context.Context) {
	if s.listCache == nil {
		return
	}
	shared.LogScaleListCacheError(ctx, s.listCache.Rebuild(ctx))
}
