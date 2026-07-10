package factor

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/authoring"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/assessmentstore"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/ports"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalelistcache"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/eventpayload"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// factorService 量表因子编辑服务实现
// 行为者：量表因子编辑者
type factorService struct {
	modelRepo      modelcatalogport.ModelRepository
	listCache      scalelistcache.PublishedListCache
	eventPublisher event.EventPublisher
	authoring      *authoring.Service
}

type ServiceOption func(*factorService)

func WithDefinitionAuthoring(service authoring.Service) ServiceOption {
	return func(s *factorService) { s.authoring = &service }
}

// NewService 创建量表因子编辑应用服务。
func NewService(modelRepo modelcatalogport.ModelRepository, listCache scalelistcache.PublishedListCache, eventPublisher event.EventPublisher, opts ...ServiceOption) ports.ScaleFactorService {
	if modelRepo == nil {
		panic("factor: assessment model repository is required")
	}
	service := &factorService{
		modelRepo:      modelRepo,
		listCache:      listCache,
		eventPublisher: eventPublisher,
	}
	for _, opt := range opts {
		opt(service)
	}
	return service
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

	factor, err := toFactorSnapshot(dto.Code, dto.Title, dto.FactorType, dto.IsTotalScore, dto.IsShow,
		dto.QuestionCodes, dto.ScoringStrategy, dto.ScoringParams, dto.MaxScore, dto.InterpretRules)
	if err != nil {
		return nil, err
	}

	return s.mutateDefinition(ctx, dto.ScaleCode, "添加因子失败", func(model *domain.AssessmentModel) error {
		return assessmentstore.AddFactorSnapshot(model, factor)
	}, eventpayload.ScaleChangeActionUpdated)
}

// UpdateFactor 更新因子
func (s *factorService) UpdateFactor(ctx context.Context, dto shared.UpdateFactorDTO) (*shared.ScaleResult, error) {
	if dto.ScaleCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if dto.Code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "因子编码不能为空")
	}

	factor, err := toFactorSnapshot(dto.Code, dto.Title, dto.FactorType, dto.IsTotalScore, dto.IsShow,
		dto.QuestionCodes, dto.ScoringStrategy, dto.ScoringParams, dto.MaxScore, dto.InterpretRules)
	if err != nil {
		return nil, err
	}

	return s.mutateDefinition(ctx, dto.ScaleCode, "更新因子失败", func(model *domain.AssessmentModel) error {
		return assessmentstore.UpdateFactorSnapshot(model, factor)
	}, eventpayload.ScaleChangeActionUpdated)
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
	}, eventpayload.ScaleChangeActionUpdated)
}

// ReplaceFactors 替换所有因子
func (s *factorService) ReplaceFactors(ctx context.Context, scaleCode string, factorDTOs []shared.FactorDTO) (*shared.ScaleResult, error) {
	if scaleCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if len(factorDTOs) == 0 {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "因子列表不能为空")
	}

	factors := make([]scalesnapshot.FactorSnapshot, 0, len(factorDTOs))

	for _, dto := range factorDTOs {
		factor, err := toFactorSnapshot(dto.Code, dto.Title, dto.FactorType, dto.IsTotalScore, dto.IsShow,
			dto.QuestionCodes, dto.ScoringStrategy, dto.ScoringParams, dto.MaxScore, dto.InterpretRules)
		if err != nil {
			return nil, err
		}

		if err := validateFactorSnapshotForReplacement(factor); err != nil {
			return nil, shared.WrapScaleDomainError(err, errorCode.ErrInvalidArgument, "验证因子失败")
		}

		factors = append(factors, factor)
	}

	return s.mutateDefinition(ctx, scaleCode, "替换因子失败", func(model *domain.AssessmentModel) error {
		return assessmentstore.ReplaceFactorSnapshots(model, factors)
	}, eventpayload.ScaleChangeActionUpdated)
}

// ReplaceFactorsWithActor is the DefinitionV2-first replacement path used by
// the REST batch editor. Legacy snapshot mutation remains only for endpoints
// that have not yet been migrated to ActorContext.
func (s *factorService) ReplaceFactorsWithActor(ctx context.Context, actor modelcatalog.ActorContext, scaleCode string, factorDTOs []shared.FactorDTO) (*shared.ScaleResult, error) {
	if s.authoring == nil {
		return nil, errors.WithCode(errorCode.ErrInternalServerError, "definition authoring service is not configured")
	}
	definition, err := definitionFromFactorDTOs(factorDTOs)
	if err != nil {
		return nil, err
	}
	if _, err := s.authoring.SaveDefinition(ctx, actor, scaleCode, definition); err != nil {
		return nil, err
	}
	model, err := assessmentstore.LoadScale(ctx, s.modelRepo, scaleCode)
	if err != nil {
		return nil, err
	}
	if evt, ok := assessmentstore.ScaleChangedEvent(model, eventpayload.ScaleChangeActionUpdated); ok {
		eventing.PublishCollectedEvents(ctx, s.eventPublisher, eventing.Collect(evt), nil, nil)
	}
	s.refreshListCache(ctx)
	return assessmentstore.ScaleResult(model)
}

// UpdateFactorInterpretRules 更新因子解读规则
func (s *factorService) UpdateFactorInterpretRules(ctx context.Context, dto shared.UpdateFactorInterpretRulesDTO) (*shared.ScaleResult, error) {
	if dto.ScaleCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if dto.FactorCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "因子编码不能为空")
	}

	rules := interpretRuleSnapshotsFromDTOsInOrder(dto.InterpretRules)

	return s.mutateDefinition(ctx, dto.ScaleCode, "更新解读规则失败", func(model *domain.AssessmentModel) error {
		return assessmentstore.UpdateFactorInterpretRulesSnapshot(model, dto.FactorCode, rules)
	}, eventpayload.ScaleChangeActionUpdated)
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
			rules := interpretRuleSnapshotsFromDTOsInOrder(dto.InterpretRules)
			if err := assessmentstore.UpdateFactorInterpretRulesSnapshot(model, dto.FactorCode, rules); err != nil {
				return err
			}
		}
		return nil
	}, eventpayload.ScaleChangeActionUpdated)
}

func (s *factorService) ReplaceInterpretRulesWithActor(ctx context.Context, actor modelcatalog.ActorContext, scaleCode string, dtos []shared.UpdateFactorInterpretRulesDTO) (*shared.ScaleResult, error) {
	if s.authoring == nil {
		return nil, errors.WithCode(errorCode.ErrInternalServerError, "definition authoring service is not configured")
	}
	current, err := s.authoring.GetDefinition(ctx, actor, scaleCode)
	if err != nil {
		return nil, err
	}
	definition, err := definitionWithInterpretRules(current, dtos)
	if err != nil {
		return nil, shared.WrapScaleDomainError(err, errorCode.ErrInvalidArgument, "更新因子解读规则失败")
	}
	if _, err := s.authoring.SaveDefinition(ctx, actor, scaleCode, definition); err != nil {
		return nil, err
	}
	model, err := assessmentstore.LoadScale(ctx, s.modelRepo, scaleCode)
	if err != nil {
		return nil, err
	}
	if evt, ok := assessmentstore.ScaleChangedEvent(model, eventpayload.ScaleChangeActionUpdated); ok {
		eventing.PublishCollectedEvents(ctx, s.eventPublisher, eventing.Collect(evt), nil, nil)
	}
	s.refreshListCache(ctx)
	return assessmentstore.ScaleResult(model)
}

type definitionMutation func(model *domain.AssessmentModel) error

func (s *factorService) mutateDefinition(ctx context.Context, scaleCode, failureMessage string, mutate definitionMutation, action eventpayload.ScaleChangeAction) (*shared.ScaleResult, error) {
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
