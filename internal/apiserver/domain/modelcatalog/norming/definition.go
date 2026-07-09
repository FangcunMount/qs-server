package norming

import (
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
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
}

type brief2CompositeIndex struct {
	Code       string   `json:"code"`
	Strategy   string   `json:"strategy,omitempty"`
	Children   []string `json:"children"`
	ParentCode string   `json:"parent_code,omitempty"`
}

type brief2FactorPayload struct {
	FactorCode string `json:"factor_code"`
}

// DefinitionFromPayload materializes the target behavioral-rating definition
// model from the legacy draft payload without changing the payload contract.
func DefinitionFromPayload(payload []byte) (*definition.Definition, error) {
	var body definitionPayload
	if err := json.Unmarshal(payload, &body); err != nil {
		return nil, fmt.Errorf("decode behavioral_rating definition: %w", err)
	}
	factors := factor.ParseLegacyFactorsFromDefinitionBody(body.Dimensions, body.InterpretRules)
	if body.Brief2 != nil {
		factors = ApplyNormMetadataToLegacyFactors(factors, MetadataContext{
			NormTableVersion: body.Brief2.NormTableVersion,
			IndexCodes:       append([]string(nil), body.Brief2.IndexCodes...),
			ValidityCodes:    append([]string(nil), body.Brief2.ValidityCodes...),
			NormFactorCodes:  normFactorCodesFromBrief2(body.Brief2),
		})
		factors = ApplyCompositeMetadataToLegacyFactors(factors, compositeSpecsFromBrief2(body.Brief2))
	}
	measure, calibration := definition.MeasureAndCalibrationFromLegacyFactors(factors)
	return &definition.Definition{
		Measure:     measure,
		Calibration: calibration,
	}, nil
}

func normFactorCodesFromBrief2(body *brief2Extension) []string {
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

func compositeSpecsFromBrief2(body *brief2Extension) []CompositeIndexSpec {
	if body == nil || len(body.CompositeIndexes) == 0 {
		return nil
	}
	specs := make([]CompositeIndexSpec, 0, len(body.CompositeIndexes))
	for _, item := range body.CompositeIndexes {
		if item.Code == "" || len(item.Children) == 0 {
			continue
		}
		specs = append(specs, CompositeIndexSpec{
			Code:       item.Code,
			Strategy:   factor.ChildrenAggregationStrategy(item.Strategy),
			Children:   append([]string(nil), item.Children...),
			ParentCode: item.ParentCode,
		})
	}
	return specs
}
