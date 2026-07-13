package factor

import "encoding/json"

// Factor 是 ModelCatalog 的通用测量节点。
type Factor struct {
	Code  string
	Title string
	Role  FactorRole
}

// ResolvedRole 返回显式 role，或归一化为空 role 的默认维度角色。
func (f Factor) ResolvedRole() FactorRole {
	return f.Role.Resolved()
}

// ScoringSourceKind 标识 Factor 计分输入来源。
type ScoringSourceKind string

const (
	ScoringSourceQuestion ScoringSourceKind = "question"
	ScoringSourceFactor   ScoringSourceKind = "factor"
)

// QuestionScoringMode selects the source of a question contribution's base score.
type QuestionScoringMode string

const (
	QuestionScoringModeQuestionScore  QuestionScoringMode = "question_score"
	QuestionScoringModeOptionOverride QuestionScoringMode = "option_override"
)

// ScoringSource 指向一个计分输入，可以是题目或子 Factor。
type ScoringSource struct {
	Kind         ScoringSourceKind   `json:"Kind"`
	Code         string              `json:"Code"`
	ScoringMode  QuestionScoringMode `json:"ScoringMode,omitempty"`
	Sign         float64             `json:"Sign,omitempty"`
	Weight       float64             `json:"Weight,omitempty"`
	OptionScores map[string]float64  `json:"OptionScores,omitempty"`
}

// UnmarshalJSON distinguishes omitted defaults from explicitly invalid zero values.
func (s *ScoringSource) UnmarshalJSON(data []byte) error {
	type alias ScoringSource
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if decoded.ScoringMode != "" {
		if _, ok := fields["Sign"]; !ok {
			decoded.Sign = 1
		}
		if _, ok := fields["Weight"]; !ok {
			decoded.Weight = 1
		}
	}
	*s = ScoringSource(decoded)
	return nil
}

// Scoring 描述一个 Factor 的分数如何由输入来源聚合得到。
type Scoring struct {
	FactorCode    string
	Sources       []ScoringSource
	Strategy      ScoringStrategy
	Params        *ScoringParams
	MaxScore      *float64
	Weights       map[string]float64
	Constant      float64
	OptionScoring OptionScoring
}

// FactorEdge 描述 FactorGraph 中一条父子边。
type FactorEdge struct {
	ParentCode string
	ChildCode  string
}

// FactorGraph 描述 Factor 之间的层级和展示顺序。
type FactorGraph struct {
	Roots      []string
	Edges      []FactorEdge
	SortOrders map[string]int
}
