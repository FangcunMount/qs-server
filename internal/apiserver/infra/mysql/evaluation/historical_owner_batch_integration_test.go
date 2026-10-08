//go:build integration

package evaluation

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type batchNativeLogger struct {
	queries, responsibility int
}

func (l *batchNativeLogger) LogMode(logger.LogLevel) logger.Interface { return l }
func (*batchNativeLogger) Info(context.Context, string, ...any)       {}
func (*batchNativeLogger) Warn(context.Context, string, ...any)       {}
func (*batchNativeLogger) Error(context.Context, string, ...any)      {}
func (l *batchNativeLogger) Trace(_ context.Context, _ time.Time, query func() (string, int64), _ error) {
	l.queries++
	text, _ := query()
	text = strings.ToLower(strings.ReplaceAll(text, "`", ""))
	for _, spec := range sqlResponsibilityTables {
		if strings.Contains(text, "from "+spec.name+" ") {
			l.responsibility++
		}
	}
}

func batchNativeTx(t *testing.T, db *gorm.DB, f func(context.Context, *SQLHistoricalResponsibilityCycle) error) error {
	t.Helper()
	return db.Transaction(func(tx *gorm.DB) error {
		ctx := hostmysql.WithTx(t.Context(), tx)
		server, database, err := historicalDatabase(tx)
		if err != nil {
			return err
		}
		cycle, err := PrepareSQLHistoricalResponsibilityCycle(ctx, sqlHistoricalIdentity(server, database), DefaultSQLResponsibilityLimits())
		if err != nil {
			return err
		}
		return f(ctx, cycle)
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
}

func TestSQLHistoricalOwnerBatchActualSharedROAndZeroLedgerRescanNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	for id := uint64(42); id < 74; id++ {
		insertHistoricalAssessment(t, db, id)
	}
	if err := db.Exec("UPDATE assessment SET conducting_context=CAST(? AS JSON) WHERE id=42", `{"frozen":"actual server JSON"}`).Error; err != nil {
		t.Fatal(err)
	}
	cycleNativeInsert(t, db, cycleTestMessage(t, "current-request", "evaluation.requested", "Evaluation", "42", cycleRequestedPayload()))
	if err := db.Exec("INSERT INTO qs_rm_evaluation_request_ref(event_id,assessment_id,org_id) VALUES('current-request',42,7)").Error; err != nil {
		t.Fatal(err)
	}
	if err := batchNativeTx(t, db, func(ctx context.Context, cycle *SQLHistoricalResponsibilityCycle) error {
		tx, err := historicalTx(ctx)
		if err != nil {
			return err
		}
		trace := &batchNativeLogger{}
		ctx = hostmysql.WithTx(ctx, tx.Session(&gorm.Session{Logger: trace}))
		request := SQLHistoricalOwnerBatchRequest{AnswerSheetIDs: []uint64{10042, 99999}}
		for id := uint64(42); id < 74; id++ {
			request.AssessmentIDs = append(request.AssessmentIDs, id)
		}
		batch, err := PrepareSQLHistoricalOwnerBatch(ctx, cycle, request, DefaultSQLHistoricalOwnerBatchLimits())
		if err != nil {
			return err
		}
		before := trace.queries
		for i := 0; i < 10000; i++ {
			facts, e := batch.OwnerByAssessment(42)
			if e != nil || len(facts.Snapshot().Runs) != 1 || len(facts.Snapshot().Responsibilities) != 2 || facts.HasVerifiedAnswerSheetAssociation(10042) {
				t.Fatal("indexed business facts or original point capability changed", e)
			}
		}
		sheet, err := batch.OwnerByAnswerSheet(10042)
		if err != nil || !sheet.HasVerifiedAnswerSheetAssociation(10042) || sheet.HasVerifiedAnswerSheetAssociation(10043) {
			t.Fatal("unique association not authenticated")
		}
		if _, err = batch.OwnerByAnswerSheet(99999); !errors.Is(err, ErrSQLHistoricalOwnerAbsent) {
			t.Fatal("actual absence converted into Admission")
		}
		copy := sheet.Snapshot()
		copy.Owner.ConductingContextBytes[0] = '['
		if sheet.Snapshot().Owner.ConductingContextBytes[0] == '[' {
			t.Fatal("private original JSON baseline was editable")
		}
		report := batch.Report()
		if trace.responsibility != 0 || trace.queries != before || report.OwnerCount != 32 || report.AnswerSheetAbsentCount != 1 || !report.Complete || report.DropReady || !report.SourceAuthenticationRequired || !report.ExternalBusinessClosureRequired {
			t.Fatal("bounded facts rescanned responsibility ledgers or claimed closure")
		}
		if err = batch.RecheckBusiness(ctx, cycle); !errors.Is(err, ErrSQLHistoricalBatchInvalid) {
			t.Fatal("same actual snapshot accepted as fresh business recheck")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	t.Log("32 original owners read as a bounded indexed page in the actual shared RR-RO transaction; 10k lookups execute zero SQL; all eight ledgers reused from opaque cycle")
}

func TestSQLHistoricalOwnerBatchUniqueGlobalAssociationNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	if err := db.Exec("UPDATE assessment SET org_id=8 WHERE id=42").Error; err != nil {
		t.Fatal(err)
	}
	if err := batchNativeTx(t, db, func(ctx context.Context, cycle *SQLHistoricalResponsibilityCycle) error {
		batch, err := PrepareSQLHistoricalOwnerBatch(ctx, cycle, SQLHistoricalOwnerBatchRequest{AnswerSheetIDs: []uint64{10042}}, DefaultSQLHistoricalOwnerBatchLimits())
		if err != nil {
			return err
		}
		facts, err := batch.OwnerByAnswerSheet(10042)
		if err != nil || facts.Snapshot().Owner.OrgID != 8 {
			t.Fatal("original cross-organization owner hidden")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("ALTER TABLE assessment DROP INDEX uk_answer_sheet_id").Error; err != nil {
		t.Fatal(err)
	}
	insertHistoricalAssessment(t, db, 43)
	if err := db.Exec("UPDATE assessment SET answer_sheet_id=10042 WHERE id=43").Error; err != nil {
		t.Fatal(err)
	}
	if err := batchNativeTx(t, db, func(ctx context.Context, cycle *SQLHistoricalResponsibilityCycle) error {
		batch, err := PrepareSQLHistoricalOwnerBatch(ctx, cycle, SQLHistoricalOwnerBatchRequest{AnswerSheetIDs: []uint64{10042}}, DefaultSQLHistoricalOwnerBatchLimits())
		if batch != nil || !errors.Is(err, ErrSQLHistoricalBatchConflict) {
			t.Fatal("ambiguous sheet without true unique index accepted", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSQLHistoricalOwnerBatchFreshAllColumnsAndPrecisionNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	if err := db.Exec("ALTER TABLE assessment ADD COLUMN future_owner_field VARBINARY(10) NULL").Error; err != nil {
		t.Fatal(err)
	}
	var original *SQLHistoricalOwnerBatch
	if err := batchNativeTx(t, db, func(ctx context.Context, cycle *SQLHistoricalResponsibilityCycle) error {
		var err error
		original, err = PrepareSQLHistoricalOwnerBatch(ctx, cycle, SQLHistoricalOwnerBatchRequest{AssessmentIDs: []uint64{42}, AnswerSheetIDs: []uint64{10042}}, DefaultSQLHistoricalOwnerBatchLimits())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := batchNativeTx(t, db, func(ctx context.Context, fresh *SQLHistoricalResponsibilityCycle) error {
		return original.RecheckBusiness(ctx, fresh)
	}); err != nil {
		t.Fatal("new actual snapshot of unchanged business failed", err)
	}
	for _, mutation := range []string{"UPDATE assessment SET future_owner_field='' WHERE id=42", "UPDATE assessment SET future_owner_field=NULL,version=version+1 WHERE id=42", "ALTER TABLE assessment MODIFY submitted_at DATETIME(3) NULL"} {
		if err := db.Exec(mutation).Error; err != nil {
			t.Fatal("owned business mutation failed", err)
		}
		if err := batchNativeTx(t, db, func(ctx context.Context, fresh *SQLHistoricalResponsibilityCycle) error {
			if err := original.RecheckBusiness(ctx, fresh); !errors.Is(err, ErrSQLHistoricalBatchConflict) {
				t.Fatal("NULL/future/version/precision mutation not detected", err)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSQLHistoricalOwnerBatchExactOutcomePOAndOriginalRunNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	record, _ := testCommittedReference(t, 9001, 42, "current-outcome-native")
	po := outcomeToPO(record)
	po.CommittedEventID, po.CommittedEventEvidence = nil, nil
	if err := db.Create(po).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE runtime_checkpoint SET finished_at=? WHERE assessment_id=42", record.EvaluatedAt().Truncate(time.Millisecond)).Error; err != nil {
		t.Fatal(err)
	}
	if err := batchNativeTx(t, db, func(ctx context.Context, cycle *SQLHistoricalResponsibilityCycle) error {
		batch, err := PrepareSQLHistoricalOwnerBatch(ctx, cycle, SQLHistoricalOwnerBatchRequest{AssessmentIDs: []uint64{42}}, DefaultSQLHistoricalOwnerBatchLimits())
		if err != nil {
			return err
		}
		facts, err := batch.OwnerByAssessment(42)
		if err != nil {
			return err
		}
		actual, err := facts.OutcomeRecord(9001)
		if err != nil || actual.RunID() != "42:1" || actual.AssessmentID().Uint64() != 42 || actual.OrgID() != 7 || actual.Model().Code != record.Model().Code || len(facts.Snapshot().Outcomes) != 1 {
			t.Fatal("exact validated original Outcome/Run/model not preserved", err)
		}
		if _, err := facts.OutcomeRecord(9002); err == nil {
			t.Fatal("unobserved or latest Outcome substituted")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE evaluation_outcome SET org_id=8 WHERE id=9001").Error; err != nil {
		t.Fatal(err)
	}
	if err := batchNativeTx(t, db, func(ctx context.Context, cycle *SQLHistoricalResponsibilityCycle) error {
		batch, err := PrepareSQLHistoricalOwnerBatch(ctx, cycle, SQLHistoricalOwnerBatchRequest{AssessmentIDs: []uint64{42}}, DefaultSQLHistoricalOwnerBatchLimits())
		if batch != nil || !errors.Is(err, ErrSQLHistoricalBatchConflict) {
			t.Fatal("cross-organization Outcome accepted", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSQLHistoricalOwnerBatchCapsAndClockUnknownNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	if err := batchNativeTx(t, db, func(ctx context.Context, cycle *SQLHistoricalResponsibilityCycle) error {
		for _, limits := range []SQLHistoricalOwnerBatchLimits{{MaxOwners: 1, MaxRows: 1, MaxBytes: 64 << 20}, {MaxOwners: 1, MaxRows: 100, MaxBytes: 1}} {
			batch, err := PrepareSQLHistoricalOwnerBatch(ctx, cycle, SQLHistoricalOwnerBatchRequest{AssessmentIDs: []uint64{42}}, limits)
			if batch != nil || err == nil {
				t.Fatal("partial budget became a complete owner page")
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("ALTER TABLE runtime_checkpoint MODIFY finished_at DATETIME NULL").Error; err != nil {
		t.Fatal(err)
	}
	if err := batchNativeTx(t, db, func(ctx context.Context, cycle *SQLHistoricalResponsibilityCycle) error {
		batch, err := PrepareSQLHistoricalOwnerBatch(ctx, cycle, SQLHistoricalOwnerBatchRequest{AssessmentIDs: []uint64{42}}, DefaultSQLHistoricalOwnerBatchLimits())
		if batch != nil || !errors.Is(err, ErrSQLHistoricalBatchInvalid) {
			t.Fatal("original Run precision gap promoted to milliseconds proof", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
