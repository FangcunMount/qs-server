package compatibilityretirementbackup

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func targetBSQLOnlyFixture() (TargetBRecoveryResumeRequest, *targetJournalSnapshot) {
	r, s := targetBReconcileFixture()
	var result targetBMigrationResult
	_ = json.Unmarshal(s.files["target-recovery-b-migration-result.json"], &result)
	result.State = "partial_or_unknown"
	result.Observation = nil
	result.Attempt.MongoAttempted = false
	result.Attempt.MongoResponse = "not_attempted"
	result.Attempt.MongoReturnedVersion = 0
	result.Attempt.MongoChanged = false
	result.Attempt.Completion = "partial_or_unknown"
	s.files["target-recovery-b-migration-result.json"], _ = json.Marshal(result)
	r.MigrationResultSHA256 = sha(s.files["target-recovery-b-migration-result.json"])
	return r, s
}

func TestBSQLOnlyGrammarDoesNotBecomeCompletedOrUnknownAuthority(t *testing.T) {
	r, s := targetBSQLOnlyFixture()
	if _, _, e := targetBReconcileInputKind(r, s, targetBJournalSQLOnly); e != nil {
		t.Fatal(e)
	}
	if e := targetValidateBMigrationJournal(s); e != ErrRecoveryUnknown {
		t.Fatal("ordinary complete reader adopted partial migration")
	}
	if _, _, e := targetBReconcileInputKind(r, s, targetBJournalKind(255)); e != ErrRecoveryBinding {
		t.Fatal("unknown private kind accepted")
	}
	for _, test := range []struct {
		name   string
		change func(*targetBMigrationResult)
	}{
		{"sql_unknown", func(r *targetBMigrationResult) { r.Attempt.SQLResponse = "error_or_unknown" }},
		{"sql_head99", func(r *targetBMigrationResult) { r.Attempt.SQLReturnedVersion = 99 }},
		{"sql_no_change", func(r *targetBMigrationResult) { r.Attempt.SQLChanged = false }},
		{"mongo_started_unknown", func(r *targetBMigrationResult) {
			r.Attempt.MongoAttempted = true
			r.Attempt.MongoResponse = "error_or_unknown"
		}},
		{"mongo39", func(r *targetBMigrationResult) { r.Attempt.MongoReturnedVersion = 39 }},
		{"mongo_changed", func(r *targetBMigrationResult) { r.Attempt.MongoChanged = true }},
		{"missing_sql_response", func(r *targetBMigrationResult) { r.Attempt.SQLAttempted = false }},
		{"completed_claim", func(r *targetBMigrationResult) { r.Attempt.Completion = "completed" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			r, s := targetBSQLOnlyFixture()
			var x targetBMigrationResult
			_ = json.Unmarshal(s.files["target-recovery-b-migration-result.json"], &x)
			test.change(&x)
			s.files["target-recovery-b-migration-result.json"], _ = json.Marshal(x)
			r.MigrationResultSHA256 = sha(s.files["target-recovery-b-migration-result.json"])
			if _, _, e := targetBReconcileInputKind(r, s, targetBJournalSQLOnly); e != ErrRecoveryUnknown {
				t.Fatalf("category=%v", e)
			}
		})
	}
	if e := targetValidateBMigrationJournalKind(&targetJournalSnapshot{files: map[string][]byte{}}, targetBJournalSQLOnly); e != ErrRecoveryHead {
		t.Fatal("ordinary journal re-labelled SQL-only")
	}
}

func TestBMigrationIntentRequiresActualOriginalUUID(t *testing.T) {
	for _, kind := range []targetBJournalKind{targetBJournalComplete, targetBJournalSQLOnly} {
		for _, mode := range []string{"missing", "wrong", "unknown_type"} {
			t.Run(mode+string(rune('0'+kind)), func(t *testing.T) {
				var s *targetJournalSnapshot
				if kind == targetBJournalSQLOnly {
					_, s = targetBSQLOnlyFixture()
				} else {
					s = targetBJournalFixture()
				}
				var intent targetBMigrationIntent
				raw := s.files["target-recovery-b-migration-intent.json"]
				_ = json.Unmarshal(raw, &intent)
				field := `"mongodb_migration_uuid":"` + intent.MongoMigrationUUID + `"`
				switch mode {
				case "missing":
					s.files["target-recovery-b-migration-intent.json"] = []byte(strings.Replace(string(raw), field+",", "", 1))
				case "wrong":
					intent.MongoMigrationUUID = strings.Repeat("9", 32)
					s.files["target-recovery-b-migration-intent.json"], _ = json.Marshal(intent)
				case "unknown_type":
					s.files["target-recovery-b-migration-intent.json"] = []byte(strings.Replace(string(raw), field, `"mongodb_migration_uuid":1`, 1))
				}
				if e := targetValidateBMigrationJournalKind(s, kind); e != ErrRecoveryJournal {
					t.Fatal("unbound original UUID accepted")
				}
			})
		}
	}
}

func TestBOriginalUUIDComesFromActualCatalogBinary(t *testing.T) {
	uuid := make([]byte, 16)
	uuid[0] = 1
	raw, e := bson.Marshal(bson.D{{Key: "info", Value: bson.D{{Key: "uuid", Value: primitive.Binary{Subtype: 4, Data: uuid}}}}})
	if e != nil {
		t.Fatal(e)
	}
	got, e := targetActualMigrationUUID(map[string]bson.Raw{"schema_migrations": raw})
	if e != nil || got != "01000000000000000000000000000000" {
		t.Fatal("native ordered UUID missing")
	}
	for _, value := range []any{strings.Repeat("1", 32), primitive.Binary{Subtype: 0, Data: uuid}, primitive.Binary{Subtype: 4, Data: uuid[:15]}} {
		raw, e := bson.Marshal(bson.D{{Key: "info", Value: bson.D{{Key: "uuid", Value: value}}}})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = targetActualMigrationUUID(map[string]bson.Raw{"schema_migrations": raw}); e != ErrIdentity {
			t.Fatal("string/unknown native UUID accepted")
		}
	}
	if _, e = targetActualMigrationUUID(map[string]bson.Raw{}); e != ErrIdentity {
		t.Fatal("missing catalog identity accepted")
	}
}

func TestBSQLOnlyProducerStillRequiresOriginalNativeMaterial(t *testing.T) {
	if _, e := ReconcileTargetBSQLOnlyRecovery(context.Background(), nil, TargetRecoveryBorrowed{}, TargetBRecoveryResumeRequest{}, "/not-used", nil); e != ErrRecoveryBinding {
		t.Fatal("material supplied authority without native handles")
	}
	if _, e := InspectTargetBSQLOnlyRecoveryJournal(context.Background(), nil, TargetRecoveryRequest{}, "/not-used", nil); e != ErrRecoveryBinding {
		t.Fatal("physical metadata authority was fabricated")
	}
}
