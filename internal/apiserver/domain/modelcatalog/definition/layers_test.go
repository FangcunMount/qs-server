package definition_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/decision"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
)

func TestDecisionSpecFromOmitsPresentationCopy(t *testing.T) {
	t.Parallel()
	def := &definition.Definition{
		Outcomes: []conclusion.Outcome{{
			Code: "low", Title: "低风险", Summary: "摘要", Description: "长文案",
		}},
		Conclusions: []conclusion.Conclusion{
			conclusion.RiskConclusion{
				FactorCode: "total",
				Rules: []conclusion.ScoreRangeOutcome{{
					MinScore: 0, MaxScore: 10, MaxInclusive: true,
					OutcomeCode: "low", Title: "规则标题", Summary: "规则摘要", Description: "规则描述",
				}},
			},
		},
	}
	spec := definition.DecisionSpecFrom(def)
	if len(spec.OutcomeRefs) != 1 || spec.OutcomeRefs[0].Code != "low" {
		t.Fatalf("OutcomeRefs = %#v", spec.OutcomeRefs)
	}
	if len(spec.ScoreRanges) != 1 || len(spec.ScoreRanges[0].Rules) != 1 {
		t.Fatalf("ScoreRanges = %#v", spec.ScoreRanges)
	}
	rule := spec.ScoreRanges[0].Rules[0]
	if rule.OutcomeCode != "low" {
		t.Fatalf("rule = %#v", rule)
	}
	// DecisionSpec must not carry presentation fields (struct has none); matching
	// still yields the same OutcomeCode if titles change on the source Definition.
	def.Conclusions[0] = conclusion.RiskConclusion{
		FactorCode: "total",
		Rules: []conclusion.ScoreRangeOutcome{{
			MinScore: 0, MaxScore: 10, MaxInclusive: true,
			OutcomeCode: "low", Title: "改写标题", Summary: "改写摘要", Description: "改写描述",
		}},
	}
	again := definition.DecisionSpecFrom(def)
	code, ok := decision.MatchOutcomeCode(5, again.ScoreRanges[0].Rules)
	if !ok || code != "low" {
		t.Fatalf("MatchOutcomeCode after copy rewrite = (%q,%v)", code, ok)
	}
}

func TestInterpretationAssetsFromKeepsPresentationAndProfiles(t *testing.T) {
	t.Parallel()
	def := &definition.Definition{
		Outcomes: []conclusion.Outcome{{Code: "ENTJ", Title: "指挥官", Summary: "摘要"}},
		Conclusions: []conclusion.Conclusion{
			conclusion.TypeConclusion{
				Decision: conclusion.TypeDecision{Kind: binding.DecisionKindPoleComposition},
				Profiles: []conclusion.TypeOutcomeProfile{{
					OutcomeCode: "ENTJ", Pattern: "E-N-T-J", Suggestions: []string{"行动"},
					Rarity: conclusion.Rarity{Percent: 1.8, Label: "稀少", OneInX: 56},
				}},
			},
		},
		ReportMap: definition.ReportMap{Sections: []definition.ReportSection{{
			Code: "main", AdapterKey: "personality_type", TemplateID: "tpl-1",
		}, {
			Code: "factors", Kind: definition.ReportSectionKindFactorScores, SourceRefs: []string{"E", "N"},
		}}},
	}
	assets := definition.InterpretationAssetsFrom(def)
	got, ok := assets.FindOutcome("ENTJ")
	if !ok || got.Title != "指挥官" || got.Summary != "摘要" {
		t.Fatalf("outcome = %#v", got)
	}
	profile, ok := assets.FindProfile("ENTJ")
	if !ok || profile.Pattern != "E-N-T-J" || len(profile.Suggestions) != 1 {
		t.Fatalf("profile = %#v", profile)
	}
	if profile.Rarity.Label != "稀少" || profile.Rarity.OneInX != 56 {
		t.Fatalf("rarity = %#v", profile.Rarity)
	}
	if len(assets.ReportSpec.Sections) != 2 || assets.ReportSpec.Sections[0].AdapterKey != "personality_type" {
		t.Fatalf("report = %#v", assets.ReportSpec)
	}
	if got := assets.ReportSpec.Sections[1].SourceRefs; len(got) != 2 || got[0] != "E" || got[1] != "N" {
		t.Fatalf("frozen source refs = %#v", got)
	}
}

func TestMaterializeLayersPopulatesStoredFields(t *testing.T) {
	t.Parallel()
	def := &definition.Definition{
		Outcomes: []conclusion.Outcome{{Code: "low", Title: "低风险", Summary: "摘要"}},
		Conclusions: []conclusion.Conclusion{conclusion.RiskConclusion{
			FactorCode: "total",
			Rules:      []conclusion.ScoreRangeOutcome{{MinScore: 0, MaxScore: 10, MaxInclusive: true, OutcomeCode: "low"}},
		}},
	}
	definition.MaterializeLayers(def)
	if !def.DecisionSpec.IsMaterialized() || !def.InterpretationAssets.IsMaterialized() {
		t.Fatalf("stored layers = decision:%#v assets:%#v", def.DecisionSpec, def.InterpretationAssets)
	}
	if spec := def.ResolvedDecisionSpec(); len(spec.ScoreRanges) != 1 {
		t.Fatalf("resolved decision = %#v", spec)
	}
	if got, ok := def.ResolvedInterpretationAssets().FindOutcome("low"); !ok || got.Summary != "摘要" {
		t.Fatalf("resolved assets = %#v", got)
	}
}
