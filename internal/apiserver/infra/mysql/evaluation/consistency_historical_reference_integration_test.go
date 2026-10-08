//go:build integration

package evaluation

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/scheduler"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"gorm.io/gorm"
)

func TestConsistencyHistoricalReferencesPhysicalNullAcceptanceNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	record, wire := testCommittedReference(t, 9001, 42, "canonical-current-id")
	po := outcomeToPO(record)
	po.CommittedEventID, po.CommittedEventEvidence = nil, nil
	if err := db.Create(po).Error; err != nil {
		t.Fatal(err)
	}
	run := evidence.HistoricalRunReferenceV1{RunID: "42:1", Attempt: 1}
	appendOld := func(id string, class evidence.Class) {
		t.Helper()
		if err := db.Transaction(func(tx *gorm.DB) error {
			ctx := hostmysql.WithTx(t.Context(), tx)
			baseline, err := PrepareOutcomeHistoricalReferences(ctx, 9001, run)
			if err != nil {
				return err
			}
			binding, err := baseline.BindingSHA256("evaluation.outcome.committed", &run)
			if err != nil {
				return err
			}
			entry := nativeHistoricalEntry(id, "evaluation.outcome.committed", binding, &run)
			entry.Proof.Class = class
			if class == evidence.Unverifiable {
				entry.Proof.Verification.Reason = "full original body comparison unavailable"
			}
			return AppendOutcomeHistoricalReferences(ctx, baseline, entry)
		}); err != nil {
			t.Fatal(err)
		}
	}
	reader := NewConsistencyReadModel(db)
	check := func(class evidence.Class, detected int) {
		t.Helper()
		batch, err := reader.ReadBatch(t.Context(), 0, 2)
		if err != nil || len(batch.Items) != 1 {
			t.Fatal(batch, err)
		}
		item := batch.Items[0]
		if class == "" {
			if item.CommittedHistory != nil || item.Outbox == nil || item.Outbox.InvalidReason == "" {
				t.Fatal("unqualified native slot rescued by history", item)
			}
		} else if item.CommittedHistory == nil || item.CommittedHistory.Class != class || item.Outbox == nil || item.Outbox.Class != "" || item.Outbox.RowCount != 0 || !item.Outbox.LegacyCanonicalAbsent {
			t.Fatal("historical branch claimed standard fingerprint or lost classification", item)
		}
		result, err := scheduler.NewService(reader).AuditBatch(t.Context(), 0, 2)
		if err != nil || result.Detected != detected {
			t.Fatal("historical acceptance suppressed other checks or double-counted gaps", result, err)
		}
	}
	check("", 1)
	appendOld("old-verified-a", evidence.RetiredVerified)
	appendOld("old-verified-b", evidence.RetiredVerified)
	check(evidence.RetiredVerified, 0)
	appendOld("old-gap-c", evidence.Unverifiable)
	check(evidence.Unverifiable, 1)
	batch, err := reader.ReadBatch(t.Context(), 0, 2)
	if err != nil || len(batch.Items[0].CommittedHistory.Reasons) != 1 {
		t.Fatal("gap reason lost", err)
	}
	for _, query := range []string{"UPDATE evaluation_outcome SET committed_event_id='' WHERE id=9001", "UPDATE evaluation_outcome SET committed_event_id='dangling-id' WHERE id=9001", "UPDATE evaluation_outcome SET committed_event_id=NULL,committed_event_evidence=CAST('null' AS JSON) WHERE id=9001"} {
		if err := db.Exec(query).Error; err != nil {
			t.Fatal(err)
		}
		check("", 2) // Existing native mismatch plus the one retained per-ID gap.
	}
	standardJSON, err := json.Marshal(record.CommittedEventEvidence())
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE evaluation_outcome SET committed_event_id=NULL,committed_event_evidence=? WHERE id=9001", string(standardJSON)).Error; err != nil {
		t.Fatal(err)
	}
	check("", 2)
	if err := db.Exec("UPDATE evaluation_outcome SET committed_event_id=?,committed_event_evidence=? WHERE id=9001", record.CommittedEventID(), string(standardJSON)).Error; err != nil {
		t.Fatal(err)
	}
	check("", 2)
	if err := db.Exec("INSERT INTO rm_outbox(producer,message_id,destination,event_type,schema_version,scope,content_type,occurred_at,payload,fingerprint,state,next_attempt_at) VALUES(?,?,?,?,?,?,?,?,?,?,'pending',UTC_TIMESTAMP(6))", wire.Producer, wire.MessageID, wire.Destination, wire.EventType, wire.SchemaVersion, wire.Scope, wire.ContentType, wire.OccurredAt, wire.Payload, make([]byte, 32)).Error; err != nil {
		t.Fatal(err)
	}
	check("", 2)
	// Existing classified historical single-slot remains a separate, valid
	// contract. It does not report a standard row/fingerprint match.
	single := record.CommittedEventEvidence().Clone()
	single.Class = evidence.RetiredVerified
	single.Reference = nil
	single.Origin = "retirement"
	single.Digest = evidence.SourceDigest("original-legacy-row/v1", []byte("legacy-single"))
	single.Verification = evidence.Verification{Method: "original-owner-verifier", Version: "v1", OperationID: "123-1", VerifiedAt: time.Date(2026, 10, 8, 1, 2, 3, 456000000, time.UTC), BusinessTerminal: true, OwnershipVerified: true, ResponsibilityClosed: true}
	singleJSON, err := json.Marshal(single)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE evaluation_outcome SET committed_event_evidence=? WHERE id=9001", string(singleJSON)).Error; err != nil {
		t.Fatal(err)
	}
	batch, err = reader.ReadBatch(t.Context(), 0, 2)
	if err != nil || batch.Items[0].CommittedHistory != nil || batch.Items[0].Outbox.Class != evidence.RetiredVerified || batch.Items[0].Outbox.InvalidReason != "" || batch.Items[0].Outbox.RowCount != 0 {
		t.Fatal("valid historical single slot rejected or promoted to standard", batch, err)
	}
	single.BusinessBindingSHA256 = strings.Repeat("0", 64)
	singleJSON, err = json.Marshal(single)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE evaluation_outcome SET committed_event_evidence=? WHERE id=9001", string(singleJSON)).Error; err != nil {
		t.Fatal(err)
	}
	check("", 2)
}

func attachLifecycleHistory(t *testing.T, db *gorm.DB, id uint64, sourceID string) evidence.HistoricalReferenceEntryV1 {
	t.Helper()
	var entry evidence.HistoricalReferenceEntryV1
	if err := db.Transaction(func(tx *gorm.DB) error {
		ctx := hostmysql.WithTx(t.Context(), tx)
		baseline, err := PrepareAssessmentHistoricalReferences(ctx, id)
		if err != nil {
			return err
		}
		binding, err := baseline.BindingSHA256("evaluation.requested", nil)
		if err != nil {
			return err
		}
		entry = nativeHistoricalEntry(sourceID, "evaluation.requested", binding, nil)
		return AppendAssessmentHistoricalReferences(ctx, baseline, entry)
	}); err != nil {
		t.Fatal(err)
	}
	return entry
}

func requireHistoricalClassification(t *testing.T, db *gorm.DB, id uint64, count int, valid bool) {
	t.Helper()
	got, err := (&consistencyReadModel{db: db}).listHistoricalReferences(t.Context(), []uint64{id})
	if err != nil || len(got[id]) != count {
		t.Fatal("historical classification count", got, err)
	}
	for _, reference := range got[id] {
		if valid != (reference.InvalidReason == "") || (valid && reference.Class != evidence.RetiredVerified) {
			t.Fatal("historical source/owner classification", reference)
		}
	}
}

func TestConsistencyHistoricalReferencesPreserveNormalSaveNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	if err := db.Exec("UPDATE assessment SET evaluation_model_kind='scale',evaluation_model_code='SCALE-1',evaluation_model_version=NULL WHERE id=42").Error; err != nil {
		t.Fatal(err)
	}
	attachLifecycleHistory(t, db, 42, "normal-save-original")
	requireHistoricalClassification(t, db, 42, 1, true)
	repository := NewAssessmentRepository(db)
	record, err := repository.FindByID(t.Context(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Save(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	requireHistoricalClassification(t, db, 42, 1, true)
	batch, err := (&consistencyReadModel{db: db}).ReadBatchTo(t.Context(), 0, 42, 2)
	if err != nil || len(batch.Items) != 1 || len(batch.Items[0].HistoricalReferences) != 1 || batch.Items[0].Outbox != nil {
		t.Fatal("new history replaced existing current evidence", batch, err)
	}
	for _, query := range []string{"UPDATE assessment SET org_id=8 WHERE id=42", "UPDATE assessment SET org_id=7,questionnaire_code='q' WHERE id=42", "UPDATE assessment SET questionnaire_code='Q',evaluation_model_code='other-model' WHERE id=42"} {
		if err := db.Exec(query).Error; err != nil {
			t.Fatal(err)
		}
		requireHistoricalClassification(t, db, 42, 1, false)
	}
}

func TestConsistencyHistoricalReferencesStrictSourceAndSetNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	first := attachLifecycleHistory(t, db, 42, "original-source-a")
	second := first.Clone()
	second.EventID, second.Proof.EventID = "original-source-b", "original-source-b"
	second.Source.Digest = evidence.SourceDigest("mysql-original-source-row-v1", []byte(second.EventID))
	second.Proof.Digest = second.Source.Digest
	if err := db.Transaction(func(tx *gorm.DB) error {
		ctx := hostmysql.WithTx(t.Context(), tx)
		baseline, err := PrepareAssessmentHistoricalReferences(ctx, 42)
		if err != nil {
			return err
		}
		return AppendAssessmentHistoricalReferences(ctx, baseline, second)
	}); err != nil {
		t.Fatal(err)
	}
	// Two event IDs cannot describe the same original legacy primary key.
	requireHistoricalClassification(t, db, 42, 2, false)
	set := &evidence.HistoricalReferenceSetV1{Version: 1, Entries: []evidence.HistoricalReferenceEntryV1{first, first}}
	duplicate, err := json.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE assessment SET historical_lifecycle_evidence=? WHERE id=42", string(duplicate)).Error; err != nil {
		t.Fatal(err)
	}
	requireHistoricalClassification(t, db, 42, 1, false)
	if err := db.Exec("ALTER TABLE assessment DROP CHECK chk_assessment_historical_lifecycle").Error; err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(duplicate, &raw); err != nil {
		t.Fatal(err)
	}
	raw["unknown_field"] = "must never become a classified success"
	unknown, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE assessment SET historical_lifecycle_evidence=? WHERE id=42", string(unknown)).Error; err != nil {
		t.Fatal(err)
	}
	requireHistoricalClassification(t, db, 42, 1, false)
}

func TestConsistencyHistoricalReferencesOutcomeOriginalGraphNative(t *testing.T) {
	db := openHistoricalReferencesDB(t)
	insertHistoricalAssessment(t, db, 42)
	record, _ := testCommittedReference(t, 9001, 42, "live-standard-current")
	if err := NewOutcomeRepository(db).Save(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	run := evidence.HistoricalRunReferenceV1{RunID: "42:1", Attempt: 1}
	if err := db.Transaction(func(tx *gorm.DB) error {
		ctx := hostmysql.WithTx(t.Context(), tx)
		baseline, err := PrepareOutcomeHistoricalReferences(ctx, 9001, run)
		if err != nil {
			return err
		}
		binding, err := baseline.BindingSHA256("evaluation.outcome.committed", &run)
		if err != nil {
			return err
		}
		return AppendOutcomeHistoricalReferences(ctx, baseline, nativeHistoricalEntry("old-outcome-a", "evaluation.outcome.committed", binding, &run), nativeHistoricalEntry("old-outcome-b", "evaluation.outcome.committed", binding, &run))
	}); err != nil {
		t.Fatal(err)
	}
	requireHistoricalClassification(t, db, 42, 2, true)
	batch, err := (&consistencyReadModel{db: db}).ReadBatchTo(t.Context(), 0, 42, 2)
	if err != nil || len(batch.Items) != 1 || len(batch.Items[0].HistoricalReferences) != 2 || batch.Items[0].Outbox == nil || batch.Items[0].Outbox.Class != evidence.StandardReferenceClass || batch.Items[0].Outbox.InvalidReason == "" {
		t.Fatal("retirement conclusions bypassed missing standard wire message", batch, err)
	}
	if err := db.Exec("UPDATE runtime_checkpoint SET resource_id='42:replacement' WHERE assessment_id=42").Error; err != nil {
		t.Fatal(err)
	}
	requireHistoricalClassification(t, db, 42, 2, false)
	if err := db.Exec("UPDATE runtime_checkpoint SET resource_id='42:1' WHERE assessment_id=42").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE assessment SET org_id=8 WHERE id=42").Error; err != nil {
		t.Fatal(err)
	}
	requireHistoricalClassification(t, db, 42, 2, false)
	if err := db.Exec("UPDATE assessment SET org_id=7 WHERE id=42").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("UPDATE evaluation_outcome SET payload_json='{}' WHERE id=9001").Error; err != nil {
		t.Fatal(err)
	}
	requireHistoricalClassification(t, db, 42, 2, false)
}
