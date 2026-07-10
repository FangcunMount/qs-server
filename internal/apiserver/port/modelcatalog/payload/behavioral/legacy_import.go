package behavioral

import (
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
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

// PayloadFromDefinition projects canonical behavioral semantics to the legacy
// wire body. Norm tables stay in their independent repository; only NormRefs
// and conclusion metadata are represented here.
func PayloadFromDefinition(def *definition.Definition) ([]byte, error) {
	body := definitionPayload{DefinitionBody: sharedpayload.DefinitionBodyFromDefinition(def)}
	brief2 := brief2ExtensionFromDefinition(def)
	if brief2 != nil {
		body.Brief2 = brief2
	}
	return json.Marshal(body)
}

// PreserveLegacyNormTables retains independently stored BRIEF-2 table data in
// the compatibility payload while its semantic references are projected from
// DefinitionV2. New domain logic never reads this embedded copy.
func PreserveLegacyNormTables(projected, legacy []byte) ([]byte, error) {
	if len(projected) == 0 || len(legacy) == 0 {
		return projected, nil
	}
	var next, previous definitionPayload
	if err := json.Unmarshal(projected, &next); err != nil {
		return nil, fmt.Errorf("decode projected behavioral payload: %w", err)
	}
	if err := json.Unmarshal(legacy, &previous); err != nil {
		return projected, nil
	}
	if next.Brief2 == nil || previous.Brief2 == nil {
		return projected, nil
	}
	if next.Brief2.NormTableVersion != "" && next.Brief2.NormTableVersion != previous.Brief2.NormTableVersion {
		return projected, nil
	}
	if next.Brief2.FormVariant == "" {
		next.Brief2.FormVariant = previous.Brief2.FormVariant
	}
	if next.Brief2.NormTableVersion == "" {
		next.Brief2.NormTableVersion = previous.Brief2.NormTableVersion
	}
	if len(next.Brief2.IndexCodes) == 0 {
		next.Brief2.IndexCodes = append([]string(nil), previous.Brief2.IndexCodes...)
	}
	if len(next.Brief2.ValidityCodes) == 0 {
		next.Brief2.ValidityCodes = append([]string(nil), previous.Brief2.ValidityCodes...)
	}
	if len(previous.Brief2.Norms) > 0 {
		next.Brief2.Norms = previous.Brief2.Norms
	}
	return json.Marshal(next)
}

func brief2ExtensionFromDefinition(def *definition.Definition) *brief2Extension {
	if def == nil {
		return nil
	}
	ext := &brief2Extension{}
	for _, ref := range def.Calibration.NormRefs {
		if ext.NormTableVersion == "" {
			ext.NormTableVersion = ref.NormTableVersion
		}
		if ref.FactorCode != "" {
			ext.Norms = append(ext.Norms, brief2FactorPayload{FactorCode: ref.FactorCode})
		}
	}
	for _, item := range def.Measure.Scoring {
		children := make([]string, 0)
		for _, source := range item.Sources {
			if source.Kind == factor.ScoringSourceFactor {
				children = append(children, source.Code)
			}
		}
		if len(children) > 0 {
			ext.CompositeIndexes = append(ext.CompositeIndexes, brief2CompositeIndex{Code: item.FactorCode, Strategy: string(item.Strategy), Children: children, ParentCode: def.Measure.FactorGraph.ParentCode(item.FactorCode)})
		}
	}
	for _, item := range def.Conclusions {
		normConclusion, ok := item.(conclusion.NormConclusion)
		if !ok {
			continue
		}
		if normConclusion.Primary {
			ext.PrimaryDimensionCode = normConclusion.FactorCode
		}
		rule := brief2TScoreRule{FactorCode: normConclusion.FactorCode, Ranges: make([]brief2TScoreRange, 0, len(normConclusion.Rules))}
		for _, value := range normConclusion.Rules {
			rule.Ranges = append(rule.Ranges, brief2TScoreRange{MinT: value.MinScore, MaxT: value.MaxScore, Level: value.Level, Conclusion: value.Summary, Suggestion: value.Description})
		}
		ext.TScoreRules = append(ext.TScoreRules, rule)
	}
	if ext.NormTableVersion == "" && len(ext.CompositeIndexes) == 0 && len(ext.TScoreRules) == 0 {
		return nil
	}
	return ext
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
