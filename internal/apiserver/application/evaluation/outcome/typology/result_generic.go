package typology

import modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"

// PersonalityDimensionResult 是model-中性 scored 因子 shown in 人格类型结果。
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
	Rank       int     `json:"rank,omitempty"`
}

// PersonalityTypeDetail 是通用明细载荷 用于 配置化 人格类型 运行时s。
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

// ClassificationFact is the schema-v2 durable typology fact. It deliberately
// excludes report prose and display assets, which are frozen in ReportInput.
type ClassificationFact struct {
	TypeCode       string  `json:"type_code"`
	Pattern        string  `json:"pattern,omitempty"`
	MatchPercent   float64 `json:"match_percent,omitempty"`
	Similarity     float64 `json:"similarity,omitempty"`
	SpecialTrigger string  `json:"special_trigger,omitempty"`
	IsSpecial      bool    `json:"is_special,omitempty"`
}

// TraitProfileFactorResult 是一个scored 因子 in 通用特质画像结果。
type TraitProfileFactorResult struct {
	Code     string  `json:"code"`
	Name     string  `json:"name"`
	RawScore float64 `json:"raw_score"`
}

// TraitProfileDetail 是通用明细载荷 用于 配置化 trait-画像 运行时s。
type TraitProfileDetail struct {
	Traits []TraitProfileFactorResult `json:"traits"`
	Source modeltypology.Source       `json:"source,omitempty"`
}
