package evaluation

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	evalevent "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
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
	mock.ExpectQuery("(?s)SELECT `id`,`aggregate_id`,`payload_json`,`status` FROM `domain_event_outbox`.*ORDER BY aggregate_id ASC, id DESC").
		WithArgs(eventcatalog.EvaluationOutcomeCommitted, evalevent.AggregateType, "42").
		WillReturnRows(sqlmock.NewRows([]string{"id", "aggregate_id", "payload_json", "status"}).
			AddRow(7, "42", `{"data":{"outcome_id":"9001","evaluation_run_id":"42:1"}}`, "published"))

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
