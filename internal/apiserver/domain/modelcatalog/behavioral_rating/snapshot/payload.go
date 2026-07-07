package snapshot

import (
	"encoding/json"
	"fmt"

	brief2norm "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/behavioral_rating/brief2"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scale/snapshot"
)

// Snapshot is a published behavioral_rating execution payload (default.v1 or brief2.v1).
type Snapshot struct {
	Code                 string
	Version              string
	Title                string
	QuestionnaireCode    string
	QuestionnaireVersion string
	Status               string
	Factors              []FactorSnapshot
	Brief2               *Brief2Profile
}

// Brief2Profile carries BRIEF-2 specific configuration beyond score_range scoring.
type Brief2Profile struct {
	FormVariant      string
	NormTableVersion string
	IndexCodes       []string
	ValidityCodes    []string
	NormTables       *brief2norm.NormTables
}

// NormTablesOrNil returns parsed norm tables when Brief-2 norm configuration is present.
func (p *Brief2Profile) NormTablesOrNil() *brief2norm.NormTables {
	if p == nil {
		return nil
	}
	return p.NormTables
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
	Dimensions     []dimensionRule  `json:"dimensions"`
	InterpretRules []interpretRule  `json:"interpret_rules"`
	Brief2         *brief2Extension `json:"brief2,omitempty"`
}

type brief2Extension struct {
	FormVariant      string                `json:"form_variant,omitempty"`
	NormTableVersion string                `json:"norm_table_version,omitempty"`
	IndexCodes       []string              `json:"index_codes,omitempty"`
	ValidityCodes    []string              `json:"validity_codes,omitempty"`
	Norms            []brief2FactorPayload `json:"norms,omitempty"`
	TScoreRules      []brief2TScoreRule    `json:"t_score_rules,omitempty"`
}

type brief2FactorPayload struct {
	FactorCode string              `json:"factor_code"`
	Bands      []brief2NormBand      `json:"bands,omitempty"`
	Lookup     []brief2LookupEntry   `json:"lookup,omitempty"`
}

type brief2NormBand struct {
	MinAgeMonths int      `json:"min_age_months,omitempty"`
	MaxAgeMonths int      `json:"max_age_months,omitempty"`
	Gender       string   `json:"gender,omitempty"`
	Mean         *float64 `json:"mean,omitempty"`
	StdDev       *float64 `json:"std_dev,omitempty"`
}

type brief2LookupEntry struct {
	RawMin     float64 `json:"raw_min"`
	RawMax     float64 `json:"raw_max"`
	TScore     float64 `json:"t_score"`
	Percentile float64 `json:"percentile"`
}

type brief2TScoreRule struct {
	FactorCode string           `json:"factor_code"`
	Ranges     []brief2TScoreRange `json:"ranges"`
}

type brief2TScoreRange struct {
	MinT       float64 `json:"min_t"`
	MaxT       float64 `json:"max_t"`
	Level      string  `json:"level,omitempty"`
	Conclusion string  `json:"conclusion,omitempty"`
	Suggestion string  `json:"suggestion,omitempty"`
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

// ParseDefinitionPayload decodes a behavioral_rating payload body into a runtime snapshot.
func ParseDefinitionPayload(modelCode, modelVersion, title, status string, payload []byte) (*Snapshot, error) {
	return parseDefinitionPayload(modelCode, modelVersion, title, status, payload)
}

// ParsePublishedPayload decodes a published snapshot using its payload format label.
func ParsePublishedPayload(payloadFormat, modelCode, modelVersion, title, status string, payload []byte) (*Snapshot, error) {
	switch payloadFormat {
	case "", "assessmentmodel.behavioral_rating.default.v1", "assessmentmodel.behavioral_rating.brief2.v1":
		return parseDefinitionPayload(modelCode, modelVersion, title, status, payload)
	default:
		return nil, fmt.Errorf("unsupported behavioral_rating payload format: %s", payloadFormat)
	}
}

func parseDefinitionPayload(modelCode, modelVersion, title, status string, payload []byte) (*Snapshot, error) {
	var body definitionPayload
	if err := json.Unmarshal(payload, &body); err != nil {
		return nil, fmt.Errorf("decode behavioral_rating payload: %w", err)
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
	out := &Snapshot{
		Code:    modelCode,
		Version: modelVersion,
		Title:   title,
		Status:  status,
		Factors: factors,
	}
	if body.Brief2 != nil {
		out.Brief2 = &Brief2Profile{
			FormVariant:      body.Brief2.FormVariant,
			NormTableVersion: body.Brief2.NormTableVersion,
			IndexCodes:       append([]string(nil), body.Brief2.IndexCodes...),
			ValidityCodes:    append([]string(nil), body.Brief2.ValidityCodes...),
			NormTables:       normTablesFromPayload(body.Brief2),
		}
	}
	return out, nil
}

func normTablesFromPayload(body *brief2Extension) *brief2norm.NormTables {
	if body == nil || (len(body.Norms) == 0 && len(body.TScoreRules) == 0) {
		return nil
	}
	tables := &brief2norm.NormTables{
		FormVariant:      body.FormVariant,
		NormTableVersion: body.NormTableVersion,
		Factors:          make([]brief2norm.FactorNormTable, 0, len(body.Norms)),
		TScoreRules:      make([]brief2norm.TScoreInterpretRule, 0, len(body.TScoreRules)),
	}
	for _, factor := range body.Norms {
		table := brief2norm.FactorNormTable{
			FactorCode: factor.FactorCode,
			Bands:      make([]brief2norm.NormBand, 0, len(factor.Bands)),
			Lookup:     make([]brief2norm.NormLookupEntry, 0, len(factor.Lookup)),
		}
		for _, band := range factor.Bands {
			table.Bands = append(table.Bands, brief2norm.NormBand{
				MinAgeMonths: band.MinAgeMonths,
				MaxAgeMonths: band.MaxAgeMonths,
				Gender:       band.Gender,
				Mean:         band.Mean,
				StdDev:       band.StdDev,
			})
		}
		for _, entry := range factor.Lookup {
			table.Lookup = append(table.Lookup, brief2norm.NormLookupEntry(entry))
		}
		tables.Factors = append(tables.Factors, table)
	}
	for _, rule := range body.TScoreRules {
		converted := brief2norm.TScoreInterpretRule{FactorCode: rule.FactorCode}
		for _, item := range rule.Ranges {
			converted.Ranges = append(converted.Ranges, brief2norm.TScoreRange(item))
		}
		tables.TScoreRules = append(tables.TScoreRules, converted)
	}
	return tables
}

func (s *Snapshot) IsPublished() bool {
	return s != nil && s.Status == "published"
}

// ToScaleSnapshot projects behavioral_rating factors into the scale execution shape.
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
