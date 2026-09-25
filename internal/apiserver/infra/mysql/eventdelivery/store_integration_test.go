//go:build integration

package eventdelivery

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	drivermysql "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestDeliveryReplayClaimsAreScopedAndUncertainClaimsAreNotResent(t *testing.T) {
	db := openDeliveryReplayMySQL(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	if err := db.Exec(`INSERT INTO event_delivery_dead_letter
		(id,message_id,event_id,org_id,delivery_attempts,payload_json,retry_disposition,updated_at) VALUES
		(1,'m1','e1',7,8,'{}','manual_required',?),
		(2,'m2','e2',7,8,'{}','manual_required',?),
		(3,'m3','e3',8,8,'{}','manual_required',?),
		(4,'m4','e4',7,8,'{}','archived_mock',?)`, now, now, now, now).Error; err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	target := func(id uint64) app.DeliveryReplayTarget {
		return app.DeliveryReplayTarget{ID: id, ExpectedDeliveryAttempts: 8}
	}
	if err := store.ValidateReplayBatch(t.Context(), 7, []app.DeliveryReplayTarget{target(1), target(3)}); err == nil {
		t.Fatal("cross-organization batch should fail before any claim")
	}
	if err := store.ValidateReplayBatch(t.Context(), 7, []app.DeliveryReplayTarget{target(4)}); err == nil {
		t.Fatal("archived mock should not be replayable")
	}
	if err := store.ValidateReplayBatch(t.Context(), 7, []app.DeliveryReplayTarget{target(1), target(2)}); err != nil {
		t.Fatal(err)
	}
	claimed, err := store.AuthorizeReplay(t.Context(), 7, "replay-1", []app.DeliveryReplayTarget{target(1)}, now)
	if err != nil || len(claimed) != 1 || claimed[0].EventID != "e1" {
		t.Fatalf("claim = %#v, %v", claimed, err)
	}
	if err := store.RecordReplayUncertain(t.Context(), 1, "replay-1", "publish outcome unknown", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AuthorizeReplay(t.Context(), 7, "replay-2", []app.DeliveryReplayTarget{target(1)}, now.Add(time.Second)); err == nil {
		t.Fatal("uncertain original event must not be resent with a new request")
	}
	checkDisposition(t, db, 1, "automatic", "replay-1", "publish outcome unknown")
	checkDisposition(t, db, 2, "manual_required", "", "")
	checkDisposition(t, db, 3, "manual_required", "", "")
	checkDisposition(t, db, 4, "archived_mock", "", "")
	if err := store.CompleteReplay(t.Context(), 1, "replay-1", now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	checkDisposition(t, db, 1, "terminal", "replay-1", "publish outcome unknown")
	if _, err := store.AuthorizeReplay(t.Context(), 7, "replay-3", []app.DeliveryReplayTarget{target(2)}, now); err != nil {
		t.Fatal(err)
	}
	if err := store.FailReplay(t.Context(), 2, "replay-3", "payload decode failed", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	checkDisposition(t, db, 2, "manual_required", "replay-3", "payload decode failed")
}

func checkDisposition(t *testing.T, db *gorm.DB, id uint64, disposition, requestID, lastError string) {
	t.Helper()
	var row deadLetterPO
	if err := db.First(&row, "id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	actualRequestID, actualLastError := "", ""
	if row.ReplayRequestID != nil {
		actualRequestID = *row.ReplayRequestID
	}
	if row.LastError != nil {
		actualLastError = *row.LastError
	}
	if row.RetryDisposition != disposition || actualRequestID != requestID || actualLastError != lastError {
		t.Fatalf("delivery %d state = %s, %s, %s; want %s, %s, %s", id, row.RetryDisposition, actualRequestID, actualLastError, disposition, requestID, lastError)
	}
}

func openDeliveryReplayMySQL(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		t.Skip("MYSQL_DSN is required")
	}
	cfg, err := drivermysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.DBName = ""
	server, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	databaseName := fmt.Sprintf("qs_delivery_replay_%d", time.Now().UnixNano())
	if _, err := server.ExecContext(t.Context(), "CREATE DATABASE `"+databaseName+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = server.ExecContext(context.Background(), "DROP DATABASE IF EXISTS `"+databaseName+"`") })
	cfg.DBName = databaseName
	cfg.ParseTime = true
	db, err := gorm.Open(gormmysql.Open(cfg.FormatDSN()), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.Exec(`CREATE TABLE event_delivery_dead_letter (
		id bigint unsigned PRIMARY KEY, message_id varchar(128) NOT NULL,
		event_id varchar(64) NULL, org_id bigint NULL, delivery_attempts int NOT NULL,
		payload_json longtext NOT NULL, last_error text NULL,
		retry_disposition varchar(32) NOT NULL, replay_request_id varchar(64) NULL,
		replayed_at datetime(3) NULL, updated_at datetime(3) NOT NULL
	)`).Error; err != nil {
		t.Fatal(err)
	}
	return db
}
