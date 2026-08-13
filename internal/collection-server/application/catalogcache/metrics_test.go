package catalogcache

import "testing"

func TestPublishedModelMetricLabelsAreStableAndLowCardinality(t *testing.T) {
	got := []string{KindPublishedModelDetail, KindPublishedModelList, KindPublishedModelOptions}
	want := []string{"published_model_detail", "published_model_list", "published_model_options"}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("published-model metric label %d = %q, want %q", index, got[index], want[index])
		}
	}
}
