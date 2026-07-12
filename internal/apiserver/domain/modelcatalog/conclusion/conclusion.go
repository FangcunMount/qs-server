package conclusion

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"

// Conclusion 是测量结果到解释、诊断、类型或能力等级的配置抽象。
type Conclusion interface {
	conclusionKind() Kind
}

type Kind string

const (
	KindRisk    Kind = "risk"
	KindType    Kind = "type"
	KindNorm    Kind = "norm"
	KindAbility Kind = "ability"
)

type Outcome struct {
	Code        string
	Title       string
	Summary     string
	Description string
}

type RiskConclusion struct {
	FactorCode string
	Rules      []ScoreRangeOutcome
	Outcomes   []Outcome
}

func (RiskConclusion) conclusionKind() Kind { return KindRisk }

type ScoreRangeOutcome struct {
	MinScore    float64
	MaxScore    float64
	Level       string
	OutcomeCode string
	Title       string
	Summary     string
	Description string
}

type TypeConclusion struct {
	FactorCodes    []string
	Decision       TypeDecision
	SpecialRules   []TypeSpecialRule
	OutcomeMapping TypeOutcomeMapping
	Profiles       []TypeOutcomeProfile
	Outcomes       []Outcome
}

func (TypeConclusion) conclusionKind() Kind { return KindType }

type NormConclusion struct {
	FactorCode string
	ScoreBasis ScoreBasis
	Primary    bool
	Rules      []ScoreRangeOutcome
	Outcomes   []Outcome
}

func (NormConclusion) conclusionKind() Kind { return KindNorm }

type AbilityConclusion struct {
	FactorCode string
	ScoreBasis ScoreBasis
	Rules      []ScoreRangeOutcome
	Outcomes   []Outcome
}

func (AbilityConclusion) conclusionKind() Kind { return KindAbility }

type ScoreBasis string

const (
	ScoreBasisRaw           ScoreBasis = "raw_score"
	ScoreBasisTScore        ScoreBasis = "t_score"
	ScoreBasisPercentile    ScoreBasis = "percentile"
	ScoreBasisStandardScore ScoreBasis = "standard_score"
)

type TypeDecision struct {
	Kind                        binding.DecisionKind
	FallbackSimilarityThreshold float64
	FallbackCode                string
	LevelRule                   *TypeLevelRule
	Poles                       []TypePole
}

type TypeLevelRule struct {
	LowMax  float64
	HighMin float64
}

type TypePole struct {
	FactorCode string
	LeftPole   string
	RightPole  string
	Threshold  float64
	Model      string
}

type TypeSpecialRule struct {
	Code          string
	Kind          TypeSpecialRuleKind
	Phase         TypeSpecialRulePhase
	Trigger       string
	OutcomeCode   string
	QuestionCodes []string
	OptionValues  []string
}

type TypeSpecialRuleKind string

const (
	TypeSpecialRuleAnswerMatch       TypeSpecialRuleKind = "answer_match"
	TypeSpecialRuleFallbackThreshold TypeSpecialRuleKind = "fallback_threshold"
)

type TypeSpecialRulePhase string

const (
	TypeSpecialRuleBeforeScore    TypeSpecialRulePhase = "before_score"
	TypeSpecialRuleBeforeDecision TypeSpecialRulePhase = "before_decision"
	TypeSpecialRuleAfterDecision  TypeSpecialRulePhase = "after_decision"
)

type TypeOutcomeMapping struct {
	DetailKind       string
	DetailAdapterKey string
	Algorithm        binding.Algorithm
}

type TypeOutcomeProfile struct {
	OutcomeCode string
	Pattern     string
	Traits      []string
	Strengths   []string
	Weaknesses  []string
	Suggestions []string
	ImageURL    string
	Image       string
	Rarity      Rarity
	IsSpecial   bool
	Trigger     string
	Commentary  string
}

type Rarity struct {
	Percent float64
	Label   string
	OneInX  int
}
