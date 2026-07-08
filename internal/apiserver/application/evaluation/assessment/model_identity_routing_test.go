package assessment

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
)

func TestEnrichModelIdentityResultDerivesRoutingFromKind(t *testing.T) {
	t.Parallel()

	got := EnrichModelIdentityResult(ModelIdentityResult{
		Kind:      string(binding.KindTypology),
		SubKind:   string(binding.SubKindTypology),
		Algorithm: string(binding.AlgorithmMBTI),
		Code:      "MBTI-16P",
	}, "")

	if got.ProductChannel == "" {
		t.Fatal("expected derived product_channel")
	}
	if got.AlgorithmFamily == "" {
		t.Fatal("expected derived algorithm_family")
	}
}

func TestEnrichModelIdentityResultPreservesExplicitProductChannel(t *testing.T) {
	t.Parallel()

	explicit := "behavior_ability"
	got := EnrichModelIdentityResult(ModelIdentityResult{
		Kind:           string(binding.KindTypology),
		SubKind:        string(binding.SubKindTypology),
		Algorithm:      string(binding.AlgorithmMBTI),
		ProductChannel: "from_row",
	}, explicit)

	if got.ProductChannel != explicit {
		t.Fatalf("product_channel = %q, want explicit %q", got.ProductChannel, explicit)
	}
}
