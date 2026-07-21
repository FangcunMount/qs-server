package typology

import (
	"encoding/json"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
)

// RuntimeSpec 是配置-driven execution 视图 of 类型学载荷。
type RuntimeSpec struct {
	FactorGraph    FactorGraphSpec         `json:"factor_graph"`
	Decision       PersonalityDecisionSpec `json:"decision"`
	SpecialRules   []SpecialRuleSpec       `json:"special_rules,omitempty"`
	OutcomeMapping OutcomeMappingSpec      `json:"outcome_mapping"`
	Report         ReportSpec              `json:"report"`
}

// FactorGraphSpec is the explicit factor graph used for scoring.
type FactorGraphSpec struct {
	Dimensions map[string]Dimension  `json:"dimensions,omitempty"`
	Factors    map[string]FactorSpec `json:"factors"`
	Roots      []string              `json:"roots"`
}

// FactorSpec 定义一个node in 显式 因子图。
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
}

// FactorSpecKind 区分叶子 和 复合 因子 nodes。
type FactorSpecKind string

const (
	FactorSpecKindLeaf      FactorSpecKind = "leaf"
	FactorSpecKindComposite FactorSpecKind = "composite"
)

// FactorAggregation 定义如何复合 因子 组合子节点 分数。
type FactorAggregation string

const (
	FactorAggregationSum         FactorAggregation = "sum"
	FactorAggregationAvg         FactorAggregation = "avg"
	FactorAggregationWeightedAvg FactorAggregation = "weighted_avg"
)

// FactorContributionSpec 映射问卷题目 到 叶子 因子 score。
type FactorContributionSpec struct {
	QuestionCode string              `json:"question_code"`
	ScoringMode  QuestionScoringMode `json:"scoring_mode,omitempty"`
	Sign         float64             `json:"sign,omitempty"`
	Weight       float64             `json:"weight,omitempty"`
	OptionScores map[string]float64  `json:"option_scores,omitempty"`
}

// QuestionScoringMode selects the source of a question contribution's base score.
type QuestionScoringMode string

const (
	QuestionScoringModeQuestionScore  QuestionScoringMode = "question_score"
	QuestionScoringModeOptionOverride QuestionScoringMode = "option_override"
)

// UnmarshalJSON applies defaults only when explicit-mode fields are omitted.
func (c *FactorContributionSpec) UnmarshalJSON(data []byte) error {
	type alias FactorContributionSpec
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if decoded.ScoringMode != "" {
		if _, ok := fields["sign"]; !ok {
			decoded.Sign = 1
		}
		if _, ok := fields["weight"]; !ok {
			decoded.Weight = 1
		}
	}
	*c = FactorContributionSpec(decoded)
	return nil
}

// HasExplicitFactorGraph 报告是否 spec 携带 显式 因子 层级。
func (fg FactorGraphSpec) HasExplicitFactorGraph() bool {
	return len(fg.Factors) > 0 && len(fg.Roots) > 0
}

// DecisionFactorOrder 返回因子 ids 供 decision 和 结果 assembly。
func (fg FactorGraphSpec) DecisionFactorOrder() []string {
	return append([]string(nil), fg.Roots...)
}

// PersonalityDecisionSpec 描述如何画像 向量转成 结果。
type PersonalityDecisionSpec struct {
	Kind                        binding.DecisionKind `json:"kind"`
	FallbackSimilarityThreshold float64              `json:"fallback_similarity_threshold,omitempty"`
	FallbackCode                string               `json:"fallback_code,omitempty"`
	LevelRule                   *LevelRuleSpec       `json:"level_rule,omitempty"`
	TopK                        int                  `json:"top_k,omitempty"`
}

// LevelRuleSpec 映射原始 因子 分数 到 离散等级 用于 模式匹配。
type LevelRuleSpec struct {
	LowMax  float64 `json:"low_max,omitempty"`
	HighMin float64 `json:"high_min,omitempty"`
}

// SpecialRulePhase 选择when special rule 是 evaluated。
type SpecialRulePhase string

const (
	SpecialRuleBeforeScore    SpecialRulePhase = "before_score"
	SpecialRuleBeforeDecision SpecialRulePhase = "before_decision"
	SpecialRuleAfterDecision  SpecialRulePhase = "after_decision"
)

// SpecialRuleSpec 描述配置urable special 结果 rule。
type SpecialRuleSpec struct {
	Code        string               `json:"code"`
	Kind        SpecialRuleKind      `json:"kind,omitempty"`
	Phase       SpecialRulePhase     `json:"phase"`
	Trigger     string               `json:"trigger,omitempty"`
	OutcomeCode string               `json:"outcome_code,omitempty"`
	Condition   SpecialRuleCondition `json:"condition,omitempty"`
}

// SpecialRuleKind 选择评估 strategy 用于 special rule。
type SpecialRuleKind string

const (
	SpecialRuleKindAnswerMatch       SpecialRuleKind = "answer_match"
	SpecialRuleKindFallbackThreshold SpecialRuleKind = "fallback_threshold"
)

// SpecialRuleCondition 携带类型-特定 match parameters。
type SpecialRuleCondition struct {
	QuestionCodes []string `json:"question_codes,omitempty"`
	OptionValues  []string `json:"option_values,omitempty"`
}

// ResolvedKind returns the explicitly declared rule kind.
func (r SpecialRuleSpec) ResolvedKind() SpecialRuleKind {
	return r.Kind
}

// ResolvedQuestionCodes returns condition question codes.
func (r SpecialRuleSpec) ResolvedQuestionCodes() []string {
	return append([]string(nil), r.Condition.QuestionCodes...)
}

// ResolvedOptionValues returns condition option values.
func (r SpecialRuleSpec) ResolvedOptionValues() []string {
	return append([]string(nil), r.Condition.OptionValues...)
}

// OutcomeDetailKind selects how scoring detail maps to canonical Execution.
type OutcomeDetailKind string

const (
	OutcomeDetailPersonalityType OutcomeDetailKind = "personality_type"
	OutcomeDetailTraitProfile    OutcomeDetailKind = "trait_profile"
)

// OutcomeMappingSpec describes how scoring detail maps to Execution fields.
type OutcomeMappingSpec struct {
	DetailKind       OutcomeDetailKind `json:"detail_kind"`
	DetailAdapterKey DetailAdapterKey  `json:"detail_adapter_key,omitempty"`
	Algorithm        binding.Algorithm `json:"algorithm,omitempty"`
}

// DetailAdapterKey 选择明细组装器 实现。
type DetailAdapterKey string

const (
	DetailAdapterPersonalityType DetailAdapterKey = "personality_type"
	DetailAdapterTraitProfile    DetailAdapterKey = "trait_profile"
)

// ResolvedDetailAdapterKey resolves the explicit mechanism adapter.
func (m OutcomeMappingSpec) ResolvedDetailAdapterKey(decisionKind binding.DecisionKind) DetailAdapterKey {
	if m.DetailAdapterKey != "" {
		return m.DetailAdapterKey
	}
	if m.DetailKind == OutcomeDetailTraitProfile {
		return DetailAdapterTraitProfile
	}
	return DetailAdapterPersonalityType
}

// ReportKind 选择报告适配器 strategy。
type ReportKind string

const (
	ReportKindPersonalityType ReportKind = "personality_type"
	ReportKindTraitProfile    ReportKind = "trait_profile"
	ReportKindTemplate        ReportKind = "template"
)

// ReportSpec 描述如何 build interpret reports 用于 类型学 model。
type ReportSpec struct {
	Kind            ReportKind       `json:"kind"`
	AdapterKey      ReportAdapterKey `json:"adapter_key,omitempty"`
	TemplateID      string           `json:"template_id,omitempty"`
	TemplateVersion string           `json:"template_version,omitempty"`
	CategoryLabel   string           `json:"category_label,omitempty"`
}

// ReportAdapterKey 选择报告构建器 实现。
type ReportAdapterKey string

const (
	ReportAdapterPersonalityType ReportAdapterKey = "personality_type"
	ReportAdapterTraitProfile    ReportAdapterKey = "trait_profile"
)

// ResolvedAdapterKey resolves an explicit or report-kind-derived mechanism adapter.
func (r ReportSpec) ResolvedAdapterKey(_ OutcomeMappingSpec, _ binding.DecisionKind) ReportAdapterKey {
	if r.AdapterKey != "" {
		return r.AdapterKey
	}
	switch r.Kind {
	case ReportKindTemplate:
		return ""
	case ReportKindTraitProfile:
		return ReportAdapterTraitProfile
	default:
		return ReportAdapterPersonalityType
	}
}
