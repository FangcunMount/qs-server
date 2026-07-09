package factor

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/assessmentstore"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/editable"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/legacyadapter"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/ports"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/scoring/shared"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scoring/definition"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalelistcache"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// factorService 量表因子编辑服务实现
// 行为者：量表因子编辑者
type factorService struct {
	repo           factorRepository
	modelRepo      modelcatalogport.ModelRepository
	listCache      scalelistcache.PublishedListCache
	eventPublisher event.EventPublisher
}

type factorRepository interface {
	CreatePublishedSnapshot(ctx context.Context, scale *scaledefinition.MedicalScale, active bool) error
	FindByCode(ctx context.Context, code string) (*scaledefinition.MedicalScale, error)
	Update(ctx context.Context, scale *scaledefinition.MedicalScale) error
}

// ServiceOption configures factor authoring collaborators.
type ServiceOption func(*factorService)

// WithAssessmentModelRepository injects the target AssessmentModel draft repository.
func WithAssessmentModelRepository(repo modelcatalogport.ModelRepository) ServiceOption {
	return func(s *factorService) {
		s.modelRepo = repo
	}
}

// NewService 创建量表因子编辑应用服务。
func NewService(repo factorRepository, listCache scalelistcache.PublishedListCache, eventPublisher event.EventPublisher, opts ...ServiceOption) ports.ScaleFactorService {
	service := &factorService{
		repo:           repo,
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

	factor, err := toFactorDomain(dto.Code, dto.Title, dto.FactorType, dto.IsTotalScore, dto.IsShow,
		dto.QuestionCodes, dto.ScoringStrategy, dto.ScoringParams, dto.MaxScore, dto.InterpretRules)
	if err != nil {
		return nil, err
	}

	return s.mutateEditableScale(ctx, dto.ScaleCode, "添加因子失败", func(scale *scaledefinition.MedicalScale) error {
		return scale.AddFactor(factor)
	})
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

	return s.mutateEditableScale(ctx, dto.ScaleCode, "更新因子失败", func(scale *scaledefinition.MedicalScale) error {
		return scale.UpdateFactor(factor)
	})
}

// RemoveFactor 删除因子
func (s *factorService) RemoveFactor(ctx context.Context, scaleCode, factorCode string) (*shared.ScaleResult, error) {
	if scaleCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if factorCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "因子编码不能为空")
	}

	return s.mutateEditableScale(ctx, scaleCode, "删除因子失败", func(scale *scaledefinition.MedicalScale) error {
		return scale.RemoveFactor(scaledefinition.NewFactorCode(factorCode))
	})
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

	return s.mutateEditableScale(ctx, scaleCode, "替换因子失败", func(scale *scaledefinition.MedicalScale) error {
		return scale.ReplaceFactors(factors)
	})
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

	return s.mutateEditableScale(ctx, dto.ScaleCode, "更新解读规则失败", func(scale *scaledefinition.MedicalScale) error {
		return scale.UpdateFactorInterpretRules(scaledefinition.NewFactorCode(dto.FactorCode), rules)
	})
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

	return s.mutateEditableScale(ctx, scaleCode, "更新解读规则失败", func(scale *scaledefinition.MedicalScale) error {
		for _, dto := range dtos {
			rules := shared.InterpretRulesFromDTOs(dto.InterpretRules)
			if err := scale.UpdateFactorInterpretRules(scaledefinition.NewFactorCode(dto.FactorCode), rules); err != nil {
				return err
			}
		}
		return nil
	})
}

type scaleMutation func(scale *scaledefinition.MedicalScale) error

func (s *factorService) usesAssessmentModelStore() bool {
	return s != nil && s.modelRepo != nil
}

func (s *factorService) mutateEditableScale(ctx context.Context, scaleCode, failureMessage string, mutate scaleMutation) (*shared.ScaleResult, error) {
	if s.usesAssessmentModelStore() {
		return s.mutateEditableAssessmentModel(ctx, scaleCode, failureMessage, mutate)
	}

	scale, err := s.loadEditableScale(ctx, scaleCode)
	if err != nil {
		return nil, err
	}
	if err := mutate(scale); err != nil {
		return nil, shared.WrapScaleDomainError(err, errorCode.ErrInvalidArgument, "%s", failureMessage)
	}
	return s.persistLegacyFactorMutation(ctx, scale)
}

func (s *factorService) mutateEditableAssessmentModel(ctx context.Context, scaleCode, failureMessage string, mutate scaleMutation) (*shared.ScaleResult, error) {
	model, err := assessmentstore.LoadScale(ctx, s.modelRepo, scaleCode)
	if err != nil {
		return nil, err
	}
	if err := assessmentstore.EnsureHeadEditable(ctx, s.modelRepo, model); err != nil {
		return nil, err
	}

	scale, err := legacyadapter.MedicalScaleFromAssessmentModel(model)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "%s", failureMessage)
	}
	if err := mutate(scale); err != nil {
		return nil, shared.WrapScaleDomainError(err, errorCode.ErrInvalidArgument, "%s", failureMessage)
	}

	now := time.Now().UTC()
	if err := legacyadapter.SyncAssessmentModelFromMedicalScale(model, scale, now); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "%s", failureMessage)
	}
	if err := assessmentstore.SaveScale(ctx, s.modelRepo, model); err != nil {
		return nil, err
	}

	eventing.PublishCollectedEvents(ctx, s.eventPublisher, scale, nil, nil)
	s.refreshListCache(ctx)
	return assessmentstore.ScaleResult(model)
}

func (s *factorService) loadEditableScale(ctx context.Context, scaleCode string) (*scaledefinition.MedicalScale, error) {
	scale, err := s.repo.FindByCode(ctx, scaleCode)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取量表失败")
	}
	if err := editable.EnsureHeadEditable(ctx, s.repo, scale); err != nil {
		return nil, err
	}
	return scale, nil
}

func (s *factorService) persistLegacyFactorMutation(ctx context.Context, scale *scaledefinition.MedicalScale) (*shared.ScaleResult, error) {
	if err := s.repo.Update(ctx, scale); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存量表失败")
	}

	eventing.PublishCollectedEvents(ctx, s.eventPublisher, scale, nil, nil)
	s.refreshListCache(ctx)

	return shared.ToScaleResult(scale), nil
}

func (s *factorService) refreshListCache(ctx context.Context) {
	if s.listCache == nil {
		return
	}
	shared.LogScaleListCacheError(ctx, s.listCache.Rebuild(ctx))
}
