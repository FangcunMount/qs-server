//go:build integration

package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	store "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/aibridge"
	jose "github.com/go-jose/go-jose/v4"
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
		migrations = append(migrations, "000091_ai_messaging", "000092_ai_messaging_failures", "000093_ai_messaging_legacy_commands", "000094_ai_messaging_admission", "000095_ai_messaging_observations")
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
func auditRequest() app.Start {
	return app.Start{RequestID: uuid.NewString(), Actor: app.Actor{OrgID: "1", SubjectID: "42"}, TesteeID: "7", AssessmentIDs: []string{"9"}, Goal: "private fixture body must not be exported"}
}
func auditRead(t *testing.T, db *sql.DB, limit int) snapshot {
	t.Helper()
	result, err := collect(context.Background(), db, limit)
	mustAudit(t, err)
	return result
}
func auditRow(t *testing.T, s snapshot, id string) pendingCommand {
	t.Helper()
	for _, r := range s.Pending {
		if r.ID == id {
			return r
		}
	}
	t.Fatal("original pending command missing")
	return pendingCommand{}
}

func TestMQAuditSnapshotPreservesHistoryAndSurfacesAmbiguousOwnership(t *testing.T) {
	db := auditDatabase(t, true)
	original := &store.Store{DB: db}
	first, second := auditRequest(), auditRequest()
	mustAudit(t, original.StageStart(context.Background(), first))
	mustAudit(t, original.StageStart(context.Background(), second))
	session := uuid.NewString()
	mustAudit(t, original.Acknowledge(context.Background(), app.Command{ID: second.RequestID, RequestID: second.RequestID}, app.Receipt{SessionID: session, Version: 1}))
	for i := 0; i < 2; i++ {
		mustAudit(t, original.StageChange(context.Background(), second.RequestID, app.Change{CommandID: uuid.NewString(), SessionID: session, Actor: second.Actor, Action: "cancel", ExpectedVersion: 1}))
	}
	before := auditRead(t, db, 100)
	if !before.ReadOnly || !before.MQTablesPresent || before.LegacyPending != 3 || before.LegacyDelivered != 1 || before.UnownedPending != 3 || before.UnknownOrderAggregates != 1 {
		t.Fatalf("wrong inventory: %+v", before)
	}
	firstRow := auditRow(t, before, first.RequestID)
	if firstRow.Ownership != "legacy" || firstRow.Review != "single_pending_requires_original_transaction_validation" {
		t.Fatal("unvalidated source claimed as transferred")
	}
	for _, r := range before.Pending {
		if r.RequestID == second.RequestID && r.Review != "unknown_commit_order_requires_review" {
			t.Fatal("mutable retry timestamp invented commit order")
		}
	}
	truncated := auditRead(t, db, 1)
	if !truncated.Truncated || len(truncated.Pending) != 1 || truncated.UnknownOrderAggregates != 1 {
		t.Fatal("bounded sample hid incomplete audit")
	}
	sig, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	mustAudit(t, err)
	crypt, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	mustAudit(t, err)
	handoff := &store.MessagingLegacyHandoff{Store: store.NewMessagingStore(), Seal: func(k pb.MessagingKind, id, agg, org, at string, b *pb.MessagingBody) (*app.PreparedMessaging, error) {
		return app.ProtectMessaging(k, id, agg, "", org, at, b, jose.JSONWebKey{Key: sig, KeyID: "audit.sign"}, jose.JSONWebKey{Key: &crypt.PublicKey, KeyID: "audit.encrypt"})
	}}
	tx, err := db.Begin()
	mustAudit(t, err)
	transferred, err := handoff.StageSingle(context.Background(), tx, first.RequestID)
	mustAudit(t, err)
	if !transferred {
		t.Fatal("single historical command not transferred")
	}
	mustAudit(t, tx.Commit())
	after := auditRead(t, db, 100)
	r := auditRow(t, after, first.RequestID)
	if after.UnownedPending != 2 || after.LegacyPending != 3 || after.LegacyDelivered != 1 || r.Ownership != "mq" || r.MQStage != "staged" || r.BodyHash != firstRow.BodyHash || r.AvailableAt != firstRow.AvailableAt || r.Attempts != firstRow.Attempts {
		t.Fatal("ownership audit changed or lost original source")
	}
	mustAudit(t, db.QueryRow("SELECT COUNT(*) FROM ai_messaging_legacy_commands").Scan(new(uint64)))
	data, err := json.Marshal(after)
	mustAudit(t, err)
	if strings.Contains(string(data), first.Goal) {
		t.Fatal("sensitive business body leaked into audit")
	}
	_, err = db.Exec("UPDATE ai_messaging_outbox SET stage='held',attempts=8,error_code='test_budget' WHERE message_id=?", first.RequestID)
	mustAudit(t, err)
	held := auditRead(t, db, 100)
	if auditRow(t, held, first.RequestID).MQStage != "held" || len(held.MQStages) != 1 || held.MQStages[0].Stage != "held" {
		t.Fatal("technical suspension was hidden")
	}
	// A tampered source hash must not be presented as a verified migration link.
	_, err = db.Exec("UPDATE ai_bridge_commands SET payload_hash=? WHERE command_id=?", strings.Repeat("0", 64), first.RequestID)
	mustAudit(t, err)
	changed := auditRead(t, db, 100)
	if auditRow(t, changed, first.RequestID).Ownership == "mq" {
		t.Fatal("conflicting source claimed as verified ownership")
	}
}
func TestMQAuditMissingSchemaAndBoundsFailClosed(t *testing.T) {
	db := auditDatabase(t, false)
	r := auditRequest()
	mustAudit(t, (&store.Store{DB: db}).StageStart(context.Background(), r))
	s := auditRead(t, db, 100)
	if s.MQTablesPresent || s.PresentTables["ai_messaging_outbox"] || s.UnownedPending != 1 || auditRow(t, s, r.RequestID).Ownership != "legacy" {
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
	r := auditRequest()
	mustAudit(t, (&store.Store{DB: db}).StageStart(context.Background(), r))
	tx, err := db.BeginTx(context.Background(), &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	mustAudit(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = readSnapshot(context.Background(), tx, snapshot{PresentTables: map[string]bool{}, Limit: 100})
	mustAudit(t, err)
	_, err = tx.Exec("UPDATE ai_bridge_commands SET attempts=99 WHERE command_id=?", r.RequestID)
	var driverError *mysql.MySQLError
	if !errors.As(err, &driverError) || driverError.Number != 1792 {
		t.Fatal("database did not enforce read-only snapshot")
	}
	mustAudit(t, tx.Rollback())
	var attempts uint64
	mustAudit(t, db.QueryRow("SELECT attempts FROM ai_bridge_commands WHERE command_id=?", r.RequestID).Scan(&attempts))
	if attempts != 0 {
		t.Fatal("audit modified original retry budget")
	}
	mustAudit(t, db.Ping())
}

func TestMQAuditReportsSchemaAndRetainedBodyMetadataWithoutExport(t *testing.T) {
	db := auditDatabase(t, true)
	missing := auditRead(t, db, 100)
	if !missing.MQTablesPresent || missing.SchemaHeadPresent || missing.SchemaHead != nil || missing.SchemaDirty != nil {
		t.Fatal("table presence claimed an unobserved migration head")
	}
	_, err := db.Exec("CREATE TABLE schema_migrations(version BIGINT NOT NULL PRIMARY KEY,dirty BOOLEAN NOT NULL); INSERT INTO schema_migrations VALUES(95,FALSE)")
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
	if !s.SchemaHeadPresent || s.SchemaHead == nil || *s.SchemaHead != 95 || s.SchemaDirty == nil || *s.SchemaDirty || s.UnconfirmedMessages != 2 || s.RetainedReferenceBodies != 1 || len(s.MessageSamples) != 3 || s.MessageSamplesTruncated {
		t.Fatal("missing schema, retained reference or unconfirmed metadata")
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
	mustAudit(t, db.Ping())
}
