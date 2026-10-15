package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

func dbFixture(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, m, e := sqlmock.New()
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		_ = db.Close()
		if e := m.ExpectationsWereMet(); e != nil {
			t.Error(e)
		}
	})
	return db, m
}
func pageColumns(i int) []string {
	var cols []string
	cols = append(cols, scope[i].keys...)
	for _, c := range scope[i].columns {
		cols = append(cols, c.Name+"_len")
	}
	for _, c := range scope[i].columns {
		cols = append(cols, c.Name)
	}
	return append(cols, "row_hash")
}
func pageValues(r rowProof) []driver.Value {
	var v []driver.Value
	for _, k := range r.Key {
		v = append(v, k)
	}
	for _, c := range r.Values {
		if c.SQLNull {
			v = append(v, nil)
		} else {
			v = append(v, int64(len(c.Bytes)))
		}
	}
	for _, c := range r.Values {
		if c.SQLNull {
			v = append(v, nil)
		} else {
			v = append(v, c.Bytes)
		}
	}
	return append(v, r.Hash)
}
func TestFullKeysetScanRequiresEmptyEOFAndSameSnapshotCount(t *testing.T) {
	db, m := dbFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	i := 1
	a, b := originalRow(i, []string{"41", "0"}), originalRow(i, []string{"41", "18446744073709551615"})
	// A short page cannot be mistaken for EOF; the next exact composite boundary
	// still requests the mandatory empty page, including Uint64MAX without +1.
	m.ExpectQuery(regexp.QuoteMeta(pageSQL(i, false))).WillReturnRows(sqlmock.NewRows(pageColumns(i)).AddRow(pageValues(a)...).AddRow(pageValues(b)...))
	m.ExpectQuery(regexp.QuoteMeta(pageSQL(i, true))).WithArgs("41", "41", "18446744073709551615").WillReturnRows(sqlmock.NewRows(pageColumns(i)))
	m.ExpectQuery(`SELECT /\*\+ MAX_EXECUTION_TIME\([1-9][0-9]*\) \*/ COUNT\(\*\) FROM ` + "`" + scope[i].name + "`").WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	rows, used := 0, 0
	out, e := scanTable(ctx, db, i, scope[i].columns, &rows, &used)
	if e != nil || !out.EOF || out.Pages != 2 || len(out.Rows) != 2 || rows != 2 || used != out.EncodedRowsBytes {
		t.Fatalf("%+v %v", out, e)
	}
	if !reflect.DeepEqual(out.Rows[0].Values, a.Values) || out.Rows[0].Hash != a.Hash {
		t.Fatal("original NULL/raw values changed")
	}
}
func TestPrimaryPageRejectsOrderOverflowAndReleaseFailure(t *testing.T) {
	for _, name := range []string{"descending", "oversized", "oversized_null_hash", "missing_hash", "rows_error", "aggregate_mismatch", "total_limit"} {
		t.Run(name, func(t *testing.T) {
			db, m := dbFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			i := 3
			r := originalRow(i, []string{"0"})
			row := pageValues(r)
			result := sqlmock.NewRows(pageColumns(i)).AddRow(row...)
			switch name {
			case "descending":
				result.AddRow(row...)
			case "oversized":
				row[len(scope[i].keys)+4] = int64(maxValueBytes + 1)
				result = sqlmock.NewRows(pageColumns(i)).AddRow(row...)
			case "oversized_null_hash":
				row[len(scope[i].keys)+4] = int64(maxValueBytes + 1)
				for n := range scope[i].columns {
					row[len(scope[i].keys)+len(scope[i].columns)+n] = nil
				}
				row[len(row)-1] = nil
				result = sqlmock.NewRows(pageColumns(i)).AddRow(row...)
			case "missing_hash":
				row[len(row)-1] = nil
				result = sqlmock.NewRows(pageColumns(i)).AddRow(row...)
			case "rows_error":
				result.RowError(0, errors.New("fixture private error"))
			}
			m.ExpectQuery(regexp.QuoteMeta(pageSQL(i, false))).WillReturnRows(result)
			if name == "aggregate_mismatch" {
				m.ExpectQuery(regexp.QuoteMeta(pageSQL(i, true))).WithArgs("0").WillReturnRows(sqlmock.NewRows(pageColumns(i)))
				m.ExpectQuery(`SELECT /\*\+ MAX_EXECUTION_TIME\([1-9][0-9]*\) \*/ COUNT`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
			}
			rows, used := 0, 0
			if name == "total_limit" {
				used = maxMaterialBytes
			}
			out, e := scanTable(ctx, db, i, scope[i].columns, &rows, &used)
			if e == nil || out.EOF && name != "aggregate_mismatch" {
				t.Fatalf("rejected range was complete: %+v %v", out, e)
			}
			if name == "oversized_null_hash" && e.Error() != "cell_value_limit_or_null_rejected" {
				t.Fatal("lost explicit oversize rejection", e)
			}
			if name == "missing_hash" && e.Error() != "bounded_row_hash_missing" {
				t.Fatal("missing server hash accepted", e)
			}
			if strings.Contains(e.Error(), "fixture private") {
				t.Fatal("raw DB error leaked")
			}
		})
	}
}
func TestFixedSchemaAndCompositePrimaryKeyDriftRejected(t *testing.T) {
	for _, name := range []string{"correct", "nullable_drift", "extra", "swapped_pk", "expression"} {
		t.Run(name, func(t *testing.T) {
			db, m := dbFixture(t)
			i := 1
			rows := sqlmock.NewRows([]string{"COLUMN_NAME", "COLUMN_TYPE", "IS_NULLABLE", "COLUMN_DEFAULT", "EXTRA", "GENERATION_EXPRESSION"})
			for n, c := range scope[i].columns {
				nullable := "NO"
				if c.Nullable {
					nullable = "YES"
				}
				expr := ""
				if name == "nullable_drift" && n == 0 {
					nullable = "YES"
				}
				if name == "expression" && n == 0 {
					expr = "1"
				}
				rows.AddRow(c.Name, c.Type, nullable, nil, "", expr)
			}
			if name == "extra" {
				rows.AddRow("unexpected", "text", "YES", nil, "", "")
			}
			m.ExpectQuery("SELECT COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE,COLUMN_DEFAULT,EXTRA,GENERATION_EXPRESSION").WithArgs(scope[i].name).WillReturnRows(rows)
			if name == "correct" || name == "swapped_pk" {
				keys := append([]string(nil), scope[i].keys...)
				if name == "swapped_pk" {
					keys[0], keys[1] = keys[1], keys[0]
				}
				pk := sqlmock.NewRows([]string{"COLUMN_NAME", "NON_UNIQUE", "SUB_PART", "COLLATION", "EXPRESSION", "INDEX_TYPE", "IS_VISIBLE"})
				for _, key := range keys {
					pk.AddRow(key, 0, nil, "A", nil, "BTREE", "YES")
				}
				m.ExpectQuery("SELECT COLUMN_NAME,NON_UNIQUE").WithArgs(scope[i].name).WillReturnRows(pk)
			}
			cols, e := columns(context.Background(), db, i)
			if (e == nil) != (name == "correct") {
				t.Fatalf("%v %v", cols, e)
			}
		})
	}
}
func TestFixedTablesRejectPartialOrNonTransactionalObjects(t *testing.T) {
	for _, name := range []string{"original", "target", "myisam", "view", "missing", "reappeared"} {
		t.Run(name, func(t *testing.T) {
			db, m := dbFixture(t)
			rows := sqlmock.NewRows([]string{"TABLE_NAME", "TABLE_TYPE", "ENGINE"})
			names := []string{"schema_migrations"}
			original := name != "target" && name != "reappeared"
			if original {
				for _, s := range scope {
					names = append(names, s.name)
				}
			}
			if name == "reappeared" {
				names = append(names, scope[4].name)
			}
			sort.Strings(names)
			for i, n := range names {
				kind, engine := "BASE TABLE", "InnoDB"
				if name == "myisam" && i == 0 {
					engine = "MyISAM"
				}
				if name == "view" && i == 0 {
					kind = "VIEW"
				}
				if name == "missing" && i == 0 {
					continue
				}
				rows.AddRow(n, kind, engine)
			}
			m.ExpectQuery("SELECT TABLE_NAME,TABLE_TYPE,ENGINE").WillReturnRows(rows)
			e := requireTables(context.Background(), db, original)
			if (e == nil) != (name == "original" || name == "target") {
				t.Fatal(name, e)
			}
		})
	}
}
func TestOnlyTerminalCountUsesRemainingActionBudgetAndTypedFailures(t *testing.T) {
	for _, tc := range []struct {
		name  string
		err   error
		count uint64
		want  string
	}{{"match", nil, 3, ""}, {"mismatch", nil, 4, "complete_range_count_mismatch"}, {"3024", &mysql.MySQLError{Number: 3024, Message: "private message"}, 0, "complete_range_count_server_timeout"}, {"deadline", context.DeadlineExceeded, 0, "complete_range_count_deadline_exceeded"}, {"canceled", context.Canceled, 0, "complete_range_count_canceled"}, {"query", errors.New("private text"), 0, "complete_range_count_query_failed"}} {
		t.Run(tc.name, func(t *testing.T) {
			db, m := dbFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			defer cancel()
			expect := m.ExpectQuery(`SELECT /\*\+ MAX_EXECUTION_TIME\(1[01][0-9]{3}\) \*/ COUNT`)
			if tc.err != nil {
				expect.WillReturnError(tc.err)
			} else {
				expect.WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(tc.count))
			}
			e := verifyCompleteRangeCount(ctx, db, 0, 3)
			if tc.want == "" && e != nil || tc.want != "" && (e == nil || e.Error() != tc.want) {
				t.Fatal(e)
			}
		})
	}
	for _, ctx := range []context.Context{context.Background(), func() context.Context { ctx, cancel := context.WithCancel(context.Background()); cancel(); return ctx }()} {
		if verifyCompleteRangeCount(ctx, nil, 0, 0) == nil {
			t.Fatal("unbounded/canceled aggregate accepted")
		}
	}
}
func TestExactMaterialLimitsRejectBeforeEOF(t *testing.T) {
	for _, tc := range []struct {
		name                                        string
		rowCount, tableBytes, totalRows, totalBytes int
	}{{"table_rows", maxRows, 0, 0, 0}, {"total_rows", 0, 0, maxTotalRows, 0}, {"table_bytes", 0, maxTableBytes, 0, 0}, {"total_bytes", 0, 0, 0, maxMaterialBytes}} {
		t.Run(tc.name, func(t *testing.T) {
			a, b, c := tc.tableBytes, tc.totalRows, tc.totalBytes
			if accountRow(1, tc.rowCount, &a, &b, &c) == nil {
				t.Fatal("would truncate table")
			}
			if a != tc.tableBytes || b != tc.totalRows || c != tc.totalBytes {
				t.Fatal("rejected row mutated accounting")
			}
		})
	}
	a, b, c := maxTableBytes-1, maxTotalRows-1, maxMaterialBytes-1
	if e := accountRow(1, maxRows-1, &a, &b, &c); e != nil || a != maxTableBytes || b != maxTotalRows || c != maxMaterialBytes {
		t.Fatal("exact boundary rejected", e)
	}
}
func TestMongoPrimaryRoleAndURIOverride(t *testing.T) {
	if mongoReadOptions("mongodb://localhost/?readPreference=secondary").ReadPreference.Mode().String() != "primary" {
		t.Fatal("URI still selects secondary")
	}
	for _, name := range []string{"primary", "secondary", "mongos", "missing_role", "duplicate_host"} {
		rawDoc := bson.D{{Key: "setName", Value: "rs0"}, {Key: "isWritablePrimary", Value: name != "secondary"}, {Key: "hosts", Value: bson.A{"mongo:27017"}}}
		if name == "mongos" {
			rawDoc = append(rawDoc, bson.E{Key: "msg", Value: "isdbgrid"})
		}
		if name == "missing_role" {
			rawDoc = append(rawDoc[:1], rawDoc[2:]...)
		}
		if name == "duplicate_host" {
			rawDoc[2].Value = bson.A{"mongo:27017", "mongo:27017"}
		}
		raw, _ := bson.Marshal(rawDoc)
		_, e := primaryReplicaHosts(raw)
		if (e == nil) != (name == "primary") {
			t.Fatal(name, e)
		}
	}
}
func TestAllPageValuesAreServerBinaryAndCanonicalBoundaries(t *testing.T) {
	for i, s := range scope {
		query := pageSQL(i, true)
		for _, c := range s.columns {
			if !strings.Contains(query, "CAST(`"+c.Name+"` AS BINARY)") {
				t.Fatal("nonbinary value", c.Name)
			}
		}
		if strings.Contains(query, "AS UNSIGNED") || strings.Contains(query, "OFFSET") {
			t.Fatal("unsafe integer boundary")
		}
		if len(s.keys) == 2 && !strings.Contains(query, "testee_id`>CAST(? AS DECIMAL(20,0))") {
			t.Fatal("composite unsigned boundary")
		}
		if !strings.Contains(query, "LIMIT 500") || !strings.Contains(query, "MAX_EXECUTION_TIME(5000)") {
			t.Fatal(query)
		}
	}
}

// Verify the small semantic guard around the hash, not a complete query golden.
func TestServerHashOnlyMaterializesWithinAllValueAndRowLimits(t *testing.T) {
	s := scope[4]
	query := pageSQL(4, false)
	hashStart := strings.Index(query, " THEN SHA2(CONCAT(")
	if hashStart < 0 || strings.Count(query, "SHA2(CONCAT(") != 1 {
		t.Fatal("missing unique bounded hash branch")
	}
	caseStart := strings.LastIndex(query[:hashStart], "CASE WHEN ")
	if caseStart < 0 {
		t.Fatal("hash is unconditionally evaluated")
	}
	guard := query[caseStart+len("CASE WHEN ") : hashStart]
	for _, c := range s.columns {
		length := "COALESCE(OCTET_LENGTH(CAST(`" + c.Name + "` AS BINARY)),0)"
		if !strings.Contains(guard, length+"<=8388608") {
			t.Fatalf("hash lacks per-value bound for %s", c.Name)
		}
	}
	if !strings.HasSuffix(guard, ")<=12582912") || !strings.Contains(query[hashStart:], "),256) ELSE NULL END FROM ") {
		t.Fatal("hash lacks complete row bound or over-limit NULL")
	}
	// Every returned original value uses exactly the same predicate.
	if strings.Count(query, "CASE WHEN "+guard+" THEN ") != len(s.columns)+1 {
		t.Fatal("hash/value eligibility diverged")
	}
}
