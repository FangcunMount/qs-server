package result

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestScaleEventAssemblerPublishesOutcomeEvents(t *testing.T) {
	a, err := assessment.NewAssessment(
		1,
		testee.NewID(2001),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("Q-SDS"), "1.0.0"),
		assessment.NewAnswerSheetRef(meta.FromUint64(5001)),
		assessment.NewAdhocOrigin(),
		assessment.WithID(assessment.NewID(101)),
		assessment.WithMedicalScale(assessment.NewMedicalScaleRef(meta.FromUint64(3001), meta.NewCode("SDS"), "抑郁自评")),
	)
	if err != nil {
		t.Fatalf("NewAssessment: %v", err)
	}
	outcome := NewOutcomeFromLegacyResult(a, nil, assessment.NewEvaluationResult(12, assessment.RiskLevelMedium, "medium", "follow", nil).
		WithModelRef(*a.EvaluationModelRef()))
	rpt := domainreport.NewInterpretReport(
		domainreport.ID(a.ID()),
		"抑郁自评",
		"SDS",
		12,
		domainreport.RiskLevelMedium,
		"medium",
		nil,
		nil,
		nil,
	)
	rpt = AttachReportOutcomeSummary(outcome, rpt)

	events := (ScaleEventAssembler{}).BuildSuccessEvents(outcome, rpt)
	if len(events) != 3 {
		t.Fatalf("events = %d, want 3", len(events))
	}
	if events[0].EventType() != eventcatalog.AssessmentInterpreted {
		t.Fatalf("first event = %s, want %s", events[0].EventType(), eventcatalog.AssessmentInterpreted)
	}
	if events[1].EventType() != eventcatalog.ReportGenerated {
		t.Fatalf("second event = %s, want %s", events[1].EventType(), eventcatalog.ReportGenerated)
	}
}
