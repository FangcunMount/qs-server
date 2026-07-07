package snapshot

import (
	"encoding/json"
	"fmt"

	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	factornorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norming"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/scale/snapshot"
)

// Snapshot 是published behavioral_rating 执行载荷 (默认.v1 或 brief2.v1)。
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

// Brief2Profile 携带BRIEF-2 特定 配置 beyond score_range 计分。
type Brief2Profile struct {
	FormVariant          string
	NormTableVersion     string
	IndexCodes           []string
	ValidityCodes        []string
	PrimaryDimensionCode string
	NormTables           *calcnorm.NormTables
}

// NormTablesOrNil 返回parsed 常模表 when Brief-2 常模 配置 是 存在。
func (p *Brief2Profile) NormTablesOrNil() *calcnorm.NormTables {
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
	FormVariant          string                 `json:"form_variant,omitempty"`
	NormTableVersion     string                 `json:"norm_table_version,omitempty"`
	IndexCodes           []string               `json:"index_codes,omitempty"`
	ValidityCodes        []string               `json:"validity_codes,omitempty"`
	PrimaryDimensionCode string                 `json:"primary_dimension_code,omitempty"`
	CompositeIndexes     []brief2CompositeIndex `json:"composite_indexes,omitempty"`
	Norms                []brief2FactorPayload  `json:"norms,omitempty"`
	TScoreRules          []brief2TScoreRule     `json:"t_score_rules,omitempty"`
}

type brief2CompositeIndex struct {
	Code       string   `json:"code"`
	Strategy   string   `json:"strategy,omitempty"`
	Children   []string `json:"children"`
	ParentCode string   `json:"parent_code,omitempty"`
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

// ParseDefinitionPayload de编码 behavioral_rating 载荷 body 为 运行时 快照。
func ParseDefinitionPayload(modelCode, modelVersion, title, status string, payload []byte) (*Snapshot, error) {
	return parseDefinitionPayload(modelCode, modelVersion, title, status, payload)
}

// ParsePublishedPayload de编码 已发布快照 using its 载荷格式 label。
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
		factors = factornorm.ApplyNormMetadata(factors, factornorm.MetadataContext{
			NormTableVersion: body.Brief2.NormTableVersion,
			IndexCodes:       append([]string(nil), body.Brief2.IndexCodes...),
			ValidityCodes:    append([]string(nil), body.Brief2.ValidityCodes...),
			NormFactorCodes:  normFactorCodesFromPayload(body.Brief2),
		})
		factors = factornorm.ApplyCompositeMetadata(factors, compositeSpecsFromPayload(body.Brief2))
	}
	out.Factors = factors
	if body.Brief2 != nil {
		out.Brief2 = &Brief2Profile{
			FormVariant:          body.Brief2.FormVariant,
			NormTableVersion:     body.Brief2.NormTableVersion,
			IndexCodes:           append([]string(nil), body.Brief2.IndexCodes...),
			ValidityCodes:        append([]string(nil), body.Brief2.ValidityCodes...),
			PrimaryDimensionCode: body.Brief2.PrimaryDimensionCode,
			NormTables:           normTablesFromPayload(body.Brief2),
		}
	}
	return out, nil
}

func normTablesFromPayload(body *brief2Extension) *calcnorm.NormTables {
	if body == nil || (len(body.Norms) == 0 && len(body.TScoreRules) == 0) {
		return nil
	}
	tables := &calcnorm.NormTables{
		FormVariant:      body.FormVariant,
		NormTableVersion: body.NormTableVersion,
		Factors:          make([]calcnorm.FactorNormTable, 0, len(body.Norms)),
		TScoreRules:      make([]calcnorm.TScoreInterpretRule, 0, len(body.TScoreRules)),
	}
	for _, factor := range body.Norms {
		table := calcnorm.FactorNormTable{
			FactorCode: factor.FactorCode,
			Bands:      make([]calcnorm.NormBand, 0, len(factor.Bands)),
			Lookup:     make([]calcnorm.NormLookupEntry, 0, len(factor.Lookup)),
		}
		for _, band := range factor.Bands {
			table.Bands = append(table.Bands, calcnorm.NormBand{
				MinAgeMonths: band.MinAgeMonths,
				MaxAgeMonths: band.MaxAgeMonths,
				Gender:       band.Gender,
				Mean:         band.Mean,
				StdDev:       band.StdDev,
			})
		}
		for _, entry := range factor.Lookup {
			table.Lookup = append(table.Lookup, calcnorm.NormLookupEntry(entry))
		}
		tables.Factors = append(tables.Factors, table)
	}
	for _, rule := range body.TScoreRules {
		converted := calcnorm.TScoreInterpretRule{FactorCode: rule.FactorCode}
		for _, item := range rule.Ranges {
			converted.Ranges = append(converted.Ranges, calcnorm.TScoreRange(item))
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

func compositeSpecsFromPayload(body *brief2Extension) []factornorm.CompositeIndexSpec {
	if body == nil || len(body.CompositeIndexes) == 0 {
		return nil
	}
	specs := make([]factornorm.CompositeIndexSpec, 0, len(body.CompositeIndexes))
	for _, item := range body.CompositeIndexes {
		if item.Code == "" || len(item.Children) == 0 {
			continue
		}
		specs = append(specs, factornorm.CompositeIndexSpec{
			Code:       item.Code,
			Strategy:   factor.ChildrenAggregationStrategy(item.Strategy),
			Children:   append([]string(nil), item.Children...),
			ParentCode: item.ParentCode,
		})
	}
	return specs
}

func (s *Snapshot) IsPublished() bool {
	return s != nil && s.Status == "published"
}

// ToScaleSnapshot 投影behavioral_rating 因子 为 scale execution 结构。
func (s *Snapshot) ToScaleSnapshot() *scalesnapshot.ScaleSnapshot {
	if s == nil {
		return nil
	}
	return scalesnapshot.BuildFromModelFactors(
		s.Code, s.Version, s.Title, s.QuestionnaireCode, s.QuestionnaireVersion, s.Status, s.Factors,
	)
}
