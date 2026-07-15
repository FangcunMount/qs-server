package personalitykind

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestClassifyAcceptsOnlyKnownLegacyTypologyIdentities(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		raw      bson.M
		eligible bool
	}{
		{
			name:     "legacy SBTI",
			raw:      bson.M{"code": "SBTI_FUN", "kind": "personality", "sub_kind": "typology", "algorithm": "sbti", "product_channel": "personality"},
			eligible: true,
		},
		{
			name:     "already canonical kind with retired channel",
			raw:      bson.M{"code": "SBTI_FUN", "kind": "typology", "sub_kind": "typology", "algorithm": "sbti", "product_channel": "personality"},
			eligible: true,
		},
		{
			name:     "unverified personality row",
			raw:      bson.M{"code": "OTHER", "kind": "personality", "sub_kind": "trait", "algorithm": "sbti", "product_channel": "personality"},
			eligible: false,
		},
		{
			name:     "wrong algorithm",
			raw:      bson.M{"code": "OTHER", "kind": "personality", "sub_kind": "typology", "algorithm": "scale_default", "product_channel": "personality"},
			eligible: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := classify(Heads, tc.raw)
			if got.Eligible != tc.eligible {
				t.Fatalf("eligible = %v, want %v (%s)", got.Eligible, tc.eligible, got.Reason)
			}
		})
	}
}

func TestClassifyUsesCanonicalTypologyConstants(t *testing.T) {
	t.Parallel()
	got := classify(Snapshots, bson.M{
		"code":            "SBTI_FUN",
		"kind":            "personality",
		"sub_kind":        string(domain.SubKindTypology),
		"algorithm":       string(domain.AlgorithmSBTI),
		"product_channel": "",
	})
	if !got.Eligible {
		t.Fatalf("published legacy typology should be eligible: %s", got.Reason)
	}
}
