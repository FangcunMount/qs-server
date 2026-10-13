package definition

import (
	"encoding/json"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
)

// These private DTOs freeze definition-v2/v1's byte contract independently of
// Definition.MarshalJSON and the public/domain struct layout. Field order,
// null vs empty collections, source omission tags and zero derived markers
// are part of persisted hashes and execution identities. Future hash formats
// require a new version; do not extend these DTOs with new authoring fields.
type hashDefinitionV1 struct {
	Measure              hashMeasureV1
	Calibration          hashCalibrationV1
	Execution            hashExecutionV1
	Conclusions          []hashConclusionV1
	Outcomes             []hashOutcomeV1
	ReportMap            hashReportMapV1
	DecisionSpec         hashDecisionMarkerV1
	InterpretationAssets hashAssetsMarkerV1
}
type hashMeasureV1 struct {
	Factors     []hashFactorV1
	FactorGraph hashGraphV1
	Scoring     []hashScoringV1
}
type hashFactorV1 struct {
	Code  string
	Title string
	Role  string
}
type hashGraphV1 struct {
	Roots      []string
	Edges      []hashEdgeV1
	SortOrders map[string]int
}
type hashEdgeV1 struct {
	ParentCode string
	ChildCode  string
}
type hashScoringV1 struct {
	FactorCode string
	Sources    []hashSourceV1
	Strategy   string
	Params     *hashParamsV1
	MaxScore   *float64
	Weights    map[string]float64
	Constant   float64
}
type hashSourceV1 struct {
	Kind         string             `json:"Kind"`
	Code         string             `json:"Code"`
	ScoringMode  string             `json:"ScoringMode,omitempty"`
	Sign         float64            `json:"Sign,omitempty"`
	Weight       float64            `json:"Weight,omitempty"`
	OptionScores map[string]float64 `json:"OptionScores,omitempty"`
}
type hashParamsV1 struct{ CntOptionContents []string }
type hashCalibrationV1 struct{ NormRefs []hashNormRefV1 }
type hashNormRefV1 struct {
	FactorCode       string
	NormTableVersion string
}
type hashExecutionV1 struct {
	Brief2 *hashBrief2V1
	SPM    *hashSPMV1
}
type hashBrief2V1 struct {
	FormVariant         string
	PrimaryFactorCode   string
	IndexFactorCodes    []string
	ValidityFactorCodes []string
}
type hashSPMV1 struct {
	TimeLimitSeconds int
	TotalFactorCode  string
	ItemSets         []hashItemSetV1
}
type hashItemSetV1 struct {
	Code  string
	Items []hashItemV1
}
type hashItemV1 struct {
	QuestionCode      string
	CorrectOptionCode string
}
type hashConclusionV1 struct {
	Kind           string
	FactorCode     string
	FactorCodes    []string
	ScoreBasis     string
	Primary        bool
	Rules          []hashRangeV1
	Outcomes       []hashOutcomeV1
	Decision       hashTypeDecisionV1
	SpecialRules   []hashSpecialRuleV1
	OutcomeMapping hashOutcomeMappingV1
	Profiles       []hashProfileV1
}
type hashRangeV1 struct {
	MinScore     float64
	MaxScore     float64
	MaxInclusive bool
	UnboundedMax bool
	Level        string
	OutcomeCode  string
	Title        string
	Summary      string
	Description  string
}
type hashOutcomeV1 struct {
	Code        string
	Title       string
	Summary     string
	Description string
}
type hashTypeDecisionV1 struct {
	Kind                        string
	FallbackSimilarityThreshold float64
	FallbackCode                string
	LevelRule                   *hashLevelRuleV1
	Poles                       []hashPoleV1
	TopK                        int
}
type hashLevelRuleV1 struct {
	LowMax  float64
	HighMin float64
}
type hashPoleV1 struct {
	FactorCode string
	LeftPole   string
	RightPole  string
	Threshold  float64
	Model      string
}
type hashSpecialRuleV1 struct {
	Code          string
	Kind          string
	Phase         string
	Trigger       string
	OutcomeCode   string
	QuestionCodes []string
	OptionValues  []string
}
type hashOutcomeMappingV1 struct {
	DetailKind       string
	DetailAdapterKey string
	Algorithm        string
}
type hashProfileV1 struct {
	OutcomeCode string
	Pattern     string
	Traits      []string
	Strengths   []string
	Weaknesses  []string
	Suggestions []string
	ImageURL    string
	Image       string
	Rarity      hashRarityV1
	IsSpecial   bool
	Trigger     string
	Commentary  string
}
type hashRarityV1 struct {
	Percent float64
	Label   string
	OneInX  int
}
type hashReportMapV1 struct{ Sections []hashSectionV1 }
type hashSectionV1 struct {
	Code            string
	Title           string
	SourceRefs      []string
	Kind            string
	AdapterKey      string
	TemplateID      string
	TemplateVersion string
	CategoryLabel   string
}

// The former zero-valued derived structs were not omitted by encoding/json.
// Preserve only their fixed empty shape, without importing the derived types.
type hashDecisionMarkerV1 struct {
	ScoreRanges     []struct{}
	TypeDecision    *struct{}
	OutcomeRefs     []struct{}
	SpecialOutcomes []string
}
type hashAssetsMarkerV1 struct {
	Outcomes   []struct{}
	Profiles   []struct{}
	ReportSpec struct{ Sections []struct{} }
}

func canonicalContentBytesV1(def *Definition) ([]byte, error) {
	items, err := hashConclusionsV1(def.Conclusions)
	if err != nil {
		return nil, err
	}
	encoded := hashDefinitionV1{
		Measure: hashMeasureV1{
			Factors:     hashSliceV1(def.Measure.Factors, func(v factor.Factor) hashFactorV1 { return hashFactorV1{v.Code, v.Title, string(v.Role)} }),
			FactorGraph: hashGraphV1{def.Measure.FactorGraph.Roots, hashSliceV1(def.Measure.FactorGraph.Edges, func(v factor.FactorEdge) hashEdgeV1 { return hashEdgeV1{v.ParentCode, v.ChildCode} }), def.Measure.FactorGraph.SortOrders},
			Scoring:     hashSliceV1(def.Measure.Scoring, hashScoring),
		},
		Calibration: hashCalibrationV1{hashSliceV1(def.Calibration.NormRefs, func(v norm.Ref) hashNormRefV1 { return hashNormRefV1{v.FactorCode, v.NormTableVersion} })},
		Execution:   hashExecution(def.Execution), Conclusions: items,
		Outcomes: hashSliceV1(def.Outcomes, hashOutcome),
		ReportMap: hashReportMapV1{hashSliceV1(def.ReportMap.Sections, func(v ReportSection) hashSectionV1 {
			return hashSectionV1(v)
		})},
	}
	return json.Marshal(encoded)
}

func hashSliceV1[T, U any](items []T, project func(T) U) []U {
	if items == nil {
		return nil
	}
	out := make([]U, len(items))
	for i, v := range items {
		out[i] = project(v)
	}
	return out
}
func hashScoring(v factor.Scoring) hashScoringV1 {
	out := hashScoringV1{FactorCode: v.FactorCode, Strategy: string(v.Strategy), MaxScore: v.MaxScore, Weights: v.Weights, Constant: v.Constant,
		Sources: hashSliceV1(v.Sources, func(s factor.ScoringSource) hashSourceV1 {
			return hashSourceV1{string(s.Kind), s.Code, string(s.ScoringMode), s.Sign, s.Weight, s.OptionScores}
		}),
	}
	if v.Params != nil {
		out.Params = &hashParamsV1{v.Params.CntOptionContents}
	}
	return out
}
func hashExecution(v ExecutionSpec) hashExecutionV1 {
	var out hashExecutionV1
	if v.Brief2 != nil {
		b := v.Brief2
		out.Brief2 = &hashBrief2V1{b.FormVariant, b.PrimaryFactorCode, b.IndexFactorCodes, b.ValidityFactorCodes}
	}
	if v.SPM != nil {
		s := v.SPM
		out.SPM = &hashSPMV1{s.TimeLimitSeconds, s.TotalFactorCode, hashSliceV1(s.ItemSets, func(set SPMItemSet) hashItemSetV1 {
			return hashItemSetV1{set.Code, hashSliceV1(set.Items, func(item SPMItem) hashItemV1 { return hashItemV1(item) })}
		})}
	}
	return out
}
func hashOutcome(v conclusion.Outcome) hashOutcomeV1 {
	return hashOutcomeV1{v.Code, v.Title, v.Summary, v.Description}
}
func hashRange(v conclusion.ScoreRangeOutcome) hashRangeV1 {
	return hashRangeV1{v.MinScore, v.MaxScore, v.MaxInclusive, v.UnboundedMax, v.Level, v.OutcomeCode, v.Title, v.Summary, v.Description}
}
func hashConclusionsV1(items []conclusion.Conclusion) ([]hashConclusionV1, error) {
	if items == nil {
		return nil, nil
	}
	out := make([]hashConclusionV1, 0, len(items))
	for _, item := range items {
		var projected hashConclusionV1
		switch v := item.(type) {
		case conclusion.RiskConclusion:
			projected = hashConclusionV1{Kind: string(conclusion.KindRisk), FactorCode: v.FactorCode, Rules: hashSliceV1(v.Rules, hashRange), Outcomes: hashSliceV1(v.Outcomes, hashOutcome)}
		case conclusion.NormConclusion:
			projected = hashConclusionV1{Kind: string(conclusion.KindNorm), FactorCode: v.FactorCode, ScoreBasis: string(v.ScoreBasis), Primary: v.Primary, Rules: hashSliceV1(v.Rules, hashRange), Outcomes: hashSliceV1(v.Outcomes, hashOutcome)}
		case conclusion.AbilityConclusion:
			projected = hashConclusionV1{Kind: string(conclusion.KindAbility), FactorCode: v.FactorCode, ScoreBasis: string(v.ScoreBasis), Primary: v.Primary, Rules: hashSliceV1(v.Rules, hashRange), Outcomes: hashSliceV1(v.Outcomes, hashOutcome)}
		case conclusion.TypeConclusion:
			projected = hashConclusionV1{Kind: string(conclusion.KindType), FactorCodes: v.FactorCodes, Decision: hashDecision(v.Decision),
				SpecialRules: hashSliceV1(v.SpecialRules, func(r conclusion.TypeSpecialRule) hashSpecialRuleV1 {
					return hashSpecialRuleV1{r.Code, string(r.Kind), string(r.Phase), r.Trigger, r.OutcomeCode, r.QuestionCodes, r.OptionValues}
				}),
				OutcomeMapping: hashOutcomeMappingV1{v.OutcomeMapping.DetailKind, v.OutcomeMapping.DetailAdapterKey, string(v.OutcomeMapping.Algorithm)}, Profiles: hashSliceV1(v.Profiles, hashProfile), Outcomes: hashSliceV1(v.Outcomes, hashOutcome),
			}
		default:
			return nil, fmt.Errorf("unsupported conclusion type %T", item)
		}
		out = append(out, projected)
	}
	return out, nil
}
func hashDecision(v conclusion.TypeDecision) hashTypeDecisionV1 {
	out := hashTypeDecisionV1{Kind: string(v.Kind), FallbackSimilarityThreshold: v.FallbackSimilarityThreshold, FallbackCode: v.FallbackCode, TopK: v.TopK, Poles: hashSliceV1(v.Poles, func(p conclusion.TypePole) hashPoleV1 {
		return hashPoleV1{p.FactorCode, p.LeftPole, p.RightPole, p.Threshold, p.Model}
	})}
	if v.LevelRule != nil {
		out.LevelRule = &hashLevelRuleV1{v.LevelRule.LowMax, v.LevelRule.HighMin}
	}
	return out
}
func hashProfile(v conclusion.TypeOutcomeProfile) hashProfileV1 {
	return hashProfileV1{v.OutcomeCode, v.Pattern, v.Traits, v.Strengths, v.Weaknesses, v.Suggestions, v.ImageURL, v.Image, hashRarityV1{v.Rarity.Percent, v.Rarity.Label, v.Rarity.OneInX}, v.IsSpecial, v.Trigger, v.Commentary}
}
