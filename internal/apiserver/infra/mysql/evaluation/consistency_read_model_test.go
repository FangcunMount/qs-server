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

func TestConsistencyReadModelReadsProjectionOutcomeLink(t *testing.T) {
	reader, mock := newConsistencyReadModelTestDB(t)
	mock.ExpectQuery("(?s)" + regexp.QuoteMeta("SELECT") + ".*" + regexp.QuoteMeta("FROM `assessment_score`")).
		WithArgs(uint64(42)).
		WillReturnRows(sqlmock.NewRows([]string{
			"row_count", "unlinked_row_count", "distinct_outcome_count", "outcome_id",
		}).AddRow(3, 0, 1, 9001))

	evidence, err := reader.FindProjectionEvidence(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if evidence == nil || evidence.RowCount != 3 || evidence.OutcomeID != "9001" || evidence.DistinctOutcomeCount != 1 {
		t.Fatalf("projection evidence = %#v", evidence)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestConsistencyReadModelReadsCommittedOutboxReferences(t *testing.T) {
	reader, mock := newConsistencyReadModelTestDB(t)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `domain_event_outbox`")).
		WithArgs(eventcatalog.EvaluationOutcomeCommitted, evalevent.AggregateType, "42").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT `payload_json`,`status` FROM `domain_event_outbox`")).
		WithArgs(eventcatalog.EvaluationOutcomeCommitted, evalevent.AggregateType, "42", 1).
		WillReturnRows(sqlmock.NewRows([]string{"payload_json", "status"}).
			AddRow(`{"data":{"outcome_id":"9001","evaluation_run_id":"42:1"}}`, "published"))

	evidence, err := reader.FindCommittedOutboxEvidence(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if evidence == nil || evidence.RowCount != 1 || evidence.OutcomeID != "9001" || evidence.RunID != "42:1" || evidence.Status != "published" {
		t.Fatalf("outbox evidence = %#v", evidence)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
