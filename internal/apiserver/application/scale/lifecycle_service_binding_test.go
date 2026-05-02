package scale

import (
	"context"
	"testing"

	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/questionnairecatalog"
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
func (r *scaleRepoBindingStub) Update(_ context.Context, _ *domainScale.MedicalScale) error {
	return nil
}
func (r *scaleRepoBindingStub) Remove(_ context.Context, _ string) error { return nil }
func (r *scaleRepoBindingStub) ExistsByCode(_ context.Context, _ string) (bool, error) {
	return false, nil
}

type questionnaireCatalogBindingStub struct {
	byCode    map[string]*questionnairecatalog.Item
	byVersion map[string]*questionnairecatalog.Item
}

func (s *questionnaireCatalogBindingStub) FindQuestionnaire(_ context.Context, code string) (*questionnairecatalog.Item, error) {
	if q, ok := s.byCode[code]; ok {
		return q, nil
	}
	return nil, domainQuestionnaire.ErrNotFound
}

func (s *questionnaireCatalogBindingStub) FindQuestionnaireVersion(_ context.Context, code, version string) (*questionnairecatalog.Item, error) {
	if q, ok := s.byVersion[code+":"+version]; ok {
		return q, nil
	}
	return nil, domainQuestionnaire.ErrNotFound
}

func (s *questionnaireCatalogBindingStub) FindPublishedQuestionnaire(_ context.Context, code string) (*questionnairecatalog.Item, error) {
	return s.FindQuestionnaire(context.Background(), code)
}

func questionnaireCatalogItem(q *domainQuestionnaire.Questionnaire) *questionnairecatalog.Item {
	return &questionnairecatalog.Item{
		Code:    q.GetCode().String(),
		Version: q.GetVersion().String(),
		Type:    q.GetType().String(),
	}
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
		questionnaireCatalog: &questionnaireCatalogBindingStub{
			byCode: map[string]*questionnairecatalog.Item{
				"Q-SURVEY": questionnaireCatalogItem(q),
			},
		},
	}

	err = svc.resolveQuestionnaireBinding().validate(ctx, "Q-SURVEY", "", "S-001")
	if err == nil {
		t.Fatal("validate() error = nil, want non-nil")
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
		questionnaireCatalog: &questionnaireCatalogBindingStub{
			byCode: map[string]*questionnairecatalog.Item{
				"Q-MS": questionnaireCatalogItem(q),
			},
			byVersion: map[string]*questionnairecatalog.Item{
				"Q-MS:1.0": questionnaireCatalogItem(q),
			},
		},
	}

	err = svc.resolveQuestionnaireBinding().validate(ctx, "Q-MS", "1.0", "S-001")
	if err == nil {
		t.Fatal("validate() error = nil, want non-nil")
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
		questionnaireCatalog: &questionnaireCatalogBindingStub{
			byCode: map[string]*questionnairecatalog.Item{
				"Q-MS": questionnaireCatalogItem(q),
			},
			byVersion: map[string]*questionnairecatalog.Item{
				"Q-MS:1.0": questionnaireCatalogItem(q),
			},
		},
	}

	if err := svc.resolveQuestionnaireBinding().validate(ctx, "Q-MS", "1.0", "S-001"); err != nil {
		t.Fatalf("validate() error = %v, want nil", err)
	}
}
