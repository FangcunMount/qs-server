package modelcatalog

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/decision"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
)

type DecisionSpecPO struct {
	ScoreRanges     []FactorScoreRangesPO `bson:"score_ranges,omitempty"`
	TypeDecision    *TypeDecisionFactPO   `bson:"type_decision,omitempty"`
	OutcomeRefs     []OutcomeRefPO        `bson:"outcome_refs,omitempty"`
	SpecialOutcomes []string              `bson:"special_outcomes,omitempty"`
}

type FactorScoreRangesPO struct {
	FactorCode string             `bson:"factor_code,omitempty"`
	Kind       string             `bson:"kind,omitempty"`
	Primary    bool               `bson:"primary,omitempty"`
	Rules      []ScoreRangeRulePO `bson:"rules,omitempty"`
}

type ScoreRangeRulePO struct {
	MinScore     float64 `bson:"min_score,omitempty"`
	MaxScore     float64 `bson:"max_score,omitempty"`
	MaxInclusive bool    `bson:"max_inclusive,omitempty"`
	UnboundedMax bool    `bson:"unbounded_max,omitempty"`
	Level        string  `bson:"level,omitempty"`
	OutcomeCode  string  `bson:"outcome_code,omitempty"`
}

type TypeDecisionFactPO struct {
	Kind                        string  `bson:"kind,omitempty"`
	FallbackSimilarityThreshold float64 `bson:"fallback_similarity_threshold,omitempty"`
	FallbackCode                string  `bson:"fallback_code,omitempty"`
	TopK                        int     `bson:"top_k,omitempty"`
}

type OutcomeRefPO struct {
	Code string `bson:"code,omitempty"`
}

type InterpretationAssetsPO struct {
	Outcomes   []OutcomePresentationPO `bson:"outcomes,omitempty"`
	Profiles   []TypeProfilePO         `bson:"profiles,omitempty"`
	ReportSpec InterpretationReportPO  `bson:"report_spec,omitempty"`
}

type OutcomePresentationPO struct {
	OutcomeCode string `bson:"outcome_code,omitempty"`
	Title       string `bson:"title,omitempty"`
	Summary     string `bson:"summary,omitempty"`
	Description string `bson:"description,omitempty"`
}

type TypeProfilePO struct {
	OutcomeCode string   `bson:"outcome_code,omitempty"`
	Pattern     string   `bson:"pattern,omitempty"`
	Traits      []string `bson:"traits,omitempty"`
	Strengths   []string `bson:"strengths,omitempty"`
	Weaknesses  []string `bson:"weaknesses,omitempty"`
	Suggestions []string `bson:"suggestions,omitempty"`
	ImageURL    string   `bson:"image_url,omitempty"`
	Image       string   `bson:"image,omitempty"`
	IsSpecial   bool     `bson:"is_special,omitempty"`
	Trigger     string   `bson:"trigger,omitempty"`
	Commentary  string   `bson:"commentary,omitempty"`
}

type InterpretationReportPO struct {
	Sections []InterpretationReportSectionPO `bson:"sections,omitempty"`
}

type InterpretationReportSectionPO struct {
	Code          string   `bson:"code,omitempty"`
	Title         string   `bson:"title,omitempty"`
	SourceRefs    []string `bson:"source_refs,omitempty"`
	Kind          string   `bson:"kind,omitempty"`
	AdapterKey    string   `bson:"adapter_key,omitempty"`
	TemplateID    string   `bson:"template_id,omitempty"`
	CategoryLabel string   `bson:"category_label,omitempty"`
}

func decisionSpecToPO(spec decision.Spec) DecisionSpecPO {
	out := DecisionSpecPO{
		SpecialOutcomes: append([]string(nil), spec.SpecialOutcomes...),
	}
	if len(spec.ScoreRanges) > 0 {
		out.ScoreRanges = make([]FactorScoreRangesPO, 0, len(spec.ScoreRanges))
		for _, item := range spec.ScoreRanges {
			rules := make([]ScoreRangeRulePO, 0, len(item.Rules))
			for _, rule := range item.Rules {
				rules = append(rules, ScoreRangeRulePO{
					MinScore: rule.MinScore, MaxScore: rule.MaxScore, MaxInclusive: rule.MaxInclusive,
					UnboundedMax: rule.UnboundedMax, Level: rule.Level, OutcomeCode: rule.OutcomeCode,
				})
			}
			out.ScoreRanges = append(out.ScoreRanges, FactorScoreRangesPO{
				FactorCode: item.FactorCode, Kind: item.Kind, Primary: item.Primary, Rules: rules,
			})
		}
	}
	if spec.TypeDecision != nil {
		out.TypeDecision = &TypeDecisionFactPO{
			Kind: string(spec.TypeDecision.Kind), FallbackSimilarityThreshold: spec.TypeDecision.FallbackSimilarityThreshold,
			FallbackCode: spec.TypeDecision.FallbackCode, TopK: spec.TypeDecision.TopK,
		}
	}
	if len(spec.OutcomeRefs) > 0 {
		out.OutcomeRefs = make([]OutcomeRefPO, 0, len(spec.OutcomeRefs))
		for _, item := range spec.OutcomeRefs {
			out.OutcomeRefs = append(out.OutcomeRefs, OutcomeRefPO{Code: item.Code})
		}
	}
	return out
}

func decisionSpecFromPO(po DecisionSpecPO) decision.Spec {
	spec := decision.Spec{SpecialOutcomes: append([]string(nil), po.SpecialOutcomes...)}
	if len(po.ScoreRanges) > 0 {
		spec.ScoreRanges = make([]decision.FactorScoreRanges, 0, len(po.ScoreRanges))
		for _, item := range po.ScoreRanges {
			rules := make([]decision.ScoreRangeRule, 0, len(item.Rules))
			for _, rule := range item.Rules {
				rules = append(rules, decision.ScoreRangeRule{
					MinScore: rule.MinScore, MaxScore: rule.MaxScore, MaxInclusive: rule.MaxInclusive,
					UnboundedMax: rule.UnboundedMax, Level: rule.Level, OutcomeCode: rule.OutcomeCode,
				})
			}
			spec.ScoreRanges = append(spec.ScoreRanges, decision.FactorScoreRanges{
				FactorCode: item.FactorCode, Kind: item.Kind, Primary: item.Primary, Rules: rules,
			})
		}
	}
	if po.TypeDecision != nil {
		spec.TypeDecision = &decision.TypeDecisionFact{
			Kind: binding.DecisionKind(po.TypeDecision.Kind), FallbackSimilarityThreshold: po.TypeDecision.FallbackSimilarityThreshold,
			FallbackCode: po.TypeDecision.FallbackCode, TopK: po.TypeDecision.TopK,
		}
	}
	if len(po.OutcomeRefs) > 0 {
		spec.OutcomeRefs = make([]decision.OutcomeRef, 0, len(po.OutcomeRefs))
		for _, item := range po.OutcomeRefs {
			spec.OutcomeRefs = append(spec.OutcomeRefs, decision.OutcomeRef{Code: item.Code})
		}
	}
	return spec
}

func interpretationAssetsToPO(assets interpretationassets.Assets) InterpretationAssetsPO {
	out := InterpretationAssetsPO{}
	if len(assets.Outcomes) > 0 {
		out.Outcomes = make([]OutcomePresentationPO, 0, len(assets.Outcomes))
		for _, item := range assets.Outcomes {
			out.Outcomes = append(out.Outcomes, OutcomePresentationPO{
				OutcomeCode: item.OutcomeCode, Title: item.Title, Summary: item.Summary, Description: item.Description,
			})
		}
	}
	if len(assets.Profiles) > 0 {
		out.Profiles = make([]TypeProfilePO, 0, len(assets.Profiles))
		for _, item := range assets.Profiles {
			out.Profiles = append(out.Profiles, TypeProfilePO{
				OutcomeCode: item.OutcomeCode, Pattern: item.Pattern,
				Traits: append([]string(nil), item.Traits...), Strengths: append([]string(nil), item.Strengths...),
				Weaknesses: append([]string(nil), item.Weaknesses...), Suggestions: append([]string(nil), item.Suggestions...),
				ImageURL: item.ImageURL, Image: item.Image, IsSpecial: item.IsSpecial, Trigger: item.Trigger, Commentary: item.Commentary,
			})
		}
	}
	if len(assets.ReportSpec.Sections) > 0 {
		sections := make([]InterpretationReportSectionPO, 0, len(assets.ReportSpec.Sections))
		for _, section := range assets.ReportSpec.Sections {
			sections = append(sections, InterpretationReportSectionPO{
				Code: section.Code, Title: section.Title, SourceRefs: append([]string(nil), section.SourceRefs...),
				Kind: section.Kind, AdapterKey: section.AdapterKey, TemplateID: section.TemplateID, CategoryLabel: section.CategoryLabel,
			})
		}
		out.ReportSpec = InterpretationReportPO{Sections: sections}
	}
	return out
}

func interpretationAssetsFromPO(po InterpretationAssetsPO) interpretationassets.Assets {
	assets := interpretationassets.Assets{}
	if len(po.Outcomes) > 0 {
		assets.Outcomes = make([]interpretationassets.OutcomePresentation, 0, len(po.Outcomes))
		for _, item := range po.Outcomes {
			assets.Outcomes = append(assets.Outcomes, interpretationassets.OutcomePresentation{
				OutcomeCode: item.OutcomeCode, Title: item.Title, Summary: item.Summary, Description: item.Description,
			})
		}
	}
	if len(po.Profiles) > 0 {
		assets.Profiles = make([]interpretationassets.TypeProfilePresentation, 0, len(po.Profiles))
		for _, item := range po.Profiles {
			assets.Profiles = append(assets.Profiles, interpretationassets.TypeProfilePresentation{
				OutcomeCode: item.OutcomeCode, Pattern: item.Pattern,
				Traits: append([]string(nil), item.Traits...), Strengths: append([]string(nil), item.Strengths...),
				Weaknesses: append([]string(nil), item.Weaknesses...), Suggestions: append([]string(nil), item.Suggestions...),
				ImageURL: item.ImageURL, Image: item.Image, IsSpecial: item.IsSpecial, Trigger: item.Trigger, Commentary: item.Commentary,
			})
		}
	}
	if len(po.ReportSpec.Sections) > 0 {
		sections := make([]interpretationassets.ReportSection, 0, len(po.ReportSpec.Sections))
		for _, section := range po.ReportSpec.Sections {
			sections = append(sections, interpretationassets.ReportSection{
				Code: section.Code, Title: section.Title, SourceRefs: append([]string(nil), section.SourceRefs...),
				Kind: section.Kind, AdapterKey: section.AdapterKey, TemplateID: section.TemplateID, CategoryLabel: section.CategoryLabel,
			})
		}
		assets.ReportSpec = interpretationassets.ReportSpec{Sections: sections}
	}
	return assets
}
