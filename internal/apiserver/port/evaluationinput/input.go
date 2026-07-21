package evaluationinput

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
	rulesetport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

type EvaluationModelKind string

const (
	EvaluationModelKindScale    EvaluationModelKind = "scale"
	EvaluationModelKindTypology EvaluationModelKind = "typology"
)

const (
	// Deprecated: use infra/ruleset/seedfixtures.SBTIModelCode.
	DefaultSBTIModelCode = "SBTI_FUN"
	// Deprecated: use infra/ruleset/seedfixtures.SBTIModelVersion.
	DefaultSBTIModelVersion = "1.0.0"
	// Deprecated: use infra/ruleset/seedfixtures.SBTIModelTitle.
	DefaultSBTIModelTitle = "SBTI 趣味人格测评"
	// Deprecated: use infra/ruleset/seedfixtures.SBTIQuestionnaireCode.
	DefaultSBTIQuestionnaireCode = "SBTI_FUN"
	// Deprecated: use infra/ruleset/seedfixtures.SBTIQuestionnaireTitle.
	DefaultSBTIQuestionnaireTitle = "SBTI 趣味人格测评"

	// Deprecated: use infra/ruleset/seedfixtures.MBTIModelCode.
	DefaultMBTIModelCode = "MBTI_OEJTS"
	// Deprecated: use infra/ruleset/seedfixtures.MBTIModelVersion.
	DefaultMBTIModelVersion = "2.0.1"
	// Deprecated: use infra/ruleset/seedfixtures.MBTIModelTitle.
	DefaultMBTIModelTitle = "MBTI 人格类型测评（OEJTS）"
	// Deprecated: use infra/ruleset/seedfixtures.MBTIQuestionnaireCode.
	DefaultMBTIQuestionnaireCode = "MBTI_OEJTS"
	// Deprecated: use infra/ruleset/seedfixtures.MBTIQuestionnaireTitle.
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
	AnswerSheetID        uint64
	QuestionnaireCode    string
	QuestionnaireVersion string
	// TesteeID identifies the subject whose demographics feed NormSubject.
	TesteeID uint64
	// AsOf is the assessment occurrence time used to compute AgeMonths.
	// Prefer Assessment.SubmittedAt; zero means AgeMonths cannot be derived.
	AsOf time.Time
}

type InputSnapshot struct {
	Model                *ModelSnapshot
	ModelPayload         ModelPayload
	AnswerSheet          *AnswerSheetSnapshot
	Questionnaire        *QuestionnaireSnapshot
	NormSubject          *NormSubjectSnapshot
	InterpretationAssets *interpretationassets.Assets
	// DefinitionV2 is canonical model semantics for runtime replay (MC-R017 batch 4).
	DefinitionV2 *modeldefinition.Definition
}

// NormSubjectSnapshot carries demographics for norm-based scoring such as Brief-2 T-scores.
type NormSubjectSnapshot struct {
	AgeMonths int
	Gender    string
}

// NormSubjectFacts is the Actor-side demographic authority used to build NormSubjectSnapshot.
type NormSubjectFacts struct {
	Gender   string // "male" / "female" / "" (unknown or missing)
	Birthday *time.Time
}

// NormSubjectReader loads Testee demographics for evaluation input materialization.
type NormSubjectReader interface {
	ReadNormSubjectFacts(ctx context.Context, testeeID uint64) (*NormSubjectFacts, error)
}

type ModelSnapshot struct {
	Kind            EvaluationModelKind
	SubKind         string
	Algorithm       string
	AlgorithmFamily string
	DecisionKind    string
	PayloadFormat   string
	ProductChannel  string
	Code            string
	Version         string
	Title           string
	Payload         ModelPayload
}

// ApplyFrozenRuntime copies publish-time RuntimeIdentity onto the evaluation ModelSnapshot.
func (m *ModelSnapshot) ApplyFrozenRuntime(family, decisionKind, payloadFormat string) *ModelSnapshot {
	if m == nil {
		return nil
	}
	m.AlgorithmFamily = family
	m.DecisionKind = decisionKind
	m.PayloadFormat = payloadFormat
	return m
}

// HasFrozenRuntime reports whether publish-time runtime identity is complete.
func (m *ModelSnapshot) HasFrozenRuntime() bool {
	return m != nil && m.AlgorithmFamily != "" && m.DecisionKind != "" && m.PayloadFormat != ""
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
	ms := &ModelSnapshot{
		Kind:           EvaluationModelKindScale,
		Algorithm:      string(modelcatalog.AlgorithmScaleDefault),
		ProductChannel: string(modelcatalog.ProductChannelMedicalScale),
		Code:           scale.Code,
		Version:        version,
		Title:          scale.Title,
		Payload:        ScaleModelPayload{Scale: scale},
	}
	return applyPublishedRuntime(ms, scale.PublishedRuntime)
}

func applyPublishedRuntime(ms *ModelSnapshot, meta *rulesetport.PublishedRuntimeMeta) *ModelSnapshot {
	if ms == nil || meta == nil {
		return ms
	}
	if meta.Kind != "" {
		ms.Kind = EvaluationModelKind(meta.Kind)
	}
	if meta.SubKind != "" {
		ms.SubKind = string(meta.SubKind)
	}
	if meta.Algorithm != "" {
		ms.Algorithm = string(meta.Algorithm)
	}
	if meta.ProductChannel != "" {
		ms.ProductChannel = string(meta.ProductChannel)
	}
	return ms.ApplyFrozenRuntime(string(meta.AlgorithmFamily), string(meta.DecisionKind), meta.PayloadFormat)
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
	return nil, false
}

func NewTypologyModelSnapshot(payload *typology.Payload) *ModelSnapshot {
	if payload == nil {
		return nil
	}
	ms := &ModelSnapshot{
		Kind:           EvaluationModelKindTypology,
		SubKind:        "typology",
		Algorithm:      string(payload.Algorithm),
		ProductChannel: string(modelcatalog.ProductChannelTypology),
		Code:           payload.Code,
		Version:        payload.Version,
		Title:          payload.Title,
		Payload:        TypologyModelPayload{Payload: payload},
	}
	return applyPublishedRuntime(ms, payload.PublishedRuntime)
}

func NewSBTIModelSnapshot(model *typology.SBTILegacyModel) *ModelSnapshot {
	return NewTypologyModelSnapshot(typology.FromSBTI(model))
}

type TypologyModelPayload struct {
	Payload *typology.Payload
}

func (TypologyModelPayload) RuleSetKind() EvaluationModelKind {
	return EvaluationModelKindTypology
}

func (TypologyModelPayload) ModelKind() EvaluationModelKind {
	return EvaluationModelKindTypology
}

func NewMBTIModelSnapshot(model *typology.MBTILegacyModel) *ModelSnapshot {
	return NewTypologyModelSnapshot(typology.FromMBTI(model))
}

// MBTIPayload extracts a legacy MBTI model from a unified typology input snapshot.
func MBTIPayload(input *InputSnapshot) (*typology.MBTILegacyModel, bool) {
	payload, ok := TypologyPayload(input)
	if !ok || payload.Algorithm != "mbti" {
		return nil, false
	}
	legacy, err := typology.ToMBTI(payload)
	if err != nil {
		return nil, false
	}
	return legacy, true
}

// SBTIPayload extracts a legacy SBTI model from a unified typology input snapshot.
func SBTIPayload(input *InputSnapshot) (*typology.SBTILegacyModel, bool) {
	payload, ok := TypologyPayload(input)
	if !ok || payload.Algorithm != "sbti" {
		return nil, false
	}
	legacy, err := typology.ToSBTI(payload)
	if err != nil {
		return nil, false
	}
	return legacy, true
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

// TypologyModelCatalog loads unified typology payloads for evaluation input resolution.
type TypologyModelCatalog interface {
	GetTypologyModelByRef(ctx context.Context, ref ModelRef) (*typology.Payload, error)
	FindTypologyModelByQuestionnaire(ctx context.Context, code, version string) (*typology.Payload, error)
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
	// FailureKindDependencyUnavailable is a transient dependency/infrastructure fault (EV-R004).
	FailureKindDependencyUnavailable FailureKind = "dependency_unavailable"
)

// DependencyCategory identifies which external system produced a resolve failure.
type DependencyCategory string

const (
	DependencyCategoryUnknown      DependencyCategory = ""
	DependencyCategoryModelCatalog DependencyCategory = "modelcatalog"
	DependencyCategorySurvey       DependencyCategory = "survey"
	DependencyCategoryActor        DependencyCategory = "actor"
	DependencyCategoryDatabase     DependencyCategory = "database"
	DependencyCategoryTransport    DependencyCategory = "transport"
)

type FailureKindCarrier interface {
	FailureKind() FailureKind
}

// RetryableCarrier reports whether an input-resolve failure may be automatically retried.
type RetryableCarrier interface {
	Retryable() bool
}

// DependencyCategoryCarrier reports the upstream dependency class for operators.
type DependencyCategoryCarrier interface {
	DependencyCategory() DependencyCategory
}

type ResolveError struct {
	kind               FailureKind
	message            string
	cause              error
	failureReason      string
	retryable          bool
	dependencyCategory DependencyCategory
}

func NewResolveError(kind FailureKind, cause error, message, failurePrefix string) *ResolveError {
	return &ResolveError{
		kind:          kind,
		message:       message,
		cause:         cause,
		failureReason: failureReason(failurePrefix, cause),
		retryable:     false,
	}
}

// NewDependencyResolveError builds a retryable infrastructure failure (EV-R004).
func NewDependencyResolveError(category DependencyCategory, cause error, message, failurePrefix string) *ResolveError {
	return &ResolveError{
		kind:               FailureKindDependencyUnavailable,
		message:            message,
		cause:              cause,
		failureReason:      failureReason(failurePrefix, cause),
		retryable:          true,
		dependencyCategory: category,
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

func (e *ResolveError) Retryable() bool {
	return e != nil && e.retryable
}

func (e *ResolveError) DependencyCategory() DependencyCategory {
	if e == nil {
		return DependencyCategoryUnknown
	}
	return e.dependencyCategory
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
