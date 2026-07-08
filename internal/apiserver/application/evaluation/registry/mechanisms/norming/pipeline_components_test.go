package norming

import (
	"reflect"
	"testing"
)

func TestNewPipelineComponentsWiresNativeTriple(t *testing.T) {
	t.Parallel()

	components := NewPipelineComponents(nil)
	if components.InputAssembler == nil || components.Calculator == nil || components.OutcomeAssembler == nil {
		t.Fatal("factor_norm pipeline triple is incomplete")
	}
	if reflect.TypeOf(components.Calculator).Name() == "evaluatorCalculator" {
		t.Fatalf("calculator = %T, want native factorNormCalculator", components.Calculator)
	}
}
