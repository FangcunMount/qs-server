package commit

import (
	"fmt"
	"math"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/classification"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

// Application orchestration may combine calculation rules and neutral frozen
// input contracts. The input port itself must not depend on calculation.
func reportInputFreezeOptions(input *evaluationinput.InputSnapshot, ref evaluationinput.ModelRef, decision modelcatalog.DecisionKind) (evaluationinput.ReportInputFreezeOptions, error) {
	opts := evaluationinput.BuildFreezeOptionsFromSnapshot(input, ref, decision)
	if !evaluationinput.RequiresMBTIPoleCatalog(ref) {
		return opts, nil
	}
	if input == nil || input.Model == nil || input.Model.Kind != ref.Kind || input.Model.Code != ref.Code || input.Model.Version != ref.Version || input.Model.Algorithm != ref.Algorithm {
		return opts, fmt.Errorf("MBTI definition snapshot model identity mismatch")
	}
	if decision != modelcatalog.DecisionKindPoleComposition {
		return opts, fmt.Errorf("MBTI pole decision is required")
	}
	def, ok := evaluationinput.DefinitionV2FromSnapshot(input)
	if !ok {
		return opts, fmt.Errorf("MBTI canonical definition is required")
	}
	var err error
	opts.MBTIPoles, err = freezeMBTIPoles(def, ref, input.Questionnaire)
	return opts, err
}

func freezeMBTIPoles(def *definition.Definition, ref evaluationinput.ModelRef, questionnaire *evaluationinput.QuestionnaireSnapshot) (*evaluationinput.MBTIPoleCatalog, error) {
	spec, err := typology.RuntimeSpecFromDefinition(def)
	if err != nil {
		return nil, err
	}
	if spec.Decision.Kind != modelcatalog.DecisionKindPoleComposition {
		return nil, fmt.Errorf("MBTI pole decision is required")
	}
	explorationQuestions := map[string]bool{}
	catalog := &evaluationinput.MBTIPoleCatalog{SchemaVersion: evaluationinput.MBTIPoleCatalogSchema}
	for _, code := range spec.FactorGraph.DecisionFactorOrder() {
		dim, ok := spec.FactorGraph.Dimensions[code]
		factor, found := spec.FactorGraph.Factors[code]
		if !ok || !found || factor.Kind != typology.FactorSpecKindLeaf || len(factor.Contributions) == 0 {
			return nil, fmt.Errorf("MBTI pole metadata is incomplete")
		}
		contributions := make([]classification.AnswerContribution, 0, len(factor.Contributions))
		for _, c := range factor.Contributions {
			if ref.Code == "MBTI_FC_93" {
				if explorationQuestions[c.QuestionCode] {
					return nil, fmt.Errorf("exploration question contributes to multiple axes")
				}
				explorationQuestions[c.QuestionCode] = true
			}
			contributions = append(contributions, classification.AnswerContribution{QuestionCode: c.QuestionCode, ScoringMode: classification.QuestionScoringMode(c.ScoringMode), Sign: c.Sign, Weight: c.Weight, OptionScores: c.OptionScores})
		}
		min, max := classification.PoleScoreRange(factor.Constant, contributions)
		if ref.Code == "MBTI_FC_93" {
			// The historical calculator's Likert fallback is not the 93-item
			// binary questionnaire range. Project actual frozen option bounds
			// without changing scoring or recomputing preference strength.
			min, max, err = explorationPoleBounds(questionnaire, factor.Constant, contributions)
			if err != nil {
				return nil, err
			}
		}
		threshold := dim.Threshold
		if threshold == 0 {
			threshold = 24
		} // Same historical boundary as the calculation layer.
		catalog.Axes = append(catalog.Axes, evaluationinput.MBTIPoleAxis{Code: code, Name: dim.Name, LeftPole: dim.LeftPole, RightPole: dim.RightPole, MinScore: min, MaxScore: max, Threshold: threshold})
	}
	if ref.Code == "MBTI_FC_93" && len(explorationQuestions) != 93 {
		return nil, fmt.Errorf("exploration question coverage mismatch")
	}
	return catalog, catalog.ValidateForModel(ref)
}

func explorationPoleBounds(q *evaluationinput.QuestionnaireSnapshot, constant float64, contributions []classification.AnswerContribution) (float64, float64, error) {
	if q == nil || q.Code != "MBTI_FC_93" || q.Version != "8.0.1" || len(q.Questions) != 93 {
		return 0, 0, fmt.Errorf("exploration frozen questionnaire is required")
	}
	questions := map[string]evaluationinput.QuestionSnapshot{}
	for _, question := range q.Questions {
		if _, duplicate := questions[question.Code]; duplicate {
			return 0, 0, fmt.Errorf("duplicate frozen question")
		}
		questions[question.Code] = question
	}
	low, high := constant, constant
	used := map[string]bool{}
	for _, c := range contributions {
		question, found := questions[c.QuestionCode]
		if !found || used[c.QuestionCode] || c.ScoringMode != classification.QuestionScoringModeQuestionScore || len(c.OptionScores) != 0 || len(question.Options) != 2 || c.Sign != 1 || c.Weight != 1 {
			return 0, 0, fmt.Errorf("invalid exploration scoring contribution")
		}
		used[c.QuestionCode] = true
		a, b := question.Options[0], question.Options[1]
		if a.Code == b.Code || a.Code == "" || b.Code == "" || math.IsNaN(a.Score) || math.IsNaN(b.Score) || math.IsInf(a.Score, 0) || math.IsInf(b.Score, 0) {
			return 0, 0, fmt.Errorf("invalid frozen option")
		}
		localMin, localMax := math.Min(a.Score, b.Score), math.Max(a.Score, b.Score)
		if localMin != 0 || localMax != 1 {
			return 0, 0, fmt.Errorf("exploration option boundary mismatch")
		}
		low += localMin
		high += localMax
	}
	return low, high, nil
}
