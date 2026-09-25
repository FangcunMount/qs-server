//go:build integration

package eventdelivery

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
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

func TestDeliveryReplaySettlementWriteFailureKeepsClaimForReconciliation(t *testing.T) {
	for _, tc := range []struct {
		name        string
		disposition string
	}{
		{name: "completion", disposition: "terminal"},
		{name: "failure", disposition: "manual_required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openDeliveryReplayMySQL(t)
			now := time.Now().UTC().Truncate(time.Millisecond)
			if err := db.Exec(`INSERT INTO event_delivery_dead_letter
				(id,message_id,event_id,org_id,delivery_attempts,payload_json,retry_disposition,updated_at)
				VALUES (1,'m1','e1',7,8,'{}','manual_required',?)`, now).Error; err != nil {
				t.Fatal(err)
			}
			store := NewStore(db)
			target := app.DeliveryReplayTarget{ID: 1, ExpectedDeliveryAttempts: 8}
			if _, err := store.AuthorizeReplay(t.Context(), 7, "replay-1", []app.DeliveryReplayTarget{target}, now); err != nil {
				t.Fatal(err)
			}
			// The trigger fails a real MySQL state transition after the claim has committed.
			if err := db.Exec(`CREATE TRIGGER reject_replay_settlement BEFORE UPDATE ON event_delivery_dead_letter
				FOR EACH ROW BEGIN
					IF NEW.retry_disposition = '` + tc.disposition + `' THEN
						SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'injected settlement write failure';
					END IF;
				END`).Error; err != nil {
				t.Fatal(err)
			}
			if tc.name == "completion" {
				if err := store.CompleteReplay(t.Context(), 1, "replay-1", now.Add(time.Second)); err == nil {
					t.Fatal("completion write should fail at MySQL")
				}
				if err := store.RecordReplayUncertain(t.Context(), 1, "replay-1", "completion outcome unknown", now.Add(time.Second)); err != nil {
					t.Fatal(err)
				}
				checkDisposition(t, db, 1, "automatic", "replay-1", "completion outcome unknown")
			} else {
				if err := store.FailReplay(t.Context(), 1, "replay-1", "payload decode failed", now.Add(time.Second)); err == nil {
					t.Fatal("failure-state write should fail at MySQL")
				}
				checkDisposition(t, db, 1, "automatic", "replay-1", "")
			}
			if _, err := store.AuthorizeReplay(t.Context(), 7, "replay-2", []app.DeliveryReplayTarget{target}, now.Add(2*time.Second)); err == nil {
				t.Fatal("settlement write failure must not release the claimed event for another replay")
			}
		})
	}
}

func TestDeliveryReplayClaimSurvivesProcessExit(t *testing.T) {
	if dsn := os.Getenv("QS_REPLAY_CLAIM_HELPER_DSN"); dsn != "" {
		db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
		if err != nil {
			t.Fatal(err)
		}
		store := NewStore(db)
		_, err = store.AuthorizeReplay(t.Context(), 7, "crashed-request", []app.DeliveryReplayTarget{{ID: 1, ExpectedDeliveryAttempts: 8}}, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		// Simulate a process dying after the MySQL claim commits, before publish.
		os.Exit(0)
	}

	db := openDeliveryReplayMySQL(t)
	if err := db.Exec(`INSERT INTO event_delivery_dead_letter
		(id,message_id,event_id,org_id,delivery_attempts,payload_json,retry_disposition,updated_at)
		VALUES (1,'m1','e1',7,8,'{}','manual_required',?)`, time.Now()).Error; err != nil {
		t.Fatal(err)
	}
	var databaseName string
	if err := db.Raw("SELECT DATABASE()").Scan(&databaseName).Error; err != nil {
		t.Fatal(err)
	}
	cfg, err := drivermysql.ParseDSN(os.Getenv("MYSQL_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.DBName = databaseName
	cfg.ParseTime = true
	child := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestDeliveryReplayClaimSurvivesProcessExit$")
	child.Env = append(os.Environ(), "QS_REPLAY_CLAIM_HELPER_DSN="+cfg.FormatDSN())
	if output, err := child.CombinedOutput(); err != nil {
		t.Fatalf("claiming process failed: %v\n%s", err, output)
	}
	checkDisposition(t, db, 1, "automatic", "crashed-request", "")
	store := NewStore(db)
	if _, err := store.AuthorizeReplay(t.Context(), 7, "new-request", []app.DeliveryReplayTarget{{ID: 1, ExpectedDeliveryAttempts: 8}}, time.Now()); err == nil {
		t.Fatal("a new request must not reclaim an event left by an exited process")
	}
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
