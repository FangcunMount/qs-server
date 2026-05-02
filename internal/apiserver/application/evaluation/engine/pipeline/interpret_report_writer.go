package pipeline

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/logger"
	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

type InterpretReportWriter interface {
	BuildAndSave(ctx context.Context, evalCtx *Context) error
}

type durableInterpretReportWriter struct {
	reportSaver    ReportDurableSaver
	reportBuilder  domainReport.ReportBuilder
	eventAssembler InterpretationEventAssembler
}

func NewInterpretReportWriter(reportBuilder domainReport.ReportBuilder, reportSaver ReportDurableSaver) InterpretReportWriter {
	return &durableInterpretReportWriter{
		reportSaver:    reportSaver,
		reportBuilder:  reportBuilder,
		eventAssembler: NewInterpretationEventAssembler(),
	}
}

// BuildAndSave 生成并保存报告。
func (w *durableInterpretReportWriter) BuildAndSave(ctx context.Context, evalCtx *Context) error {
	l := logger.L(ctx)
	assessmentID, _ := evalCtx.Assessment.ID().Value()
	l.Infow("Generating report", "assessment_id", assessmentID)
	if w.reportSaver == nil {
		return evalerrors.ModuleNotConfigured("report durable saver is not configured")
	}

	rpt, err := w.reportBuilder.Build(reportInputFromContext(evalCtx))
	if err != nil {
		l.Errorw("Failed to build report",
			"assessment_id", assessmentID,
			"error", err)
		return evalerrors.AssessmentInterpretFailed(err, "生成报告失败")
	}
	reportID, _ := rpt.ID().Value()
	l.Debugw("Report built successfully", "report_id", reportID)

	if err := w.reportSaver.SaveReportDurably(ctx, rpt, evalCtx.Assessment.TesteeID(), w.eventAssembler.BuildSuccessEvents(evalCtx, rpt)); err != nil {
		reportID, _ := rpt.ID().Value()
		assessmentID, _ := evalCtx.Assessment.ID().Value()
		l.Errorw("Failed to save report",
			"report_id", reportID,
			"assessment_id", assessmentID,
			"error", err)
		return evalerrors.Database(err, "保存报告失败")
	}
	reportID, _ = rpt.ID().Value()
	assessmentID, _ = evalCtx.Assessment.ID().Value()
	l.Infow("Report saved successfully", "report_id", reportID, "assessment_id", assessmentID)

	evalCtx.Report = rpt
	return nil
}

func reportInputFromContext(evalCtx *Context) domainReport.GenerateReportInput {
	input := domainReport.GenerateReportInput{}
	if evalCtx == nil {
		return input
	}
	if evalCtx.Assessment != nil {
		input.AssessmentID = domainReport.ID(evalCtx.Assessment.ID())
	}
	if evalCtx.MedicalScale != nil {
		input.ScaleName = evalCtx.MedicalScale.Title
		input.ScaleCode = evalCtx.MedicalScale.Code
	}
	if evalCtx.EvaluationResult != nil {
		input.TotalScore = evalCtx.EvaluationResult.TotalScore
		input.RiskLevel = domainReport.RiskLevel(evalCtx.EvaluationResult.RiskLevel)
		input.Conclusion = evalCtx.EvaluationResult.Conclusion
		input.Suggestion = evalCtx.EvaluationResult.Suggestion
		input.FactorScores = reportFactorScoreInputs(evalCtx.EvaluationResult.FactorScores, evalCtx.MedicalScale)
	}
	return input
}

func reportFactorScoreInputs(
	factorScores []assessment.FactorScoreResult,
	scaleSnapshot *evaluationinput.ScaleSnapshot,
) []domainReport.FactorScoreInput {
	factorMeta := make(map[string]evaluationinput.FactorSnapshot)
	if scaleSnapshot != nil {
		for _, f := range scaleSnapshot.Factors {
			factorMeta[f.Code] = f
		}
	}
	inputs := make([]domainReport.FactorScoreInput, 0, len(factorScores))
	for _, fs := range factorScores {
		meta, ok := factorMeta[string(fs.FactorCode)]
		factorName := fs.FactorName
		var maxScore *float64
		if ok {
			if factorName == "" {
				factorName = meta.Title
			}
			maxScore = meta.MaxScore
		}
		if factorName == "" {
			factorName = string(fs.FactorCode)
		}
		inputs = append(inputs, domainReport.FactorScoreInput{
			FactorCode:   domainReport.FactorCode(fs.FactorCode),
			FactorName:   factorName,
			RawScore:     fs.RawScore,
			MaxScore:     maxScore,
			RiskLevel:    domainReport.RiskLevel(fs.RiskLevel),
			Description:  fs.Conclusion,
			Suggestion:   fs.Suggestion,
			IsTotalScore: fs.IsTotalScore,
		})
	}
	return inputs
}
