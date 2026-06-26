package characterization_test

import (
	"context"
	"testing"

	evaluationresult "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/result"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// V1 contract: scale result writer persists in order
// report_build -> report_save -> score -> assessment -> waiter,
// and stages assessment.interpreted + report.generated + footprint.report_generated.
func TestV1ScaleWriterPersistenceOrderAndStagedEvents(t *testing.T) {
	order := make([]string, 0)
	a := submittedScaleAssessment(t)
	outcome := evaluationresult.Outcome{
		Assessment: a,
		Input:      scaleInputSnapshot(),
		Result: assessment.NewEvaluationResult(7, assessment.RiskLevelLow, "low", "keep", nil).
			WithModelRef(*a.EvaluationModelRef()),
	}

	scoreProjectors, err := evaluationresult.NewScoreProjectorRegistry(
		evaluationresult.NewScaleScoreProjector(&writerScoreRepoStub{order: &order}),
	)
	if err != nil {
		t.Fatalf("NewScoreProjectorRegistry: %v", err)
	}
	reportSaver := &writerReportSaverStub{order: &order}
	reportBuilders, err := evaluationresult.NewReportBuilderRegistry(&writerReportBuilderStub{
		order: &order,
		rpt: domainreport.NewInterpretReport(
			domainreport.ID(a.ID()),
			"Scale",
			"S-001",
			7,
			domainreport.RiskLevelLow,
			"low",
			nil,
			nil,
			nil,
		),
	})
	if err != nil {
		t.Fatalf("NewReportBuilderRegistry: %v", err)
	}
	writer, err := evaluationresult.NewWriter(
		&writerAssessmentRepoStub{order: &order},
		scoreProjectors,
		reportBuilders,
		reportSaver,
		&writerNotifierStub{order: &order},
		nil,
	)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	if err := writer.Write(context.Background(), outcome); err != nil {
		t.Fatalf("Write: %v", err)
	}

	wantOrder := []string{"report_build", "report_save", "score", "assessment", "waiter"}
	if len(order) != len(wantOrder) {
		t.Fatalf("order = %#v, want %#v", order, wantOrder)
	}
	for i := range wantOrder {
		if order[i] != wantOrder[i] {
			t.Fatalf("order = %#v, want %#v", order, wantOrder)
		}
	}

	wantEvents := []string{assessment.EventTypeInterpretedV2, domainreport.EventTypeGeneratedV2, "footprint.report_generated"}
	if len(reportSaver.eventTypes) != len(wantEvents) {
		t.Fatalf("event types = %#v, want %#v", reportSaver.eventTypes, wantEvents)
	}
	for i := range wantEvents {
		if reportSaver.eventTypes[i] != wantEvents[i] {
			t.Fatalf("event types = %#v, want %#v", reportSaver.eventTypes, wantEvents)
		}
	}
}

type writerAssessmentRepoStub struct {
	order *[]string
}

func (r *writerAssessmentRepoStub) Save(_ context.Context, _ *assessment.Assessment) error {
	*r.order = append(*r.order, "assessment")
	return nil
}
func (*writerAssessmentRepoStub) FindByID(context.Context, assessment.ID) (*assessment.Assessment, error) {
	return nil, nil
}
func (*writerAssessmentRepoStub) Delete(context.Context, assessment.ID) error { return nil }
func (*writerAssessmentRepoStub) FindByAnswerSheetID(context.Context, assessment.AnswerSheetRef) (*assessment.Assessment, error) {
	return nil, nil
}

type writerScoreRepoStub struct {
	order *[]string
}

func (r *writerScoreRepoStub) SaveScoresWithContext(_ context.Context, _ *assessment.Assessment, _ *assessment.AssessmentScore) error {
	*r.order = append(*r.order, "score")
	return nil
}
func (*writerScoreRepoStub) DeleteByAssessmentID(context.Context, assessment.ID) error { return nil }

type writerReportBuilderStub struct {
	order *[]string
	rpt   *domainreport.InterpretReport
}

func (*writerReportBuilderStub) Kind() assessment.EvaluationModelKind {
	return assessment.EvaluationModelKindScale
}
func (*writerReportBuilderStub) ReportType() domainreport.ReportType {
	return domainreport.ReportTypeStandard
}
func (b *writerReportBuilderStub) Build(context.Context, evaluationresult.Outcome) (*domainreport.InterpretReport, error) {
	*b.order = append(*b.order, "report_build")
	return b.rpt, nil
}

type writerReportSaverStub struct {
	order      *[]string
	eventTypes []string
	testeeID   testee.ID
	err        error
}

func (s *writerReportSaverStub) SaveReportDurably(_ context.Context, _ *domainreport.InterpretReport, testeeID testee.ID, events []event.DomainEvent) error {
	*s.order = append(*s.order, "report_save")
	s.testeeID = testeeID
	for _, evt := range events {
		s.eventTypes = append(s.eventTypes, evt.EventType())
	}
	return s.err
}

type writerNotifierStub struct {
	order *[]string
}

func (n *writerNotifierStub) NotifyCompletion(context.Context, evaluationresult.Outcome) {
	*n.order = append(*n.order, "waiter")
}
