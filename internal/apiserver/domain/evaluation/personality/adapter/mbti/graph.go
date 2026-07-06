package mbti

import (
	"fmt"

	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/profile"
)

// BuildFromLegacy converts an MBTI legacy model into a factor graph and pole decision spec.
func BuildFromLegacy(model *modeltypology.MBTILegacyModel) (profile.FactorGraph, profile.DecisionSpec, error) {
	if model == nil {
		return profile.FactorGraph{}, profile.DecisionSpec{}, fmt.Errorf("mbti model is required")
	}
	if len(model.DimensionOrder) == 0 {
		return profile.FactorGraph{}, profile.DecisionSpec{}, fmt.Errorf("mbti dimension order is required")
	}

	mappingsByDimension := groupMappings(model.QuestionMappings)
	factors := make(map[profile.FactorID]profile.PersonalityFactor, len(model.DimensionOrder))
	leafSpecs := make(map[profile.FactorID]profile.LeafScoringSpec, len(model.DimensionOrder))
	poles := make([]profile.PoleSpec, 0, len(model.DimensionOrder))
	roots := make([]profile.FactorID, 0, len(model.DimensionOrder))

	for _, dimCode := range model.DimensionOrder {
		meta, ok := model.Dimensions[dimCode]
		if !ok {
			return profile.FactorGraph{}, profile.DecisionSpec{}, fmt.Errorf("mbti dimension %s is not defined", dimCode)
		}
		factorID := profile.FactorID(dimCode)
		mappings := mappingsByDimension[dimCode]
		contributions := make([]profile.AnswerContribution, 0, len(mappings))
		for _, mapping := range mappings {
			contributions = append(contributions, profile.AnswerContribution{
				QuestionCode: mapping.QuestionCode,
				Sign:         mapping.Sign,
			})
		}
		factors[factorID] = profile.PersonalityFactor{
			ID:   factorID,
			Code: dimCode,
			Name: meta.Name,
			Kind: profile.FactorKindLeaf,
		}
		leafSpecs[factorID] = profile.LeafScoringSpec{
			Constant:      meta.Constant,
			Contributions: contributions,
		}
		poles = append(poles, profile.PoleSpec{
			FactorID:     factorID,
			LeftPole:     meta.LeftPole,
			RightPole:    meta.RightPole,
			Threshold:    meta.Threshold,
			MaxDeviation: dimensionMaxDeviation(meta, mappings),
		})
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
	return graph, profile.DecisionSpec{
		Kind:  profile.DecisionKindPoleComposition,
		Poles: poles,
	}, nil
}

func groupMappings(mappings []modeltypology.MBTILegacyQuestionMapping) map[string][]modeltypology.MBTILegacyQuestionMapping {
	grouped := make(map[string][]modeltypology.MBTILegacyQuestionMapping)
	for _, mapping := range mappings {
		grouped[mapping.Dimension] = append(grouped[mapping.Dimension], mapping)
	}
	return grouped
}

func dimensionMaxDeviation(meta modeltypology.MBTILegacyDimension, mappings []modeltypology.MBTILegacyQuestionMapping) float64 {
	minScore := meta.Constant
	maxScore := meta.Constant
	for _, mapping := range mappings {
		if mapping.Dimension != meta.Code {
			continue
		}
		if mapping.Sign > 0 {
			minScore += mapping.Sign * 1
			maxScore += mapping.Sign * 5
		} else {
			minScore += mapping.Sign * 5
			maxScore += mapping.Sign * 1
		}
	}
	threshold := meta.Threshold
	if threshold == 0 {
		threshold = 24
	}
	left := threshold - minScore
	right := maxScore - threshold
	if left > right {
		return left
	}
	return right
}
