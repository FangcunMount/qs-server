package factor

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/assessmentmodel/behavior/scale/editable"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/assessmentmodel/behavior/scale/ports"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/assessmentmodel/behavior/scale/shared"
	scaledefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/scale/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalelistcache"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// factorService 量表因子编辑服务实现
// 行为者：量表因子编辑者
type factorService struct {
	repo           factorRepository
	listCache      scalelistcache.PublishedListCache
	eventPublisher event.EventPublisher
}

type factorRepository interface {
	CreatePublishedSnapshot(ctx context.Context, scale *scaledefinition.MedicalScale, active bool) error
	FindByCode(ctx context.Context, code string) (*scaledefinition.MedicalScale, error)
	Update(ctx context.Context, scale *scaledefinition.MedicalScale) error
}

// NewService 创建量表因子编辑应用服务。
func NewService(repo factorRepository, listCache scalelistcache.PublishedListCache, eventPublisher event.EventPublisher) ports.ScaleFactorService {
	return &factorService{
		repo:           repo,
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

	m, err := s.loadEditableScale(ctx, dto.ScaleCode)
	if err != nil {
		return nil, err
	}

	factor, err := toFactorDomain(dto.Code, dto.Title, dto.FactorType, dto.IsTotalScore, dto.IsShow,
		dto.QuestionCodes, dto.ScoringStrategy, dto.ScoringParams, dto.MaxScore, dto.InterpretRules)
	if err != nil {
		return nil, err
	}

	if err := m.AddFactor(factor); err != nil {
		return nil, shared.WrapScaleDomainError(err, errorCode.ErrInvalidArgument, "添加因子失败")
	}

	return s.persistFactorMutation(ctx, m)
}

// UpdateFactor 更新因子
func (s *factorService) UpdateFactor(ctx context.Context, dto shared.UpdateFactorDTO) (*shared.ScaleResult, error) {
	if dto.ScaleCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if dto.Code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "因子编码不能为空")
	}

	m, err := s.loadEditableScale(ctx, dto.ScaleCode)
	if err != nil {
		return nil, err
	}

	factor, err := toFactorDomain(dto.Code, dto.Title, dto.FactorType, dto.IsTotalScore, dto.IsShow,
		dto.QuestionCodes, dto.ScoringStrategy, dto.ScoringParams, dto.MaxScore, dto.InterpretRules)
	if err != nil {
		return nil, err
	}

	if err := m.UpdateFactor(factor); err != nil {
		return nil, shared.WrapScaleDomainError(err, errorCode.ErrInvalidArgument, "更新因子失败")
	}

	return s.persistFactorMutation(ctx, m)
}

// RemoveFactor 删除因子
func (s *factorService) RemoveFactor(ctx context.Context, scaleCode, factorCode string) (*shared.ScaleResult, error) {
	if scaleCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if factorCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "因子编码不能为空")
	}

	m, err := s.loadEditableScale(ctx, scaleCode)
	if err != nil {
		return nil, err
	}

	if err := m.RemoveFactor(scaledefinition.NewFactorCode(factorCode)); err != nil {
		return nil, shared.WrapScaleDomainError(err, errorCode.ErrInvalidArgument, "删除因子失败")
	}

	return s.persistFactorMutation(ctx, m)
}

// ReplaceFactors 替换所有因子
func (s *factorService) ReplaceFactors(ctx context.Context, scaleCode string, factorDTOs []shared.FactorDTO) (*shared.ScaleResult, error) {
	if scaleCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if len(factorDTOs) == 0 {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "因子列表不能为空")
	}

	m, err := s.loadEditableScale(ctx, scaleCode)
	if err != nil {
		return nil, err
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

	if err := m.ReplaceFactors(factors); err != nil {
		return nil, shared.WrapScaleDomainError(err, errorCode.ErrInvalidArgument, "替换因子失败")
	}

	return s.persistFactorMutation(ctx, m)
}

// UpdateFactorInterpretRules 更新因子解读规则
func (s *factorService) UpdateFactorInterpretRules(ctx context.Context, dto shared.UpdateFactorInterpretRulesDTO) (*shared.ScaleResult, error) {
	if dto.ScaleCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if dto.FactorCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "因子编码不能为空")
	}

	m, err := s.loadEditableScale(ctx, dto.ScaleCode)
	if err != nil {
		return nil, err
	}

	rules := shared.InterpretRulesFromDTOs(dto.InterpretRules)

	if err := m.UpdateFactorInterpretRules(scaledefinition.NewFactorCode(dto.FactorCode), rules); err != nil {
		return nil, shared.WrapScaleDomainError(err, errorCode.ErrInvalidArgument, "更新解读规则失败")
	}

	return s.persistFactorMutation(ctx, m)
}

// ReplaceInterpretRules 批量设置所有因子的解读规则
func (s *factorService) ReplaceInterpretRules(ctx context.Context, scaleCode string, dtos []shared.UpdateFactorInterpretRulesDTO) (*shared.ScaleResult, error) {
	if scaleCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if len(dtos) == 0 {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "因子解读规则列表不能为空")
	}

	m, err := s.loadEditableScale(ctx, scaleCode)
	if err != nil {
		return nil, err
	}

	for _, dto := range dtos {
		if dto.FactorCode == "" {
			return nil, errors.WithCode(errorCode.ErrInvalidArgument, "因子编码不能为空")
		}

		rules := shared.InterpretRulesFromDTOs(dto.InterpretRules)

		if err := m.UpdateFactorInterpretRules(scaledefinition.NewFactorCode(dto.FactorCode), rules); err != nil {
			return nil, shared.WrapScaleDomainError(err, errorCode.ErrInvalidArgument, "更新因子[%s]解读规则失败", dto.FactorCode)
		}
	}

	return s.persistFactorMutation(ctx, m)
}

func (s *factorService) loadEditableScale(ctx context.Context, scaleCode string) (*scaledefinition.MedicalScale, error) {
	m, err := s.repo.FindByCode(ctx, scaleCode)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取量表失败")
	}
	if err := editable.EnsureHeadEditable(ctx, s.repo, m); err != nil {
		return nil, err
	}
	return m, nil
}

func (s *factorService) persistFactorMutation(ctx context.Context, m *scaledefinition.MedicalScale) (*shared.ScaleResult, error) {
	if err := s.repo.Update(ctx, m); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存量表失败")
	}

	eventing.PublishCollectedEvents(ctx, s.eventPublisher, m, nil, nil)
	s.refreshListCache(ctx)

	return shared.ToScaleResult(m), nil
}

func (s *factorService) refreshListCache(ctx context.Context) {
	if s.listCache == nil {
		return
	}
	shared.LogScaleListCacheError(ctx, s.listCache.Rebuild(ctx))
}
