package statisticsv2

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	appv2 "github.com/FangcunMount/qs-server/internal/apiserver/application/statisticsv2"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func newReadStoreTestDB(t *testing.T) (*ReadStore, sqlmock.Sqlmock) {
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
	return NewReadStore(db, nil), mock
}

func TestLatestVisibleSnapshotBlocksDatabaseAfterUnpublishedRepair(t *testing.T) {
	store, mock := newReadStoreTestDB(t)
	asOf := time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)
	snapshotAt := time.Date(2026, 7, 22, 0, 30, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id,as_of_date,COALESCE(data_committed_at,finished_at,started_at) snapshot_at,cache_generation")).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "as_of_date", "snapshot_at", "cache_generation"}).AddRow(10, asOf, snapshotAt, 4))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM statistics_sync_run").
		WithArgs(int64(7), uint64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	snapshot, err := store.LatestVisibleSnapshot(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot == nil || snapshot.DatabaseReadable || snapshot.CacheGeneration != 4 {
		t.Fatalf("snapshot=%+v", snapshot)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLatestVisibleSnapshotAllowsPublishedDatabaseGeneration(t *testing.T) {
	store, mock := newReadStoreTestDB(t)
	asOf := time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)
	snapshotAt := time.Date(2026, 7, 22, 0, 30, 0, 0, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id,as_of_date,COALESCE(data_committed_at,finished_at,started_at) snapshot_at,cache_generation")).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "as_of_date", "snapshot_at", "cache_generation"}).AddRow(11, asOf, snapshotAt, 5))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM statistics_sync_run").
		WithArgs(int64(7), uint64(11)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	snapshot, err := store.LatestVisibleSnapshot(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot == nil || !snapshot.DatabaseReadable || snapshot.CacheGeneration != 5 {
		t.Fatalf("snapshot=%+v", snapshot)
	}
}

func TestContentBatchUsesPublishedAsOfDate(t *testing.T) {
	store, mock := newReadStoreTestDB(t)
	asOf := time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)
	mock.ExpectQuery("SELECT COALESCE\\(SUM\\(answersheet_submitted_count\\),0\\) total_submissions").
		WithArgs(int64(7), asOf, "Q-1").
		WillReturnRows(sqlmock.NewRows([]string{"total_submissions"}).AddRow(12))

	items, err := store.ContentBatch(context.Background(), 7, asOf, []appv2.ContentRef{{Kind: "questionnaire", Code: "Q-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].TotalSubmissions != 12 || items[0].HasCompletion {
		t.Fatalf("items=%+v", items)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
