package intake

import (
	"context"
	"fmt"

	evalassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainmodel "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// PublishedEvaluationModelValidator ensures every new assessment pins the
// currently active immutable model snapshot. Existing assessments do not pass
// through this admission boundary and remain executable against archived
// exact-version snapshots.
type PublishedEvaluationModelValidator struct {
	reader port.ActivePublishedModelReader
}

func NewPublishedEvaluationModelValidator(reader port.ActivePublishedModelReader) EvaluationModelValidator {
	return PublishedEvaluationModelValidator{reader: reader}
}

func (v PublishedEvaluationModelValidator) ValidateEvaluationModel(
	ctx context.Context,
	modelRef evalassessment.EvaluationModelRef,
	questionnaireRef evalassessment.QuestionnaireRef,
) error {
	if v.reader == nil || modelRef.IsEmpty() {
		return nil
	}
	if modelRef.Version() == "" {
		return fmt.Errorf("%w: model version is required", evalassessment.ErrEvaluationModelNotPublished)
	}
	snapshot, err := v.reader.GetActivePublishedModelByRef(ctx, port.Ref{
		Kind:      domainmodel.Kind(modelRef.Kind()),
		SubKind:   modelRef.SubKind(),
		Algorithm: modelRef.Algorithm(),
		Code:      modelRef.Code().String(),
		Version:   modelRef.Version(),
	})
	if err != nil {
		if domainmodel.IsNotFound(err) {
			return fmt.Errorf("%w: %s@%s", evalassessment.ErrEvaluationModelNotPublished, modelRef.Code(), modelRef.Version())
		}
		return fmt.Errorf("failed to validate published model: %w", err)
	}
	if snapshot == nil {
		return fmt.Errorf("%w: %s@%s", evalassessment.ErrEvaluationModelNotPublished, modelRef.Code(), modelRef.Version())
	}
	if snapshot.QuestionnaireCode != "" &&
		snapshot.QuestionnaireCode != questionnaireRef.Code().String() {
		return evalassessment.ErrEvaluationModelQuestionnaireMismatch
	}
	if snapshot.QuestionnaireVersion != "" &&
		snapshot.QuestionnaireVersion != questionnaireRef.Version() {
		return evalassessment.ErrEvaluationModelQuestionnaireMismatch
	}
	return nil
}
