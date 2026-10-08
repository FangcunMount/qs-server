package retirement

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	eventoutcome "github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var fixtureClock = time.Date(2026, 10, 8, 11, 12, 13, 123456789, time.FixedZone("fixture", 8*3600))

const fixtureLargeID = int64(639678084915671598)

// These fixtures use the real host generic Event + exact typed producer data.
// Historical provenance: dce4598a4^ domain/evaluation/event/events.go,
// domain/survey/answersheet/events.go, domain/interpretation/outcome.go and
// infra/{mysql,mongo}/eventoutbox/store.go; component-base v0.6.11 eventcodec
// EncodeDomainEvent and current domainwire.EncodeEvent both json.Marshal(evt).
// They are synthetic identities, not copied production payloads.
func wireFixture(t *testing.T, kind string) []byte {
	t.Helper()
	base := event.BaseEvent{ID: "original-event-1", EventTypeValue: kind, OccurredAtValue: fixtureClock.Add(time.Second), AggregateTypeValue: "Evaluation", AggregateIDValue: "639678084915671598"}
	var evt event.DomainEvent
	switch kind {
	case "evaluation.requested", "evaluation.retry.requested":
		p := eventpayload.EvaluationRequestedData{OrgID: fixtureLargeID, AssessmentID: fixtureLargeID, TesteeID: math.MaxUint64, QuestionnaireCode: "q-code", QuestionnaireVer: "v1", AnswerSheetID: "sheet-1", ModelKind: "scale", ModelCode: "scale-1", ModelVersion: "v1", RequestedAt: fixtureClock}
		if kind == "evaluation.retry.requested" {
			p.ExpectedAttempt = 2
			p.AttemptOrigin = "automatic"
			p.Mode = "next_attempt"
		}
		evt = event.Event[eventpayload.EvaluationRequestedData]{BaseEvent: base, Data: p}
	case "evaluation.failed":
		evt = event.Event[eventpayload.EvaluationFailedData]{BaseEvent: base, Data: eventpayload.EvaluationFailedData{OrgID: fixtureLargeID, AssessmentID: fixtureLargeID, TesteeID: math.MaxUint64, Reason: "historical reason", FailedAt: fixtureClock}}
	case "evaluation.outcome.committed":
		evt = event.Event[eventpayload.EvaluationOutcomeCommittedData]{BaseEvent: base, Data: eventpayload.EvaluationOutcomeCommittedData{OrgID: fixtureLargeID, AssessmentID: fixtureLargeID, TesteeID: math.MaxUint64, OutcomeID: "outcome-1", EvaluationRunID: "run-1", CommittedAt: fixtureClock}}
	case "answersheet.submitted":
		base.AggregateTypeValue = "AnswerSheet"
		base.AggregateIDValue = "sheet-1"
		evt = event.Event[eventpayload.AnswerSheetSubmittedData]{BaseEvent: base, Data: eventpayload.AnswerSheetSubmittedData{AnswerSheetID: "sheet-1", QuestionnaireCode: "q-code", QuestionnaireVersion: "v1", TesteeID: math.MaxUint64, OrgID: uint64(fixtureLargeID), FillerID: math.MaxUint64, FillerType: "testee", SubmittedAt: fixtureClock, Admission: &eventpayload.AssessmentAdmission{Purpose: eventpayload.AdmissionPurposeIndependentQuestionnaire, QuestionnaireCode: "q-code", QuestionnaireVersion: "v1"}}}
	case "interpretation.report.generated":
		base.AggregateTypeValue = "ReportGeneration"
		base.AggregateIDValue = "generation-1"
		evt = event.Event[eventoutcome.ReportGeneratedPayload]{BaseEvent: base, Data: eventoutcome.ReportGeneratedPayload{OrgID: fixtureLargeID, GenerationID: "generation-1", RunID: "run-1", ReportID: "report-1", AssessmentID: "639678084915671598", OutcomeID: "outcome-1", TesteeID: math.MaxUint64, Attempt: 2, ReportType: "scale", TemplateVersion: "v1", BuilderIdentity: "builder:v1", ContentSchemaVersion: "v1", Model: eventoutcome.ModelIdentity{Kind: "scale", Code: "scale-1", Version: "v1"}, PrimaryScore: &eventoutcome.ScoreValue{Kind: "score", Value: 5.1}, GeneratedAt: fixtureClock}}
	default:
		t.Fatal("unsupported fixture")
	}
	raw, err := domainwire.EncodeEvent(evt)
	if err != nil {
		t.Fatal("fixture encoding failed")
	}
	return raw
}

func fixtureEnvelope(t *testing.T, body []byte) originalDomainEnvelope {
	t.Helper()
	var v originalDomainEnvelope
	if json.Unmarshal(body, &v) != nil {
		t.Fatal("fixture envelope invalid")
	}
	return v
}
func strptr(s string) *string { return &s }

func fixtureSQLColumns() SQLColumns {
	columns := make(SQLColumns, len(sqlSourceColumns))
	for i, c := range sqlSourceColumns {
		columns[i] = []*string{strptr(c.name), strptr(c.kind), strptr(c.nullable), nil, strptr(""), nil}
		if strings.HasPrefix(c.kind, "varchar") || c.kind == "longtext" || c.kind == "text" {
			columns[i][5] = strptr("utf8mb4_unicode_ci")
		}
	}
	columns[0][4] = strptr("auto_increment")
	return columns
}
func fixtureSQLRow(t *testing.T, body []byte, id string) [][]byte {
	t.Helper()
	e := fixtureEnvelope(t, body)
	return [][]byte{[]byte(id), []byte(e.ID), []byte(e.EventType), []byte(e.AggregateType), []byte(e.AggregateID), []byte("639678084915671598"), []byte("qs-evaluation"), body, []byte("published"), []byte("0"), nil, []byte("2026-10-08 03:12:13.123"), nil, nil, nil, []byte("2026-10-08 03:12:13.123"), []byte("2026-10-08 03:12:14.123"), []byte("2026-10-08 03:12:14.123")}
}

// independentFrame is the v2 inventory framing specification, written in one
// buffer instead of the decoder helper; golden tests also assert fixed hashes.
func independentFrame(raw []byte, null bool) []byte {
	out := make([]byte, 9)
	if !null {
		out[0] = 1
		binary.BigEndian.PutUint64(out[1:], uint64(len(raw)))
		out = append(out, raw...)
	}
	return out
}
func fixtureSQLCopy(t *testing.T, rows [][][]byte, columns SQLColumns) ([]byte, SourceCopyExpectation) {
	t.Helper()
	if columns == nil {
		columns = fixtureSQLColumns()
	}
	b := SourceBoundary{Database: "mysql", Name: "domain_event_outbox", Kind: "base_table", Present: true, Empty: len(rows) == 0, PKType: "uint64", SchemaHash: strings.Repeat("a", 64), IdentityHash: strings.Repeat("b", 64)}
	if len(rows) > 0 {
		b.UpperToken = base64.StdEncoding.EncodeToString(rows[len(rows)-1][0])
	}
	var out bytes.Buffer
	if json.NewEncoder(&out).Encode(sqlSourceHeader{Protocol: SQLSourceProtocol, Columns: columns, Boundary: b}) != nil {
		t.Fatal("fixture header encoding failed")
	}
	columnJSON, err := json.Marshal(columns)
	if err != nil {
		t.Fatal("fixture columns encoding failed")
	}
	columnSHA := sha256.Sum256(columnJSON)
	hashInput := independentFrame([]byte(hex.EncodeToString(columnSHA[:])), false)
	total := uint64(0)
	for _, row := range rows {
		encoded := make([]*string, len(row))
		for i, raw := range row {
			hashInput = append(hashInput, independentFrame(raw, raw == nil)...)
			total += uint64(len(raw))
			if raw != nil {
				encoded[i] = strptr(base64.StdEncoding.EncodeToString(raw))
			}
		}
		if json.NewEncoder(&out).Encode(encoded) != nil {
			t.Fatal("fixture row encoding failed")
		}
	}
	sum := sha256.Sum256(hashInput)
	return out.Bytes(), SourceCopyExpectation{Boundary: b, DataHash: hex.EncodeToString(sum[:]), Records: uint64(len(rows)), Bytes: total}
}

func fixtureMongoRow(t *testing.T, body []byte, pk any) bson.D {
	t.Helper()
	e := fixtureEnvelope(t, body)
	return bson.D{{Key: "_id", Value: pk}, {Key: "event_id", Value: e.ID}, {Key: "event_type", Value: e.EventType}, {Key: "aggregate_type", Value: e.AggregateType}, {Key: "aggregate_id", Value: e.AggregateID}, {Key: "org_id", Value: fixtureLargeID}, {Key: "topic_name", Value: "qs-survey"}, {Key: "payload_json", Value: string(body)}, {Key: "status", Value: "published"}, {Key: "attempt_count", Value: 0}, {Key: "next_attempt_at", Value: fixtureClock}, {Key: "created_at", Value: fixtureClock}, {Key: "updated_at", Value: fixtureClock.Add(time.Second)}, {Key: "published_at", Value: fixtureClock.Add(time.Second)}}
}
func marshalBSON(t *testing.T, d bson.D) []byte {
	t.Helper()
	raw, err := bson.Marshal(d)
	if err != nil {
		t.Fatal("fixture BSON encoding failed")
	}
	return raw
}
func fixtureMongoCopy(t *testing.T, rows [][]byte) ([]byte, SourceCopyExpectation) {
	t.Helper()
	b := SourceBoundary{Database: "mongodb", Name: "domain_event_outbox", Kind: "collection", Present: true, Empty: len(rows) == 0, SchemaHash: strings.Repeat("a", 64), IdentityHash: strings.Repeat("b", 64)}
	if len(rows) > 0 {
		value := bson.Raw(rows[len(rows)-1]).Lookup("_id")
		token := marshalBSON(t, bson.D{{Key: "_id", Value: value}})
		b.UpperToken = base64.StdEncoding.EncodeToString(token)
		b.PKType = mongoPKType(value)
	}
	var out bytes.Buffer
	hashInput := []byte{}
	total := uint64(0)
	for _, row := range rows {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(row)))
		out.Write(length[:])
		out.Write(row)
		hashInput = append(hashInput, independentFrame(row, false)...)
		total += uint64(len(row))
	}
	sum := sha256.Sum256(hashInput)
	return out.Bytes(), SourceCopyExpectation{Boundary: b, DataHash: hex.EncodeToString(sum[:]), Records: uint64(len(rows)), Bytes: total}
}
func setMongoField(d bson.D, key string, value any) bson.D {
	out := append(bson.D(nil), d...)
	for i := range out {
		if out[i].Key == key {
			out[i].Value = value
			return out
		}
	}
	return append(out, bson.E{Key: key, Value: value})
}
func omitMongoField(d bson.D, key string) bson.D {
	out := bson.D{}
	for _, e := range d {
		if e.Key != key {
			out = append(out, e)
		}
	}
	return out
}

func TestSourceSixHistoricalWireTypes(t *testing.T) {
	for _, kind := range []string{"evaluation.requested", "evaluation.retry.requested", "evaluation.outcome.committed", "evaluation.failed", "answersheet.submitted", "interpretation.report.generated"} {
		t.Run(kind, func(t *testing.T) {
			body := wireFixture(t, kind)
			var v *DecodedSourceEvent
			if strings.HasPrefix(kind, "evaluation.") {
				raw, expected := fixtureSQLCopy(t, [][][]byte{fixtureSQLRow(t, body, "18446744073709551615")}, nil)
				reader, err := NewSQLSourceReader(bytes.NewReader(raw), expected)
				if err != nil {
					t.Fatal(err)
				}
				v, err = reader.Next()
				if err != nil {
					t.Fatal(err)
				}
				if reader.Receipt().Complete {
					t.Fatal("premature complete")
				}
				if _, err = reader.Next(); err != io.EOF || !reader.Receipt().Complete || reader.Receipt().DropReady || reader.Receipt().BusinessClosureVerified {
					t.Fatal("source-only completion invalid")
				}
			} else {
				pk := primitive.NewObjectIDFromTimestamp(fixtureClock)
				raw, expected := fixtureMongoCopy(t, [][]byte{marshalBSON(t, fixtureMongoRow(t, body, pk))})
				reader, err := NewMongoSourceReader(bytes.NewReader(raw), expected)
				if err != nil {
					t.Fatal(err)
				}
				v, err = reader.Next()
				if err != nil {
					t.Fatal(err)
				}
				if _, err = reader.Next(); err != io.EOF || !reader.Receipt().Complete || reader.Receipt().DropReady || reader.Receipt().BusinessClosureVerified {
					t.Fatal("source-only completion invalid")
				}
			}
			if v.EventID != "original-event-1" || v.EventType != kind || v.OrgID != uint64(fixtureLargeID) || v.BusinessIDs["testee_id"] != "18446744073709551615" || !v.BusinessAt.Equal(fixtureClock) || !v.OccurredAt.Equal(fixtureClock.Add(time.Second)) {
				t.Fatal("original typed facts changed")
			}
			if v.ContentDigest.Kind == evidence.SDKFingerprintKind || v.Source.Digest.Kind == evidence.SDKFingerprintKind || v.ContentDigest == v.Source.Digest {
				t.Fatal("distinct digests collapsed")
			}
			if _, err := json.Marshal(v); !errors.Is(err, ErrSourceSerialization) {
				t.Fatal("private payload could serialize")
			}
			if v.Requested != nil || v.Failed != nil {
				if v.OriginalRun.RunID != "" || v.OriginalRun.Attempt != nil || !reflect.DeepEqual(v.OriginalRun.Missing, []string{"original_run_id", "original_attempt"}) {
					t.Fatal("run invented from expected/current facts")
				}
			}
			if v.Generated != nil && (v.OriginalRun.RunID != "run-1" || v.OriginalRun.Attempt == nil || *v.OriginalRun.Attempt != 2) {
				t.Fatal("original run facts lost")
			}
		})
	}
}

func TestSourceJSONStrictSixTypeSchemas(t *testing.T) {
	base := wireFixture(t, "evaluation.requested")
	outer := outerEventFacts{engine: "mysql", id: "original-event-1", eventType: "evaluation.requested", aggregateType: "Evaluation", aggregateID: "639678084915671598", org: func() *int64 { n := fixtureLargeID; return &n }()}
	for _, tc := range []struct {
		name string
		raw  []byte
		want error
	}{
		{"duplicate_envelope", bytes.Replace(base, []byte(`"id":"original-event-1"`), []byte(`"id":"original-event-1","id":"original-event-1"`), 1), ErrSourceDuplicateKey},
		{"duplicate_nested", bytes.Replace(base, []byte(`"org_id":639678084915671598`), []byte(`"org_id":639678084915671598,"org_id":639678084915671598`), 1), ErrSourceDuplicateKey},
		{"escaped_duplicate", bytes.Replace(base, []byte(`"org_id":639678084915671598`), []byte(`"org_id":639678084915671598,"org\u005fid":639678084915671598`), 1), ErrSourceDuplicateKey},
		{"unknown_body", bytes.Replace(base, []byte(`"org_id":`), []byte(`"old_body":{},"org_id":`), 1), ErrSourceUnknownField},
		{"field_case_alias", bytes.Replace(base, []byte(`"org_id"`), []byte(`"Org_ID"`), 1), ErrSourceSchema},
		{"numeric_id_as_float", bytes.Replace(base, []byte(`"assessment_id":639678084915671598`), []byte(`"assessment_id":639678084915671598.0`), 1), ErrSourcePrecision},
		{"numeric_id_as_string", bytes.Replace(base, []byte(`"assessment_id":639678084915671598`), []byte(`"assessment_id":"639678084915671598"`), 1), ErrSourceSchema},
		{"overflow_uint64", bytes.Replace(base, []byte(`18446744073709551615`), []byte(`18446744073709551616`), 1), ErrSourcePrecision},
		{"null_required", bytes.Replace(base, []byte(`"questionnaire_code":"q-code"`), []byte(`"questionnaire_code":null`), 1), ErrSourceSchema},
		{"invalid_utf8", bytes.Replace(base, []byte(`q-code`), []byte{'q', 0xff}, 1), ErrSourceJSON},
		{"unpaired_surrogate", bytes.Replace(base, []byte(`q-code`), []byte(`q-\ud800`), 1), ErrSourceJSON},
		{"trailing_document", append(append([]byte{}, base...), []byte(`{}`)...), ErrSourceJSON},
		{"old_envelope_schema", []byte(`{"uuid":"original-event-1","payload":"e30=","schema_revision":2}`), ErrSourceSchema},
		{"identity_conflict", bytes.Replace(base, []byte(`"id":"original-event-1"`), []byte(`"id":"another-event"`), 1), ErrSourceIdentity},
		{"payload_aggregate_conflict", bytes.Replace(base, []byte(`"assessment_id":639678084915671598`), []byte(`"assessment_id":639678084915671599`), 1), ErrSourceIdentity},
		{"organization_conflict", bytes.Replace(base, []byte(`"org_id":639678084915671598`), []byte(`"org_id":639678084915671599`), 1), ErrSourceOrganization},
		{"invalid_clock", bytes.Replace(base, []byte(`"occurredAt":"2026-10-08T11:12:14.123456789+08:00"`), []byte(`"occurredAt":"not-a-clock"`), 1), ErrSourceSchema},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := decodeDomain(tc.raw, outer); err != tc.want {
				t.Fatalf("category got %v want %v", err, tc.want)
			}
		})
	}
	for _, kind := range []string{"answersheet.submitted", "interpretation.report.generated"} {
		t.Run(kind+"_unknown_nested", func(t *testing.T) {
			raw := wireFixture(t, kind)
			field := []byte(`"purpose":`)
			if kind == "interpretation.report.generated" {
				field = []byte(`"kind":"scale"`)
			}
			mutated := bytes.Replace(raw, field, append([]byte(`"legacy_flag":true,`), field...), 1)
			env := fixtureEnvelope(t, raw)
			if _, err := decodeDomain(mutated, outerEventFacts{engine: "mongodb", id: env.ID, eventType: kind, aggregateType: env.AggregateType, aggregateID: env.AggregateID}); err != ErrSourceUnknownField {
				t.Fatal("unknown nested payload accepted")
			}
		})
	}
}

func TestSourceBusinessTimeMillisecondAndOriginalClock(t *testing.T) {
	if !BusinessTimeEqual(fixtureClock, fixtureClock.UTC().Truncate(time.Millisecond)) || BusinessTimeEqual(fixtureClock, fixtureClock.Add(time.Millisecond)) || BusinessTimeEqual(time.Time{}, time.Time{}) {
		t.Fatal("persisted-clock comparison invalid")
	}
	// Event OccurredAt is independently created by historical event.New; it
	// need not equal FailedAt/GeneratedAt or the outbox transport CreatedAt.
	for _, kind := range []string{"evaluation.failed", "interpretation.report.generated"} {
		env := fixtureEnvelope(t, wireFixture(t, kind))
		engine := "mysql"
		if strings.HasPrefix(kind, "interpretation") {
			engine = "mongodb"
		}
		v, err := decodeDomain(wireFixture(t, kind), outerEventFacts{engine: engine, id: env.ID, eventType: kind, aggregateType: env.AggregateType, aggregateID: env.AggregateID})
		if err != nil || BusinessTimeEqual(v.BusinessAt, v.OccurredAt) {
			t.Fatal("distinct legitimate original clocks conflated")
		}
	}
}

func TestSourceReportScorePrecisionAndUnsupportedOldShape(t *testing.T) {
	base := wireFixture(t, "interpretation.report.generated")
	env := fixtureEnvelope(t, base)
	outer := outerEventFacts{engine: "mongodb", id: env.ID, eventType: env.EventType, aggregateType: env.AggregateType, aggregateID: env.AggregateID}
	for _, tc := range []struct {
		name string
		raw  []byte
		want error
	}{
		{"rounded_large_score", bytes.Replace(base, []byte(`"value":5.1`), []byte(`"value":9007199254740993.0`), 1), ErrSourcePrecision},
		{"over_precise_decimal", bytes.Replace(base, []byte(`"value":5.1`), []byte(`"value":5.10000000000000001`), 1), ErrSourcePrecision},
		{"missing_original_run", bytes.Replace(base, []byte(`"run_id":"run-1",`), nil, 1), ErrSourceSchema},
		{"old_report_payload", []byte(`{"id":"original-event-1","eventType":"interpretation.report.generated","occurredAt":"2026-10-08T03:12:14Z","aggregateType":"ReportGeneration","aggregateID":"generation-1","data":{"org_id":1,"report_id":"report-1"}}`), ErrSourceSchema},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := decodeDomain(tc.raw, outer); err != tc.want {
				t.Fatalf("category got %v want %v", err, tc.want)
			}
		})
	}
}
