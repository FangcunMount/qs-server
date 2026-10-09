//go:build integration

package evaluation

import (
	"context"
	"errors"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/scheduler"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
)

// One policy family uses only the existing ephemeral native-schema helper.
// The low-level CAS observes persistence; it never grants source/closure/DROP.
func TestSQLHistoricalMissingOriginalRunPolicyNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	record, _ := testCommittedReference(t, 9001, 42, "unused-standard-fixture-id")
	po := outcomeToPO(record)
	po.CommittedEventID, po.CommittedEventEvidence = nil, nil
	if err := db.Create(po).Error; err != nil {
		t.Fatal(err)
	}
	if r := db.Exec("UPDATE assessment SET evaluated_at=? WHERE id=42", record.EvaluatedAt()); r.Error != nil || r.RowsAffected != 1 {
		t.Fatal("exact canonical owner preparation failed", r.Error, r.RowsAffected)
	}
	if r := db.Exec("DELETE FROM runtime_checkpoint WHERE assessment_id=42"); r.Error != nil || r.RowsAffected != 1 {
		t.Fatal("actual original Run absence preparation failed", r.Error, r.RowsAffected)
	}

	var attachment SQLHistoricalBatchAttachment
	plan := casNativePlan(t, db, []uint64{42}, func(ctx context.Context, b *SQLHistoricalOwnerBatch) []SQLHistoricalBatchAttachment {
		attachment = casNativeEntry(t, ctx, b, 42, 9001, "old-outcome-missing-run", "evaluation.outcome.committed", nil)
		attachment.Entry.Proof.Class = evidence.Unverifiable
		attachment.Entry.Proof.Verification.Reason = "original_outcome_run_absent"
		return []SQLHistoricalBatchAttachment{attachment}
	})
	t.Run("commit_independent_readback_and_audit_gap", func(t *testing.T) {
		statement, err := casNativeApply(t, db, plan, false)
		if err != nil || statement == nil {
			t.Fatal("actual nil-Run CAS failed", err)
		}
		if err = casNativeVerify(t, db, statement, []uint64{42}); err != nil {
			t.Fatal("actual independent persisted baseline rejected", err)
		}
		report := statement.Report()
		if report.HostCommitVerified || report.SourceAuthenticated || report.BusinessClosureVerified || report.DropReady {
			t.Fatal("persistence observation became whole retirement authority")
		}
		set := storedHistoricalSet(t, db, "evaluation_outcome", "historical_committed_evidence", 9001)
		if set == nil || len(set.Entries) != 1 || set.Entries[0].Run != nil || set.Entries[0].Proof.Class != evidence.Unverifiable {
			t.Fatal("original Run/attempt invented or gap class lost")
		}
		reader := NewConsistencyReadModel(db)
		batch, err := reader.ReadBatch(t.Context(), 0, 2)
		if err != nil || len(batch.Items) != 1 {
			t.Fatal("actual business audit read failed", err)
		}
		item := batch.Items[0]
		if item.Run != nil || item.CommittedHistory == nil || item.CommittedHistory.Class != evidence.Unverifiable || item.CommittedHistory.RunID != "42:1" || len(item.HistoricalReferences) != 1 || item.HistoricalReferences[0].Attempt != 0 || item.HistoricalReferences[0].InvalidReason != "" || item.Outbox == nil || item.Outbox.Class != "" || item.Outbox.RowCount != 0 {
			t.Fatal("actual owner-only historical audit forged standard fingerprint or execution identity")
		}
		result, err := scheduler.NewService(reader).AuditBatch(t.Context(), 0, 2)
		if err != nil || result.Detected != 1 {
			t.Fatal("historical gap was hidden or repeated as a high missing Run", result, err)
		}
	})
	t.Run("global_cross_owner_scope_deleted_run_rejects", func(t *testing.T) {
		insertHistoricalAssessment(t, db, 43)
		result := db.Exec("INSERT INTO runtime_checkpoint(scope,resource_id,attempt_no,assessment_id,status,started_at,finished_at,deleted_at) VALUES('other_scope','42:1',1,43,'succeeded',?,?,?)", record.EvaluatedAt(), record.EvaluatedAt(), record.EvaluatedAt())
		if result.Error != nil || result.RowsAffected != 1 {
			t.Fatal("real cross-owner/scope/deleted counterexample missing", result.Error, result.RowsAffected)
		}
		if err := batchNativeTx(t, db, func(ctx context.Context, c *SQLHistoricalResponsibilityCycle) error {
			b, err := PrepareSQLHistoricalOwnerBatch(ctx, c, SQLHistoricalOwnerBatchRequest{AssessmentIDs: []uint64{42}}, DefaultSQLHistoricalOwnerBatchLimits())
			if err != nil {
				return err
			}
			if hash, err := SQLHistoricalBatchBinding(ctx, b, 42, 9001, "evaluation.outcome.committed", nil); hash != "" || !errors.Is(err, ErrSQLHistoricalBatchCAS) {
				t.Fatal("owner predicate hid retained original Run", err)
			}
			if p, err := PrepareSQLHistoricalBatchCAS(ctx, b, []SQLHistoricalBatchAttachment{attachment}); p != nil || !errors.Is(err, ErrSQLHistoricalBatchCAS) {
				t.Fatal("global original Run contradiction authorized prepare", err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := casNativeApply(t, db, plan, false); !errors.Is(err, ErrSQLHistoricalBatchCAS) {
			t.Fatal("RW capture hid a newly retained original Run", err)
		}
		if r := db.Exec("DELETE FROM runtime_checkpoint WHERE scope='other_scope' AND resource_id='42:1'"); r.Error != nil || r.RowsAffected != 1 {
			t.Fatal("owned counterexample cleanup failed", r.Error, r.RowsAffected)
		}
	})
	t.Run("actual_current_pending_responsibility_rejects", func(t *testing.T) {
		row := cycleTestMessage(t, "current-owner-request-pending", "evaluation.requested", "Evaluation", "42", cycleRequestedPayload())
		row["state"], row["transport_confirmed_at"] = evidence.String("pending"), nil
		cycleNativeInsert(t, db, row)
		if r := db.Exec("INSERT INTO qs_rm_evaluation_request_ref(event_id,assessment_id,org_id) VALUES('current-owner-request-pending',42,7)"); r.Error != nil || r.RowsAffected != 1 {
			t.Fatal("actual current protocol reference missing", r.Error, r.RowsAffected)
		}
		if err := batchNativeTx(t, db, func(ctx context.Context, c *SQLHistoricalResponsibilityCycle) error {
			b, err := PrepareSQLHistoricalOwnerBatch(ctx, c, SQLHistoricalOwnerBatchRequest{AssessmentIDs: []uint64{42}}, DefaultSQLHistoricalOwnerBatchLimits())
			if err != nil {
				return err
			}
			owner, err := b.OwnerByAssessment(42)
			if err != nil {
				return err
			}
			pending := false
			for _, r := range owner.Snapshot().Responsibilities {
				if r.Store == "rm_outbox" && r.EventID == "current-owner-request-pending" && r.Unfinished {
					pending = true
				}
			}
			if !pending {
				t.Fatal("actual SQL8 pending counterexample was not observed")
			}
			if p, err := PrepareSQLHistoricalBatchCAS(ctx, b, []SQLHistoricalBatchAttachment{attachment}); p != nil || !errors.Is(err, ErrSQLHistoricalBatchCAS) {
				t.Fatal("missing historical Run settled current pending responsibility", err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})
}
