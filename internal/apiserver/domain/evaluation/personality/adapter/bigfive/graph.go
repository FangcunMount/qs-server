package bigfive

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/profile"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
)

// BuildFromPayload converts a v2 typology payload into a trait-profile factor graph.
func BuildFromPayload(payload *modeltypology.Payload) (profile.FactorGraph, profile.DecisionSpec, error) {
	if err := validatePayload(payload); err != nil {
		return profile.FactorGraph{}, profile.DecisionSpec{}, err
	}
	mappingsByDimension := groupMappings(payload.QuestionMappings)
	factors := make(map[profile.FactorID]profile.PersonalityFactor, len(payload.DimensionOrder))
	leafSpecs := make(map[profile.FactorID]profile.LeafScoringSpec, len(payload.DimensionOrder))
	roots := make([]profile.FactorID, 0, len(payload.DimensionOrder))

	for _, dimCode := range payload.DimensionOrder {
		meta, ok := payload.Dimensions[dimCode]
		if !ok {
			return profile.FactorGraph{}, profile.DecisionSpec{}, fmt.Errorf("bigfive dimension %s is not defined", dimCode)
		}
		mappings := mappingsByDimension[dimCode]
		if len(mappings) == 0 {
			return profile.FactorGraph{}, profile.DecisionSpec{}, fmt.Errorf("bigfive dimension %s has no mapped answers", dimCode)
		}
		contributions := make([]profile.AnswerContribution, 0, len(mappings))
		optionScoring := profile.OptionScoringStrict
		for _, mapping := range mappings {
			contribution := profile.AnswerContribution{
				QuestionCode: mapping.QuestionCode,
				Sign:         mapping.Sign,
			}
			if len(mapping.OptionScores) > 0 {
				contribution.OptionScores = cloneOptionScores(mapping.OptionScores)
			}
			contributions = append(contributions, contribution)
		}
		factorID := profile.FactorID(dimCode)
		factors[factorID] = profile.PersonalityFactor{
			ID:   factorID,
			Code: meta.Code,
			Name: meta.Name,
			Kind: profile.FactorKindLeaf,
		}
		leafSpecs[factorID] = profile.LeafScoringSpec{
			Constant:      meta.Constant,
			Contributions: contributions,
			OptionScoring: optionScoring,
		}
		roots = append(roots, factorID)
	}

	graph := profile.FactorGraph{
		Factors:   factors,
		LeafSpecs: leafSpecs,
		Roots:     roots,
	}
	if err := graph.Validate(); err != nil {
		return profile.FactorGraph{}, profile.DecisionSpec{}, err
	}
	return graph, profile.DecisionSpec{Kind: profile.DecisionKindTraitProfile}, nil
}

func validatePayload(payload *modeltypology.Payload) error {
	if payload == nil {
		return fmt.Errorf("bigfive payload is required")
	}
	if payload.Algorithm != modelcatalog.AlgorithmBigFive {
		return fmt.Errorf("typology algorithm %s is not bigfive", payload.Algorithm)
	}
	if len(payload.DimensionOrder) == 0 {
		return fmt.Errorf("bigfive dimension order is required")
	}
	kind := payload.MatchingSpec.Kind
	if kind == "" {
		kind = modelcatalog.DecisionKindTraitProfile
	}
	if kind != modelcatalog.DecisionKindTraitProfile {
		return fmt.Errorf("bigfive matching kind %s is not trait_profile", kind)
	}
	return nil
}

func groupMappings(mappings []modeltypology.QuestionMapping) map[string][]modeltypology.QuestionMapping {
	grouped := make(map[string][]modeltypology.QuestionMapping)
	for _, mapping := range mappings {
		grouped[mapping.Dimension] = append(grouped[mapping.Dimension], mapping)
	}
	return grouped
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
