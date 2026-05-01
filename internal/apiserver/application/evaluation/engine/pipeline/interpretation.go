package pipeline

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
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

// NewInterpretationHandler 创建测评分析解读处理器
func NewInterpretationHandler(
	assessmentRepo assessment.Repository,
	reportRepo domainReport.ReportRepository,
	reportBuilder domainReport.ReportBuilder,
) *InterpretationHandler {
	return &InterpretationHandler{
		BaseHandler: NewBaseHandler("InterpretationHandler"),
		generator:   &InterpretationGenerator{},
		finalizer: &InterpretationFinalizer{
			assessmentWriter: NewAssessmentResultWriter(assessmentRepo),
			reportWriter:     NewInterpretReportWriter(reportBuilder, NewReportDurableSaver(reportRepo)),
		},
	}
}

func (h *InterpretationHandler) SetReportDurableSaver(saver ReportDurableSaver) {
	if saver == nil {
		return
	}
	if h.finalizer == nil {
		h.finalizer = &InterpretationFinalizer{}
	}
	h.finalizer.SetReportDurableSaver(saver)
}

func (h *InterpretationHandler) SetInterpretEngine(interpreter interpretengine.Interpreter, defaultProvider interpretengine.DefaultProvider) {
	if h.generator == nil {
		h.generator = &InterpretationGenerator{}
	}
	if interpreter != nil {
		h.generator.interpreter = interpreter
	}
	if defaultProvider != nil {
		h.generator.defaultProvider = defaultProvider
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

	h.ensureGenerator().Generate(ctx, evalCtx)
	l.Debugw("Evaluation result built",
		"conclusion", evalCtx.EvaluationResult.Conclusion,
		"suggestion", evalCtx.EvaluationResult.Suggestion)

	if err := h.ensureFinalizer().Finalize(ctx, evalCtx); err != nil {
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

func (h *InterpretationHandler) ensureGenerator() *InterpretationGenerator {
	if h.generator == nil {
		h.generator = &InterpretationGenerator{}
	}
	return h.generator
}

func (h *InterpretationHandler) ensureFinalizer() *InterpretationFinalizer {
	if h.finalizer == nil {
		h.finalizer = &InterpretationFinalizer{}
	}
	return h.finalizer
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

	if err := f.ensureAssessmentWriter().ApplyAndSave(ctx, evalCtx); err != nil {
		return err
	}
	if err := f.ensureReportWriter().BuildAndSave(ctx, evalCtx); err != nil {
		return err
	}

	return nil
}

func (f *InterpretationFinalizer) SetReportDurableSaver(saver ReportDurableSaver) {
	if saver == nil {
		return
	}
	f.ensureReportWriter().SetReportDurableSaver(saver)
}

func (f *InterpretationFinalizer) ensureAssessmentWriter() AssessmentResultWriter {
	if f.assessmentWriter == nil {
		f.assessmentWriter = repositoryAssessmentResultWriter{}
	}
	return f.assessmentWriter
}

func (f *InterpretationFinalizer) ensureReportWriter() InterpretReportWriter {
	if f.reportWriter == nil {
		f.reportWriter = &durableInterpretReportWriter{}
	}
	return f.reportWriter
}
