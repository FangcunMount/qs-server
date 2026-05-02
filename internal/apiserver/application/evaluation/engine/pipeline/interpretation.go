package pipeline

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/logger"
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretengine"
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
	generator *InterpretationGenerator
	finalizer *InterpretationFinalizer
}

type InterpretationGenerator struct {
	interpreter     interpretengine.Interpreter
	defaultProvider interpretengine.DefaultProvider
}

type InterpretationFinalizer struct {
	assessmentWriter AssessmentResultWriter
	reportWriter     InterpretReportWriter
}

func NewInterpretationGenerator(interpreter interpretengine.Interpreter, defaultProvider interpretengine.DefaultProvider) *InterpretationGenerator {
	return &InterpretationGenerator{
		interpreter:     interpreter,
		defaultProvider: defaultProvider,
	}
}

func NewInterpretationFinalizer(
	assessmentWriter AssessmentResultWriter,
	reportWriter InterpretReportWriter,
) *InterpretationFinalizer {
	return &InterpretationFinalizer{
		assessmentWriter: assessmentWriter,
		reportWriter:     reportWriter,
	}
}

// NewInterpretationHandler 创建测评分析解读处理器。
func NewInterpretationHandler(
	generator *InterpretationGenerator,
	finalizer *InterpretationFinalizer,
) *InterpretationHandler {
	return &InterpretationHandler{
		BaseHandler: NewBaseHandler("InterpretationHandler"),
		generator:   generator,
		finalizer:   finalizer,
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

	if h.generator == nil {
		err := evalerrors.ModuleNotConfigured("interpretation generator is not configured")
		evalCtx.SetError(err)
		return err
	}
	h.generator.Generate(ctx, evalCtx)
	l.Debugw("Evaluation result built",
		"conclusion", evalCtx.EvaluationResult.Conclusion,
		"suggestion", evalCtx.EvaluationResult.Suggestion)

	if h.finalizer == nil {
		err := evalerrors.ModuleNotConfigured("interpretation finalizer is not configured")
		evalCtx.SetError(err)
		return err
	}
	if err := h.finalizer.Finalize(ctx, evalCtx); err != nil {
		assessmentID, _ := evalCtx.Assessment.ID().Value()
		l.Errorw("Failed to finalize interpretation",
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

func (g *InterpretationGenerator) Generate(ctx context.Context, evalCtx *Context) {
	g.generateFactorInterpretations(ctx, evalCtx)
	g.generateOverallInterpretation(ctx, evalCtx)
	evalCtx.EvaluationResult = g.buildEvaluationResult(evalCtx)
}

func (f *InterpretationFinalizer) Finalize(ctx context.Context, evalCtx *Context) error {
	evalResult := evalCtx.EvaluationResult
	if evalResult == nil {
		evalResult = assessment.NewEvaluationResult(
			evalCtx.TotalScore,
			evalCtx.RiskLevel,
			evalCtx.Conclusion,
			evalCtx.Suggestion,
			evalCtx.FactorScores,
		)
		evalCtx.EvaluationResult = evalResult
	}

	if f.assessmentWriter == nil {
		return evalerrors.ModuleNotConfigured("assessment result writer is not configured")
	}
	if err := f.assessmentWriter.ApplyAndSave(ctx, evalCtx); err != nil {
		return err
	}
	if f.reportWriter == nil {
		return evalerrors.ModuleNotConfigured("interpret report writer is not configured")
	}
	if err := f.reportWriter.BuildAndSave(ctx, evalCtx); err != nil {
		return err
	}

	return nil
}
