package shared

import (
	"encoding/json"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/scorerange"
)

// DefinitionBody is the shared behavioral/cognitive wire payload.
type DefinitionBody struct {
	Dimensions     []DimensionRule `json:"dimensions"`
	InterpretRules []InterpretRule `json:"interpret_rules"`
}

type ScoringParamsPayload struct {
	CntOptionContents []string `json:"cnt_option_contents,omitempty"`
}

type DimensionRule struct {
	Code            string                 `json:"code"`
	Title           string                 `json:"title"`
	ParentCode      string                 `json:"parent_code,omitempty"`
	SortOrder       int                    `json:"sort_order,omitempty"`
	Level           int                    `json:"level,omitempty"`
	QuestionCodes   []string               `json:"question_codes"`
	ScoringStrategy string                 `json:"scoring_strategy"`
	ScoringParams   *ScoringParamsPayload  `json:"scoring_params,omitempty"`
	MaxScore        *float64               `json:"max_score,omitempty"`
	IsTotalScore    bool                   `json:"is_total_score,omitempty"`
	IsShow          bool                   `json:"is_show"`
	Role            string                 `json:"role,omitempty"`
	ChildrenPolicy  *ChildrenPolicyPayload `json:"children_policy,omitempty"`
}

type ChildrenPolicyPayload struct {
	Strategy string             `json:"strategy"`
	Children []string           `json:"children"`
	Weights  map[string]float64 `json:"weights,omitempty"`
}

type InterpretRule struct {
	DimensionCode string           `json:"dimension_code"`
	Ranges        []ScoreRangeRule `json:"ranges"`
}

type ScoreRangeRule struct {
	MinScore     float64 `json:"min_score"`
	MaxScore     float64 `json:"max_score"`
	MaxInclusive bool    `json:"max_inclusive,omitempty"`
	UnboundedMax bool    `json:"unbounded_max,omitempty"`
	Level        string  `json:"level,omitempty"`
	Conclusion   string  `json:"conclusion"`
	Suggestion   string  `json:"suggestion,omitempty"`
}

func (r ScoreRangeRule) Matches(score float64) bool {
	return scorerange.Bound{
		Min: r.MinScore, Max: r.MaxScore, MaxInclusive: r.MaxInclusive, UnboundedMax: r.UnboundedMax,
	}.Contains(score)
}

func ParseDefinitionBodyJSON(payload []byte) (DefinitionBody, error) {
	var body DefinitionBody
	if err := json.Unmarshal(payload, &body); err != nil {
		return DefinitionBody{}, err
	}
	return body, nil
}
