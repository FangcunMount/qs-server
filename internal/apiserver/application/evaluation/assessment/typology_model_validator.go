package assessment

import (
	"context"
	"fmt"

	evalassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainmodel "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// TypologyEvaluationModelValidator ensures typology assessments pin published snapshots.
type TypologyEvaluationModelValidator struct {
	reader port.PublishedModelReader
}

func NewTypologyEvaluationModelValidator(reader port.PublishedModelReader) evalassessment.EvaluationModelValidator {
	return TypologyEvaluationModelValidator{reader: reader}
}

func (v TypologyEvaluationModelValidator) ValidateEvaluationModel(
	ctx context.Context,
	modelRef evalassessment.EvaluationModelRef,
	questionnaireRef evalassessment.QuestionnaireRef,
) error {
	if v.reader == nil || modelRef.IsEmpty() || modelRef.Kind() != evalassessment.EvaluationModelKindPersonality {
		return nil
	}
	if modelRef.Version() == "" {
		return fmt.Errorf("%w: typology model version is required", evalassessment.ErrEvaluationModelNotPublished)
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
		return fmt.Errorf("failed to validate typology model: %w", err)
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
