package result

import (
	"context"
	"errors"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scale/snapshot"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type resultAssessmentRepoStub struct {
	order *[]string
	saved *assessment.Assessment
	err   error
}

func (r *resultAssessmentRepoStub) Save(_ context.Context, a *assessment.Assessment) error {
	*r.order = append(*r.order, "assessment")
	r.saved = a
	return r.err
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
	score *assessment.ScaleScoreProjection
	err   error
}

func (r *resultScoreRepoStub) SaveScoresWithContext(_ context.Context, _ *assessment.Assessment, score *assessment.ScaleScoreProjection) error {
	*r.order = append(*r.order, "score")
	r.score = score
	return r.err
}

func (r *resultScoreRepoStub) DeleteByAssessmentID(context.Context, assessment.ID) error { return nil }

type resultReportBuilderStub struct {
	order *[]string
	rpt   *domainReport.InterpretReport
	key   evaluation.EvaluatorKey
	err   error
}

func (b *resultReportBuilderStub) Key() evaluation.EvaluatorKey {
	if !b.key.IsZero() {
		return b.key
	}
	return evaluation.EvaluatorKeyScaleDefault
}

func (*resultReportBuilderStub) ReportType() domainReport.ReportType {
	return domainReport.ReportTypeStandard
}

func (b *resultReportBuilderStub) Build(context.Context, Outcome) (*domainReport.InterpretReport, error) {
	*b.order = append(*b.order, "report_build")
	return b.rpt, b.err
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
	calls int
}

func (n *resultNotifierStub) NotifyCompletion(context.Context, Outcome) {
	*n.order = append(*n.order, "waiter")
	n.calls++
}

func TestGenericEventAssemblerIsFallbackOnly(t *testing.T) {
	if got := (GenericEventAssembler{}).Key(); !got.IsZero() {
		t.Fatalf("GenericEventAssembler key = %q, want empty fallback key", got)
	}
}

func TestWriterPersistsScaleOutcomeAfterReportDurableSaveAndStagesEvents(t *testing.T) {
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
			nil,
		),
	})
	if err != nil {
		t.Fatalf("NewReportBuilderRegistry returned error: %v", err)
	}
	writer, err := NewWriter(
		&resultAssessmentRepoStub{order: &order},
		scoreProjectors,
		reportBuilders,
		reportSaver,
		&resultNotifierStub{order: &order},
		nil,
	)
	if err != nil {
		t.Fatalf("NewWriter returned error: %v", err)
	}

	if err := writer.Write(context.Background(), outcome); err != nil {
		t.Fatalf("Write returned error: %v", err)
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
	wantEvents := []string{assessment.EventTypeInterpretedOutcome, domainReport.EventTypeGeneratedOutcome, "footprint.report_generated"}
	if len(reportSaver.eventTypes) != len(wantEvents) {
		t.Fatalf("event types = %#v, want %#v", reportSaver.eventTypes, wantEvents)
	}
	for i := range wantEvents {
		if reportSaver.eventTypes[i] != wantEvents[i] {
			t.Fatalf("event types = %#v, want %#v", reportSaver.eventTypes, wantEvents)
		}
	}
}

func TestWriterReportBuilderFailureDoesNotPersistInterpretedAssessment(t *testing.T) {
	order := make([]string, 0)
	a := submittedScaleAssessment(t)
	buildErr := errors.New("report build failed")
	scoreProjectors, _ := NewScoreProjectorRegistry(NewScaleScoreProjector(&resultScoreRepoStub{order: &order}))
	reportBuilders, _ := NewReportBuilderRegistry(&resultReportBuilderStub{
		order: &order,
		err:   buildErr,
	})
	assessmentRepo := &resultAssessmentRepoStub{order: &order}
	writer, err := NewWriter(
		assessmentRepo,
		scoreProjectors,
		reportBuilders,
		&resultReportSaverStub{order: &order},
		&resultNotifierStub{order: &order},
		nil,
	)
	if err != nil {
		t.Fatalf("NewWriter returned error: %v", err)
	}

	err = writer.Write(context.Background(), scaleOutcomeForWriterTest(a))
	if err == nil {
		t.Fatal("Write error = nil, want report build failure")
	}
	if assessmentRepo.saved != nil || a.Status().IsInterpreted() {
		t.Fatalf("assessment should not be persisted or changed when report build fails; saved=%#v status=%s", assessmentRepo.saved, a.Status())
	}
	if len(order) != 1 || order[0] != "report_build" {
		t.Fatalf("order = %#v, want only report_build", order)
	}
}

func TestWriterReportSaveFailureDoesNotPersistInterpretedAssessment(t *testing.T) {
	order := make([]string, 0)
	a := submittedScaleAssessment(t)
	reportErr := errors.New("report save failed")
	scoreProjectors, _ := NewScoreProjectorRegistry(NewScaleScoreProjector(&resultScoreRepoStub{order: &order}))
	reportBuilders, _ := NewReportBuilderRegistry(&resultReportBuilderStub{
		order: &order,
		rpt:   domainReport.NewInterpretReport(domainReport.ID(a.ID()), "Scale", "S-001", 7, domainReport.RiskLevelLow, "ok", nil, nil, nil),
	})
	assessmentRepo := &resultAssessmentRepoStub{order: &order}
	writer, err := NewWriter(
		assessmentRepo,
		scoreProjectors,
		reportBuilders,
		&resultReportSaverStub{order: &order, err: reportErr},
		&resultNotifierStub{order: &order},
		nil,
	)
	if err != nil {
		t.Fatalf("NewWriter returned error: %v", err)
	}

	err = writer.Write(context.Background(), scaleOutcomeForWriterTest(a))
	if err == nil {
		t.Fatal("Write error = nil, want report save failure")
	}
	if assessmentRepo.saved != nil || a.Status().IsInterpreted() {
		t.Fatalf("assessment should not be persisted or changed when report save fails; saved=%#v status=%s", assessmentRepo.saved, a.Status())
	}
	wantOrder := []string{"report_build", "report_save"}
	for i := range wantOrder {
		if order[i] != wantOrder[i] {
			t.Fatalf("order = %#v, want prefix %#v", order, wantOrder)
		}
	}
}

func TestWriterScoreProjectionFailureKeepsAssessmentUninterpreted(t *testing.T) {
	order := make([]string, 0)
	a := submittedScaleAssessment(t)
	scoreErr := errors.New("score save failed")
	scoreRepo := &resultScoreRepoStub{order: &order, err: scoreErr}
	scoreProjectors, _ := NewScoreProjectorRegistry(NewScaleScoreProjector(scoreRepo))
	reportBuilders, _ := NewReportBuilderRegistry(&resultReportBuilderStub{
		order: &order,
		rpt:   domainReport.NewInterpretReport(domainReport.ID(a.ID()), "Scale", "S-001", 7, domainReport.RiskLevelLow, "ok", nil, nil, nil),
	})
	assessmentRepo := &resultAssessmentRepoStub{order: &order}
	notifier := &resultNotifierStub{order: &order}
	writer, err := NewWriter(
		assessmentRepo,
		scoreProjectors,
		reportBuilders,
		&resultReportSaverStub{order: &order},
		notifier,
		nil,
	)
	if err != nil {
		t.Fatalf("NewWriter returned error: %v", err)
	}

	err = writer.Write(context.Background(), scaleOutcomeForWriterTest(a))
	if !errors.Is(err, scoreErr) {
		t.Fatalf("Write error = %v, want score save failure", err)
	}
	if assessmentRepo.saved != nil || a.Status().IsInterpreted() {
		t.Fatalf("assessment should not be persisted or changed when score save fails; saved=%#v status=%s", assessmentRepo.saved, a.Status())
	}
	if notifier.calls != 0 {
		t.Fatalf("notifier calls = %d, want 0", notifier.calls)
	}
	wantOrder := []string{"report_build", "report_save", "score"}
	for i := range wantOrder {
		if order[i] != wantOrder[i] {
			t.Fatalf("order = %#v, want prefix %#v", order, wantOrder)
		}
	}
}

func TestWriterAssessmentSaveFailureDoesNotNotifyWaiter(t *testing.T) {
	order := make([]string, 0)
	a := submittedScaleAssessment(t)
	saveErr := errors.New("assessment save failed")
	scoreProjectors, _ := NewScoreProjectorRegistry(NewScaleScoreProjector(&resultScoreRepoStub{order: &order}))
	reportBuilders, _ := NewReportBuilderRegistry(&resultReportBuilderStub{
		order: &order,
		rpt:   domainReport.NewInterpretReport(domainReport.ID(a.ID()), "Scale", "S-001", 7, domainReport.RiskLevelLow, "ok", nil, nil, nil),
	})
	assessmentRepo := &resultAssessmentRepoStub{order: &order, err: saveErr}
	notifier := &resultNotifierStub{order: &order}
	writer, err := NewWriter(
		assessmentRepo,
		scoreProjectors,
		reportBuilders,
		&resultReportSaverStub{order: &order},
		notifier,
		nil,
	)
	if err != nil {
		t.Fatalf("NewWriter returned error: %v", err)
	}

	err = writer.Write(context.Background(), scaleOutcomeForWriterTest(a))
	if !errors.Is(err, saveErr) {
		t.Fatalf("Write error = %v, want assessment save failure", err)
	}
	if !a.Status().IsInterpreted() {
		t.Fatalf("assessment in memory should be interpreted after ApplyEvaluation; status=%s", a.Status())
	}
	if notifier.calls != 0 {
		t.Fatalf("notifier calls = %d, want 0", notifier.calls)
	}
	wantOrder := []string{"report_build", "report_save", "score", "assessment"}
	if len(order) != len(wantOrder) {
		t.Fatalf("order = %#v, want %#v", order, wantOrder)
	}
	for i := range wantOrder {
		if order[i] != wantOrder[i] {
			t.Fatalf("order = %#v, want %#v", order, wantOrder)
		}
	}
}

func TestWriterUsesGenericEventsAndNoopScoreProjectionForNonScaleOutcome(t *testing.T) {
	order := make([]string, 0)
	modelRef := assessment.NewEvaluationModelRefWithIdentity(
		assessment.EvaluationModelKindPersonality,
		modelcatalog.SubKindTypology,
		modelcatalog.AlgorithmMBTI,
		meta.ID(0),
		meta.NewCode("MBTI-16P"),
		"1.0.0",
		"MBTI",
	)
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
		key:   evaluation.EvaluatorKeyMBTI,
		rpt:   domainReport.NewInterpretReport(domainReport.ID(a.ID()), "MBTI", "MBTI-16P", 0, domainReport.RiskLevelNone, "INTJ", nil, nil, nil),
	})
	if err != nil {
		t.Fatalf("NewReportBuilderRegistry returned error: %v", err)
	}
	reportSaver := &resultReportSaverStub{order: &order}
	writer, err := NewWriter(
		&resultAssessmentRepoStub{order: &order},
		nil,
		reportBuilders,
		reportSaver,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewWriter returned error: %v", err)
	}
	result := assessment.NewModelEvaluationResult(modelRef, assessment.ResultSummary{PrimaryLabel: "INTJ"}, assessment.EvaluationDetail{
		Kind:    assessment.EvaluationModelKindPersonality,
		Payload: "INTJ",
	})

	if err := writer.Write(context.Background(), NewOutcomeFromLegacyResult(a, nil, result)); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	wantOrder := []string{"report_build", "report_save", "assessment"}
	for i := range wantOrder {
		if order[i] != wantOrder[i] {
			t.Fatalf("order = %#v, want prefix %#v", order, wantOrder)
		}
	}
	wantEvents := []string{assessment.EventTypeInterpretedOutcome, domainReport.EventTypeGeneratedOutcome, "footprint.report_generated"}
	if len(reportSaver.eventTypes) != len(wantEvents) {
		t.Fatalf("event types = %#v, want %#v", reportSaver.eventTypes, wantEvents)
	}
	for i := range wantEvents {
		if reportSaver.eventTypes[i] != wantEvents[i] {
			t.Fatalf("event types = %#v, want %#v", reportSaver.eventTypes, wantEvents)
		}
	}
}

func TestNewWriterReturnsEventAssemblerRegistryError(t *testing.T) {
	if _, err := NewWriterWithEventAssemblers(nil, nil, nil, nil, nil, nil, nil); err == nil {
		t.Fatal("NewWriterWithEventAssemblers error = nil, want nil assembler error")
	}
	if _, err := NewWriterWithEventAssemblers(nil, nil, nil, nil, nil, nil, ScaleEventAssembler{}, ScaleEventAssembler{}); err == nil {
		t.Fatal("NewWriterWithEventAssemblers error = nil, want duplicate assembler error")
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
	return NewOutcomeFromLegacyResult(a, &evaluationinput.InputSnapshot{
		MedicalScale: &scalesnapshot.ScaleSnapshot{
			Code:                 "S-001",
			Title:                "Scale",
			QuestionnaireVersion: "1.0.0",
		},
	}, result)
}
