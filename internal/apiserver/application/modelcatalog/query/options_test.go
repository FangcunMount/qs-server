package query

import (
	"testing"

	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestCatalogOptionsFilterAlgorithmsByCanonicalKind(t *testing.T) {
	t.Parallel()

	options := catalogOptionsForKind(modelcatalog.KindScale)
	if len(options.Algorithms) != 1 || options.Algorithms[0].Value != string(domain.AlgorithmScaleDefault) {
		t.Fatalf("scale algorithms = %#v", options.Algorithms)
	}
	if got := algorithmOptions("personality"); len(got) != 0 {
		t.Fatalf("personality algorithms = %#v, want empty", got)
	}
	typology := catalogOptionsForKind(modelcatalog.KindTypology)
	if len(typology.Algorithms) != 1 || typology.Algorithms[0].Value != string(domain.AlgorithmPersonalityTypology) {
		t.Fatalf("typology algorithms = %#v", typology.Algorithms)
	}
	behavioral := catalogOptionsForKind(modelcatalog.KindBehavioralRating)
	if len(behavioral.Algorithms) != 2 || behavioral.Algorithms[0].Value != string(domain.AlgorithmBrief2) || behavioral.Algorithms[1].Value != string(domain.AlgorithmSPMSensory) {
		t.Fatalf("behavioral_rating algorithms = %#v", behavioral.Algorithms)
	}
}
