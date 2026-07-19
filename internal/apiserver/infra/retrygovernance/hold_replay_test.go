package retrygovernance

import (
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestRetryHoldManualReplayIsOrgScopedAndDoesNotResetAttemptCount(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	reader := &Reader{mysql: db}
	now := time.Date(2026, 7, 19, 2, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `retry_event_hold` WHERE event_id = ? ORDER BY `retry_event_hold`.`id` LIMIT ? FOR UPDATE")).
		WithArgs("event-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "event_id", "org_id", "status", "retry_disposition", "replay_attempt_count"}).
			AddRow(7, "event-1", 9, "failed", "manual_required", 30))
	mock.ExpectExec("UPDATE `retry_event_hold` SET").
		WithArgs("request-1", now, "automatic", now, 7).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	results, err := reader.AuthorizeManualReplay(t.Context(), 9, "request-1", []outboxport.ManualReplayTarget{{EventID: "event-1", ExpectedAttemptCount: 30}}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || !results[0].Authorized {
		t.Fatalf("results=%#v", results)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRetryHoldManualReplayRejectsOtherOrganization(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	reader := &Reader{mysql: db}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*retry_event_hold.*FOR UPDATE").
		WithArgs("event-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "event_id", "org_id", "status", "retry_disposition", "replay_attempt_count"}).
			AddRow(7, "event-1", 10, "failed", "manual_required", 30))
	mock.ExpectCommit()
	results, err := reader.AuthorizeManualReplay(t.Context(), 9, "request-1", []outboxport.ManualReplayTarget{{EventID: "event-1", ExpectedAttemptCount: 30}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Authorized || results[0].Reason != "organization_mismatch" {
		t.Fatalf("results=%#v", results)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
