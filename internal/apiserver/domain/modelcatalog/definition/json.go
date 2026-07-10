package definition

import (
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
)

// MarshalJSON makes the polymorphic conclusion layer explicit at transport
// boundaries. The domain stores concrete conclusion values behind an interface;
// canonical DefinitionV2 JSON carries their kind so it can be reconstructed.
func (d Definition) MarshalJSON() ([]byte, error) {
	items, err := marshalConclusions(d.Conclusions)
	if err != nil {
		return nil, err
	}
	return json.Marshal(definitionJSON{
		Measure:     d.Measure,
		Calibration: d.Calibration,
		Conclusions: items,
		Outcomes:    d.Outcomes,
		ReportMap:   d.ReportMap,
	})
}

// UnmarshalJSON reconstructs canonical DefinitionV2 from its tagged
// conclusion representation. Untagged conclusion objects are rejected rather
// than guessed, because norm and ability conclusions have overlapping fields.
func (d *Definition) UnmarshalJSON(data []byte) error {
	var raw definitionJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	items, err := unmarshalConclusions(raw.Conclusions)
	if err != nil {
		return err
	}
	*d = Definition{
		Measure:     raw.Measure,
		Calibration: raw.Calibration,
		Conclusions: items,
		Outcomes:    raw.Outcomes,
		ReportMap:   raw.ReportMap,
	}
	return nil
}

type definitionJSON struct {
	Measure     MeasureSpec          `json:"Measure"`
	Calibration Calibration          `json:"Calibration"`
	Conclusions []conclusionJSON     `json:"Conclusions"`
	Outcomes    []conclusion.Outcome `json:"Outcomes"`
	ReportMap   ReportMap            `json:"ReportMap"`
}

type conclusionJSON struct {
	Kind           conclusion.Kind                 `json:"Kind"`
	FactorCode     string                          `json:"FactorCode"`
	FactorCodes    []string                        `json:"FactorCodes"`
	ScoreBasis     conclusion.ScoreBasis           `json:"ScoreBasis"`
	Primary        bool                            `json:"Primary"`
	Rules          []conclusion.ScoreRangeOutcome  `json:"Rules"`
	Outcomes       []conclusion.Outcome            `json:"Outcomes"`
	Decision       conclusion.TypeDecision         `json:"Decision"`
	SpecialRules   []conclusion.TypeSpecialRule    `json:"SpecialRules"`
	OutcomeMapping conclusion.TypeOutcomeMapping   `json:"OutcomeMapping"`
	Profiles       []conclusion.TypeOutcomeProfile `json:"Profiles"`
}

func marshalConclusions(items []conclusion.Conclusion) ([]conclusionJSON, error) {
	if items == nil {
		return nil, nil
	}
	out := make([]conclusionJSON, 0, len(items))
	for _, item := range items {
		switch value := item.(type) {
		case conclusion.RiskConclusion:
			out = append(out, conclusionJSON{Kind: conclusion.KindRisk, FactorCode: value.FactorCode, Rules: value.Rules, Outcomes: value.Outcomes})
		case conclusion.NormConclusion:
			out = append(out, conclusionJSON{Kind: conclusion.KindNorm, FactorCode: value.FactorCode, ScoreBasis: value.ScoreBasis, Primary: value.Primary, Rules: value.Rules, Outcomes: value.Outcomes})
		case conclusion.AbilityConclusion:
			out = append(out, conclusionJSON{Kind: conclusion.KindAbility, FactorCode: value.FactorCode, ScoreBasis: value.ScoreBasis, Rules: value.Rules, Outcomes: value.Outcomes})
		case conclusion.TypeConclusion:
			out = append(out, conclusionJSON{Kind: conclusion.KindType, FactorCodes: value.FactorCodes, Decision: value.Decision, SpecialRules: value.SpecialRules, OutcomeMapping: value.OutcomeMapping, Profiles: value.Profiles, Outcomes: value.Outcomes})
		default:
			return nil, fmt.Errorf("unsupported conclusion type %T", item)
		}
	}
	return out, nil
}

func unmarshalConclusions(items []conclusionJSON) ([]conclusion.Conclusion, error) {
	if items == nil {
		return nil, nil
	}
	out := make([]conclusion.Conclusion, 0, len(items))
	for index, item := range items {
		switch item.Kind {
		case conclusion.KindRisk:
			out = append(out, conclusion.RiskConclusion{FactorCode: item.FactorCode, Rules: item.Rules, Outcomes: item.Outcomes})
		case conclusion.KindNorm:
			out = append(out, conclusion.NormConclusion{FactorCode: item.FactorCode, ScoreBasis: item.ScoreBasis, Primary: item.Primary, Rules: item.Rules, Outcomes: item.Outcomes})
		case conclusion.KindAbility:
			out = append(out, conclusion.AbilityConclusion{FactorCode: item.FactorCode, ScoreBasis: item.ScoreBasis, Rules: item.Rules, Outcomes: item.Outcomes})
		case conclusion.KindType:
			out = append(out, conclusion.TypeConclusion{FactorCodes: item.FactorCodes, Decision: item.Decision, SpecialRules: item.SpecialRules, OutcomeMapping: item.OutcomeMapping, Profiles: item.Profiles, Outcomes: item.Outcomes})
		default:
			return nil, fmt.Errorf("conclusions[%d].Kind %q is required and must be supported", index, item.Kind)
		}
	}
	return out, nil
}
