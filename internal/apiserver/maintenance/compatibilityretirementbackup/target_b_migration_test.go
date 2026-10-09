package compatibilityretirementbackup

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/migration"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func targetBTestSchema(t *testing.T, id byte) map[string]any {
	t.Helper()
	raw, e := bson.Marshal(bson.D{{Key: "name", Value: "schema_migrations"}, {Key: "type", Value: "collection"}, {Key: "options", Value: bson.D{}}, {Key: "info", Value: bson.D{{Key: "readOnly", Value: false}, {Key: "uuid", Value: primitive.Binary{Subtype: 4, Data: []byte{id, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}}}}}})
	if e != nil {
		t.Fatal(e)
	}
	canonical, e := canonicalMongo(raw)
	if e != nil {
		t.Fatal(e)
	}
	return map[string]any{"collection:schema_migrations": canonical, "indexes:schema_migrations": []any{map[string]any{"name": "_id_", "key": map[string]any{"_id": 1}}}, "collection:answersheets": map[string]any{"name": "answersheets", "type": "collection", "info": map[string]any{"uuid": "same-owned-business-uuid"}}}
}
func TestBGenerationNormalizationAllowsOnlyMigrationUUID(t *testing.T) {
	first := targetBTestSchema(t, 1)
	raw, _ := json.Marshal(first)
	second := targetBTestSchema(t, 2)
	a, e := targetBStableMongoNonTarget(first)
	if e != nil {
		t.Fatal(e)
	}
	b, e := targetBStableMongoNonTarget(second)
	if e != nil || a != b {
		t.Fatal("exact driver generation change rejected")
	}
	after, _ := json.Marshal(first)
	if string(raw) != string(after) {
		t.Fatal("original metadata mutated")
	}
	for _, test := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"retained_uuid", func(s map[string]any) {
			s["collection:answersheets"].(map[string]any)["info"] = map[string]any{"uuid": "changed"}
		}},
		{"retained_namespace", func(s map[string]any) { delete(s, "collection:answersheets") }},
		{"migration_options", func(s map[string]any) {
			s["collection:schema_migrations"].(map[string]any)["options"] = map[string]any{"collation": map[string]any{"locale": "en"}}
		}},
		{"migration_index", func(s map[string]any) { s["indexes:schema_migrations"] = []any{map[string]any{"name": "another"}} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := targetBTestSchema(t, 3)
			test.change(s)
			h, e := targetBStableMongoNonTarget(s)
			if e == nil && h == a {
				t.Fatal("non-generation change normalized away")
			}
		})
	}
	broken := targetBTestSchema(t, 4)
	broken["collection:schema_migrations"].(map[string]any)["info"].(map[string]any)["uuid"] = map[string]any{"$binary": map[string]any{"subType": "00", "base64": "AA=="}}
	if _, e = targetBStableMongoNonTarget(broken); e != ErrStructure {
		t.Fatal("unknown generation accepted")
	}
}
func targetBJournalFixture() *targetJournalSnapshot {
	r := TargetRecoveryRequest{SourceSHA: strings.Repeat("a", 40), OperationID: "123-1", OriginalRunID: "124-1", ActualRunID: "125-1", ManifestSHA256: strings.Repeat("b", 64), ArchiveSHA256: strings.Repeat("c", 64), SQLNonTargetSHA256: strings.Repeat("d", 64), MongoNonTargetSHA256: strings.Repeat("e", 64), SQLHead: 99, MongoHead: 38}
	binding := migration.CompatibilityPairRunBinding{ApprovedSourceSHA: strings.Repeat("f", 40), OriginalSourceSHA: r.SourceSHA, OperationID: r.OperationID, OriginalRunID: r.OriginalRunID, ActualRunID: r.ActualRunID, ManifestSHA256: r.ManifestSHA256, ArchiveSHA256: r.ArchiveSHA256, RequestSHA256: jsonSHA(r), WindowStartSHA256: strings.Repeat("0", 64), ResourcesSHA256: migration.CompatibilityMigrationResourcesSHA256()}
	before, after := strings.Repeat("1", 32), strings.Repeat("2", 32)
	intent := targetBMigrationIntent{Binding: binding, SQLIdentitySHA256: strings.Repeat("3", 64), MongoIdentitySHA256: strings.Repeat("4", 64), MongoGenerationSHA256: parts("mongodb_migration_generation_v1", before), MongoMigrationUUID: before, MongoStableSchemaSHA256: strings.Repeat("5", 64)}
	raw, _ := json.Marshal(intent)
	o := migration.CompatibilityPairMigrationObservation{Binding: binding, SQLBefore: 99, SQLAfter: 100, MongoBefore: 38, MongoAfter: 39, SQLIdentitySHA256: intent.SQLIdentitySHA256, MongoBeforeIdentitySHA256: intent.MongoIdentitySHA256, MongoAfterIdentitySHA256: strings.Repeat("6", 64), MongoBeforeGenerationSHA256: intent.MongoGenerationSHA256, MongoAfterGenerationSHA256: parts("mongodb_migration_generation_v1", after), MongoBeforeMigrationUUID: before, MongoAfterMigrationUUID: after, SQLChanged: true, MongoChanged: true}
	result := targetBMigrationResult{IntentSHA256: sha(raw), State: "completed", Attempt: migration.CompatibilityPairRunAttempt{SQLAttempted: true, SQLResponse: "success", SQLReturnedVersion: 100, SQLChanged: true, MongoAttempted: true, MongoResponse: "success", MongoReturnedVersion: 39, MongoChanged: true, Completion: "completed"}, Observation: &o}
	rr, _ := json.Marshal(result)
	return &targetJournalSnapshot{request: r, windowStart: binding.WindowStartSHA256, files: map[string][]byte{"target-recovery-b-migration-intent.json": raw, "target-recovery-b-migration-result.json": rr}}
}
func TestBPhysicalJournalNeverAdoptsPartialOrDifferentGeneration(t *testing.T) {
	if e := targetValidateBMigrationJournal(targetBJournalFixture()); e != nil {
		t.Fatal(e)
	}
	if !targetBMigrationJournalPresent(targetBJournalFixture()) || targetBMigrationJournalPresent(&targetJournalSnapshot{files: map[string][]byte{}}) {
		t.Fatal("B physical report must not enter legacy cross-process rebind")
	}
	// Passing this physical grammar proves no actual statement or authority.
	for _, test := range []struct {
		name   string
		mutate func(*targetJournalSnapshot)
		want   error
	}{
		{"missing_result", func(s *targetJournalSnapshot) { delete(s.files, "target-recovery-b-migration-result.json") }, ErrRecoveryUnknown},
		{"missing_intent", func(s *targetJournalSnapshot) { delete(s.files, "target-recovery-b-migration-intent.json") }, ErrRecoveryJournal},
		{"other_window", func(s *targetJournalSnapshot) { s.windowStart = strings.Repeat("9", 64) }, ErrRecoveryJournal},
		{"wrong_source", func(s *targetJournalSnapshot) { s.request.SourceSHA = strings.Repeat("9", 40) }, ErrRecoveryJournal},
		{"head_plus_one", func(s *targetJournalSnapshot) { s.request.SQLHead = 100 }, ErrRecoveryJournal},
		{"partial_sql_only", func(s *targetJournalSnapshot) {
			var r targetBMigrationResult
			_ = json.Unmarshal(s.files["target-recovery-b-migration-result.json"], &r)
			r.State = "partial_or_unknown"
			r.Observation = nil
			r.Attempt.MongoResponse = "error_or_unknown"
			r.Attempt.Completion = "partial_or_unknown"
			s.files["target-recovery-b-migration-result.json"], _ = json.Marshal(r)
		}, ErrRecoveryUnknown},
		{"same_old_uuid", func(s *targetJournalSnapshot) {
			var r targetBMigrationResult
			_ = json.Unmarshal(s.files["target-recovery-b-migration-result.json"], &r)
			r.Observation.MongoAfterMigrationUUID = r.Observation.MongoBeforeMigrationUUID
			s.files["target-recovery-b-migration-result.json"], _ = json.Marshal(r)
		}, ErrRecoveryJournal},
		{"false_mongo_response", func(s *targetJournalSnapshot) {
			var r targetBMigrationResult
			_ = json.Unmarshal(s.files["target-recovery-b-migration-result.json"], &r)
			r.Attempt.MongoAttempted = false
			s.files["target-recovery-b-migration-result.json"], _ = json.Marshal(r)
		}, ErrRecoveryJournal},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := targetBJournalFixture()
			test.mutate(s)
			if e := targetValidateBMigrationJournal(s); e != test.want {
				t.Fatalf("category=%v expected=%v", e, test.want)
			}
		})
	}
}
func TestBRecoveryNoJSONOrHeadIncrementCapability(t *testing.T) {
	if targetSupportedHead(100, 99) || targetSupportedHead(39, 38) || !targetSupportedHead(99, 99) || !targetSupportedHead(38, 38) {
		t.Fatal("same-head recovery boundary changed")
	}
	if _, e := RunTargetBMigration(context.Background(), nil, strings.Repeat("a", 40)); e != ErrRecoveryBinding {
		t.Fatal("missing process plan accepted")
	}
	var proof migration.CompatibilityPairMigrationProof
	if proof.VerifyAfter(context.Background(), nil, nil) == nil {
		t.Fatal("physical observation reconstructed a proof")
	}
	if targetValidateBMigrationJournal(&targetJournalSnapshot{files: map[string][]byte{}}) != nil {
		t.Fatal("old exact journal changed")
	}
}
