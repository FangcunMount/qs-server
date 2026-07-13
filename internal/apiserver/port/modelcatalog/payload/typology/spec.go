package typology

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"

// RuntimeSpec 是配置-driven execution 视图 of 类型学载荷。
type RuntimeSpec struct {
	FactorGraph    FactorGraphSpec         `json:"factor_graph"`
	Decision       PersonalityDecisionSpec `json:"decision"`
	SpecialRules   []SpecialRuleSpec       `json:"special_rules,omitempty"`
	OutcomeMapping OutcomeMappingSpec      `json:"outcome_mapping"`
	Report         ReportSpec              `json:"report"`
}

// FactorGraphSpec 描述因子 维度 和 question mappings 用于 计分。
// When 因子 和 根s 是 set, 显式 因子图 takes precedence over。
// 旧版 维度Order/维度/QuestionMappings flat layout。
type FactorGraphSpec struct {
	DimensionOrder   []string              `json:"dimension_order,omitempty"`
	Dimensions       map[string]Dimension  `json:"dimensions,omitempty"`
	QuestionMappings []QuestionMapping     `json:"question_mappings,omitempty"`
	Factors          map[string]FactorSpec `json:"factors,omitempty"`
	Roots            []string              `json:"roots,omitempty"`
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
	OptionScoring FactorOptionScoring      `json:"option_scoring,omitempty"`
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

// FactorOptionScoring 控制选项-mapped answer 计分 用于 叶子 因子。
type FactorOptionScoring string

const (
	FactorOptionScoringStrict FactorOptionScoring = "strict"
	FactorOptionScoringCompat FactorOptionScoring = "compat"
)

// FactorContributionSpec 映射问卷题目 到 叶子 因子 score。
type FactorContributionSpec struct {
	QuestionCode string             `json:"question_code"`
	Sign         float64            `json:"sign,omitempty"`
	OptionScores map[string]float64 `json:"option_scores,omitempty"`
}

// HasExplicitFactorGraph 报告是否 spec 携带 显式 因子 层级。
func (fg FactorGraphSpec) HasExplicitFactorGraph() bool {
	return len(fg.Factors) > 0 && len(fg.Roots) > 0
}

// DecisionFactorOrder 返回因子 ids 供 decision 和 结果 assembly。
func (fg FactorGraphSpec) DecisionFactorOrder() []string {
	if fg.HasExplicitFactorGraph() {
		return append([]string(nil), fg.Roots...)
	}
	return append([]string(nil), fg.DimensionOrder...)
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
	Code          string               `json:"code"`
	Kind          SpecialRuleKind      `json:"kind,omitempty"`
	Phase         SpecialRulePhase     `json:"phase"`
	Trigger       string               `json:"trigger,omitempty"`
	OutcomeCode   string               `json:"outcome_code,omitempty"`
	Condition     SpecialRuleCondition `json:"condition,omitempty"`
	QuestionCodes []string             `json:"question_codes,omitempty"`
	OptionValues  []string             `json:"option_values,omitempty"`
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

// ResolvedKind 返回配置化 类型, deriving 从 旧版 字段 when needed。
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

// ResolvedQuestionCodes 返回question 编码 从 condition 或 旧版 flat 字段。
func (r SpecialRuleSpec) ResolvedQuestionCodes() []string {
	if len(r.Condition.QuestionCodes) > 0 {
		return append([]string(nil), r.Condition.QuestionCodes...)
	}
	return append([]string(nil), r.QuestionCodes...)
}

// ResolvedOptionValues 返回选项 values 从 condition 或 旧版 flat 字段。
func (r SpecialRuleSpec) ResolvedOptionValues() []string {
	if len(r.Condition.OptionValues) > 0 {
		return append([]string(nil), r.Condition.OptionValues...)
	}
	return append([]string(nil), r.OptionValues...)
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
	DetailAdapterMBTI            DetailAdapterKey = "mbti"
	DetailAdapterSBTI            DetailAdapterKey = "sbti"
	DetailAdapterBigFive         DetailAdapterKey = "bigfive"
)

// ResolvedDetailAdapterKey 返回配置化 adapter 键, deriving 从 旧版 字段 when needed。
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
	Kind          ReportKind       `json:"kind"`
	AdapterKey    ReportAdapterKey `json:"adapter_key,omitempty"`
	TemplateID    string           `json:"template_id,omitempty"`
	CategoryLabel string           `json:"category_label,omitempty"`
}

// ReportAdapterKey 选择报告构建器 实现。
type ReportAdapterKey string

const (
	ReportAdapterPersonalityType ReportAdapterKey = "personality_type"
	ReportAdapterTraitProfile    ReportAdapterKey = "trait_profile"
	ReportAdapterMBTI            ReportAdapterKey = "mbti"
	ReportAdapterSBTI            ReportAdapterKey = "sbti"
	ReportAdapterBigFive         ReportAdapterKey = "bigfive"
)

// ResolvedAdapterKey 返回配置化 报告适配器 从 显式 键 或 通用 report 类型。
// Legacy model-特定 报告适配器 必须 be set on Adapter键 在 旧版 derivation。
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
