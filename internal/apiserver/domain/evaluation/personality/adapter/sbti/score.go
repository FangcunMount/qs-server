package sbti

import (
	"fmt"
	"strings"

	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/profile"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
)

// Score evaluates an SBTI legacy model through the generic factor-graph pipeline.
func Score(model *modeltypology.SBTILegacyModel, sheet *evaluationinput.AnswerSheet) (evaluationtypology.SBTIResultDetail, error) {
	if model == nil {
		return evaluationtypology.SBTIResultDetail{}, fmt.Errorf("sbti model is required")
	}
	if sheet == nil {
		return evaluationtypology.SBTIResultDetail{}, fmt.Errorf("answer sheet is required")
	}
	if outcome, ok := triggeredDrinkOutcome(model, sheet.Answers); ok {
		return resultDetailFromOutcome(model, outcome, nil, 1, strings.TrimSpace(outcome.Trigger)), nil
	}

	graph, spec, err := BuildFromLegacy(model)
	if err != nil {
		return evaluationtypology.SBTIResultDetail{}, err
	}
	vector, err := profile.ScoreGraph(graph, sheet)
	if err != nil {
		return evaluationtypology.SBTIResultDetail{}, err
	}
	dimensions := buildDimensionResults(model, vector, spec.LevelRule)
	candidate, err := profile.SelectOutcome(vector, spec)
	if err != nil {
		return evaluationtypology.SBTIResultDetail{}, err
	}

	outcome, ok := findOutcome(model.NormalOutcomes, candidate.Code)
	if !ok {
		return evaluationtypology.SBTIResultDetail{}, fmt.Errorf("sbti outcome %s is not configured", candidate.Code)
	}
	trigger := ""
	similarity := candidate.MatchScore
	if spec.FallbackThreshold > 0 && similarity < spec.FallbackThreshold {
		fallback, ok := findOutcome(model.SpecialOutcomes, spec.FallbackCode)
		if !ok {
			return evaluationtypology.SBTIResultDetail{}, fmt.Errorf("sbti fallback outcome %s is not configured", spec.FallbackCode)
		}
		outcome = fallback
		trigger = fallback.Trigger
	}
	return resultDetailFromOutcome(model, outcome, dimensions, similarity, trigger), nil
}

func buildDimensionResults(
	model *modeltypology.SBTILegacyModel,
	vector profile.ProfileVector,
	rule profile.LevelRule,
) []evaluationtypology.SBTIDimensionResult {
	results := make([]evaluationtypology.SBTIDimensionResult, 0, len(model.DimensionOrder))
	for _, dimCode := range model.DimensionOrder {
		meta := model.Dimensions[dimCode]
		score := vector.Scores[profile.FactorID(dimCode)]
		results = append(results, evaluationtypology.SBTIDimensionResult{
			Code:     dimCode,
			Name:     meta.Name,
			Model:    meta.Model,
			RawScore: score.Raw,
			Level:    profile.LevelForScore(score.Raw, rule),
		})
	}
	return results
}

func resultDetailFromOutcome(
	model *modeltypology.SBTILegacyModel,
	outcome modeltypology.SBTILegacyOutcome,
	dimensions []evaluationtypology.SBTIDimensionResult,
	similarity float64,
	trigger string,
) evaluationtypology.SBTIResultDetail {
	return evaluationtypology.SBTIResultDetail{
		TypeCode:       outcome.Code,
		TypeName:       outcome.Name,
		OneLiner:       outcome.OneLiner,
		Pattern:        outcome.Pattern,
		Similarity:     similarity,
		ImageURL:       outcome.Image,
		Rarity:         outcome.Rarity,
		Dimensions:     dimensions,
		Outcome:        outcome,
		Source:         model.Source,
		SpecialTrigger: trigger,
	}
}
