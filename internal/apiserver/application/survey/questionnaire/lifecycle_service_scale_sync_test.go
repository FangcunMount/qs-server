package questionnaire

import (
	"context"
	"testing"

	scaleApp "github.com/FangcunMount/qs-server/internal/apiserver/application/scale"
	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
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
	return nil, domainQuestionnaire.ErrNotFound
}
func (r *questionnaireRepoSyncStub) FindPublishedByCode(_ context.Context, _ string) (*domainQuestionnaire.Questionnaire, error) {
	return nil, domainQuestionnaire.ErrNotFound
}
func (r *questionnaireRepoSyncStub) FindLatestPublishedByCode(_ context.Context, _ string) (*domainQuestionnaire.Questionnaire, error) {
	return nil, domainQuestionnaire.ErrNotFound
}
func (r *questionnaireRepoSyncStub) FindByCodeVersion(_ context.Context, _ string, _ string) (*domainQuestionnaire.Questionnaire, error) {
	return nil, domainQuestionnaire.ErrNotFound
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
	return nil, domainScale.ErrNotFound
}
func (r *scaleRepoSyncStub) FindByQuestionnaireCode(_ context.Context, _ string) (*domainScale.MedicalScale, error) {
	if r.item == nil {
		return nil, domainScale.ErrNotFound
	}
	return r.item, nil
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

func TestSyncScaleQuestionnaireVersion(t *testing.T) {
	tests := []struct {
		name             string
		code             string
		questionnaireTyp domainQuestionnaire.QuestionnaireType
		wantUpdateCalls  int
		wantVersion      string
	}{
		{
			name:             "updates single medical scale binding",
			code:             "Q-MS",
			questionnaireTyp: domainQuestionnaire.TypeMedicalScale,
			wantUpdateCalls:  1,
			wantVersion:      "2.0",
		},
		{
			name:             "skips survey questionnaire",
			code:             "Q-SURVEY",
			questionnaireTyp: domainQuestionnaire.TypeSurvey,
			wantUpdateCalls:  0,
			wantVersion:      "1.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			q, err := domainQuestionnaire.NewQuestionnaire(
				meta.NewCode(tt.code),
				"Questionnaire",
				domainQuestionnaire.WithType(tt.questionnaireTyp),
			)
			if err != nil {
				t.Fatalf("NewQuestionnaire() error = %v", err)
			}
			scaleItem, err := domainScale.NewMedicalScale(
				meta.NewCode("S-001"),
				"Scale",
				domainScale.WithQuestionnaire(meta.NewCode(tt.code), "1.0"),
			)
			if err != nil {
				t.Fatalf("NewMedicalScale() error = %v", err)
			}

			scaleRepo := &scaleRepoSyncStub{item: scaleItem}
			svc := &lifecycleService{
				repo: &questionnaireRepoSyncStub{
					byCode: map[string]*domainQuestionnaire.Questionnaire{
						tt.code: q,
					},
				},
				scaleSyncer: scaleApp.NewQuestionnaireBindingSyncer(scaleRepo),
			}

			if err := svc.syncScaleQuestionnaireVersion(ctx, tt.code, "2.0"); err != nil {
				t.Fatalf("syncScaleQuestionnaireVersion() error = %v", err)
			}
			if scaleRepo.updateCalls != tt.wantUpdateCalls {
				t.Fatalf("syncScaleQuestionnaireVersion() updateCalls = %d, want %d", scaleRepo.updateCalls, tt.wantUpdateCalls)
			}
			if got := scaleRepo.item.GetQuestionnaireVersion(); got != tt.wantVersion {
				t.Fatalf("syncScaleQuestionnaireVersion() questionnaire version = %q, want %q", got, tt.wantVersion)
			}
		})
	}
}
