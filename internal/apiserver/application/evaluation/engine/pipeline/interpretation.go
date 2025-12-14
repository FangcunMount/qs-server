package pipeline

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// InterpretationHandler 测评分析解读处理器
// 职责：
// 1. 根据因子得分和风险等级生成解读结论和建议
// 2. 应用评估结果到 Assessment
// 3. 生成并保存 InterpretReport
// 输入：Context（包含因子得分、总分、风险等级）
// 输出：填充 Context.Conclusion, Suggestion, EvaluationResult
type InterpretationHandler struct {
	*BaseHandler
	assessmentRepo  assessment.Repository
	reportRepo      report.ReportRepository
	reportBuilder   report.ReportBuilder
	interpreter     interpretation.Interpreter                    // 解读服务
	defaultProvider *interpretation.DefaultInterpretationProvider // 默认解读提供者
}

// NewInterpretationHandler 创建测评分析解读处理器
func NewInterpretationHandler(
	assessmentRepo assessment.Repository,
	reportRepo report.ReportRepository,
	reportBuilder report.ReportBuilder,
) *InterpretationHandler {
	return &InterpretationHandler{
		BaseHandler:     NewBaseHandler("InterpretationHandler"),
		assessmentRepo:  assessmentRepo,
		reportRepo:      reportRepo,
		reportBuilder:   reportBuilder,
		interpreter:     interpretation.GetDefaultInterpreter(), // 使用默认解读器
		defaultProvider: interpretation.GetDefaultProvider(),    // 使用默认解读提供者
	}
}

// Handle 处理测评分析解读
func (h *InterpretationHandler) Handle(ctx context.Context, evalCtx *Context) error {
	l := logger.L(ctx)
	assessmentID, _ := evalCtx.Assessment.ID().Value()
	l.Infow("Starting interpretation handler",
		"assessment_id", assessmentID,
		"factor_count", len(evalCtx.FactorScores),
		"total_score", evalCtx.TotalScore,
		"risk_level", evalCtx.RiskLevel)

	// 1. 生成因子解读
	h.generateFactorInterpretations(ctx, evalCtx)

	// 2. 生成整体解读
	h.generateOverallInterpretation(ctx, evalCtx)

	// 3. 构建完整的评估结果
	evalResult := h.buildEvaluationResult(evalCtx)
	evalCtx.EvaluationResult = evalResult
	l.Debugw("Evaluation result built",
		"conclusion", evalResult.Conclusion,
		"suggestion", evalResult.Suggestion)

	// 4. 应用评估结果到 Assessment
	if err := evalCtx.Assessment.ApplyEvaluation(evalResult); err != nil {
		assessmentID, _ := evalCtx.Assessment.ID().Value()
		l.Errorw("Failed to apply evaluation result",
			"assessment_id", assessmentID,
			"error", err)
		evalCtx.SetError(errors.WrapC(err, errorCode.ErrAssessmentInterpretFailed, "应用评估结果失败"))
		return evalCtx.Error
	}

	// 5. 保存 Assessment
	if err := h.assessmentRepo.Save(ctx, evalCtx.Assessment); err != nil {
		assessmentID, _ := evalCtx.Assessment.ID().Value()
		l.Errorw("Failed to save assessment",
			"assessment_id", assessmentID,
			"error", err)
		evalCtx.SetError(errors.WrapC(err, errorCode.ErrDatabase, "保存测评失败"))
		return evalCtx.Error
	}
	assessmentID, _ = evalCtx.Assessment.ID().Value()
	l.Infow("Assessment saved successfully",
		"assessment_id", assessmentID)

	// 6. 生成并保存报告
	if err := h.generateAndSaveReport(ctx, evalCtx); err != nil {
		assessmentID, _ := evalCtx.Assessment.ID().Value()
		l.Errorw("Failed to generate and save report",
			"assessment_id", assessmentID,
			"error", err)
		evalCtx.SetError(err)
		return err
	}

	assessmentID, _ = evalCtx.Assessment.ID().Value()
	l.Infow("Interpretation handler completed successfully",
		"assessment_id", assessmentID)

	// 继续下一个处理器
	return h.Next(ctx, evalCtx)
}

// generateFactorInterpretations 生成因子解读
func (h *InterpretationHandler) generateFactorInterpretations(ctx context.Context, evalCtx *Context) {
	l := logger.L(ctx)
	l.Infow("Generating factor interpretations", "factor_count", len(evalCtx.FactorScores))

	updatedScores := make([]assessment.FactorScoreResult, 0, len(evalCtx.FactorScores))

	for _, fs := range evalCtx.FactorScores {
		// 尝试从量表获取因子的解读规则
		conclusion, suggestion := h.interpretFactorWithRules(ctx, evalCtx, fs)

		updatedScore := assessment.NewFactorScoreResult(
			fs.FactorCode,
			fs.FactorName,
			fs.RawScore,
			fs.RiskLevel,
			conclusion,
			suggestion,
			fs.IsTotalScore,
		)
		updatedScores = append(updatedScores, updatedScore)

		h.logInterpretation(ctx, string(fs.FactorCode), fs.FactorName, fs.RawScore, conclusion, suggestion)
	}

	evalCtx.FactorScores = updatedScores
	l.Infow("Factor interpretations generated", "factor_count", len(updatedScores))
}

// interpretFactorWithRules 使用量表规则解读因子
func (h *InterpretationHandler) interpretFactorWithRules(ctx context.Context, evalCtx *Context, fs assessment.FactorScoreResult) (conclusion, suggestion string) {
	l := logger.L(ctx)

	// 卫语句：没有量表配置，直接使用默认解读
	if evalCtx.MedicalScale == nil {
		h.logUseDefault(ctx, string(fs.FactorCode), fs.RawScore)
		return h.interpretFactorDefault(fs.FactorName, fs.RiskLevel, fs.RawScore)
	}

	// 卫语句：查找因子
	scaleFactorCode := scale.NewFactorCode(string(fs.FactorCode))
	factor, found := evalCtx.MedicalScale.FindFactorByCode(scaleFactorCode)
	if !found {
		l.Warnw("Factor not found in scale",
			"factor_code", string(fs.FactorCode))
		h.logUseDefault(ctx, string(fs.FactorCode), fs.RawScore)
		return h.interpretFactorDefault(fs.FactorName, fs.RiskLevel, fs.RawScore)
	}

	l.Debugw("Found factor in scale",
		"factor_code", string(fs.FactorCode),
		"score", fs.RawScore)

	// 卫语句：构建解读配置
	config := h.buildInterpretConfig(factor)
	if config == nil || len(config.Rules) == 0 {
		l.Debugw("No interpret rules in factor config",
			"factor_code", string(fs.FactorCode))
		h.logUseDefault(ctx, string(fs.FactorCode), fs.RawScore)
		return h.interpretFactorDefault(fs.FactorName, fs.RiskLevel, fs.RawScore)
	}

	// 使用解读服务进行解读（默认使用区间策略）
	result, err := h.interpreter.InterpretFactor(
		fs.RawScore,
		config,
		interpretation.StrategyTypeRange,
	)
	if err != nil || result == nil {
		l.Warnw("Failed to interpret factor",
			"factor_code", string(fs.FactorCode),
			"error", err)
		h.logUseDefault(ctx, string(fs.FactorCode), fs.RawScore)
		return h.interpretFactorDefault(fs.FactorName, fs.RiskLevel, fs.RawScore)
	}

	// 成功使用规则解读
	h.logRuleMatch(ctx, string(fs.FactorCode), fs.RawScore, result)
	return result.Description, result.Suggestion
}

// buildInterpretConfig 将 scale.Factor 的解读规则转换为 interpretation.InterpretConfig
func (h *InterpretationHandler) buildInterpretConfig(factor *scale.Factor) *interpretation.InterpretConfig {
	scaleRules := factor.GetInterpretRules()
	if len(scaleRules) == 0 {
		return nil
	}

	// 转换为领域解读规则
	rules := make([]interpretation.InterpretRule, 0, len(scaleRules))
	for _, scaleRule := range scaleRules {
		rules = append(rules, interpretation.InterpretRule{
			Min:         scaleRule.GetScoreRange().Min(),
			Max:         scaleRule.GetScoreRange().Max(),
			RiskLevel:   interpretation.RiskLevel(scaleRule.GetRiskLevel()),
			Label:       string(scaleRule.GetRiskLevel()),
			Description: scaleRule.GetConclusion(),
			Suggestion:  scaleRule.GetSuggestion(),
		})
	}

	return &interpretation.InterpretConfig{
		FactorCode: factor.GetCode().Value(),
		Rules:      rules,
		Params:     nil,
	}
}

// interpretFactorDefault 使用默认模板解读因子
func (h *InterpretationHandler) interpretFactorDefault(factorName string, riskLevel assessment.RiskLevel, score float64) (conclusion, suggestion string) {
	// 转换风险等级
	interpretRiskLevel := interpretation.RiskLevel(riskLevel)

	// 使用领域服务提供默认解读
	result := h.defaultProvider.ProvideFactor(factorName, score, interpretRiskLevel)
	return result.Description, result.Suggestion
}

// generateOverallInterpretation 生成整体解读
func (h *InterpretationHandler) generateOverallInterpretation(ctx context.Context, evalCtx *Context) {
	l := logger.L(ctx)
	l.Infow("Generating overall interpretation",
		"total_score", evalCtx.TotalScore,
		"risk_level", evalCtx.RiskLevel)

	// 卫语句：没有量表配置，使用默认解读
	if evalCtx.MedicalScale == nil {
		l.Debugw("No medical scale, using default template for overall interpretation")
		h.generateOverallInterpretationDefault(evalCtx)
		l.Infow("Overall interpretation generated",
			"conclusion", evalCtx.Conclusion)
		return
	}

	// 尝试从总分因子的解读规则获取整体解读
	totalScoreFactor := h.findTotalScoreFactor(evalCtx)
	if totalScoreFactor == nil {
		l.Debugw("No total score factor found, using default template")
		h.generateOverallInterpretationDefault(evalCtx)
		l.Infow("Overall interpretation generated",
			"conclusion", evalCtx.Conclusion)
		return
	}

	l.Debugw("Found total score factor",
		"factor_code", string(totalScoreFactor.FactorCode),
		"score", totalScoreFactor.RawScore)

	// 尝试使用总分因子的解读规则
	if h.tryInterpretWithTotalScoreRule(ctx, evalCtx, totalScoreFactor) {
		l.Infow("Overall interpretation from total score rule",
			"conclusion", evalCtx.Conclusion)
		return
	}

	// 降级使用默认模板
	l.Debugw("No matching rule for total score, using default template")
	h.generateOverallInterpretationDefault(evalCtx)
	l.Infow("Overall interpretation generated",
		"conclusion", evalCtx.Conclusion)
}

// findTotalScoreFactor 查找总分因子
func (h *InterpretationHandler) findTotalScoreFactor(evalCtx *Context) *assessment.FactorScoreResult {
	for _, fs := range evalCtx.FactorScores {
		if fs.IsTotalScore {
			return &fs
		}
	}
	return nil
}

// tryInterpretWithTotalScoreRule 尝试使用总分因子的规则进行解读
// 返回 true 表示成功解读，false 表示失败需要降级
func (h *InterpretationHandler) tryInterpretWithTotalScoreRule(
	ctx context.Context,
	evalCtx *Context,
	totalScoreFactor *assessment.FactorScoreResult,
) bool {
	l := logger.L(ctx)

	// 查找因子配置
	scaleFactorCode := scale.NewFactorCode(string(totalScoreFactor.FactorCode))
	factor, found := evalCtx.MedicalScale.FindFactorByCode(scaleFactorCode)
	if !found {
		l.Warnw("Total score factor not found in scale",
			"factor_code", string(totalScoreFactor.FactorCode))
		return false
	}

	// 查找匹配的解读规则
	rule := factor.FindInterpretRule(totalScoreFactor.RawScore)
	if rule == nil {
		l.Debugw("No interpret rule matched for total score",
			"factor_code", string(totalScoreFactor.FactorCode),
			"score", totalScoreFactor.RawScore)
		return false
	}

	// 检查规则是否有内容
	if rule.GetConclusion() == "" {
		l.Debugw("Interpret rule has empty conclusion",
			"factor_code", string(totalScoreFactor.FactorCode))
		return false
	}

	// 应用规则解读
	evalCtx.Conclusion = rule.GetConclusion()
	evalCtx.Suggestion = rule.GetSuggestion()
	return true
}

// generateOverallInterpretationDefault 使用默认模板生成整体解读
func (h *InterpretationHandler) generateOverallInterpretationDefault(evalCtx *Context) {
	// 转换风险等级
	interpretRiskLevel := interpretation.RiskLevel(evalCtx.RiskLevel)

	// 使用领域服务提供默认整体解读
	result := h.defaultProvider.ProvideOverall(evalCtx.TotalScore, interpretRiskLevel)
	evalCtx.Conclusion = result.Description
	evalCtx.Suggestion = result.Suggestion
}

// buildEvaluationResult 构建评估结果
func (h *InterpretationHandler) buildEvaluationResult(evalCtx *Context) *assessment.EvaluationResult {
	return assessment.NewEvaluationResult(
		evalCtx.TotalScore,
		evalCtx.RiskLevel,
		evalCtx.Conclusion,
		evalCtx.Suggestion,
		evalCtx.FactorScores,
	)
}

// generateAndSaveReport 生成并保存报告
func (h *InterpretationHandler) generateAndSaveReport(ctx context.Context, evalCtx *Context) error {
	l := logger.L(ctx)
	assessmentID, _ := evalCtx.Assessment.ID().Value()
	l.Infow("Generating report", "assessment_id", assessmentID)

	// 生成报告
	rpt, err := h.reportBuilder.Build(evalCtx.Assessment, evalCtx.MedicalScale, evalCtx.EvaluationResult)
	if err != nil {
		l.Errorw("Failed to build report",
			"assessment_id", assessmentID,
			"error", err)
		return errors.WrapC(err, errorCode.ErrAssessmentInterpretFailed, "生成报告失败")
	}
	reportID, _ := rpt.ID().Value()
	l.Debugw("Report built successfully", "report_id", reportID)

	// 保存报告
	if err := h.reportRepo.Save(ctx, rpt); err != nil {
		reportID, _ := rpt.ID().Value()
		assessmentID, _ := evalCtx.Assessment.ID().Value()
		l.Errorw("Failed to save report",
			"report_id", reportID,
			"assessment_id", assessmentID,
			"error", err)
		return errors.WrapC(err, errorCode.ErrDatabase, "保存报告失败")
	}
	reportID, _ = rpt.ID().Value()
	assessmentID, _ = evalCtx.Assessment.ID().Value()
	l.Infow("Report saved successfully", "report_id", reportID, "assessment_id", assessmentID)

	return nil
}

// ==================== 日志辅助方法 ====================

// logInterpretation 记录因子解读结果
func (h *InterpretationHandler) logInterpretation(
	ctx context.Context,
	factorCode string,
	factorName string,
	score float64,
	conclusion string,
	suggestion string,
) {
	l := logger.L(ctx)
	l.Infow("Factor interpretation generated",
		"factor_code", factorCode,
		"factor_name", factorName,
		"score", score,
		"conclusion", conclusion,
		"suggestion", suggestion)
}

// logRuleMatch 记录规则匹配
func (h *InterpretationHandler) logRuleMatch(
	ctx context.Context,
	factorCode string,
	score float64,
	result *interpretation.InterpretResult,
) {
	l := logger.L(ctx)
	l.Infow("Interpretation rule matched",
		"factor_code", factorCode,
		"score", score,
		"risk_level", result.RiskLevel,
		"label", result.Label)
}

// logUseDefault 记录使用默认模板
func (h *InterpretationHandler) logUseDefault(
	ctx context.Context,
	factorCode string,
	score float64,
) {
	l := logger.L(ctx)
	l.Debugw("Using default interpretation template",
		"factor_code", factorCode,
		"score", score)
}
