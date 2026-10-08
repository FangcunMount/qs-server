package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequestClassAndLimitsCannotBecomeDropPermit(t *testing.T) {
	sourceSHA = strings.Repeat("a", 40)
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	r := request{FormatVersion: 1, Kind: "readonly_inventory_request", OperationID: "123-1", SourceSHA: sourceSHA, TargetHash: digest(targets), DatabaseScope: "mysql-and-mongodb", Identities: map[string]string{"mysql": strings.Repeat("1", 64), "mongodb": strings.Repeat("2", 64)}, Migrations: map[string]uint64{"mysql": 95, "mongodb": 36}}
	r.Limits.QuerySeconds = querySeconds
	r.Limits.TotalSeconds = totalSeconds
	r.Limits.MaxRecords = maxRecords
	r.Limits.MaxBytes = maxBytes
	for _, change := range []func(*request){func(r *request) {}, func(r *request) { r.Kind = "retirement_manifest" }, func(r *request) { r.TargetHash = strings.Repeat("0", 64) }, func(r *request) { r.SourceSHA = strings.Repeat("b", 40) }, func(r *request) { r.Limits.TotalSeconds = 3600 }, func(r *request) { r.Identities["extra"] = strings.Repeat("3", 64) }} {
		copy := r
		copy.Identities = map[string]string{"mysql": r.Identities["mysql"], "mongodb": r.Identities["mongodb"]}
		change(&copy)
		b, _ := json.Marshal(copy)
		path := filepath.Join(dir, "inventory-request.json")
		if err := os.WriteFile(path, b, 0600); err != nil {
			t.Fatal(err)
		}
		_, err := readRequest(path, digestRaw(b), "123-1")
		if copy.Kind == r.Kind && copy.SourceSHA == r.SourceSHA && copy.TargetHash == r.TargetHash && copy.Limits == r.Limits && len(copy.Identities) == 2 {
			if err != nil {
				t.Fatal(err)
			}
		} else if err == nil {
			t.Fatal("invalid request accepted")
		}
	}
}

func TestSourceFrameSeparatesNullEmptyAndColumnBoundaries(t *testing.T) {
	var a, b, c strings.Builder
	frame(&a, nil, true)
	frame(&b, nil, false)
	frame(&c, []byte("ab"), false)
	frame(&c, []byte("c"), false)
	var d strings.Builder
	frame(&d, []byte("a"), false)
	frame(&d, []byte("bc"), false)
	if a.String() == b.String() || c.String() == d.String() {
		t.Fatal("ambiguous source byte frame")
	}
}

func TestDuplicateJSONRequestFieldsRejectAtEveryDepth(t *testing.T) {
	for _, value := range []string{`{"kind":"a","kind":"b"}`, `{"identity_hashes":{"mysql":"a","mysql":"b"}}`, `{"limits":{"max_records":1,"max_records":2}}`} {
		if rejectDuplicateJSON([]byte(value)) == nil {
			t.Fatal("duplicate key accepted")
		}
	}
	if e := rejectDuplicateJSON([]byte(`{"kind":"a","identity_hashes":{"mysql":"a","mongodb":"b"}}`)); e != nil {
		t.Fatal(e)
	}
}

func TestSafeSummaryDoesNotContainPrivateSource(t *testing.T) {
	r := report{DatabaseBindings: map[string]databaseInventory{}, Targets: []snapshot{{Database: "mysql", Name: "ai_bridge_commands", SourceFile: "TEST_PRIVATE_PAYLOAD_SENTINEL"}}}
	b, _ := json.Marshal(safeSummary(r))
	if strings.Contains(string(b), "TEST_PRIVATE_PAYLOAD_SENTINEL") {
		t.Fatal("private source field exposed")
	}
	if safeSummary(r)["drop_ready"] != false {
		t.Fatal("inventory permits drop")
	}
}

func TestPrivateOutputCannotOverwriteAnotherRun(t *testing.T) {
	p := filepath.Join(t.TempDir(), "inventory.private.json")
	if e := writeJSON(p, map[string]int{"first": 1}); e != nil {
		t.Fatal(e)
	}
	if e := writeJSON(p, map[string]int{"second": 2}); e == nil {
		t.Fatal("overwrote evidence")
	}
	info, e := os.Stat(p)
	if e != nil || info.Mode().Perm() != 0600 {
		t.Fatal("output privacy changed")
	}
}

func TestIdentityRequestContainsNoExpectedIdentityAndUsesSeparateClass(t *testing.T) {
	sourceSHA = strings.Repeat("a", 40)
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	r := identityRequest{FormatVersion: 1, Kind: "readonly_identity_discovery_request", SourceSHA: sourceSHA, OperationID: "123-1", TargetHash: digest(targets), DatabaseScope: "mysql-and-mongodb", Protocols: identityProtocols()}
	r.Limits.QuerySeconds, r.Limits.TotalSeconds = 15, 90
	for i, mutate := range []func(*identityRequest){func(*identityRequest) {}, func(r *identityRequest) { r.Kind = "readonly_inventory_request" }, func(r *identityRequest) { r.Limits.TotalSeconds = 30 }, func(r *identityRequest) { r.SourceSHA = strings.Repeat("b", 40) }} {
		changed := r
		mutate(&changed)
		raw, _ := json.Marshal(changed)
		p := filepath.Join(dir, "identity-request.json")
		if e := os.WriteFile(p, raw, 0600); e != nil {
			t.Fatal(e)
		}
		e := readIdentityRequest(p, digestRaw(raw), "123-1")
		if (i == 0) != (e == nil) {
			t.Fatal("identity class or bounds not enforced")
		}
	}
	raw, _ := json.Marshal(r)
	var value map[string]any
	_ = json.Unmarshal(raw, &value)
	value["identity_hashes"] = map[string]string{"mysql": strings.Repeat("1", 64)}
	raw, _ = json.Marshal(value)
	p := filepath.Join(dir, "identity-request.json")
	if e := os.WriteFile(p, raw, 0600); e != nil {
		t.Fatal(e)
	}
	if e := readIdentityRequest(p, digestRaw(raw), "123-1"); e == nil {
		t.Fatal("discovery accepted supplied expected identity")
	}
}

func TestDiagnosticLabelsNeverPassUnknownSourceText(t *testing.T) {
	b := bucket("PRIVATE_UNKNOWN_TYPE", "PRIVATE_UNKNOWN_STATE", 7)
	raw, _ := json.Marshal(b)
	if strings.Contains(string(raw), "PRIVATE") || b.TypeLabel != "unknown_type" || b.StateLabel != "unknown_state" || !hashRE.MatchString(b.TypeHash) || !hashRE.MatchString(b.StateHash) {
		t.Fatal("unknown source label leaked")
	}
	if typeLabel("footprint.entry_opened") != "footprint.entry_opened" || typeLabel("footprint.injected_unknown") != "unknown_type" {
		t.Fatal("historical type allowlist is not exact")
	}
}
