package pipeline

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// AssessmentScoreHandler 测评分数计算处理器
// 职责：
// 1. 根据因子得分计算总分和风险等级
// 2. 保存 AssessmentScore 到仓储
// 输入：Context.FactorScores
// 输出：填充 Context.TotalScore, RiskLevel
type AssessmentScoreHandler struct {
	*BaseHandler
	scoreRepo assessment.ScoreRepository
}

// NewAssessmentScoreHandler 创建测评分数计算处理器
func NewAssessmentScoreHandler(scoreRepo assessment.ScoreRepository) *AssessmentScoreHandler {
	return &AssessmentScoreHandler{
		BaseHandler: NewBaseHandler("AssessmentScoreHandler"),
		scoreRepo:   scoreRepo,
	}
}

// Handle 处理测评分数计算
func (h *AssessmentScoreHandler) Handle(ctx context.Context, evalCtx *Context) error {
	// 检查前置条件
	if len(evalCtx.FactorScores) == 0 {
		evalCtx.SetError(ErrFactorScoresRequired)
		return evalCtx.Error
	}

	// 1. 计算总分
	totalScore := h.calculateTotalScore(evalCtx.FactorScores)
	evalCtx.TotalScore = totalScore

	// 2. 更新因子得分的风险等级（根据量表的阈值规则）
	h.updateFactorRiskLevels(evalCtx)

	// 3. 计算整体风险等级（基于因子风险等级）
	riskLevel := h.calculateOverallRiskLevel(evalCtx)
	evalCtx.RiskLevel = riskLevel

	// 4. 保存 AssessmentScore
	if err := h.saveAssessmentScore(ctx, evalCtx); err != nil {
		evalCtx.SetError(err)
		return err
	}

	// 继续下一个处理器
	return h.Next(ctx, evalCtx)
}

// calculateTotalScore 计算总分
func (h *AssessmentScoreHandler) calculateTotalScore(factorScores []assessment.FactorScoreResult) float64 {
	var totalScore float64
	var count int

	for _, fs := range factorScores {
		// 如果有明确标记为总分的因子，直接使用
		if fs.IsTotalScore {
			return fs.RawScore
		}
		totalScore += fs.RawScore
		count++
	}

	// 如果没有总分因子，返回所有因子得分之和
	return totalScore
}

// updateFactorRiskLevels 更新因子风险等级
// 使用量表中定义的解读规则来计算每个因子的风险等级
func (h *AssessmentScoreHandler) updateFactorRiskLevels(evalCtx *Context) {
	updatedScores := make([]assessment.FactorScoreResult, 0, len(evalCtx.FactorScores))

	for _, fs := range evalCtx.FactorScores {
		riskLevel := h.calculateFactorRiskLevel(evalCtx.MedicalScale, fs.FactorCode, fs.RawScore)

		updatedScore := assessment.NewFactorScoreResult(
			fs.FactorCode,
			fs.FactorName,
			fs.RawScore,
			riskLevel,
			fs.Conclusion,
			fs.Suggestion,
			fs.IsTotalScore,
		)
		updatedScores = append(updatedScores, updatedScore)
	}

	evalCtx.FactorScores = updatedScores
}

// calculateFactorRiskLevel 计算因子风险等级
// 优先使用量表中定义的解读规则，如果没有则使用默认阈值
func (h *AssessmentScoreHandler) calculateFactorRiskLevel(
	medicalScale *scale.MedicalScale,
	factorCode assessment.FactorCode,
	score float64,
) assessment.RiskLevel {
	// 尝试从量表获取因子的解读规则
	if medicalScale != nil {
		scaleFactorCode := scale.NewFactorCode(string(factorCode))
		if factor, found := medicalScale.FindFactorByCode(scaleFactorCode); found {
			if rule := factor.FindInterpretRule(score); rule != nil {
				// 将 scale.RiskLevel 转换为 assessment.RiskLevel
				return convertScaleRiskLevel(rule.GetRiskLevel())
			}
		}
	}

	// 使用默认阈值判断
	return h.defaultRiskLevelByScore(score)
}

// calculateOverallRiskLevel 计算整体风险等级
// 综合所有因子的风险等级，取最高风险作为整体风险
func (h *AssessmentScoreHandler) calculateOverallRiskLevel(evalCtx *Context) assessment.RiskLevel {
	// 优先使用量表的整体解读规则
	if evalCtx.MedicalScale != nil {
		// 尝试查找总分因子的解读规则
		for _, fs := range evalCtx.FactorScores {
			if fs.IsTotalScore {
				scaleFactorCode := scale.NewFactorCode(string(fs.FactorCode))
				if factor, found := evalCtx.MedicalScale.FindFactorByCode(scaleFactorCode); found {
					if rule := factor.FindInterpretRule(fs.RawScore); rule != nil {
						return convertScaleRiskLevel(rule.GetRiskLevel())
					}
				}
			}
		}
	}

	// 没有总分因子规则时，取所有因子中的最高风险等级
	maxRisk := assessment.RiskLevelNone
	for _, fs := range evalCtx.FactorScores {
		if riskLevelOrder(fs.RiskLevel) > riskLevelOrder(maxRisk) {
			maxRisk = fs.RiskLevel
		}
	}

	return maxRisk
}

// defaultRiskLevelByScore 根据分数使用默认阈值计算风险等级
func (h *AssessmentScoreHandler) defaultRiskLevelByScore(score float64) assessment.RiskLevel {
	switch {
	case score >= 80:
		return assessment.RiskLevelSevere
	case score >= 60:
		return assessment.RiskLevelHigh
	case score >= 40:
		return assessment.RiskLevelMedium
	case score >= 20:
		return assessment.RiskLevelLow
	default:
		return assessment.RiskLevelNone
	}
}

// convertScaleRiskLevel 将 scale.RiskLevel 转换为 assessment.RiskLevel
func convertScaleRiskLevel(scaleLevel scale.RiskLevel) assessment.RiskLevel {
	switch scaleLevel {
	case scale.RiskLevelNone:
		return assessment.RiskLevelNone
	case scale.RiskLevelLow:
		return assessment.RiskLevelLow
	case scale.RiskLevelMedium:
		return assessment.RiskLevelMedium
	case scale.RiskLevelHigh:
		return assessment.RiskLevelHigh
	case scale.RiskLevelSevere:
		return assessment.RiskLevelSevere
	default:
		return assessment.RiskLevelNone
	}
}

// riskLevelOrder 返回风险等级的排序值（用于比较）
func riskLevelOrder(level assessment.RiskLevel) int {
	switch level {
	case assessment.RiskLevelNone:
		return 0
	case assessment.RiskLevelLow:
		return 1
	case assessment.RiskLevelMedium:
		return 2
	case assessment.RiskLevelHigh:
		return 3
	case assessment.RiskLevelSevere:
		return 4
	default:
		return 0
	}
}

// saveAssessmentScore 保存测评得分
func (h *AssessmentScoreHandler) saveAssessmentScore(ctx context.Context, evalCtx *Context) error {
	// 转换因子得分
	factorScores := make([]assessment.FactorScore, 0, len(evalCtx.FactorScores))
	for _, fs := range evalCtx.FactorScores {
		factorScores = append(factorScores, assessment.NewFactorScore(
			fs.FactorCode,
			fs.FactorName,
			fs.RawScore,
			fs.RiskLevel,
			fs.IsTotalScore,
		))
	}

	// 创建 AssessmentScore
	score := assessment.NewAssessmentScore(
		evalCtx.Assessment.ID(),
		evalCtx.TotalScore,
		evalCtx.RiskLevel,
		factorScores,
	)

	// 保存到仓储
	if err := h.scoreRepo.SaveScores(ctx, []*assessment.AssessmentScore{score}); err != nil {
		return errors.WrapC(err, errorCode.ErrDatabase, "保存测评得分失败")
	}

	return nil
}
