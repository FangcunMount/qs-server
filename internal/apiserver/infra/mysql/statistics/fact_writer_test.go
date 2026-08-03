package statistics

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	statisticsDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func newFactWriterTestDB(t *testing.T) (factWriter, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return factWriter{db: db}, mock
}

func testAccessFact(key string, at time.Time) map[string]any {
	return baseFact(1, key, "entry_opened", at, "entry_resolve", key)
}

func TestFactWriterBatchCollapsesMissingFactsIntoOneInsert(t *testing.T) {
	writer, mock := newFactWriterTestDB(t)
	at := time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC)
	facts := []map[string]any{testAccessFact("fact-1", at), testAccessFact("fact-2", at)}
	mock.ExpectQuery("SELECT fact_key,core_hash FROM .*statistics_access_fact.*fact_key IN \\(\\?,\\?\\)").
		WithArgs("fact-1", "fact-2").
		WillReturnRows(sqlmock.NewRows([]string{"fact_key", "core_hash"}))
	mock.ExpectExec("^" + regexp.QuoteMeta("INSERT INTO `statistics_access_fact`")).
		WillReturnResult(sqlmock.NewResult(0, 2))

	dispositions, err := writer.writeBatch(context.Background(), "statistics_access_fact", facts, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(dispositions) != 2 || dispositions[0] != factWriteInserted || dispositions[1] != factWriteInserted {
		t.Fatalf("dispositions=%v", dispositions)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFactWriterBatchUsesTwoDatabaseRoundTripsForFullCollectorPage(t *testing.T) {
	writer, mock := newFactWriterTestDB(t)
	at := time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC)
	facts := make([]map[string]any, 0, collectorBatchSize)
	for index := 0; index < collectorBatchSize; index++ {
		facts = append(facts, testAccessFact("fact-"+time.Unix(int64(index), 0).Format("150405.000000000"), at))
	}
	mock.ExpectQuery("SELECT fact_key,core_hash FROM .*statistics_access_fact").
		WillReturnRows(sqlmock.NewRows([]string{"fact_key", "core_hash"}))
	mock.ExpectExec("^" + regexp.QuoteMeta("INSERT INTO `statistics_access_fact`")).
		WillReturnResult(sqlmock.NewResult(0, collectorBatchSize))

	dispositions, err := writer.writeBatch(context.Background(), "statistics_access_fact", facts, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(dispositions) != collectorBatchSize {
		t.Fatalf("dispositions=%d", len(dispositions))
	}
	for index, disposition := range dispositions {
		if disposition != factWriteInserted {
			t.Fatalf("disposition[%d]=%v", index, disposition)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFactWriterBatchValidateRemainsReadOnly(t *testing.T) {
	writer, mock := newFactWriterTestDB(t)
	facts := []map[string]any{testAccessFact("fact-1", time.Now())}
	mock.ExpectQuery("SELECT fact_key,core_hash FROM .*statistics_access_fact.*fact_key IN \\(\\?\\)").
		WithArgs("fact-1").
		WillReturnRows(sqlmock.NewRows([]string{"fact_key", "core_hash"}))

	dispositions, err := writer.writeBatch(context.Background(), "statistics_access_fact", facts, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(dispositions) != 1 || dispositions[0] != factWriteInserted {
		t.Fatalf("dispositions=%v", dispositions)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFactWriterBatchPreservesExistingAndConflictSemantics(t *testing.T) {
	writer, mock := newFactWriterTestDB(t)
	at := time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC)
	facts := []map[string]any{testAccessFact("fact-1", at), testAccessFact("fact-2", at)}
	existingHash := hashCore(facts[0])
	mock.ExpectQuery("SELECT fact_key,core_hash FROM .*statistics_access_fact.*fact_key IN \\(\\?,\\?\\)").
		WithArgs("fact-1", "fact-2").
		WillReturnRows(sqlmock.NewRows([]string{"fact_key", "core_hash"}).
			AddRow("fact-1", existingHash).
			AddRow("fact-2", "different"))

	dispositions, err := writer.writeBatch(context.Background(), "statistics_access_fact", facts, false)
	if err == nil || err.Error() != "fact conflict: fact-2" {
		t.Fatalf("err=%v", err)
	}
	if dispositions[0] != factWriteExisting || dispositions[1] != factWriteConflict {
		t.Fatalf("dispositions=%v", dispositions)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFactWriterLegacyBatchTreatsExistingKeyAsImmutableWithoutRehashingMutableSource(t *testing.T) {
	writer, mock := newFactWriterTestDB(t)
	fact := testAccessFact("task:42:task_created", time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC))
	fact["planned_at"] = time.Date(2026, 8, 1, 8, 0, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT .*fact_key.* FROM .*statistics_plan_fact.*fact_key IN \\(\\?\\)").
		WithArgs("task:42:task_created").
		WillReturnRows(sqlmock.NewRows([]string{"fact_key"}).AddRow("task:42:task_created"))

	dispositions, err := writer.writeBatchByKey(context.Background(), "statistics_plan_fact", []map[string]any{fact}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(dispositions) != 1 || dispositions[0] != factWriteExisting {
		t.Fatalf("dispositions=%v", dispositions)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestWriteFactCandidatesCountsEachSourceOnce(t *testing.T) {
	writer, mock := newFactWriterTestDB(t)
	at := time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC)
	candidates := []factCandidate{
		{SourceID: 10, FactType: "intake_confirmed", Values: testAccessFact("fact-1", at)},
		{SourceID: 10, FactType: "care_relationship_established", Values: testAccessFact("fact-2", at)},
	}
	mock.ExpectQuery("SELECT fact_key,core_hash FROM .*statistics_access_fact.*fact_key IN \\(\\?,\\?\\)").
		WithArgs("fact-1", "fact-2").
		WillReturnRows(sqlmock.NewRows([]string{"fact_key", "core_hash"}))
	mock.ExpectExec("^" + regexp.QuoteMeta("INSERT INTO `statistics_access_fact`")).
		WillReturnResult(sqlmock.NewResult(0, 2))
	result := statisticsDomain.CollectResult{Collector: "access", FactTypeCounts: map[string]int64{}}

	if err := writeFactCandidates(context.Background(), writer, "statistics_access_fact", candidates, false, &result); err != nil {
		t.Fatal(err)
	}
	if result.SourceCount != 1 || result.InsertedCount != 2 || result.FactTypeCounts["intake_confirmed"] != 1 || result.FactTypeCounts["care_relationship_established"] != 1 {
		t.Fatalf("result=%+v", result)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
