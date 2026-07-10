package behavioral

import (
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	catalognorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	sharedpayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/shared"
)

// ImportLegacyDefinition converts a behavioral wire payload into DefinitionV2.
// It is a legacy-import boundary; new publication and runtime paths consume
// the resulting DefinitionV2 rather than this payload's mechanism extensions.
func ImportLegacyDefinition(payload []byte) (sharedpayload.DefinitionMaterialization, error) {
	var body definitionPayload
	if err := json.Unmarshal(payload, &body); err != nil {
		return sharedpayload.DefinitionMaterialization{}, fmt.Errorf("decode behavioral_rating definition: %w", err)
	}
	measure := sharedpayload.MeasureSpecFromDefinitionBody(body.DefinitionBody)
	calibration := definition.Calibration{}
	conclusions := riskConclusionsFromLegacyBody(body.DefinitionBody)
	materializedNorms := make([]*catalognorm.Norm, 0, 1)
	if body.Brief2 != nil {
		measure, calibration = applyBrief2NormMetadata(measure, brief2MetadataContext{
			NormTableVersion: body.Brief2.NormTableVersion,
			IndexCodes:       append([]string(nil), body.Brief2.IndexCodes...),
			ValidityCodes:    append([]string(nil), body.Brief2.ValidityCodes...),
			NormFactorCodes:  normFactorCodesFromPayload(body.Brief2),
		})
		measure = applyBrief2CompositeMetadata(measure, compositeSpecsFromPayload(body.Brief2))
		conclusions = append(conclusions, normConclusionsFromPayload(body.Brief2)...)
		if table := normFromPayload(body.Brief2); table != nil {
			materializedNorms = append(materializedNorms, table)
		}
	}
	return sharedpayload.DefinitionMaterialization{
		Definition: &definition.Definition{Measure: measure, Calibration: calibration, Conclusions: conclusions},
		Norms:      materializedNorms,
	}, nil
}

// DefinitionFromLegacyPayload imports a behavioral wire payload as DefinitionV2.
func DefinitionFromLegacyPayload(payload []byte) (*definition.Definition, error) {
	materialized, err := ImportLegacyDefinition(payload)
	if err != nil {
		return nil, err
	}
	return materialized.Definition, nil
}

func riskConclusionsFromLegacyBody(body sharedpayload.DefinitionBody) []conclusion.Conclusion {
	if body.InterpretRules == nil {
		return nil
	}
	out := make([]conclusion.Conclusion, 0, len(body.InterpretRules))
	for _, rule := range body.InterpretRules {
		if rule.DimensionCode == "" {
			continue
		}
		ranges := make([]conclusion.ScoreRangeOutcome, 0, len(rule.Ranges))
		for _, item := range rule.Ranges {
			ranges = append(ranges, conclusion.ScoreRangeOutcome{
				MinScore: item.MinScore, MaxScore: item.MaxScore, Level: item.Level,
				Summary: item.Conclusion, Description: item.Suggestion,
			})
		}
		out = append(out, conclusion.RiskConclusion{FactorCode: rule.DimensionCode, Rules: ranges})
	}
	return out
}
