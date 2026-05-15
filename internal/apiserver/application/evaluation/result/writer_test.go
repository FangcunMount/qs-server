package result

import (
	"context"
	"errors"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type resultAssessmentRepoStub struct {
	order *[]string
	saved *assessment.Assessment
}

func (r *resultAssessmentRepoStub) Save(_ context.Context, a *assessment.Assessment) error {
	*r.order = append(*r.order, "assessment")
	r.saved = a
	return nil
}

func (r *resultAssessmentRepoStub) FindByID(context.Context, assessment.ID) (*assessment.Assessment, error) {
	return nil, nil
}
func (r *resultAssessmentRepoStub) Delete(context.Context, assessment.ID) error { return nil }
func (r *resultAssessmentRepoStub) FindByAnswerSheetID(context.Context, assessment.AnswerSheetRef) (*assessment.Assessment, error) {
	return nil, nil
}

type resultScoreRepoStub struct {
	order *[]string
	score *assessment.AssessmentScore
}

func (r *resultScoreRepoStub) SaveScoresWithContext(_ context.Context, _ *assessment.Assessment, score *assessment.AssessmentScore) error {
	*r.order = append(*r.order, "score")
	r.score = score
	return nil
}

func (r *resultScoreRepoStub) DeleteByAssessmentID(context.Context, assessment.ID) error { return nil }

type resultReportBuilderStub struct {
	order *[]string
	rpt   *domainReport.InterpretReport
	kind  assessment.EvaluationModelKind
}

func (b *resultReportBuilderStub) Kind() assessment.EvaluationModelKind {
	if b.kind != "" {
		return b.kind
	}
	return assessment.EvaluationModelKindScale
}

func (b *resultReportBuilderStub) Build(context.Context, Outcome) (*domainReport.InterpretReport, error) {
	*b.order = append(*b.order, "report_build")
	return b.rpt, nil
}

type resultReportSaverStub struct {
	order      *[]string
	err        error
	eventTypes []string
	testeeID   testee.ID
}

func (s *resultReportSaverStub) SaveReportDurably(_ context.Context, _ *domainReport.InterpretReport, testeeID testee.ID, events []event.DomainEvent) error {
	*s.order = append(*s.order, "report_save")
	s.testeeID = testeeID
	for _, evt := range events {
		s.eventTypes = append(s.eventTypes, evt.EventType())
	}
	return s.err
}

type resultNotifierStub struct {
	order *[]string
}

func (n resultNotifierStub) NotifyCompletion(context.Context, Outcome) {
	*n.order = append(*n.order, "waiter")
}

func TestGenericEventAssemblerIsFallbackOnly(t *testing.T) {
	if got := (GenericEventAssembler{}).Kind(); got != "" {
		t.Fatalf("GenericEventAssembler kind = %q, want empty fallback kind", got)
	}
}

func TestWriterPersistsScaleOutcomeInLegacyOrderAndStagesEvents(t *testing.T) {
	order := make([]string, 0)
	a := submittedScaleAssessment(t)
	outcome := scaleOutcomeForWriterTest(a)
	scoreProjectors, err := NewScoreProjectorRegistry(NewScaleScoreProjector(&resultScoreRepoStub{order: &order}))
	if err != nil {
		t.Fatalf("NewScoreProjectorRegistry returned error: %v", err)
	}
	reportSaver := &resultReportSaverStub{order: &order}
	reportBuilders, err := NewReportBuilderRegistry(&resultReportBuilderStub{
		order: &order,
		rpt: domainReport.NewInterpretReport(
			domainReport.ID(a.ID()),
			"Scale",
			"S-001",
			7,
			domainReport.RiskLevelLow,
			"ok",
			nil,
			nil,
		),
	})
	if err != nil {
		t.Fatalf("NewReportBuilderRegistry returned error: %v", err)
	}
	writer := NewWriter(
		&resultAssessmentRepoStub{order: &order},
		scoreProjectors,
		reportBuilders,
		reportSaver,
		resultNotifierStub{order: &order},
	)

	if err := writer.Write(context.Background(), outcome); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	wantOrder := []string{"score", "assessment", "report_build", "report_save", "waiter"}
	if len(order) != len(wantOrder) {
		t.Fatalf("order = %#v, want %#v", order, wantOrder)
	}
	for i := range wantOrder {
		if order[i] != wantOrder[i] {
			t.Fatalf("order = %#v, want %#v", order, wantOrder)
		}
	}
	wantEvents := []string{assessment.EventTypeInterpreted, domainReport.EventTypeGenerated, "footprint.report_generated"}
	if len(reportSaver.eventTypes) != len(wantEvents) {
		t.Fatalf("event types = %#v, want %#v", reportSaver.eventTypes, wantEvents)
	}
	for i := range wantEvents {
		if reportSaver.eventTypes[i] != wantEvents[i] {
			t.Fatalf("event types = %#v, want %#v", reportSaver.eventTypes, wantEvents)
		}
	}
}

func TestWriterReportSaveFailureKeepsInterpretedSaveBeforeReturningError(t *testing.T) {
	order := make([]string, 0)
	a := submittedScaleAssessment(t)
	reportErr := errors.New("report save failed")
	scoreProjectors, _ := NewScoreProjectorRegistry(NewScaleScoreProjector(&resultScoreRepoStub{order: &order}))
	reportBuilders, _ := NewReportBuilderRegistry(&resultReportBuilderStub{
		order: &order,
		rpt:   domainReport.NewInterpretReport(domainReport.ID(a.ID()), "Scale", "S-001", 7, domainReport.RiskLevelLow, "ok", nil, nil),
	})
	assessmentRepo := &resultAssessmentRepoStub{order: &order}
	writer := NewWriter(
		assessmentRepo,
		scoreProjectors,
		reportBuilders,
		&resultReportSaverStub{order: &order, err: reportErr},
		resultNotifierStub{order: &order},
	)

	err := writer.Write(context.Background(), scaleOutcomeForWriterTest(a))
	if err == nil {
		t.Fatal("Write error = nil, want report save failure")
	}
	if assessmentRepo.saved == nil || !assessmentRepo.saved.Status().IsInterpreted() {
		t.Fatalf("assessment should have been saved as interpreted before report failure")
	}
	if got := order[len(order)-1]; got != "report_save" {
		t.Fatalf("last operation = %s, want report_save before returning error; order=%#v", got, order)
	}
}

func TestWriterUsesGenericEventsAndNoopScoreProjectionForNonScaleOutcome(t *testing.T) {
	order := make([]string, 0)
	modelRef := assessment.NewEvaluationModelRefByCode(assessment.EvaluationModelKindMBTI, meta.NewCode("MBTI-16P"), "1.0.0", "MBTI")
	a, err := assessment.NewAssessment(
		1,
		testee.NewID(8002),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("Q-MBTI"), "1.0.0"),
		assessment.NewAnswerSheetRef(meta.FromUint64(6002)),
		assessment.NewAdhocOrigin(),
		assessment.WithID(assessment.NewID(7002)),
		assessment.WithEvaluationModel(modelRef),
	)
	if err != nil {
		t.Fatalf("NewAssessment returned error: %v", err)
	}
	if err := a.Submit(); err != nil {
		t.Fatalf("Submit returned error: %v", err)
	}
	a.ClearEvents()

	reportBuilders, err := NewReportBuilderRegistry(&resultReportBuilderStub{
		order: &order,
		kind:  assessment.EvaluationModelKindMBTI,
		rpt:   domainReport.NewInterpretReport(domainReport.ID(a.ID()), "MBTI", "MBTI-16P", 0, domainReport.RiskLevelNone, "INTJ", nil, nil),
	})
	if err != nil {
		t.Fatalf("NewReportBuilderRegistry returned error: %v", err)
	}
	reportSaver := &resultReportSaverStub{order: &order}
	writer := NewWriter(
		&resultAssessmentRepoStub{order: &order},
		nil,
		reportBuilders,
		reportSaver,
		nil,
	)
	result := assessment.NewEvaluationResult(0, assessment.RiskLevelNone, "INTJ", "", nil).WithModelRef(modelRef)

	if err := writer.Write(context.Background(), Outcome{Assessment: a, Result: result}); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	wantOrder := []string{"assessment", "report_build", "report_save"}
	for i := range wantOrder {
		if order[i] != wantOrder[i] {
			t.Fatalf("order = %#v, want prefix %#v", order, wantOrder)
		}
	}
	if len(reportSaver.eventTypes) != 1 || reportSaver.eventTypes[0] != assessment.EventTypeInterpreted {
		t.Fatalf("event types = %#v, want generic assessment interpreted only", reportSaver.eventTypes)
	}
}

func submittedScaleAssessment(t *testing.T) *assessment.Assessment {
	t.Helper()
	scaleRef := assessment.NewMedicalScaleRef(meta.FromUint64(9001), meta.NewCode("S-001"), "Scale")
	a, err := assessment.NewAssessment(
		1,
		testee.NewID(8001),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("Q-001"), "1.0.0"),
		assessment.NewAnswerSheetRef(meta.FromUint64(6001)),
		assessment.NewAdhocOrigin(),
		assessment.WithID(assessment.NewID(7001)),
		assessment.WithMedicalScale(scaleRef),
	)
	if err != nil {
		t.Fatalf("NewAssessment returned error: %v", err)
	}
	if err := a.Submit(); err != nil {
		t.Fatalf("Submit returned error: %v", err)
	}
	a.ClearEvents()
	return a
}

func scaleOutcomeForWriterTest(a *assessment.Assessment) Outcome {
	result := assessment.NewEvaluationResult(7, assessment.RiskLevelLow, "ok", "keep", nil).
		WithModelRef(*a.EvaluationModelRef())
	return Outcome{
		Assessment: a,
		Input: &evaluationinput.InputSnapshot{
			MedicalScale: &evaluationinput.ScaleSnapshot{
				Code:                 "S-001",
				Title:                "Scale",
				QuestionnaireVersion: "1.0.0",
			},
		},
		Result: result,
	}
}
