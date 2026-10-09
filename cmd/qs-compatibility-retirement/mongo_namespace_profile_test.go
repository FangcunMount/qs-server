package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	identitymeta "github.com/FangcunMount/qs-server/internal/pkg/databaseidentity"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func namespaceCommandFixture(t *testing.T) *identitymeta.MongoNamespaceAnchor {
	t.Helper()
	hello := anchorRaw(t, bson.D{{Key: "setName", Value: "fixture-rs"}})
	raw := anchorRaw(t, bson.D{{Key: "name", Value: "answersheets"}, {Key: "type", Value: "collection"}, {Key: "info", Value: bson.D{{Key: "uuid", Value: primitive.Binary{Subtype: 4, Data: []byte("1234567890abcdef")}}}}})
	endpoint, err := identitymeta.MongoEndpointSHA256("fixture-host", 27017, "fixture-business")
	if err != nil {
		t.Fatal("fixture_endpoint_failed")
	}
	a, err := identitymeta.MongoNamespaceAnchorFromMetadata(hello, []bson.Raw{raw}, "fixture-business", endpoint)
	if err != nil {
		t.Fatal("fixture_anchor_failed")
	}
	return a
}

func TestIdentityProfileIsChosenByApprovedRequestBytesBeforeConnection(t *testing.T) {
	previous := sourceSHA
	sourceSHA = strings.Repeat("a", 40)
	t.Cleanup(func() { sourceSHA = previous })
	d := t.TempDir()
	if os.Chmod(d, 0700) != nil {
		t.Fatal("fixture_mode_failed")
	}
	for _, profile := range []string{"", identitymeta.MongoReplicaAnchorKind, identitymeta.MongoNamespaceAnchorKind, "unknown"} {
		r := identityRequest{FormatVersion: 1, Kind: "readonly_identity_discovery_request", SourceSHA: sourceSHA, OperationID: "123-1", TargetHash: digest(targets), DatabaseScope: "mysql-and-mongodb", Protocols: identityProtocols(), MongoAnchorProfile: profile}
		r.Limits.QuerySeconds = 15
		r.Limits.TotalSeconds = 90
		raw, err := json.Marshal(r)
		if err != nil {
			t.Fatal("fixture_encode_failed")
		}
		path := filepath.Join(d, "identity-request.json")
		if os.WriteFile(path, raw, 0600) != nil {
			t.Fatal("fixture_write_failed")
		}
		loaded, e := loadIdentityRequest(path, digestRaw(raw), "123-1")
		if profile == "unknown" {
			if e == nil {
				t.Fatal("unknown_profile_accepted")
			}
			continue
		}
		if e != nil || loaded.MongoAnchorProfile != profile {
			t.Fatal("approved_profile_not_preserved")
		}
		if _, e = loadIdentityRequest(path, strings.Repeat("f", 64), "123-1"); e == nil {
			t.Fatal("unapproved_request_accepted")
		}
		if _, e = loadIdentityRequest(path, digestRaw(raw), "124-1"); e == nil {
			t.Fatal("wrong_operation_accepted")
		}
	}
}

func TestNamespaceInventoryRequestDoesNotChangeIdentityV1OrAcceptLegacyMix(t *testing.T) {
	previous := sourceSHA
	sourceSHA = strings.Repeat("a", 40)
	t.Cleanup(func() { sourceSHA = previous })
	d := t.TempDir()
	if os.Chmod(d, 0700) != nil {
		t.Fatal("fixture_mode_failed")
	}
	a := namespaceCommandFixture(t)
	r := request{FormatVersion: 2, Kind: "readonly_inventory_boundary_request", OperationID: "123-1", SourceSHA: sourceSHA, TargetHash: digest(targets), DatabaseScope: "mysql-and-mongodb", Identities: map[string]string{"mysql": strings.Repeat("b", 64), "mongodb": strings.Repeat("c", 64)}, Migrations: map[string]uint64{"mysql": 99, "mongodb": 38}, Limits: productionLimits(), MongoNamespaceAnchor: a}
	path := filepath.Join(d, "boundary-request.json")
	raw, err := json.Marshal(r)
	if err != nil || os.WriteFile(path, raw, 0600) != nil {
		t.Fatal("fixture_write_failed")
	}
	loaded, e := readRequest(path, digestRaw(raw), r.OperationID)
	if e != nil || loaded.Identities["mongodb"] != r.Identities["mongodb"] || !identitymeta.MatchMongoNamespaceAnchors(a, loaded.MongoNamespaceAnchor) {
		t.Fatal("namespace_request_not_bound")
	}
	r.MongoNamespaceAnchor.Kind = identitymeta.MongoReplicaAnchorKind
	raw, err = json.Marshal(r)
	if err != nil || os.WriteFile(path, raw, 0600) != nil {
		t.Fatal("fixture_write_failed")
	}
	if _, e = readRequest(path, digestRaw(raw), r.OperationID); e == nil {
		t.Fatal("legacy_profile_mixed_with_namespace_uuids")
	}
}
