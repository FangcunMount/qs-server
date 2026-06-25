package interpretationmodel

// ModelKind 解释模型类型，与 EvaluationModelKind 字符串值对齐。
type ModelKind string

const (
	ModelKindScale ModelKind = "scale"
	ModelKindMBTI  ModelKind = "mbti"
	ModelKindSBTI  ModelKind = "sbti"
)

func (k ModelKind) String() string {
	return string(k)
}

// DecisionKind 结果判定方式。
type DecisionKind string

const (
	DecisionKindScoreRangeInterpretation DecisionKind = "score_range_interpretation"
	DecisionKindPoleComposition          DecisionKind = "pole_composition"
	DecisionKindNearestPattern           DecisionKind = "nearest_pattern"
)

// QuestionnaireBinding 模型与问卷版本的绑定关系。
type QuestionnaireBinding struct {
	QuestionnaireCode    string
	QuestionnaireVersion string
}

// ModelDefinition 规则资产元信息。
type ModelDefinition struct {
	Kind    ModelKind
	Code    string
	Version string
	Title   string
	Status  string
}

// RuleSetSnapshot 已发布规则集快照（v1 envelope + 原始 payload）。
type RuleSetSnapshot struct {
	Definition   ModelDefinition
	Binding      QuestionnaireBinding
	DecisionKind DecisionKind
	Source       map[string]any
	Payload      []byte
}
