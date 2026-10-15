package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func expectRetainedMetadata(m sqlmock.Sqlmock, s tableSpec) {
	cols := sqlmock.NewRows([]string{"name", "type", "nullable", "generated"})
	for _, c := range s.columns {
		nullable := "NO"
		if c.Nullable {
			nullable = "YES"
		}
		cols.AddRow(c.Name, c.Type, nullable, "")
	}
	m.ExpectQuery("SELECT COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE,GENERATION_EXPRESSION").WithArgs(s.name).WillReturnRows(cols)
	pk := sqlmock.NewRows([]string{"name", "nonunique", "prefix", "order", "expression", "type", "visible"}).AddRow(s.keys[0], 0, nil, "A", nil, "BTREE", "YES")
	m.ExpectQuery("SELECT COLUMN_NAME,NON_UNIQUE").WithArgs(s.name).WillReturnRows(pk)
	m.ExpectQuery(regexp.QuoteMeta("SHOW CREATE TABLE `" + s.name + "`")).WillReturnRows(sqlmock.NewRows([]string{"table", "ddl"}).AddRow(s.name, "CREATE TABLE `"+s.name+"` (retained)"))
}

func TestRetainedEightScopeAndWholeRangeProof(t *testing.T) {
	for i, s := range retainedScope {
		t.Run(s.name, func(t *testing.T) {
			db, m := dbFixture(t)
			expectRetainedMetadata(m, s)
			first, last := "0", "18446744073709551615"
			if s.binaryFirst {
				first, last = "5A", "61"
			}
			m.ExpectQuery(regexp.QuoteMeta(retainedPageSQL(s, false))).WillReturnRows(sqlmock.NewRows([]string{"key", "hash"}).AddRow(first, strings.Repeat("a", 64)).AddRow(last, strings.Repeat("b", 64)))
			m.ExpectQuery(regexp.QuoteMeta(retainedPageSQL(s, true))).WithArgs(last).WillReturnRows(sqlmock.NewRows([]string{"key", "hash"}))
			m.ExpectQuery("SELECT /\\*\\+ MAX_EXECUTION_TIME\\([1-9][0-9]*\\) \\*/ COUNT").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			n, b := 0, 0
			p, e := scanRetained(ctx, db, i, &n, &b)
			if e != nil || !p.EOF || p.Pages != 2 || len(p.Rows) != 2 || n != 2 || b != p.EncodedRowsBytes {
				t.Fatalf("%+v %v", p, e)
			}
			encoded, _ := json.Marshal(p)
			if strings.Contains(string(encoded), "server_bytes") || strings.Contains(string(encoded), "original_values") {
				t.Fatal("retained facts copied body")
			}
		})
	}
}

func TestRetainedHashBoundsPrecedeLargeValueFrame(t *testing.T) {
	s := retainedScope[4]
	q := retainedPageSQL(s, true)
	guard := "CASE WHEN "
	if !strings.Contains(q, "COALESCE(OCTET_LENGTH(CAST(`recovery_payload` AS BINARY)),0)<=8388608") || !strings.Contains(q, "<=12582912 THEN SHA2(CONCAT(") || !strings.Contains(q, "ELSE NULL END FROM") || strings.Index(q, guard) > strings.Index(q, "SHA2(CONCAT(") || !strings.Contains(q, "CAST(`request_id` AS BINARY)>UNHEX(?)") {
		t.Fatal("retained large-value hash bypasses bounds/binary keyset")
	}
	for _, bad := range []string{"NULL hash", "descending", "mismatch", "table limit"} {
		t.Run(bad, func(t *testing.T) {
			db, m := dbFixture(t)
			expectRetainedMetadata(m, s)
			rows := sqlmock.NewRows([]string{"key", "hash"})
			if bad == "NULL hash" {
				rows.AddRow("41", nil)
			} else {
				rows.AddRow("41", strings.Repeat("a", 64))
			}
			if bad == "descending" {
				rows.AddRow("40", strings.Repeat("b", 64))
			}
			m.ExpectQuery(regexp.QuoteMeta(retainedPageSQL(s, false))).WillReturnRows(rows)
			if bad == "mismatch" {
				m.ExpectQuery(regexp.QuoteMeta(retainedPageSQL(s, true))).WithArgs("41").WillReturnRows(sqlmock.NewRows([]string{"key", "hash"}))
				m.ExpectQuery("SELECT /\\*\\+ MAX_EXECUTION_TIME").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
			}
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			n, b := 0, 0
			if bad == "table limit" {
				n = maxTotalRows
			}
			p, e := scanRetained(ctx, db, 4, &n, &b)
			if e == nil || p.EOF && bad != "mismatch" {
				t.Fatal("incomplete retained range accepted", e)
			}
		})
	}
}

func TestFormatTwoRequiresAllRetainedOriginalFacts(t *testing.T) {
	for name, change := range map[string]func(*material){
		"v1":            func(m *material) { m.Format = 1 },
		"missing_table": func(m *material) { m.Retained[7] = retainedTableProof{} },
		"missing_eof":   func(m *material) { m.Retained[0].EOF = false },
		"duplicate_pk":  func(m *material) { m.Retained[4].Rows = append(m.Retained[4].Rows, m.Retained[4].Rows[0]) },
		"invalid_hash":  func(m *material) { m.Retained[1].Rows[0].Hash = strings.Repeat("0", 64) },
		"changed_ddl":   func(m *material) { m.Tables[0].DDL += " " },
	} {
		t.Run(name, func(t *testing.T) {
			m := cloneMaterial(originalMaterial())
			change(&m)
			if validateMaterial(m, testTool, testBinding()) == nil {
				t.Fatal("incomplete format2 accepted")
			}
		})
	}
	original := originalMaterial()
	target := cloneMaterial(original)
	target.Tables = [5]tableProof{}
	target.Ledger = []ledgerRow{{103, false}}
	if verifyTarget(original, target) != nil {
		t.Fatal("unchanged eight-table proof rejected")
	}
	for _, field := range []string{"hash", "DDL", "EOF", "missingrow"} {
		t.Run(field, func(t *testing.T) {
			m := cloneMaterial(target)
			switch field {
			case "hash":
				m.Retained[3].Rows[0].Hash = strings.Repeat("b", 64)
			case "DDL":
				m.Retained[4].DDL += " "
			case "EOF":
				m.Retained[5].EOF = false
			case "missingrow":
				m.Retained[7].Rows = nil
			}
			if verifyTarget(original, m) == nil {
				t.Fatal("retained drift accepted")
			}
		})
	}
}

func TestOriginal075078ClosedDefinitionQualification(t *testing.T) {
	root := filepath.Join("..", "..", "..", "internal", "pkg", "migration", "migrations", "mysql")
	for name, raw := range map[string][]byte{"000075_operator_retirement_manifests.up.sql": original075, "000078_testee_store_migration_manifest.up.sql": original078} {
		actual, e := os.ReadFile(filepath.Join(root, name))
		if e != nil || string(actual) != string(raw) {
			t.Fatal("original resource copy drift")
		}
	}
	for i := range scope {
		for _, collation := range originalCollations {
			if e := qualifyOriginalDDL(i, originalDDL(i, collation)); e != nil {
				t.Fatal(e)
			}
		}
		base := originalDDL(i, originalCollations[0])
		for name, raw := range map[string]string{"unknown_collation": strings.ReplaceAll(base, originalCollations[0], "utf8mb4_bin"), "nullable": strings.Replace(base, " NOT NULL", " DEFAULT NULL", 1), "extra_index": strings.Replace(base, "\n) ENGINE", ",\n  KEY `foreign` (`migration_id`)\n) ENGINE", 1), "column_collation": strings.Replace(base, "varchar(64)", "varchar(64) COLLATE utf8mb4_bin", 1), "trailing_sql": base + "; DELETE FROM testee"} {
			// The clinician archive has no varchar first-column difference, but
			// still has migration_id varchar(64); every mutation changes its DDL.
			if raw == base {
				t.Fatal("bad mutation", name)
			}
			if qualifyOriginalDDL(i, raw) == nil {
				t.Fatal("foreign original definition accepted", name)
			}
		}
	}
	for _, i := range []int{3, 4} {
		base := originalDDL(i, originalCollations[0])
		if qualifyOriginalDDL(i, strings.Replace(base, "json_valid", "json_length", 1)) == nil {
			t.Fatal("changed CHECK accepted")
		}
	}
}

func TestTerminalCountRejectsOutsideFixedThirteenTables(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	if e := completeRangeCount(ctx, nil, "assessment", 0); e == nil || e.Error() != "complete_range_count_scope_rejected" {
		t.Fatal("count expanded beyond D fixed protection scope", e)
	}
}
