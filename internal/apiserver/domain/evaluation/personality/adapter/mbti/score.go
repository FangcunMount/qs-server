package mbti

import (
	"fmt"

	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/profile"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
)

// Score evaluates an MBTI legacy model through the generic factor-graph pipeline.
func Score(model *modeltypology.MBTILegacyModel, sheet *evaluationinput.AnswerSheet) (evaluationtypology.MBTIResultDetail, error) {
	if model == nil {
		return evaluationtypology.MBTIResultDetail{}, fmt.Errorf("mbti model is required")
	}
	if sheet == nil {
		return evaluationtypology.MBTIResultDetail{}, fmt.Errorf("answer sheet is required")
	}

	graph, spec, err := BuildFromLegacy(model)
	if err != nil {
		return evaluationtypology.MBTIResultDetail{}, err
	}
	vector, err := profile.ScoreGraph(graph, sheet)
	if err != nil {
		return evaluationtypology.MBTIResultDetail{}, err
	}
	outcome, err := profile.SelectOutcome(vector, spec)
	if err != nil {
		return evaluationtypology.MBTIResultDetail{}, err
	}
	typeProfile, ok := model.FindTypeProfile(outcome.Code)
	if !ok {
		return evaluationtypology.MBTIResultDetail{}, fmt.Errorf("mbti type profile not found for %s", outcome.Code)
	}

	dimensions := make([]evaluationtypology.MBTIDimensionResult, 0, len(spec.Poles))
	for _, pole := range spec.Poles {
		score, ok := vector.Scores[pole.FactorID]
		if !ok {
			return evaluationtypology.MBTIResultDetail{}, fmt.Errorf("missing factor score for %s", pole.FactorID)
		}
		meta := model.Dimensions[string(pole.FactorID)]
		preference, strength := profile.ResolvePole(pole, score.Raw)
		dimensions = append(dimensions, evaluationtypology.MBTIDimensionResult{
			Code:       meta.Code,
			Name:       meta.Name,
			LeftPole:   meta.LeftPole,
			RightPole:  meta.RightPole,
			RawScore:   score.Raw,
			Preference: preference,
			Strength:   strength,
		})
	}

	return evaluationtypology.MBTIResultDetail{
		TypeCode:     outcome.Code,
		TypeName:     typeProfile.TypeName,
		OneLiner:     typeProfile.OneLiner,
		MatchPercent: outcome.MatchScore,
		ImageURL:     typeProfile.ImageURL,
		Dimensions:   dimensions,
		Profile:      typeProfile,
		Source:       model.Source,
	}, nil
}
