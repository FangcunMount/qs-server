package evaluation

import (
	"context"
	"slices"
)

// SQLHistoricalBatchBindings reconstructs stable business digests from one
// actual bounded opaque owner page. The existing attachment type supplies only
// assessment/outcome/event-type/declared Run selectors here; its Proof, source,
// content and caller flags are deliberately not used as qualification.
// This is a read-only digest producer, not approval or a CAS constructor.
func SQLHistoricalBatchBindings(ctx context.Context, b *SQLHistoricalOwnerBatch, selections []SQLHistoricalBatchAttachment) ([]string, error) {
	if ctx == nil || b == nil || !b.report.Complete || len(selections) == 0 || len(selections) > 512 {
		return nil, ErrSQLHistoricalBatchCAS
	}
	if err := b.ValidateBorrowedSnapshot(ctx); err != nil {
		return nil, err
	}
	tx, err := historicalTx(ctx)
	if err != nil {
		return nil, err
	}
	server, database, err := historicalDatabase(tx)
	if err != nil || sqlHistoricalIdentity(server, database) != b.report.DatabaseIdentitySHA256 {
		return nil, ErrSQLHistoricalBatchCAS
	}
	image := sqlHistoricalCASImage{rows: b.rows}
	result := make([]string, len(selections))
	missing := map[string]bool{}
	for i, selection := range selections {
		// This same exact target mapper is used by the original single digest
		// reader and the actual batch CAS. No latest/nearest Run is selected.
		table, _, _, row, run, err := casTargetForBinding(image, selection, true)
		if err != nil {
			return nil, err
		}
		if table == "evaluation_outcome" && selection.Entry.Run == nil {
			missing[valueOrEmpty(row["evaluation_run_id"])] = true
		}
		binding, err := historicalStableBinding(server, database, table, selection.Entry.EventType, row, run)
		if err != nil {
			return nil, err
		}
		result[i] = binding
	}
	if len(missing) != 0 {
		ids := make([]string, 0, len(missing))
		for id := range missing {
			if id == "" {
				return nil, ErrSQLHistoricalBatchCAS
			}
			ids = append(ids, id)
		}
		slices.Sort(ids)
		// Real global absence is mandatory, with no org/scope/deleted filter.
		// It remains a read-only check and never grants execution permission.
		if err := casMissingOriginalRuns(tx, ids, false); err != nil {
			return nil, err
		}
	}
	if err := b.ValidateBorrowedSnapshot(ctx); err != nil {
		return nil, err
	}
	return result, nil
}
