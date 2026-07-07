package legacy_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/legacy"
)

func TestKindMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		legacy        string
		wantKind      string
		wantSubKind   string
		wantAlgorithm string
	}{
		{"scale", "scale", "", "scale_default"},
		{legacy.KindMBTIMigration, "personality", "typology", "mbti"},
		{legacy.KindSBTIMigration, "personality", "typology", "sbti"},
	}
	for _, tc := range tests {
		kind, subKind, algorithm, ok := legacy.KindMapping(tc.legacy)
		if !ok || kind != tc.wantKind || subKind != tc.wantSubKind || algorithm != tc.wantAlgorithm {
			t.Fatalf("KindMapping(%s) = (%s,%s,%s,%v), want (%s,%s,%s,true)",
				tc.legacy, kind, subKind, algorithm, ok, tc.wantKind, tc.wantSubKind, tc.wantAlgorithm)
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
