//go:build integration

package evaluation

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"gorm.io/gorm"
)

func TestSQLHistoricalOwnerFactsUniqueAnswerSheetAndRawAnchorNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	if err := db.Exec("UPDATE assessment SET conducting_context=CAST(? AS JSON) WHERE id=42", `{"purpose":"assessment","frozen":true}`).Error; err != nil {
		t.Fatal(err)
	}
	var uniqueFacts *SQLHistoricalOwnerFacts
	if err := db.Transaction(func(tx *gorm.DB) error {
		ctx := hostmysql.WithTx(t.Context(), tx)
		server, database, err := historicalDatabase(tx)
		if err != nil {
			return err
		}
		identity := sqlHistoricalIdentity(server, database)
		facts, err := PrepareSQLHistoricalAnswerSheetFacts(ctx, 10042, "original-mongo-submission", identity)
		if err != nil {
			return err
		}
		uniqueFacts = facts
		if !facts.HasVerifiedAnswerSheetAssociation(10042) || facts.HasVerifiedAnswerSheetAssociation(10043) {
			t.Fatal("unique association capability lost or widened")
		}
		point, err := PrepareSQLHistoricalOwnerFacts(ctx, 42, "original-mongo-submission", identity)
		if err != nil || point.HasVerifiedAnswerSheetAssociation(10042) {
			t.Fatal("ID point read promoted to unique association", err)
		}
		var original []byte
		if err := tx.Raw("SELECT CAST(conducting_context AS BINARY) FROM assessment WHERE id=42").Row().Scan(&original); err != nil {
			return err
		}
		copy := facts.Snapshot()
		if copy.Owner.AssessmentID != 42 || !copy.Owner.ConductingContextPresent || !bytes.Equal(copy.Owner.ConductingContextBytes, original) || copy.Owner.ConductingContextSHA256 != evidence.SourceDigest("assessment-conducting-context/v1", original).SHA256 {
			t.Fatal("server JSON original representation lost")
		}
		copy.Owner.ConductingContextBytes[0] = '['
		if !bytes.Equal(facts.Snapshot().Owner.ConductingContextBytes, original) {
			t.Fatal("editable raw anchor changed snapshot")
		}
		if _, err := PrepareSQLHistoricalAnswerSheetFacts(ctx, 99999, "unknown-original-submission", identity); !errors.Is(err, ErrSQLHistoricalOwnerAbsent) {
			t.Fatal("absence converted to an independent Admission", err)
		}
		return facts.Recheck(ctx)
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("ALTER TABLE assessment DROP INDEX uk_answer_sheet_id").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		server, database, err := historicalDatabase(tx)
		if err != nil {
			return err
		}
		_, err = PrepareSQLHistoricalAnswerSheetFacts(hostmysql.WithTx(t.Context(), tx), 10042, "original-mongo-submission", sqlHistoricalIdentity(server, database))
		if !errors.Is(err, ErrSQLHistoricalFactsConflict) {
			t.Fatal("unproven unique association accepted", err)
		}
		if err := uniqueFacts.Recheck(hostmysql.WithTx(t.Context(), tx)); !errors.Is(err, ErrSQLHistoricalFactsConflict) {
			t.Fatal("association recheck bypassed changed unique index", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSQLHistoricalOwnerFactsRetainedReferencesAndGapDecisionsNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	if err := db.Exec("INSERT INTO qs_rm_evaluation_request_ref(event_id,assessment_id,org_id) VALUES('original-request-ref',42,8)").Error; err != nil {
		t.Fatal(err)
	}
	read := func(check func([]SQLHistoricalResponsibility)) {
		t.Helper()
		if err := db.Transaction(func(tx *gorm.DB) error {
			server, database, err := historicalDatabase(tx)
			if err != nil {
				return err
			}
			facts, err := PrepareSQLHistoricalOwnerFacts(hostmysql.WithTx(t.Context(), tx), 42, "old-original-source", sqlHistoricalIdentity(server, database))
			if err != nil {
				return err
			}
			check(facts.Snapshot().Responsibilities)
			return facts.Recheck(hostmysql.WithTx(t.Context(), tx))
		}); err != nil {
			t.Fatal(err)
		}
	}
	read(func(rows []SQLHistoricalResponsibility) {
		if len(rows) != 1 || rows[0].Store != "qs_rm_evaluation_request_ref" || !rows[0].Invalid || rows[0].OrgID != 8 {
			t.Fatal("wrong organization hidden by owner filter")
		}
	})
	if err := db.Exec("UPDATE qs_rm_evaluation_request_ref SET org_id=7").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO qs_rm_gap_recovery_request(org_id,request_id,actor_id,assessment_id,event_id,expected_version,reason,submitted_before,input_hash,result_code,authorized,outbox_version_before,outbox_version_after) VALUES(7,'retained-gap-request',21,42,'original-request-ref',1,'synthetic','synthetic',REPEAT(CHAR(0),32),'authorized',1,1,2)").Error; err != nil {
		t.Fatal(err)
	}
	read(func(rows []SQLHistoricalResponsibility) {
		if len(rows) != 2 || rows[1].Store != "qs_rm_gap_recovery_request" || rows[1].Invalid || !rows[1].Unfinished {
			t.Fatal("missing authorized current message treated as settled")
		}
	})
	if err := db.Exec("UPDATE qs_rm_gap_recovery_request SET authorized=0,result_code='ever_claimed',outbox_version_before=NULL,outbox_version_after=NULL").Error; err != nil {
		t.Fatal(err)
	}
	read(func(rows []SQLHistoricalResponsibility) {
		if len(rows) != 2 || rows[1].Invalid || rows[1].Unfinished {
			t.Fatal("completed denial invented a replay responsibility")
		}
	})
	if err := db.Exec("UPDATE qs_rm_gap_recovery_request SET result_code='unknown'").Error; err != nil {
		t.Fatal(err)
	}
	read(func(rows []SQLHistoricalResponsibility) {
		if len(rows) != 2 || !rows[1].Invalid {
			t.Fatal("unknown gap decision accepted")
		}
	})
}

func TestSQLHistoricalOwnerFactsRealBorrowedSnapshotAndRecheckNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	record, _ := testCommittedReference(t, 9001, 42, "original-current-reference")
	if err := NewOutcomeRepository(db).Save(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	var observed *SQLHistoricalOwnerFacts
	err := db.Transaction(func(tx *gorm.DB) error {
		ctx := hostmysql.WithTx(t.Context(), tx)
		server, database, err := historicalDatabase(tx)
		if err != nil {
			return err
		}
		facts, err := PrepareSQLHistoricalOwnerFacts(ctx, 42, "original-source-event", sqlHistoricalIdentity(server, database))
		if err != nil {
			return err
		}
		observed = facts
		if snapshot := facts.Snapshot(); snapshot.Owner.AssessmentID != 42 || snapshot.Owner.OrgID != 7 || len(snapshot.Runs) != 1 || snapshot.Runs[0].Attempt != 1 || len(snapshot.Outcomes) != 1 || len(snapshot.Responsibilities) != 0 {
			t.Fatal("actual owner graph missing")
		}
		original, err := facts.OutcomeRecord(9001)
		if err != nil || original.ID().Uint64() != 9001 || original.RunID() != "42:1" || original.Model().Code != record.Model().Code || !bytes.Equal(original.Payload(), record.Payload()) {
			t.Fatal("exact original immutable Outcome missing", err)
		}
		payload := original.Payload()
		payload[0] = '['
		reloaded, err := facts.OutcomeRecord(9001)
		if err != nil || reloaded == original || !bytes.Equal(reloaded.Payload(), record.Payload()) {
			t.Fatal("editable getter changed original Outcome", err)
		}
		if _, err := facts.OutcomeRecord(9002); !errors.Is(err, evaluationfact.ErrNotFound) {
			t.Fatal("unbound Outcome ID accepted", err)
		}
		copy := facts.Snapshot()
		copy.Owner.OrgID = 8
		copy.Runs[0].ResourceID = "edited"
		if facts.Snapshot().Owner.OrgID != 7 || facts.Snapshot().Runs[0].ResourceID == "edited" {
			t.Fatal("editable getter changed opaque snapshot")
		}
		if err := facts.Recheck(ctx); err != nil {
			return err
		}
		if err := tx.Exec("UPDATE assessment SET failure_reason='' WHERE id=42").Error; err != nil {
			return err
		}
		if err := facts.Recheck(ctx); !errors.Is(err, ErrSQLHistoricalFactsConflict) {
			t.Fatal("physical NULL or owner change accepted", err)
		}
		return fmt.Errorf("intentional host rollback")
	})
	if err == nil || observed == nil {
		t.Fatal("borrowed transaction fixture did not run")
	}
	var reason *string
	if err := db.Raw("SELECT failure_reason FROM assessment WHERE id=42").Scan(&reason).Error; err != nil || reason != nil {
		t.Fatal("helper committed caller rollback", err)
	}
	if err := observed.Recheck(hostmysql.WithTx(t.Context(), db)); err == nil {
		t.Fatal("normal pool accepted as recheck transaction")
	}
}

func TestSQLHistoricalOwnerFactsActualPayloadCandidatesAndResponsibilitiesNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	record, wire := testCommittedReference(t, 9001, 42, "actual-current-event")
	if err := NewOutcomeRepository(db).Save(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO rm_outbox(producer,message_id,destination,event_type,schema_version,scope,content_type,occurred_at,payload,fingerprint,state,next_attempt_at,transport_confirmed_at) VALUES(?,?,?,?,?,?,?,?,?,?,'published',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6))", wire.Producer, wire.MessageID, wire.Destination, wire.EventType, wire.SchemaVersion, wire.Scope, wire.ContentType, wire.OccurredAt, wire.Payload, wire.Fingerprint).Error; err != nil {
		t.Fatal(err)
	}
	var rawJSON, textJSON int
	var aggregate string
	if err := db.Raw("SELECT JSON_VALID(payload),JSON_VALID(CONVERT(payload USING utf8mb4)),JSON_UNQUOTE(JSON_EXTRACT(CONVERT(payload USING utf8mb4),'$.metadata.aggregate_id')) FROM rm_outbox").Row().Scan(&rawJSON, &textJSON, &aggregate); err != nil {
		t.Fatal(err)
	}
	if textJSON != 1 || aggregate != "42" {
		t.Fatal("synthetic UTF-8 candidate hint unavailable")
	}
	t.Logf("synthetic JSON candidate requires binary/text handling: raw_valid=%d text_valid=%d", rawJSON, textJSON)
	read := func(expectInvalid, expectUnfinished bool) {
		t.Helper()
		if err := db.Transaction(func(tx *gorm.DB) error {
			server, database, err := historicalDatabase(tx)
			if err != nil {
				return err
			}
			facts, err := PrepareSQLHistoricalOwnerFacts(hostmysql.WithTx(t.Context(), tx), 42, "old-event-id-that-is-not-the-current-ID", sqlHistoricalIdentity(server, database))
			if err != nil {
				return err
			}
			snapshot := facts.Snapshot()
			if len(snapshot.Responsibilities) != 1 || snapshot.Responsibilities[0].Store != "rm_outbox" || snapshot.Responsibilities[0].Invalid != expectInvalid || snapshot.Responsibilities[0].Unfinished != expectUnfinished {
				t.Fatalf("candidate missing or misclassified: count=%d expected_invalid=%t expected_unfinished=%t facts=%+v", len(snapshot.Responsibilities), expectInvalid, expectUnfinished, snapshot.Responsibilities)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	read(false, false)
	if err := db.Exec("UPDATE rm_outbox SET scope='org:8'").Error; err != nil {
		t.Fatal(err)
	}
	read(true, false)
	if err := db.Exec("UPDATE rm_outbox SET scope='org:7',state='quarantined',transport_confirmed_at=NULL").Error; err != nil {
		t.Fatal(err)
	}
	read(false, true)
	outer, recognized, err := legacy.Decode(wire.Payload)
	if err != nil || !recognized {
		t.Fatal("actual SDK wire fixture invalid")
	}
	outer.Metadata["aggregate_id"] = "99999"
	wrongHint, err := legacy.Encode(outer, legacy.Revision2)
	if err != nil {
		t.Fatal(err)
	}
	mutated, err := message.New(message.Input{Producer: wire.Producer, ID: wire.MessageID, Destination: wire.Destination, EventType: wire.EventType, SchemaVersion: wire.SchemaVersion, Scope: wire.Scope, ContentType: wire.ContentType, OccurredAt: wire.OccurredAt, Payload: wrongHint})
	if err != nil {
		t.Fatal(err)
	}
	fingerprint := mutated.Fingerprint()
	if err := db.Exec("UPDATE rm_outbox SET payload=?,fingerprint=?", wrongHint, fingerprint[:]).Error; err != nil {
		t.Fatal(err)
	}
	// The real inner owner still selects this corrupt outer candidate, even
	// after recomputing its SDK fingerprint. It must remain invalid.
	read(true, true)
}

func TestSQLHistoricalOwnerFactsNoSilentRowCapNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	for i := 2; i <= SQLHistoricalOwnerRowLimit+1; i++ {
		if err := db.Exec("INSERT INTO runtime_checkpoint(scope,resource_id,attempt_no,assessment_id,status,started_at,finished_at) VALUES('evaluation_run',?,?,42,'succeeded',UTC_TIMESTAMP(3),UTC_TIMESTAMP(3))", fmt.Sprintf("actual:%d", i), i).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		server, database, err := historicalDatabase(tx)
		if err != nil {
			return err
		}
		_, err = PrepareSQLHistoricalOwnerFacts(hostmysql.WithTx(t.Context(), tx), 42, "original-event", sqlHistoricalIdentity(server, database))
		if !errors.Is(err, ErrSQLHistoricalFactsBounds) {
			t.Fatal("silent partial owner history", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSQLHistoricalOwnerFactsActualClockMetadataRecheckNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	var original *SQLHistoricalOwnerFacts
	read := func(fn func(*SQLHistoricalOwnerFacts, *gorm.DB) error) {
		t.Helper()
		if err := db.Transaction(func(tx *gorm.DB) error {
			server, database, err := historicalDatabase(tx)
			if err != nil {
				return err
			}
			facts, err := PrepareSQLHistoricalOwnerFacts(hostmysql.WithTx(t.Context(), tx), 42, "original-event", sqlHistoricalIdentity(server, database))
			if err != nil {
				return err
			}
			return fn(facts, tx)
		}); err != nil {
			t.Fatal(err)
		}
	}
	read(func(facts *SQLHistoricalOwnerFacts, _ *gorm.DB) error {
		owner := facts.Snapshot().Owner
		if owner.ActualSubmittedAtDataType != "datetime" || owner.ActualSubmittedAtPrecision != 0 || owner.ClockComparisonRuleVersion != SQLHistoricalClockComparisonRuleVersion {
			t.Fatal("actual storage header missing")
		}
		original = facts
		return nil
	})
	if err := db.Exec("ALTER TABLE assessment MODIFY submitted_at DATETIME(3) NULL").Error; err != nil {
		t.Fatal(err)
	}
	read(func(facts *SQLHistoricalOwnerFacts, tx *gorm.DB) error {
		if facts.Snapshot().Owner.ActualSubmittedAtPrecision != 3 {
			t.Fatal("actual changed precision not read")
		}
		if err := original.Recheck(hostmysql.WithTx(t.Context(), tx)); !errors.Is(err, ErrSQLHistoricalFactsConflict) {
			t.Fatal("actual schema-header change bypassed recheck", err)
		}
		return nil
	})
	if err := db.Exec("ALTER TABLE assessment MODIFY submitted_at DATETIME(2) NULL").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		server, database, err := historicalDatabase(tx)
		if err != nil {
			return err
		}
		if _, err := PrepareSQLHistoricalOwnerFacts(hostmysql.WithTx(t.Context(), tx), 42, "original-event", sqlHistoricalIdentity(server, database)); !errors.Is(err, ErrSQLHistoricalFactsInvalid) {
			t.Fatal("unsupported actual clock precision accepted", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
