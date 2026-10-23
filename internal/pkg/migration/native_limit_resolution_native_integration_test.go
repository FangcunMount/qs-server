//go:build integration

package migration

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/messagingruntime"
	"github.com/FangcunMount/reliable-messaging/transaction"
)

// Controlled schema fixture only: it tests governance reads/imports, not
// operator approval, a real native fault, RPC recovery, or production release.
func nativeResolutionFixture(t *testing.T, offset int) messagingruntime.NativeLimitEvidence {
	t.Helper()
	raw := []byte(`{"topic":"TRANS_CHECK_MAX_TIME_TOPIC","key_sha256":"1111111111111111111111111111111111111111111111111111111111111111","records":[{"broker_message_id":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","queue_id":0,"queue_offset":1,"commitlog_offset":2,"index_hour":3}],"raw_admin_sha256":"2222222222222222222222222222222222222222222222222222222222222222","read_only":true,"body_exported":false,"truncated_possible":false,"alert":"native_transaction_check_limit_reached","severity":"critical"}`)
	if offset == 2 {
		raw = bytes.ReplaceAll(raw, []byte(`"queue_offset":1,"commitlog_offset":2`), []byte(`"queue_offset":2,"commitlog_offset":3`))
	}
	evidence, err := messagingruntime.ParseNativeLimitEvidence(raw, sha256.Sum256(raw))
	if err != nil {
		t.Fatal(err)
	}
	return evidence
}
func verifySQLNativeResolutionFixture(t *testing.T, db *sql.DB, owner string) {
	t.Helper()
	b := transaction.Binding{Owner: owner, StoreID: "owned-test-primary", WriterEpoch: "m3-1"}
	// The preceding independent schema-version refusal deliberately set v2.
	if _, err := db.ExecContext(t.Context(), `UPDATE rm_tx_binding SET schema_version=1 WHERE owner=? AND schema_version=2`, []byte(owner)); err != nil {
		t.Fatal(err)
	}
	store, err := messagingruntime.NewSQLNativeLimits(db, b, "m3-1")
	if err != nil {
		t.Fatal(err)
	}
	evidence := nativeResolutionFixture(t, 1)
	read := func(want bool) {
		t.Helper()
		actual, err := store.ReadNativeLimit(t.Context())
		if err != nil || actual != want {
			t.Fatal("native governance read", actual, err)
		}
	}
	read(false)
	if err = store.RecordNativeLimit(t.Context(), evidence); err != nil {
		t.Fatal(err)
	}
	read(true)
	// This is a controlled database fixture, NOT the recovery production CAS.
	receipt := sha256.Sum256([]byte("fixture-only-no-rpc-proof"))
	result, err := db.ExecContext(t.Context(), `UPDATE messaging_native_limit_notices SET status='resolved',resolved_at=UTC_TIMESTAMP(6),resolution_receipt_sha256=? WHERE owner=? AND status='blocked'`, receipt[:], []byte(owner))
	if err != nil {
		t.Fatal(err)
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		t.Fatal(changed, err)
	}
	read(false)
	// A re-export has a different raw digest but the same original index.
	raw := strings.Replace(string([]byte(`{"topic":"TRANS_CHECK_MAX_TIME_TOPIC","key_sha256":"1111111111111111111111111111111111111111111111111111111111111111","records":[{"broker_message_id":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","queue_id":0,"queue_offset":1,"commitlog_offset":2,"index_hour":3}],"raw_admin_sha256":"2222222222222222222222222222222222222222222222222222222222222222","read_only":true,"body_exported":false,"truncated_possible":false,"alert":"native_transaction_check_limit_reached","severity":"critical"}`)), strings.Repeat("2", 64), strings.Repeat("3", 64), 1)
	later, err := messagingruntime.ParseNativeLimitEvidence([]byte(raw), sha256.Sum256([]byte(raw)))
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err = store.RecordNativeLimit(t.Context(), later); err != nil {
			t.Fatal(err)
		}
	}
	read(false)
	var proof, actualReceipt []byte
	var state string
	if err = db.QueryRowContext(t.Context(), `SELECT evidence_sha256,status,resolution_receipt_sha256 FROM messaging_native_limit_notices WHERE owner=?`, []byte(owner)).Scan(&proof, &state, &actualReceipt); err != nil {
		t.Fatal(err)
	}
	first := evidence.Digest()
	if !bytes.Equal(proof, first[:]) || state != "resolved" || !bytes.Equal(actualReceipt, receipt[:]) {
		t.Fatal("re-import rewrote first governance evidence")
	}
	if err = store.RecordNativeLimit(t.Context(), nativeResolutionFixture(t, 2)); err != nil {
		t.Fatal(err)
	}
	read(true)
	var count int
	if err = db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM messaging_native_limit_notices WHERE owner=?`, []byte(owner)).Scan(&count); err != nil || count != 2 {
		t.Fatal(count, err)
	}
	if _, err = db.ExecContext(t.Context(), `UPDATE messaging_native_limit_notices SET resolution_receipt_sha256=UNHEX(REPEAT('00',32)) WHERE status='resolved'`); err == nil {
		t.Fatal("zero resolution receipt passed original constraint")
	}
	t.Log("controlled original-MySQL governance fixture: resolved reimport immutable; new physical index blocked; no Broker/RPC proof")
}

func verifyMongoNativeResolutionFixture(t *testing.T, n nativeSchemaPair) {
	t.Helper()
	b := transaction.Binding{Owner: "qs.mongo", StoreID: "owned-test-primary", WriterEpoch: "m3-1"}
	store, err := messagingruntime.NewMongoNativeLimits(n.mongoDB, b, "m3-1")
	if err != nil {
		t.Fatal(err)
	}
	read := func(want bool) {
		t.Helper()
		actual, err := store.ReadNativeLimit(t.Context())
		if err != nil || actual != want {
			t.Fatal("Mongo governance read", actual, err)
		}
	}
	read(false)
	first := nativeResolutionFixture(t, 1)
	if err = store.RecordNativeLimit(t.Context(), first); err != nil {
		t.Fatal(err)
	}
	read(true)
	receipt := sha256.Sum256([]byte("fixture-only-no-rpc-proof"))
	collection := n.mongoDB.Collection("messaging_native_limit_notices")
	// Controlled fixture, never offered as the approved production recovery CAS.
	result, err := collection.UpdateOne(t.Context(), bson.M{"owner": b.Owner, "status": "blocked"}, bson.M{"$set": bson.M{"status": "resolved", "resolved_at": time.Now().UTC(), "resolution_receipt_sha256": receipt[:]}})
	if err != nil || result.ModifiedCount != 1 {
		t.Fatal(result, err)
	}
	read(false)
	for range 2 {
		if err = store.RecordNativeLimit(t.Context(), first); err != nil {
			t.Fatal(err)
		}
	}
	read(false)
	var row bson.M
	if err = collection.FindOne(t.Context(), bson.M{"owner": b.Owner}).Decode(&row); err != nil {
		t.Fatal(err)
	}
	digest := first.Digest()
	proof, ok := row["evidence_sha256"].(primitive.Binary)
	if !ok || !bytes.Equal(proof.Data, digest[:]) || row["status"] != "resolved" {
		t.Fatal("Mongo original governance evidence changed")
	}
	if err = store.RecordNativeLimit(t.Context(), nativeResolutionFixture(t, 2)); err != nil {
		t.Fatal(err)
	}
	read(true)
	count, err := collection.CountDocuments(t.Context(), bson.M{"owner": b.Owner})
	if err != nil || count != 2 {
		t.Fatal(count, err)
	}
	t.Log("controlled original-Mongo governance fixture: resolved reimport immutable; new physical index blocked; no Broker/RPC proof")
}
