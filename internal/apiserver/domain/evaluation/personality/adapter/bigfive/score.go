package bigfive

import (
	"fmt"

	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/profile"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
)

// Score evaluates a Big Five v2 typology payload through the trait-profile pipeline.
func Score(payload *modeltypology.Payload, sheet *evaluationinput.AnswerSheet) (evaluationtypology.BigFiveResultDetail, error) {
	if sheet == nil {
		return evaluationtypology.BigFiveResultDetail{}, fmt.Errorf("answer sheet is required")
	}
	graph, spec, err := BuildFromPayload(payload)
	if err != nil {
		return evaluationtypology.BigFiveResultDetail{}, err
	}
	vector, err := profile.ScoreGraph(graph, sheet)
	if err != nil {
		return evaluationtypology.BigFiveResultDetail{}, err
	}
	outcome, err := profile.SelectOutcome(vector, spec)
	if err != nil {
		return evaluationtypology.BigFiveResultDetail{}, err
	}
	traits := make([]evaluationtypology.BigFiveTraitResult, 0, len(payload.DimensionOrder))
	for _, dimCode := range payload.DimensionOrder {
		meta := payload.Dimensions[dimCode]
		raw, ok := outcome.TraitScores[profile.FactorID(dimCode)]
		if !ok {
			return evaluationtypology.BigFiveResultDetail{}, fmt.Errorf("missing trait score for %s", dimCode)
		}
		traits = append(traits, evaluationtypology.BigFiveTraitResult{
			Code:     meta.Code,
			Name:     meta.Name,
			RawScore: raw,
		})
	}
	return evaluationtypology.BigFiveResultDetail{
		Traits: traits,
		Source: payload.Source,
	}, nil
}
