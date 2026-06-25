package assessmentmodel

// Kind 测评模型类型，与 EvaluationModelKind 字符串值对齐。
type Kind string

const (
	KindScale Kind = "scale"
	KindMBTI  Kind = "mbti"
	KindSBTI  Kind = "sbti"
)

// RuleSetKind is kept as a compatibility name while callers migrate to Kind.
type RuleSetKind = Kind

const (
	RuleSetKindScale = KindScale
	RuleSetKindMBTI  = KindMBTI
	RuleSetKindSBTI  = KindSBTI
)

func (k Kind) String() string {
	return string(k)
}

func (k Kind) IsValid() bool {
	switch k {
	case KindScale, KindMBTI, KindSBTI:
		return true
	default:
		return false
	}
}

// DecisionKind 结果判定方式。
type DecisionKind string

const (
	DecisionKindScoreRangeInterpretation DecisionKind = "score_range_interpretation"
	DecisionKindPoleComposition          DecisionKind = "pole_composition"
	DecisionKindNearestPattern           DecisionKind = "nearest_pattern"
)

const (
	SchemaVersionV1 = "1"

	PayloadFormatScaleV1 = "ruleset.scale.v1"
	PayloadFormatMBTIV1  = "ruleset.mbti.v1"
	PayloadFormatSBTIV1  = "ruleset.sbti.v1"

	PayloadFormatScaleV1Legacy = "evaluationinput.scale.v1"
	PayloadFormatMBTIV1Legacy  = "evaluationinput.mbti.v1"
	PayloadFormatSBTIV1Legacy  = "evaluationinput.sbti.v1"
)

// QuestionnaireBinding 测评模型与问卷版本的绑定关系。
type QuestionnaireBinding struct {
	QuestionnaireCode    string
	QuestionnaireVersion string
}

// Definition 规则资产元信息。
type Definition struct {
	Kind    Kind
	Code    string
	Version string
	Title   string
	Status  string
}

// RuleSetDefinition is kept as a compatibility name while callers migrate to Definition.
type RuleSetDefinition = Definition

// Snapshot 已发布测评模型快照（v1 envelope + typed payload bytes）。
type Snapshot struct {
	SchemaVersion string
	PayloadFormat string
	Definition    Definition
	Binding       QuestionnaireBinding
	DecisionKind  DecisionKind
	Source        map[string]any
	Payload       []byte
}

// RuleSetSnapshot is kept as a compatibility name while callers migrate to Snapshot.
type RuleSetSnapshot = Snapshot

const RuleSetSchemaVersionV1 = SchemaVersionV1

func IsScalePayloadFormat(format string) bool {
	return format == PayloadFormatScaleV1 || format == PayloadFormatScaleV1Legacy
}

func IsMBTIPayloadFormat(format string) bool {
	return format == PayloadFormatMBTIV1 || format == PayloadFormatMBTIV1Legacy
}

func IsSBTIPayloadFormat(format string) bool {
	return format == PayloadFormatSBTIV1 || format == PayloadFormatSBTIV1Legacy
}
