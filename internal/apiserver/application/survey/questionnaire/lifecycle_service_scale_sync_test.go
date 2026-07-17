package questionnaire

import (
	"context"
	"testing"

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

type scaleBindingSyncerRecorder struct {
	syncCalls   int
	lastCode    string
	lastVersion string
	bound       bool
}

func (r *scaleBindingSyncerRecorder) IsQuestionnaireBound(context.Context, string) (bool, error) {
	return r.bound, nil
}

func TestStandaloneLifecycleRejectsQuestionnaireBoundToAssessmentRelease(t *testing.T) {
	svc := &lifecycleService{bindingSyncer: &scaleBindingSyncerRecorder{bound: true}}
	err := svc.rejectBoundStandaloneLifecycle(context.Background(), "Q-BOUND")
	if err == nil {
		t.Fatal("rejectBoundStandaloneLifecycle() error = nil, want conflict")
	}
}

func (r *scaleBindingSyncerRecorder) SyncQuestionnaireVersion(_ context.Context, questionnaireCode, version string) error {
	r.syncCalls++
	r.lastCode = questionnaireCode
	r.lastVersion = version
	return nil
}

func TestSyncScaleQuestionnaireVersion(t *testing.T) {
	tests := []struct {
		name              string
		code              string
		questionnaireTyp  domainQuestionnaire.QuestionnaireType
		wantSyncCalls     int
		wantSyncedVersion string
	}{
		{
			name:              "syncs medical scale questionnaire version",
			code:              "Q-MS",
			questionnaireTyp:  domainQuestionnaire.TypeMedicalScale,
			wantSyncCalls:     1,
			wantSyncedVersion: "2.0",
		},
		{
			name:             "skips survey questionnaire",
			code:             "Q-SURVEY",
			questionnaireTyp: domainQuestionnaire.TypeSurvey,
			wantSyncCalls:    0,
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
			syncer := &scaleBindingSyncerRecorder{}
			svc := &lifecycleService{
				repo: &questionnaireRepoSyncStub{
					byCode: map[string]*domainQuestionnaire.Questionnaire{
						tt.code: q,
					},
				},
				bindingSyncer: syncer,
			}

			if err := svc.syncQuestionnaireBindingVersion(ctx, tt.code, "2.0"); err != nil {
				t.Fatalf("syncQuestionnaireBindingVersion() error = %v", err)
			}
			if syncer.syncCalls != tt.wantSyncCalls {
				t.Fatalf("syncQuestionnaireBindingVersion() syncCalls = %d, want %d", syncer.syncCalls, tt.wantSyncCalls)
			}
			if tt.wantSyncCalls > 0 && syncer.lastVersion != tt.wantSyncedVersion {
				t.Fatalf("syncQuestionnaireBindingVersion() synced version = %q, want %q", syncer.lastVersion, tt.wantSyncedVersion)
			}
		})
	}
}
