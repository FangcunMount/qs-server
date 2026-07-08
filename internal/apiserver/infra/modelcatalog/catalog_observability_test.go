package modelcatalog

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRecordLegacyFallbackIncrementsMetric(t *testing.T) {
	store := CatalogStoreDualStore
	operation := "get_published_by_ref"
	before := testutil.ToFloat64(legacyFallbackHitsTotal.WithLabelValues(string(store), operation))

	recordLegacyFallback(context.Background(), store, operation, "unit-test")

	after := testutil.ToFloat64(legacyFallbackHitsTotal.WithLabelValues(string(store), operation))
	if after != before+1 {
		t.Fatalf("fallback counter = %v, want %v", after, before+1)
	}
}

func TestFallbackRecorderHookForTest(t *testing.T) {
	var hits []string
	restore := SetFallbackRecorderForTest(func(store CatalogStore, operation string) {
		hits = append(hits, string(store)+":"+operation)
	})
	defer restore()

	recordLegacyFallback(context.Background(), CatalogStorePublishedTypology, "find_by_questionnaire", "hook-test")
	if len(hits) != 1 || hits[0] != "published_typology:find_by_questionnaire" {
		t.Fatalf("hits = %#v, want published_typology hook", hits)
	}
}
