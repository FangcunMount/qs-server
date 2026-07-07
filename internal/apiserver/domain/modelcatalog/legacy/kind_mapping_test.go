package legacy_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/legacy"
)

func TestKindMapping(t *testing.T) {
	t.Parallel()

	kind, subKind, algorithm, ok := legacy.KindMapping("scale")
	if !ok || kind != "scale" || subKind != "" || algorithm != "scale_default" {
		t.Fatalf("KindMapping(scale) = (%s,%s,%s,%v), want (scale,,scale_default,true)", kind, subKind, algorithm, ok)
	}
	for _, flatKind := range []string{legacy.KindMBTIMigration, legacy.KindSBTIMigration} {
		if _, _, _, ok := legacy.KindMapping(flatKind); ok {
			t.Fatalf("KindMapping(%s) should be removed from runtime read paths", flatKind)
		}
	}
}

func TestIsMigrationOnlyKind(t *testing.T) {
	t.Parallel()

	if !legacy.IsMigrationOnlyKind(legacy.KindMBTIMigration) {
		t.Fatal("mbti migration kind should be migration-only")
	}
	if legacy.IsMigrationOnlyKind("personality") {
		t.Fatal("personality kind should not be migration-only")
	}
}
