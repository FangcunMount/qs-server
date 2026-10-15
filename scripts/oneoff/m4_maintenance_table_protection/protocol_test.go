package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"go.mongodb.org/mongo-driver/bson"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

const testTool = "1111111111111111111111111111111111111111"

func testBinding() binding {
	return binding{"qs", "qs", strings.Repeat("1", 64), strings.Repeat("2", 64), strings.Repeat("3", 64)}
}
func originalRow(i int, key []string) rowProof {
	s := scope[i]
	r := rowProof{Key: key, Values: make([]cell, len(s.columns))}
	for n, c := range s.columns {
		v := "0"
		switch {
		case strings.HasPrefix(c.Type, "varchar"), strings.HasPrefix(c.Type, "char"):
			v = ""
		case c.Type == "longtext":
			v = "{ \"z\": 1, \"a\":\"四字节😀\" }"
		case c.Type == "datetime(6)":
			v = "2026-10-15 00:01:02.123456"
		}
		r.Values[n] = cell{Bytes: []byte(v)}
		if c.Nullable {
			r.Values[n] = cell{SQLNull: true}
		}
	}
	for n, k := range s.keys {
		for j, c := range s.columns {
			if c.Name == k {
				raw := []byte(key[n])
				if n == 0 && s.binaryFirst {
					raw, _ = hex.DecodeString(key[n])
				}
				r.Values[j] = cell{Bytes: raw}
			}
		}
	}
	r.Hash = rowHash(s.columns, r.Values)
	return r
}
func originalMaterial() material {
	raw, _ := bson.Marshal(bson.D{{Key: "_id", Value: "original"}, {Key: "version", Value: int64(40)}, {Key: "dirty", Value: false}})
	m := material{Format: 1, Kind: "m4_d_maintenance_table_protection", ToolSource: testTool, DSource: dSource, DResources: dResources, DResourceFiles: dResourceFiles, CapturedAt: time.Now().UTC().Format(time.RFC3339Nano), CapturePath: "/private/d/original.json", Binding: testBinding(), SessionZone: "+00:00", SessionCharset: "utf8mb4", SessionSQLMode: sessionMode, LedgerDDL: "CREATE TABLE `schema_migrations` (version bigint NOT NULL, dirty tinyint(1) NOT NULL)", Ledger: []ledgerRow{{102, false}}, MongoHead: raw}
	for i, s := range scope {
		key := []string{"41"}
		if len(s.keys) == 2 {
			key = append(key, "0")
		}
		if !s.binaryFirst {
			key = []string{"18446744073709551615"}
		}
		r := originalRow(i, key)
		encoded, _ := json.Marshal(r)
		m.Tables[i] = tableProof{Table: s.name, DDL: "CREATE TABLE `" + s.name + "` (original)", Columns: s.columns, Keys: s.keys, Rows: []rowProof{r}, Pages: 2, EOF: true, EncodedRowsBytes: len(encoded) + 1}
	}
	return m
}
func cloneMaterial(m material) material {
	raw, _ := json.Marshal(m)
	var c material
	_ = json.Unmarshal(raw, &c)
	return c
}
func TestMaterialKeepsServerBytesAndNullFrames(t *testing.T) {
	m := originalMaterial()
	if e := validateMaterial(m, testTool, testBinding()); e != nil {
		t.Fatal(e)
	}
	r := m.Tables[3].Rows[0]
	want := append([]byte(nil), r.Values[4].Bytes...)
	raw, _ := json.Marshal(m)
	var decoded material
	if e := strictDecode(raw, &decoded); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(decoded.Tables[3].Rows[0].Values[4].Bytes, want) || decoded.Tables[1].Rows[0].Values[3].SQLNull != true || decoded.Tables[2].Rows[0].Values[3].SQLNull || len(decoded.Tables[2].Rows[0].Values[3].Bytes) != 0 {
		t.Fatal("server LONGTEXT/NULL/empty changed")
	}
	if !bytes.Equal(decoded.Tables[3].Rows[0].Values[6].Bytes, []byte("2026-10-15 00:01:02.123456")) {
		t.Fatal("server temporal representation changed")
	}
	a := []column{{"value", "longtext", true}}
	if rowHash(a, []cell{{SQLNull: true}}) == rowHash(a, []cell{{Bytes: []byte{}}}) {
		t.Fatal("SQL NULL/empty collision")
	}
	if rowHash(a, []cell{{Bytes: []byte("null")}}) == rowHash(a, []cell{{Bytes: []byte("[]")}}) {
		t.Fatal("original JSON encodings collided")
	}
}
func TestMaterialRejectsIncompleteOrReboundInputs(t *testing.T) {
	changes := map[string]func(*material){"wrong_source": func(m *material) { m.DSource = testTool }, "wrong_resources": func(m *material) { m.DResources = strings.Repeat("4", 64) }, "dirty": func(m *material) { m.Ledger[0].Dirty = true }, "duplicate_ledger": func(m *material) { m.Ledger = append(m.Ledger, m.Ledger[0]) }, "missing_eof": func(m *material) { m.Tables[0].EOF = false }, "wrong_pages": func(m *material) { m.Tables[1].Pages = 1 }, "swapped_columns": func(m *material) {
		m.Tables[2].Columns[0], m.Tables[2].Columns[1] = m.Tables[2].Columns[1], m.Tables[2].Columns[0]
	}, "value_changed": func(m *material) { m.Tables[3].Rows[0].Values[4].Bytes = []byte("{}") }, "key_value_mismatch": func(m *material) { m.Tables[4].Rows[0].Key = []string{"42"} }, "oversized_cell": func(m *material) { m.Tables[4].Rows[0].Values[2].Bytes = bytes.Repeat([]byte{'x'}, maxValueBytes+1) }, "wrong_count": func(m *material) { m.Tables[1].EncodedRowsBytes++ }, "duplicate_pk": func(m *material) { m.Tables[0].Rows = append(m.Tables[0].Rows, m.Tables[0].Rows[0]) }}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			m := cloneMaterial(originalMaterial())
			change(&m)
			if validateMaterial(m, testTool, testBinding()) == nil {
				t.Fatal("accepted invalid original material")
			}
		})
	}
}
func TestCompositeBinaryKeysAndUint64Edges(t *testing.T) {
	s := scope[0]
	keys := [][]string{{"", "0"}, {"", "18446744073709551615"}, {"41", "0"}, {"61", "0"}, {"F09F9880", "0"}}
	for i, k := range keys {
		if !validKey(s, k) || validateRow(s, originalRow(0, k)) != nil {
			t.Fatalf("valid key %v", k)
		}
		if i > 0 && !keyLess(s, keys[i-1], k) {
			t.Fatal("binary/decimal order lost")
		}
	}
	for _, k := range [][]string{{"41", "00"}, {"41", "-1"}, {"41", "18446744073709551616"}, {"aA", "0"}, {"0", "0"}} {
		if validKey(s, k) {
			t.Fatal("noncanonical key accepted")
		}
	}
}
func TestMongo40StrictRawLedger(t *testing.T) {
	base := bson.D{{Key: "_id", Value: "ledger"}, {Key: "version", Value: int64(40)}, {Key: "dirty", Value: false}}
	cases := []bson.D{base, {{Key: "_id", Value: "ledger"}, {Key: "version", Value: int32(40)}, {Key: "dirty", Value: false}}, {{Key: "_id", Value: nil}, {Key: "version", Value: int64(40)}, {Key: "dirty", Value: false}}, {{Key: "_id", Value: "ledger"}, {Key: "version", Value: float64(40)}, {Key: "dirty", Value: false}}, {{Key: "_id", Value: "ledger"}, {Key: "version", Value: int64(40)}}, {{Key: "_id", Value: "ledger"}, {Key: "version", Value: int64(40)}, {Key: "dirty", Value: false}, {Key: "other", Value: 1}}, {{Key: "_id", Value: "ledger"}, {Key: "version", Value: int64(40)}, {Key: "dirty", Value: true}}, {{Key: "_id", Value: "ledger"}, {Key: "dirty", Value: false}, {Key: "dirty", Value: false}}}
	for i, c := range cases {
		raw, _ := bson.Marshal(c)
		if cleanMongo40(raw) != (i < 2) {
			t.Fatalf("strict typed ledger case%d", i)
		}
	}
}
func TestMaterialStrictJSONRejectsUnknownDuplicateAndTrailing(t *testing.T) {
	for _, raw := range []string{`{"format_version":1,"unknown":true}`, `{"format_version":1,"format_version":1}`, `{"format_version":1} {}`} {
		var m material
		if strictDecode([]byte(raw), &m) == nil {
			t.Fatal("ambiguous schema accepted")
		}
	}
}
func TestTargetVerificationDoesNotClaimSurvivingWholeDatabase(t *testing.T) {
	m := originalMaterial()
	target := cloneMaterial(m)
	target.Ledger = []ledgerRow{{103, false}}
	target.Tables = [5]tableProof{}
	if verifyTarget(m, target) != nil {
		t.Fatal("clean103 removal rejected")
	}
	target.Ledger[0].Dirty = true
	if verifyTarget(m, target) == nil {
		t.Fatal("dirty accepted")
	}
	target.Ledger[0].Dirty = false
	target.Tables[4].Table = scope[4].name
	if verifyTarget(m, target) == nil {
		t.Fatal("reappeared object accepted")
	}
}
func privateFixture(t *testing.T) string {
	t.Helper()
	dir, e := os.MkdirTemp("", "m4-d-private-")
	if e != nil {
		t.Fatal(e)
	}
	real, e := filepath.EvalSymlinks(dir)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = os.RemoveAll(real) })
	return real
}
func TestPrivateCaptureOneTimeAndParentIdentity(t *testing.T) {
	dir := privateFixture(t)
	path := filepath.Join(dir, "original.json")
	parent, e := privateParent(path)
	if e != nil {
		t.Fatal(e)
	}
	st, _ := parent.Stat()
	uid := st.Sys().(*syscall.Stat_t).Uid
	_ = parent.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	raw := []byte(`{"original":"bytes"}`)
	if finishCapture(ctx, path, raw, st, uid) != nil {
		t.Fatal("capture failed")
	}
	if got, e := readPrivate(path, fingerprint(raw)); e != nil || !bytes.Equal(got, raw) {
		t.Fatal("readback failed")
	}
	if finishCapture(ctx, path, []byte("new"), st, uid) == nil {
		t.Fatal("overwrote original")
	}
	if finishCapture(ctx, filepath.Join(dir, "wrongowner.json"), raw, st, uid+1) == nil {
		t.Fatal("owner changed accepted")
	}
	moved := dir + "-old"
	if e := os.Rename(dir, moved); e != nil {
		t.Fatal(e)
	}
	defer func() { _ = os.RemoveAll(moved) }()
	if e := os.Mkdir(dir, 0700); e != nil {
		t.Fatal(e)
	}
	if finishCapture(ctx, filepath.Join(dir, "replaced.json"), raw, st, uid) == nil {
		t.Fatal("parent replaced accepted")
	}
	if _, e := os.Stat(filepath.Join(dir, "replaced.json")); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("wrote after parent replaced")
	}
}
func TestPrivateMaterialRejectsHashAndSymlink(t *testing.T) {
	dir := privateFixture(t)
	path := filepath.Join(dir, "material.json")
	raw := []byte("original")
	if writeExclusive(path, raw) != nil {
		t.Fatal("write")
	}
	if _, e := readPrivate(path, strings.Repeat("0", 64)); e == nil {
		t.Fatal("zero hash")
	}
	if _, e := readPrivate(path, strings.Repeat("4", 64)); e == nil {
		t.Fatal("wrong hash")
	}
	link := filepath.Join(dir, "alias.json")
	if e := os.Symlink(path, link); e != nil {
		t.Fatal(e)
	}
	if _, e := readPrivate(link, fingerprint(raw)); e == nil {
		t.Fatal("symlink")
	}
}

func TestFrozenDInventoryMatchesOriginalEmbeddedFiles(t *testing.T) {
	root := filepath.Join("..", "..", "..", "internal", "pkg", "migration", "migrations")
	var inventory [][3]string
	for _, backend := range []string{"mysql", "mongodb"} {
		entries, e := os.ReadDir(filepath.Join(root, backend))
		if e != nil {
			t.Fatal(e)
		}
		for _, entry := range entries {
			ext := ".sql"
			if backend == "mongodb" {
				ext = ".json"
			}
			if entry.IsDir() || (!strings.HasSuffix(entry.Name(), ".up"+ext) && !strings.HasSuffix(entry.Name(), ".down"+ext)) {
				continue
			}
			raw, e := os.ReadFile(filepath.Join(root, backend, entry.Name()))
			if e != nil {
				t.Fatal(e)
			}
			inventory = append(inventory, [3]string{backend, "migrations/" + backend + "/" + entry.Name(), fingerprint(raw)})
		}
	}
	raw, e := json.Marshal(inventory)
	if e != nil || len(inventory) != dResourceFiles || fingerprint(raw) != dResources {
		t.Fatalf("D resource binding drift: files=%d sha=%s err=%v", len(inventory), fingerprint(raw), e)
	}
}
func TestCaptureDeadlineNeverLeavesSuccessfulFingerprint(t *testing.T) {
	dir := privateFixture(t)
	path := filepath.Join(dir, "too-late.json")
	p, e := privateParent(path)
	if e != nil {
		t.Fatal(e)
	}
	st, _ := p.Stat()
	uid := st.Sys().(*syscall.Stat_t).Uid
	_ = p.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if finishCapture(ctx, path, []byte("original"), st, uid) == nil {
		t.Fatal("canceled capture accepted")
	}
	if _, e := os.Stat(path); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("created despite deadline")
	}
}
