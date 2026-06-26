package evaluationinput

import (
	"context"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/scale/snapshot"
)

type EvaluationModelKind string

const (
	EvaluationModelKindScale         EvaluationModelKind = "scale"
	EvaluationModelKindPersonality   EvaluationModelKind = "personality"
	EvaluationModelKindMBTIMigration EvaluationModelKind = "mbti"
	EvaluationModelKindSBTIMigration EvaluationModelKind = "sbti"
)

const (
	DefaultSBTIModelCode          = "SBTI_FUN"
	DefaultSBTIModelVersion       = "1.0.0"
	DefaultSBTIModelTitle         = "SBTI 趣味人格测评"
	DefaultSBTIQuestionnaireCode  = "SBTI_FUN"
	DefaultSBTIQuestionnaireTitle = "SBTI 趣味人格测评"

	DefaultMBTIModelCode          = "MBTI_OEJTS"
	DefaultMBTIModelVersion       = "1.0.0"
	DefaultMBTIModelTitle         = "MBTI 人格类型测评（OEJTS）"
	DefaultMBTIQuestionnaireCode  = "MBTI_OEJTS"
	DefaultMBTIQuestionnaireTitle = "MBTI 人格类型测评（OEJTS 32题）"
)

func (k EvaluationModelKind) String() string {
	return string(k)
}

type ModelRef struct {
	Kind      EvaluationModelKind
	SubKind   string
	Algorithm string
	Code      string
	Version   string
	Title     string
}

func (r ModelRef) IsEmpty() bool {
	return r.Kind == "" && r.Code == ""
}

type InputRef struct {
	AssessmentID         uint64
	ModelRef             ModelRef
	MedicalScaleCode     string
	AnswerSheetID        uint64
	QuestionnaireCode    string
	QuestionnaireVersion string
}

type InputSnapshot struct {
	Model        *ModelSnapshot
	ModelPayload ModelPayload
	// Deprecated: use ScalePayload(input) instead.
	MedicalScale  *scalesnapshot.ScaleSnapshot
	AnswerSheet   *AnswerSheetSnapshot
	Questionnaire *QuestionnaireSnapshot
}

type ModelSnapshot struct {
	Kind      EvaluationModelKind
	SubKind   string
	Algorithm string
	Code      string
	Version   string
	Title     string
	Payload   ModelPayload
}

func (m *ModelSnapshot) ModelRef() ModelRef {
	if m == nil {
		return ModelRef{}
	}
	return ModelRef{
		Kind:      m.Kind,
		SubKind:   m.SubKind,
		Algorithm: m.Algorithm,
		Code:      m.Code,
		Version:   m.Version,
		Title:     m.Title,
	}
}

type ModelPayload interface {
	RuleSetKind() EvaluationModelKind
}

func NewScaleModelSnapshot(scale *scalesnapshot.ScaleSnapshot) *ModelSnapshot {
	if scale == nil {
		return nil
	}
	version := scale.ScaleVersion
	if version == "" {
		version = scale.QuestionnaireVersion
	}
	return &ModelSnapshot{
		Kind:      EvaluationModelKindScale,
		Algorithm: "scale_default",
		Code:      scale.Code,
		Version:   version,
		Title:     scale.Title,
		Payload:   ScaleModelPayload{Scale: scale},
	}
}

type ScaleModelPayload struct {
	Scale *scalesnapshot.ScaleSnapshot
}

func (ScaleModelPayload) RuleSetKind() EvaluationModelKind {
	return EvaluationModelKindScale
}

func ScalePayload(input *InputSnapshot) (*scalesnapshot.ScaleSnapshot, bool) {
	if input == nil {
		return nil, false
	}
	if payload, ok := input.ModelPayload.(ScaleModelPayload); ok && payload.Scale != nil {
		return payload.Scale, true
	}
	if input.Model != nil {
		if payload, ok := input.Model.Payload.(ScaleModelPayload); ok && payload.Scale != nil {
			return payload.Scale, true
		}
	}
	if input.MedicalScale != nil {
		return input.MedicalScale, true
	}
	return nil, false
}

func NewTypologyModelSnapshot(payload *typology.Payload) *ModelSnapshot {
	if payload == nil {
		return nil
	}
	return &ModelSnapshot{
		Kind:      EvaluationModelKindPersonality,
		SubKind:   "typology",
		Algorithm: string(payload.Algorithm),
		Code:      payload.Code,
		Version:   payload.Version,
		Title:     payload.Title,
		Payload:   TypologyModelPayload{Payload: payload},
	}
}

func NewSBTIModelSnapshot(model *typology.SBTILegacyModel) *ModelSnapshot {
	return NewTypologyModelSnapshot(typology.FromSBTI(model))
}

type TypologyModelPayload struct {
	Payload *typology.Payload
}

func (TypologyModelPayload) RuleSetKind() EvaluationModelKind {
	return EvaluationModelKindPersonality
}

func (p TypologyModelPayload) ModelKind() EvaluationModelKind {
	if p.Payload == nil {
		return ""
	}
	switch p.Payload.Algorithm {
	case "sbti":
		return EvaluationModelKindSBTIMigration
	case "mbti":
		return EvaluationModelKindMBTIMigration
	default:
		return EvaluationModelKind(p.Payload.Algorithm)
	}
}

func NewMBTIModelSnapshot(model *typology.MBTILegacyModel) *ModelSnapshot {
	return NewTypologyModelSnapshot(typology.FromMBTI(model))
}

type SBTIModelPayload struct {
	Model *typology.SBTILegacyModel `json:"model"`
}

func (SBTIModelPayload) RuleSetKind() EvaluationModelKind {
	return EvaluationModelKindSBTIMigration
}

func SBTIPayload(input *InputSnapshot) (*typology.SBTILegacyModel, bool) {
	if payload, ok := TypologyPayload(input); ok && payload.Algorithm == "sbti" {
		legacy, err := typology.ToSBTI(payload)
		if err == nil {
			return legacy, true
		}
	}
	if input == nil {
		return nil, false
	}
	if payload, ok := input.ModelPayload.(SBTIModelPayload); ok && payload.Model != nil {
		return payload.Model, true
	}
	if input.Model != nil {
		if payload, ok := input.Model.Payload.(SBTIModelPayload); ok && payload.Model != nil {
			return payload.Model, true
		}
	}
	return nil, false
}

type MBTIModelPayload struct {
	Model *typology.MBTILegacyModel `json:"model"`
}

func (MBTIModelPayload) RuleSetKind() EvaluationModelKind {
	return EvaluationModelKindMBTIMigration
}

func MBTIPayload(input *InputSnapshot) (*typology.MBTILegacyModel, bool) {
	if payload, ok := TypologyPayload(input); ok && payload.Algorithm == "mbti" {
		legacy, err := typology.ToMBTI(payload)
		if err == nil {
			return legacy, true
		}
	}
	if input == nil {
		return nil, false
	}
	if payload, ok := input.ModelPayload.(MBTIModelPayload); ok && payload.Model != nil {
		return payload.Model, true
	}
	if input.Model != nil {
		if payload, ok := input.Model.Payload.(MBTIModelPayload); ok && payload.Model != nil {
			return payload.Model, true
		}
	}
	return nil, false
}

type AnswerSheetSnapshot struct {
	ID                   uint64
	QuestionnaireCode    string
	QuestionnaireVersion string
	QuestionnaireTitle   string
	Answers              []AnswerSnapshot
}

type AnswerSnapshot struct {
	QuestionCode string
	Score        float64
	Value        any
}

type QuestionnaireSnapshot struct {
	Code      string
	Version   string
	Title     string
	Questions []QuestionSnapshot
}

type QuestionSnapshot struct {
	Code    string
	Type    string
	Options []OptionSnapshot
}

type OptionSnapshot struct {
	Code    string
	Content string
	Score   float64
}

type Resolver interface {
	Resolve(ctx context.Context, ref InputRef) (*InputSnapshot, error)
}

type ScaleCatalog interface {
	GetScale(ctx context.Context, code string) (*scalesnapshot.ScaleSnapshot, error)
}

type ScaleModelCatalog interface {
	ScaleCatalog
	GetScaleByRef(ctx context.Context, ref ModelRef) (*scalesnapshot.ScaleSnapshot, error)
}

type SBTIModelCatalog interface {
	GetSBTIModelByRef(ctx context.Context, ref ModelRef) (*typology.SBTILegacyModel, error)
	FindSBTIModelByQuestionnaire(ctx context.Context, code, version string) (*typology.SBTILegacyModel, error)
}

type MBTIModelCatalog interface {
	GetMBTIModelByRef(ctx context.Context, ref ModelRef) (*typology.MBTILegacyModel, error)
	FindMBTIModelByQuestionnaire(ctx context.Context, code, version string) (*typology.MBTILegacyModel, error)
}

type AnswerSheetReader interface {
	GetAnswerSheet(ctx context.Context, id uint64) (*AnswerSheetSnapshot, error)
}

type QuestionnaireReader interface {
	GetQuestionnaire(ctx context.Context, code, version string) (*QuestionnaireSnapshot, error)
}

type FailureReasonCarrier interface {
	FailureReason() string
}

type FailureKind string

const (
	FailureKindUnknown                      FailureKind = "unknown"
	FailureKindModelNotFound                FailureKind = "model_not_found"
	FailureKindUnsupportedModel             FailureKind = "unsupported_model"
	FailureKindScaleNotFound                FailureKind = "scale_not_found"
	FailureKindAnswerSheetNotFound          FailureKind = "answersheet_not_found"
	FailureKindQuestionnaireNotFound        FailureKind = "questionnaire_not_found"
	FailureKindQuestionnaireVersionMismatch FailureKind = "questionnaire_version_mismatch"
)

type FailureKindCarrier interface {
	FailureKind() FailureKind
}

type ResolveError struct {
	kind          FailureKind
	message       string
	cause         error
	failureReason string
}

func NewResolveError(kind FailureKind, cause error, message, failurePrefix string) *ResolveError {
	return &ResolveError{
		kind:          kind,
		message:       message,
		cause:         cause,
		failureReason: failureReason(failurePrefix, cause),
	}
}

func (e *ResolveError) Error() string {
	if e == nil {
		return ""
	}
	if e.message != "" {
		return e.message
	}
	if e.cause != nil {
		return e.cause.Error()
	}
	if e.kind != "" {
		return string(e.kind)
	}
	return string(FailureKindUnknown)
}

func (e *ResolveError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (e *ResolveError) FailureKind() FailureKind {
	if e == nil || e.kind == "" {
		return FailureKindUnknown
	}
	return e.kind
}

func (e *ResolveError) FailureReason() string {
	if e == nil {
		return ""
	}
	if e.failureReason != "" {
		return e.failureReason
	}
	return failureReason("评估输入加载失败", e.cause)
}

func failureReason(prefix string, cause error) string {
	if prefix == "" {
		prefix = "评估输入加载失败"
	}
	if cause == nil {
		return prefix
	}
	return fmt.Sprintf("%s: %s", prefix, cause.Error())
}
