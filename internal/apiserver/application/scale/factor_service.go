package scale

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// factorService 量表因子编辑服务实现
// 行为者：量表因子编辑者
type factorService struct {
	repo          scale.Repository
	factorManager scale.FactorManager
}

// NewFactorService 创建量表因子编辑服务
func NewFactorService(repo scale.Repository) ScaleFactorService {
	return &factorService{
		repo:          repo,
		factorManager: scale.FactorManager{},
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

	// 2. 获取量表
	m, err := s.repo.FindByCode(ctx, dto.ScaleCode)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取量表失败")
	}

	// 3. 检查量表状态
	if m.IsArchived() {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表已归档，不能编辑")
	}

	// 4. 创建因子
	factor, err := toFactorDomain(dto.Code, dto.Title, dto.FactorType, dto.IsTotalScore,
		dto.QuestionCodes, dto.ScoringStrategy, dto.ScoringParams, dto.MaxScore, dto.InterpretRules)
	if err != nil {
		return nil, err
	}

	// 5. 添加因子
	if err := s.factorManager.AddFactor(m, factor); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "添加因子失败")
	}

	// 6. 持久化
	if err := s.repo.Update(ctx, m); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存量表失败")
	}

	return toScaleResult(m), nil
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

	// 2. 获取量表
	m, err := s.repo.FindByCode(ctx, dto.ScaleCode)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取量表失败")
	}

	// 3. 检查量表状态
	if m.IsArchived() {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表已归档，不能编辑")
	}

	// 4. 创建更新后的因子
	factor, err := toFactorDomain(dto.Code, dto.Title, dto.FactorType, dto.IsTotalScore,
		dto.QuestionCodes, dto.ScoringStrategy, dto.ScoringParams, dto.MaxScore, dto.InterpretRules)
	if err != nil {
		return nil, err
	}

	// 5. 更新因子
	if err := s.factorManager.UpdateFactor(m, factor); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "更新因子失败")
	}

	// 6. 持久化
	if err := s.repo.Update(ctx, m); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存量表失败")
	}

	return toScaleResult(m), nil
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

	// 2. 获取量表
	m, err := s.repo.FindByCode(ctx, scaleCode)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取量表失败")
	}

	// 3. 检查量表状态
	if m.IsArchived() {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表已归档，不能编辑")
	}

	// 4. 删除因子
	if err := s.factorManager.RemoveFactor(m, scale.NewFactorCode(factorCode)); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "删除因子失败")
	}

	// 5. 持久化
	if err := s.repo.Update(ctx, m); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存量表失败")
	}

	return toScaleResult(m), nil
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

	// 2. 获取量表
	m, err := s.repo.FindByCode(ctx, scaleCode)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取量表失败")
	}

	// 3. 检查量表状态
	if m.IsArchived() {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表已归档，不能编辑")
	}

	// 4. 转换因子列表并验证
	factors := make([]*scale.Factor, 0, len(factorDTOs))
	var allValidationErrors []scale.ValidationError

	for _, dto := range factorDTOs {
		factor, err := toFactorDomain(dto.Code, dto.Title, dto.FactorType, dto.IsTotalScore,
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
		return nil, scale.ToError(allValidationErrors)
	}

	// 5. 替换因子
	if err := s.factorManager.ReplaceFactors(m, factors); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "替换因子失败")
	}

	// 6. 持久化
	if err := s.repo.Update(ctx, m); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存量表失败")
	}

	return toScaleResult(m), nil
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

	// 2. 获取量表
	m, err := s.repo.FindByCode(ctx, dto.ScaleCode)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取量表失败")
	}

	// 3. 检查量表状态
	if m.IsArchived() {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表已归档，不能编辑")
	}

	// 4. 转换解读规则
	rules := make([]scale.InterpretationRule, 0, len(dto.InterpretRules))
	for _, ruleDTO := range dto.InterpretRules {
		rule := scale.NewInterpretationRule(
			scale.NewScoreRange(ruleDTO.MinScore, ruleDTO.MaxScore),
			scale.RiskLevel(ruleDTO.RiskLevel),
			ruleDTO.Conclusion,
			ruleDTO.Suggestion,
		)
		rules = append(rules, rule)
	}

	// 5. 更新解读规则
	if err := s.factorManager.UpdateFactorInterpretRules(m, scale.NewFactorCode(dto.FactorCode), rules); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "更新解读规则失败")
	}

	// 6. 持久化
	if err := s.repo.Update(ctx, m); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存量表失败")
	}

	return toScaleResult(m), nil
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

	// 2. 获取量表
	m, err := s.repo.FindByCode(ctx, scaleCode)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrMedicalScaleNotFound, "获取量表失败")
	}

	// 3. 检查量表状态
	if m.IsArchived() {
		return nil, errors.WithCode(errorCode.ErrInvalidArgument, "量表已归档，不能编辑")
	}

	// 4. 批量更新各因子的解读规则
	for _, dto := range dtos {
		if dto.FactorCode == "" {
			return nil, errors.WithCode(errorCode.ErrInvalidArgument, "因子编码不能为空")
		}

		// 转换解读规则
		rules := make([]scale.InterpretationRule, 0, len(dto.InterpretRules))
		for _, ruleDTO := range dto.InterpretRules {
			rule := scale.NewInterpretationRule(
				scale.NewScoreRange(ruleDTO.MinScore, ruleDTO.MaxScore),
				scale.RiskLevel(ruleDTO.RiskLevel),
				ruleDTO.Conclusion,
				ruleDTO.Suggestion,
			)
			rules = append(rules, rule)
		}

		// 更新解读规则
		if err := s.factorManager.UpdateFactorInterpretRules(m, scale.NewFactorCode(dto.FactorCode), rules); err != nil {
			return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "更新因子[%s]解读规则失败", dto.FactorCode)
		}
	}

	// 5. 持久化
	if err := s.repo.Update(ctx, m); err != nil {
		return nil, errors.WrapC(err, errorCode.ErrDatabase, "保存量表失败")
	}

	return toScaleResult(m), nil
}

// ============= 辅助函数 =============

// toFactorDomain 将 DTO 转换为因子领域对象
func toFactorDomain(
	code, title, factorType string,
	isTotalScore bool,
	questionCodes []string,
	scoringStrategy string,
	scoringParams *ScoringParamsDTO,
	maxScore *float64,
	interpretRules []InterpretRuleDTO,
) (*scale.Factor, error) {
	// 转换题目编码
	qCodes := make([]meta.Code, 0, len(questionCodes))
	for _, qc := range questionCodes {
		qCodes = append(qCodes, meta.NewCode(qc))
	}

	// 转换解读规则
	rules := make([]scale.InterpretationRule, 0, len(interpretRules))
	for _, ruleDTO := range interpretRules {
		rule := scale.NewInterpretationRule(
			scale.NewScoreRange(ruleDTO.MinScore, ruleDTO.MaxScore),
			scale.RiskLevel(ruleDTO.RiskLevel),
			ruleDTO.Conclusion,
			ruleDTO.Suggestion,
		)
		rules = append(rules, rule)
	}

	// 确定计分策略
	strategy := scale.ScoringStrategySum
	if scoringStrategy != "" {
		strategy = scale.ScoringStrategyCode(scoringStrategy)
	}

	// 确定因子类型
	fType := scale.FactorTypePrimary
	if factorType != "" {
		fType = scale.FactorType(factorType)
	}

	// 转换计分参数为领域层的 ScoringParams
	var scoringParamsDomain *scale.ScoringParams
	if scoringParams != nil {
		scoringParamsDomain = scale.NewScoringParams().
			WithCntOptionContents(scoringParams.CntOptionContents)
	} else {
		scoringParamsDomain = scale.NewScoringParams()
	}

	// 验证：cnt 策略必须提供非空的 CntOptionContents
	if strategy == scale.ScoringStrategyCnt {
		if scoringParamsDomain == nil || len(scoringParamsDomain.GetCntOptionContents()) == 0 {
			return nil, errors.WithCode(errorCode.ErrInvalidArgument, "cnt 计分策略必须提供 cnt_option_contents 参数")
		}
	}

	// 创建因子
	factor, err := scale.NewFactor(
		scale.NewFactorCode(code),
		title,
		scale.WithFactorType(fType),
		scale.WithIsTotalScore(isTotalScore),
		scale.WithQuestionCodes(qCodes),
		scale.WithScoringStrategy(strategy),
		scale.WithScoringParams(scoringParamsDomain),
		scale.WithMaxScore(maxScore),
		scale.WithInterpretRules(rules),
	)
	if err != nil {
		return nil, errors.WrapC(err, errorCode.ErrInvalidArgument, "创建因子失败")
	}

	return factor, nil
}
