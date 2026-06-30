package assessment

import (
	"context"
	"fmt"

	domainmodel "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	evalassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/assessmentmodel"
)

// PersonalityEvaluationModelValidator ensures personality assessments pin a published snapshot.
type PersonalityEvaluationModelValidator struct {
	reader port.PublishedModelReader
}

func NewPersonalityEvaluationModelValidator(reader port.PublishedModelReader) evalassessment.EvaluationModelValidator {
	return PersonalityEvaluationModelValidator{reader: reader}
}

func (v PersonalityEvaluationModelValidator) ValidateEvaluationModel(
	ctx context.Context,
	modelRef evalassessment.EvaluationModelRef,
	questionnaireRef evalassessment.QuestionnaireRef,
) error {
	if v.reader == nil || modelRef.IsEmpty() || modelRef.Kind() != evalassessment.EvaluationModelKindPersonality {
		return nil
	}
	if modelRef.Version() == "" {
		return fmt.Errorf("%w: personality model version is required", evalassessment.ErrEvaluationModelNotPublished)
	}
	snapshot, err := v.reader.GetPublishedModelByRef(ctx, port.Ref{
		Kind:      domainmodel.KindPersonality,
		SubKind:   modelRef.SubKind(),
		Algorithm: modelRef.Algorithm(),
		Code:      modelRef.Code().String(),
		Version:   modelRef.Version(),
	})
	if err != nil {
		if domainmodel.IsNotFound(err) {
			return fmt.Errorf("%w: %s@%s", evalassessment.ErrEvaluationModelNotPublished, modelRef.Code(), modelRef.Version())
		}
		return fmt.Errorf("failed to validate personality model: %w", err)
	}
	if snapshot == nil {
		return fmt.Errorf("%w: %s@%s", evalassessment.ErrEvaluationModelNotPublished, modelRef.Code(), modelRef.Version())
	}
	if snapshot.Binding.QuestionnaireCode != "" &&
		snapshot.Binding.QuestionnaireCode != questionnaireRef.Code().String() {
		return evalassessment.ErrEvaluationModelQuestionnaireMismatch
	}
	if snapshot.Binding.QuestionnaireVersion != "" &&
		snapshot.Binding.QuestionnaireVersion != questionnaireRef.Version() {
		return evalassessment.ErrEvaluationModelQuestionnaireMismatch
	}
	return nil
}
