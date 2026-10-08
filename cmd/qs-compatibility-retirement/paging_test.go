package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func v2Request(kind string) request {
	return request{FormatVersion: 2, Kind: kind, OperationID: "123-1", SourceSHA: sourceSHA, TargetHash: digest(targets), DatabaseScope: "mysql-and-mongodb", Identities: map[string]string{"mysql": strings.Repeat("1", 64), "mongodb": strings.Repeat("2", 64)}, Migrations: map[string]uint64{"mysql": 98, "mongodb": 37}, Limits: productionLimits()}
}
func TestMongoTokensKeepExactBSONTypesAndRefuseUnsupported(t *testing.T) {
	for _, id := range []any{"cursor-private-id", primitive.NewObjectID(), int32(31), int64(2147483649)} {
		raw, e := bson.Marshal(bson.D{{Key: "_id", Value: id}})
		if e != nil {
			t.Fatal(e)
		}
		token, kind, e := mongoToken(raw)
		if e != nil {
			t.Fatal(e)
		}
		decoded, e := base64.StdEncoding.DecodeString(token)
		if e != nil || string(decoded) != string(raw) {
			t.Fatal("BSON source cursor changed")
		}
		actual, actualKind, e := decodeMongoToken(decoded)
		if e != nil || actual != id || actualKind != kind {
			t.Fatal("BSON cursor type changed")
		}
	}
	for _, id := range []any{nil, true, float64(1), bson.D{{Key: "compound", Value: 1}}, bson.A{1}, ""} {
		raw, e := bson.Marshal(bson.D{{Key: "_id", Value: id}})
		if e != nil {
			t.Fatal(e)
		}
		if _, _, e = decodeMongoToken(raw); e == nil {
			t.Fatal("unsupported cursor accepted")
		}
	}
}
func TestV2BoundDiscoveryNeverApprovesInventory(t *testing.T) {
	sourceSHA = strings.Repeat("a", 40)
	dir := t.TempDir()
	if e := os.Chmod(dir, 0700); e != nil {
		t.Fatal(e)
	}
	r := v2Request("readonly_inventory_boundary_request")
	write := func(name string, r request) error {
		raw, e := json.Marshal(r)
		if e != nil {
			t.Fatal(e)
		}
		p := filepath.Join(dir, name)
		if e = os.WriteFile(p, raw, 0600); e != nil {
			t.Fatal(e)
		}
		_, e = readRequest(p, digestRaw(raw), r.OperationID)
		return e
	}
	if e := write("boundary-request.json", r); e != nil {
		t.Fatal(e)
	}
	r.Kind = "readonly_inventory_request"
	if e := write("inventory-request.json", r); e == nil {
		t.Fatal("auto-approved observed boundary")
	}
	r.Kind = "readonly_inventory_boundary_request"
	r.Limits.PageSize = 2000
	if e := write("boundary-request.json", r); e == nil {
		t.Fatal("arbitrary page allowance accepted")
	}
	r.Limits = productionLimits()
	r.BoundaryRunID = "456-1"
	if e := write("boundary-request.json", r); e == nil {
		t.Fatal("mixed input classes accepted")
	}
}
func TestProductionEntryCannotUseV1OrMixedClass(t *testing.T) {
	if productionRequestClass(request{FormatVersion: 1, Kind: "readonly_inventory_request"}, "inventory") == nil {
		t.Fatal("production v1 bypass accepted")
	}
	for _, mode := range []string{"bounds", "inventory"} {
		r := v2Request("readonly_inventory_request")
		if mode == "bounds" {
			r.Kind = "readonly_inventory_boundary_request"
		}
		if e := productionRequestClass(r, mode); e != nil {
			t.Fatal(e)
		}
		other := "bounds"
		if mode == "bounds" {
			other = "inventory"
		}
		if productionRequestClass(r, other) == nil {
			t.Fatal("mixed classes accepted")
		}
	}
}
func TestNumericSQLCursorUsesNumericDriverValue(t *testing.T) {
	for _, tc := range []struct {
		kind, source string
		expect       any
	}{{"uint64", "18446744073709551615", uint64(^uint64(0))}, {"int64", "-9223372036854775808", int64(-9223372036854775808)}, {"ascii_string", "f49bb460-1945-42c8-bf12-6f1c979c02ab", "f49bb460-1945-42c8-bf12-6f1c979c02ab"}} {
		got, e := sqlToken(base64.StdEncoding.EncodeToString([]byte(tc.source)), tc.kind)
		if e != nil || got != tc.expect {
			t.Fatal("SQL key cursor coerced or overflowed")
		}
	}
}
func TestSourceAssetsAndCheckpointsCannotOverwriteOrExpose(t *testing.T) {
	dir := t.TempDir()
	b := targetBoundary{Database: "mongodb", Name: "domain_event_outbox", UpperToken: "PRIVATE_CURSOR_SENTINEL"}
	ctx := context.WithValue(context.Background(), scanRunKey{}, scanRunIdentity{OperationID: "123-1", RunID: "456-1", RequestHash: strings.Repeat("a", 64)})
	if e := registerSource(context.Background(), dir, "other.bsonframes", "test", b); e == nil {
		t.Fatal("unbound source copy registered")
	}
	if e := registerSource(ctx, dir, "source.bsonframes", "test", b); e != nil {
		t.Fatal(e)
	}
	if e := registerSource(ctx, dir, "source.bsonframes", "test", b); e == nil {
		t.Fatal("asset ownership overwritten")
	}
	raw, e := os.ReadFile(filepath.Join(dir, "source.bsonframes.asset.json"))
	if e != nil || !strings.Contains(string(raw), "purge_required_after_acceptance") {
		t.Fatal("temporary copy not registered")
	}
	r := report{Targets: []snapshot{{Boundary: &b, SourceFile: "PRIVATE_SOURCE_SENTINEL"}}}
	public, _ := json.Marshal(safeSummary(r))
	if strings.Contains(string(public), "PRIVATE_") {
		t.Fatal("private cursor/source leaked")
	}
}

func TestEncodedSourceBytesCannotBypassRawBudget(t *testing.T) {
	var dst bytes.Buffer
	w := &boundedSourceWriter{dst: &dst, limit: 3}
	if n, e := w.Write([]byte("abc")); e != nil || n != 3 {
		t.Fatal("exact output bound rejected")
	}
	if _, e := w.Write([]byte("d")); !errors.Is(e, errSourceFileBound) || dst.String() != "abc" {
		t.Fatal("encoded byte bound bypassed")
	}
}
