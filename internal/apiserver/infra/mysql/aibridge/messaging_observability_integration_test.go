//go:build integration

package aibridge

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

func TestMQSnapshotCommittedStateWithoutSettlementOrInventedHistory(t *testing.T) {
	cfg, err := mysql.ParseDSN(os.Getenv("QS_AI_MQ_TEST_DSN"))
	mustMQ(t, err)
	if cfg.DBName == "" {
		t.Fatal("required disposable source schema missing")
	}
	sourceSchema := cfg.DBName
	if strings.ContainsAny(sourceSchema, "`\\") {
		t.Fatal("unsafe test schema")
	}
	source, err := sql.Open("mysql", cfg.FormatDSN())
	mustMQ(t, err)
	defer source.Close()
	name := "mq_observation_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = source.Exec("CREATE DATABASE `" + name + "`")
	mustMQ(t, err)
	defer func() {
		_, err := source.Exec("DROP DATABASE `" + name + "`")
		if err != nil {
			t.Error(err)
		}
	}()
	for _, table := range []string{"ai_messaging_outbox", "ai_messaging_inbox", "ai_messaging_quarantine", "ai_messaging_observations"} {
		_, err = source.Exec("CREATE TABLE `" + name + "`." + table + " LIKE `" + sourceSchema + "`." + table)
		mustMQ(t, err)
	}
	cfg.DBName = name
	db, err := sql.Open("mysql", cfg.FormatDSN())
	mustMQ(t, err)
	defer db.Close()
	now := time.Now().UTC()
	past := now.Add(-5 * time.Minute)
	future := now.Add(time.Minute)
	insert := func(tx *sql.Tx, id, stage string, created, available time.Time) error {
		_, err := tx.Exec(`INSERT INTO ai_messaging_outbox(producer,destination,message_id,body_sha256,body,wire,wire_sha256,kind,organization_id,topic,aggregate_key,aggregate_sequence,ordered,requires_receipt,stage,available_at,created_at) VALUES('qs-server','qs-ai',? ,?, ?,?, ?,1,1,'qs.ai.commands.v1',?,1,FALSE,TRUE,?,?,?)`, id, strings.Repeat("a", 64), []byte("synthetic state; not delivered"), []byte("synthetic state; not delivered"), strings.Repeat("b", 64), id, stage, available, created)
		return err
	}
	writer, err := db.BeginTx(t.Context(), nil)
	mustMQ(t, err)
	defer writer.Rollback()
	mustMQ(t, insert(writer, uuid.NewString(), "staged", past, past))
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	before, err := MessagingSnapshot(ctx, db)
	mustMQ(t, err)
	if before["staged_messages"] != 0 {
		t.Fatal("uncommitted message visible")
	}
	mustMQ(t, writer.Commit())
	for _, stage := range []string{"awaiting_receipt", "held", "confirmed"} {
		tx, err := db.BeginTx(t.Context(), nil)
		mustMQ(t, err)
		mustMQ(t, insert(tx, uuid.NewString(), stage, past, past))
		mustMQ(t, tx.Commit())
	}
	tx, err := db.BeginTx(t.Context(), nil)
	mustMQ(t, err)
	mustMQ(t, insert(tx, uuid.NewString(), "staged", future, future))
	mustMQ(t, tx.Commit())
	snapshot, err := MessagingSnapshot(t.Context(), db)
	mustMQ(t, err)
	if snapshot["staged_messages"] != 2 || snapshot["due_messages"] != 2 || snapshot["awaiting_receipt_messages"] != 1 || snapshot["held_messages"] != 1 || snapshot["oldest_staged_seconds"] < 300 {
		t.Fatal("stages/age conflated", snapshot)
	}
	if snapshot["duplicate_observations_available"] != 0 || snapshot["payload_error_observations_available"] != 0 {
		t.Fatal("invented observations", snapshot)
	}
	_, err = db.Exec("INSERT INTO ai_messaging_quarantine(wire_sha256,wire,code,attempts,first_seen_at,last_seen_at) VALUES(?,?,'identity_conflict',8,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", strings.Repeat("c", 64), []byte("untrusted fixture"))
	mustMQ(t, err)
	snapshot, err = MessagingSnapshot(t.Context(), db)
	mustMQ(t, err)
	if snapshot["quarantine_identity_conflict_records"] != 1 {
		t.Fatal("capped budget counted as lifetime errors", snapshot)
	}
	writer, err = db.BeginTx(t.Context(), nil)
	mustMQ(t, err)
	defer writer.Rollback()
	_, err = writer.Exec("UPDATE ai_messaging_outbox SET stage='confirmed' WHERE stage='staged'")
	mustMQ(t, err)
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	observed, err := MessagingSnapshot(ctx2, db)
	mustMQ(t, err)
	if observed["staged_messages"] != 2 {
		t.Fatal("reader waited for/observed writer")
	}
	mustMQ(t, writer.Rollback())
	mustMQ(t, db.Ping())
	_, err = db.Exec("DELETE FROM ai_messaging_outbox WHERE stage='staged' AND created_at<UTC_TIMESTAMP(6)")
	mustMQ(t, err)
	futureOnly, err := MessagingSnapshot(t.Context(), db)
	mustMQ(t, err)
	if futureOnly["staged_messages"] != 1 || futureOnly["oldest_staged_seconds"] != 0 {
		t.Fatal("future creation time was not clamped", futureOnly)
	}
	// This is the test-created schema only. A late query failure must discard
	// even already-read Outbox counts rather than expose a partial empty state.
	_, err = db.Exec("DROP TABLE ai_messaging_quarantine")
	mustMQ(t, err)
	partial, err := MessagingSnapshot(t.Context(), db)
	if err == nil || partial != nil {
		t.Fatal("failed snapshot exposed partial state", partial, err)
	}
}
