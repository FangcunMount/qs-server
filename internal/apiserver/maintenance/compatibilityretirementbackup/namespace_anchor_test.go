package compatibilityretirementbackup

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	identitymeta "github.com/FangcunMount/qs-server/internal/pkg/databaseidentity"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// Synthetic metadata verifies grammar/comparison only, never actual database
// permission, source approval, isolated restore success or writer authority.
func backupNamespaceFixture(t *testing.T) (Binding, bson.Raw, map[string]bson.Raw) {
	t.Helper()
	hello, err := bson.Marshal(bson.D{{Key: "setName", Value: "namespace_fixture_rs"}})
	if err != nil {
		t.Fatal("fixture_bson_failed")
	}
	collection, err := bson.Marshal(bson.D{{Key: "name", Value: "answersheets"}, {Key: "type", Value: "collection"}, {Key: "info", Value: bson.D{{Key: "uuid", Value: primitive.Binary{Subtype: 4, Data: bytes.Repeat([]byte{1}, 16)}}}}})
	if err != nil {
		t.Fatal("fixture_bson_failed")
	}
	endpoint, err := identitymeta.MongoEndpointSHA256("127.0.0.1", 34567, "namespace_fixture")
	if err != nil {
		t.Fatal("fixture_endpoint_failed")
	}
	anchor, err := identitymeta.MongoNamespaceAnchorFromMetadata(bson.Raw(hello), []bson.Raw{bson.Raw(collection)}, "namespace_fixture", endpoint)
	if err != nil {
		t.Fatal("fixture_anchor_failed")
	}
	b := Binding{IdentityHash: strings.Repeat("a", 64), AnchorHash: anchor.Hash, GenerationHash: strings.Repeat("b", 64), NamespaceAnchor: anchor, Version: 38, HeadMatch: true, IdentityMatch: true, MetadataComplete: true, ErrorCategory: "none"}
	return b, bson.Raw(hello), map[string]bson.Raw{"answersheets": bson.Raw(collection)}
}

func TestBackupNamespaceBindingClosedGrammarAndLegacyOmission(t *testing.T) {
	b, _, _ := backupNamespaceFixture(t)
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal("fixture_json_failed")
	}
	var decoded Binding
	if exactJSON(raw, &decoded) != nil || !identitymeta.MatchMongoNamespaceAnchors(b.NamespaceAnchor, decoded.NamespaceAnchor) {
		t.Fatal("new_profile_binding_rejected")
	}
	decoded.NamespaceAnchor.Collections[0].UUID = strings.Repeat("f", 32)
	if b.NamespaceAnchor.Collections[0].UUID == decoded.NamespaceAnchor.Collections[0].UUID {
		t.Fatal("decoded_binding_aliases_approval")
	}
	legacy := b
	legacy.NamespaceAnchor = nil
	legacy.AnchorHash = strings.Repeat("c", 64)
	old, err := json.Marshal(legacy)
	if err != nil || bytes.Contains(old, []byte("namespace_anchor")) {
		t.Fatal("legacy_wire_changed")
	}
	if exactJSON(old, &decoded) != nil || decoded.NamespaceAnchor != nil || decoded.AnchorHash != legacy.AnchorHash {
		t.Fatal("legacy_binding_not_preserved")
	}
	for _, name := range []string{"null_profile", "case_alias", "unknown_field", "wrong_anchor_hash", "generation_missing", "wrong_kind", "uuid_missing"} {
		t.Run(name, func(t *testing.T) {
			var v map[string]any
			if json.Unmarshal(raw, &v) != nil {
				t.Fatal("fixture_json_failed")
			}
			switch name {
			case "null_profile":
				v["namespace_anchor"] = nil
			case "case_alias":
				v["Namespace_Anchor"] = v["namespace_anchor"]
				delete(v, "namespace_anchor")
			case "unknown_field":
				v["unknown"] = true
			case "wrong_anchor_hash":
				v["database_anchor_hash"] = strings.Repeat("d", 64)
			case "generation_missing":
				v["migration_generation_hash"] = ""
			case "wrong_kind":
				v["namespace_anchor"].(map[string]any)["kind"] = identitymeta.MongoReplicaAnchorKind
			case "uuid_missing":
				delete(v["namespace_anchor"].(map[string]any)["collections"].([]any)[0].(map[string]any), "uuid")
			}
			malformed, err := json.Marshal(v)
			if err != nil || exactJSON(malformed, new(Binding)) == nil {
				t.Fatal("invalid_profile_accepted")
			}
		})
	}
	// A duplicate optional field cannot erase or replace the first profile.
	duplicated := append(append([]byte(nil), raw[:len(raw)-1]...), []byte(`,"namespace_anchor":null}`)...)
	if exactJSON(duplicated, new(Binding)) == nil {
		t.Fatal("duplicate_profile_accepted")
	}
}

func TestBackupNamespaceOriginalMetadataRejectsDrift(t *testing.T) {
	b, hello, collections := backupNamespaceFixture(t)
	if matchMongoNamespaceMetadata(hello, collections, "namespace_fixture", b) != nil {
		t.Fatal("matching_original_metadata_rejected")
	}
	for _, name := range []string{"database", "uuid_replacement", "present_to_absent", "map_name_alias", "replica_set", "approved_hash", "legacy_kind"} {
		t.Run(name, func(t *testing.T) {
			candidate := b
			candidate.NamespaceAnchor = b.NamespaceAnchor.Clone()
			rows := map[string]bson.Raw{}
			for k, v := range collections {
				rows[k] = append(bson.Raw(nil), v...)
			}
			database, observedHello := "namespace_fixture", hello
			switch name {
			case "database":
				database = "other_namespace"
			case "uuid_replacement":
				raw, err := bson.Marshal(bson.D{{Key: "name", Value: "answersheets"}, {Key: "type", Value: "collection"}, {Key: "info", Value: bson.D{{Key: "uuid", Value: primitive.Binary{Subtype: 4, Data: bytes.Repeat([]byte{2}, 16)}}}}})
				if err != nil {
					t.Fatal("fixture_bson_failed")
				}
				rows["answersheets"] = raw
			case "present_to_absent":
				delete(rows, "answersheets")
			case "map_name_alias":
				rows["alias"] = rows["answersheets"]
				delete(rows, "answersheets")
			case "replica_set":
				raw, err := bson.Marshal(bson.D{{Key: "setName", Value: "another_rs"}})
				if err != nil {
					t.Fatal("fixture_bson_failed")
				}
				observedHello = raw
			case "approved_hash":
				candidate.AnchorHash = strings.Repeat("e", 64)
			case "legacy_kind":
				candidate.NamespaceAnchor.Kind = identitymeta.MongoReplicaAnchorKind
			}
			if matchMongoNamespaceMetadata(observedHello, rows, database, candidate) != ErrIdentity {
				t.Fatal("original_namespace_drift_accepted")
			}
		})
	}
	if b.NamespaceAnchor.Collections[0].UUID != strings.Repeat("01", 16) {
		t.Fatal("fixture_approval_changed")
	}
}

func TestBackupNamespaceRestoreRequiresDistinctOriginalHandle(t *testing.T) {
	b, _, _ := backupNamespaceFixture(t)
	a := &Archive{data: manifest{Inventory: inventory{Bindings: map[string]Binding{"mongodb": b}}}}
	if _, err := RestoreMongo(context.Background(), nil, a); err != ErrIdentity {
		t.Fatal("new_profile_silently_used_legacy_restore")
	}
	if _, err := RestoreMongoWithOriginal(context.Background(), nil, nil, a); err != ErrIdentity {
		t.Fatal("missing_original_handle_accepted")
	}
	zero := &mongo.Database{}
	if _, err := RestoreMongoWithOriginal(context.Background(), zero, zero, a); err != ErrIdentity {
		t.Fatal("same_original_and_isolated_handle_accepted")
	}
	if _, err := RestoreMongoWithOriginal(nil, nil, nil, a); err != ErrBudget { //nolint:staticcheck // Nil context is the rejection case being tested.
		t.Fatal("nil_scope_accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := RestoreMongoWithOriginal(ctx, nil, nil, a); err != ErrBudget {
		t.Fatal("cancelled_scope_accepted")
	}
	if _, err := RestoreMongo(context.Background(), nil, nil); err != ErrRestore {
		t.Fatal("legacy_nil_restore_contract_changed")
	}
}
