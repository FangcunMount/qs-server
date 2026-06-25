package ruleset

// RuleSetKind 测评规则集类型，与 EvaluationModelKind 字符串值对齐。
type RuleSetKind string

const (
	RuleSetKindScale RuleSetKind = "scale"
	RuleSetKindMBTI  RuleSetKind = "mbti"
	RuleSetKindSBTI  RuleSetKind = "sbti"
)

func (k RuleSetKind) String() string {
	return string(k)
}

// DecisionKind 结果判定方式。
type DecisionKind string

const (
	DecisionKindScoreRangeInterpretation DecisionKind = "score_range_interpretation"
	DecisionKindPoleComposition          DecisionKind = "pole_composition"
	DecisionKindNearestPattern           DecisionKind = "nearest_pattern"
)

const (
	RuleSetSchemaVersionV1 = "1"

	PayloadFormatScaleV1 = "ruleset.scale.v1"
	PayloadFormatMBTIV1  = "ruleset.mbti.v1"
	PayloadFormatSBTIV1  = "ruleset.sbti.v1"

	PayloadFormatScaleV1Legacy = "evaluationinput.scale.v1"
	PayloadFormatMBTIV1Legacy  = "evaluationinput.mbti.v1"
	PayloadFormatSBTIV1Legacy  = "evaluationinput.sbti.v1"
)

// QuestionnaireBinding 规则集与问卷版本的绑定关系。
type QuestionnaireBinding struct {
	QuestionnaireCode    string
	QuestionnaireVersion string
}

// RuleSetDefinition 规则资产元信息。
type RuleSetDefinition struct {
	Kind    RuleSetKind
	Code    string
	Version string
	Title   string
	Status  string
}

// RuleSetSnapshot 已发布规则集快照（v1 envelope + typed payload bytes）。
type RuleSetSnapshot struct {
	SchemaVersion string
	PayloadFormat string
	Definition    RuleSetDefinition
	Binding       QuestionnaireBinding
	DecisionKind  DecisionKind
	Source        map[string]any
	Payload       []byte
}

func IsScalePayloadFormat(format string) bool {
	return format == PayloadFormatScaleV1 || format == PayloadFormatScaleV1Legacy
}

func IsMBTIPayloadFormat(format string) bool {
	return format == PayloadFormatMBTIV1 || format == PayloadFormatMBTIV1Legacy
}

func IsSBTIPayloadFormat(format string) bool {
	return format == PayloadFormatSBTIV1 || format == PayloadFormatSBTIV1Legacy
}
