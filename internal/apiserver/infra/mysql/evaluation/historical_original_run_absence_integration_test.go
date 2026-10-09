//go:build integration

package evaluation

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"

	"gorm.io/gorm"
)

// This one family uses the existing native helper's previously absent schema.
// Only actual RR-RO batches construct the absence capability; no private map,
// editable Snapshot, source authorization, or terminal qualification is minted.
func TestSQLHistoricalOriginalRunGlobalAbsenceNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	insertHistoricalAssessment(t, db, 43)
	if r := db.Exec("UPDATE assessment SET org_id=8 WHERE id=43"); r.Error != nil || r.RowsAffected != 1 {
		t.Fatal("different organization fixture was not prepared", r.Error, r.RowsAffected)
	}
	record, _ := testCommittedReference(t, 9001, 42, "unused-absence-fixture-reference")
	po := outcomeToPO(record)
	po.CommittedEventID, po.CommittedEventEvidence = nil, nil
	if r := db.Create(po); r.Error != nil || r.RowsAffected != 1 {
		t.Fatal("canonical Outcome fixture was not prepared", r.Error, r.RowsAffected)
	}
	if r := db.Exec("UPDATE assessment SET evaluated_at=? WHERE id=42", record.EvaluatedAt()); r.Error != nil || r.RowsAffected != 1 {
		t.Fatal("canonical owner clock was not prepared", r.Error, r.RowsAffected)
	}
	if r := db.Exec("DELETE FROM runtime_checkpoint WHERE assessment_id=42"); r.Error != nil || r.RowsAffected != 1 {
		t.Fatal("original Run fixture was not actually removed", r.Error, r.RowsAffected)
	}

	request := SQLHistoricalOwnerBatchRequest{AssessmentIDs: []uint64{42}}
	var first *SQLHistoricalOwnerBatch
	var endedPool gorm.ConnPool
	t.Run("actual_rrro_original_id_absent", func(t *testing.T) {
		if err := batchNativeTx(t, db, func(ctx context.Context, cycle *SQLHistoricalResponsibilityCycle) error {
			b, err := PrepareSQLHistoricalOwnerBatch(ctx, cycle, request, DefaultSQLHistoricalOwnerBatchLimits())
			if err != nil {
				return err
			}
			facts, err := b.OwnerByAssessment(42)
			if err != nil {
				return err
			}
			actual, err := facts.OutcomeRecord(9001)
			if err != nil || actual == nil || actual.RunID() != record.RunID() || len(facts.Snapshot().Runs) != 0 {
				t.Fatal("actual immutable owner/Outcome absent-Run baseline missing", err)
			}
			if err = facts.OriginalOutcomeRunAbsent(ctx, 9001, actual.RunID()); err != nil {
				t.Fatal("actual RR-RO global absence rejected", err)
			}
			if err = facts.OriginalOutcomeRunAbsent(ctx, 9001, "42:2"); !errors.Is(err, ErrSQLHistoricalBatchConflict) {
				t.Fatal("different declared original Run accepted", err)
			}
			report := b.Report()
			if !report.Complete || !report.SourceAuthenticationRequired || !report.ExternalBusinessClosureRequired || !report.WriterFenceRequired || !report.CASRequired || report.DropReady {
				t.Fatal("native absence observation became retirement authority")
			}
			tx, err := historicalTx(ctx)
			if err != nil {
				return err
			}
			first, endedPool = b, tx.Statement.ConnPool
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})
	if first == nil || endedPool == nil {
		t.Fatal("first actual batch/borrowed epoch was not retained")
	}
	var ignored int
	if err := endedPool.QueryRowContext(t.Context(), "SELECT 1").Scan(&ignored); !errors.Is(err, sql.ErrTxDone) {
		t.Fatal("host did not end the old actual read transaction", err)
	}

	t.Run("cross_org_scope_deleted_original_id_rejects", func(t *testing.T) {
		// The original ID physically exists, but none of its organization,
		// scope, owner, or soft-delete facts belongs to the selected owner.
		// This prevents an owner-filtered or live-scope query passing as a
		// global original-ID absence observation.
		if r := db.Exec("INSERT INTO runtime_checkpoint(scope,resource_id,attempt_no,assessment_id,status,started_at,finished_at,deleted_at) VALUES('other_scope',?,1,43,'succeeded',?,?,?)", record.RunID(), record.EvaluatedAt(), record.EvaluatedAt(), record.EvaluatedAt()); r.Error != nil || r.RowsAffected != 1 {
			t.Fatal("physical cross-org/scope/deleted Run was not prepared", r.Error, r.RowsAffected)
		}
		if err := batchNativeTx(t, db, func(ctx context.Context, cycle *SQLHistoricalResponsibilityCycle) error {
			b, err := PrepareSQLHistoricalOwnerBatch(ctx, cycle, request, DefaultSQLHistoricalOwnerBatchLimits())
			if err != nil {
				return err
			}
			facts, err := b.OwnerByAssessment(42)
			if err != nil {
				return err
			}
			if len(facts.Snapshot().Runs) != 0 {
				t.Fatal("global contradiction unexpectedly became a selected-owner Run")
			}
			if err = facts.OriginalOutcomeRunAbsent(ctx, 9001, record.RunID()); !errors.Is(err, ErrSQLHistoricalBatchConflict) {
				t.Fatal("physical retained original ID hidden by org/scope/deletion", err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("fresh_after_ended_epoch_detects_original_id_appearance", func(t *testing.T) {
		if err := batchNativeTx(t, db, func(ctx context.Context, cycle *SQLHistoricalResponsibilityCycle) error {
			fresh, err := PrepareSQLHistoricalOwnerBatch(ctx, cycle, request, DefaultSQLHistoricalOwnerBatchLimits())
			if err != nil {
				return err
			}
			if first.cycle.transaction == cycle.transaction {
				t.Fatal("old transaction reused as fresh original-ID observation")
			}
			if !reflect.DeepEqual(first.rows, fresh.rows) || first.Report().BusinessRowsSHA256 != fresh.Report().BusinessRowsSHA256 {
				t.Fatal("counterexample altered selected-owner rows instead of global absence")
			}
			if err = first.RecheckBusiness(ctx, cycle); !errors.Is(err, ErrSQLHistoricalBatchConflict) {
				t.Fatal("fresh recheck missed an original-ID row outside selected-owner baseline", err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})
}
