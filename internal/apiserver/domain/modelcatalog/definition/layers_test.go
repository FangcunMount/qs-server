package definition_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
)

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
