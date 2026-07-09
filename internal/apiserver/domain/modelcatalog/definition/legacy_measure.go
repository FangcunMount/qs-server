package definition

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
)

// MeasureAndCalibrationFromLegacyFactors decomposes transitional flat factors into target definition layers.
func MeasureAndCalibrationFromLegacyFactors(factors []factor.LegacyFactor) (MeasureSpec, Calibration) {
	return MeasureSpecFromLegacyFactors(factors), CalibrationFromLegacyFactors(factors)
}

// MeasureSpecFromLegacyFactors decomposes transitional flat factors into measure-layer parts.
func MeasureSpecFromLegacyFactors(factors []factor.LegacyFactor) MeasureSpec {
	return MeasureSpec{
		Factors:     factor.SlimFactorsFromLegacy(factors),
		FactorGraph: factor.FactorGraphFromLegacy(factors),
		Scoring:     factor.ScoringFromLegacy(factors),
	}
}

// CalibrationFromLegacyFactors extracts calibration-layer norm references from transitional flat factors.
func CalibrationFromLegacyFactors(factors []factor.LegacyFactor) Calibration {
	legacyRefs := factor.NormRefsFromLegacy(factors)
	if legacyRefs == nil {
		return Calibration{}
	}
	refs := make([]norm.Ref, 0, len(legacyRefs))
	for _, ref := range legacyRefs {
		refs = append(refs, norm.Ref{
			FactorCode:       ref.FactorCode,
			NormTableVersion: ref.NormTableVersion,
		})
	}
	return Calibration{NormRefs: refs}
}

// LegacyFactorsFromMeasureSpec projects target definition layers back to transitional flat factors.
func LegacyFactorsFromMeasureSpec(measure MeasureSpec, calibration Calibration) []factor.LegacyFactor {
	if measure.Factors == nil {
		return nil
	}
	out := make([]factor.LegacyFactor, 0, len(measure.Factors))
	levels := measure.FactorGraph.Levels()
	scoringByFactor := make(map[string]factor.Scoring, len(measure.Scoring))
	for _, scoring := range measure.Scoring {
		scoringByFactor[scoring.FactorCode] = scoring
	}
	normByFactor := make(map[string]norm.Ref, len(calibration.NormRefs))
	for _, ref := range calibration.NormRefs {
		normByFactor[ref.FactorCode] = ref
	}
	for _, item := range measure.Factors {
		projected := factor.LegacyFactor{
			Code:       item.Code,
			Title:      item.Title,
			Role:       item.Role,
			ParentCode: measure.FactorGraph.ParentCode(item.Code),
			SortOrder:  measure.FactorGraph.SortOrders[item.Code],
			Level:      levels[item.Code],
		}
		if rule, ok := scoringByFactor[item.Code]; ok {
			applyLegacyScoring(&projected, rule)
		}
		if ref, ok := normByFactor[item.Code]; ok {
			projected.Norm = &factor.NormRef{
				FactorCode:       ref.FactorCode,
				NormTableVersion: ref.NormTableVersion,
			}
		}
		out = append(out, projected)
	}
	return out
}

// FactorSnapshotsFromMeasureSpec projects target definition layers to runtime/published compatibility DTOs.
func FactorSnapshotsFromMeasureSpec(measure MeasureSpec, calibration Calibration) []factor.FactorSnapshot {
	return factor.SnapshotsFromLegacyFactors(LegacyFactorsFromMeasureSpec(measure, calibration))
}

func applyLegacyScoring(projected *factor.LegacyFactor, rule factor.Scoring) {
	projected.ScoringStrategy = rule.Strategy.String()
	projected.ScoringParams = cloneScoringParams(rule.Params)
	projected.MaxScore = cloneFloat64(rule.MaxScore)
	switch sourceKind(rule.Sources) {
	case factor.ScoringSourceQuestion:
		projected.QuestionCodes = sourceCodes(rule.Sources)
	case factor.ScoringSourceFactor:
		projected.ChildrenPolicy = &factor.ChildrenPolicy{
			Strategy: factor.ChildrenAggregationStrategy(rule.Strategy),
			Children: sourceCodes(rule.Sources),
			Weights:  cloneWeights(rule.Weights),
		}
	}
}

func sourceKind(sources []factor.ScoringSource) factor.ScoringSourceKind {
	if len(sources) == 0 {
		return ""
	}
	return sources[0].Kind
}

func sourceCodes(sources []factor.ScoringSource) []string {
	if len(sources) == 0 {
		return nil
	}
	out := make([]string, 0, len(sources))
	for _, source := range sources {
		out = append(out, source.Code)
	}
	return out
}

func cloneScoringParams(params *factor.ScoringParams) *factor.ScoringParams {
	if params == nil {
		return nil
	}
	return &factor.ScoringParams{CntOptionContents: append([]string(nil), params.CntOptionContents...)}
}

func cloneFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneWeights(weights map[string]float64) map[string]float64 {
	if weights == nil {
		return nil
	}
	out := make(map[string]float64, len(weights))
	for key, value := range weights {
		out[key] = value
	}
	return out
}
