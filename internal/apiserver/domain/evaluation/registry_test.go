package evaluation

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
)

func TestDefaultModelDescriptors(t *testing.T) {
	descs := DefaultModelDescriptors()
	if len(descs) != 3 {
		t.Fatalf("descriptor count = %d, want 3", len(descs))
	}
	algorithms := TypologyAlgorithms(descs)
	if len(algorithms) != 2 {
		t.Fatalf("typology algorithms = %#v", algorithms)
	}
	if algorithms[0] != assessmentmodel.AlgorithmMBTI || algorithms[1] != assessmentmodel.AlgorithmSBTI {
		t.Fatalf("algorithms = %#v", algorithms)
	}
}
