package pipeline

import (
	"context"
	"strconv"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type InterpretReportWriter interface {
	BuildAndSave(ctx context.Context, evalCtx *Context) error
	SetReportDurableSaver(saver ReportDurableSaver)
}

type durableInterpretReportWriter struct {
	reportSaver   ReportDurableSaver
	reportBuilder domainReport.ReportBuilder
}

func NewInterpretReportWriter(reportBuilder domainReport.ReportBuilder, reportSaver ReportDurableSaver) InterpretReportWriter {
	return &durableInterpretReportWriter{
		reportSaver:   reportSaver,
		reportBuilder: reportBuilder,
	}
}

// BuildAndSave 生成并保存报告。
func (w *durableInterpretReportWriter) BuildAndSave(ctx context.Context, evalCtx *Context) error {
	l := logger.L(ctx)
	assessmentID, _ := evalCtx.Assessment.ID().Value()
	l.Infow("Generating report", "assessment_id", assessmentID)

	rpt, err := w.reportBuilder.Build(evalCtx.Assessment, evalCtx.MedicalScale, evalCtx.EvaluationResult)
	if err != nil {
		l.Errorw("Failed to build report",
			"assessment_id", assessmentID,
			"error", err)
		return errors.WrapC(err, errorCode.ErrAssessmentInterpretFailed, "生成报告失败")
	}
	reportID, _ := rpt.ID().Value()
	l.Debugw("Report built successfully", "report_id", reportID)

	if err := w.reportSaver.SaveReportDurably(ctx, rpt, evalCtx.Assessment.TesteeID(), w.buildSuccessEvents(evalCtx, rpt)); err != nil {
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

	evalCtx.Report = rpt
	return nil
}

func (w *durableInterpretReportWriter) SetReportDurableSaver(saver ReportDurableSaver) {
	if saver != nil {
		w.reportSaver = saver
	}
}

func (w *durableInterpretReportWriter) buildSuccessEvents(evalCtx *Context, rpt *domainReport.InterpretReport) []event.DomainEvent {
	now := time.Now()
	assessmentRef := evalCtx.Assessment.MedicalScaleRef()
	if assessmentRef == nil {
		return nil
	}

	scaleVersion := ""
	if evalCtx.MedicalScale != nil {
		scaleVersion = evalCtx.MedicalScale.GetQuestionnaireVersion()
	} else if !evalCtx.Assessment.QuestionnaireRef().IsEmpty() {
		scaleVersion = evalCtx.Assessment.QuestionnaireRef().Version()
	}

	scaleRef := assessment.NewMedicalScaleRef(
		assessmentRef.ID(),
		assessmentRef.Code(),
		scaleVersion,
	)

	assessmentID := evalCtx.Assessment.ID().Uint64()
	reportID := rpt.ID().Uint64()
	testeeID := evalCtx.Assessment.TesteeID().Uint64()

	return []event.DomainEvent{
		assessment.NewAssessmentInterpretedEvent(
			evalCtx.Assessment.OrgID(),
			evalCtx.Assessment.ID(),
			evalCtx.Assessment.TesteeID(),
			scaleRef,
			evalCtx.EvaluationResult.TotalScore,
			evalCtx.EvaluationResult.RiskLevel,
			now,
		),
		domainReport.NewReportGeneratedEvent(
			strconv.FormatUint(reportID, 10),
			strconv.FormatUint(assessmentID, 10),
			testeeID,
			rpt.ScaleCode(),
			scaleVersion,
			rpt.TotalScore(),
			string(rpt.RiskLevel()),
			now,
		),
		domainStatistics.NewFootprintReportGeneratedEvent(
			evalCtx.Assessment.OrgID(),
			testeeID,
			assessmentID,
			reportID,
			now,
		),
	}
}
