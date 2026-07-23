// Package inputinvariant guards shared Assessment/Model/AnswerSheet/Questionnaire
// identity and version invariants before family-specific calculation.
package inputinvariant

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// Input is the shared execution input checked before family calculators run.
type Input struct {
	Assessment    *assessment.Assessment
	Snapshot      *evaluationinput.InputSnapshot
	DescriptorKey string
}

// Error is a stable validation failure that never falls back to latest versions.
type Error struct {
	Code             string
	Message          string
	ModelRef         string
	QuestionnaireRef string
	AnswerSheetID    uint64
	DescriptorKey    string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s (model=%s questionnaire=%s answersheet=%d descriptor=%s)",
		e.Code, e.Message, e.ModelRef, e.QuestionnaireRef, e.AnswerSheetID, e.DescriptorKey)
}

// Validate enforces shared identity/version invariants. Family validators retain
// payload-shape, scoring, and NormSubject checks.
func Validate(in Input) error {
	ctx := newFailureContext(in)
	if in.DescriptorKey == "" {
		return ctx.fail("input.descriptor.required", "runtime descriptor key is required")
	}
	if in.Assessment == nil {
		return ctx.fail("input.assessment.required", "assessment is required")
	}
	if !in.Assessment.Status().IsSubmitted() {
		return ctx.fail("input.assessment.not_submitted", "assessment is not submitted")
	}
	if in.Snapshot == nil {
		return ctx.fail("input.snapshot.required", "evaluation input snapshot is required")
	}
	qRef := in.Assessment.QuestionnaireRef()
	wantCode := qRef.Code().String()
	wantVersion := qRef.Version()
	ctx.QuestionnaireRef = fmt.Sprintf("%s@%s", wantCode, wantVersion)

	if in.Snapshot.AnswerSheet == nil {
		return ctx.fail("input.answersheet.required", "answer sheet not found")
	}
	ctx.AnswerSheetID = in.Snapshot.AnswerSheet.ID
	if in.Snapshot.AnswerSheet.QuestionnaireCode != wantCode {
		return ctx.fail("input.answersheet.questionnaire_mismatch", "answer sheet does not match the questionnaire")
	}
	if err := requireVersion("answer sheet", in.Snapshot.AnswerSheet.QuestionnaireVersion, wantVersion, ctx); err != nil {
		return err
	}

	if in.Snapshot.Questionnaire == nil {
		return ctx.fail("input.questionnaire.required", "questionnaire snapshot not found")
	}
	if in.Snapshot.Questionnaire.Code != wantCode {
		return ctx.fail("input.questionnaire.code_mismatch", "questionnaire snapshot does not match the assessment questionnaire")
	}
	if err := requireVersion("questionnaire snapshot", in.Snapshot.Questionnaire.Version, wantVersion, ctx); err != nil {
		return err
	}

	if modelCode, modelVersion, ok := modelQuestionnaireBinding(in.Snapshot); ok {
		if modelCode != wantCode {
			return ctx.fail("input.model.questionnaire_mismatch", "model questionnaire binding does not match the assessment questionnaire")
		}
		if err := requireVersion("model binding", modelVersion, wantVersion, ctx); err != nil {
			return err
		}
		if err := requireVersion("model binding", modelVersion, in.Snapshot.AnswerSheet.QuestionnaireVersion, ctx); err != nil {
			return err
		}
		if err := requireVersion("model binding", modelVersion, in.Snapshot.Questionnaire.Version, ctx); err != nil {
			return err
		}
	}

	if err := requireModelIdentity(in.Assessment, in.Snapshot, ctx); err != nil {
		return err
	}
	return nil
}

func requireModelIdentity(a *assessment.Assessment, snapshot *evaluationinput.InputSnapshot, ctx failureContext) error {
	modelRef := a.EvaluationModelRef()
	if modelRef == nil || modelRef.IsEmpty() {
		return ctx.fail("input.model_ref.required", "assessment evaluation model reference is required")
	}
	if snapshot.Model == nil {
		return ctx.fail("input.model_snapshot.required", "evaluation model snapshot is required")
	}
	ctx.ModelRef = fmt.Sprintf("%s@%s", snapshot.Model.Code, snapshot.Model.Version)
	refKind := string(modelRef.Kind())
	snapshotKind := string(snapshot.Model.Kind)
	if refKind == "" || snapshotKind == "" {
		return ctx.fail("input.model.kind_required", "evaluation model kind is required")
	}
	if refKind != snapshotKind {
		return ctx.fail("input.model.kind_mismatch", "evaluation model kind does not match the input model snapshot")
	}
	refAlgorithm := string(modelRef.Algorithm())
	if refAlgorithm == "" || snapshot.Model.Algorithm == "" {
		return ctx.fail("input.model.algorithm_required", "evaluation model algorithm is required")
	}
	if refAlgorithm != snapshot.Model.Algorithm {
		return ctx.fail("input.model.algorithm_mismatch", "evaluation model algorithm does not match the input model snapshot")
	}
	refCode := modelRef.Code().String()
	if refCode == "" || snapshot.Model.Code == "" {
		return ctx.fail("input.model.code_required", "evaluation model code is required")
	}
	if refCode != snapshot.Model.Code {
		return ctx.fail("input.model.code_mismatch", "evaluation model code does not match the input model snapshot")
	}
	refVersion := modelRef.Version()
	if refVersion == "" || snapshot.Model.Version == "" {
		return ctx.fail("input.model.version_required", "evaluation model version is required")
	}
	if refVersion != snapshot.Model.Version {
		return ctx.fail("input.model.version_mismatch", "evaluation model version does not match the input model snapshot")
	}
	return nil
}

func modelQuestionnaireBinding(snapshot *evaluationinput.InputSnapshot) (code, version string, ok bool) {
	if scale, ok := evaluationinput.ScalePayload(snapshot); ok && scale != nil && scale.QuestionnaireCode != "" {
		return scale.QuestionnaireCode, scale.QuestionnaireVersion, true
	}
	if payload, ok := evaluationinput.TypologyPayload(snapshot); ok && payload != nil && payload.QuestionnaireCode != "" {
		return payload.QuestionnaireCode, payload.QuestionnaireVersion, true
	}
	if behavioral, ok := evaluationinput.BehavioralRatingPayload(snapshot); ok && behavioral.Snapshot != nil && behavioral.Snapshot.QuestionnaireCode != "" {
		return behavioral.Snapshot.QuestionnaireCode, behavioral.Snapshot.QuestionnaireVersion, true
	}
	if cognitive, ok := evaluationinput.CognitivePayload(snapshot); ok && cognitive.Snapshot != nil && cognitive.Snapshot.QuestionnaireCode != "" {
		return cognitive.Snapshot.QuestionnaireCode, cognitive.Snapshot.QuestionnaireVersion, true
	}
	return "", "", false
}

func requireVersion(label, got, want string, ctx failureContext) error {
	if got == "" || want == "" {
		return ctx.fail("input.questionnaire.version_required", label+" questionnaire version is required")
	}
	if got != want {
		return ctx.fail("input.questionnaire.version_mismatch", label+" questionnaire version does not match")
	}
	return nil
}

type failureContext struct {
	ModelRef         string
	QuestionnaireRef string
	AnswerSheetID    uint64
	DescriptorKey    string
}

func newFailureContext(in Input) failureContext {
	ctx := failureContext{DescriptorKey: in.DescriptorKey}
	if in.Snapshot != nil && in.Snapshot.Model != nil {
		ctx.ModelRef = fmt.Sprintf("%s@%s", in.Snapshot.Model.Code, in.Snapshot.Model.Version)
	}
	if in.Assessment != nil {
		q := in.Assessment.QuestionnaireRef()
		ctx.QuestionnaireRef = fmt.Sprintf("%s@%s", q.Code().String(), q.Version())
		if modelRef := in.Assessment.EvaluationModelRef(); modelRef != nil && ctx.ModelRef == "" {
			ctx.ModelRef = fmt.Sprintf("%s@%s", modelRef.Code().String(), modelRef.Version())
		}
	}
	if in.Snapshot != nil && in.Snapshot.AnswerSheet != nil {
		ctx.AnswerSheetID = in.Snapshot.AnswerSheet.ID
	}
	return ctx
}

func (c failureContext) fail(code, message string) error {
	return &Error{
		Code:             code,
		Message:          message,
		ModelRef:         c.ModelRef,
		QuestionnaireRef: c.QuestionnaireRef,
		AnswerSheetID:    c.AnswerSheetID,
		DescriptorKey:    c.DescriptorKey,
	}
}
