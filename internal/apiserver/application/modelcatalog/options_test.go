package modelcatalog

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestCatalogOptionsFilterAlgorithmsByCanonicalKind(t *testing.T) {
	t.Parallel()

	options := catalogOptionsForKind(KindScale)
	if len(options.Algorithms) != 1 || options.Algorithms[0].Value != string(domain.AlgorithmScaleDefault) {
		t.Fatalf("scale algorithms = %#v", options.Algorithms)
	}
	if got := algorithmOptions("personality"); len(got) != 0 {
		t.Fatalf("personality algorithms = %#v, want empty", got)
	}
}
