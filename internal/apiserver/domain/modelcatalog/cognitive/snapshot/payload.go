package snapshot

import (
	"encoding/json"
	"fmt"

	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scale/snapshot"
)

// Snapshot is the published cognitive.default.v1 execution payload.
type Snapshot struct {
	Code                 string
	Version              string
	Title                string
	QuestionnaireCode    string
	QuestionnaireVersion string
	Status               string
	Factors              []FactorSnapshot
}

type FactorSnapshot struct {
	Code            string
	Title           string
	IsTotalScore    bool
	QuestionCodes   []string
	ScoringStrategy string
	MaxScore        *float64
	InterpretRules  []InterpretRuleSnapshot
}

type InterpretRuleSnapshot struct {
	MinScore   float64
	MaxScore   float64
	Conclusion string
	Suggestion string
	Level      string
}

type definitionPayload struct {
	Dimensions     []dimensionRule `json:"dimensions"`
	InterpretRules []interpretRule `json:"interpret_rules"`
}

type dimensionRule struct {
	Code            string   `json:"code"`
	Title           string   `json:"title"`
	QuestionCodes   []string `json:"question_codes"`
	ScoringStrategy string   `json:"scoring_strategy"`
	MaxScore        *float64 `json:"max_score,omitempty"`
	IsTotalScore    bool     `json:"is_total_score,omitempty"`
}

type interpretRule struct {
	DimensionCode string       `json:"dimension_code"`
	Ranges        []scoreRange `json:"ranges"`
}

type scoreRange struct {
	MinScore   float64 `json:"min_score"`
	MaxScore   float64 `json:"max_score"`
	Conclusion string  `json:"conclusion"`
	Suggestion string  `json:"suggestion,omitempty"`
	Level      string  `json:"level,omitempty"`
}

// ParseDefinitionPayload decodes cognitive.default.v1 body into a runtime snapshot.
func ParseDefinitionPayload(modelCode, modelVersion, title, status string, payload []byte) (*Snapshot, error) {
	var body definitionPayload
	if err := json.Unmarshal(payload, &body); err != nil {
		return nil, fmt.Errorf("decode cognitive payload: %w", err)
	}
	rulesByDimension := make(map[string][]InterpretRuleSnapshot, len(body.InterpretRules))
	for _, rule := range body.InterpretRules {
		converted := make([]InterpretRuleSnapshot, 0, len(rule.Ranges))
		for _, item := range rule.Ranges {
			converted = append(converted, InterpretRuleSnapshot(item))
		}
		rulesByDimension[rule.DimensionCode] = converted
	}
	factors := make([]FactorSnapshot, 0, len(body.Dimensions))
	for _, dimension := range body.Dimensions {
		factors = append(factors, FactorSnapshot{
			Code:            dimension.Code,
			Title:           dimension.Title,
			IsTotalScore:    dimension.IsTotalScore,
			QuestionCodes:   append([]string(nil), dimension.QuestionCodes...),
			ScoringStrategy: dimension.ScoringStrategy,
			MaxScore:        dimension.MaxScore,
			InterpretRules:  rulesByDimension[dimension.Code],
		})
	}
	return &Snapshot{
		Code:    modelCode,
		Version: modelVersion,
		Title:   title,
		Status:  status,
		Factors: factors,
	}, nil
}

func (s *Snapshot) IsPublished() bool {
	return s != nil && s.Status == "published"
}

// ToScaleSnapshot projects cognitive factors into the scale execution shape.
func (s *Snapshot) ToScaleSnapshot() *scalesnapshot.ScaleSnapshot {
	if s == nil {
		return nil
	}
	factors := make([]scalesnapshot.FactorSnapshot, 0, len(s.Factors))
	for _, factor := range s.Factors {
		rules := make([]scalesnapshot.InterpretRuleSnapshot, 0, len(factor.InterpretRules))
		for _, rule := range factor.InterpretRules {
			rules = append(rules, scalesnapshot.InterpretRuleSnapshot{
				Min:        rule.MinScore,
				Max:        rule.MaxScore,
				RiskLevel:  rule.Level,
				Conclusion: rule.Conclusion,
				Suggestion: rule.Suggestion,
			})
		}
		factors = append(factors, scalesnapshot.FactorSnapshot{
			Code:            factor.Code,
			Title:           factor.Title,
			IsTotalScore:    factor.IsTotalScore,
			QuestionCodes:   append([]string(nil), factor.QuestionCodes...),
			ScoringStrategy: factor.ScoringStrategy,
			MaxScore:        factor.MaxScore,
			InterpretRules:  rules,
		})
	}
	return &scalesnapshot.ScaleSnapshot{
		Code:                 s.Code,
		ScaleVersion:         s.Version,
		Title:                s.Title,
		QuestionnaireCode:    s.QuestionnaireCode,
		QuestionnaireVersion: s.QuestionnaireVersion,
		Status:               s.Status,
		Factors:              factors,
	}
}
