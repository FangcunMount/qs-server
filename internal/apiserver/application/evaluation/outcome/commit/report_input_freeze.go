package commit

import (
	"fmt"

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
	opts.MBTIPoles, err = freezeMBTIPoles(def)
	return opts, err
}

func freezeMBTIPoles(def *definition.Definition) (*evaluationinput.MBTIPoleCatalog, error) {
	spec, err := typology.RuntimeSpecFromDefinition(def)
	if err != nil {
		return nil, err
	}
	if spec.Decision.Kind != modelcatalog.DecisionKindPoleComposition {
		return nil, fmt.Errorf("MBTI pole decision is required")
	}
	catalog := &evaluationinput.MBTIPoleCatalog{SchemaVersion: evaluationinput.MBTIPoleCatalogSchema}
	for _, code := range spec.FactorGraph.DecisionFactorOrder() {
		dim, ok := spec.FactorGraph.Dimensions[code]
		factor, found := spec.FactorGraph.Factors[code]
		if !ok || !found || factor.Kind != typology.FactorSpecKindLeaf || len(factor.Contributions) == 0 {
			return nil, fmt.Errorf("MBTI pole metadata is incomplete")
		}
		contributions := make([]classification.AnswerContribution, 0, len(factor.Contributions))
		for _, c := range factor.Contributions {
			contributions = append(contributions, classification.AnswerContribution{QuestionCode: c.QuestionCode, ScoringMode: classification.QuestionScoringMode(c.ScoringMode), Sign: c.Sign, Weight: c.Weight, OptionScores: c.OptionScores})
		}
		min, max := classification.PoleScoreRange(factor.Constant, contributions)
		threshold := dim.Threshold
		if threshold == 0 {
			threshold = 24
		} // Same historical boundary as the calculation layer.
		catalog.Axes = append(catalog.Axes, evaluationinput.MBTIPoleAxis{Code: code, Name: dim.Name, LeftPole: dim.LeftPole, RightPole: dim.RightPole, MinScore: min, MaxScore: max, Threshold: threshold})
	}
	return catalog, catalog.Validate()
}
