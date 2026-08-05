package interpretation

import (
	"testing"
	"time"
)

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

func TestCatalogDriftPageCanAdvanceCursorWithNoFindings(t *testing.T) {
	t.Parallel()
	page := catalogDriftPage([]CatalogDriftItem{}, 42, false)
	if len(page.Items) != 0 || page.NextCursor != "42" {
		t.Fatalf("page = %#v", page)
	}
}

func TestCatalogAuditCheckpointPORoundTripPreservesOrgSnapshots(t *testing.T) {
	t.Parallel()
	completedAt := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	source := CatalogAuditCheckpoint{
		SchemaVersion: 1, Revision: 9, CycleID: "cycle-1", Phase: CatalogAuditPhaseCatalog,
		AfterAssessmentID: 99, SourceUpperAssessmentID: 100, CatalogUpperAssessmentID: 101,
		WorkingCounts: CatalogDriftCounts{Missing: 1}, WorkingOrgCounts: map[int64]CatalogDriftCounts{7: {Missing: 1}},
		LastCompleted: &CatalogCompletedAuditSnapshot{CycleID: "cycle-0", CompletedAt: completedAt, Counts: CatalogDriftCounts{Dangling: 2}, OrgCounts: map[int64]CatalogDriftCounts{7: {Dangling: 2}}},
	}
	result := checkpointFromPO(checkpointToPO(source))
	if result.Revision != 9 || result.WorkingOrgCounts[7].Missing != 1 || result.LastCompleted == nil || result.LastCompleted.OrgCounts[7].Dangling != 2 {
		t.Fatalf("checkpoint round-trip = %#v", result)
	}
}

func TestClassifyCatalogAuditEntryDetectsCatalogDriftClasses(t *testing.T) {
	t.Parallel()
	entry := ReportCatalogPO{AssessmentID: 1, OrgID: 7, TesteeID: 8, SourceKind: ReportCatalogSourceArchive, SourceID: 1}
	danglingAndWrong := classifyCatalogAuditEntry(entry, CatalogSourceAssociation{}, false, 99)
	if danglingAndWrong.Dangling != 1 || danglingAndWrong.WrongWinner != 1 {
		t.Fatalf("dangling/wrong = %#v", danglingAndWrong)
	}
	mismatch := classifyCatalogAuditEntry(entry, CatalogSourceAssociation{AssessmentID: 1, OrgID: 7, HasOrgID: true, TesteeID: 9}, true, 0)
	if mismatch.AssociationMismatch != 1 || mismatch.Total() != 1 {
		t.Fatalf("mismatch = %#v", mismatch)
	}
}
