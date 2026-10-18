//go:build integration

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
)

func auditDatabase(t *testing.T, mq bool) *sql.DB {
	t.Helper()
	dsn := os.Getenv("QS_AI_MQ_TEST_DSN")
	if dsn == "" {
		t.Fatal("required disposable audit database binding is missing")
	}
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal("invalid disposable database binding")
	}
	if cfg.Net != "tcp" || (!strings.HasPrefix(cfg.Addr, "127.0.0.1:") && !strings.HasPrefix(cfg.Addr, "localhost:")) {
		t.Fatal("disposable loopback database required")
	}
	cfg.DBName = ""
	cfg.MultiStatements = true
	cfg.ParseTime = true
	root, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	name := "rm_ai_mq_audit_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = root.Exec("CREATE DATABASE " + name); err != nil {
		_ = root.Close()
		t.Fatal(err)
	}
	cfg.DBName = name
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		if _, err := root.Exec("DROP DATABASE " + name); err != nil {
			t.Error(err)
		}
		_ = root.Close()
	})
	migrations := []string{"000072_ai_bridge_delivery", "000083_ai_runtime_index"}
	if mq {
		migrations = append(migrations, "000091_ai_messaging", "000092_ai_messaging_failures", "000093_ai_messaging_legacy_commands", "000094_ai_messaging_admission", "000095_ai_messaging_observations", "000097_ai_command_retirement")
	}
	for _, migration := range migrations {
		body, err := os.ReadFile(filepath.Join("..", "..", "internal", "pkg", "migration", "migrations", "mysql", migration+".up.sql"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(string(body)); err != nil {
			t.Fatal(err)
		}
	}
	return db
}
func mustAudit(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func auditRead(t *testing.T, db *sql.DB, limit int) snapshot {
	t.Helper()
	result, err := collect(context.Background(), db, limit)
	mustAudit(t, err)
	return result
}
func TestMQAuditMissingSchemaAndBoundsFailClosed(t *testing.T) {
	db := auditDatabase(t, false)
	s := auditRead(t, db, 100)
	if s.MQTablesPresent || s.PresentTables["ai_messaging_outbox"] || !s.PresentTables["ai_bridge_requests"] {
		t.Fatal("missing MQ schema treated as ready")
	}
	for _, limit := range []int{0, 1001} {
		if _, err := collect(context.Background(), db, limit); err == nil {
			t.Fatal("unbounded database snapshot accepted")
		}
	}
}
func TestMQAuditSnapshotTransactionActuallyRejectsWrites(t *testing.T) {
	db := auditDatabase(t, true)
	tx, err := db.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	mustAudit(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = readSnapshot(context.Background(), tx, snapshot{PresentTables: map[string]bool{}, Limit: 100})
	mustAudit(t, err)
	_, err = tx.Exec("UPDATE ai_messaging_admission SET revision=99 WHERE singleton=1")
	var driverError *mysql.MySQLError
	if !errors.As(err, &driverError) || driverError.Number != 1792 {
		t.Fatal("database did not enforce read-only snapshot")
	}
	mustAudit(t, tx.Rollback())
	var revision uint64
	mustAudit(t, db.QueryRow("SELECT revision FROM ai_messaging_admission WHERE singleton=1").Scan(&revision))
	if revision != 0 {
		t.Fatal("audit modified current admission state")
	}
	mustAudit(t, db.Ping())
}

func TestMQAuditReportsCurrentSchemaAndRetainedBodyMetadataAfterLegacyRetirement(t *testing.T) {
	db := auditDatabase(t, true)
	_, err := db.Exec("DROP TABLE ai_bridge_commands,ai_messaging_legacy_commands")
	mustAudit(t, err)
	var legacyTables int
	mustAudit(t, db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN ('domain_event_outbox','ai_bridge_commands','ai_messaging_legacy_commands')").Scan(&legacyTables))
	if legacyTables != 0 {
		t.Fatal("retired storage remains in disposable fixture")
	}
	missing := auditRead(t, db, 100)
	if !missing.MQTablesPresent || !missing.PresentTables["ai_bridge_requests"] || missing.SchemaHeadPresent || missing.SchemaHead != nil || missing.SchemaDirty != nil {
		t.Fatal("table presence claimed an unobserved migration head")
	}
	_, err = db.Exec("CREATE TABLE schema_migrations(version BIGINT NOT NULL PRIMARY KEY,dirty BOOLEAN NOT NULL); INSERT INTO schema_migrations VALUES(95,FALSE)")
	mustAudit(t, err)
	private := strings.Repeat("private Unicode 消息🙂", 3000)
	// Metadata-only storage fixture: this audit must not interpret/decrypt the
	// encrypted wire or claim the fixture is a valid transferable message.
	insert := `INSERT INTO ai_messaging_outbox(producer,destination,message_id,body_sha256,body,wire,wire_sha256,kind,organization_id,topic,aggregate_key,aggregate_sequence,ordered,requires_receipt,stage,attempts,available_at,created_at) VALUES('qs-server','qs-ai',?,?,?, ?,?,1,1,'qs.ai.commands.v1',?,18446744073709551615,TRUE,TRUE,?,8,NOW(6),NOW(6))`
	for _, stage := range []string{"confirmed", "held", "awaiting_receipt"} {
		id := uuid.NewString()
		body := []byte("short private body")
		if stage == "confirmed" {
			body = []byte(private)
		}
		_, err = db.Exec(insert, id, strings.Repeat("a", 64), body, []byte("opaque private wire"), strings.Repeat("b", 64), id, stage)
		mustAudit(t, err)
	}
	s := auditRead(t, db, 100)
	if !s.ReadOnly || !s.SchemaHeadPresent || s.SchemaHead == nil || *s.SchemaHead != 95 || s.SchemaDirty == nil || *s.SchemaDirty || s.UnconfirmedMessages != 2 || s.RetainedReferenceBodies != 1 || len(s.MessageSamples) != 3 || s.MessageSamplesTruncated {
		t.Fatal("missing schema, retained reference or unconfirmed metadata")
	}
	if len(s.MQStages) != 3 {
		t.Fatal("current MQ stages were hidden")
	}
	for _, stage := range s.MQStages {
		if stage.Kind != 1 || stage.Count != 1 {
			t.Fatal("current MQ stage count changed")
		}
	}
	refs := 0
	for _, row := range s.MessageSamples {
		if row.Sequence != ^uint64(0) || row.Attempts != 8 {
			t.Fatal("identity ordering integer or failure budget lost")
		}
		if row.BodyReference {
			refs++
			if row.Stage != "confirmed" {
				t.Fatal("reference retention misreported")
			}
		}
	}
	if refs != 1 {
		t.Fatal("confirmed retained reference hidden")
	}
	raw, err := json.Marshal(s)
	mustAudit(t, err)
	for _, sensitive := range []string{private, "short private body", "opaque private wire"} {
		if strings.Contains(string(raw), sensitive) {
			t.Fatal("audit exported protected body or wire")
		}
	}
	if !strings.Contains(string(raw), `"aggregate_sequence":"18446744073709551615"`) {
		t.Fatal("JSON precision lost")
	}
	bounded := auditRead(t, db, 1)
	if !bounded.MessageSamplesTruncated || len(bounded.MessageSamples) != 1 || bounded.UnconfirmedMessages != 2 || bounded.RetainedReferenceBodies != 1 {
		t.Fatal("bounded sample concealed full inventory counts")
	}
	_, err = db.Exec("UPDATE schema_migrations SET dirty=TRUE")
	mustAudit(t, err)
	dirty := auditRead(t, db, 100)
	if dirty.SchemaDirty == nil || !*dirty.SchemaDirty {
		t.Fatal("dirty schema reported clean")
	}
	var n int
	mustAudit(t, db.QueryRow("SELECT COUNT(*) FROM ai_messaging_outbox WHERE attempts=8").Scan(&n))
	if n != 3 {
		t.Fatal("read-only audit changed durable budget")
	}
	_, err = db.Exec("DROP TABLE ai_messaging_admission")
	mustAudit(t, err)
	incomplete := auditRead(t, db, 100)
	if incomplete.MQTablesPresent || incomplete.PresentTables["ai_messaging_admission"] {
		t.Fatal("missing current MQ table treated as ready")
	}
	mustAudit(t, db.Ping())
}
