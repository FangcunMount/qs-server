package typology

import (
	"reflect"
	"testing"
)

func TestNewPipelineComponentsWiresNativeTriple(t *testing.T) {
	t.Parallel()

	components := NewPipelineComponents()
	if components.InputAssembler == nil || components.Calculator == nil || components.OutcomeAssembler == nil {
		t.Fatal("factor_classification pipeline triple is incomplete")
	}
	if reflect.TypeOf(components.Calculator).Name() == "evaluatorCalculator" {
		t.Fatalf("calculator = %T, want native typologyCalculator", components.Calculator)
	}
}
