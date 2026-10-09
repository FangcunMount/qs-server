//go:build integration

package evaluation

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
)

// All lifecycle calls here belong to the native TEST HOST. The component is
// given actual borrowed transactions; none of its methods opens or ends them.
type provenanceNativeReadEpoch struct {
	tx    *gorm.DB
	ctx   context.Context
	batch *SQLHistoricalOwnerBatch
}

func provenanceNativeRead(t *testing.T, db *gorm.DB, request SQLHistoricalOwnerBatchRequest) (*provenanceNativeReadEpoch, error) {
	t.Helper()
	tx := db.Begin(&sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if tx.Error != nil {
		return nil, tx.Error
	}
	t.Cleanup(func() {
		if err := tx.Rollback().Error; err != nil && !errors.Is(err, sql.ErrTxDone) {
			t.Error("native test host read cleanup", err)
		}
	})
	ctx := hostmysql.WithTx(t.Context(), tx)
	server, database, err := historicalDatabase(tx)
	if err != nil {
		return nil, err
	}
	cycle, err := PrepareSQLHistoricalResponsibilityCycle(ctx, sqlHistoricalIdentity(server, database), DefaultSQLResponsibilityLimits())
	if err != nil {
		return nil, err
	}
	batch, err := PrepareSQLHistoricalOwnerBatch(ctx, cycle, request, DefaultSQLHistoricalOwnerBatchLimits())
	if err != nil {
		return nil, err
	}
	return &provenanceNativeReadEpoch{tx: tx, ctx: ctx, batch: batch}, nil
}

func provenanceNativeMustRead(t *testing.T, db *gorm.DB, request SQLHistoricalOwnerBatchRequest) *provenanceNativeReadEpoch {
	t.Helper()
	epoch, err := provenanceNativeRead(t, db, request)
	if err != nil {
		t.Fatal("actual RRRO full-schema/full-column page", err)
	}
	return epoch
}

func provenanceNativeEndRead(t *testing.T, epoch *provenanceNativeReadEpoch) {
	t.Helper()
	if err := epoch.tx.Rollback().Error; err != nil {
		t.Fatal("test host ends original RRRO", err)
	}
}

func provenanceNativePrepared(t *testing.T, db *gorm.DB, id string) (*provenanceNativeReadEpoch, *SQLHistoricalBatchCASPlan, *SQLHistoricalCASProvenance) {
	t.Helper()
	epoch := provenanceNativeMustRead(t, db, SQLHistoricalOwnerBatchRequest{AssessmentIDs: []uint64{42}})
	entry := casNativeEntry(t, epoch.ctx, epoch.batch, 42, 0, id, "evaluation.requested", nil)
	plan, err := PrepareSQLHistoricalBatchCAS(epoch.ctx, epoch.batch, []SQLHistoricalBatchAttachment{entry})
	if err != nil {
		t.Fatal("actual prepared plan", err)
	}
	provenance, err := SealSQLHistoricalCASProvenance(epoch.ctx, plan, epoch.batch)
	if err != nil {
		t.Fatal("actual original baseline/pool seal", err)
	}
	// Native business facts and transaction provenance are real; these compact
	// test conclusions are NOT authenticated production source/closure proof.
	if plan.oldTransaction != epoch.batch.cycle.transaction || provenance.readPool != epoch.tx.Statement.ConnPool {
		t.Fatal("native provenance lost original private instances")
	}
	return epoch, plan, provenance
}

func provenanceNativeHostWrite(t *testing.T, db *gorm.DB, plan *SQLHistoricalBatchCASPlan, original *SQLHistoricalCASProvenance, rollback bool, whileActive func(context.Context, *SQLHistoricalCASStatementBinding)) (*SQLHistoricalCASStatementBinding, error) {
	t.Helper()
	var binding *SQLHistoricalCASStatementBinding
	sentinel := errors.New("test host deliberately rolls back actual Apply")
	err := db.Transaction(func(tx *gorm.DB) error {
		ctx := hostmysql.WithTx(t.Context(), tx)
		statement, err := plan.Apply(ctx)
		if err != nil {
			return err
		}
		binding, err = original.BindStatement(ctx, statement)
		if err != nil {
			return err
		}
		if whileActive != nil {
			whileActive(ctx, binding)
		}
		if rollback {
			return sentinel
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if rollback && errors.Is(err, sentinel) {
		return binding, nil
	}
	return binding, err
}

func TestSQLCASProvenanceNativeCommitRollbackAndUnknownCommitScope(t *testing.T) {
	for _, rollback := range []bool{false, true} {
		name := "commit"
		if rollback {
			name = "rollback"
		}
		t.Run(name, func(t *testing.T) {
			db := openHistoricalReferencesDB(t)
			insertHistoricalAssessment(t, db, 42)
			read, plan, original := provenanceNativePrepared(t, db, "provenance-original-1")
			provenanceNativeEndRead(t, read)
			binding, err := provenanceNativeHostWrite(t, db, plan, original, rollback, nil)
			if err != nil || binding == nil {
				t.Fatal("actual write binding", err)
			}
			fresh := provenanceNativeMustRead(t, db, plan.request)
			err = binding.VerifyPersisted(fresh.ctx, fresh.batch)
			if rollback {
				if !errors.Is(err, ErrSQLHistoricalBatchConflict) {
					t.Fatal("ended rolled-back write acquired expected-state proof", err)
				}
				if storedHistoricalSet(t, db, "assessment", "historical_lifecycle_evidence", 42) != nil {
					t.Fatal("rollback left maintenance evidence")
				}
			} else {
				if err != nil {
					t.Fatal("actual committed raw baseline was not visible in independent RRRO", err)
				}
				if len(storedHistoricalSet(t, db, "assessment", "historical_lifecycle_evidence", 42).Entries) != 1 {
					t.Fatal("original event not appended once")
				}
			}
			// Even the successful branch feeds no Commit return value into the
			// component. From its perspective that outcome is still unauthenticated.
			if r := binding.StatementReport(); r.HostCommitVerified || r.BusinessClosureVerified || r.SourceAuthenticated || r.DropReady {
				t.Fatal("independent visibility certified host Commit/retirement")
			}
			provenanceNativeEndRead(t, fresh)
			if !rollback {
				duplicate, e := provenanceNativeHostWrite(t, db, plan, original, false, nil)
				if e != nil {
					t.Fatal("same exact plan expected-state idempotence", e)
				}
				for _, location := range duplicate.StatementReport().Locations {
					if location.StatementWrote || !location.AlreadyMatched {
						t.Fatal("matched evidence was rewritten")
					}
				}
				fresh = provenanceNativeMustRead(t, db, plan.request)
				if e = duplicate.VerifyPersisted(fresh.ctx, fresh.batch); e != nil {
					t.Fatal("idempotent statement lost independent expected baseline", e)
				}
			}
			pool, e := db.DB()
			if e != nil || pool.PingContext(t.Context()) != nil {
				t.Fatal("component closed host pool")
			}
		})
	}
}

func TestSQLCASProvenanceNativeOriginalReadAndApplyEpochMustEnd(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	read, plan, original := provenanceNativePrepared(t, db, "provenance-active-1")
	sentinel := errors.New("host rollback denied unended-original binding")
	err := db.Transaction(func(tx *gorm.DB) error {
		ctx := hostmysql.WithTx(t.Context(), tx)
		statement, err := plan.Apply(ctx)
		if err != nil {
			return err
		}
		if bound, err := original.BindStatement(ctx, statement); bound != nil || !errors.Is(err, ErrSQLHistoricalCASProvenance) {
			t.Fatal("active original read produced Apply provenance", err)
		}
		return sentinel
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if !errors.Is(err, sentinel) {
		t.Fatal("native denied-bind branch was not reached", err)
	}
	if err := read.batch.ValidateBorrowedSnapshot(read.ctx); err != nil {
		t.Fatal("denied Bind closed original borrowed epoch", err)
	}
	provenanceNativeEndRead(t, read)
	if storedHistoricalSet(t, db, "assessment", "historical_lifecycle_evidence", 42) != nil {
		t.Fatal("test host rollback leaked denied-bind statement")
	}
	binding, err := provenanceNativeHostWrite(t, db, plan, original, false, func(writeCtx context.Context, bound *SQLHistoricalCASStatementBinding) {
		if err := bound.VerifyPersisted(writeCtx, read.batch); !errors.Is(err, ErrSQLHistoricalCASProvenance) {
			t.Fatal("same actual write Tx became fresh RRRO", err)
		}
		fresh := provenanceNativeMustRead(t, db, plan.request)
		if err := bound.VerifyPersisted(fresh.ctx, fresh.batch); !errors.Is(err, ErrSQLHistoricalCASProvenance) {
			t.Fatal("independent read while original Apply still active was accepted", err)
		}
		provenanceNativeEndRead(t, fresh)
	})
	if err != nil {
		t.Fatal(err)
	}
	fresh := provenanceNativeMustRead(t, db, plan.request)
	if err = binding.VerifyPersisted(fresh.ctx, fresh.batch); err != nil {
		t.Fatal("properly ended write could not be independently read", err)
	}
}

func TestSQLCASProvenanceNativeIndependentReadbackRejectsRawAndSchemaDrift(t *testing.T) {
	for _, test := range []struct {
		name, query string
		changedRow  bool
	}{
		{"physical_null", "UPDATE assessment SET conducting_context=CAST('null' AS JSON) WHERE id=42", true},
		{"nullable_empty", "UPDATE assessment SET evaluation_model_code='' WHERE id=42", true},
		{"version", "UPDATE assessment SET version=version+1 WHERE id=42", true},
		{"millisecond", "UPDATE runtime_checkpoint SET finished_at=DATE_ADD(finished_at,INTERVAL 1000 MICROSECOND) WHERE assessment_id=42", true},
		{"future_column", "ALTER TABLE assessment ADD COLUMN provenance_future_column VARCHAR(20) NULL", false},
		{"clock_schema", "ALTER TABLE runtime_checkpoint MODIFY finished_at DATETIME NULL", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := openHistoricalReferencesDB(t)
			insertHistoricalAssessment(t, db, 42)
			read, plan, original := provenanceNativePrepared(t, db, "provenance-drift-1")
			provenanceNativeEndRead(t, read)
			binding, err := provenanceNativeHostWrite(t, db, plan, original, false, nil)
			if err != nil {
				t.Fatal(err)
			}
			before := provenanceNativeMustRead(t, db, plan.request)
			if err = binding.VerifyPersisted(before.ctx, before.batch); err != nil {
				t.Fatal("fixture was not an exact committed baseline before adversarial mutation", err)
			}
			provenanceNativeEndRead(t, before)
			result := db.Exec(test.query)
			if result.Error != nil || test.changedRow && result.RowsAffected != 1 {
				t.Fatal("adversarial fixture mutation was not performed exactly once", result.Error, result.RowsAffected)
			}
			fresh, err := provenanceNativeRead(t, db, plan.request)
			if err == nil && binding.VerifyPersisted(fresh.ctx, fresh.batch) == nil {
				t.Fatal("changed raw NULL/clock/column/schema was exempted from persisted expected baseline")
			}
			if r := binding.StatementReport(); r.HostCommitVerified || r.DropReady {
				t.Fatal("drift granted full retirement qualification")
			}
		})
	}
}

func TestSQLCASProvenanceNativeUnchangedDependencyIncludesAbsenceAndNULL(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	request := SQLHistoricalOwnerBatchRequest{AnswerSheetIDs: []uint64{10042, 99999}}
	read := provenanceNativeMustRead(t, db, request)
	baseline, err := SealSQLHistoricalCASReadBaseline(read.ctx, read.batch)
	if err != nil {
		t.Fatal(err)
	}
	if err = baseline.VerifyUnchanged(read.ctx, read.batch); !errors.Is(err, ErrSQLHistoricalCASProvenance) {
		t.Fatal("same actual read snapshot passed dependency freshness", err)
	}
	provenanceNativeEndRead(t, read)
	fresh := provenanceNativeMustRead(t, db, request)
	if err = baseline.VerifyUnchanged(fresh.ctx, fresh.batch); err != nil {
		t.Fatal("independent unchanged raw dependency/absence was rejected", err)
	}
	provenanceNativeEndRead(t, fresh)
	mutation := db.Exec("UPDATE assessment SET answer_sheet_id=99999 WHERE id=42")
	if mutation.Error != nil || mutation.RowsAffected != 1 {
		t.Fatal("actual missing-range mutation not prepared", mutation.Error, mutation.RowsAffected)
	}
	fresh, err = provenanceNativeRead(t, db, request)
	if err == nil && baseline.VerifyUnchanged(fresh.ctx, fresh.batch) == nil {
		t.Fatal("new association in a formerly absent selector was ignored")
	}
}
