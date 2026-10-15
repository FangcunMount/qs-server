package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

const fixtureSource = "1234567890123456789012345678901234567890"

func fixtureBinding() binding {
	return binding{"qs_fixture", "qs_fixture", strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)}
}
func fixtureMaterial(t *testing.T) material {
	t.Helper()
	raw, e := bson.Marshal(bson.D{{Key: "version", Value: int64(40)}, {Key: "dirty", Value: false}})
	if e != nil {
		t.Fatal(e)
	}
	m := material{Format: 1, Kind: "m4_c_actor_column_protection", ToolSource: fixtureSource, CSource: cSource, CResources: cResources, CResourceFiles: cResourceFiles, CapturedAt: "2026-10-15T00:00:00Z", Binding: fixtureBinding(), SessionZone: "+00:00", SessionCharset: "utf8mb4", SessionSQLMode: sessionMode, LedgerDDL: "CREATE TABLE `schema_migrations` (`version` bigint NOT NULL, `dirty` tinyint(1) NOT NULL)", Ledger: []ledgerRow{{101, false}}, MongoHead: raw}
	for i, s := range scope {
		ddl := "CREATE TABLE `" + s.table + "` (\n  `" + s.key + "` bigint unsigned NOT NULL,\n"
		switch i {
		case 0:
			ddl += "  `birthday` date DEFAULT NULL,\n  `tags` json DEFAULT NULL COMMENT '标签（JSON数组）',\n"
		case 1:
			ddl += "  `roles` json NOT NULL,\n  `effective_roles` json NOT NULL,\n"
		}
		ddl += "  PRIMARY KEY (`" + s.key + "`)\n) ENGINE=InnoDB"
		retained, e := retainedDDL(ddl, i, i < 2)
		if e != nil {
			t.Fatal(e)
		}
		key, kind := "0", "bigint unsigned"
		if s.binaryKey {
			key, kind = "41", "varchar(64)"
		}
		row := rowProof{Key: key, Retained: strings.Repeat("d", 64)}
		if i < 2 {
			row.Value = &originalValue{Kind: "json", JSON: []byte("[]")}
		}
		m.Tables[i] = tableProof{Table: s.table, DDL: ddl, RetainedDDL: retained, Columns: []column{{Name: s.key, Type: kind}}, Rows: []rowProof{row}, Pages: 2, EOF: true}
	}
	return m
}
func cloneMaterial(t *testing.T, m material) material {
	t.Helper()
	raw, e := json.Marshal(m)
	if e != nil {
		t.Fatal(e)
	}
	var out material
	if e := strictDecode(raw, &out); e != nil {
		t.Fatal(e)
	}
	return out
}

func TestOriginalJSONFraming(t *testing.T) {
	values := []*originalValue{{Kind: "sql_null"}, {Kind: "json", JSON: []byte("null")}, {Kind: "json", JSON: []byte("[]")}}
	seen := map[string]bool{}
	for _, v := range values {
		if !checkValue(v) {
			t.Fatal("legitimate SQLNULL/JSONnull/array was rejected")
		}
		raw, e := json.Marshal(v)
		if e != nil || seen[string(raw)] {
			t.Fatal("distinct original representations collapsed")
		}
		seen[string(raw)] = true
	}
	for _, v := range []*originalValue{nil, {Kind: "sql_null", JSON: []byte("null")}, {Kind: "json"}, {Kind: "json", JSON: []byte("{")}, {Kind: "other"}, {Kind: "json", JSON: []byte(`"` + strings.Repeat("a", maxValueBytes) + `"`)}} {
		if checkValue(v) {
			t.Fatal("invalid or oversized original representation accepted")
		}
	}
}

func TestMongo40RequiresPresentTypedUniqueCleanFields(t *testing.T) {
	for _, version := range []any{int32(40), int64(40)} {
		raw, _ := bson.Marshal(bson.D{{Key: "version", Value: version}, {Key: "dirty", Value: false}})
		if !cleanMongo40(raw) {
			t.Fatal("clean typed version40 rejected")
		}
	}
	for name, doc := range map[string]bson.D{
		"missing_dirty": {{Key: "version", Value: int64(40)}},
		"null_dirty":    {{Key: "version", Value: int64(40)}, {Key: "dirty", Value: nil}},
		"dirty":         {{Key: "version", Value: int64(40)}, {Key: "dirty", Value: true}},
		"wrong_type":    {{Key: "version", Value: float64(40)}, {Key: "dirty", Value: false}},
		"wrong_version": {{Key: "version", Value: int64(39)}, {Key: "dirty", Value: false}},
		"duplicate":     {{Key: "version", Value: int64(40)}, {Key: "dirty", Value: true}, {Key: "dirty", Value: false}},
	} {
		t.Run(name, func(t *testing.T) {
			raw, _ := bson.Marshal(doc)
			if cleanMongo40(raw) {
				t.Fatal("unknown/dirty Mongo ledger accepted")
			}
		})
	}
}

func TestMaterialRejectsUnboundOrIncompleteCapture(t *testing.T) {
	base := fixtureMaterial(t)
	if e := validateMaterial(base, fixtureSource, fixtureBinding()); e != nil {
		t.Fatal(e)
	}
	cases := map[string]func(*material){
		"source":       func(m *material) { m.CSource = fixtureSource },
		"resources":    func(m *material) { m.CResources = strings.Repeat("e", 64) },
		"scope":        func(m *material) { m.Tables[0].Table = "assessment" },
		"identity":     func(m *material) { m.Binding.MongoUUID = strings.Repeat("f", 64) },
		"session":      func(m *material) { m.SessionZone = "+08:00" },
		"ledger_dirty": func(m *material) { m.Ledger[0].Dirty = true },
		"ledger_extra": func(m *material) { m.Ledger = append(m.Ledger, ledgerRow{102, false}) },
		"no_eof":       func(m *material) { m.Tables[0].EOF = false },
		"page_count":   func(m *material) { m.Tables[0].Pages = 1 },
		"duplicate_id": func(m *material) { m.Tables[0].Rows = append(m.Tables[0].Rows, m.Tables[0].Rows[0]) },
		"json_bytes":   func(m *material) { m.Tables[0].Rows[0].Value.JSON = []byte("{") },
		"notnull":      func(m *material) { m.Tables[1].Rows[0].Value = &originalValue{Kind: "sql_null"} },
		"archive_body": func(m *material) { m.Tables[3].Rows[0].Value = &originalValue{Kind: "json", JSON: []byte("[]")} },
		"ddl_drift": func(m *material) {
			m.Tables[0].DDL = strings.ReplaceAll(m.Tables[0].DDL, "json DEFAULT NULL", "json NOT NULL")
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			m := cloneMaterial(t, base)
			change(&m)
			if validateMaterial(m, fixtureSource, fixtureBinding()) == nil {
				t.Fatal("unbound/incomplete material accepted")
			}
		})
	}
}

func TestStrictMaterialDecoderRejectsAmbiguity(t *testing.T) {
	for _, raw := range []string{`{"kind":"one","kind":"two"}`, `{"unknown":1}`, `{} {}`, `{"tables":[{"table":"a","table":"b"}]}`} {
		var m material
		if strictDecode([]byte(raw), &m) == nil {
			t.Fatalf("ambiguous schema accepted: %s", raw)
		}
	}
}

func TestVerifyTargetPreservesFullRetainedScope(t *testing.T) {
	original := fixtureMaterial(t)
	target := cloneMaterial(t, original)
	target.Ledger = []ledgerRow{{102, false}}
	for i := range target.Tables {
		target.Tables[i].DDL = target.Tables[i].RetainedDDL
		for n := range target.Tables[i].Rows {
			target.Tables[i].Rows[n].Value = nil
		}
	}
	if e := verifyTarget(original, target); e != nil {
		t.Fatal(e)
	}
	for name, change := range map[string]func(*material){
		"new_person": func(m *material) {
			m.Tables[0].Rows = append(m.Tables[0].Rows, rowProof{Key: "1", Retained: strings.Repeat("d", 64)})
		},
		"archive_changed":        func(m *material) { m.Tables[3].Rows[0].Retained = strings.Repeat("e", 64) },
		"original_value_remains": func(m *material) { m.Tables[1].Rows[0].Value = &originalValue{Kind: "json", JSON: []byte("[]")} },
		"extra_ddl":              func(m *material) { m.Tables[2].DDL += " COMMENT='changed'" },
		"mongo_changed":          func(m *material) { m.MongoHead = append(m.MongoHead, 0) },
		"tool_changed":           func(m *material) { m.ToolSource = cSource },
		"target_dirty":           func(m *material) { m.Ledger[0].Dirty = true },
		"not_eof":                func(m *material) { m.Tables[2].EOF = false },
	} {
		t.Run(name, func(t *testing.T) {
			m := cloneMaterial(t, target)
			change(&m)
			if verifyTarget(original, m) == nil {
				t.Fatal("target drift accepted")
			}
		})
	}
}

func TestRestoreRejectedBeforeConfigurationOrMaterialAccess(t *testing.T) {
	r := run(context.Background(), []string{"--action=restore", "--material-file=/unread"}, func(string) string {
		t.Fatal("restore accessed connection configuration")
		return ""
	}, fixtureSource)
	if r.Error != "restore_not_implemented" || r.Complete || r.ApprovalConsumed || r.SQLConnectionClosed || r.SQLPoolClosed || r.MongoClosed {
		t.Fatal("unsupported restore upgraded execution or release claims")
	}
	if _, e := parseArgs([]string{"--action=restore", "--approval-file=/unread"}); e == nil {
		t.Fatal("unimplemented approval flag accepted")
	}
}

func TestPrivateFirstCaptureIsExclusiveAndExternallyBound(t *testing.T) {
	dir, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if e := os.Chmod(dir, 0700); e != nil {
		t.Fatal(e)
	}
	path, raw := filepath.Join(dir, "capture.json"), []byte(`{"fixed":"bytes"}`)
	if e := writeExclusive(path, raw); e != nil {
		t.Fatal(e)
	}
	if e := writeExclusive(path, []byte("replacement")); e == nil {
		t.Fatal("first capture was overwritten")
	}
	if _, e := readPrivate(path, strings.Repeat("0", 64)); e == nil {
		t.Fatal("input authorized its own new fingerprint")
	}
	if got, e := readPrivate(path, fingerprint(raw)); e != nil || string(got) != string(raw) {
		t.Fatal("original material unavailable", e)
	}
	link := filepath.Join(dir, "link.json")
	if e := os.Symlink(path, link); e != nil {
		t.Fatal(e)
	}
	if _, e := readPrivate(link, fingerprint(raw)); e == nil {
		t.Fatal("symlink material accepted")
	}
	if e := os.Chmod(path, 0644); e != nil {
		t.Fatal(e)
	}
	if _, e := readPrivate(path, fingerprint(raw)); e == nil {
		t.Fatal("public material accepted")
	}
}
