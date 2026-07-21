package evaluationinput

import (
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
)

// AttachCanonicalDefinition materializes canonical DefinitionV2 and presentation
// assets onto an evaluation input snapshot (MC-R017 batch 4).
func AttachCanonicalDefinition(snapshot *InputSnapshot, def *modeldefinition.Definition) {
	if snapshot == nil || def == nil {
		return
	}
	snapshot.DefinitionV2 = def
	if snapshot.InterpretationAssets != nil && snapshot.InterpretationAssets.IsMaterialized() {
		return
	}
	assets := def.ResolvedInterpretationAssets()
	if assets.IsMaterialized() {
		copy := assets
		snapshot.InterpretationAssets = &copy
	}
}

// DefinitionV2FromSnapshot returns canonical Definition when materialized on input.
func DefinitionV2FromSnapshot(input *InputSnapshot) (*modeldefinition.Definition, bool) {
	if input == nil || input.DefinitionV2 == nil {
		return nil, false
	}
	return input.DefinitionV2, true
}

// MeasureSpecFromSnapshot returns the canonical Definition.Measure.
func MeasureSpecFromSnapshot(input *InputSnapshot) (modeldefinition.MeasureSpec, bool) {
	if def, ok := DefinitionV2FromSnapshot(input); ok && len(def.Measure.Factors) > 0 {
		return def.Measure, true
	}
	return modeldefinition.MeasureSpec{}, false
}

// FactorCatalogFromDefinition builds minimal factor metadata from canonical Measure.
func FactorCatalogFromDefinition(measure modeldefinition.MeasureSpec) []FactorCatalogEntry {
	if len(measure.Factors) == 0 {
		return nil
	}
	scoringByFactor := make(map[string]factor.Scoring, len(measure.Scoring))
	for _, rule := range measure.Scoring {
		scoringByFactor[rule.FactorCode] = rule
	}
	out := make([]FactorCatalogEntry, 0, len(measure.Factors))
	for _, item := range measure.Factors {
		entry := FactorCatalogEntry{
			Code: item.Code, Title: item.Title, IsTotalScore: item.ResolvedRole() == factor.FactorRoleTotal,
		}
		if rule, ok := scoringByFactor[item.Code]; ok {
			entry.MaxScore = rule.MaxScore
		}
		out = append(out, entry)
	}
	return out
}

// InterpretationAssetsFromSnapshot resolves frozen presentation assets from an
// evaluation input snapshot (canonical layers, report input envelope, scale payload field, etc.).
func InterpretationAssetsFromSnapshot(input *InputSnapshot) (interpretationassets.Assets, bool) {
	if input == nil {
		return interpretationassets.Assets{}, false
	}
	if input.InterpretationAssets != nil && input.InterpretationAssets.IsMaterialized() {
		return *input.InterpretationAssets, true
	}
	if def, ok := DefinitionV2FromSnapshot(input); ok {
		if assets := def.ResolvedInterpretationAssets(); assets.IsMaterialized() {
			return assets, true
		}
	}
	return interpretationassets.Assets{}, false
}

// FactorScoreVisibleCodesFromSnapshot resolves frozen factor-score section
// visibility codes from canonical DefinitionV2 on the evaluation input snapshot.
// Callers map these codes into Interpretation-owned presentation types.
func FactorScoreVisibleCodesFromSnapshot(input *InputSnapshot) ([]string, bool) {
	def, ok := DefinitionV2FromSnapshot(input)
	if !ok || def == nil {
		return nil, false
	}
	codes, configured := def.ReportMap.FactorScoreSources()
	if !configured {
		return nil, false
	}
	return append([]string(nil), codes...), true
}
