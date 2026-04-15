package engine

import (
	"context"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	domainAnswerSheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type fakeAssessmentRepo struct {
	assessment *domainAssessment.Assessment
	saveCalls  int
}

func (r *fakeAssessmentRepo) Save(_ context.Context, assessment *domainAssessment.Assessment) error {
	r.assessment = assessment
	r.saveCalls++
	return nil
}

func (r *fakeAssessmentRepo) FindByID(_ context.Context, _ domainAssessment.ID) (*domainAssessment.Assessment, error) {
	return r.assessment, nil
}

func (r *fakeAssessmentRepo) Delete(_ context.Context, _ domainAssessment.ID) error { return nil }
func (r *fakeAssessmentRepo) FindByAnswerSheetID(_ context.Context, _ domainAssessment.AnswerSheetRef) (*domainAssessment.Assessment, error) {
	return nil, nil
}
func (r *fakeAssessmentRepo) FindByTesteeID(_ context.Context, _ testee.ID, _ domainAssessment.Pagination) ([]*domainAssessment.Assessment, int64, error) {
	return nil, 0, nil
}
func (r *fakeAssessmentRepo) FindByTesteeIDWithFilters(_ context.Context, _ testee.ID, _ string, _ string, _ string, _ *time.Time, _ *time.Time, _ domainAssessment.Pagination) ([]*domainAssessment.Assessment, int64, error) {
	return nil, 0, nil
}
func (r *fakeAssessmentRepo) FindByTesteeIDAndScaleID(_ context.Context, _ testee.ID, _ domainAssessment.MedicalScaleRef, _ domainAssessment.Pagination) ([]*domainAssessment.Assessment, int64, error) {
	return nil, 0, nil
}
func (r *fakeAssessmentRepo) FindByPlanID(_ context.Context, _ string, _ domainAssessment.Pagination) ([]*domainAssessment.Assessment, int64, error) {
	return nil, 0, nil
}
func (r *fakeAssessmentRepo) CountByStatus(_ context.Context, _ domainAssessment.Status) (int64, error) {
	return 0, nil
}
func (r *fakeAssessmentRepo) CountByTesteeIDAndStatus(_ context.Context, _ testee.ID, _ domainAssessment.Status) (int64, error) {
	return 0, nil
}
func (r *fakeAssessmentRepo) CountByOrgIDAndStatus(_ context.Context, _ int64, _ domainAssessment.Status) (int64, error) {
	return 0, nil
}
func (r *fakeAssessmentRepo) FindByIDs(_ context.Context, _ []domainAssessment.ID) ([]*domainAssessment.Assessment, error) {
	return nil, nil
}
func (r *fakeAssessmentRepo) FindPendingSubmission(_ context.Context, _ domainAssessment.Pagination) ([]*domainAssessment.Assessment, int64, error) {
	return nil, 0, nil
}
func (r *fakeAssessmentRepo) FindByOrgID(_ context.Context, _ int64, _ *domainAssessment.Status, _ domainAssessment.Pagination) ([]*domainAssessment.Assessment, int64, error) {
	return nil, 0, nil
}
func (r *fakeAssessmentRepo) FindByOrgIDAndTesteeIDs(_ context.Context, _ int64, _ []testee.ID, _ *domainAssessment.Status, _ domainAssessment.Pagination) ([]*domainAssessment.Assessment, int64, error) {
	return nil, 0, nil
}

type fakeScaleRepo struct {
	scale *domainScale.MedicalScale
}

func (r *fakeScaleRepo) Create(_ context.Context, _ *domainScale.MedicalScale) error { return nil }
func (r *fakeScaleRepo) FindByCode(_ context.Context, _ string) (*domainScale.MedicalScale, error) {
	return r.scale, nil
}
func (r *fakeScaleRepo) FindByQuestionnaireCode(_ context.Context, _ string) (*domainScale.MedicalScale, error) {
	return nil, nil
}
func (r *fakeScaleRepo) FindSummaryList(_ context.Context, _ int, _ int, _ map[string]interface{}) ([]*domainScale.MedicalScale, error) {
	return nil, nil
}
func (r *fakeScaleRepo) CountWithConditions(_ context.Context, _ map[string]interface{}) (int64, error) {
	return 0, nil
}
func (r *fakeScaleRepo) Update(_ context.Context, _ *domainScale.MedicalScale) error { return nil }
func (r *fakeScaleRepo) Remove(_ context.Context, _ string) error                    { return nil }
func (r *fakeScaleRepo) ExistsByCode(_ context.Context, _ string) (bool, error)      { return false, nil }

type fakeAnswerSheetRepo struct {
	answerSheet *domainAnswerSheet.AnswerSheet
}

func (r *fakeAnswerSheetRepo) Create(_ context.Context, _ *domainAnswerSheet.AnswerSheet) error {
	return nil
}
func (r *fakeAnswerSheetRepo) Update(_ context.Context, _ *domainAnswerSheet.AnswerSheet) error {
	return nil
}
func (r *fakeAnswerSheetRepo) FindByID(_ context.Context, _ meta.ID) (*domainAnswerSheet.AnswerSheet, error) {
	return r.answerSheet, nil
}
func (r *fakeAnswerSheetRepo) FindSummaryListByFiller(_ context.Context, _ uint64, _ int, _ int) ([]*domainAnswerSheet.AnswerSheetSummary, error) {
	return nil, nil
}
func (r *fakeAnswerSheetRepo) FindSummaryListByQuestionnaire(_ context.Context, _ string, _ int, _ int) ([]*domainAnswerSheet.AnswerSheetSummary, error) {
	return nil, nil
}
func (r *fakeAnswerSheetRepo) CountByFiller(_ context.Context, _ uint64) (int64, error) {
	return 0, nil
}
func (r *fakeAnswerSheetRepo) CountByQuestionnaire(_ context.Context, _ string) (int64, error) {
	return 0, nil
}
func (r *fakeAnswerSheetRepo) CountWithConditions(_ context.Context, _ map[string]interface{}) (int64, error) {
	return 0, nil
}
func (r *fakeAnswerSheetRepo) Delete(_ context.Context, _ meta.ID) error { return nil }

type fakeQuestionnaireRepo struct{}

func (r *fakeQuestionnaireRepo) Create(_ context.Context, _ *domainQuestionnaire.Questionnaire) error {
	return nil
}
func (r *fakeQuestionnaireRepo) FindByCode(_ context.Context, _ string) (*domainQuestionnaire.Questionnaire, error) {
	return nil, nil
}
func (r *fakeQuestionnaireRepo) FindPublishedByCode(_ context.Context, _ string) (*domainQuestionnaire.Questionnaire, error) {
	return nil, nil
}
func (r *fakeQuestionnaireRepo) FindLatestPublishedByCode(_ context.Context, _ string) (*domainQuestionnaire.Questionnaire, error) {
	return nil, nil
}
func (r *fakeQuestionnaireRepo) FindByCodeVersion(_ context.Context, _ string, _ string) (*domainQuestionnaire.Questionnaire, error) {
	return nil, nil
}
func (r *fakeQuestionnaireRepo) FindBaseByCode(_ context.Context, _ string) (*domainQuestionnaire.Questionnaire, error) {
	return nil, nil
}
func (r *fakeQuestionnaireRepo) FindBasePublishedByCode(_ context.Context, _ string) (*domainQuestionnaire.Questionnaire, error) {
	return nil, nil
}
func (r *fakeQuestionnaireRepo) FindBaseByCodeVersion(_ context.Context, _ string, _ string) (*domainQuestionnaire.Questionnaire, error) {
	return nil, nil
}
func (r *fakeQuestionnaireRepo) LoadQuestions(_ context.Context, _ *domainQuestionnaire.Questionnaire) error {
	return nil
}
func (r *fakeQuestionnaireRepo) FindBaseList(_ context.Context, _ int, _ int, _ map[string]interface{}) ([]*domainQuestionnaire.Questionnaire, error) {
	return nil, nil
}
func (r *fakeQuestionnaireRepo) FindBasePublishedList(_ context.Context, _ int, _ int, _ map[string]interface{}) ([]*domainQuestionnaire.Questionnaire, error) {
	return nil, nil
}
func (r *fakeQuestionnaireRepo) CountWithConditions(_ context.Context, _ map[string]interface{}) (int64, error) {
	return 0, nil
}
func (r *fakeQuestionnaireRepo) CountPublishedWithConditions(_ context.Context, _ map[string]interface{}) (int64, error) {
	return 0, nil
}
func (r *fakeQuestionnaireRepo) Update(_ context.Context, _ *domainQuestionnaire.Questionnaire) error {
	return nil
}
func (r *fakeQuestionnaireRepo) CreatePublishedSnapshot(_ context.Context, _ *domainQuestionnaire.Questionnaire, _ bool) error {
	return nil
}
func (r *fakeQuestionnaireRepo) SetActivePublishedVersion(_ context.Context, _ string, _ string) error {
	return nil
}
func (r *fakeQuestionnaireRepo) ClearActivePublishedVersion(_ context.Context, _ string) error {
	return nil
}
func (r *fakeQuestionnaireRepo) Remove(_ context.Context, _ string) error     { return nil }
func (r *fakeQuestionnaireRepo) HardDelete(_ context.Context, _ string) error { return nil }
func (r *fakeQuestionnaireRepo) HardDeleteFamily(_ context.Context, _ string) error {
	return nil
}
func (r *fakeQuestionnaireRepo) ExistsByCode(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (r *fakeQuestionnaireRepo) HasPublishedSnapshots(_ context.Context, _ string) (bool, error) {
	return false, nil
}

var _ domainAssessment.ScoreRepository = (*noopScoreRepo)(nil)
var _ domainReport.ReportRepository = (*noopReportRepo)(nil)

type noopScoreRepo struct{}

func (r *noopScoreRepo) SaveScores(_ context.Context, _ []*domainAssessment.AssessmentScore) error {
	return nil
}
func (r *noopScoreRepo) SaveScoresWithContext(_ context.Context, _ *domainAssessment.Assessment, _ *domainAssessment.AssessmentScore) error {
	return nil
}
func (r *noopScoreRepo) FindByAssessmentID(_ context.Context, _ domainAssessment.ID) ([]*domainAssessment.AssessmentScore, error) {
	return nil, nil
}
func (r *noopScoreRepo) FindByTesteeIDAndFactorCode(_ context.Context, _ testee.ID, _ domainAssessment.FactorCode, _ int) ([]*domainAssessment.AssessmentScore, error) {
	return nil, nil
}
func (r *noopScoreRepo) FindLatestByTesteeIDAndScaleID(_ context.Context, _ testee.ID, _ domainAssessment.MedicalScaleRef) ([]*domainAssessment.AssessmentScore, error) {
	return nil, nil
}
func (r *noopScoreRepo) DeleteByAssessmentID(_ context.Context, _ domainAssessment.ID) error {
	return nil
}

type noopReportRepo struct{}

func (r *noopReportRepo) Save(_ context.Context, _ *domainReport.InterpretReport) error { return nil }
func (r *noopReportRepo) FindByID(_ context.Context, _ domainReport.ID) (*domainReport.InterpretReport, error) {
	return nil, nil
}
func (r *noopReportRepo) FindByAssessmentID(_ context.Context, _ domainReport.AssessmentID) (*domainReport.InterpretReport, error) {
	return nil, nil
}
func (r *noopReportRepo) FindByTesteeID(_ context.Context, _ testee.ID, _ domainReport.Pagination) ([]*domainReport.InterpretReport, int64, error) {
	return nil, 0, nil
}
func (r *noopReportRepo) FindByTesteeIDs(_ context.Context, _ []testee.ID, _ domainReport.Pagination) ([]*domainReport.InterpretReport, int64, error) {
	return nil, 0, nil
}
func (r *noopReportRepo) Update(_ context.Context, _ *domainReport.InterpretReport) error { return nil }
func (r *noopReportRepo) Delete(_ context.Context, _ domainReport.ID) error               { return nil }
func (r *noopReportRepo) ExistsByID(_ context.Context, _ domainReport.ID) (bool, error) {
	return false, nil
}

func TestEvaluateFailsWhenQuestionnaireVersionDoesNotResolveCurrentQuestionnaire(t *testing.T) {
	aRepo := &fakeAssessmentRepo{
		assessment: domainAssessment.Reconstruct(
			meta.FromUint64(101),
			1,
			testee.NewID(202),
			domainAssessment.NewQuestionnaireRefByCode(meta.NewCode("Q-001"), "0.9.0"),
			domainAssessment.NewAnswerSheetRef(meta.FromUint64(303)),
			ptr(domainAssessment.NewMedicalScaleRef(meta.FromUint64(404), meta.NewCode("S-001"), "Scale")),
			domainAssessment.NewAdhocOrigin(),
			domainAssessment.StatusSubmitted,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
		),
	}

	scaleDomain, err := domainScale.NewMedicalScale(
		meta.NewCode("S-001"),
		"Scale",
		domainScale.WithQuestionnaire(meta.NewCode("Q-001"), "0.9.0"),
		domainScale.WithStatus(domainScale.StatusPublished),
	)
	if err != nil {
		t.Fatalf("NewMedicalScale() error = %v", err)
	}

	answerSheet := domainAnswerSheet.Reconstruct(
		meta.FromUint64(303),
		domainAnswerSheet.NewQuestionnaireRef("Q-001", "0.9.0", "Questionnaire"),
		nil,
		nil,
		time.Now(),
		0,
	)

	svc := &service{
		assessmentRepo:    aRepo,
		scoreRepo:         &noopScoreRepo{},
		reportRepo:        &noopReportRepo{},
		scaleRepo:         &fakeScaleRepo{scale: scaleDomain},
		answerSheetRepo:   &fakeAnswerSheetRepo{answerSheet: answerSheet},
		questionnaireRepo: &fakeQuestionnaireRepo{},
	}

	err = svc.Evaluate(context.Background(), 101)
	if err == nil {
		t.Fatal("Evaluate() error = nil, want questionnaire version failure")
	}
	if !aRepo.assessment.Status().IsFailed() {
		t.Fatalf("assessment status = %s, want failed", aRepo.assessment.Status())
	}
	if aRepo.saveCalls == 0 {
		t.Fatal("assessment should be persisted after markAsFailed")
	}
}

func ptr[T any](v T) *T {
	return &v
}
