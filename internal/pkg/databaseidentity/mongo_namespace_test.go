package databaseidentity

import (
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func namespaceRaw(t *testing.T, value any) bson.Raw {
	t.Helper()
	raw, err := bson.Marshal(value)
	if err != nil {
		t.Fatal("fixture_bson_failed")
	}
	return raw
}
func namespaceFixture(t *testing.T) (bson.Raw, []bson.Raw, string) {
	t.Helper()
	hello := namespaceRaw(t, bson.D{{Key: "setName", Value: "native-fixture"}, {Key: "hosts", Value: bson.A{"a:27017", "b:27017"}}, {Key: "me", Value: "a:27017"}})
	endpoint, err := MongoEndpointSHA256("127.0.0.1", 34567, "selected-fixture")
	if err != nil {
		t.Fatal("fixture_endpoint_failed")
	}
	rows := []bson.Raw{}
	for i, name := range keptMongoNames {
		uuid := make([]byte, 16)
		uuid[0] = byte(i + 1)
		rows = append(rows, namespaceRaw(t, bson.D{{Key: "name", Value: name}, {Key: "type", Value: "collection"}, {Key: "info", Value: bson.D{{Key: "uuid", Value: primitive.Binary{Subtype: 4, Data: uuid}}}}}))
	}
	return hello, rows, endpoint
}
func TestMongoNamespaceAnchorBindsExactKeptObjectsAndEndpoint(t *testing.T) {
	hello, rows, endpoint := namespaceFixture(t)
	a, err := MongoNamespaceAnchorFromMetadata(hello, rows, "selected-fixture", endpoint)
	if err != nil || a.Validate() != nil || a.Kind != MongoNamespaceAnchorKind {
		t.Fatal("actual_metadata_not_bound")
	}
	for _, mutate := range []func(*MongoNamespaceAnchor){
		func(x *MongoNamespaceAnchor) { x.Kind = MongoReplicaAnchorKind },
		func(x *MongoNamespaceAnchor) { x.Database = "wrong" },
		func(x *MongoNamespaceAnchor) { x.EndpointSHA256 = strings.Repeat("b", 64) },
		func(x *MongoNamespaceAnchor) { x.ReplicaSetName = "wrong" },
		func(x *MongoNamespaceAnchor) { x.Collections[0].UUID = strings.Repeat("a", 32) },
		func(x *MongoNamespaceAnchor) { x.Collections[0].Present = false; x.Collections[0].UUID = "" },
		func(x *MongoNamespaceAnchor) { x.Collections[0], x.Collections[1] = x.Collections[1], x.Collections[0] },
		func(x *MongoNamespaceAnchor) { x.Collections[0].Name = "domain_event_outbox" },
	} {
		other := a.Clone()
		mutate(other)
		if MatchMongoNamespaceAnchors(a, other) || other.Validate() == nil {
			t.Fatal("tampered_binding_accepted")
		}
	}
	clone := a.Clone()
	clone.Collections[0].UUID = "changed"
	if a.Collections[0].UUID == "changed" || !MatchMongoNamespaceAnchors(a, a.Clone()) {
		t.Fatal("mutable_alias_or_equal_rejected")
	}
	if MatchMongoNamespaceAnchors(a, nil) || MatchMongoNamespaceAnchors(nil, a) || !MatchMongoNamespaceAnchors(nil, nil) {
		t.Fatal("nil_profile_semantics_changed")
	}
}
func TestMongoNamespaceAnchorAbsenceIsExplicitAndAtLeastOneExists(t *testing.T) {
	hello, rows, endpoint := namespaceFixture(t)
	a, err := MongoNamespaceAnchorFromMetadata(hello, rows[:1], "selected-fixture", endpoint)
	if err != nil || a.Validate() != nil || len(a.Collections) != 4 || !a.Collections[0].Present {
		t.Fatal("exact_sparse_observation_failed")
	}
	for _, row := range a.Collections[1:] {
		if row.Present || row.UUID != "" {
			t.Fatal("absence_not_explicit")
		}
	}
	if _, err = MongoNamespaceAnchorFromMetadata(hello, nil, "selected-fixture", endpoint); err == nil {
		t.Fatal("empty_anchor_accepted")
	}
}
func TestMongoNamespaceAnchorRejectsUnsupportedMetadata(t *testing.T) {
	hello, rows, endpoint := namespaceFixture(t)
	for _, bad := range []bson.Raw{
		namespaceRaw(t, bson.D{{Key: "name", Value: keptMongoNames[0]}, {Key: "type", Value: "view"}}),
		namespaceRaw(t, bson.D{{Key: "name", Value: keptMongoNames[0]}, {Key: "name", Value: keptMongoNames[0]}, {Key: "type", Value: "collection"}}),
		namespaceRaw(t, bson.D{{Key: "name", Value: keptMongoNames[0]}, {Key: "type", Value: "collection"}, {Key: "info", Value: bson.D{{Key: "uuid", Value: primitive.Binary{Subtype: 0, Data: make([]byte, 16)}}}}}),
		namespaceRaw(t, bson.D{{Key: "name", Value: keptMongoNames[0]}, {Key: "type", Value: "collection"}, {Key: "info", Value: bson.D{{Key: "uuid", Value: primitive.Binary{Subtype: 4, Data: make([]byte, 16)}}}}}),
		bson.Raw{1, 2, 3},
	} {
		if _, err := MongoNamespaceAnchorFromMetadata(hello, []bson.Raw{bad}, "selected-fixture", endpoint); err == nil {
			t.Fatal("unsupported_catalog_accepted")
		}
	}
	if _, err := MongoNamespaceAnchorFromMetadata(hello, append(rows, rows[0]), "selected-fixture", endpoint); err == nil {
		t.Fatal("duplicate_name_accepted")
	}
	for _, h := range []bson.Raw{
		namespaceRaw(t, bson.D{{Key: "isWritablePrimary", Value: true}}),
		namespaceRaw(t, bson.D{{Key: "setName", Value: "fixture"}, {Key: "msg", Value: "isdbgrid"}}),
		namespaceRaw(t, bson.D{{Key: "setName", Value: "fixture"}, {Key: "setName", Value: "other"}}),
	} {
		if _, err := MongoNamespaceAnchorFromMetadata(h, rows, "selected-fixture", endpoint); err == nil {
			t.Fatal("unsupported_hello_accepted")
		}
	}
	if _, err := MongoEndpointSHA256("", 34567, "selected-fixture"); err == nil {
		t.Fatal("missing_endpoint_accepted")
	}
	if _, err := MongoEndpointSHA256("host", 0, "selected-fixture"); err == nil {
		t.Fatal("invalid_endpoint_accepted")
	}
}
func TestMongoNamespaceAnchorExcludesPrimaryAndMigrationCollectionGeneration(t *testing.T) {
	hello, rows, endpoint := namespaceFixture(t)
	a, err := MongoNamespaceAnchorFromMetadata(hello, rows, "selected-fixture", endpoint)
	if err != nil {
		t.Fatal("fixture_failed")
	}
	newHello := namespaceRaw(t, bson.D{{Key: "setName", Value: "native-fixture"}, {Key: "hosts", Value: bson.A{"b:27017", "a:27017"}}, {Key: "me", Value: "b:27017"}})
	newRows := append([]bson.Raw(nil), rows...)
	newRows = append(newRows, namespaceRaw(t, bson.D{{Key: "name", Value: "schema_migrations"}, {Key: "type", Value: "collection"}, {Key: "info", Value: bson.D{{Key: "uuid", Value: primitive.Binary{Subtype: 4, Data: []byte("1234567890abcdef")}}}}}))
	b, err := MongoNamespaceAnchorFromMetadata(newHello, newRows, "selected-fixture", endpoint)
	if err != nil || !MatchMongoNamespaceAnchors(a, b) {
		t.Fatal("mutable_generation_or_primary_changed_anchor")
	}
	// The legacy identity_v1 still contains migration UUID and is unchanged by
	// this helper. This hash equality never permits accepting a new legacy head.
}
func TestMongoNamespaceFramingGolden(t *testing.T) {
	hello, rows, endpoint := namespaceFixture(t)
	a, err := MongoNamespaceAnchorFromMetadata(hello, rows, "selected-fixture", endpoint)
	if err != nil {
		t.Fatal("fixture_failed")
	}
	parts := []string{MongoNamespaceAnchorKind, endpoint, "selected-fixture", "native-fixture"}
	for i, name := range keptMongoNames {
		uuid := make([]byte, 16)
		uuid[0] = byte(i + 1)
		parts = append(parts, name, "present", hex.EncodeToString(uuid))
	}
	if endpoint != "13705f4e7c62572a312b9456f8b64e66d87c744eceb929c87eb307639c66ca5c" || a.Hash != "fa5cb5044106930af4e2743e4345a3ea28f870e707a13da5d871062b4a4d03d7" || a.Hash != framedHash(parts...) {
		t.Fatal("raw_length_framing_changed")
	}
}

func TestMongoNamespaceAnchorJSONRejectsMissingAndDuplicateFields(t *testing.T) {
	hello, rows, endpoint := namespaceFixture(t)
	a, err := MongoNamespaceAnchorFromMetadata(hello, rows, "selected-fixture", endpoint)
	if err != nil {
		t.Fatal("fixture_failed")
	}
	raw, err := json.Marshal(a)
	if err != nil {
		t.Fatal("fixture_json_failed")
	}
	var decoded MongoNamespaceAnchor
	if json.Unmarshal(raw, &decoded) != nil || !MatchMongoNamespaceAnchors(a, &decoded) {
		t.Fatal("valid_json_rejected")
	}
	for _, bad := range []string{
		strings.Replace(string(raw), `"present":true,`, "", 1),
		strings.Replace(string(raw), `"present":true`, `"present":true,"present":true`, 1),
		strings.Replace(string(raw), `"present":true`, `"Present":true`, 1),
		strings.Replace(string(raw), `"present":true`, `"present":null`, 1),
		strings.Replace(string(raw), `"uuid":`, `"extra":false,"uuid":`, 1),
		string(raw) + "{}",
	} {
		if json.Unmarshal([]byte(bad), &decoded) == nil {
			t.Fatal("incomplete_or_ambiguous_json_accepted")
		}
	}
}

func TestMongoNamespaceOptionalJSONCannotSilentlyBecomeLegacy(t *testing.T) {
	hello, rows, endpoint := namespaceFixture(t)
	a, err := MongoNamespaceAnchorFromMetadata(hello, rows, "selected-fixture", endpoint)
	if err != nil {
		t.Fatal("fixture_failed")
	}
	good, err := json.Marshal(map[string]any{"mongodb_namespace_anchor": a})
	if err != nil {
		t.Fatal("fixture_json_failed")
	}
	if ValidateOptionalMongoNamespaceJSON(good, "mongodb_namespace_anchor") != nil || ValidateOptionalMongoNamespaceJSON([]byte(`{}`), "mongodb_namespace_anchor") != nil {
		t.Fatal("valid_profile_or_missing_legacy_rejected")
	}
	for _, bad := range []string{
		`{"mongodb_namespace_anchor":null}`,
		strings.Replace(string(good), "mongodb_namespace_anchor", "MongoDB_namespace_anchor", 1),
		`{"mongo_anchor_profile":null}`,
		`{"mongo_anchor_profile":""}`,
		`{"Mongo_anchor_profile":"selected_namespace_kept_uuids_v1"}`,
	} {
		field := "mongodb_namespace_anchor"
		if strings.Contains(strings.ToLower(bad), "mongo_anchor_profile") {
			field = "mongo_anchor_profile"
		}
		if ValidateOptionalMongoNamespaceJSON([]byte(bad), field) == nil {
			t.Fatal("explicit_invalid_profile_became_legacy")
		}
	}
}
