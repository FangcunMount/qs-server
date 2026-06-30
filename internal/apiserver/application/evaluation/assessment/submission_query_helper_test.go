package assessment

import (
	"context"
	"testing"
)

func TestMyAssessmentQueryListPassesModelKindFilter(t *testing.T) {
	reader := &managementAssessmentReaderStub{}
	query := myAssessmentQuery{reader: reader}
	testeeID := uint64(618855887087350318)

	_, _, err := query.List(context.Background(), ListMyAssessmentsDTO{
		TesteeID:  testeeID,
		Page:      1,
		PageSize:  20,
		ModelKind: "scale",
	}, 1, 20)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if reader.filter.ModelKind != "scale" {
		t.Fatalf("filter.ModelKind = %q, want scale", reader.filter.ModelKind)
	}
	if reader.filter.TesteeID == nil || *reader.filter.TesteeID != testeeID {
		t.Fatalf("filter.TesteeID = %#v, want %d", reader.filter.TesteeID, testeeID)
	}
}
