//go:build integration

package migration

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	actor "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	driver "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestTaskEntryTokenNativeRetirement(t *testing.T) {
	for _, initial := range []uint{103, 104} {
		t.Run(fmt.Sprintf("from_SQL%d", initial), func(t *testing.T) { taskEntryTokenNativeRetirement(t, initial) })
	}
}

func taskEntryTokenNativeRetirement(t *testing.T, initial uint) {
	n := openNativeSchemaPairWithPrefix(t, "qs_task_token_test")
	old := migrationResourcesThrough(t, initial, 40)
	n.approve(t, old)
	n.runBoth(t, n.preflight(t, old))
	_, err := n.db.ExecContext(t.Context(), `INSERT INTO assessment_task
(id,plan_id,enrollment_id,seq,org_id,testee_id,scale_code,planned_at,status,entry_token,entry_url)
VALUES(900123,900124,900125,1,7,900126,'scale','2026-10-18','opened','retired-test-token','https://example.invalid/entry?task_id=900123')`)
	if err != nil {
		t.Fatal(err)
	}
	retained := func() string {
		t.Helper()
		var value string
		if err := n.db.QueryRowContext(t.Context(), `SELECT CONCAT_WS('|',id,plan_id,enrollment_id,seq,org_id,testee_id,scale_code,planned_at,status,entry_url,version) FROM assessment_task WHERE id=900123`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := retained()
	if initial == 104 {
		_, err := n.db.ExecContext(t.Context(), `INSERT INTO event_delivery_transport_evidence
(identity_hash,provider,topic_name,channel_name,transport_message_id,application_id,physical_topic,original_wire,metadata_bytes)
VALUES(UNHEX(REPEAT('ab',32)),'rocketmq','topic','group','message','application','physical',0x00ff,0x7b7d)`)
		if err != nil {
			t.Fatal(err)
		}
	}

	var mongoBefore bson.Raw
	if err := n.mongoDB.Collection("schema_migrations").FindOne(t.Context(), bson.D{}).Decode(&mongoBefore); err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: n.db, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	if actor.RequireCurrentOperatorPlanSchema(t.Context(), db) == nil {
		t.Fatal("current maintenance plans accepted old SQL103")
	}
	current := migrationResourcesThrough(t, 105, 40)
	p := n.preflight(t, current)
	if err := p.VerifyComplete(t.Context()); err == nil {
		t.Fatal("disabled DDL accepted a database awaiting token retirement")
	}
	cfg := offlineNativeApproval(t, n, current)
	_, snapshot, err := inspectOffline(t.Context(), n.db, n.client, cfg.Pair, current)
	if err != nil {
		t.Fatal(err)
	}
	a, err := readOfflineAuthorization(cfg, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	a.ProtectionScope = offlineProtectionTaskEntryToken
	if initial == 103 {
		a.ProtectionScope = offlineProtectionTaskEntryTokenAndTransportEvidence
	}
	writeOfflineApproval(t, &cfg, a)
	result, err := runOffline(t.Context(), n.db, n.client, cfg, current)
	if err != nil || !result.Verified || !result.AuthorizationConsumed || result.MySQLChanged == nil || !*result.MySQLChanged || result.MongoChanged == nil || *result.MongoChanged {
		t.Fatal("Task-only offline retirement failed", result, err)
	}
	if err := actor.RequireCurrentOperatorPlanSchema(t.Context(), db); err != nil {
		t.Fatal("current maintenance plans rejected SQL105", err)
	}
	if _, err := runOffline(t.Context(), n.db, n.client, cfg, current); err == nil {
		t.Fatal("consumed Task approval reused")
	}
	var evidenceRows int
	if err := n.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM event_delivery_transport_evidence").Scan(&evidenceRows); err != nil || evidenceRows != int(initial-103) {
		t.Fatal("transport evidence table or facts changed", evidenceRows, err)
	}
	if initial == 104 {
		var wire, metadata string
		if err := n.db.QueryRowContext(t.Context(), "SELECT HEX(original_wire),HEX(metadata_bytes) FROM event_delivery_transport_evidence WHERE identity_hash=UNHEX(REPEAT('ab',32))").Scan(&wire, &metadata); err != nil || wire != "00FF" || metadata != "7B7D" {
			t.Fatal("original transport evidence bytes changed", err)
		}
	}
	if retained() != before {
		t.Fatal("Task identity, entry URL or state changed during token retirement")
	}
	var columns int
	if err := n.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='assessment_task' AND column_name='entry_token'").Scan(&columns); err != nil || columns != 0 {
		t.Fatal("retired token column remains", columns, err)
	}
	var mongoAfter bson.Raw
	if err := n.mongoDB.Collection("schema_migrations").FindOne(t.Context(), bson.D{}).Decode(&mongoAfter); err != nil || !bytes.Equal(mongoBefore, mongoAfter) {
		t.Fatal("SQL-only retirement changed Mongo metadata", err)
	}
	if err := n.preflight(t, current).VerifyComplete(t.Context()); err != nil {
		t.Fatal("clean target rejected by read-only restart", err)
	}
	down, err := migrations.ReadFile("migrations/mysql/000105_retire_task_entry_token.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	_, err = n.db.ExecContext(t.Context(), string(down))
	var refusal *driver.MySQLError
	if !errors.As(err, &refusal) || string(refusal.SQLState[:]) != "45000" {
		t.Fatal("down must refuse token recreation", err)
	}
	if _, err := n.db.ExecContext(t.Context(), "ALTER TABLE assessment_task ADD COLUMN entry_token VARCHAR(255) NULL"); err != nil {
		t.Fatal(err)
	}
	if _, err := preflightCompatibilityPair(t.Context(), n.db, n.client, n.cfg, current); err == nil {
		t.Fatal("retired token column reappearance accepted")
	}
}
