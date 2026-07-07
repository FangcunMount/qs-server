package snapshot

import (
	"encoding/json"
	"fmt"

	brief2norm "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/behavioral_rating/brief2"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
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

type (
	FactorSnapshot        = factor.FactorSnapshot
	InterpretRuleSnapshot = factor.ScoreRangeRule
)

type definitionPayload struct {
	factor.DefinitionBody
	Brief2 *brief2Extension `json:"brief2,omitempty"`
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
	Bands      []brief2NormBand    `json:"bands,omitempty"`
	Lookup     []brief2LookupEntry `json:"lookup,omitempty"`
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
	FactorCode string              `json:"factor_code"`
	Ranges     []brief2TScoreRange `json:"ranges"`
}

type brief2TScoreRange struct {
	MinT       float64 `json:"min_t"`
	MaxT       float64 `json:"max_t"`
	Level      string  `json:"level,omitempty"`
	Conclusion string  `json:"conclusion,omitempty"`
	Suggestion string  `json:"suggestion,omitempty"`
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
	out := &Snapshot{
		Code:    modelCode,
		Version: modelVersion,
		Title:   title,
		Status:  status,
	}
	factors := factor.ParseFactorsFromDefinitionBody(body.Dimensions, body.InterpretRules)
	if body.Brief2 != nil {
		factors = factor.ApplyBrief2NormMetadata(factors, factor.Brief2NormContext{
			NormTableVersion: body.Brief2.NormTableVersion,
			IndexCodes:       append([]string(nil), body.Brief2.IndexCodes...),
			ValidityCodes:    append([]string(nil), body.Brief2.ValidityCodes...),
			NormFactorCodes:  normFactorCodesFromPayload(body.Brief2),
		})
	}
	out.Factors = factors
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

func normFactorCodesFromPayload(body *brief2Extension) []string {
	if body == nil || len(body.Norms) == 0 {
		return nil
	}
	codes := make([]string, 0, len(body.Norms))
	for _, item := range body.Norms {
		if item.FactorCode != "" {
			codes = append(codes, item.FactorCode)
		}
	}
	return codes
}

func (s *Snapshot) IsPublished() bool {
	return s != nil && s.Status == "published"
}

// ToScaleSnapshot projects behavioral_rating factors into the scale execution shape.
func (s *Snapshot) ToScaleSnapshot() *scalesnapshot.ScaleSnapshot {
	if s == nil {
		return nil
	}
	return scalesnapshot.BuildFromModelFactors(
		s.Code, s.Version, s.Title, s.QuestionnaireCode, s.QuestionnaireVersion, s.Status, s.Factors,
	)
}
