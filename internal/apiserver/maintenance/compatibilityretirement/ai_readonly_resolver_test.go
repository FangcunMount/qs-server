package retirement

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"go.mongodb.org/mongo-driver/bson"
)

func TestAIReadOnlyStatementClosedReadSet(t *testing.T) {
	queries := []string{aiRequestQuery, aiBridgeQuery, aiLegacyQuery, aiOperationQuery, aiOutboxQuery, aiAdmissionQuery,
		"SELECT @@server_uuid,DATABASE(),VERSION()",
		"SELECT CAST(version AS BINARY),CAST(dirty AS BINARY) FROM schema_migrations FOR UPDATE",
		"SHOW CREATE TABLE `ai_bridge_commands`", "SHOW CREATE TABLE `ai_messaging_legacy_commands`",
		"SELECT COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE,COLUMN_DEFAULT,EXTRA,COLLATION_NAME FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? ORDER BY ORDINAL_POSITION",
		"SELECT CAST(event_id AS BINARY),CAST(request_id AS BINARY),CAST(version AS BINARY),CAST(payload_hash AS BINARY) FROM ai_bridge_events WHERE event_id=? OR request_id=? ORDER BY event_id LIMIT 1025 FOR UPDATE",
		"SELECT CAST(request_id AS BINARY),CAST(assessment_id AS BINARY) FROM ai_bridge_request_assessments WHERE request_id=? ORDER BY assessment_id LIMIT 1025 FOR UPDATE",
		"SELECT CAST(aggregate_key AS BINARY),CAST(next_sequence AS BINARY) FROM ai_messaging_aggregates WHERE aggregate_key=? FOR UPDATE",
		"SELECT CAST(wire_sha256 AS BINARY),CAST(code AS BINARY),CAST(attempts AS BINARY) FROM ai_messaging_quarantine ORDER BY wire_sha256 LIMIT 1 FOR UPDATE"}
	for _, base := range []string{aiInboxBase, aiFailureBase} {
		for _, count := range []int{1, 3, 2*aiLocalMaxRows + 3} {
			q, _ := aiIDsClause(base, make([]string, count), " ORDER BY producer,message_id LIMIT 1025 FOR UPDATE")
			queries = append(queries, q)
		}
	}
	for i, q := range queries {
		got, err := aiReadOnlyStatement(q)
		want := strings.TrimSuffix(strings.TrimSuffix(q, " FOR UPDATE"), " LOCK IN SHARE MODE")
		if err != nil || got != want || strings.Contains(got, "FOR UPDATE") || strings.Contains(got, "LOCK IN SHARE MODE") {
			t.Fatal("readonly fixed read changed more than its terminal lock", i)
		}
	}
	for _, q := range []string{"UPDATE ai_messaging_admission SET closed=TRUE", "SELECT * FROM ai_bridge_commands", aiBridgeQuery + ";DELETE FROM ai_bridge_commands", aiBridgeQuery + " ", strings.TrimSuffix(aiBridgeQuery, " FOR UPDATE"), "SHOW CREATE TABLE `private_target`", aiInboxBase + "'private') ORDER BY producer,message_id LIMIT 1025 FOR UPDATE", aiInboxBase + "?) OR TRUE ORDER BY producer,message_id LIMIT 1025 FOR UPDATE", aiFailureBase + ") ORDER BY producer,message_id LIMIT 1025 FOR UPDATE"} {
		if got, err := aiReadOnlyStatement(q); err != ErrAILocalRead || got != "" {
			t.Fatal("nonfixed statement admitted")
		}
	}
	q, _ := aiIDsClause(aiInboxBase, make([]string, 2*aiLocalMaxRows+4), " ORDER BY producer,message_id LIMIT 1025 FOR UPDATE")
	if _, err := aiReadOnlyStatement(q); err != ErrAILocalRead {
		t.Fatal("unbounded identity list admitted")
	}
}

func TestAIReadOnlyReadRewritesOnlyPrivateMode(t *testing.T) {
	for _, readonly := range []bool{false, true} {
		t.Run(map[bool]string{false: "rw_lock_unchanged", true: "readonly_exact_select"}[readonly], func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal("mock preparation")
			}
			defer func() {
				if db.Close() != nil {
					t.Error("mock close")
				}
			}()
			r := &AILocalResolver{pool: db}
			q := aiBridgeQuery
			if readonly {
				r.readOnly = &aiReadOnlyMode{closed: "0"}
				q = strings.TrimSuffix(q, " FOR UPDATE")
			}
			mock.ExpectQuery("^"+regexp.QuoteMeta(q)+"$").WithArgs("original", "request").WillReturnRows(sqlmock.NewRows([]string{"fact"}).AddRow("private-body")).RowsWillBeClosed()
			if rows, err := r.read(context.Background(), aiBridgeQuery, "original", "request"); err != nil || len(rows) != 1 {
				t.Fatal("fixed read not executed")
			}
			if readonly {
				if _, err := r.read(context.Background(), "DELETE FROM ai_bridge_commands"); err != ErrAILocalRead {
					t.Fatal("write reached pool")
				}
			}
			mock.ExpectClose()
			if err = db.Close(); err != nil {
				t.Fatal("mock close")
			}
			if mock.ExpectationsWereMet() != nil {
				t.Fatal("read statement/lifecycle mismatch")
			}
		})
	}
}

func TestAIReadOnlyAdmissionBindsFullSourceRow(t *testing.T) {
	var prior string
	for _, closed := range []string{"0", "1"} {
		rows := [][][]byte{{[]byte(closed), []byte("0"), []byte("2026-10-08 11:12:14.123456")}}
		got, revision, digest, err := aiAdmissionSnapshot(rows)
		if err != nil || got != closed || revision != 0 || !evidenceHash(digest) || digest == prior {
			t.Fatal("actual admission fact rejected or conflated")
		}
		prior = digest
		rows[0][2] = []byte("2026-10-08 11:12:14.123457")
		_, _, changed, err := aiAdmissionSnapshot(rows)
		if err != nil || changed == digest {
			t.Fatal("admission update time not bound")
		}
	}
	for _, rows := range [][][][]byte{nil, {{[]byte("2"), []byte("7"), []byte("at")}}, {{[]byte("0"), []byte("07"), []byte("at")}}, {{[]byte("1"), nil, []byte("at")}}, {{[]byte("1"), []byte("7"), nil}}, {{[]byte("1"), []byte("7")}}} {
		if _, _, _, err := aiAdmissionSnapshot(rows); err != ErrAILocalBinding {
			t.Fatal("unknown admission admitted")
		}
	}
}

func TestAIReadOnlyOpaquePreparationAndPageRefuseMissingSnapshot(t *testing.T) {
	f := sourceAuthFixture(t, true, true)
	c, err := PrepareHistoricalCoordinator(context.Background(), coordinatorBinding(), f.inputs(), DefaultHistoricalCoordinatorLimits())
	if err != nil {
		t.Fatal("coordinator fixture")
	}
	for _, snapshot := range []*SQLResponsibilitySnapshot{nil, {}} {
		if _, err := c.PrepareAIReadOnlyResolver(context.Background(), snapshot, 99, f.inputs()[1:3]); err == nil {
			t.Fatal("missing actual snapshot admitted")
		}
	}
	p, err := c.NextPage(context.Background())
	if err != nil {
		t.Fatal("source page")
	}
	if err = c.QualifyAIReadOnlyPage(context.Background(), p, nil); err != ErrAILocalBinding || p.consumed || len(c.candidates) != 0 {
		t.Fatal("missing readonly authority consumed page")
	}
	for _, value := range []any{&AIReadOnlyResolver{}, &AIReadOnlyQualification{}} {
		if _, err := json.Marshal(value); !errors.Is(err, ErrSourceSerialization) {
			t.Fatal("opaque JSON authority exported")
		}
		if _, err := bson.Marshal(value); err != ErrSourceSerialization {
			t.Fatal("opaque BSON authority exported")
		}
	}
	q := &AIReadOnlyQualification{summary: AIReadOnlySummary{AILocalSummary: AILocalSummary{Scope: "qs-local-readonly-point-graph-only", LocalQualified: true, Gaps: []string{"external_unknown"}, ExternalClosure: "unknown", GlobalReverseCoverage: "unknown", RetirementConclusion: "not_generated"}, LocalReadOnly: true}}
	s := q.Summary()
	s.Gaps[0] = "edited"
	if q.Summary().Gaps[0] != "external_unknown" || s.CASAuthority || s.DropReady {
		t.Fatal("summary changes authority or aliases private facts")
	}
	if _, err = c.NextPage(context.Background()); err == io.EOF {
		t.Fatal("unconsumed page reached EOF")
	}
}
