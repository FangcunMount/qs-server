//go:build integration

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	retirement "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirement"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	eventoutcome "github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const historyDecoderLargeID = int64(639678084915671598)

func historyDecoderClock() time.Time {
	return time.Date(2026, 10, 8, 11, 12, 13, 123456789, time.FixedZone("fixture", 8*3600))
}

func historyDecoderWire(t *testing.T, kind, id string) []byte {
	t.Helper()
	clock := historyDecoderClock()
	base := event.BaseEvent{ID: id, EventTypeValue: kind, OccurredAtValue: clock.Add(time.Second), AggregateTypeValue: "Evaluation", AggregateIDValue: strconv.FormatInt(historyDecoderLargeID, 10)}
	var evt event.DomainEvent
	switch kind {
	case "evaluation.requested", "evaluation.retry.requested":
		p := eventpayload.EvaluationRequestedData{OrgID: historyDecoderLargeID, AssessmentID: historyDecoderLargeID, TesteeID: math.MaxUint64, QuestionnaireCode: "q-code", QuestionnaireVer: "v1", AnswerSheetID: "sheet-1", ModelKind: "scale", ModelCode: "scale-1", ModelVersion: "v1", RequestedAt: clock}
		if kind == "evaluation.retry.requested" {
			p.ExpectedAttempt = 2
			p.AttemptOrigin = "automatic"
			p.Mode = "next_attempt"
		}
		evt = event.Event[eventpayload.EvaluationRequestedData]{BaseEvent: base, Data: p}
	case "evaluation.failed":
		evt = event.Event[eventpayload.EvaluationFailedData]{BaseEvent: base, Data: eventpayload.EvaluationFailedData{OrgID: historyDecoderLargeID, AssessmentID: historyDecoderLargeID, TesteeID: math.MaxUint64, Reason: "fixture historical failure", FailedAt: clock}}
	case "evaluation.outcome.committed":
		evt = event.Event[eventpayload.EvaluationOutcomeCommittedData]{BaseEvent: base, Data: eventpayload.EvaluationOutcomeCommittedData{OrgID: historyDecoderLargeID, AssessmentID: historyDecoderLargeID, TesteeID: math.MaxUint64, OutcomeID: "outcome-1", EvaluationRunID: "run-1", CommittedAt: clock}}
	case "answersheet.submitted":
		base.AggregateTypeValue = "AnswerSheet"
		base.AggregateIDValue = "sheet-1"
		evt = event.Event[eventpayload.AnswerSheetSubmittedData]{BaseEvent: base, Data: eventpayload.AnswerSheetSubmittedData{AnswerSheetID: "sheet-1", QuestionnaireCode: "q-code", QuestionnaireVersion: "v1", OrgID: uint64(historyDecoderLargeID), TesteeID: math.MaxUint64, FillerID: math.MaxUint64, FillerType: "testee", SubmittedAt: clock, Admission: &eventpayload.AssessmentAdmission{Purpose: eventpayload.AdmissionPurposeIndependentQuestionnaire, QuestionnaireCode: "q-code", QuestionnaireVersion: "v1"}}}
	case "interpretation.report.generated":
		base.AggregateTypeValue = "ReportGeneration"
		base.AggregateIDValue = "generation-1"
		evt = event.Event[eventoutcome.ReportGeneratedPayload]{BaseEvent: base, Data: eventoutcome.ReportGeneratedPayload{OrgID: historyDecoderLargeID, GenerationID: "generation-1", RunID: "run-1", ReportID: "report-1", AssessmentID: strconv.FormatInt(historyDecoderLargeID, 10), OutcomeID: "outcome-1", TesteeID: math.MaxUint64, Attempt: 2, ReportType: "scale", TemplateVersion: "v1", BuilderIdentity: "builder:v1", ContentSchemaVersion: "v1", Model: eventoutcome.ModelIdentity{Kind: "scale", Code: "scale-1", Version: "v1"}, PrimaryScore: &eventoutcome.ScoreValue{Kind: "score", Value: 5.1}, GeneratedAt: clock}}
	default:
		t.Fatal("fixture event type invalid")
	}
	raw, err := domainwire.EncodeEvent(evt)
	if err != nil {
		t.Fatal("fixture wire encoding failed")
	}
	return raw
}

func historyDecoderExpectation(t *testing.T, b targetBoundary, s snapshot) retirement.SourceCopyExpectation {
	t.Helper()
	if !s.Complete || s.SourceFile == "" || s.Passes != 2 || s.Records == 0 {
		t.Fatal("actual inventory producer did not complete")
	}
	// Independent test approval occurs only after the producer's two passes,
	// using the previously frozen real boundary. It is no production gate.
	return retirement.SourceCopyExpectation{Boundary: retirement.SourceBoundary{Database: b.Database, Name: b.Name, Kind: b.Kind, Present: b.Present, Empty: b.Empty, PKType: b.PKType, UpperToken: b.UpperToken, SchemaHash: b.SchemaHash, IdentityHash: b.IdentityHash}, DataHash: s.DataHash, Records: s.Records, Bytes: s.Bytes}
}

func historyDecoderCtx(ctx context.Context) context.Context {
	return context.WithValue(ctx, scanRunKey{}, scanRunIdentity{OperationID: "123-1", RunID: "789-1", RequestHash: strings.Repeat("c", 64)})
}

func historyDecoderReadSQL(t *testing.T, path string, expected retirement.SourceCopyExpectation) []*retirement.DecodedSourceEvent {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal("private copy unavailable")
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Error("copy close failed")
		}
	}()
	reader, err := retirement.NewSQLSourceReader(f, expected)
	if err != nil {
		t.Fatal(err)
	}
	var out []*retirement.DecodedSourceEvent
	for {
		v, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, v)
		if reader.Receipt().Complete {
			t.Fatal("premature source proof")
		}
	}
	if !reader.Receipt().Complete || reader.Receipt().DropReady || reader.Receipt().BusinessClosureVerified || reader.Receipt().Records != expected.Records {
		t.Fatal("source-only completion invalid")
	}
	return out
}

func historyDecoderReadMongo(t *testing.T, path string, expected retirement.SourceCopyExpectation) []*retirement.DecodedSourceEvent {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal("private copy unavailable")
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Error("copy close failed")
		}
	}()
	reader, err := retirement.NewMongoSourceReader(f, expected)
	if err != nil {
		t.Fatal(err)
	}
	var out []*retirement.DecodedSourceEvent
	for {
		v, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, v)
		if reader.Receipt().Complete {
			t.Fatal("premature source proof")
		}
	}
	if !reader.Receipt().Complete || reader.Receipt().DropReady || reader.Receipt().BusinessClosureVerified || reader.Receipt().Records != expected.Records {
		t.Fatal("source-only completion invalid")
	}
	return out
}

func TestHistoryDecoderActualMySQLPagedSourceNative(t *testing.T) {
	if os.Getenv("QS_RETIREMENT_HISTORY_SOURCE_NATIVE_REQUIRED") == "1" && os.Getenv("QS_RETIREMENT_LOCAL_INTEGRATION") != "1" {
		t.Fatal("owned native fixture missing")
	}
	localInventoryGuard(t)
	sourceSHA = strings.Repeat("a", 40)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	admin, err := mysqlOpen(ctx)
	if err != nil {
		t.Fatal("owned fixture connection failed")
	}
	defer func() {
		if err := admin.Close(); err != nil {
			t.Error("fixture pool close failed")
		}
	}()
	name := "qs_retirement_inventory_test_history_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+quote(name)); err != nil {
		t.Fatal("isolated database create failed")
	}
	defer func() {
		if _, err := admin.ExecContext(context.Background(), "DROP DATABASE "+quote(name)); err != nil {
			t.Error("isolated database cleanup failed")
		}
	}()
	t.Setenv("MYSQL_DATABASE", name)
	db, err := mysqlOpen(ctx)
	if err != nil {
		t.Fatal("isolated connection failed")
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error("isolated pool close failed")
		}
	}()
	// Use the actual immutable historical DDL resources, not a guessed test
	// schema. Migration49's DCE ALTER is extracted as its exact one statement;
	// its unrelated checkpoint/data updates are outside this source test.
	create, err := os.ReadFile("../../internal/pkg/migration/migrations/mysql/000018_add_domain_event_outbox.up.sql")
	if err != nil {
		t.Fatal("historical18 resource unavailable")
	}
	if _, err := db.ExecContext(ctx, string(create)); err != nil {
		t.Fatal("historical18 create failed")
	}
	resource49, err := os.ReadFile("../../internal/pkg/migration/migrations/mysql/000049_add_retry_governance.up.sql")
	if err != nil {
		t.Fatal("historical49 resource unavailable")
	}
	text49 := string(resource49)
	start := strings.Index(text49, "ALTER TABLE `domain_event_outbox`")
	if start < 0 {
		t.Fatal("historical DCE ALTER absent")
	}
	end := strings.IndexByte(text49[start:], ';')
	if end < 0 {
		t.Fatal("historical DCE ALTER invalid")
	}
	if _, err := db.ExecContext(ctx, text49[start:start+end+1]); err != nil {
		t.Fatal("historical49 alter failed")
	}
	kinds := []string{"evaluation.requested", "evaluation.outcome.committed", "evaluation.retry.requested", "evaluation.failed"}
	for i, kind := range kinds {
		raw := historyDecoderWire(t, kind, "history-sql-event-"+strconv.Itoa(i))
		clock := historyDecoderClock().UTC().Truncate(time.Millisecond)
		if _, err := db.ExecContext(ctx, "INSERT INTO domain_event_outbox(event_id,event_type,aggregate_type,aggregate_id,org_id,topic_name,payload_json,status,attempt_count,next_attempt_at,last_error,created_at,updated_at,published_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)", "history-sql-event-"+strconv.Itoa(i), kind, "Evaluation", strconv.FormatInt(historyDecoderLargeID, 10), historyDecoderLargeID, "qs-evaluation", string(raw), "published", i, clock, func() any {
			if i == 0 {
				return nil
			}
			return ""
		}(), clock, clock, clock); err != nil {
			t.Fatal("real SQL producer row insert failed")
		}
	}
	var version string
	if db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version) != nil || !strings.HasPrefix(version, "8.") {
		t.Fatal("fixture is not MySQL8")
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		t.Fatal("read snapshot failed")
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			t.Error("read snapshot release failed")
		}
	}()
	catalog, defs, err := mysqlCatalog(ctx, tx)
	if err != nil {
		t.Fatal("real schema read failed")
	}
	r := request{FormatVersion: 2, Kind: "readonly_inventory_boundary_request", Limits: productionLimits()}
	bounded, err := mysqlTargetV2(ctx, tx, "domain_event_outbox", catalog["domain_event_outbox"], defs, privateTestDir(t), r)
	if err != nil || !bounded.Complete || bounded.Boundary == nil {
		t.Fatal("actual SQL fixed boundary failed")
	}
	b := *bounded.Boundary
	r.Kind = "readonly_inventory_request"
	r.Boundaries = []targetBoundary{b}
	dir := privateTestDir(t)
	produced, err := mysqlTargetV2(historyDecoderCtx(ctx), tx, "domain_event_outbox", catalog["domain_event_outbox"], defs, dir, r)
	if err != nil {
		t.Fatal("actual SQL two-pass source producer failed")
	}
	values := historyDecoderReadSQL(t, filepath.Join(dir, produced.SourceFile), historyDecoderExpectation(t, b, produced))
	if len(values) != 4 {
		t.Fatal("SQL event coverage incomplete")
	}
	for i, v := range values {
		if v.EventType != kinds[i] || v.OrgID != uint64(historyDecoderLargeID) || v.BusinessIDs["testee_id"] != "18446744073709551615" || !v.BusinessAt.Equal(historyDecoderClock()) {
			t.Fatal("actual CAST lost original typed facts")
		}
	}
	if values[0].Transport.Fields["last_error"] != nil || values[1].Transport.Fields["last_error"] == nil || *values[1].Transport.Fields["last_error"] != "" {
		t.Fatal("real SQL NULL and empty collapsed")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal("accepted source snapshot release failed")
	}
	original := historyDecoderWire(t, "evaluation.requested", "history-sql-event-0")
	for _, tc := range []struct {
		name string
		body []byte
		want error
	}{
		{"server_stored_unknown_schema", bytes.Replace(original, []byte(`"org_id":`), []byte(`"unknown_legacy_body":true,"org_id":`), 1), retirement.ErrSourceUnknownField},
		{"server_stored_duplicate_key", bytes.Replace(original, []byte(`"org_id":639678084915671598`), []byte(`"org_id":639678084915671598,"org_id":639678084915671598`), 1), retirement.ErrSourceDuplicateKey},
		{"server_stored_precision_loss", bytes.Replace(original, []byte(`18446744073709551615`), []byte(`18446744073709551616`), 1), retirement.ErrSourcePrecision},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := db.ExecContext(ctx, "UPDATE domain_event_outbox SET payload_json=? WHERE id=1", string(tc.body)); err != nil {
				t.Fatal("owned source mutation failed")
			}
			path, approved := historyDecoderActualSQLCopy(t, ctx, db)
			f, err := os.Open(path)
			if err != nil {
				t.Fatal("private copy unavailable")
			}
			defer func() {
				if err := f.Close(); err != nil {
					t.Error("copy close failed")
				}
			}()
			reader, err := retirement.NewSQLSourceReader(f, approved)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = reader.Next(); err != tc.want || reader.Receipt().Complete {
				t.Fatal("server stored unsupported body not rejected")
			}
		})
	}
	t.Run("actual_additional_column_schema", func(t *testing.T) {
		if _, err := db.ExecContext(ctx, "UPDATE domain_event_outbox SET payload_json=? WHERE id=1", string(original)); err != nil {
			t.Fatal("owned original body restoration failed")
		}
		if _, err := db.ExecContext(ctx, "ALTER TABLE domain_event_outbox ADD COLUMN unknown_payload LONGTEXT NULL"); err != nil {
			t.Fatal("owned unknown schema fixture failed")
		}
		path, approved := historyDecoderActualSQLCopy(t, ctx, db)
		f, err := os.Open(path)
		if err != nil {
			t.Fatal("private copy unavailable")
		}
		defer func() {
			if err := f.Close(); err != nil {
				t.Error("copy close failed")
			}
		}()
		if _, err := retirement.NewSQLSourceReader(f, approved); err != retirement.ErrSourceSchema {
			t.Fatal("real unknown schema was projected")
		}
	})
}

func historyDecoderActualSQLCopy(t *testing.T, ctx context.Context, db *sql.DB) (string, retirement.SourceCopyExpectation) {
	t.Helper()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		t.Fatal("read snapshot failed")
	}
	defer func() {
		if err := tx.Rollback(); err != nil {
			t.Error("read snapshot release failed")
		}
	}()
	catalog, defs, err := mysqlCatalog(ctx, tx)
	if err != nil {
		t.Fatal("actual metadata read failed")
	}
	r := request{FormatVersion: 2, Kind: "readonly_inventory_boundary_request", Limits: productionLimits()}
	bound, err := mysqlTargetV2(ctx, tx, "domain_event_outbox", catalog["domain_event_outbox"], defs, privateTestDir(t), r)
	if err != nil || bound.Boundary == nil || !bound.Complete {
		t.Fatal("actual boundary failed")
	}
	b := *bound.Boundary
	r.Kind = "readonly_inventory_request"
	r.Boundaries = []targetBoundary{b}
	dir := privateTestDir(t)
	out, err := mysqlTargetV2(historyDecoderCtx(ctx), tx, "domain_event_outbox", catalog["domain_event_outbox"], defs, dir, r)
	if err != nil {
		t.Fatal("actual two-pass source failed")
	}
	return filepath.Join(dir, out.SourceFile), historyDecoderExpectation(t, b, out)
}

// This is the actual retired Mongo OutboxPO shape from dce4598a4^, without _id:
// the native server assigns it unless a scalar _id is explicitly supplied.
type historyDecoderLegacyMongoPO struct {
	EventID               string    `bson:"event_id"`
	EventType             string    `bson:"event_type"`
	AggregateType         string    `bson:"aggregate_type"`
	AggregateID           string    `bson:"aggregate_id"`
	OrgID                 *int64    `bson:"org_id,omitempty"`
	TopicName             string    `bson:"topic_name"`
	PayloadJSON           string    `bson:"payload_json"`
	Status                string    `bson:"status"`
	AttemptCount          int       `bson:"attempt_count"`
	RetryDisposition      string    `bson:"retry_disposition,omitempty"`
	NextAttemptAt         time.Time `bson:"next_attempt_at"`
	LastError             string    `bson:"last_error,omitempty"`
	LastErrorKind         string    `bson:"last_error_kind,omitempty"`
	ManualReplayRequestID string    `bson:"manual_replay_request_id,omitempty"`
	CreatedAt             time.Time `bson:"created_at"`
	UpdatedAt             time.Time `bson:"updated_at"`
	PublishedAt           time.Time `bson:"published_at,omitempty"`
	ClaimToken            string    `bson:"claim_token,omitempty"`
}

func TestHistoryDecoderActualMongoPagedServerBSONNative(t *testing.T) {
	if os.Getenv("QS_RETIREMENT_HISTORY_SOURCE_NATIVE_REQUIRED") == "1" && os.Getenv("QS_RETIREMENT_LOCAL_INTEGRATION") != "1" {
		t.Fatal("owned native fixture missing")
	}
	localInventoryGuard(t)
	uri := os.Getenv("QS_RETIREMENT_HISTORY_SOURCE_MONGO_URI")
	if uri != "mongodb://127.0.0.1:33317/?replicaSet=qscompat&directConnection=true" {
		t.Fatal("owned loopback replica URI missing/rejected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri).SetMaxPoolSize(1))
	if err != nil {
		t.Fatal("owned Mongo connect failed")
	}
	defer func() {
		if err := client.Disconnect(context.Background()); err != nil {
			t.Error("owned client close failed")
		}
	}()
	if client.Ping(ctx, nil) != nil {
		t.Fatal("owned Mongo not ready")
	}
	sourceSHA = strings.Repeat("a", 40)
	name := "qs_retirement_inventory_test_history_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	db := client.Database(name)
	defer func() {
		if err := db.Drop(context.Background()); err != nil {
			t.Error("owned Mongo database cleanup failed")
		}
	}()
	for _, pkType := range []string{"objectId", "string", "long"} {
		t.Run(pkType, func(t *testing.T) {
			col := db.Collection("domain_event_outbox")
			for i, kind := range []string{"answersheet.submitted", "interpretation.report.generated"} {
				id := "history-mongo-event-" + strconv.Itoa(i)
				body := historyDecoderWire(t, kind, id)
				aggregateType, aggregateID := "AnswerSheet", "sheet-1"
				if i == 1 {
					aggregateType, aggregateID = "ReportGeneration", "generation-1"
				}
				org := historyDecoderLargeID
				po := historyDecoderLegacyMongoPO{EventID: id, EventType: kind, AggregateType: aggregateType, AggregateID: aggregateID, OrgID: &org, TopicName: "qs-survey", PayloadJSON: string(body), Status: "published", AttemptCount: i, NextAttemptAt: historyDecoderClock(), CreatedAt: historyDecoderClock(), UpdatedAt: historyDecoderClock(), PublishedAt: historyDecoderClock()}
				var doc any = po
				if pkType != "objectId" {
					raw, err := bson.Marshal(po)
					if err != nil {
						t.Fatal("legacy PO BSON encoding failed")
					}
					var d bson.D
					if bson.Unmarshal(raw, &d) != nil {
						t.Fatal("legacy PO BSON decode failed")
					}
					var pk any = strconv.FormatInt(historyDecoderLargeID+int64(i), 10)
					if pkType == "long" {
						pk = historyDecoderLargeID + int64(i)
					}
					doc = append(bson.D{{Key: "_id", Value: pk}}, d...)
				}
				if _, err := col.InsertOne(ctx, doc); err != nil {
					t.Fatal("legacy PO server insert failed")
				}
			}
			catalog, defs, err := mongoSchemas(ctx, db)
			if err != nil {
				t.Fatal("actual Mongo metadata failed")
			}
			r := request{FormatVersion: 2, Kind: "readonly_inventory_boundary_request", Limits: productionLimits()}
			bounded, err := mongoTargetV2(ctx, db, catalog, defs, privateTestDir(t), r)
			if err != nil || !bounded.Complete || bounded.Boundary == nil || bounded.Boundary.PKType != pkType {
				t.Fatal("actual Mongo fixed typed boundary failed")
			}
			b := *bounded.Boundary
			r.Kind = "readonly_inventory_request"
			r.Boundaries = []targetBoundary{b}
			dir := privateTestDir(t)
			produced, err := mongoTargetV2(historyDecoderCtx(ctx), db, catalog, defs, dir, r)
			if err != nil {
				t.Fatal("actual Mongo two-pass server source failed")
			}
			values := historyDecoderReadMongo(t, filepath.Join(dir, produced.SourceFile), historyDecoderExpectation(t, b, produced))
			if len(values) != 2 {
				t.Fatal("Mongo event coverage incomplete")
			}
			seen := map[string]bool{}
			for _, v := range values {
				if v.OrgID != uint64(historyDecoderLargeID) || v.BusinessIDs["testee_id"] != "18446744073709551615" || !v.BusinessAt.Equal(historyDecoderClock()) {
					t.Fatal("server BSON lost typed payload facts")
				}
				seen[v.EventType] = true
				token, err := base64DecodeHistoryToken(v.PrimaryKeyToken)
				if err != nil || mongoPKKindHistoryToken(token) != pkType {
					t.Fatal("real ordered BSON PK type changed")
				}
			}
			if !seen["answersheet.submitted"] || !seen["interpretation.report.generated"] {
				t.Fatal("Mongo types lost")
			}
			t.Run("server_stored_unknown_body", func(t *testing.T) {
				original := historyDecoderWire(t, "answersheet.submitted", "history-mongo-event-0")
				bad := bytes.Replace(original, []byte(`"org_id":`), []byte(`"unknown_legacy_body":true,"org_id":`), 1)
				if _, err := col.UpdateOne(ctx, bson.D{{Key: "event_id", Value: "history-mongo-event-0"}}, bson.D{{Key: "$set", Value: bson.D{{Key: "payload_json", Value: string(bad)}}}}); err != nil {
					t.Fatal("owned Mongo source mutation failed")
				}
				dir := privateTestDir(t)
				produced, err := mongoTargetV2(historyDecoderCtx(ctx), db, catalog, defs, dir, r)
				if err != nil {
					t.Fatal("actual Mongo mutated two-pass source failed")
				}
				f, err := os.Open(filepath.Join(dir, produced.SourceFile))
				if err != nil {
					t.Fatal("private source unavailable")
				}
				defer func() {
					if err := f.Close(); err != nil {
						t.Error("source close failed")
					}
				}()
				reader, err := retirement.NewMongoSourceReader(f, historyDecoderExpectation(t, b, produced))
				if err != nil {
					t.Fatal(err)
				}
				for {
					_, err = reader.Next()
					if err != nil {
						break
					}
				}
				if err != retirement.ErrSourceUnknownField || reader.Receipt().Complete {
					t.Fatal("server stored unsupported Mongo body not rejected")
				}
			})
			if err := col.Drop(ctx); err != nil {
				t.Fatal("owned collection cleanup failed")
			}
		})
	}
}

func base64DecodeHistoryToken(token string) ([]byte, error) {
	// The existing pager validates/decodes exactly this opaque BSON token.
	var raw []byte
	var err error
	raw, err = base64.StdEncoding.Strict().DecodeString(token)
	return raw, err
}
func mongoPKKindHistoryToken(raw []byte) string {
	_, kind, err := decodeMongoToken(raw)
	if err != nil {
		return ""
	}
	return kind
}
