package main

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

func TestURIReadPreferenceCannotSelectSecondary(t *testing.T) {
	for _, query := range []string{"readPreference=secondary", "readPreference=secondaryPreferred&readPreferenceTags=region:fixture&maxStalenessSeconds=120", "readPreference=nearest", "directConnection=true"} {
		opts := mongoReadOptions("mongodb://localhost:27017/qs_fixture?" + query)
		if e := opts.Validate(); e != nil || opts.ReadPreference == nil || opts.ReadPreference.Mode() != readpref.PrimaryMode {
			t.Fatal("URI escaped fixed Primary read routing", e)
		}
		if opts.RetryReads == nil || *opts.RetryReads || opts.RetryWrites == nil || *opts.RetryWrites || opts.Timeout == nil || *opts.Timeout != queryLimit {
			t.Fatal("routing fix changed existing finite read/retry policy")
		}
	}
}

func TestHelloRequiresActualReplicaPrimary(t *testing.T) {
	base := bson.D{{Key: "setName", Value: "rs0"}, {Key: "hosts", Value: bson.A{"b:27017", "a:27017"}}, {Key: "isWritablePrimary", Value: true}}
	raw, _ := bson.Marshal(base)
	hosts, e := primaryReplicaHosts(raw)
	if e != nil || strings.Join(hosts, ",") != "a:27017,b:27017" {
		t.Fatal("legitimate primary or stable identity order rejected", e)
	}
	for _, name := range []string{"secondary", "missing_role", "null_role", "numeric_role", "mongos", "wrong_set", "duplicate_host", "typed_host"} {
		t.Run(name, func(t *testing.T) {
			doc := append(bson.D(nil), base...)
			switch name {
			case "secondary":
				doc[2].Value = false
			case "missing_role":
				doc = doc[:2]
			case "null_role":
				doc[2].Value = nil
			case "numeric_role":
				doc[2].Value = int32(1)
			case "mongos":
				doc = append(doc, bson.E{Key: "msg", Value: "isdbgrid"})
			case "wrong_set":
				doc[0].Value = "other"
			case "duplicate_host":
				doc[1].Value = bson.A{"a:27017", "a:27017"}
			case "typed_host":
				doc[1].Value = bson.A{int32(1)}
			}
			raw, _ := bson.Marshal(doc)
			if _, e := primaryReplicaHosts(raw); e == nil {
				t.Fatal("non-primary/unknown topology accepted")
			}
		})
	}
}

func TestPrimaryKeyOrderingPreservesZeroAndUint64Max(t *testing.T) {
	const max = "18446744073709551615"
	for _, key := range []string{"0", "1", max} {
		if !validKey(key, false) {
			t.Fatal("valid uint64 key rejected")
		}
	}
	for _, key := range []string{"", "00", "-1", "18446744073709551616", "1e2"} {
		if validKey(key, false) {
			t.Fatal("noncanonical key accepted")
		}
	}
	if !keyLess("0", max, false) || keyLess(max, "0", false) || keyLess(max, max, false) || !keyLess("00", "01", true) || validKey("aa", true) || !validKey("00FF", true) {
		t.Fatal("numeric/binary ordering is not strict")
	}
	q := pageSQL(0, []column{{Name: "id", Type: "bigint unsigned"}}, true, true)
	if !strings.Contains(q, "`id`>CAST(? AS DECIMAL(20,0))") || strings.Contains(q, "AS UNSIGNED") || strings.Contains(q, "+1") || !strings.Contains(q, "LIMIT 1000") {
		t.Fatal("pinned driver UINT64 boundary protection missing")
	}
	q = pageSQL(3, []column{{Name: "request_id", Type: "varchar(64)"}}, true, true)
	if !strings.Contains(q, "CAST(`request_id` AS BINARY)>UNHEX(?)") || !strings.Contains(q, "ORDER BY CAST(`request_id` AS BINARY)") {
		t.Fatal("archive key lost binary ordering")
	}
}

func TestFullScanRequiresEmptyEOFAndCountConservation(t *testing.T) {
	db, mock, e := sqlmock.New()
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = db.Close() }()
	cols := []column{{Name: "id", Type: "bigint unsigned"}, {Name: "name", Type: "varchar(100)"}}
	fields := []string{"id", "original_length", "original_value", "retained_hash"}
	hash := strings.Repeat("a", 64)
	const max = "18446744073709551615"
	mock.ExpectQuery(regexp.QuoteMeta(pageSQL(0, cols, true, false))).WillReturnRows(sqlmock.NewRows(fields).AddRow("0", nil, nil, hash).AddRow(max, 2, []byte("[]"), hash))
	mock.ExpectQuery(regexp.QuoteMeta(pageSQL(0, cols, true, true))).WithArgs(max).WillReturnRows(sqlmock.NewRows(fields))
	mock.ExpectQuery("SELECT /\\*\\+ MAX_EXECUTION_TIME\\([1-9][0-9]*\\) \\*/ COUNT\\(\\*\\) FROM `testee`").WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(2))
	used := 0
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	probe := &countDeadlineProbe{sqlReader: db}
	out, e := scanTable(ctx, probe, 0, cols, true, &used)
	if e != nil || !out.EOF || out.Pages != 2 || len(out.Rows) != 2 || out.Rows[0].Value.Kind != "sql_null" || out.Rows[1].Value.Kind != "json" || string(out.Rows[1].Value.JSON) != "[]" {
		t.Fatal("full scan failed to preserve original keys/values and EOF", e)
	}
	deadline, _ := ctx.Deadline()
	if len(probe.deadlines) != 3 || !probe.deadlines[2].Equal(deadline) {
		t.Fatal("terminal COUNT lost the action context")
	}
	for _, d := range probe.deadlines[:2] {
		if d.After(time.Now().Add(queryLimit)) || !d.Before(deadline) {
			t.Fatal("page reads inherited the longer aggregate deadline")
		}
	}
	if e := mock.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}

func TestScanRejectsIncompleteOrAmbiguousRows(t *testing.T) {
	for _, name := range []string{"duplicate_key", "oversize_value", "rows_error", "count_drift", "permission"} {
		t.Run(name, func(t *testing.T) {
			db, mock, e := sqlmock.New()
			if e != nil {
				t.Fatal(e)
			}
			defer func() { _ = db.Close() }()
			cols := []column{{Name: "id", Type: "bigint unsigned"}}
			fields := []string{"id", "original_length", "original_value", "retained_hash"}
			rows := sqlmock.NewRows(fields).AddRow("1", 2, []byte("[]"), strings.Repeat("a", 64))
			first := mock.ExpectQuery(regexp.QuoteMeta(pageSQL(0, cols, true, false)))
			switch name {
			case "duplicate_key":
				rows.AddRow("1", 2, []byte("[]"), strings.Repeat("a", 64))
			case "oversize_value":
				rows = sqlmock.NewRows(fields).AddRow("1", maxValueBytes+1, nil, strings.Repeat("a", 64))
			case "rows_error":
				rows.RowError(0, errors.New("private detail must not escape"))
			case "count_drift":
				mock.ExpectQuery(regexp.QuoteMeta(pageSQL(0, cols, true, true))).WithArgs("1").WillReturnRows(sqlmock.NewRows(fields))
				mock.ExpectQuery("SELECT /\\*\\+ MAX_EXECUTION_TIME\\([1-9][0-9]*\\) \\*/ COUNT\\(\\*\\) FROM `testee`").WillReturnRows(sqlmock.NewRows([]string{"n"}).AddRow(2))
			case "permission":
				first.WillReturnError(errors.New("private detail must not escape"))
			}
			if name != "permission" {
				first.WillReturnRows(rows)
			}
			used := 0
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			if out, e := scanTable(ctx, db, 0, cols, true, &used); e == nil || strings.Contains(e.Error(), "private detail") || (out.EOF && name != "count_drift") {
				t.Fatal("incomplete scan became successful or leaked driver error", e)
			}
			if e := mock.ExpectationsWereMet(); e != nil {
				t.Fatal(e)
			}
		})
	}
}

type countDeadlineProbe struct {
	sqlReader
	queries   []string
	deadlines []time.Time
}

func (p *countDeadlineProbe) QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error) {
	d, _ := ctx.Deadline()
	p.queries, p.deadlines = append(p.queries, q), append(p.deadlines, d)
	return p.sqlReader.QueryContext(ctx, q, args...)
}
func (p *countDeadlineProbe) QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row {
	d, _ := ctx.Deadline()
	p.queries, p.deadlines = append(p.queries, q), append(p.deadlines, d)
	return p.sqlReader.QueryRowContext(ctx, q, args...)
}

func TestCompleteRangeCountUsesCallerSnapshotAndFiniteAction(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		value      any
		err        error
	}{
		{"complete", "", uint64(2), nil},
		{"mismatch", "complete_range_count_mismatch", uint64(3), nil},
		{"invalid_result", "complete_range_count_query_failed", "invalid", nil},
		{"query_failed", "complete_range_count_query_failed", nil, errors.New("private SQL connection details")},
		{"deadline", "complete_range_count_deadline_exceeded", nil, context.DeadlineExceeded},
		{"canceled", "complete_range_count_canceled", nil, context.Canceled},
		{"server3024", "complete_range_count_server_timeout", nil, errors.Join(&mysql.MySQLError{Number: 3024, Message: "private server text"}, context.DeadlineExceeded)},
		{"other_server", "complete_range_count_query_failed", nil, &mysql.MySQLError{Number: 1064, Message: "private SQL text"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, e := sqlmock.New()
			if e != nil {
				t.Fatal(e)
			}
			mock.ExpectBegin()
			query := mock.ExpectQuery("SELECT /\\*\\+ MAX_EXECUTION_TIME\\([1-9][0-9]*\\) \\*/ COUNT\\(\\*\\) FROM `operator_recovery_archives`")
			if tc.err != nil {
				query.WillReturnError(tc.err)
			} else {
				query.WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(tc.value))
			}
			mock.ExpectRollback()
			mock.ExpectClose()
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			tx, e := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
			if e != nil {
				t.Fatal(e)
			}
			probe := &countDeadlineProbe{sqlReader: tx}
			deadline, _ := ctx.Deadline()
			before := time.Now()
			e = verifyCompleteRangeCount(ctx, probe, 3, 2)
			after := time.Now()
			if (tc.want == "" && e != nil) || (tc.want != "" && (e == nil || e.Error() != tc.want)) || len(probe.queries) != 1 || !probe.deadlines[0].Equal(deadline) {
				t.Fatal("COUNT escaped supplied RR transaction/action or leaked an error", e)
			}
			start := strings.Index(probe.queries[0], "MAX_EXECUTION_TIME(") + len("MAX_EXECUTION_TIME(")
			end := strings.Index(probe.queries[0][start:], ")") + start
			milliseconds, e := strconv.ParseInt(probe.queries[0][start:end], 10, 64)
			if e != nil || milliseconds <= queryLimit.Milliseconds() || milliseconds > deadline.Sub(before).Milliseconds() || milliseconds < deadline.Sub(after).Milliseconds() {
				t.Fatal("server timeout was zero, lengthened or replaced by the page timeout")
			}
			if tx.Rollback() != nil || db.Close() != nil || mock.ExpectationsWereMet() != nil {
				t.Fatal("COUNT reader or supplied transaction not released")
			}
		})
	}
}

func TestCompleteRangeCountRejectsUnboundedOrExpiredAction(t *testing.T) {
	for _, which := range []string{"missing", "expired", "sub_millisecond", "canceled", "excessive"} {
		t.Run(which, func(t *testing.T) {
			ctx := context.Background()
			var cancel context.CancelFunc
			want := "complete_range_count_budget_rejected"
			switch which {
			case "expired":
				ctx, cancel = context.WithDeadline(ctx, time.Now().Add(-time.Second))
				want = "complete_range_count_deadline_exceeded"
			case "sub_millisecond":
				ctx, cancel = context.WithDeadline(ctx, time.Now().Add(time.Microsecond))
				want = "complete_range_count_deadline_exceeded"
			case "canceled":
				ctx, cancel = context.WithTimeout(ctx, time.Minute)
				cancel()
				want = "complete_range_count_canceled"
			case "excessive":
				ctx, cancel = context.WithTimeout(ctx, time.Hour)
			}
			if cancel != nil {
				defer cancel()
			}
			if e := verifyCompleteRangeCount(ctx, nil, 0, 0); e == nil || e.Error() != want {
				t.Fatal("invalid COUNT budget reached the reader", e)
			}
		})
	}
}

func TestEveryActionUsesFiniteSocketReadButShortConnectionLimits(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	conf := mysql.NewConfig()
	conf.MultiStatements, conf.ParseTime = true, true
	conf.Params = map[string]string{"charset": "utf8mb3"}
	if !configureMySQLAction(ctx, conf) || conf.MultiStatements || conf.ParseTime || conf.Timeout != queryLimit || conf.WriteTimeout != queryLimit || conf.ReadTimeout <= queryLimit || conf.ReadTimeout > time.Minute || conf.Params["charset"] != "utf8mb3" {
		t.Fatal("action socket budget changed short dial/write limits or connection identity")
	}
	if configureMySQLAction(context.Background(), mysql.NewConfig()) {
		t.Fatal("unbounded action socket wait accepted")
	}
	canceled, c := context.WithCancel(ctx)
	c()
	if configureMySQLAction(canceled, mysql.NewConfig()) {
		t.Fatal("canceled action accepted")
	}
}

func TestCompleteLedgerRejectsExtraColumns(t *testing.T) {
	for _, name := range []string{"exact", "extra", "nullable", "wrong_type"} {
		t.Run(name, func(t *testing.T) {
			db, mock, e := sqlmock.New()
			if e != nil {
				t.Fatal(e)
			}
			defer func() { _ = db.Close() }()
			rows := sqlmock.NewRows([]string{"COLUMN_NAME", "COLUMN_TYPE", "IS_NULLABLE"}).AddRow("version", "bigint", "NO")
			kind, nullable := "tinyint(1)", "NO"
			if name == "nullable" {
				nullable = "YES"
			}
			if name == "wrong_type" {
				kind = "tinyint unsigned"
			}
			rows.AddRow("dirty", kind, nullable)
			if name == "extra" {
				rows.AddRow("metadata", "json", "NO")
			}
			mock.ExpectQuery("SELECT COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE.*schema_migrations").WillReturnRows(rows)
			e = requireLedgerShape(context.Background(), db)
			if (e == nil) != (name == "exact") {
				t.Fatal("incomplete SQL ledger scope accepted", e)
			}
			if e := mock.ExpectationsWereMet(); e != nil {
				t.Fatal(e)
			}
		})
	}
}

func TestLedgerRequiresOneTypedRow(t *testing.T) {
	for _, name := range []string{"exact", "missing", "extra", "dirty_type"} {
		t.Run(name, func(t *testing.T) {
			db, mock, e := sqlmock.New()
			if e != nil {
				t.Fatal(e)
			}
			defer func() { _ = db.Close() }()
			rows := sqlmock.NewRows([]string{"version", "dirty"})
			if name != "missing" {
				if name == "dirty_type" {
					rows.AddRow(101, nil)
				} else {
					rows.AddRow(101, false)
				}
			}
			if name == "extra" {
				rows.AddRow(102, false)
			}
			mock.ExpectQuery("SELECT version,dirty FROM schema_migrations").WillReturnRows(rows)
			got, e := readLedger(context.Background(), db)
			if (e == nil) != (name == "exact") || (e == nil && got[0] != (ledgerRow{101, false})) {
				t.Fatal("unproved ledger accepted", e)
			}
			if e := mock.ExpectationsWereMet(); e != nil {
				t.Fatal(e)
			}
		})
	}
}
