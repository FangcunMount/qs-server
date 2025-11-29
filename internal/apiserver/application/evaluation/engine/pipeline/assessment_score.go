package pipeline

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
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

	// 2. 计算整体风险等级
	riskLevel := h.calculateOverallRiskLevel(evalCtx.FactorScores, totalScore)
	evalCtx.RiskLevel = riskLevel

	// 3. 更新因子得分的风险等级（根据量表的阈值规则）
	h.updateFactorRiskLevels(evalCtx)

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

	// 如果没有总分因子，计算平均分
	if count > 0 {
		return totalScore / float64(count)
	}
	return 0
}

// calculateOverallRiskLevel 计算整体风险等级
func (h *AssessmentScoreHandler) calculateOverallRiskLevel(factorScores []assessment.FactorScoreResult, totalScore float64) assessment.RiskLevel {
	// TODO: 根据量表的阈值规则计算风险等级
	// 当前使用简单的阈值判断
	switch {
	case totalScore >= 80:
		return assessment.RiskLevelSevere
	case totalScore >= 60:
		return assessment.RiskLevelHigh
	case totalScore >= 40:
		return assessment.RiskLevelMedium
	case totalScore >= 20:
		return assessment.RiskLevelLow
	default:
		return assessment.RiskLevelNone
	}
}

// updateFactorRiskLevels 更新因子风险等级
func (h *AssessmentScoreHandler) updateFactorRiskLevels(evalCtx *Context) {
	// TODO: 根据量表中各因子的阈值规则更新风险等级
	// 当前使用简单的阈值判断
	updatedScores := make([]assessment.FactorScoreResult, 0, len(evalCtx.FactorScores))

	for _, fs := range evalCtx.FactorScores {
		riskLevel := h.calculateFactorRiskLevel(fs.RawScore)

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
func (h *AssessmentScoreHandler) calculateFactorRiskLevel(score float64) assessment.RiskLevel {
	// TODO: 根据因子的具体阈值规则计算
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
