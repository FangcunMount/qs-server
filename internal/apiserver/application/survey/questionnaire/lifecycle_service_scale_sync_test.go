package questionnaire

import (
	"context"
	"testing"

	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"go.mongodb.org/mongo-driver/mongo"
)

type questionnaireRepoSyncStub struct {
	byCode map[string]*domainQuestionnaire.Questionnaire
}

func (r *questionnaireRepoSyncStub) Create(_ context.Context, _ *domainQuestionnaire.Questionnaire) error {
	return nil
}
func (r *questionnaireRepoSyncStub) FindByCode(_ context.Context, code string) (*domainQuestionnaire.Questionnaire, error) {
	if q, ok := r.byCode[code]; ok {
		return q, nil
	}
	return nil, mongo.ErrNoDocuments
}
func (r *questionnaireRepoSyncStub) FindPublishedByCode(_ context.Context, _ string) (*domainQuestionnaire.Questionnaire, error) {
	return nil, mongo.ErrNoDocuments
}
func (r *questionnaireRepoSyncStub) FindLatestPublishedByCode(_ context.Context, _ string) (*domainQuestionnaire.Questionnaire, error) {
	return nil, mongo.ErrNoDocuments
}
func (r *questionnaireRepoSyncStub) FindByCodeVersion(_ context.Context, _ string, _ string) (*domainQuestionnaire.Questionnaire, error) {
	return nil, mongo.ErrNoDocuments
}
func (r *questionnaireRepoSyncStub) FindBaseByCode(_ context.Context, _ string) (*domainQuestionnaire.Questionnaire, error) {
	return nil, nil
}
func (r *questionnaireRepoSyncStub) FindBasePublishedByCode(_ context.Context, _ string) (*domainQuestionnaire.Questionnaire, error) {
	return nil, nil
}
func (r *questionnaireRepoSyncStub) FindBaseByCodeVersion(_ context.Context, _ string, _ string) (*domainQuestionnaire.Questionnaire, error) {
	return nil, nil
}
func (r *questionnaireRepoSyncStub) LoadQuestions(_ context.Context, _ *domainQuestionnaire.Questionnaire) error {
	return nil
}
func (r *questionnaireRepoSyncStub) FindBaseList(_ context.Context, _ int, _ int, _ map[string]interface{}) ([]*domainQuestionnaire.Questionnaire, error) {
	return nil, nil
}
func (r *questionnaireRepoSyncStub) FindBasePublishedList(_ context.Context, _ int, _ int, _ map[string]interface{}) ([]*domainQuestionnaire.Questionnaire, error) {
	return nil, nil
}
func (r *questionnaireRepoSyncStub) CountWithConditions(_ context.Context, _ map[string]interface{}) (int64, error) {
	return 0, nil
}
func (r *questionnaireRepoSyncStub) CountPublishedWithConditions(_ context.Context, _ map[string]interface{}) (int64, error) {
	return 0, nil
}
func (r *questionnaireRepoSyncStub) Update(_ context.Context, _ *domainQuestionnaire.Questionnaire) error {
	return nil
}
func (r *questionnaireRepoSyncStub) CreatePublishedSnapshot(_ context.Context, _ *domainQuestionnaire.Questionnaire, _ bool) error {
	return nil
}
func (r *questionnaireRepoSyncStub) SetActivePublishedVersion(_ context.Context, _ string, _ string) error {
	return nil
}
func (r *questionnaireRepoSyncStub) ClearActivePublishedVersion(_ context.Context, _ string) error {
	return nil
}
func (r *questionnaireRepoSyncStub) Remove(_ context.Context, _ string) error { return nil }
func (r *questionnaireRepoSyncStub) HardDelete(_ context.Context, _ string) error {
	return nil
}
func (r *questionnaireRepoSyncStub) HardDeleteFamily(_ context.Context, _ string) error {
	return nil
}
func (r *questionnaireRepoSyncStub) ExistsByCode(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (r *questionnaireRepoSyncStub) HasPublishedSnapshots(_ context.Context, _ string) (bool, error) {
	return false, nil
}

type scaleRepoSyncStub struct {
	item        *domainScale.MedicalScale
	updateCalls int
}

func (r *scaleRepoSyncStub) Create(_ context.Context, _ *domainScale.MedicalScale) error { return nil }
func (r *scaleRepoSyncStub) FindByCode(_ context.Context, _ string) (*domainScale.MedicalScale, error) {
	return nil, mongo.ErrNoDocuments
}
func (r *scaleRepoSyncStub) FindByQuestionnaireCode(_ context.Context, _ string) (*domainScale.MedicalScale, error) {
	if r.item == nil {
		return nil, mongo.ErrNoDocuments
	}
	return r.item, nil
}
func (r *scaleRepoSyncStub) FindSummaryList(_ context.Context, _ int, _ int, _ map[string]interface{}) ([]*domainScale.MedicalScale, error) {
	return nil, nil
}
func (r *scaleRepoSyncStub) CountWithConditions(_ context.Context, _ map[string]interface{}) (int64, error) {
	return 0, nil
}
func (r *scaleRepoSyncStub) Update(_ context.Context, item *domainScale.MedicalScale) error {
	r.item = item
	r.updateCalls++
	return nil
}
func (r *scaleRepoSyncStub) Remove(_ context.Context, _ string) error { return nil }
func (r *scaleRepoSyncStub) ExistsByCode(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func TestSyncScaleQuestionnaireVersionUpdatesSingleMedicalScaleBinding(t *testing.T) {
	ctx := context.Background()
	q, err := domainQuestionnaire.NewQuestionnaire(
		meta.NewCode("Q-MS"),
		"Medical",
		domainQuestionnaire.WithType(domainQuestionnaire.TypeMedicalScale),
	)
	if err != nil {
		t.Fatalf("NewQuestionnaire() error = %v", err)
	}
	scaleItem, err := domainScale.NewMedicalScale(
		meta.NewCode("S-001"),
		"Scale",
		domainScale.WithQuestionnaire(meta.NewCode("Q-MS"), "1.0"),
	)
	if err != nil {
		t.Fatalf("NewMedicalScale() error = %v", err)
	}
	scaleRepo := &scaleRepoSyncStub{item: scaleItem}
	svc := &lifecycleService{
		repo: &questionnaireRepoSyncStub{
			byCode: map[string]*domainQuestionnaire.Questionnaire{
				"Q-MS": q,
			},
		},
		scaleRepo: scaleRepo,
	}

	if err := svc.syncScaleQuestionnaireVersion(ctx, "Q-MS", "2.0"); err != nil {
		t.Fatalf("syncScaleQuestionnaireVersion() error = %v", err)
	}
	if scaleRepo.updateCalls != 1 {
		t.Fatalf("syncScaleQuestionnaireVersion() updateCalls = %d, want 1", scaleRepo.updateCalls)
	}
	if got := scaleRepo.item.GetQuestionnaireVersion(); got != "2.0" {
		t.Fatalf("syncScaleQuestionnaireVersion() questionnaire version = %q, want %q", got, "2.0")
	}
}

func TestSyncScaleQuestionnaireVersionSkipsSurveyQuestionnaire(t *testing.T) {
	ctx := context.Background()
	q, err := domainQuestionnaire.NewQuestionnaire(
		meta.NewCode("Q-SURVEY"),
		"Survey",
		domainQuestionnaire.WithType(domainQuestionnaire.TypeSurvey),
	)
	if err != nil {
		t.Fatalf("NewQuestionnaire() error = %v", err)
	}
	scaleItem, err := domainScale.NewMedicalScale(
		meta.NewCode("S-001"),
		"Scale",
		domainScale.WithQuestionnaire(meta.NewCode("Q-SURVEY"), "1.0"),
	)
	if err != nil {
		t.Fatalf("NewMedicalScale() error = %v", err)
	}
	scaleRepo := &scaleRepoSyncStub{item: scaleItem}
	svc := &lifecycleService{
		repo: &questionnaireRepoSyncStub{
			byCode: map[string]*domainQuestionnaire.Questionnaire{
				"Q-SURVEY": q,
			},
		},
		scaleRepo: scaleRepo,
	}

	if err := svc.syncScaleQuestionnaireVersion(ctx, "Q-SURVEY", "2.0"); err != nil {
		t.Fatalf("syncScaleQuestionnaireVersion() error = %v", err)
	}
	if scaleRepo.updateCalls != 0 {
		t.Fatalf("syncScaleQuestionnaireVersion() updateCalls = %d, want 0", scaleRepo.updateCalls)
	}
	if got := scaleRepo.item.GetQuestionnaireVersion(); got != "1.0" {
		t.Fatalf("syncScaleQuestionnaireVersion() questionnaire version = %q, want %q", got, "1.0")
	}
}
