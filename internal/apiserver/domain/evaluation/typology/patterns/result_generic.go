package patterns

import modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"

// PersonalityDimensionResult is a model-neutral scored factor shown in a personality type result.
type PersonalityDimensionResult struct {
	Code       string  `json:"code"`
	Name       string  `json:"name"`
	Model      string  `json:"model,omitempty"`
	LeftPole   string  `json:"left_pole,omitempty"`
	RightPole  string  `json:"right_pole,omitempty"`
	RawScore   float64 `json:"raw_score"`
	Preference string  `json:"preference,omitempty"`
	Strength   float64 `json:"strength,omitempty"`
	Level      string  `json:"level,omitempty"`
}

// PersonalityTypeDetail is the generic detail payload for configured personality type runtimes.
type PersonalityTypeDetail struct {
	TypeCode       string                       `json:"type_code"`
	TypeName       string                       `json:"type_name"`
	OneLiner       string                       `json:"one_liner,omitempty"`
	Summary        string                       `json:"summary,omitempty"`
	Pattern        string                       `json:"pattern,omitempty"`
	MatchPercent   float64                      `json:"match_percent,omitempty"`
	Similarity     float64                      `json:"similarity,omitempty"`
	ImageURL       string                       `json:"image_url,omitempty"`
	Rarity         modeltypology.Rarity         `json:"rarity,omitempty"`
	Dimensions     []PersonalityDimensionResult `json:"dimensions,omitempty"`
	Strengths      []string                     `json:"strengths,omitempty"`
	Weaknesses     []string                     `json:"weaknesses,omitempty"`
	Suggestions    []string                     `json:"suggestions,omitempty"`
	Outcome        modeltypology.Outcome        `json:"outcome,omitempty"`
	Source         modeltypology.Source         `json:"source,omitempty"`
	SpecialTrigger string                       `json:"special_trigger,omitempty"`
	IsSpecial      bool                         `json:"is_special,omitempty"`
	Commentary     string                       `json:"commentary,omitempty"`
}

// TraitProfileFactorResult is one scored factor in a generic trait-profile result.
type TraitProfileFactorResult struct {
	Code     string  `json:"code"`
	Name     string  `json:"name"`
	RawScore float64 `json:"raw_score"`
}

// TraitProfileDetail is the generic detail payload for configured trait-profile runtimes.
type TraitProfileDetail struct {
	Traits []TraitProfileFactorResult `json:"traits"`
	Source modeltypology.Source       `json:"source,omitempty"`
}
