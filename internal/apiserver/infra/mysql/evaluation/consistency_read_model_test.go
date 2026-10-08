package evaluation

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	mysqlDriver "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func newConsistencyReadModelTestDB(t *testing.T) (*consistencyReadModel, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(mysqlDriver.New(mysqlDriver.Config{
		Conn: sqlDB, SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return &consistencyReadModel{db: db}, mock
}

func TestConsistencyReadModelReadsLightweightOutcomeEvidence(t *testing.T) {
	reader, mock := newConsistencyReadModelTestDB(t)
	mock.ExpectQuery("^" + regexp.QuoteMeta("SELECT `assessment_id`,`id`,`evaluation_run_id`,`model_kind` FROM `evaluation_outcome` WHERE assessment_id IN (?)") + "$").
		WithArgs(uint64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"assessment_id", "id", "evaluation_run_id", "model_kind"}).AddRow(42, 9001, "42:1", "scale"))

	evidenceByAssessment, err := reader.listOutcomeEvidence(context.Background(), []uint64{42})
	if err != nil {
		t.Fatal(err)
	}
	evidence := evidenceByAssessment[42]
	if evidence == nil || evidence.ID != "9001" || evidence.RunID != "42:1" || evidence.ModelKind != "scale" {
		t.Fatalf("outcome evidence = %#v", evidence)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestConsistencyReadModelReadsAuditEvidenceInBoundedBatches(t *testing.T) {
	reader, mock := newConsistencyReadModelTestDB(t)
	mock.ExpectQuery("(?s)SELECT `id`,`status` FROM `assessment`.*ORDER BY id ASC LIMIT \\?").
		WithArgs("submitted", "evaluated", "failed", uint64(0), 2).
		WillReturnRows(sqlmock.NewRows([]string{"id", "status"}).AddRow(42, "evaluated"))
	mock.ExpectQuery("^" + regexp.QuoteMeta("SELECT `assessment_id`,`id`,`evaluation_run_id`,`model_kind` FROM `evaluation_outcome` WHERE assessment_id IN (?)") + "$").
		WithArgs(uint64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"assessment_id", "id", "evaluation_run_id", "model_kind"}).AddRow(42, 9001, "42:1", "scale"))
	mock.ExpectQuery("(?s)SELECT `id`,`assessment_id`,`resource_id`,`status`,`lease_expires_at`,`attempt_no` FROM `runtime_checkpoint`.*ORDER BY assessment_id ASC, attempt_no DESC, id DESC").
		WithArgs("evaluation_run", uint64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "assessment_id", "resource_id", "status", "lease_expires_at", "attempt_no"}).AddRow(1, 42, "42:1", "succeeded", nil, 1))
	mock.ExpectQuery("(?s)SELECT assessment_id,.*FROM `assessment_score`.*GROUP BY `assessment_id`").
		WithArgs(uint64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"assessment_id", "row_count", "unlinked_row_count", "distinct_outcome_count", "outcome_id"}).AddRow(42, 1, 0, 1, 9001))
	// Missing historical evidence must not consult the retired ledger.
	mock.ExpectQuery("^" + regexp.QuoteMeta("SELECT evaluation_outcome.*, committed_event_id IS NULL AND committed_event_evidence IS NULL AS canonical_pair_null FROM `evaluation_outcome` WHERE assessment_id IN (?)") + "$").
		WithArgs(uint64(42)).WillReturnRows(sqlmock.NewRows([]string{"id", "assessment_id", "testee_id", "org_id", "evaluation_run_id", "model_kind", "model_code", "payload_json", "evaluated_at"}).
		AddRow(9001, 42, 21, 7, "42:1", "scale", "SCALE-1", `{}`, time.Now()))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT @@server_uuid AS server,DATABASE() AS `database`")).WillReturnRows(sqlmock.NewRows([]string{"server", "database"}).AddRow("fixture-server", "fixture-db"))
	mock.ExpectQuery("SELECT \\* FROM `assessment` WHERE id IN \\(\\?\\) LIMIT \\?").WithArgs(uint64(42), 2).WillReturnRows(sqlmock.NewRows([]string{"id", "historical_lifecycle_evidence"}).AddRow(42, nil))
	mock.ExpectQuery("SELECT \\* FROM `evaluation_outcome` WHERE assessment_id IN \\(\\?\\) LIMIT \\?").WithArgs(uint64(42), 2).WillReturnRows(sqlmock.NewRows([]string{"id", "assessment_id", "historical_committed_evidence"}).AddRow(9001, 42, nil))

	batch, err := reader.ReadBatch(context.Background(), 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Items) != 1 || !batch.CycleComplete || batch.NextCursor != 42 {
		t.Fatalf("batch = %#v", batch)
	}
	item := batch.Items[0]
	if item.Outcome == nil || item.Outcome.ID != "9001" || item.Run == nil || item.Run.ID != "42:1" || item.Projection == nil || item.Outbox == nil {
		t.Fatalf("batch item = %#v", item)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestHistoricalUnclassifiedOutcomeNeverClaimsStandardSuccess(t *testing.T) {
	reader, mock := newConsistencyReadModelTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT evaluation_outcome.*, committed_event_id IS NULL AND committed_event_evidence IS NULL AS canonical_pair_null FROM `evaluation_outcome`")).WithArgs(uint64(42)).WillReturnRows(sqlmock.NewRows([]string{"id", "assessment_id", "testee_id", "org_id", "evaluation_run_id", "model_kind", "model_code", "payload_json", "evaluated_at"}).AddRow(9001, 42, 21, 7, "42:1", "scale", "SCALE-1", `{}`, time.Now()))
	got, err := reader.listCommittedOutboxEvidence(context.Background(), []uint64{42})
	if err != nil {
		t.Fatal(err)
	}
	if got[42] == nil || got[42].InvalidReason == "" || got[42].Class != "" {
		t.Fatalf("unclassified=%#v", got[42])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
