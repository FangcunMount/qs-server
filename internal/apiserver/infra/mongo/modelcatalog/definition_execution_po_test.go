package modelcatalog

import (
	"reflect"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestDefinitionExecutionSpecRoundTripPO(t *testing.T) {
	t.Parallel()
	value := &domain.Definition{Execution: domain.ExecutionSpec{
		Brief2: &domain.Brief2Spec{FormVariant: "parent", PrimaryFactorCode: "gec", IndexFactorCodes: []string{"bri"}, ValidityFactorCodes: []string{"negativity"}},
		SPM:    &domain.SPMSpec{TimeLimitSeconds: 2400, TotalFactorCode: "total", ItemSets: []domain.SPMItemSet{{Code: "A", Items: []domain.SPMItem{{QuestionCode: "A1", CorrectOptionCode: "1"}}}}},
	}}
	got := definitionFromPO(definitionToPO(value))
	if got == nil || !reflect.DeepEqual(got.Execution, value.Execution) {
		t.Fatalf("execution = %#v, want %#v", got, value.Execution)
	}
}
