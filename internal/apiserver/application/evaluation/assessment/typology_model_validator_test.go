package assessment

import (
	"context"
	"errors"
	"testing"

	evalassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainmodel "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type publishedModelReaderStub struct {
	snapshot *domainmodel.PublishedModelSnapshot
	err      error
}

func (s publishedModelReaderStub) GetPublishedModelByRef(context.Context, port.Ref) (*domainmodel.PublishedModelSnapshot, error) {
	return s.snapshot, s.err
}

func (s publishedModelReaderStub) FindPublishedModelByQuestionnaire(context.Context, string, string) (*domainmodel.PublishedModelSnapshot, error) {
	return nil, domainmodel.ErrNotFound
}

func TestPersonalityEvaluationModelValidatorRequiresPublishedSnapshot(t *testing.T) {
	validator := NewPersonalityEvaluationModelValidator(publishedModelReaderStub{
		snapshot: &domainmodel.PublishedModelSnapshot{
			Model: domainmodel.ModelDefinition{Code: "personality_e2e"},
			Binding: domainmodel.QuestionnaireBinding{
				QuestionnaireCode:    "Q_FRONTEND_MBTI",
				QuestionnaireVersion: "1.0.0",
			},
		},
	})
	modelRef := evalassessment.NewEvaluationModelRefWithIdentity(
		evalassessment.EvaluationModelKindPersonality,
		domainmodel.SubKindTypology,
		domainmodel.AlgorithmMBTI,
		meta.ID(0),
		meta.NewCode("personality_e2e"),
		"v4",
		"E2E MBTI",
	)
	questionnaireRef := evalassessment.NewQuestionnaireRefByCode(meta.NewCode("Q_FRONTEND_MBTI"), "1.0.0")

	if err := validator.ValidateEvaluationModel(context.Background(), modelRef, questionnaireRef); err != nil {
		t.Fatalf("ValidateEvaluationModel() error = %v", err)
	}
}

func TestPersonalityEvaluationModelValidatorRejectsMissingPublishedSnapshot(t *testing.T) {
	validator := NewPersonalityEvaluationModelValidator(publishedModelReaderStub{err: domainmodel.ErrNotFound})
	modelRef := evalassessment.NewEvaluationModelRefByCode(
		evalassessment.EvaluationModelKindPersonality,
		meta.NewCode("missing_model"),
		"v1",
		"Missing",
	)
	questionnaireRef := evalassessment.NewQuestionnaireRefByCode(meta.NewCode("Q_FRONTEND_MBTI"), "1.0.0")

	err := validator.ValidateEvaluationModel(context.Background(), modelRef, questionnaireRef)
	if !errors.Is(err, evalassessment.ErrEvaluationModelNotPublished) {
		t.Fatalf("ValidateEvaluationModel() error = %v, want ErrEvaluationModelNotPublished", err)
	}
}

func TestPersonalityEvaluationModelValidatorRejectsQuestionnaireMismatch(t *testing.T) {
	validator := NewPersonalityEvaluationModelValidator(publishedModelReaderStub{
		snapshot: &domainmodel.PublishedModelSnapshot{
			Binding: domainmodel.QuestionnaireBinding{
				QuestionnaireCode:    "Q_OTHER",
				QuestionnaireVersion: "1.0.0",
			},
		},
	})
	modelRef := evalassessment.NewEvaluationModelRefByCode(
		evalassessment.EvaluationModelKindPersonality,
		meta.NewCode("personality_e2e"),
		"v1",
		"E2E MBTI",
	)
	questionnaireRef := evalassessment.NewQuestionnaireRefByCode(meta.NewCode("Q_FRONTEND_MBTI"), "1.0.0")

	err := validator.ValidateEvaluationModel(context.Background(), modelRef, questionnaireRef)
	if !errors.Is(err, evalassessment.ErrEvaluationModelQuestionnaireMismatch) {
		t.Fatalf("ValidateEvaluationModel() error = %v, want ErrEvaluationModelQuestionnaireMismatch", err)
	}
}
