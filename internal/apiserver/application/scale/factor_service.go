package scale

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalelistcache"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// factorService 量表因子编辑服务实现
// 行为者：量表因子编辑者
type factorService struct {
	repo           scale.Repository
	listCache      scalelistcache.PublishedListCache
	eventPublisher event.EventPublisher
}

// NewFactorService 创建量表因子编辑服务
func NewFactorService(repo scale.Repository, listCache scalelistcache.PublishedListCache, eventPublisher event.EventPublisher) ScaleFactorService {
	return &factorService{
		repo:           repo,
		listCache:      listCache,
		eventPublisher: eventPublisher,
	}
}

// AddFactor 添加因子
func (s *factorService) AddFactor(ctx context.Context, dto AddFactorDTO) (*ScaleResult, error) {
	// 1. 验证输入参数
	if dto.ScaleCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if dto.Code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "因子编码不能为空")
	}
	if dto.Title == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "因子标题不能为空")
	}

	// 2. 获取可编辑量表
	m, err := s.loadEditableScale(ctx, dto.ScaleCode)
	if err != nil {
		return nil, err
	}

	// 3. 创建因子
	factor, err := toFactorDomain(dto.Code, dto.Title, dto.FactorType, dto.IsTotalScore, dto.IsShow,
		dto.QuestionCodes, dto.ScoringStrategy, dto.ScoringParams, dto.MaxScore, dto.InterpretRules)
	if err != nil {
		return nil, err
	}

	// 4. 添加因子
	if err := m.AddFactor(factor); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "添加因子失败")
	}

	return s.persistFactorMutation(ctx, m)
}

// UpdateFactor 更新因子
func (s *factorService) UpdateFactor(ctx context.Context, dto UpdateFactorDTO) (*ScaleResult, error) {
	// 1. 验证输入参数
	if dto.ScaleCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if dto.Code == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "因子编码不能为空")
	}

	// 2. 获取可编辑量表
	m, err := s.loadEditableScale(ctx, dto.ScaleCode)
	if err != nil {
		return nil, err
	}

	// 3. 创建更新后的因子
	factor, err := toFactorDomain(dto.Code, dto.Title, dto.FactorType, dto.IsTotalScore, dto.IsShow,
		dto.QuestionCodes, dto.ScoringStrategy, dto.ScoringParams, dto.MaxScore, dto.InterpretRules)
	if err != nil {
		return nil, err
	}

	// 4. 更新因子
	if err := m.UpdateFactor(factor); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "更新因子失败")
	}

	return s.persistFactorMutation(ctx, m)
}

// RemoveFactor 删除因子
func (s *factorService) RemoveFactor(ctx context.Context, scaleCode, factorCode string) (*ScaleResult, error) {
	// 1. 验证输入参数
	if scaleCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if factorCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "因子编码不能为空")
	}

	// 2. 获取可编辑量表
	m, err := s.loadEditableScale(ctx, scaleCode)
	if err != nil {
		return nil, err
	}

	// 3. 删除因子
	if err := m.RemoveFactor(scale.NewFactorCode(factorCode)); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "删除因子失败")
	}

	return s.persistFactorMutation(ctx, m)
}

// ReplaceFactors 替换所有因子
func (s *factorService) ReplaceFactors(ctx context.Context, scaleCode string, factorDTOs []FactorDTO) (*ScaleResult, error) {
	// 1. 验证输入参数
	if scaleCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if len(factorDTOs) == 0 {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "因子列表不能为空")
	}

	// 2. 获取可编辑量表
	m, err := s.loadEditableScale(ctx, scaleCode)
	if err != nil {
		return nil, err
	}

	// 3. 转换因子列表并验证
	factors := make([]*scale.Factor, 0, len(factorDTOs))
	var allValidationErrors []scale.ValidationError

	for _, dto := range factorDTOs {
		factor, err := toFactorDomain(dto.Code, dto.Title, dto.FactorType, dto.IsTotalScore, dto.IsShow,
			dto.QuestionCodes, dto.ScoringStrategy, dto.ScoringParams, dto.MaxScore, dto.InterpretRules)
		if err != nil {
			return nil, err
		}

		// 验证因子
		factorErrs := scale.ValidateFactor(factor)
		if len(factorErrs) > 0 {
			allValidationErrors = append(allValidationErrors, factorErrs...)
		}

		factors = append(factors, factor)
	}

	// 如果有验证错误，返回所有错误
	if len(allValidationErrors) > 0 {
		return nil, wrapScaleDomainError(scale.ToError(allValidationErrors), errorCode.ErrInvalidArgument, "验证因子失败")
	}

	// 4. 替换因子
	if err := m.ReplaceFactors(factors); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "替换因子失败")
	}

	return s.persistFactorMutation(ctx, m)
}

// UpdateFactorInterpretRules 更新因子解读规则
func (s *factorService) UpdateFactorInterpretRules(ctx context.Context, dto UpdateFactorInterpretRulesDTO) (*ScaleResult, error) {
	// 1. 验证输入参数
	if dto.ScaleCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if dto.FactorCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "因子编码不能为空")
	}

	// 2. 获取可编辑量表
	m, err := s.loadEditableScale(ctx, dto.ScaleCode)
	if err != nil {
		return nil, err
	}

	// 3. 转换解读规则
	rules := interpretRulesFromDTOs(dto.InterpretRules)

	// 4. 更新解读规则
	if err := m.UpdateFactorInterpretRules(scale.NewFactorCode(dto.FactorCode), rules); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "更新解读规则失败")
	}

	return s.persistFactorMutation(ctx, m)
}

// ReplaceInterpretRules 批量设置所有因子的解读规则
func (s *factorService) ReplaceInterpretRules(ctx context.Context, scaleCode string, dtos []UpdateFactorInterpretRulesDTO) (*ScaleResult, error) {
	// 1. 验证输入参数
	if scaleCode == "" {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表编码不能为空")
	}
	if len(dtos) == 0 {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "因子解读规则列表不能为空")
	}

	// 2. 获取可编辑量表
	m, err := s.loadEditableScale(ctx, scaleCode)
	if err != nil {
		return nil, err
	}

	// 3. 批量更新各因子的解读规则
	for _, dto := range dtos {
		if dto.FactorCode == "" {
			return nil, errors.WithCode(errorCode.ErrInvalidArgument, "因子编码不能为空")
		}

		rules := interpretRulesFromDTOs(dto.InterpretRules)

		// 更新解读规则
		if err := m.UpdateFactorInterpretRules(scale.NewFactorCode(dto.FactorCode), rules); err != nil {
			return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "更新因子[%s]解读规则失败", dto.FactorCode)
		}
	}

	return s.persistFactorMutation(ctx, m)
}

func (s *factorService) loadEditableScale(ctx context.Context, scaleCode string) (*scale.MedicalScale, error) {
	m, err := s.repo.FindByCode(ctx, scaleCode)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取量表失败")
	}
	if m.IsArchived() {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表已归档，不能编辑")
	}
	return m, nil
}

func (s *factorService) persistFactorMutation(ctx context.Context, m *scale.MedicalScale) (*ScaleResult, error) {
	if err := s.repo.Update(ctx, m); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存量表失败")
	}

	s.publishScaleUpdated(ctx, m)
	s.refreshListCache(ctx)

	return toScaleResult(m), nil
}

func (s *factorService) refreshListCache(ctx context.Context) {
	if s.listCache == nil {
		return
	}
	logScaleListCacheError(ctx, s.listCache.Rebuild(ctx))
}

func (s *factorService) publishScaleUpdated(ctx context.Context, m *scale.MedicalScale) {
	if s.eventPublisher == nil || m == nil {
		return
	}
	eventing.PublishCollectedEvents(ctx, s.eventPublisher, eventing.Collect(
		scale.NewScaleChangedEvent(
			m.GetID().Uint64(),
			m.GetCode().String(),
			"",
			m.GetTitle(),
			scale.ChangeActionUpdated,
			time.Now(),
		),
	), nil, nil)
}
