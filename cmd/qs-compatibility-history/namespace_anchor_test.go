package main

import (
	"encoding/json"
	"os"
	"testing"

	identitymeta "github.com/FangcunMount/qs-server/internal/pkg/databaseidentity"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func historyNamespaceFixture(t *testing.T) *identitymeta.MongoNamespaceAnchor {
	t.Helper()
	hello, _ := bson.Marshal(bson.D{{Key: "setName", Value: "fixture-rs"}})
	row, _ := bson.Marshal(bson.D{{Key: "name", Value: "answersheets"}, {Key: "type", Value: "collection"}, {Key: "info", Value: bson.D{{Key: "uuid", Value: primitive.Binary{Subtype: 4, Data: []byte("1234567890abcdef")}}}}})
	endpoint, err := identitymeta.MongoEndpointSHA256("fixture-host", 27017, "fixture-business")
	if err != nil {
		t.Fatal("fixture_endpoint_failed")
	}
	a, err := identitymeta.MongoNamespaceAnchorFromMetadata(hello, []bson.Raw{row}, "fixture-business", endpoint)
	if err != nil {
		t.Fatal("fixture_anchor_failed")
	}
	return a
}
func TestHistoryNamespaceAnchorBindsTheOriginalApprovedInventoryTuple(t *testing.T) {
	testSource(t)
	for _, which := range []string{"match", "request_only", "report_only", "wrong_hash", "wrong_endpoint", "wrong_kind"} {
		t.Run(which, func(t *testing.T) {
			path, _, req := requestFixture(t)
			var inv inventoryRequest
			raw, err := os.ReadFile(req.InventoryRequest.Path)
			if err != nil || json.Unmarshal(raw, &inv) != nil {
				t.Fatal("fixture_inventory_failed")
			}
			var report inventoryReport
			raw, err = os.ReadFile(req.InventoryReport.Path)
			if err != nil || json.Unmarshal(raw, &report) != nil {
				t.Fatal("fixture_report_failed")
			}
			anchor := historyNamespaceFixture(t)
			inv.MongoNamespaceAnchor = anchor.Clone()
			binding := report.DatabaseBindings["mongodb"]
			binding.NamespaceAnchor = anchor.Clone()
			binding.DatabaseAnchorHash = anchor.Hash
			switch which {
			case "request_only":
				binding.NamespaceAnchor = nil
			case "report_only":
				inv.MongoNamespaceAnchor = nil
			case "wrong_hash":
				binding.DatabaseAnchorHash = report.DatabaseBindings["mongodb"].DatabaseAnchorHash
			case "wrong_endpoint":
				binding.NamespaceAnchor.EndpointSHA256 = report.DatabaseBindings["mongodb"].IdentityHash
			case "wrong_kind":
				binding.NamespaceAnchor.Kind = identitymeta.MongoReplicaAnchorKind
			}
			report.DatabaseBindings["mongodb"] = binding
			req.InventoryRequest.SHA256 = writeFixtureJSON(t, req.InventoryRequest.Path, inv)
			report.RequestHash = req.InventoryRequest.SHA256
			req.InventoryReport.SHA256 = writeFixtureJSON(t, req.InventoryReport.Path, report)
			hash := writeFixtureJSON(t, path, req)
			a, e := loadInputs(t.Context(), path, hash, "123-1", "125-1")
			if a != nil {
				if a.close() != nil {
					t.Fatal("fixture_close_failed")
				}
			}
			if which == "match" {
				if e != nil {
					t.Fatal(safeCategory(e))
				}
			} else if e == nil {
				t.Fatal("unapproved_namespace_tuple_accepted")
			}
		})
	}
}
