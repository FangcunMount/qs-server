package behavioral

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
)

func TestEvaluateAlgorithmBackfillRequiresRetainedAlias(t *testing.T) {
	t.Parallel()
	got := EvaluateAlgorithmBackfill(binding.AlgorithmBrief2, &definition.Definition{
		Execution: definition.ExecutionSpec{Brief2: &definition.Brief2Spec{}},
	}, "")
	if got.Eligible || got.Reason != "not_retained_read_alias" {
		t.Fatalf("got = %#v", got)
	}
}

func TestEvaluateAlgorithmBackfillAcceptsBrief2Spec(t *testing.T) {
	t.Parallel()
	got := EvaluateAlgorithmBackfill(binding.AlgorithmBehavioralRatingDefault, &definition.Definition{
		Execution: definition.ExecutionSpec{Brief2: &definition.Brief2Spec{FormVariant: "parent"}},
	}, "")
	if !got.Eligible || got.To != binding.AlgorithmBrief2 {
		t.Fatalf("got = %#v", got)
	}
}

func TestEvaluateAlgorithmBackfillNormRefsRequireExplicitTarget(t *testing.T) {
	t.Parallel()
	def := &definition.Definition{
		Calibration: definition.Calibration{NormRefs: []norm.Ref{{FactorCode: "bri", NormTableVersion: "v1"}}},
	}
	got := EvaluateAlgorithmBackfill(binding.AlgorithmBehavioralRatingDefault, def, "")
	if got.Eligible || got.Reason != "ambiguous_brief2_or_spm_sensory" {
		t.Fatalf("ambiguous = %#v", got)
	}
	got = EvaluateAlgorithmBackfill(binding.AlgorithmBehavioralRatingDefault, def, binding.AlgorithmSPMSensory)
	if !got.Eligible || got.To != binding.AlgorithmSPMSensory {
		t.Fatalf("explicit spm = %#v", got)
	}
	got = EvaluateAlgorithmBackfill(binding.AlgorithmBehavioralRatingDefault, def, binding.AlgorithmBrief2)
	if !got.Eligible || got.To != binding.AlgorithmBrief2 {
		t.Fatalf("explicit brief2 = %#v", got)
	}
}

func TestEvaluateAlgorithmBackfillRejectsBareDefinition(t *testing.T) {
	t.Parallel()
	got := EvaluateAlgorithmBackfill(binding.AlgorithmBehavioralRatingDefault, &definition.Definition{}, "")
	if got.Eligible || got.Reason != "requires_brief2_execution_or_norm_refs" {
		t.Fatalf("got = %#v", got)
	}
}
