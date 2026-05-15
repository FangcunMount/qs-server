package evaluationinput

import (
	"context"
	"fmt"
)

type EvaluationModelKind string

const (
	EvaluationModelKindScale EvaluationModelKind = "scale"
	EvaluationModelKindMBTI  EvaluationModelKind = "mbti"
)

func (k EvaluationModelKind) String() string {
	return string(k)
}

type ModelRef struct {
	Kind    EvaluationModelKind
	Code    string
	Version string
	Title   string
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
	// Deprecated: use ScalePayload(input) instead. Kept for one compatibility
	// cycle while Scale-specific consumers move behind the model payload seam.
	MedicalScale  *ScaleSnapshot
	AnswerSheet   *AnswerSheetSnapshot
	Questionnaire *QuestionnaireSnapshot
}

type ModelSnapshot struct {
	Kind    EvaluationModelKind
	Code    string
	Version string
	Title   string
	Payload ModelPayload
}

type ModelPayload interface {
	ModelKind() EvaluationModelKind
}

func NewScaleModelSnapshot(scale *ScaleSnapshot) *ModelSnapshot {
	if scale == nil {
		return nil
	}
	version := scale.ScaleVersion
	if version == "" {
		version = scale.QuestionnaireVersion
	}
	return &ModelSnapshot{
		Kind:    EvaluationModelKindScale,
		Code:    scale.Code,
		Version: version,
		Title:   scale.Title,
		Payload: ScaleModelPayload{Scale: scale},
	}
}

type ScaleModelPayload struct {
	Scale *ScaleSnapshot
}

func (ScaleModelPayload) ModelKind() EvaluationModelKind {
	return EvaluationModelKindScale
}

func ScalePayload(input *InputSnapshot) (*ScaleSnapshot, bool) {
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

type ScaleSnapshot struct {
	ID                   uint64
	Code                 string
	ScaleVersion         string
	Title                string
	QuestionnaireCode    string
	QuestionnaireVersion string
	Status               string
	Factors              []FactorSnapshot
}

func (s *ScaleSnapshot) IsPublished() bool {
	return s != nil && s.Status == "published"
}

func (s *ScaleSnapshot) FindFactor(code string) (*FactorSnapshot, bool) {
	if s == nil {
		return nil, false
	}
	for i := range s.Factors {
		if s.Factors[i].Code == code {
			return &s.Factors[i], true
		}
	}
	return nil, false
}

type FactorSnapshot struct {
	Code            string
	Title           string
	IsTotalScore    bool
	QuestionCodes   []string
	ScoringStrategy string
	ScoringParams   ScoringParamsSnapshot
	MaxScore        *float64
	InterpretRules  []InterpretRuleSnapshot
}

func (f FactorSnapshot) QuestionCount() int {
	return len(f.QuestionCodes)
}

func (f FactorSnapshot) FindInterpretRule(score float64) *InterpretRuleSnapshot {
	for i := range f.InterpretRules {
		if f.InterpretRules[i].Matches(score) {
			return &f.InterpretRules[i]
		}
	}
	return nil
}

type ScoringParamsSnapshot struct {
	CntOptionContents []string
}

type InterpretRuleSnapshot struct {
	Min        float64
	Max        float64
	RiskLevel  string
	Conclusion string
	Suggestion string
}

func (r InterpretRuleSnapshot) Matches(score float64) bool {
	return score >= r.Min && score < r.Max
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
	GetScale(ctx context.Context, code string) (*ScaleSnapshot, error)
}

type ScaleModelCatalog interface {
	ScaleCatalog
	GetScaleByRef(ctx context.Context, ref ModelRef) (*ScaleSnapshot, error)
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
