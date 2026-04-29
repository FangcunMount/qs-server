package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type interpretationAssessmentRepoStub struct {
	saved *domainAssessment.Assessment
}

func (r *interpretationAssessmentRepoStub) Save(_ context.Context, a *domainAssessment.Assessment) error {
	r.saved = a
	return nil
}

func (r *interpretationAssessmentRepoStub) SaveWithEvents(_ context.Context, a *domainAssessment.Assessment) error {
	r.saved = a
	a.ClearEvents()
	return nil
}
func (r *interpretationAssessmentRepoStub) SaveWithAdditionalEvents(_ context.Context, a *domainAssessment.Assessment, _ []event.DomainEvent) error {
	r.saved = a
	a.ClearEvents()
	return nil
}

func (r *interpretationAssessmentRepoStub) FindByID(context.Context, domainAssessment.ID) (*domainAssessment.Assessment, error) {
	return nil, nil
}
func (r *interpretationAssessmentRepoStub) Delete(context.Context, domainAssessment.ID) error {
	return nil
}
func (r *interpretationAssessmentRepoStub) FindByAnswerSheetID(context.Context, domainAssessment.AnswerSheetRef) (*domainAssessment.Assessment, error) {
	return nil, nil
}
func (r *interpretationAssessmentRepoStub) FindByTesteeID(context.Context, testee.ID, domainAssessment.Pagination) ([]*domainAssessment.Assessment, int64, error) {
	return nil, 0, nil
}
func (r *interpretationAssessmentRepoStub) FindByTesteeIDWithFilters(context.Context, testee.ID, string, string, string, *time.Time, *time.Time, domainAssessment.Pagination) ([]*domainAssessment.Assessment, int64, error) {
	return nil, 0, nil
}
func (r *interpretationAssessmentRepoStub) FindByTesteeIDAndScaleID(context.Context, testee.ID, domainAssessment.MedicalScaleRef, domainAssessment.Pagination) ([]*domainAssessment.Assessment, int64, error) {
	return nil, 0, nil
}
func (r *interpretationAssessmentRepoStub) FindByPlanID(context.Context, string, domainAssessment.Pagination) ([]*domainAssessment.Assessment, int64, error) {
	return nil, 0, nil
}
func (r *interpretationAssessmentRepoStub) CountByStatus(context.Context, domainAssessment.Status) (int64, error) {
	return 0, nil
}
func (r *interpretationAssessmentRepoStub) CountByTesteeIDAndStatus(context.Context, testee.ID, domainAssessment.Status) (int64, error) {
	return 0, nil
}
func (r *interpretationAssessmentRepoStub) CountByOrgIDAndStatus(context.Context, int64, domainAssessment.Status) (int64, error) {
	return 0, nil
}
func (r *interpretationAssessmentRepoStub) FindByIDs(context.Context, []domainAssessment.ID) ([]*domainAssessment.Assessment, error) {
	return nil, nil
}
func (r *interpretationAssessmentRepoStub) FindPendingSubmission(context.Context, domainAssessment.Pagination) ([]*domainAssessment.Assessment, int64, error) {
	return nil, 0, nil
}
func (r *interpretationAssessmentRepoStub) FindByOrgID(context.Context, int64, *domainAssessment.Status, domainAssessment.Pagination) ([]*domainAssessment.Assessment, int64, error) {
	return nil, 0, nil
}
func (r *interpretationAssessmentRepoStub) FindByOrgIDAndTesteeIDs(context.Context, int64, []testee.ID, *domainAssessment.Status, domainAssessment.Pagination) ([]*domainAssessment.Assessment, int64, error) {
	return nil, 0, nil
}

type interpretationReportRepoStub struct {
	report      *domainReport.InterpretReport
	testeeID    testee.ID
	stagedTypes []string
}

func (r *interpretationReportRepoStub) Save(context.Context, *domainReport.InterpretReport) error {
	return nil
}
func (r *interpretationReportRepoStub) SaveWithTesteeAndEvents(_ context.Context, report *domainReport.InterpretReport, testeeID testee.ID, events []event.DomainEvent) error {
	r.report = report
	r.testeeID = testeeID
	r.stagedTypes = r.stagedTypes[:0]
	for _, evt := range events {
		r.stagedTypes = append(r.stagedTypes, evt.EventType())
	}
	return nil
}
func (r *interpretationReportRepoStub) SaveReportDurably(ctx context.Context, report *domainReport.InterpretReport, testeeID testee.ID, events []event.DomainEvent) error {
	return r.SaveWithTesteeAndEvents(ctx, report, testeeID, events)
}
func (r *interpretationReportRepoStub) FindByID(context.Context, domainReport.ID) (*domainReport.InterpretReport, error) {
	return nil, nil
}
func (r *interpretationReportRepoStub) FindByAssessmentID(context.Context, domainReport.AssessmentID) (*domainReport.InterpretReport, error) {
	return nil, nil
}
func (r *interpretationReportRepoStub) FindByTesteeID(context.Context, testee.ID, domainReport.Pagination) ([]*domainReport.InterpretReport, int64, error) {
	return nil, 0, nil
}
func (r *interpretationReportRepoStub) FindByTesteeIDs(context.Context, []testee.ID, domainReport.Pagination) ([]*domainReport.InterpretReport, int64, error) {
	return nil, 0, nil
}
func (r *interpretationReportRepoStub) Update(context.Context, *domainReport.InterpretReport) error {
	return nil
}
func (r *interpretationReportRepoStub) Delete(context.Context, domainReport.ID) error { return nil }
func (r *interpretationReportRepoStub) ExistsByID(context.Context, domainReport.ID) (bool, error) {
	return false, nil
}

func TestInterpretationHandlerStagesInterpretedAndReportGeneratedInOrder(t *testing.T) {
	assessmentID := domainAssessment.NewID(7001)
	testeeID := testee.NewID(8001)
	scaleRef := domainAssessment.NewMedicalScaleRef(meta.FromUint64(9001), meta.NewCode("scale-code"), "scale-name")

	a, err := domainAssessment.NewAssessment(
		1,
		testeeID,
		domainAssessment.NewQuestionnaireRefByCode(meta.NewCode("q-code"), "v1"),
		domainAssessment.NewAnswerSheetRef(meta.FromUint64(6001)),
		domainAssessment.NewAdhocOrigin(),
		domainAssessment.WithID(assessmentID),
		domainAssessment.WithMedicalScale(scaleRef),
	)
	if err != nil {
		t.Fatalf("NewAssessment returned error: %v", err)
	}
	if err := a.Submit(); err != nil {
		t.Fatalf("Submit returned error: %v", err)
	}
	a.ClearEvents()

	rpt := domainReport.NewInterpretReport(
		domainReport.ID(assessmentID),
		"Scale",
		"scale-code",
		88,
		domainReport.RiskLevelHigh,
		"high risk",
		nil,
		nil,
	)

	assessmentRepo := &interpretationAssessmentRepoStub{}
	reportRepo := &interpretationReportRepoStub{}
	handler := &InterpretationHandler{
		BaseHandler:    NewBaseHandler("InterpretationHandler"),
		assessmentRepo: assessmentRepo,
		reportSaver:    reportRepo,
		reportBuilder:  &reportBuilderAdapter{report: rpt},
	}

	evalCtx := NewContext(a, nil, nil)
	evalCtx.TotalScore = 88
	evalCtx.RiskLevel = domainAssessment.RiskLevelHigh
	evalCtx.Conclusion = "high risk"
	evalCtx.Suggestion = "follow up"

	if err := handler.Handle(context.Background(), evalCtx); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	if assessmentRepo.saved == nil || !assessmentRepo.saved.Status().IsInterpreted() {
		t.Fatalf("expected interpreted assessment to be saved")
	}
	if reportRepo.report == nil {
		t.Fatalf("expected report to be saved")
	}
	if reportRepo.testeeID != testeeID {
		t.Fatalf("expected report save to carry testee id %d, got %d", testeeID, reportRepo.testeeID)
	}
	if len(reportRepo.stagedTypes) != 3 {
		t.Fatalf("expected three staged events, got %d", len(reportRepo.stagedTypes))
	}
	if reportRepo.stagedTypes[0] != domainAssessment.EventTypeInterpreted {
		t.Fatalf("expected first staged event to be assessment.interpreted, got %s", reportRepo.stagedTypes[0])
	}
	if reportRepo.stagedTypes[1] != domainReport.EventTypeGenerated {
		t.Fatalf("expected second staged event to be report.generated, got %s", reportRepo.stagedTypes[1])
	}
	if reportRepo.stagedTypes[2] != "footprint.report_generated" {
		t.Fatalf("expected third staged event to be footprint.report_generated, got %s", reportRepo.stagedTypes[2])
	}
}

type reportBuilderAdapter struct {
	report *domainReport.InterpretReport
}

func (b *reportBuilderAdapter) Build(_ *domainAssessment.Assessment, _ *domainScale.MedicalScale, _ *domainAssessment.EvaluationResult) (*domainReport.InterpretReport, error) {
	return b.report, nil
}
