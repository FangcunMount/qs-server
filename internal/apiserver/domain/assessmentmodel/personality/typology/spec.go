package typology

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"

// RuntimeSpec is the configuration-driven execution view of a typology payload.
type RuntimeSpec struct {
	FactorGraph    FactorGraphSpec         `json:"factor_graph"`
	Decision       PersonalityDecisionSpec `json:"decision"`
	SpecialRules   []SpecialRuleSpec       `json:"special_rules,omitempty"`
	OutcomeMapping OutcomeMappingSpec      `json:"outcome_mapping"`
	Report         ReportSpec              `json:"report"`
}

// FactorGraphSpec describes factor dimensions and question mappings for scoring.
// When Factors and Roots are set, the explicit factor graph takes precedence over
// the legacy DimensionOrder/Dimensions/QuestionMappings flat layout.
type FactorGraphSpec struct {
	DimensionOrder   []string              `json:"dimension_order,omitempty"`
	Dimensions       map[string]Dimension  `json:"dimensions,omitempty"`
	QuestionMappings []QuestionMapping     `json:"question_mappings,omitempty"`
	Factors          map[string]FactorSpec `json:"factors,omitempty"`
	Roots            []string              `json:"roots,omitempty"`
}

// FactorSpec defines one node in an explicit factor graph.
type FactorSpec struct {
	ID            string                   `json:"id"`
	Code          string                   `json:"code,omitempty"`
	Name          string                   `json:"name,omitempty"`
	Kind          FactorSpecKind           `json:"kind"`
	Children      []string                 `json:"children,omitempty"`
	Aggregation   FactorAggregation        `json:"aggregation,omitempty"`
	Weights       map[string]float64       `json:"weights,omitempty"`
	Constant      float64                  `json:"constant,omitempty"`
	Contributions []FactorContributionSpec `json:"contributions,omitempty"`
	OptionScoring FactorOptionScoring      `json:"option_scoring,omitempty"`
}

// FactorSpecKind distinguishes leaf and composite factor nodes.
type FactorSpecKind string

const (
	FactorSpecKindLeaf      FactorSpecKind = "leaf"
	FactorSpecKindComposite FactorSpecKind = "composite"
)

// FactorAggregation defines how composite factors combine child scores.
type FactorAggregation string

const (
	FactorAggregationSum         FactorAggregation = "sum"
	FactorAggregationAvg         FactorAggregation = "avg"
	FactorAggregationWeightedAvg FactorAggregation = "weighted_avg"
)

// FactorOptionScoring controls option-mapped answer scoring for leaf factors.
type FactorOptionScoring string

const (
	FactorOptionScoringStrict FactorOptionScoring = "strict"
	FactorOptionScoringCompat FactorOptionScoring = "compat"
)

// FactorContributionSpec maps a questionnaire item to a leaf factor score.
type FactorContributionSpec struct {
	QuestionCode string             `json:"question_code"`
	Sign         float64            `json:"sign,omitempty"`
	OptionScores map[string]float64 `json:"option_scores,omitempty"`
}

// HasExplicitFactorGraph reports whether the spec carries an explicit factor hierarchy.
func (fg FactorGraphSpec) HasExplicitFactorGraph() bool {
	return len(fg.Factors) > 0 && len(fg.Roots) > 0
}

// DecisionFactorOrder returns the factor ids used by decision and outcome assembly.
func (fg FactorGraphSpec) DecisionFactorOrder() []string {
	if fg.HasExplicitFactorGraph() {
		return append([]string(nil), fg.Roots...)
	}
	return append([]string(nil), fg.DimensionOrder...)
}

// PersonalityDecisionSpec describes how profile vectors become outcomes.
type PersonalityDecisionSpec struct {
	Kind                        assessmentmodel.DecisionKind `json:"kind"`
	FallbackSimilarityThreshold float64                      `json:"fallback_similarity_threshold,omitempty"`
	FallbackCode                string                       `json:"fallback_code,omitempty"`
	LevelRule                   *LevelRuleSpec               `json:"level_rule,omitempty"`
}

// LevelRuleSpec maps raw factor scores to discrete levels for pattern matching.
type LevelRuleSpec struct {
	LowMax  float64 `json:"low_max,omitempty"`
	HighMin float64 `json:"high_min,omitempty"`
}

// SpecialRulePhase selects when a special rule is evaluated.
type SpecialRulePhase string

const (
	SpecialRuleBeforeScore    SpecialRulePhase = "before_score"
	SpecialRuleBeforeDecision SpecialRulePhase = "before_decision"
	SpecialRuleAfterDecision  SpecialRulePhase = "after_decision"
)

// SpecialRuleSpec describes a configurable special outcome rule.
type SpecialRuleSpec struct {
	Code        string               `json:"code"`
	Kind        SpecialRuleKind      `json:"kind,omitempty"`
	Phase       SpecialRulePhase     `json:"phase"`
	Trigger     string               `json:"trigger,omitempty"`
	OutcomeCode string               `json:"outcome_code,omitempty"`
	Condition   SpecialRuleCondition   `json:"condition,omitempty"`
	QuestionCodes []string             `json:"question_codes,omitempty"`
	OptionValues  []string             `json:"option_values,omitempty"`
}

// SpecialRuleKind selects the evaluation strategy for a special rule.
type SpecialRuleKind string

const (
	SpecialRuleKindAnswerMatch       SpecialRuleKind = "answer_match"
	SpecialRuleKindFallbackThreshold SpecialRuleKind = "fallback_threshold"
)

// SpecialRuleCondition carries kind-specific match parameters.
type SpecialRuleCondition struct {
	QuestionCodes []string `json:"question_codes,omitempty"`
	OptionValues  []string `json:"option_values,omitempty"`
}

// ResolvedKind returns the configured kind, deriving from legacy fields when needed.
func (r SpecialRuleSpec) ResolvedKind() SpecialRuleKind {
	if r.Kind != "" {
		return r.Kind
	}
	if len(r.ResolvedQuestionCodes()) > 0 {
		return SpecialRuleKindAnswerMatch
	}
	if r.Phase == SpecialRuleAfterDecision {
		return SpecialRuleKindFallbackThreshold
	}
	return ""
}

// ResolvedQuestionCodes returns question codes from condition or legacy flat fields.
func (r SpecialRuleSpec) ResolvedQuestionCodes() []string {
	if len(r.Condition.QuestionCodes) > 0 {
		return append([]string(nil), r.Condition.QuestionCodes...)
	}
	return append([]string(nil), r.QuestionCodes...)
}

// ResolvedOptionValues returns option values from condition or legacy flat fields.
func (r SpecialRuleSpec) ResolvedOptionValues() []string {
	if len(r.Condition.OptionValues) > 0 {
		return append([]string(nil), r.Condition.OptionValues...)
	}
	return append([]string(nil), r.OptionValues...)
}

// OutcomeDetailKind selects how scoring detail maps to AssessmentOutcome.
type OutcomeDetailKind string

const (
	OutcomeDetailPersonalityType OutcomeDetailKind = "personality_type"
	OutcomeDetailTraitProfile    OutcomeDetailKind = "trait_profile"
)

// OutcomeMappingSpec describes how scoring detail becomes AssessmentOutcome fields.
type OutcomeMappingSpec struct {
	DetailKind       OutcomeDetailKind         `json:"detail_kind"`
	DetailAdapterKey DetailAdapterKey          `json:"detail_adapter_key,omitempty"`
	Algorithm        assessmentmodel.Algorithm `json:"algorithm,omitempty"`
}

// DetailAdapterKey selects the detail assembler implementation.
type DetailAdapterKey string

const (
	DetailAdapterMBTI    DetailAdapterKey = "mbti"
	DetailAdapterSBTI    DetailAdapterKey = "sbti"
	DetailAdapterBigFive DetailAdapterKey = "bigfive"
)

// ResolvedDetailAdapterKey returns the configured adapter key, deriving from legacy fields when needed.
func (m OutcomeMappingSpec) ResolvedDetailAdapterKey(decisionKind assessmentmodel.DecisionKind) DetailAdapterKey {
	if m.DetailAdapterKey != "" {
		return m.DetailAdapterKey
	}
	if m.DetailKind == OutcomeDetailTraitProfile {
		return DetailAdapterBigFive
	}
	if decisionKind == assessmentmodel.DecisionKindNearestPattern {
		return DetailAdapterSBTI
	}
	return DetailAdapterMBTI
}

// ReportKind selects the report adapter strategy.
type ReportKind string

const (
	ReportKindPersonalityType ReportKind = "personality_type"
	ReportKindTraitProfile    ReportKind = "trait_profile"
	ReportKindTemplate        ReportKind = "template"
)

// ReportSpec describes how to build interpret reports for a typology model.
type ReportSpec struct {
	Kind          ReportKind       `json:"kind"`
	AdapterKey    ReportAdapterKey `json:"adapter_key,omitempty"`
	TemplateID    string           `json:"template_id,omitempty"`
	CategoryLabel string           `json:"category_label,omitempty"`
}

// ReportAdapterKey selects the report builder implementation.
type ReportAdapterKey string

const (
	ReportAdapterMBTI    ReportAdapterKey = "mbti"
	ReportAdapterSBTI    ReportAdapterKey = "sbti"
	ReportAdapterBigFive ReportAdapterKey = "bigfive"
)

// ResolvedAdapterKey returns the configured report adapter, deriving from kind and outcome mapping when needed.
func (r ReportSpec) ResolvedAdapterKey(mapping OutcomeMappingSpec, decisionKind assessmentmodel.DecisionKind) ReportAdapterKey {
	if r.AdapterKey != "" {
		return r.AdapterKey
	}
	if r.Kind == ReportKindTraitProfile {
		return ReportAdapterBigFive
	}
	switch mapping.ResolvedDetailAdapterKey(decisionKind) {
	case DetailAdapterSBTI:
		return ReportAdapterSBTI
	case DetailAdapterBigFive:
		return ReportAdapterBigFive
	default:
		return ReportAdapterMBTI
	}
}
