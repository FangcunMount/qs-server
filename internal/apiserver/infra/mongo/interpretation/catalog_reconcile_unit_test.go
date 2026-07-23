package interpretation

import "testing"

func TestCountAssociationMismatchesUsesSharedValidatorAndSkipsDangling(t *testing.T) {
	entries := []ReportCatalogPO{
		{AssessmentID: 1, OrgID: 10, TesteeID: 100, SourceID: 11},
		{AssessmentID: 2, OrgID: 10, TesteeID: 200, SourceID: 22},
		{AssessmentID: 3, OrgID: 10, TesteeID: 300, SourceID: 33},
	}
	sources := map[uint64]CatalogSourceAssociation{
		11: {AssessmentID: 1, OrgID: 10, HasOrgID: true, TesteeID: 100},
		22: {AssessmentID: 2, OrgID: 99, HasOrgID: true, TesteeID: 200},
		// Source 33 is deliberately absent and belongs to the dangling count.
	}
	if got := countAssociationMismatches(entries, sources); got != 1 {
		t.Fatalf("countAssociationMismatches() = %d, want 1", got)
	}
}
