//go:build integration

package evaluation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"gorm.io/gorm"
)

func casNativeEntry(t *testing.T, ctx context.Context, b *SQLHistoricalOwnerBatch, owner, outcome uint64, id, eventType string, run *evidence.HistoricalRunReferenceV1) SQLHistoricalBatchAttachment {
	t.Helper()
	binding, e := SQLHistoricalBatchBinding(ctx, b, owner, outcome, eventType, run)
	if e != nil {
		t.Fatal(e)
	}
	entry := nativeHistoricalEntry(id, eventType, binding, run)
	entry.Source.Digest = evidence.SourceDigest("mysql-cast-binary-row-v2", []byte("original-row:"+id))
	entry.Proof.Digest = entry.Source.Digest
	return SQLHistoricalBatchAttachment{AssessmentID: owner, OutcomeID: outcome, Entry: entry, ContentDigest: evidence.SourceDigest("legacy-domain-json-bytes-v1", []byte("original-content:"+id))}
}
func casNativePlan(t *testing.T, db *gorm.DB, ids []uint64, build func(context.Context, *SQLHistoricalOwnerBatch) []SQLHistoricalBatchAttachment) *SQLHistoricalBatchCASPlan {
	t.Helper()
	var p *SQLHistoricalBatchCASPlan
	if e := batchNativeTx(t, db, func(ctx context.Context, c *SQLHistoricalResponsibilityCycle) error {
		b, e := PrepareSQLHistoricalOwnerBatch(ctx, c, SQLHistoricalOwnerBatchRequest{AssessmentIDs: ids}, DefaultSQLHistoricalOwnerBatchLimits())
		if e != nil {
			return e
		}
		p, e = PrepareSQLHistoricalBatchCAS(ctx, b, build(ctx, b))
		return e
	}); e != nil {
		t.Fatal(e)
	}
	return p
}
func casNativeApply(t *testing.T, db *gorm.DB, p *SQLHistoricalBatchCASPlan, rollback bool) (*SQLHistoricalBatchCASStatement, error) {
	t.Helper()
	var s *SQLHistoricalBatchCASStatement
	sentinel := errors.New("host rollback after statement")
	e := db.Transaction(func(tx *gorm.DB) error {
		var e error
		s, e = p.Apply(hostmysql.WithTx(t.Context(), tx))
		if e != nil {
			if errors.Is(e, ErrSQLHistoricalBatchConflict) {
				actual, _ := p.capture(tx, true)
				expected, _ := p.expectedImage(tx)
				for table, rows := range actual.rows {
					for i, row := range rows {
						if i >= len(expected.rows[table]) {
							continue
						}
						for field, value := range row {
							before := expected.rows[table][i][field]
							if (value == nil) != (before == nil) || value != nil && before != nil && *value != *before {
								actualSize, expectedSize := -1, -1
								if value != nil {
									actualSize = len(*value)
								}
								if before != nil {
									expectedSize = len(*before)
								}
								t.Logf("CAS exact-byte difference table=%s field=%s actualBytes=%d expectedBytes=%d", table, field, actualSize, expectedSize)
							}
						}
					}
				}
			}
			return e
		}
		if rollback {
			return sentinel
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if rollback && errors.Is(e, sentinel) {
		return s, nil
	}
	return s, e
}
func casNativeVerify(t *testing.T, db *gorm.DB, s *SQLHistoricalBatchCASStatement, ids []uint64) error {
	t.Helper()
	return batchNativeTx(t, db, func(ctx context.Context, c *SQLHistoricalResponsibilityCycle) error {
		b, e := PrepareSQLHistoricalOwnerBatch(ctx, c, SQLHistoricalOwnerBatchRequest{AssessmentIDs: ids}, DefaultSQLHistoricalOwnerBatchLimits())
		if e != nil {
			return e
		}
		return s.VerifyPersisted(ctx, b)
	})
}

func TestSQLHistoricalBatchCASMergedOwnerStandardPreservedAndReadbackNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	record, _ := testCommittedReference(t, 9001, 42, "current-standard-committed")
	if e := db.Create(outcomeToPO(record)).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.Exec("UPDATE runtime_checkpoint SET finished_at=? WHERE assessment_id=42", record.EvaluatedAt().Truncate(time.Millisecond)).Error; e != nil {
		t.Fatal(e)
	}
	p := casNativePlan(t, db, []uint64{42}, func(ctx context.Context, b *SQLHistoricalOwnerBatch) []SQLHistoricalBatchAttachment {
		return []SQLHistoricalBatchAttachment{
			casNativeEntry(t, ctx, b, 42, 0, "old-request-1", "evaluation.requested", nil),
			casNativeEntry(t, ctx, b, 42, 0, "old-retry-2", "evaluation.retry.requested", &evidence.HistoricalRunReferenceV1{RunID: "42:1", Attempt: 1}),
			casNativeEntry(t, ctx, b, 42, 0, "old-failed-3", "evaluation.failed", nil),
			casNativeEntry(t, ctx, b, 42, 9001, "old-outcome-1", "evaluation.outcome.committed", &evidence.HistoricalRunReferenceV1{RunID: "42:1", Attempt: 1}),
			casNativeEntry(t, ctx, b, 42, 9001, "old-outcome-2", "evaluation.outcome.committed", &evidence.HistoricalRunReferenceV1{RunID: "42:1", Attempt: 1}),
		}
	})
	s, e := casNativeApply(t, db, p, false)
	if e != nil {
		t.Fatal(e)
	}
	if len(storedHistoricalSet(t, db, "assessment", "historical_lifecycle_evidence", 42).Entries) != 3 || len(storedHistoricalSet(t, db, "evaluation_outcome", "historical_committed_evidence", 9001).Entries) != 2 {
		t.Fatal("same owner multi-source append lost original IDs")
	}
	r := s.Report()
	if len(r.Locations) != 5 || !r.StatementApplied || r.HostCommitVerified || r.DropReady || !r.HostCommitRequired {
		t.Fatal("statement declared host commit/closure")
	}
	if e = casNativeVerify(t, db, s, []uint64{42}); e != nil {
		t.Fatal("independent actual persisted full baseline", e)
	}
	duplicate, e := casNativeApply(t, db, p, false)
	if e != nil {
		t.Fatal("exact post-state idempotence", e)
	}
	for _, v := range duplicate.Report().Locations {
		if v.StatementWrote || !v.AlreadyMatched {
			t.Fatal("idempotent replay rewrote original proof")
		}
	}
	var po EvaluationOutcomePO
	if e = db.Where("id=?", 9001).Take(&po).Error; e != nil {
		t.Fatal(e)
	}
	if po.CommittedEventID == nil || *po.CommittedEventID != "current-standard-committed" || po.CommittedEventEvidence == nil {
		t.Fatal("standard single-slot overwritten")
	}
	if _, e = outcomeFromPO(&po); e != nil {
		t.Fatal("standard/outcome immutable facts changed", e)
	}
	if err := db.Exec("UPDATE assessment SET version=version+1 WHERE id=42").Error; err != nil {
		t.Fatal(err)
	}
	if err := casNativeVerify(t, db, s, []uint64{42}); !errors.Is(err, ErrSQLHistoricalBatchConflict) {
		t.Fatal("post-CAS non-evidence change was exempted", err)
	}
	pool, e := db.DB()
	if e != nil || pool.PingContext(t.Context()) != nil {
		t.Fatal("borrowed pool closed")
	}
}

func TestSQLHistoricalBatchCASRollbackAndUnknownCommitNoReceiptNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	p := casNativePlan(t, db, []uint64{42}, func(ctx context.Context, b *SQLHistoricalOwnerBatch) []SQLHistoricalBatchAttachment {
		return []SQLHistoricalBatchAttachment{casNativeEntry(t, ctx, b, 42, 0, "rollback-1", "evaluation.requested", nil)}
	})
	s, e := casNativeApply(t, db, p, true)
	if e != nil || s == nil {
		t.Fatal(e)
	}
	if storedHistoricalSet(t, db, "assessment", "historical_lifecycle_evidence", 42) != nil {
		t.Fatal("borrowed rollback leaked evidence")
	}
	if e = casNativeVerify(t, db, s, []uint64{42}); !errors.Is(e, ErrSQLHistoricalBatchConflict) {
		t.Fatal("rolled-back or unknown commit became persistent proof", e)
	}
	r := s.Report()
	if r.HostCommitVerified || r.DropReady {
		t.Fatal("statement receipt upgraded unknown commit")
	}
	if _, e = casNativeApply(t, db, p, false); e != nil {
		t.Fatal("CAS bridge committed/rolled back borrowed transaction", e)
	}
}

func TestSQLHistoricalBatchCASConcurrentIdempotentAndConflictingProofNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	build := func(id string) func(context.Context, *SQLHistoricalOwnerBatch) []SQLHistoricalBatchAttachment {
		return func(ctx context.Context, b *SQLHistoricalOwnerBatch) []SQLHistoricalBatchAttachment {
			return []SQLHistoricalBatchAttachment{casNativeEntry(t, ctx, b, 42, 0, id, "evaluation.requested", nil)}
		}
	}
	a := casNativePlan(t, db, []uint64{42}, build("old-same"))
	b := casNativePlan(t, db, []uint64{42}, build("old-other"))
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := casNativeApply(t, db, a, false); results <- e }()
	}
	wg.Wait()
	close(results)
	for e := range results {
		if e != nil {
			t.Fatal("concurrent exact state failed", e)
		}
	}
	if _, e := casNativeApply(t, db, b, false); !errors.Is(e, ErrSQLHistoricalBatchConflict) {
		t.Fatal("unrelated append used stale baseline", e)
	}
	if len(storedHistoricalSet(t, db, "assessment", "historical_lifecycle_evidence", 42).Entries) != 1 {
		t.Fatal("conflict corrupted original set")
	}
	fresh := casNativePlan(t, db, []uint64{42}, build("old-other"))
	if _, e := casNativeApply(t, db, fresh, false); e != nil {
		t.Fatal("fresh exact baseline could not append a legal existing set", e)
	}

	if e := batchNativeTx(t, db, func(ctx context.Context, c *SQLHistoricalResponsibilityCycle) error {
		page, e := PrepareSQLHistoricalOwnerBatch(ctx, c, SQLHistoricalOwnerBatchRequest{AssessmentIDs: []uint64{42}}, DefaultSQLHistoricalOwnerBatchLimits())
		if e != nil {
			return e
		}
		entry := casNativeEntry(t, ctx, page, 42, 0, "old-same", "evaluation.requested", nil)
		entry.Entry.Proof.Verification.OperationID = "124-1"
		_, e = PrepareSQLHistoricalBatchCAS(ctx, page, []SQLHistoricalBatchAttachment{entry})
		if !errors.Is(e, evidence.ErrHistoricalReferenceConflict) {
			return fmt.Errorf("different conclusion was accepted: %w", e)
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
}

func TestSQLHistoricalBatchCASFullBaselineNullClockRunSchemaNative(t *testing.T) {
	for _, test := range []struct{ name, mutation string }{
		{"physical_null", "UPDATE assessment SET conducting_context=CAST('null' AS JSON) WHERE id=42"},
		{"nullable_model", "UPDATE assessment SET evaluation_model_code='' WHERE id=42"},
		{"version", "UPDATE assessment SET version=version+1 WHERE id=42"},
		{"millisecond", "UPDATE runtime_checkpoint SET finished_at=DATE_ADD(finished_at,INTERVAL 1000 MICROSECOND) WHERE assessment_id=42"},
		{"new_run", "INSERT INTO runtime_checkpoint(scope,resource_id,attempt_no,assessment_id,status,started_at) VALUES('evaluation_run','42:2',2,42,'pending','2026-10-08 01:02:03.456')"},
		{"future_column", "ALTER TABLE assessment ADD COLUMN future_retirement_test VARCHAR(20) NULL"},
		{"clock_metadata", "ALTER TABLE runtime_checkpoint MODIFY finished_at DATETIME NULL"},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := openHistoricalReferencesDB(t)
			insertHistoricalAssessment(t, db, 42)
			p := casNativePlan(t, db, []uint64{42}, func(ctx context.Context, b *SQLHistoricalOwnerBatch) []SQLHistoricalBatchAttachment {
				return []SQLHistoricalBatchAttachment{casNativeEntry(t, ctx, b, 42, 0, "baseline-1", "evaluation.requested", nil)}
			})
			if e := db.Exec(test.mutation).Error; e != nil {
				t.Fatal(e)
			}
			if s, e := casNativeApply(t, db, p, false); e == nil || s != nil {
				t.Fatal("full raw baseline change accepted")
			}
			if storedHistoricalSet(t, db, "assessment", "historical_lifecycle_evidence", 42) != nil {
				t.Fatal("failed all-row preflight wrote evidence")
			}
		})
	}
}

func TestSQLHistoricalBatchCASActualTransactionIdentityAndInputValidationNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	var p *SQLHistoricalBatchCASPlan
	if e := batchNativeTx(t, db, func(ctx context.Context, c *SQLHistoricalResponsibilityCycle) error {
		b, e := PrepareSQLHistoricalOwnerBatch(ctx, c, SQLHistoricalOwnerBatchRequest{AssessmentIDs: []uint64{42}}, DefaultSQLHistoricalOwnerBatchLimits())
		if e != nil {
			return e
		}
		entry := casNativeEntry(t, ctx, b, 42, 0, "input-1", "evaluation.requested", nil)
		p, e = PrepareSQLHistoricalBatchCAS(ctx, b, []SQLHistoricalBatchAttachment{entry})
		if e != nil {
			return e
		}
		if s, e := p.Apply(ctx); e == nil || s != nil {
			return errors.New("old RRRO accepted for writing")
		}
		bad := entry
		bad.Entry = entry.Entry.Clone()
		bad.Entry.Proof.BusinessBindingSHA256 = evidence.SourceDigest("other-owner", []byte("43")).SHA256
		if s, e := PrepareSQLHistoricalBatchCAS(ctx, b, []SQLHistoricalBatchAttachment{bad}); e == nil || s != nil {
			return errors.New("copied owner binding accepted")
		}
		bad = entry
		bad.Entry = entry.Entry.Clone()
		bad.Entry.Run = &evidence.HistoricalRunReferenceV1{RunID: "42:2", Attempt: 2}
		if s, e := PrepareSQLHistoricalBatchCAS(ctx, b, []SQLHistoricalBatchAttachment{bad}); e == nil || s != nil {
			return errors.New("unknown original Run accepted")
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	if s, e := p.Apply(hostmysql.WithTx(t.Context(), db)); e == nil || s != nil {
		t.Fatal("ordinary pool accepted as borrowed Tx")
	}
	if e := db.Transaction(func(tx *gorm.DB) error {
		_, e := p.Apply(hostmysql.WithTx(t.Context(), tx))
		if e == nil {
			return errors.New("read committed accepted")
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted}); e != nil {
		t.Fatal(e)
	}
	other := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, other, 42)
	if s, e := casNativeApply(t, other, p, false); s != nil || !errors.Is(e, ErrSQLHistoricalFactsIdentity) {
		t.Fatal("different actual schema accepted", e)
	}
	if _, e := casNativeApply(t, db.Session(&gorm.Session{PrepareStmt: true}), p, false); e != nil {
		t.Fatal("legitimate prepared borrowed RW transaction rejected", e)
	}
}

func TestSQLHistoricalBatchCASCapacityAndCrossOwnerDuplicateNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	insertHistoricalAssessment(t, db, 43)
	if err := batchNativeTx(t, db, func(ctx context.Context, c *SQLHistoricalResponsibilityCycle) error {
		page, e := PrepareSQLHistoricalOwnerBatch(ctx, c, SQLHistoricalOwnerBatchRequest{AssessmentIDs: []uint64{42, 43}}, DefaultSQLHistoricalOwnerBatchLimits())
		if e != nil {
			return e
		}
		a := casNativeEntry(t, ctx, page, 42, 0, "shared-id", "evaluation.requested", nil)
		b := casNativeEntry(t, ctx, page, 43, 0, "shared-id", "evaluation.requested", nil)
		if p, e := PrepareSQLHistoricalBatchCAS(ctx, page, []SQLHistoricalBatchAttachment{a, b}); p != nil || !errors.Is(e, evidence.ErrHistoricalReferenceConflict) {
			return errors.New("same original ID assigned two owners")
		}
		b = casNativeEntry(t, ctx, page, 42, 0, "another-id", "evaluation.requested", nil)
		b.Entry.Source.PrimaryKeySHA256 = a.Entry.Source.PrimaryKeySHA256
		if p, e := PrepareSQLHistoricalBatchCAS(ctx, page, []SQLHistoricalBatchAttachment{a, b}); p != nil || !errors.Is(e, evidence.ErrHistoricalReferenceConflict) {
			return errors.New("one physical source PK bound to different original IDs")
		}
		var entries []SQLHistoricalBatchAttachment
		for i := 0; i <= evidence.HistoricalReferenceMaxEntries; i++ {
			entries = append(entries, casNativeEntry(t, ctx, page, 42, 0, fmt.Sprintf("bounded-%d", i), "evaluation.requested", nil))
		}
		if p, e := PrepareSQLHistoricalBatchCAS(ctx, page, entries); p != nil || !errors.Is(e, evidence.ErrHistoricalReferenceLimit) {
			return errors.New("owner evidence capacity overrun accepted")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if storedHistoricalSet(t, db, "assessment", "historical_lifecycle_evidence", 42) != nil || storedHistoricalSet(t, db, "assessment", "historical_lifecycle_evidence", 43) != nil {
		t.Fatal("invalid preparation wrote business evidence")
	}
}

func TestSQLHistoricalBatchCASMixedMatchedAndNewSlotsNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	record, _ := testCommittedReference(t, 9001, 42, "mixed-standard")
	if e := db.Create(outcomeToPO(record)).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.Exec("UPDATE runtime_checkpoint SET finished_at=? WHERE assessment_id=42", record.EvaluatedAt().Truncate(time.Millisecond)).Error; e != nil {
		t.Fatal(e)
	}
	first := casNativePlan(t, db, []uint64{42}, func(ctx context.Context, b *SQLHistoricalOwnerBatch) []SQLHistoricalBatchAttachment {
		return []SQLHistoricalBatchAttachment{casNativeEntry(t, ctx, b, 42, 0, "mixed-old-request", "evaluation.requested", nil)}
	})
	if _, e := casNativeApply(t, db, first, false); e != nil {
		t.Fatal(e)
	}
	second := casNativePlan(t, db, []uint64{42}, func(ctx context.Context, b *SQLHistoricalOwnerBatch) []SQLHistoricalBatchAttachment {
		return []SQLHistoricalBatchAttachment{casNativeEntry(t, ctx, b, 42, 0, "mixed-old-request", "evaluation.requested", nil), casNativeEntry(t, ctx, b, 42, 9001, "mixed-new-outcome", "evaluation.outcome.committed", &evidence.HistoricalRunReferenceV1{RunID: "42:1", Attempt: 1})}
	})
	s, e := casNativeApply(t, db, second, false)
	if e != nil {
		t.Fatal("unchanged slot produced a zero-row write conflict", e)
	}
	if len(s.Report().Locations) != 2 || !s.Report().Locations[0].AlreadyMatched || !s.Report().Locations[1].StatementWrote {
		t.Fatal("mixed idempotent/new statement facts inaccurate")
	}
	if e := casNativeVerify(t, db, s, []uint64{42}); e != nil {
		t.Fatal(e)
	}
	third := casNativePlan(t, db, []uint64{42}, func(ctx context.Context, b *SQLHistoricalOwnerBatch) []SQLHistoricalBatchAttachment {
		return []SQLHistoricalBatchAttachment{casNativeEntry(t, ctx, b, 42, 9001, "mixed-new-outcome", "evaluation.outcome.committed", &evidence.HistoricalRunReferenceV1{RunID: "42:1", Attempt: 1}), casNativeEntry(t, ctx, b, 42, 9001, "mixed-another-outcome", "evaluation.outcome.committed", &evidence.HistoricalRunReferenceV1{RunID: "42:1", Attempt: 1})}
	})
	s, e = casNativeApply(t, db, third, false)
	if e != nil {
		t.Fatal(e)
	}
	locations := s.Report().Locations
	if !locations[0].StatementWrote || !locations[0].AlreadyMatched || locations[0].ReferenceAppended || !locations[1].ReferenceAppended || locations[1].AlreadyMatched {
		t.Fatal("same-slot existing and new original IDs conflated")
	}
	if e = casNativeVerify(t, db, s, []uint64{42}); e != nil {
		t.Fatal(e)
	}
}
