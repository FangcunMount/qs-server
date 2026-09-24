//go:build integration

package eventdelivery

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/transport"
	drivermysql "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestArchivedMockDeadLetterRejectsReplayAndReentryIsVisible(t *testing.T) {
	dsn := os.Getenv("RM_M5_ARCHIVE_MYSQL_DSN")
	if dsn == "" {
		t.Skip("disposable RM_M5_ARCHIVE_MYSQL_DSN is required")
	}
	cfg, err := drivermysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Net != "tcp" || !strings.HasPrefix(cfg.Addr, "127.0.0.1:") || cfg.DBName != "rm_m5_archive_test" {
		t.Fatal("only a disposable local rm_m5_archive_test database is allowed")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.PingContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	ddl, err := os.ReadFile("../../../../pkg/migration/migrations/mysql/000049_add_retry_governance.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(ddl), "CREATE TABLE `event_delivery_dead_letter`")
	if start < 0 {
		t.Fatal("dead-letter migration table is missing")
	}
	end := strings.Index(string(ddl[start:]), ";")
	if end < 0 {
		t.Fatal("dead-letter migration statement is incomplete")
	}
	if _, err := db.ExecContext(t.Context(), string(ddl[start:start+end])); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DROP TABLE IF EXISTS event_delivery_dead_letter")
	})

	recorder, err := transport.NewSQLDeadLetterRecorder(db)
	if err != nil {
		t.Fatal(err)
	}
	orgID := int64(7)
	record := transport.DeadLetterRecord{
		MessageID: "m5-mock-archive-1", EventID: "m5-mock-event-1", OrgID: &orgID,
		Provider: "nsq", Topic: "qs.evaluation.lifecycle", Channel: "qs-worker",
		DeliveryAttempts: 2, Payload: []byte("{}"), FailedAt: time.Now(),
	}
	if err := recorder.RecordDeadLetter(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	var id uint64
	if err := db.QueryRowContext(t.Context(), "SELECT id FROM event_delivery_dead_letter WHERE message_id=?", record.MessageID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	result, err := db.ExecContext(t.Context(), "UPDATE event_delivery_dead_letter SET retry_disposition='archived_mock' WHERE id=? AND retry_disposition='manual_required'", id)
	if err != nil {
		t.Fatal(err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		t.Fatalf("archive affected=%d err=%v", affected, err)
	}
	gormDB, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(gormDB)
	_, err = store.AuthorizeReplay(t.Context(), orgID, "m5-archive-replay", []systemgovernance.DeliveryReplayTarget{{ID: id, ExpectedDeliveryAttempts: 2}}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "replay state conflict") {
		t.Fatalf("archived mock replay must be refused, got %v", err)
	}
	var archivedState string
	if err := db.QueryRowContext(t.Context(), "SELECT retry_disposition FROM event_delivery_dead_letter WHERE id=?", id).Scan(&archivedState); err != nil {
		t.Fatal(err)
	}
	if archivedState != "archived_mock" {
		t.Fatalf("rejected replay changed archived state to %q", archivedState)
	}

	record.DeliveryAttempts = 3
	if err := recorder.RecordDeadLetter(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	var gotID uint64
	var disposition string
	var attempts int
	if err := db.QueryRowContext(t.Context(), "SELECT id,retry_disposition,delivery_attempts FROM event_delivery_dead_letter WHERE message_id=?", record.MessageID).Scan(&gotID, &disposition, &attempts); err != nil {
		t.Fatal(err)
	}
	if gotID != id || disposition != "manual_required" || attempts != 3 {
		t.Fatalf("same-message reentry id=%d disposition=%s attempts=%d; want %d/manual_required/3", gotID, disposition, attempts, id)
	}
}
