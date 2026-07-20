package capability_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/capability"
)

func TestMissingAnswerPolicyMatrix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		path  capability.Path
		usage capability.Usage
		want  capability.MissingAnswerPolicy
	}{
		{capability.PathScaleDescriptor, capability.UsageQuestionAggregation, capability.MissingAnswerSkip},
		{capability.PathBehavioralRatingDescriptor, capability.UsageQuestionAggregation, capability.MissingAnswerSkip},
		{capability.PathCognitiveDescriptor, capability.UsageQuestionAggregation, capability.MissingAnswerSkip},
		{capability.PathTypologyDescriptor, capability.UsageTypologyLeaf, capability.MissingAnswerFail},
		{capability.PathTypologyDescriptor, capability.UsageTypologyComposite, capability.MissingAnswerFail},
		{capability.PathScaleDescriptor, capability.UsageCompositeProjection, capability.MissingAnswerFail},
	}
	for _, tc := range cases {
		if got := capability.MissingAnswerPolicyFor(tc.path, tc.usage); got != tc.want {
			t.Fatalf("MissingAnswerPolicyFor(%s,%s) = %s, want %s", tc.path, tc.usage, got, tc.want)
		}
	}
}

func TestRequiresExecutableScoringByPath(t *testing.T) {
	t.Parallel()
	if !capability.RequiresExecutableScoring(capability.PathScaleDescriptor, "total") {
		t.Fatal("scale total must require executable scoring")
	}
	if !capability.RequiresExecutableScoring(capability.PathScaleDescriptor, "") {
		t.Fatal("empty role (dimension) must require executable scoring on scale")
	}
	if capability.RequiresExecutableScoring(capability.PathScaleDescriptor, "report_group") {
		t.Fatal("report_group must not require scoring")
	}
	if capability.RequiresExecutableScoring(capability.PathCognitiveDescriptor, "total") {
		t.Fatal("cognitive total must not require measure scoring (SPM execution)")
	}
	if !capability.RequiresExecutableScoring(capability.PathTypologyDescriptor, "dimension") {
		t.Fatal("typology dimension must require scoring")
	}
}

func TestAuthoringStrategyCodesExposePathSubset(t *testing.T) {
	t.Parallel()
	scale := capability.AuthoringStrategyCodes(capability.PathScaleDescriptor)
	if len(scale) == 0 {
		t.Fatal("scale authoring strategies empty")
	}
	for _, code := range scale {
		if code == "weighted_avg" || code == "max" {
			t.Fatalf("scale authoring should not expose %q", code)
		}
	}
	typology := capability.AuthoringStrategyCodes(capability.PathTypologyDescriptor)
	foundWeightedAvg := false
	for _, code := range typology {
		if code == "cnt" {
			t.Fatal("typology authoring should not expose cnt")
		}
		if code == "weighted_avg" {
			foundWeightedAvg = true
		}
	}
	if !foundWeightedAvg {
		t.Fatal("typology authoring should expose weighted_avg")
	}
}

func TestDeclaredAuthoringStrategyCodesIsCatalogUnion(t *testing.T) {
	t.Parallel()
	got := capability.DeclaredAuthoringStrategyCodes()
	want := []string{"sum", "avg", "cnt", "weighted_sum", "none", "lookup", "custom", "weighted_avg"}
	if len(got) != len(want) {
		t.Fatalf("DeclaredAuthoringStrategyCodes() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("DeclaredAuthoringStrategyCodes() = %#v, want %#v", got, want)
		}
	}
	for _, code := range []string{"max", "min", "first", "last"} {
		for _, item := range got {
			if item == code {
				t.Fatalf("declared authoring must not include %q", code)
			}
		}
	}
}
