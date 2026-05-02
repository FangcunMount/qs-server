package pipeline

import (
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type InterpretationEventAssembler interface {
	BuildSuccessEvents(evalCtx *Context, rpt *domainReport.InterpretReport) []event.DomainEvent
}

type defaultInterpretationEventAssembler struct{}

func NewInterpretationEventAssembler() InterpretationEventAssembler {
	return defaultInterpretationEventAssembler{}
}

func (defaultInterpretationEventAssembler) BuildSuccessEvents(evalCtx *Context, rpt *domainReport.InterpretReport) []event.DomainEvent {
	now := time.Now()
	assessmentRef := evalCtx.Assessment.MedicalScaleRef()
	if assessmentRef == nil {
		return nil
	}

	scaleVersion := ""
	if evalCtx.MedicalScale != nil {
		scaleVersion = evalCtx.MedicalScale.QuestionnaireVersion
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
