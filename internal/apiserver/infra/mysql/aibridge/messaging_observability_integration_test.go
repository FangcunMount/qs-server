//go:build integration

package aibridge

import (
	"context"
	"crypto/sha256"
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
	for _, table := range []string{"ai_messaging_operations", "ai_messaging_archive", "rm_tx_execution", "rm_tx_witness", "ai_messaging_inbox", "ai_messaging_quarantine", "ai_messaging_observations"} {
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
	insert := func(tx *sql.Tx, id, decision string, created time.Time) error {
		key := sha256.Sum256([]byte(id))
		statements := []struct {
			query string
			args  []any
		}{
			{`INSERT INTO rm_tx_execution(execution_key,request_key,owner,operation_id,writer_epoch,fence,token,plan,state,lease_until) VALUES(?,?,'qs.mysql',?,'fixture',1,?,?,'committed',?)`, []any{key[:], key[:], id, key[:], []byte("fixture"), created}},
			{`INSERT INTO rm_tx_witness(execution_key,branch_key,reference,state) VALUES(?,?,?,'committed')`, []any{key[:], key[:], []byte("fixture-reference")}},
			{`INSERT INTO ai_messaging_archive(producer,destination,message_id,organization_id,frozen_payload,first_wire,execution_key,branch_key,reference) VALUES('qs-server','qs-ai',?,1,?,?,?, ?,?)`, []any{id, []byte("fixture"), []byte("fixture"), key[:], key[:], []byte("fixture-reference")}},
			{`INSERT INTO ai_messaging_operations(command_id,kind,body_sha256,organization_id,subject_id,resource_id,aggregate_key,aggregate_sequence,decision,created_at) VALUES(?,1,?,1,'fixture',?,?,1,?,?)`, []any{id, strings.Repeat("a", 64), id, id, decision, created}},
		}
		for _, statement := range statements {
			if _, err := tx.Exec(statement.query, statement.args...); err != nil {
				return err
			}
		}
		return nil
	}
	writer, err := db.BeginTx(t.Context(), nil)
	mustMQ(t, err)
	defer writer.Rollback()
	mustMQ(t, insert(writer, uuid.NewString(), "", past))
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	before, err := MessagingSnapshot(ctx, db)
	mustMQ(t, err)
	if before["awaiting_command_receipts"] != 0 {
		t.Fatal("uncommitted message visible")
	}
	mustMQ(t, writer.Commit())
	for _, stage := range []string{"held", "accepted", "rejected"} {
		tx, err := db.BeginTx(t.Context(), nil)
		mustMQ(t, err)
		mustMQ(t, insert(tx, uuid.NewString(), stage, past))
		mustMQ(t, tx.Commit())
	}
	tx, err := db.BeginTx(t.Context(), nil)
	mustMQ(t, err)
	mustMQ(t, insert(tx, uuid.NewString(), "", future))
	mustMQ(t, tx.Commit())
	snapshot, err := MessagingSnapshot(t.Context(), db)
	mustMQ(t, err)
	if snapshot["committed_commands"] != 5 || snapshot["awaiting_command_receipts"] != 3 || snapshot["held_command_receipts"] != 1 || snapshot["oldest_awaiting_command_receipt_seconds"] < 300 {
		t.Fatal("stages/age conflated", snapshot)
	}
	if snapshot["duplicate_observations_available"] != 0 || snapshot["payload_error_observations_available"] != 0 {
		t.Fatal("invented observations", snapshot)
	}
	// Committed business rows alone cannot stand in for exact transaction evidence.
	// Every mutation below is confined to this test's randomly owned schema.
	for _, change := range []struct{ damage, restore string }{
		{"UPDATE rm_tx_execution SET state='prepared'", "UPDATE rm_tx_execution SET state='committed'"},
		{"UPDATE rm_tx_execution SET owner='other.mysql'", "UPDATE rm_tx_execution SET owner='qs.mysql'"},
		{"UPDATE rm_tx_witness SET state='aborted'", "UPDATE rm_tx_witness SET state='committed'"},
		{"UPDATE rm_tx_witness SET reference='conflicting-reference'", "UPDATE rm_tx_witness SET reference='fixture-reference'"},
	} {
		_, err = db.Exec(change.damage)
		mustMQ(t, err)
		invalid, err := MessagingSnapshot(t.Context(), db)
		if err != ErrMessagingLedgerIntegrity || invalid != nil {
			t.Fatal("unproven transaction exposed receipt counts", change.damage, invalid, err)
		}
		_, err = db.Exec(change.restore)
		mustMQ(t, err)
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
	_, err = writer.Exec("UPDATE ai_messaging_operations SET decision='accepted' WHERE decision=''")
	mustMQ(t, err)
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	observed, err := MessagingSnapshot(ctx2, db)
	mustMQ(t, err)
	if observed["awaiting_command_receipts"] != 3 {
		t.Fatal("reader waited for/observed writer")
	}
	mustMQ(t, writer.Rollback())
	mustMQ(t, db.Ping())
	_, err = db.Exec("UPDATE ai_messaging_operations SET decision='accepted' WHERE created_at<UTC_TIMESTAMP(6)")
	mustMQ(t, err)
	futureOnly, err := MessagingSnapshot(t.Context(), db)
	mustMQ(t, err)
	if futureOnly["awaiting_command_receipts"] != 1 || futureOnly["oldest_awaiting_command_receipt_seconds"] != 0 {
		t.Fatal("future creation time was not clamped", futureOnly)
	}
	// This is the test-created schema only. A late query failure must discard
	// even already-read transaction counts rather than expose a partial empty state.
	_, err = db.Exec("DROP TABLE ai_messaging_quarantine")
	mustMQ(t, err)
	partial, err := MessagingSnapshot(t.Context(), db)
	if err == nil || partial != nil {
		t.Fatal("failed snapshot exposed partial state", partial, err)
	}
}
