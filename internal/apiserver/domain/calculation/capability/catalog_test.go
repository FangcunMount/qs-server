package capability_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/capability"
)

func TestCatalogFreezesExecutionPathStrategyMatrix(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path   capability.Path
		usage  capability.Usage
		want   []string
		accept []string
		reject []string
	}{
		{
			path: capability.PathScaleDescriptor, usage: capability.UsageQuestionAggregation,
			want:   []string{"sum", "avg", "cnt"},
			accept: []string{"sum", "avg", "average", "cnt", "count"},
			reject: []string{"weighted_sum", "max", "min", "weighted_avg"},
		},
		{
			path: capability.PathScaleDescriptor, usage: capability.UsageCompositeProjection,
			want:   []string{"sum", "avg", "weighted_sum", "none", "lookup", "custom"},
			accept: []string{"sum", "average", "weighted_sum", "none"},
			reject: []string{"cnt", "weighted_avg", "max"},
		},
		{
			path: capability.PathTypologyDescriptor, usage: capability.UsageTypologyLeaf,
			want:   []string{"sum"},
			accept: []string{"sum"},
			reject: []string{"avg", "weighted_avg", "cnt"},
		},
		{
			path: capability.PathTypologyDescriptor, usage: capability.UsageTypologyComposite,
			want:   []string{"sum", "avg", "weighted_avg"},
			accept: []string{"sum", "avg", "average", "weighted_avg"},
			reject: []string{"cnt", "weighted_sum", "max"},
		},
		{
			path: capability.PathBehavioralRatingDescriptor, usage: capability.UsageQuestionAggregation,
			want:   []string{"sum", "avg", "cnt"},
			reject: []string{"weighted_sum"},
		},
		{
			path: capability.PathCognitiveDescriptor, usage: capability.UsageQuestionAggregation,
			want:   []string{"sum", "avg", "cnt"},
			reject: []string{"max"},
		},
	}
	for _, tc := range cases {
		got := capability.SupportedCodes(tc.path, tc.usage)
		if len(got) != len(tc.want) {
			t.Fatalf("%s/%s codes = %v, want %v", tc.path, tc.usage, got, tc.want)
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Fatalf("%s/%s codes = %v, want %v", tc.path, tc.usage, got, tc.want)
			}
		}
		for _, strategy := range tc.accept {
			if !capability.Supports(tc.path, tc.usage, strategy) {
				t.Fatalf("%s/%s should accept %q", tc.path, tc.usage, strategy)
			}
		}
		for _, strategy := range tc.reject {
			if capability.Supports(tc.path, tc.usage, strategy) {
				t.Fatalf("%s/%s should reject %q", tc.path, tc.usage, strategy)
			}
		}
	}
}

func TestCanonicalNormalizesAliases(t *testing.T) {
	t.Parallel()
	code, ok := capability.Canonical(capability.PathScaleDescriptor, capability.UsageQuestionAggregation, "average")
	if !ok || code != "avg" {
		t.Fatalf("Canonical(average) = %q,%v want avg,true", code, ok)
	}
	code, ok = capability.Canonical(capability.PathScaleDescriptor, capability.UsageQuestionAggregation, "count")
	if !ok || code != "cnt" {
		t.Fatalf("Canonical(count) = %q,%v want cnt,true", code, ok)
	}
}
