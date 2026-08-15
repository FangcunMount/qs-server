package attentionprojection

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestMySQLProjectionPORoundTrip(t *testing.T) {
	at := time.Date(2026, 8, 15, 11, 0, 0, 789000000, time.UTC)
	po := mysqlProjectionPO{
		EventID: "evt-1", ReportID: "101", AssessmentID: "202", TesteeID: 303,
		RiskLevel: "high", MarkKeyFocus: true, Status: StatusFailed, Attempt: 4,
		LastError: "temporary", CreatedAt: at.Add(-time.Minute), UpdatedAt: at,
	}
	record := mysqlPOToRecord(&po)
	if record.EventID != po.EventID || record.Status != StatusFailed || record.Attempt != 4 {
		t.Fatalf("round trip record = %#v", record)
	}
	if !record.MarkKeyFocus || !record.UpdatedAt.Equal(at) || record.LastError != "temporary" {
		t.Fatalf("round trip lost projection state: %#v", record)
	}
}

func TestMySQLStoreRecordFailureLocksAndMovesToManualRequired(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `interpretation_attention_projection`")).
		WillReturnRows(sqlmock.NewRows([]string{"event_id", "attempt", "status", "created_at", "updated_at"}).
			AddRow("evt-1", 0, string(StatusPending), now, now))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `interpretation_attention_projection` SET")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	status, err := (&MySQLStore{db: db}).RecordFailure(context.Background(), "evt-1", "downstream unavailable", 1)
	if err != nil {
		t.Fatal(err)
	}
	if status != StatusManualRequired {
		t.Fatalf("status = %s, want %s", status, StatusManualRequired)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
