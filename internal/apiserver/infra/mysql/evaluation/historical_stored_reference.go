package evaluation

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
)

// SQLHistoricalStoredReference reads one already persisted reference from the
// genuine borrowed owner batch. It does not derive terminal status or write a
// missing reference. The host must independently rebind it to original sources.
func SQLHistoricalStoredReference(ctx context.Context, b *SQLHistoricalOwnerBatch, assessment, outcome uint64, eventID string) (evidence.HistoricalReferenceEntryV1, error) {
	if b == nil || eventID == "" || assessment == 0 || b.ValidateBorrowedSnapshot(ctx) != nil {
		return evidence.HistoricalReferenceEntryV1{}, ErrSQLHistoricalBatchCAS
	}
	table, column, id := "assessment", "historical_lifecycle_evidence", assessment
	if outcome != 0 {
		table, column, id = "evaluation_outcome", "historical_committed_evidence", outcome
	}
	row, e := casRow(sqlHistoricalCASImage{rows: b.rows}, table, id)
	if e != nil {
		return evidence.HistoricalReferenceEntryV1{}, e
	}
	stored, e := historicalDecode(row[column])
	if e != nil || stored == nil {
		return evidence.HistoricalReferenceEntryV1{}, ErrSQLHistoricalBatchCAS
	}
	var found *evidence.HistoricalReferenceEntryV1
	for _, entry := range stored.Entries {
		if entry.EventID == eventID {
			if found != nil {
				return evidence.HistoricalReferenceEntryV1{}, evidence.ErrHistoricalReferenceConflict
			}
			copy := entry.Clone()
			found = &copy
		}
	}
	if found == nil || found.Validate() != nil || b.ValidateBorrowedSnapshot(ctx) != nil {
		return evidence.HistoricalReferenceEntryV1{}, ErrSQLHistoricalBatchCAS
	}
	return found.Clone(), nil
}
