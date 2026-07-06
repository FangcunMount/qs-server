package sbti

import (
	"fmt"
	"strings"

	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/profile"
)

// BuildFromLegacy converts an SBTI legacy model into a factor graph and pattern decision spec.
func BuildFromLegacy(model *modeltypology.SBTILegacyModel) (profile.FactorGraph, profile.DecisionSpec, error) {
	if model == nil {
		return profile.FactorGraph{}, profile.DecisionSpec{}, fmt.Errorf("sbti model is required")
	}
	if len(model.DimensionOrder) == 0 {
		return profile.FactorGraph{}, profile.DecisionSpec{}, fmt.Errorf("sbti dimension order is required")
	}

	mappingsByDimension := groupMappings(model.QuestionMappings)
	factors := make(map[profile.FactorID]profile.PersonalityFactor, len(model.DimensionOrder))
	leafSpecs := make(map[profile.FactorID]profile.LeafScoringSpec, len(model.DimensionOrder))
	patternOrder := make([]profile.FactorID, 0, len(model.DimensionOrder))
	roots := make([]profile.FactorID, 0, len(model.DimensionOrder))

	for _, dimCode := range model.DimensionOrder {
		meta, ok := model.Dimensions[dimCode]
		if !ok {
			return profile.FactorGraph{}, profile.DecisionSpec{}, fmt.Errorf("sbti dimension %s is not defined", dimCode)
		}
		factorID := profile.FactorID(dimCode)
		mappings := mappingsByDimension[dimCode]
		if len(mappings) == 0 {
			return profile.FactorGraph{}, profile.DecisionSpec{}, fmt.Errorf("sbti dimension %s has no mapped answers", dimCode)
		}
		contributions := make([]profile.AnswerContribution, 0, len(mappings))
		for _, mapping := range mappings {
			contributions = append(contributions, profile.AnswerContribution{
				QuestionCode: mapping.QuestionCode,
				OptionScores: cloneOptionScores(mapping.OptionScores),
			})
		}
		factors[factorID] = profile.PersonalityFactor{
			ID:   factorID,
			Code: dimCode,
			Name: meta.Name,
			Kind: profile.FactorKindLeaf,
		}
		leafSpecs[factorID] = profile.LeafScoringSpec{
			Contributions: contributions,
			OptionScoring: profile.OptionScoringCompat,
		}
		patternOrder = append(patternOrder, factorID)
		roots = append(roots, factorID)
	}

	patterns := make([]profile.PatternCandidate, 0, len(model.NormalOutcomes))
	for _, outcome := range model.NormalOutcomes {
		patterns = append(patterns, profile.PatternCandidate{
			Code:    outcome.Code,
			Label:   outcome.Name,
			Pattern: patternLevelsByOrder(outcome.Pattern, patternOrder),
		})
	}

	graph := profile.FactorGraph{
		Factors:   factors,
		LeafSpecs: leafSpecs,
		Roots:     roots,
	}
	if err := graph.Validate(); err != nil {
		return profile.FactorGraph{}, profile.DecisionSpec{}, err
	}
	return graph, profile.DecisionSpec{
		Kind:              profile.DecisionKindNearestPattern,
		PatternOrder:      patternOrder,
		Patterns:          patterns,
		LevelRule:         profile.LevelRule{LowMax: 3, HighMin: 5},
		FallbackThreshold: fallbackThreshold(model),
		FallbackCode:      "HHHH",
	}, nil
}

func groupMappings(mappings []modeltypology.SBTILegacyQuestionMapping) map[string][]modeltypology.SBTILegacyQuestionMapping {
	grouped := make(map[string][]modeltypology.SBTILegacyQuestionMapping)
	for _, mapping := range mappings {
		grouped[mapping.Dimension] = append(grouped[mapping.Dimension], mapping)
	}
	return grouped
}

func patternLevelsByOrder(pattern string, order []profile.FactorID) map[profile.FactorID]string {
	compact := strings.ReplaceAll(pattern, "-", "")
	levels := make(map[profile.FactorID]string, len(order))
	for i, factorID := range order {
		if i >= len(compact) {
			break
		}
		levels[factorID] = strings.ToUpper(string(compact[i]))
	}
	return levels
}

func cloneOptionScores(source map[string]float64) map[string]float64 {
	if source == nil {
		return nil
	}
	cloned := make(map[string]float64, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func fallbackThreshold(model *modeltypology.SBTILegacyModel) float64 {
	if model == nil || model.FallbackSimilarityThreshold <= 0 {
		return 0.6
	}
	return model.FallbackSimilarityThreshold
}
