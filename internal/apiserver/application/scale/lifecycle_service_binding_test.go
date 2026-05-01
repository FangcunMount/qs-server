package scale

import (
	"context"
	"testing"

	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type scaleRepoBindingStub struct {
	boundByQuestionnaire map[string]*domainScale.MedicalScale
}

func (r *scaleRepoBindingStub) Create(_ context.Context, _ *domainScale.MedicalScale) error {
	return nil
}
func (r *scaleRepoBindingStub) FindByCode(_ context.Context, _ string) (*domainScale.MedicalScale, error) {
	return nil, domainScale.ErrNotFound
}
func (r *scaleRepoBindingStub) FindByQuestionnaireCode(_ context.Context, questionnaireCode string) (*domainScale.MedicalScale, error) {
	if scale, ok := r.boundByQuestionnaire[questionnaireCode]; ok {
		return scale, nil
	}
	return nil, domainScale.ErrNotFound
}
func (r *scaleRepoBindingStub) FindSummaryList(_ context.Context, _ int, _ int, _ map[string]interface{}) ([]*domainScale.MedicalScale, error) {
	return nil, nil
}
func (r *scaleRepoBindingStub) CountWithConditions(_ context.Context, _ map[string]interface{}) (int64, error) {
	return 0, nil
}
func (r *scaleRepoBindingStub) Update(_ context.Context, _ *domainScale.MedicalScale) error {
	return nil
}
func (r *scaleRepoBindingStub) Remove(_ context.Context, _ string) error { return nil }
func (r *scaleRepoBindingStub) ExistsByCode(_ context.Context, _ string) (bool, error) {
	return false, nil
}

type questionnaireRepoBindingStub struct {
	byCode    map[string]*domainQuestionnaire.Questionnaire
	byVersion map[string]*domainQuestionnaire.Questionnaire
}

func (r *questionnaireRepoBindingStub) Create(_ context.Context, _ *domainQuestionnaire.Questionnaire) error {
	return nil
}
func (r *questionnaireRepoBindingStub) FindByCode(_ context.Context, code string) (*domainQuestionnaire.Questionnaire, error) {
	if q, ok := r.byCode[code]; ok {
		return q, nil
	}
	return nil, domainQuestionnaire.ErrNotFound
}
func (r *questionnaireRepoBindingStub) FindPublishedByCode(_ context.Context, _ string) (*domainQuestionnaire.Questionnaire, error) {
	return nil, domainQuestionnaire.ErrNotFound
}
func (r *questionnaireRepoBindingStub) FindLatestPublishedByCode(_ context.Context, _ string) (*domainQuestionnaire.Questionnaire, error) {
	return nil, domainQuestionnaire.ErrNotFound
}
func (r *questionnaireRepoBindingStub) FindByCodeVersion(_ context.Context, code, version string) (*domainQuestionnaire.Questionnaire, error) {
	if q, ok := r.byVersion[code+":"+version]; ok {
		return q, nil
	}
	return nil, domainQuestionnaire.ErrNotFound
}
func (r *questionnaireRepoBindingStub) FindBaseByCode(_ context.Context, _ string) (*domainQuestionnaire.Questionnaire, error) {
	return nil, nil
}
func (r *questionnaireRepoBindingStub) FindBasePublishedByCode(_ context.Context, _ string) (*domainQuestionnaire.Questionnaire, error) {
	return nil, nil
}
func (r *questionnaireRepoBindingStub) FindBaseByCodeVersion(_ context.Context, _ string, _ string) (*domainQuestionnaire.Questionnaire, error) {
	return nil, nil
}
func (r *questionnaireRepoBindingStub) LoadQuestions(_ context.Context, _ *domainQuestionnaire.Questionnaire) error {
	return nil
}
func (r *questionnaireRepoBindingStub) FindBaseList(_ context.Context, _ int, _ int, _ map[string]interface{}) ([]*domainQuestionnaire.Questionnaire, error) {
	return nil, nil
}
func (r *questionnaireRepoBindingStub) FindBasePublishedList(_ context.Context, _ int, _ int, _ map[string]interface{}) ([]*domainQuestionnaire.Questionnaire, error) {
	return nil, nil
}
func (r *questionnaireRepoBindingStub) CountWithConditions(_ context.Context, _ map[string]interface{}) (int64, error) {
	return 0, nil
}
func (r *questionnaireRepoBindingStub) CountPublishedWithConditions(_ context.Context, _ map[string]interface{}) (int64, error) {
	return 0, nil
}
func (r *questionnaireRepoBindingStub) Update(_ context.Context, _ *domainQuestionnaire.Questionnaire) error {
	return nil
}
func (r *questionnaireRepoBindingStub) CreatePublishedSnapshot(_ context.Context, _ *domainQuestionnaire.Questionnaire, _ bool) error {
	return nil
}
func (r *questionnaireRepoBindingStub) SetActivePublishedVersion(_ context.Context, _ string, _ string) error {
	return nil
}
func (r *questionnaireRepoBindingStub) ClearActivePublishedVersion(_ context.Context, _ string) error {
	return nil
}
func (r *questionnaireRepoBindingStub) Remove(_ context.Context, _ string) error { return nil }
func (r *questionnaireRepoBindingStub) HardDelete(_ context.Context, _ string) error {
	return nil
}
func (r *questionnaireRepoBindingStub) HardDeleteFamily(_ context.Context, _ string) error {
	return nil
}
func (r *questionnaireRepoBindingStub) ExistsByCode(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (r *questionnaireRepoBindingStub) HasPublishedSnapshots(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func TestValidateMedicalScaleQuestionnaireBindingRejectsSurveyQuestionnaire(t *testing.T) {
	ctx := context.Background()
	q, err := domainQuestionnaire.NewQuestionnaire(
		meta.NewCode("Q-SURVEY"),
		"Survey",
		domainQuestionnaire.WithType(domainQuestionnaire.TypeSurvey),
	)
	if err != nil {
		t.Fatalf("NewQuestionnaire() error = %v", err)
	}

	svc := &lifecycleService{
		repo: &scaleRepoBindingStub{},
		questionnaireRepo: &questionnaireRepoBindingStub{
			byCode: map[string]*domainQuestionnaire.Questionnaire{
				"Q-SURVEY": q,
			},
		},
	}

	err = svc.validateMedicalScaleQuestionnaireBinding(ctx, "Q-SURVEY", "", "S-001")
	if err == nil {
		t.Fatal("validateMedicalScaleQuestionnaireBinding() error = nil, want non-nil")
	}
}

func TestValidateMedicalScaleQuestionnaireBindingRejectsOtherScaleBinding(t *testing.T) {
	ctx := context.Background()
	q, err := domainQuestionnaire.NewQuestionnaire(
		meta.NewCode("Q-MS"),
		"Medical",
		domainQuestionnaire.WithType(domainQuestionnaire.TypeMedicalScale),
		domainQuestionnaire.WithVersion(domainQuestionnaire.NewVersion("1.0")),
	)
	if err != nil {
		t.Fatalf("NewQuestionnaire() error = %v", err)
	}
	otherScale, err := domainScale.NewMedicalScale(
		meta.NewCode("S-OTHER"),
		"Other Scale",
		domainScale.WithQuestionnaire(meta.NewCode("Q-MS"), "1.0"),
	)
	if err != nil {
		t.Fatalf("NewMedicalScale() error = %v", err)
	}

	svc := &lifecycleService{
		repo: &scaleRepoBindingStub{
			boundByQuestionnaire: map[string]*domainScale.MedicalScale{
				"Q-MS": otherScale,
			},
		},
		questionnaireRepo: &questionnaireRepoBindingStub{
			byCode: map[string]*domainQuestionnaire.Questionnaire{
				"Q-MS": q,
			},
			byVersion: map[string]*domainQuestionnaire.Questionnaire{
				"Q-MS:1.0": q,
			},
		},
	}

	err = svc.validateMedicalScaleQuestionnaireBinding(ctx, "Q-MS", "1.0", "S-001")
	if err == nil {
		t.Fatal("validateMedicalScaleQuestionnaireBinding() error = nil, want non-nil")
	}
}

func TestValidateMedicalScaleQuestionnaireBindingAllowsSameScaleRebind(t *testing.T) {
	ctx := context.Background()
	q, err := domainQuestionnaire.NewQuestionnaire(
		meta.NewCode("Q-MS"),
		"Medical",
		domainQuestionnaire.WithType(domainQuestionnaire.TypeMedicalScale),
		domainQuestionnaire.WithVersion(domainQuestionnaire.NewVersion("1.0")),
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

	svc := &lifecycleService{
		repo: &scaleRepoBindingStub{
			boundByQuestionnaire: map[string]*domainScale.MedicalScale{
				"Q-MS": scaleItem,
			},
		},
		questionnaireRepo: &questionnaireRepoBindingStub{
			byCode: map[string]*domainQuestionnaire.Questionnaire{
				"Q-MS": q,
			},
			byVersion: map[string]*domainQuestionnaire.Questionnaire{
				"Q-MS:1.0": q,
			},
		},
	}

	if err := svc.validateMedicalScaleQuestionnaireBinding(ctx, "Q-MS", "1.0", "S-001"); err != nil {
		t.Fatalf("validateMedicalScaleQuestionnaireBinding() error = %v, want nil", err)
	}
}
