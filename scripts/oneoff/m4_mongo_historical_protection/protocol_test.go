package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const testSource = "1111111111111111111111111111111111111111"

func rawBSON(t *testing.T, d bson.D) []byte {
	t.Helper()
	r, e := bson.Marshal(d)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func idSpec(t *testing.T) []byte {
	return rawBSON(t, bson.D{{Key: "v", Value: int32(2)}, {Key: "key", Value: bson.D{{Key: "_id", Value: int32(1)}}}, {Key: "name", Value: "_id_"}})
}

// Exact ListIndexes BSON from the retained Mongo7.0.31 V7 fixture. Only
// the four index metadata documents are copied; no business BSON is included.
var original038ServerSpecs = [4]string{
	"e600000010760002000000036b65790036000000106c65676163795f7375626d697373696f6e5f65766964656e63652e656e74726965732e6576656e745f6964000100000000026e616d65002a000000756b5f616e7377657273686565745f6c65676163795f7375626d697373696f6e5f6576656e745f69640008756e697175650001037061727469616c46696c74657245787072657373696f6e0049000000036c65676163795f7375626d697373696f6e5f65766964656e63652e656e74726965732e6576656e745f696400170000000224747970650007000000737472696e6700000000",
	"8f00000010760002000000036b6579004c000000106c65676163795f7375626d697373696f6e5f65766964656e63652e76657273696f6e00010000001064656c657465645f6174000100000010646f6d61696e5f6964000100000000026e616d6500280000006964785f616e7377657273686565745f6c65676163795f7375626d697373696f6e5f61756469740000",
	"ee00000010760002000000036b6579003900000010686973746f726963616c5f67656e6572617465645f65766964656e63652e656e74726965732e6576656e745f6964000100000000026e616d65002c000000756b5f67656e65726174696f6e5f686973746f726963616c5f67656e6572617465645f6576656e745f69640008756e697175650001037061727469616c46696c74657245787072657373696f6e004c00000003686973746f726963616c5f67656e6572617465645f65766964656e63652e656e74726965732e6576656e745f696400170000000224747970650007000000737472696e6700000000",
	"a000000010760002000000036b6579005b00000010686973746f726963616c5f67656e6572617465645f65766964656e63652e76657273696f6e00010000001064656c657465645f6174000100000010737461747573000100000010646f6d61696e5f6964000100000000026e616d65002a0000006964785f67656e65726174696f6e5f686973746f726963616c5f67656e6572617465645f61756469740000",
}

func targetSpec(t *testing.T, i int) []byte {
	t.Helper()
	raw, e := hex.DecodeString(original038ServerSpecs[i])
	if e != nil {
		t.Fatal(e)
	}
	return raw
}

func fixtureMaterial(t *testing.T) material {
	t.Helper()
	count, ledgerCount := int64(17), int64(1)
	m := material{Format: 1, Kind: "m4_b_mongo_index_object_protection", ToolSource: testSource, BSource: bSource, BResources: bResources, BResourceFiles: bResourceFiles, CapturedAt: "2026-10-15T09:50:00Z", SQLName: "qs_owned", MongoName: "qs_owned", SQLIdentity: strings.Repeat("a", 64), MongoIdentity: strings.Repeat("b", 64), SQLVersion: 101}
	m.CapturePath = "/private/original-b-capture.json"
	for i, name := range []string{"answersheets", "report_generations"} {
		m.Collections[i] = namespaceProof{Name: name, UUID: bytes.Repeat([]byte{byte(i + 1)}, 16), Options: rawBSON(t, bson.D{}), Indexes: [][]byte{idSpec(t)}, Count: &count}
		for j, x := range targets {
			if x.collection == name {
				m.Collections[i].Indexes = append(m.Collections[i].Indexes, targetSpec(t, j))
			}
		}
	}
	m.Ledger = ledgerProof{Namespace: namespaceProof{Name: "schema_migrations", UUID: bytes.Repeat([]byte{3}, 16), Options: rawBSON(t, bson.D{}), Indexes: [][]byte{idSpec(t)}, Count: &ledgerCount}, Document: rawBSON(t, bson.D{{Key: "_id", Value: primitive.ObjectID{1}}, {Key: "dirty", Value: false}, {Key: "version", Value: int64(39)}})}
	return m
}
func cloneMaterial(t *testing.T, m material) material {
	t.Helper()
	raw, _ := json.Marshal(m)
	var out material
	if strictDecode(raw, &out) != nil {
		t.Fatal("fixture could not retain original BSON")
	}
	return out
}
func changeSpec(t *testing.T, raw []byte, key string, value any) []byte {
	t.Helper()
	var d bson.D
	if bson.Unmarshal(raw, &d) != nil {
		t.Fatal("fixture spec")
	}
	for i := range d {
		if d[i].Key == key {
			d[i].Value = value
			return rawBSON(t, d)
		}
	}
	return rawBSON(t, append(d, bson.E{Key: key, Value: value}))
}

func TestMaterialPreservesCompleteOrderedSpecsAndOriginalLedgerBSON(t *testing.T) {
	m := fixtureMaterial(t)
	if e := validateMaterial(m, testSource, "qs_owned", "qs_owned"); e != nil {
		t.Fatal(e)
	}
	copy := cloneMaterial(t, m)
	if !bytes.Equal(copy.Ledger.Document, m.Ledger.Document) || !bytes.Equal(copy.Collections[0].Indexes[2], m.Collections[0].Indexes[2]) {
		t.Fatal("material rewrote original _id/field order/integer width or index key order")
	}
	mutations := map[string]func(*material){
		"different_execution_source": func(m *material) { m.BSource = testSource },
		"current_main_resources":     func(m *material) { m.BResourceFiles = 286 },
		"different_tool":             func(m *material) { m.ToolSource = bSource },
		"SQL100":                     func(m *material) { m.SQLVersion = 100 },
		"SQL102":                     func(m *material) { m.SQLVersion = 102 },
		"missing_count":              func(m *material) { m.Collections[0].Count = nil },
		"negative_count":             func(m *material) { n := int64(-1); m.Collections[0].Count = &n },
		"repeated_UUID":              func(m *material) { m.Collections[1].UUID = m.Collections[0].UUID },
		"key_order": func(m *material) {
			m.Collections[0].Indexes[2] = changeSpec(t, m.Collections[0].Indexes[2], "key", bson.D{{Key: "domain_id", Value: int32(1)}, {Key: "deleted_at", Value: int32(1)}, {Key: "legacy_submission_evidence.version", Value: int32(1)}})
		},
		"unique_disabled": func(m *material) {
			m.Collections[0].Indexes[1] = changeSpec(t, m.Collections[0].Indexes[1], "unique", false)
		},
		"partial_filter": func(m *material) {
			m.Collections[0].Indexes[1] = changeSpec(t, m.Collections[0].Indexes[1], "partialFilterExpression", bson.D{{Key: targets[0].uniquePath, Value: bson.D{{Key: "$exists", Value: true}}}})
		},
		"TTL_added": func(m *material) {
			m.Collections[0].Indexes[1] = changeSpec(t, m.Collections[0].Indexes[1], "expireAfterSeconds", int32(1))
		},
		"missing_target": func(m *material) { m.Collections[0].Indexes = m.Collections[0].Indexes[:2] },
		"duplicate_index_name": func(m *material) {
			m.Collections[0].Indexes = append(m.Collections[0].Indexes, m.Collections[0].Indexes[1])
		},
		"non_simple_collection_collation": func(m *material) {
			m.Collections[0].Options = rawBSON(t, bson.D{{Key: "collation", Value: bson.D{{Key: "locale", Value: "en"}}}})
		},
		"ledger_options":      func(m *material) { m.Ledger.Namespace.Options = rawBSON(t, bson.D{{Key: "capped", Value: true}}) },
		"custom_ledger_index": func(m *material) { m.Ledger.Namespace.Indexes = append(m.Ledger.Namespace.Indexes, targetSpec(t, 0)) },
		"floating_version": func(m *material) {
			m.Ledger.Document = rawBSON(t, bson.D{{Key: "_id", Value: primitive.ObjectID{1}}, {Key: "version", Value: float64(39)}, {Key: "dirty", Value: false}})
		},
		"dirty_original": func(m *material) { m.Ledger.Document = changeSpec(t, m.Ledger.Document, "dirty", true) },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			bad := cloneMaterial(t, m)
			mutate(&bad)
			if validateMaterial(bad, testSource, "qs_owned", "qs_owned") == nil {
				t.Fatal("unreviewed metadata entered fixed B material")
			}
		})
	}
	zero := int64(0)
	m.Collections[0].Count = &zero
	if validateMaterial(m, testSource, "qs_owned", "qs_owned") != nil {
		t.Fatal("observed zero must differ from a missing count")
	}
}

func currentState(t *testing.T, m material, missing int, dirty bool) observation {
	t.Helper()
	c := cloneMaterial(t, m)
	c.Ledger.Namespace.UUID = bytes.Repeat([]byte{4}, 16)
	c.Ledger.Document = rawBSON(t, bson.D{{Key: "_id", Value: primitive.ObjectID{2}}, {Key: "version", Value: int32(40)}, {Key: "dirty", Value: dirty}})
	for i := 0; i < missing; i++ {
		for j := range c.Collections {
			var kept [][]byte
			for _, spec := range c.Collections[j].Indexes {
				name, _ := indexName(spec)
				if name != targets[i].name {
					kept = append(kept, spec)
				}
			}
			c.Collections[j].Indexes = kept
		}
	}
	return observation{SQLIdentity: m.SQLIdentity, MongoIdentity: m.MongoIdentity, SQLVersion: 101, Collections: c.Collections, Ledger: c.Ledger}
}
func TestRestorePlanOnlyAcceptsActualBPrefixAndProviderUUID(t *testing.T) {
	m := fixtureMaterial(t)
	for n := 0; n <= 4; n++ {
		plan, e := planRestore(m, currentState(t, m, n, true))
		if e != nil || len(plan) != n {
			t.Fatal("valid dirty40 prefix refused", n, e)
		}
		for i, position := range plan {
			if position != i {
				t.Fatal("plan left fixed compiled target order")
			}
		}
		cleanPlan, e := planRestore(m, currentState(t, m, n, false))
		if e != nil || !reflect.DeepEqual(plan, cleanPlan) {
			t.Fatal("clean40 reverse-restore prefix refused", n, e)
		}
	}
	mutations := map[string]func(*observation){
		"SQL_dirty":                 func(o *observation) { o.SQLDirty = true },
		"SQL102":                    func(o *observation) { o.SQLVersion = 102 },
		"identity":                  func(o *observation) { o.MongoIdentity = strings.Repeat("c", 64) },
		"same_original_ledger_UUID": func(o *observation) { o.Ledger.Namespace.UUID = m.Ledger.Namespace.UUID },
		"business_UUID":             func(o *observation) { o.Collections[0].UUID = bytes.Repeat([]byte{8}, 16) },
		"business_count":            func(o *observation) { n := int64(18); o.Collections[0].Count = &n },
		"business_options": func(o *observation) {
			o.Collections[0].Options = rawBSON(t, bson.D{{Key: "validationLevel", Value: "strict"}})
		},
		"present_index_changed": func(o *observation) {
			o.Collections[1].Indexes[1] = changeSpec(t, o.Collections[1].Indexes[1], "unique", false)
		},
		"unexpected_index": func(o *observation) {
			o.Collections[0].Indexes = append(o.Collections[0].Indexes, changeSpec(t, idSpec(t), "name", "unknown"))
		},
		"reverse_partial_restore": func(o *observation) {
			o.Collections[0].Indexes = append(o.Collections[0].Indexes, m.Collections[0].Indexes[1])
		},
	}
	for _, dirty := range []bool{true, false} {
		for name, mutate := range mutations {
			t.Run(fmt.Sprintf("dirty_%t/%s", dirty, name), func(t *testing.T) {
				o := currentState(t, m, 2, dirty)
				mutate(&o)
				if _, e := planRestore(m, o); e == nil {
					t.Fatal("unknown recovery shape accepted")
				}
			})
		}
	}
}

func TestRestoreApprovalWindowOwnershipAndFirstRuntimeBoundary(t *testing.T) {
	m := fixtureMaterial(t)
	now := time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC)
	dir, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	a := approval{Format: 1, Kind: "m4_b_pre_start_mongo_object_restore", Operation: "123-1", ToolSource: testSource, BSource: bSource, BResources: bResources, BResourceFiles: bResourceFiles, MaterialHash: strings.Repeat("c", 64), FreezeHash: strings.Repeat("d", 64), ResponsibilityHash: strings.Repeat("e", 64), SQLIdentity: m.SQLIdentity, MongoIdentity: m.MongoIdentity, RuntimeFrozen: true, RuntimeNotStarted: true, Marker: filepath.Join(dir, "first-runtime"), WindowStarted: now.Add(-20 * time.Minute).Format(time.RFC3339), RecoveryDecision: now.Add(-time.Minute).Format(time.RFC3339), Deadline: now.Add(30 * time.Minute).Format(time.RFC3339), Expires: now.Add(time.Hour).Format(time.RFC3339), RetentionDays: 30, DataOwner: "system (10001)", RecoveryOwner: "system (10001)"}
	if e := validateApproval(a, m, a.MaterialHash, testSource, now); e != nil {
		t.Fatal(e)
	}
	mutations := map[string]func(*approval){
		"material_self_authorization": func(a *approval) { a.MaterialHash = strings.Repeat("f", 64) },
		"wrong_scope":                 func(a *approval) { a.BSource = testSource },
		"no_freeze":                   func(a *approval) { a.RuntimeFrozen = false },
		"new_runtime_issued":          func(a *approval) { a.RuntimeNotStarted = false },
		"owner":                       func(a *approval) { a.RecoveryOwner = "unknown" },
		"retention29":                 func(a *approval) { a.RetentionDays = 29 },
		"window_after_capture":        func(a *approval) { a.WindowStarted = now.Format(time.RFC3339) },
		"over60m":                     func(a *approval) { a.Deadline = now.Add(time.Hour).Format(time.RFC3339) },
		"expired":                     func(a *approval) { a.Expires = now.Add(-time.Second).Format(time.RFC3339) },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			bad := a
			mutate(&bad)
			if validateApproval(bad, m, a.MaterialHash, testSource, now) == nil {
				t.Fatal("unapproved restoration accepted")
			}
		})
	}
	if e := os.WriteFile(a.Marker, []byte("any content means start intent exists"), 0600); e != nil {
		t.Fatal(e)
	}
	if validateApproval(a, m, a.MaterialHash, testSource, now) == nil {
		t.Fatal("existing first-runtime marker did not stop recovery")
	}
}

func TestEntryRejectsInvalidInputsBeforeConnectionsAndDoesNotEcho(t *testing.T) {
	base := []string{"--mysql-database=qs_owned", "--mongo-database=qs_owned"}
	for _, args := range [][]string{base, append(append([]string{}, base...), "--action=secret-value-that-must-not-leak")} {
		var out bytes.Buffer
		// Missing connection input fails before any DB operation. Rejected flags
		// and invalid connection strings never enter the structured receipt.
		if serve(context.Background(), args, func(string) string { return "secret-value-that-must-not-leak" }, testSource, &out) == 0 {
			t.Fatal("invalid input completed")
		}
		var r receipt
		if json.Unmarshal(out.Bytes(), &r) != nil || r.Complete || r.FreezeCertified || strings.Contains(out.String(), "secret-value") {
			t.Fatal("input leaked or freeze certified")
		}
	}

	raw, _ := json.Marshal(fixtureMaterial(t))
	raw = append(raw[:len(raw)-1], []byte(`,"business_document_body":"forbidden"}`)...)
	var m material
	if strictDecode(raw, &m) == nil || strictDecode([]byte(`{} {}`), &m) == nil {
		t.Fatal("unowned fields or trailing documents entered the format")
	}
}
