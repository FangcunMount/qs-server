package ruleset

import (
	"context"
	"testing"

	catalogobs "github.com/FangcunMount/qs-server/internal/apiserver/infra/modelcatalog"
)

func TestLayeredCatalogRecordsStaticFallbackObservability(t *testing.T) {
	var hits []string
	restore := catalogobs.SetFallbackRecorderForTest(func(store catalogobs.CatalogStore, operation string) {
		hits = append(hits, string(store)+":"+operation)
	})
	defer restore()

	static, err := NewDefaultStaticCatalog(nil)
	if err != nil {
		t.Fatalf("NewDefaultStaticCatalog: %v", err)
	}
	catalog := NewLayeredCatalog(stubStore{}, static)
	if _, _, err := catalog.ResolveByQuestionnaire(context.Background(), "SBTI_FUN", "1.0.0"); err != nil {
		t.Fatalf("ResolveByQuestionnaire: %v", err)
	}
	if len(hits) != 1 || hits[0] != "layered_static:resolve_by_questionnaire" {
		t.Fatalf("hits = %#v, want layered_static fallback observability", hits)
	}
}
