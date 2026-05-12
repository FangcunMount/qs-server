package result

import (
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type EventAssembler interface {
	BuildSuccessEvents(outcome Outcome, rpt *domainReport.InterpretReport) []event.DomainEvent
}

type defaultEventAssembler struct{}

func NewEventAssembler() EventAssembler {
	return defaultEventAssembler{}
}

func (defaultEventAssembler) BuildSuccessEvents(outcome Outcome, rpt *domainReport.InterpretReport) []event.DomainEvent {
	if outcome.Assessment == nil || outcome.Result == nil || rpt == nil {
		return nil
	}
	now := time.Now()
	assessmentRef := outcome.Assessment.MedicalScaleRef()
	if assessmentRef == nil {
		return nil
	}
	modelRef := outcome.Assessment.EvaluationModelRef()
	if modelRef == nil {
		ref := assessmentRef.ToEvaluationModelRef()
		modelRef = &ref
	}

	scaleVersion := ""
	if outcome.Input != nil && outcome.Input.MedicalScale != nil {
		scaleVersion = outcome.Input.MedicalScale.QuestionnaireVersion
	} else if !outcome.Assessment.QuestionnaireRef().IsEmpty() {
		scaleVersion = outcome.Assessment.QuestionnaireRef().Version()
	}

	scaleRef := assessment.NewMedicalScaleRef(
		assessmentRef.ID(),
		assessmentRef.Code(),
		scaleVersion,
	)

	assessmentID := outcome.Assessment.ID().Uint64()
	reportID := rpt.ID().Uint64()
	testeeID := outcome.Assessment.TesteeID().Uint64()

	return []event.DomainEvent{
		assessment.NewAssessmentInterpretedEvent(
			outcome.Assessment.OrgID(),
			outcome.Assessment.ID(),
			outcome.Assessment.TesteeID(),
			*modelRef,
			scaleRef,
			outcome.Result.TotalScore,
			outcome.Result.RiskLevel,
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
			outcome.Assessment.OrgID(),
			testeeID,
			assessmentID,
			reportID,
			now,
		),
	}
}
