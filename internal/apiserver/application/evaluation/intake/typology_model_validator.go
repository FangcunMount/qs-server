package intake

import (
	"context"
	"fmt"

	evalerrors "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/apperrors"
	evalassessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainmodel "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// PublishedEvaluationModelValidator ensures every new assessment pins the
// currently active immutable model snapshot. Existing assessments do not pass
// through this admission boundary and remain executable against archived
// exact-version snapshots.
type PublishedEvaluationModelValidator struct {
	activeReader   port.ActivePublishedModelReader
	retainedReader port.PublishedModelReader
}

func NewPublishedEvaluationModelValidator(
	activeReader port.ActivePublishedModelReader,
	retainedReader port.PublishedModelReader,
) EvaluationModelValidator {
	return PublishedEvaluationModelValidator{
		activeReader:   activeReader,
		retainedReader: retainedReader,
	}
}

func (v PublishedEvaluationModelValidator) ValidateEvaluationModel(
	ctx context.Context,
	modelRef evalassessment.EvaluationModelRef,
	questionnaireRef evalassessment.QuestionnaireRef,
	mode ModelValidationMode,
) error {
	normalizedMode, err := normalizeModelValidationMode(mode)
	if err != nil {
		return err
	}
	if modelRef.IsEmpty() {
		return nil
	}
	if modelRef.Version() == "" {
		return fmt.Errorf("%w: model version is required", evalassessment.ErrEvaluationModelNotPublished)
	}
	ref := port.Ref{
		Kind:      domainmodel.Kind(modelRef.Kind()),
		SubKind:   modelRef.SubKind(),
		Algorithm: modelRef.Algorithm(),
		Code:      modelRef.Code().String(),
		Version:   modelRef.Version(),
	}
	var snapshot *port.PublishedModel
	switch normalizedMode {
	case ModelValidationModeActiveRelease:
		if v.activeReader == nil {
			return evalerrors.ModuleNotConfigured("active published model reader is not configured")
		}
		snapshot, err = v.activeReader.GetActivePublishedModelByRef(ctx, ref)
	case ModelValidationModeRetainedExact:
		if v.retainedReader == nil {
			return evalerrors.ModuleNotConfigured("retained published model reader is not configured")
		}
		snapshot, err = v.retainedReader.GetPublishedModelByRef(ctx, ref)
	default:
		return evalerrors.InvalidArgument("invalid evaluation model validation mode: %s", normalizedMode)
	}
	if err != nil {
		if domainmodel.IsNotFound(err) {
			return fmt.Errorf("%w: %s@%s", evalassessment.ErrEvaluationModelNotPublished, modelRef.Code(), modelRef.Version())
		}
		return fmt.Errorf("failed to validate published model: %w", err)
	}
	if snapshot == nil {
		return fmt.Errorf("%w: %s@%s", evalassessment.ErrEvaluationModelNotPublished, modelRef.Code(), modelRef.Version())
	}
	if !publishedModelMatchesRef(snapshot, ref) {
		return fmt.Errorf(
			"%w: model identity mismatch for %s@%s",
			evalassessment.ErrEvaluationModelNotPublished,
			modelRef.Code(),
			modelRef.Version(),
		)
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

func publishedModelMatchesRef(snapshot *port.PublishedModel, ref port.Ref) bool {
	if snapshot == nil ||
		snapshot.Kind != ref.Kind ||
		snapshot.Code != ref.Code ||
		snapshot.Version != ref.Version {
		return false
	}
	if ref.SubKind != "" && snapshot.SubKind != ref.SubKind {
		return false
	}
	if ref.Algorithm != "" && snapshot.Algorithm != ref.Algorithm {
		return false
	}
	return true
}
