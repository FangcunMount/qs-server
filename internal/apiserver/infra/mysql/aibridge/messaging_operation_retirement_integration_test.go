//go:build integration

package aibridge

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/google/uuid"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func retirementGORM(t *testing.T, db *sql.DB, tx *sql.Tx) *gorm.DB {
	t.Helper()
	g, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
	mustMQ(t, err)
	if tx != nil {
		g.Statement.ConnPool = tx
	}
	return g
}

func retirementEvidence(t *testing.T, f *mqFixture) CommandRetirementEvidence {
	t.Helper()
	var raw []byte
	var businessHash string
	mustMQ(t, f.db.QueryRow("SELECT CAST(payload AS BINARY),payload_hash FROM ai_bridge_commands WHERE command_id=?", f.request.RequestID).Scan(&raw, &businessHash))
	var revision uint64
	mustMQ(t, f.db.QueryRow("SELECT revision FROM ai_messaging_admission WHERE singleton=1").Scan(&revision))
	return CommandRetirementEvidence{Version: 1, OperationID: "123-1", VerifierVersion: "qs-retirement/v1", VerificationMethod: "source_identity_hash_and_business_closure", VerifiedAt: time.Now().UTC(), AdmissionRevision: revision, CommandID: f.request.RequestID, RequestID: f.request.RequestID, SourceKind: "start", OrganizationID: f.scope.OrganizationID, SubjectID: f.scope.SubjectID, ResourceID: f.request.RequestID, Sources: []CommandRetirementSource{{Table: "ai_bridge_commands", CommandID: f.request.RequestID, BytesKind: "mysql_json_payload_cast_binary_sha256", BytesSHA256: messagingHash(raw), BusinessPayloadHash: businessHash}}, References: []CommandRetirementReference{{Kind: "business_record", ID: f.request.RequestID}}, Conclusion: "verified", Reason: "history_terminal_verified", OwnershipVerified: true, ResponsibilityClosed: true, BusinessTerminal: true}
}

func TestMQRetiredIdentityPreservesNoReplayAcrossRestoreAndLateReceipt(t *testing.T) {
	f := newMQFixture(t)
	mustMQ(t, seedLegacyStartFixture(t.Context(), f.request, &Store{DB: f.db}))
	_, err := f.db.Exec("UPDATE ai_bridge_requests SET status='completed' WHERE request_id=?", f.request.RequestID)
	mustMQ(t, err)
	_, err = f.db.Exec("UPDATE ai_messaging_admission SET closed=TRUE WHERE singleton=1")
	mustMQ(t, err)
	e := retirementEvidence(t, f)
	if e.Sources[0].BytesSHA256 == e.Sources[0].BusinessPayloadHash {
		t.Fatal("fixture does not distinguish MySQL CAST bytes from business JSON encoding")
	}
	if err = RecordOperationRetirement(t.Context(), retirementGORM(t, f.db, nil), e); !errors.Is(err, app.ErrConflict) {
		t.Fatal("unowned pool was accepted", err)
	}
	for i := 0; i < 2; i++ {
		mustMQ(t, f.tx(func(tx *sql.Tx) error {
			g := retirementGORM(t, f.db, tx)
			if i == 0 {
				g = g.Session(&gorm.Session{PrepareStmt: true})
			}
			if err := RecordOperationRetirement(t.Context(), g, e); err != nil {
				return err
			}
			var n int
			return tx.QueryRow("SELECT 1").Scan(&n) // helper did not commit/close
		}))
	}
	changed := e
	changed.OperationID = "123-2"
	if err = f.tx(func(tx *sql.Tx) error {
		return RecordOperationRetirement(t.Context(), retirementGORM(t, f.db, tx), changed)
	}); !errors.Is(err, app.ErrConflict) {
		t.Fatal("different evidence overwrote a retirement", err)
	}
	changed = e
	changed.VerifierVersion = "qs-retirement/v2"
	if err = f.tx(func(tx *sql.Tx) error {
		return RecordOperationRetirement(t.Context(), retirementGORM(t, f.db, tx), changed)
	}); !errors.Is(err, app.ErrConflict) {
		t.Fatal("same operation with different evidence overwrote a retirement", err)
	}
	// Restored old pending rows remain present. Neither normal staging nor the
	// still-packaged pre-cutover handoff path may manufacture a new command.
	_, err = f.db.Exec("UPDATE ai_messaging_admission SET closed=FALSE WHERE singleton=1")
	mustMQ(t, err)
	calls := 0
	s := f.commandStore(&calls)
	for _, mutation := range []string{"same", "body", "organization"} {
		r := f.request
		if mutation == "body" {
			r.Goal = "different body"
		}
		if mutation == "organization" {
			r.Actor.OrgID = "2"
		}
		if err = s.StageStart(t.Context(), r); !errors.Is(err, app.ErrConflict) {
			t.Fatal(mutation, err)
		}
	}
	if err = f.tx(func(tx *sql.Tx) error {
		_, err := f.stage(tx, pb.MessagingKind_START, e.CommandID, e.RequestID, f.startBody(), f.scope, &calls)
		return err
	}); !errors.Is(err, app.ErrConflict) {
		t.Fatal("lower-level staging reused retired PK", err)
	}
	if err = f.tx(func(tx *sql.Tx) error {
		_, err := handoffFixture(f, &calls).StageSingle(t.Context(), tx, e.CommandID)
		return err
	}); !errors.Is(err, app.ErrConflict) {
		t.Fatal("restored old pending source was retransferred", err)
	}
	if _, err = s.ReadOperation(t.Context(), f.scope, e.CommandID); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("retirement exposed as live operation", err)
	}
	if _, err = s.ReadRequestOperation(t.Context(), f.scope, e.RequestID, e.CommandID); !errors.Is(err, app.ErrNotFound) {
		t.Fatal("retirement exposed as participant operation", err)
	}
	receipt := &pb.MessagingCommandReceipt{CommandId: e.CommandID, CommandBodySha256: strings.Repeat("a", 64), Decision: pb.MessagingDecision_ACCEPTED}
	body := &pb.MessagingBody{Value: &pb.MessagingBody_CommandReceipt{CommandReceipt: receipt}}
	m := f.protect(pb.MessagingKind_COMMAND_RECEIPT, uuid.NewString(), e.RequestID, e.CommandID, body, true)
	if err = f.tx(func(tx *sql.Tx) error {
		_, err := f.store.applyReceipt(t.Context(), tx, m.Envelope, receipt, m.Body)
		return err
	}); !errors.Is(err, app.ErrConflict) {
		t.Fatal("late receipt was a technical NULL error", err)
	}
	if err = f.tx(func(tx *sql.Tx) error {
		_, err := f.store.heldEventOrganization(t.Context(), tx, m.Envelope, body)
		return err
	}); !errors.Is(err, app.ErrConflict) {
		t.Fatal("held late receipt was a technical NULL error", err)
	}
	mustMQ(t, receiverFixture(f, &localEventBody{}).Receive(t.Context(), m.Wire))
	var quarantined int
	mustMQ(t, f.db.QueryRow("SELECT COUNT(*) FROM ai_messaging_quarantine WHERE wire_sha256=? AND code='identity_conflict'", messagingHash(m.Wire)).Scan(&quarantined))
	if quarantined != 1 || calls != 0 {
		t.Fatal("late receipt was not isolated or retired command was sealed", quarantined, calls)
	}
	t.Cleanup(func() {
		_, err := f.db.Exec("DELETE FROM ai_messaging_quarantine WHERE wire_sha256=?", messagingHash(m.Wire))
		if err != nil {
			t.Error(err)
		}
	})
	for _, table := range []string{"ai_messaging_outbox", "ai_messaging_inbox", "ai_messaging_failures", "ai_messaging_aggregates"} {
		var n int
		mustMQ(t, f.db.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE aggregate_key=?", e.RequestID).Scan(&n))
		if n != 0 {
			t.Fatal("retired identity created live facts", table, n)
		}
	}
	var nulls bool
	mustMQ(t, f.db.QueryRow("SELECT body_sha256 IS NULL AND aggregate_sequence IS NULL AND created_at IS NULL AND receipt IS NULL AND receipt_id IS NULL AND decision='' FROM ai_messaging_operations WHERE command_id=?", e.CommandID).Scan(&nulls))
	if !nulls {
		t.Fatal("retirement invented standard evidence")
	}
}

func TestMQRetirementSourceDriftUnknownAndTransferEvidence(t *testing.T) {
	f := newMQFixture(t)
	mustMQ(t, seedLegacyStartFixture(t.Context(), f.request, &Store{DB: f.db}))
	e := retirementEvidence(t, f)
	_, err := f.db.Exec("UPDATE ai_messaging_admission SET closed=TRUE WHERE singleton=1")
	mustMQ(t, err)
	for _, scenario := range []string{"unknown_responsibility", "not_terminal", "unverified_owner", "bytes_hash", "business_hash", "revision", "digest_kind"} {
		bad := e
		bad.Sources = append([]CommandRetirementSource(nil), e.Sources...)
		switch scenario {
		case "unknown_responsibility":
			bad.ResponsibilityClosed = false
		case "not_terminal":
			bad.BusinessTerminal = false
		case "unverified_owner":
			bad.OwnershipVerified = false
		case "bytes_hash":
			bad.Sources[0].BytesSHA256 = strings.Repeat("0", 64)
		case "business_hash":
			bad.Sources[0].BusinessPayloadHash = strings.Repeat("0", 64)
		case "revision":
			bad.AdmissionRevision++
		case "digest_kind":
			bad.Sources[0].BytesKind = "standard_mq_body_sha256"
		}
		if err = f.tx(func(tx *sql.Tx) error {
			return RecordOperationRetirement(t.Context(), retirementGORM(t, f.db, tx), bad)
		}); !errors.Is(err, app.ErrConflict) {
			t.Fatal(scenario, err)
		}
	}
	calls := 0
	mustMQ(t, f.tx(func(tx *sql.Tx) error {
		_, err := handoffFixture(f, &calls).StageSingle(t.Context(), tx, e.CommandID)
		return err
	}))
	var bodyHash, wireHash string
	var sequence uint64
	var created time.Time
	mustMQ(t, f.db.QueryRow("SELECT body_sha256,aggregate_sequence,created_at FROM ai_messaging_operations WHERE command_id=?", e.CommandID).Scan(&bodyHash, &sequence, &created))
	mustMQ(t, f.db.QueryRow("SELECT wire_sha256 FROM ai_messaging_outbox WHERE message_id=?", e.CommandID).Scan(&wireHash))
	e.Conclusion = "transferred_verified"
	e.Reason = "handoff_verified"
	e.VerificationMethod = "source_identity_hash_and_live_ledger"
	e.LiveBodySHA256 = bodyHash
	e.BusinessTerminal = false
	e.ResponsibilityClosed = false
	var source []byte
	var sourceHash string
	mustMQ(t, f.db.QueryRow("SELECT source_payload,source_payload_hash FROM ai_messaging_legacy_commands WHERE command_id=?", e.CommandID).Scan(&source, &sourceHash))
	e.Sources = append(e.Sources, CommandRetirementSource{Table: "ai_messaging_legacy_commands", CommandID: e.CommandID, BytesKind: "mysql_blob_source_payload_sha256", BytesSHA256: messagingHash(source), BusinessPayloadHash: sourceHash})
	for i := 0; i < 2; i++ {
		mustMQ(t, f.tx(func(tx *sql.Tx) error {
			g := retirementGORM(t, f.db, tx)
			if i == 0 {
				g = g.Session(&gorm.Session{PrepareStmt: true})
			}
			return RecordOperationTransferEvidence(t.Context(), g, e)
		}))
	}
	changed := e
	changed.VerifierVersion = "qs-retirement/v2"
	if err = f.tx(func(tx *sql.Tx) error {
		return RecordOperationTransferEvidence(t.Context(), retirementGORM(t, f.db, tx), changed)
	}); !errors.Is(err, app.ErrConflict) {
		t.Fatal("same operation with different evidence overwrote a live handoff", err)
	}
	var same bool
	mustMQ(t, f.db.QueryRow("SELECT retired=FALSE AND body_sha256=? AND aggregate_sequence=? AND created_at=? AND receipt IS NULL AND decision='' FROM ai_messaging_operations WHERE command_id=?", bodyHash, sequence, created, e.CommandID).Scan(&same))
	if !same {
		t.Fatal("live handoff was converted or mutated")
	}
	var sameWire string
	mustMQ(t, f.db.QueryRow("SELECT wire_sha256 FROM ai_messaging_outbox WHERE message_id=?", e.CommandID).Scan(&sameWire))
	if sameWire != wireHash {
		t.Fatal("handoff metadata resealed live wire")
	}
	bad := e
	bad.LiveBodySHA256 = strings.Repeat("0", 64)
	if err = f.tx(func(tx *sql.Tx) error {
		return RecordOperationTransferEvidence(t.Context(), retirementGORM(t, f.db, tx), bad)
	}); !errors.Is(err, app.ErrConflict) {
		t.Fatal("changed live evidence accepted", err)
	}
}

func TestMQRetirementSchemaEnforcedAndIndexesRequired(t *testing.T) {
	f := newMQFixture(t)
	mustMQ(t, f.tx(func(tx *sql.Tx) error { return RequireMessagingOperationSchema(context.Background(), tx) }))
	for _, values := range []string{
		"NULL,1,UTC_TIMESTAMP(6),FALSE,NULL,NULL,'',NULL",
		"NULL,NULL,NULL,TRUE,JSON_OBJECT('version',1),UTC_TIMESTAMP(6),'',NULL",
		"NULL,NULL,NULL,TRUE,JSON_OBJECT('version',1),UTC_TIMESTAMP(6),'accepted',NULL",
		"NULL,NULL,NULL,TRUE,JSON_OBJECT('version',1),UTC_TIMESTAMP(6),'','invented receipt'",
		"NULL,NULL,NULL,TRUE,JSON_OBJECT('version',1,'ownership_verified','true','responsibility_closed','true','business_terminal','true'),UTC_TIMESTAMP(6),'',NULL",
		"NULL,NULL,NULL,TRUE,JSON_OBJECT('version','1','ownership_verified',CAST('true' AS JSON),'responsibility_closed',CAST('true' AS JSON),'business_terminal',CAST('true' AS JSON)),UTC_TIMESTAMP(6),'',NULL",
	} {
		_, err := f.db.Exec("INSERT INTO ai_messaging_operations(command_id,kind,organization_id,subject_id,resource_id,aggregate_key,body_sha256,aggregate_sequence,created_at,retired,retirement_evidence,retired_at,decision,receipt) VALUES(?,1,1,'42',?,?,"+values+")", uuid.NewString(), f.request.RequestID, f.request.RequestID)
		if err == nil {
			t.Fatal("invalid retirement shape passed CHECK", values)
		}
	}
}

func TestMQRetirementRejectsOrphanLegacyMapping(t *testing.T) {
	f := newMQFixture(t)
	mustMQ(t, seedLegacyStartFixture(t.Context(), f.request, &Store{DB: f.db}))
	_, err := f.db.Exec("UPDATE ai_messaging_admission SET closed=TRUE WHERE singleton=1")
	mustMQ(t, err)
	e := retirementEvidence(t, f)
	calls := 0
	mustMQ(t, f.tx(func(tx *sql.Tx) error {
		_, err := handoffFixture(f, &calls).StageSingle(t.Context(), tx, e.CommandID)
		return err
	}))
	_, err = f.db.Exec("UPDATE ai_bridge_requests SET status='completed' WHERE request_id=?", e.RequestID)
	mustMQ(t, err)
	_, err = f.db.Exec("DELETE FROM ai_messaging_outbox WHERE message_id=?", e.CommandID)
	mustMQ(t, err)
	_, err = f.db.Exec("DELETE FROM ai_messaging_operations WHERE command_id=?", e.CommandID)
	mustMQ(t, err)
	if err = f.tx(func(tx *sql.Tx) error {
		return RecordOperationRetirement(t.Context(), retirementGORM(t, f.db, tx), e)
	}); !errors.Is(err, app.ErrConflict) {
		t.Fatal("orphaned mapping was treated as an unmapped historical ID", err)
	}
	var n int
	mustMQ(t, f.db.QueryRow("SELECT COUNT(*) FROM ai_messaging_operations WHERE command_id=?", e.CommandID).Scan(&n))
	if n != 0 {
		t.Fatal("orphaned mapping acquired a retired replacement")
	}
}
