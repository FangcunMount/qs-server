package assessment

import (
	"reflect"
	"testing"
)

func TestScoringProjectionContainsOnlyAssessmentFinalizationFacts(t *testing.T) {
	typeOfProjection := reflect.TypeOf(ScoringProjection{})
	want := []string{"ModelRef", "Summary", "Score", "Level"}
	if typeOfProjection.NumField() != len(want) {
		t.Fatalf("ScoringProjection fields = %d, want %d", typeOfProjection.NumField(), len(want))
	}
	for index, name := range want {
		if typeOfProjection.Field(index).Name != name {
			t.Fatalf("ScoringProjection field %d = %s, want %s", index, typeOfProjection.Field(index).Name, name)
		}
	}
}
