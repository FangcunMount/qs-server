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

// PayloadFromDefinition projects canonical behavioral semantics to the legacy
// wire body. It is intended for definitions without an embedded BRIEF-2 norm
// table, such as import and compatibility tests.
func PayloadFromDefinition(def *definition.Definition) ([]byte, error) {
	return PayloadFromDefinitionWithNorm(def, nil)
}

// PayloadFromDefinitionWithNorm projects DefinitionV2 and independently stored
// BRIEF-2 norm material into the published wire artifact.
func PayloadFromDefinitionWithNorm(def *definition.Definition, table *catalognorm.Norm) ([]byte, error) {
	body := definitionPayload{DefinitionBody: sharedpayload.DefinitionBodyFromDefinition(def)}
	brief2, err := brief2ExtensionFromDefinition(def, table)
	if err != nil {
		return nil, err
	}
	if brief2 != nil {
		body.Brief2 = brief2
	}
	return json.Marshal(body)
}

func brief2ExtensionFromDefinition(def *definition.Definition, table *catalognorm.Norm) (*brief2Extension, error) {
	if def == nil {
		return nil, nil
	}
	ext := &brief2Extension{}
	if spec := def.Execution.Brief2; spec != nil {
		ext.FormVariant = spec.FormVariant
		ext.PrimaryDimensionCode = spec.PrimaryFactorCode
		ext.IndexCodes = append(ext.IndexCodes, spec.IndexFactorCodes...)
		ext.ValidityCodes = append(ext.ValidityCodes, spec.ValidityFactorCodes...)
	}
	for _, ref := range def.Calibration.NormRefs {
		if ref.NormTableVersion != "" {
			if ext.NormTableVersion != "" && ext.NormTableVersion != ref.NormTableVersion {
				return nil, fmt.Errorf("brief2 definition references multiple norm table versions: %s and %s", ext.NormTableVersion, ref.NormTableVersion)
			}
			ext.NormTableVersion = ref.NormTableVersion
		}
		if ref.FactorCode != "" {
			ext.Norms = append(ext.Norms, brief2FactorPayload{FactorCode: ref.FactorCode})
		}
	}
	for _, item := range def.Measure.Factors {
		switch item.ResolvedRole() {
		case factor.FactorRoleIndex:
			ext.IndexCodes = append(ext.IndexCodes, item.Code)
		case factor.FactorRoleValidity:
			ext.ValidityCodes = append(ext.ValidityCodes, item.Code)
		}
	}
	ext.IndexCodes = uniqueStrings(ext.IndexCodes)
	ext.ValidityCodes = uniqueStrings(ext.ValidityCodes)
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
			rule.Ranges = append(rule.Ranges, brief2TScoreRange{
				MinT: value.MinScore, MaxT: value.MaxScore, MaxInclusive: value.MaxInclusive, UnboundedMax: value.UnboundedMax,
				Level: value.Level, Conclusion: value.Summary, Suggestion: value.Description,
			})
		}
		ext.TScoreRules = append(ext.TScoreRules, rule)
	}
	if table != nil {
		if ext.NormTableVersion != "" && ext.NormTableVersion != table.TableVersion {
			return nil, fmt.Errorf("brief2 norm table version %s does not match definition reference %s", table.TableVersion, ext.NormTableVersion)
		}
		ext.NormTableVersion = table.TableVersion
		ext.FormVariant = table.FormVariant
		ext.Norms = brief2NormsFromTable(table)
	}
	if ext.NormTableVersion == "" && len(ext.CompositeIndexes) == 0 && len(ext.TScoreRules) == 0 {
		return nil, nil
	}
	return ext, nil
}

func uniqueStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func brief2NormsFromTable(table *catalognorm.Norm) []brief2FactorPayload {
	if table == nil || len(table.Factors) == 0 {
		return nil
	}
	out := make([]brief2FactorPayload, 0, len(table.Factors))
	for _, factorTable := range table.Factors {
		item := brief2FactorPayload{FactorCode: factorTable.FactorCode}
		if len(factorTable.Bands) > 0 {
			item.Bands = make([]brief2NormBand, 0, len(factorTable.Bands))
			for _, band := range factorTable.Bands {
				item.Bands = append(item.Bands, brief2NormBand{
					MinAgeMonths: band.MinAgeMonths,
					MaxAgeMonths: band.MaxAgeMonths,
					Gender:       band.Gender,
					Mean:         cloneFloat64(band.Mean),
					StdDev:       cloneFloat64(band.StdDev),
				})
			}
		}
		if len(factorTable.Lookup) > 0 {
			item.Lookup = make([]brief2LookupEntry, 0, len(factorTable.Lookup))
			for _, entry := range factorTable.Lookup {
				item.Lookup = append(item.Lookup, brief2LookupEntry{
					RawMin:       entry.RawScoreMin,
					RawMax:       entry.RawScoreMax,
					MinAgeMonths: entry.MinAgeMonths,
					MaxAgeMonths: entry.MaxAgeMonths,
					Gender:       entry.Gender,
					TScore:       entry.TScore,
					Percentile:   entry.Percentile,
				})
			}
		}
		out = append(out, item)
	}
	return out
}
