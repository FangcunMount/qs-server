package retirement

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func readOneSQL(t *testing.T, row [][]byte) *DecodedSourceEvent {
	t.Helper()
	raw, expected := fixtureSQLCopy(t, [][][]byte{row}, nil)
	reader, err := NewSQLSourceReader(bytes.NewReader(raw), expected)
	if err != nil {
		t.Fatal(err)
	}
	v, err := reader.Next()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Next(); err != io.EOF || !reader.Receipt().Complete {
		t.Fatal("copy incomplete")
	}
	return v
}
func readOneMongo(t *testing.T, row bson.D) *DecodedSourceEvent {
	t.Helper()
	raw, expected := fixtureMongoCopy(t, [][]byte{marshalBSON(t, row)})
	reader, err := NewMongoSourceReader(bytes.NewReader(raw), expected)
	if err != nil {
		t.Fatal(err)
	}
	v, err := reader.Next()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Next(); err != io.EOF || !reader.Receipt().Complete {
		t.Fatal("copy incomplete")
	}
	return v
}

func TestSourceTransportMutationDistinctDigestsAndNull(t *testing.T) {
	row := fixtureSQLRow(t, wireFixture(t, "evaluation.failed"), "1")
	first := readOneSQL(t, row)
	changed := append([][]byte(nil), row...)
	changed[8] = []byte("failed")
	changed[9] = []byte("17")
	changed[11] = []byte("2026-10-09 01:02:03.000")
	changed[12] = []byte("delivery failed")
	changed[16] = []byte("2026-10-09 01:02:03.000")
	changed[17] = nil
	second := readOneSQL(t, changed)
	if first.ContentDigest != second.ContentDigest || first.Source.Digest == second.Source.Digest || first.Transport.AttemptCount == second.Transport.AttemptCount {
		t.Fatal("transport mixed into immutable event content")
	}
	if err := ValidateSourceRowDigest(first, second.Source.Digest); err != ErrSourceDigest {
		t.Fatal("wrong source digest accepted")
	}
	empty := append([][]byte(nil), row...)
	empty[12] = []byte{}
	third := readOneSQL(t, empty)
	if first.Source.Digest == third.Source.Digest || first.ContentDigest != third.ContentDigest || first.Transport.Fields["last_error"] != nil || third.Transport.Fields["last_error"] == nil || *third.Transport.Fields["last_error"] != "" {
		t.Fatal("NULL and empty collapsed")
	}
	var tracker SourceIdentityTracker
	if tracker.Observe(first) != nil {
		t.Fatal("initial source identity rejected")
	}
	if tracker.Observe(first) != nil {
		t.Fatal("identical source observation not idempotent")
	}
	if tracker.Observe(second) != ErrSourceDigest {
		t.Fatal("same original event with different source bytes not rejected")
	}
	mongo := fixtureMongoRow(t, wireFixture(t, "answersheet.submitted"), int64(639678084915671598))
	mfirst := readOneMongo(t, mongo)
	msecond := readOneMongo(t, setMongoField(setMongoField(mongo, "attempt_count", int64(18)), "status", "failed"))
	if mfirst.ContentDigest != msecond.ContentDigest || mfirst.Source.Digest == msecond.Source.Digest {
		t.Fatal("Mongo transport mixed into event content")
	}
	absent := readOneMongo(t, omitMongoField(mongo, "org_id"))
	if absent.OuterOrgID != nil || !containsString(absent.ResolverGaps, "outer_organization_absent") {
		t.Fatal("absent org invented")
	}
}

func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func TestSourceSQLProtocolSchemaAndBounds(t *testing.T) {
	body := wireFixture(t, "evaluation.requested")
	row := fixtureSQLRow(t, body, "1")
	t.Run("prior_column_schema", func(t *testing.T) {
		columns := fixtureSQLColumns()
		columns = append(columns[:5], columns[6:]...)
		raw, expected := fixtureSQLCopy(t, [][][]byte{row}, columns)
		if _, err := NewSQLSourceReader(bytes.NewReader(raw), expected); err != ErrSourceSchema {
			t.Fatal("unsupported earlier SQL schema projected")
		}
	})
	t.Run("extra_column", func(t *testing.T) {
		columns := fixtureSQLColumns()
		columns = append(columns, []*string{strptr("future_body"), strptr("longtext"), strptr("YES"), nil, strptr(""), nil})
		raw, expected := fixtureSQLCopy(t, [][][]byte{row}, columns)
		if _, err := NewSQLSourceReader(bytes.NewReader(raw), expected); err != ErrSourceSchema {
			t.Fatal("extra schema accepted")
		}
	})
	t.Run("nullable_metadata_changed", func(t *testing.T) {
		columns := fixtureSQLColumns()
		columns[7][2] = strptr("YES")
		raw, expected := fixtureSQLCopy(t, [][][]byte{row}, columns)
		if _, err := NewSQLSourceReader(bytes.NewReader(raw), expected); err != ErrSourceSchema {
			t.Fatal("nullable payload schema accepted")
		}
	})
	t.Run("independent_boundary_mismatch", func(t *testing.T) {
		raw, expected := fixtureSQLCopy(t, [][][]byte{row}, nil)
		expected.Boundary.IdentityHash = strings.Repeat("c", 64)
		if _, err := NewSQLSourceReader(bytes.NewReader(raw), expected); err != ErrSourceProtocol {
			t.Fatal("observed header auto-approved")
		}
	})
	t.Run("boundary_missing_field", func(t *testing.T) {
		raw, expected := fixtureSQLCopy(t, [][][]byte{row}, nil)
		raw = bytes.Replace(raw, []byte(`"empty":false,`), nil, 1)
		if _, err := NewSQLSourceReader(bytes.NewReader(raw), expected); err != ErrSourceProtocol {
			t.Fatal("missing header boundary field accepted")
		}
	})
	t.Run("exact_header_column_hash", func(t *testing.T) {
		raw, expected := fixtureSQLCopy(t, [][][]byte{row}, nil)
		raw = bytes.Replace(raw, []byte(`utf8mb4_unicode_ci`), []byte(`utf8mb4_0900_ai_ci`), 1)
		reader, err := NewSQLSourceReader(bytes.NewReader(raw), expected)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = reader.Next(); err != nil {
			t.Fatal(err)
		}
		if _, err = reader.Next(); err != ErrSourceDigest || reader.Receipt().Complete {
			t.Fatal("altered metadata hash accepted at EOF")
		}
	})
	for _, tc := range []struct {
		name   string
		mutate func([][]byte)
		want   error
	}{
		{"null_required", func(r [][]byte) { r[7] = nil }, ErrSourceSchema},
		{"empty_org", func(r [][]byte) { r[5] = []byte{} }, ErrSourceSchema},
		{"outer_org_conflict", func(r [][]byte) { r[5] = []byte("1") }, ErrSourceOrganization},
		{"aggregate_conflict", func(r [][]byte) { r[4] = []byte("2") }, ErrSourceIdentity},
		{"outer_eventid_conflict", func(r [][]byte) { r[1] = []byte("another-event") }, ErrSourceIdentity},
		{"invalid_transport_clock", func(r [][]byte) { r[15] = []byte("2026-02-30 01:02:03.000") }, ErrSourceSchema},
		{"transport_submillisecond", func(r [][]byte) { r[15] = []byte("2026-10-08 01:02:03.1234") }, ErrSourceSchema},
		{"unsigned_attempt_overflow", func(r [][]byte) { r[9] = []byte("4294967296") }, ErrSourceSchema},
		{"negative_attempt", func(r [][]byte) { r[9] = []byte("-1") }, ErrSourceSchema},
		{"unknown_status", func(r [][]byte) { r[8] = []byte("acknowledged") }, ErrSourceSchema},
		{"unknown_event_type", func(r [][]byte) {
			r[2] = []byte("footprint.entry_opened")
			r[7] = bytes.Replace(r[7], []byte(`evaluation.requested`), r[2], 1)
		}, ErrSourceEventType},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copyRow := append([][]byte(nil), row...)
			tc.mutate(copyRow)
			raw, expected := fixtureSQLCopy(t, [][][]byte{copyRow}, nil)
			reader, err := NewSQLSourceReader(bytes.NewReader(raw), expected)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = reader.Next(); err != tc.want || reader.Receipt().Complete {
				t.Fatalf("category got %v want %v", err, tc.want)
			}
		})
	}
	t.Run("noncanonical_base64", func(t *testing.T) {
		raw, expected := fixtureSQLCopy(t, [][][]byte{row}, nil)
		headerEnd := bytes.IndexByte(raw, '\n') + 1
		raw = append(append([]byte(nil), raw[:headerEnd]...), bytes.Replace(raw[headerEnd:], []byte(`"MQ=="`), []byte(`"MR=="`), 1)...)
		reader, err := NewSQLSourceReader(bytes.NewReader(raw), expected)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = reader.Next(); err != ErrSourceProtocol {
			t.Fatal("bad base64 padding accepted")
		}
	})
	t.Run("duplicate_event_different_pk", func(t *testing.T) {
		other := fixtureSQLRow(t, body, "2")
		raw, expected := fixtureSQLCopy(t, [][][]byte{row, other}, nil)
		reader, err := NewSQLSourceReader(bytes.NewReader(raw), expected)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = reader.Next(); err != nil {
			t.Fatal(err)
		}
		if _, err = reader.Next(); err != ErrSourceDigest || reader.Receipt().Complete {
			t.Fatal("duplicate original event source accepted")
		}
	})
	t.Run("primary_key_order", func(t *testing.T) {
		raw, expected := fixtureSQLCopy(t, [][][]byte{row, row}, nil)
		reader, err := NewSQLSourceReader(bytes.NewReader(raw), expected)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = reader.Next(); err != nil {
			t.Fatal(err)
		}
		if _, err = reader.Next(); err != ErrSourceOrder {
			t.Fatal("duplicate ordered PK accepted")
		}
	})
	t.Run("digest_not_verified_until_eof", func(t *testing.T) {
		raw, expected := fixtureSQLCopy(t, [][][]byte{row}, nil)
		expected.DataHash = strings.Repeat("c", 64)
		reader, err := NewSQLSourceReader(bytes.NewReader(raw), expected)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = reader.Next(); err != nil {
			t.Fatal(err)
		}
		if reader.Receipt().Complete {
			t.Fatal("premature proof")
		}
		if _, err = reader.Next(); err != ErrSourceDigest {
			t.Fatal("wrong approved digest accepted")
		}
	})
	t.Run("extra_bytes_not_synthetic_eof", func(t *testing.T) {
		raw, expected := fixtureSQLCopy(t, [][][]byte{row}, nil)
		raw = append(raw, []byte("garbage")...)
		reader, err := NewSQLSourceReader(bytes.NewReader(raw), expected)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = reader.Next(); err != nil {
			t.Fatal(err)
		}
		if _, err = reader.Next(); err != ErrSourceJSON || reader.Receipt().Complete {
			t.Fatal("trailing data hidden")
		}
	})
	t.Run("empty_copy", func(t *testing.T) {
		raw, expected := fixtureSQLCopy(t, nil, nil)
		reader, err := NewSQLSourceReader(bytes.NewReader(raw), expected)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = reader.Next(); err != io.EOF || !reader.Receipt().Complete || reader.Receipt().DropReady {
			t.Fatal("empty source completion invalid")
		}
	})
}

func TestSourceMongoExactPKAndFrame(t *testing.T) {
	body := wireFixture(t, "answersheet.submitted")
	oid, err := primitive.ObjectIDFromHex("507f1f77bcf86cd799439011")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		pk   any
		kind string
	}{{"objectid", oid, "mongodb_objectid"}, {"string", "639678084915671598", "mongodb_raw_bson_v1"}, {"int64", int64(639678084915671598), "mongodb_raw_bson_v1"}, {"int32", int32(123), "mongodb_raw_bson_v1"}} {
		t.Run(tc.name, func(t *testing.T) {
			row := fixtureMongoRow(t, body, tc.pk)
			v := readOneMongo(t, row)
			token := marshalBSON(t, bson.D{{Key: "_id", Value: tc.pk}})
			if v.PrimaryKeyToken != base64.StdEncoding.EncodeToString(token) || v.Source.PrimaryKeySHA256 != sourceSHA(token) || v.Source.PrimaryKeyKind != tc.kind {
				t.Fatal("original BSON PK bytes/type changed")
			}
		})
	}
	row := fixtureMongoRow(t, body, int64(639678084915671598))
	for _, tc := range []struct {
		name string
		row  bson.D
		want error
	}{
		{"duplicate_top_key", append(append(bson.D(nil), row...), bson.E{Key: "event_id", Value: "original-event-1"}), ErrSourceBSON},
		{"unknown_top_field", append(append(bson.D(nil), row...), bson.E{Key: "future_body", Value: bson.M{}}), ErrSourceUnknownField},
		{"org_wrong_bson_type", setMongoField(row, "org_id", float64(fixtureLargeID)), ErrSourceSchema},
		{"org_null", setMongoField(row, "org_id", nil), ErrSourceSchema},
		{"org_conflict", setMongoField(row, "org_id", int64(1)), ErrSourceOrganization},
		{"aggregate_conflict", setMongoField(row, "aggregate_id", "another-sheet"), ErrSourceIdentity},
		{"datetime_wrong_type", setMongoField(row, "created_at", "2026-10-08T03:12:13Z"), ErrSourceSchema},
		{"attempt_precision", setMongoField(row, "attempt_count", 0.0), ErrSourceSchema},
		{"missing_required", omitMongoField(row, "payload_json"), ErrSourceSchema},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, expected := fixtureMongoCopy(t, [][]byte{marshalBSON(t, tc.row)})
			reader, err := NewMongoSourceReader(bytes.NewReader(raw), expected)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = reader.Next(); err != tc.want || reader.Receipt().Complete {
				t.Fatalf("category got %v want %v", err, tc.want)
			}
		})
	}
	t.Run("truncated_frame", func(t *testing.T) {
		raw, expected := fixtureMongoCopy(t, [][]byte{marshalBSON(t, row)})
		reader, err := NewMongoSourceReader(bytes.NewReader(raw[:len(raw)-1]), expected)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = reader.Next(); err != ErrSourceIncomplete {
			t.Fatal("truncated source accepted")
		}
	})
	t.Run("trailing_partial_frame", func(t *testing.T) {
		raw, expected := fixtureMongoCopy(t, [][]byte{marshalBSON(t, row)})
		raw = append(raw, 0)
		reader, err := NewMongoSourceReader(bytes.NewReader(raw), expected)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = reader.Next(); err != nil {
			t.Fatal(err)
		}
		if _, err = reader.Next(); err != ErrSourceIncomplete || reader.Receipt().Complete {
			t.Fatal("trailing partial frame hidden")
		}
	})
	t.Run("mixed_bson_pk_types", func(t *testing.T) {
		first := setMongoField(row, "_id", int32(1))
		last := setMongoField(row, "_id", int64(2))
		raw, expected := fixtureMongoCopy(t, [][]byte{marshalBSON(t, first), marshalBSON(t, last)})
		reader, err := NewMongoSourceReader(bytes.NewReader(raw), expected)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = reader.Next(); err != ErrSourceSchema {
			t.Fatal("BSON PK type bracket violated")
		}
	})
	t.Run("duplicate_original_event", func(t *testing.T) {
		raw, expected := fixtureMongoCopy(t, [][]byte{marshalBSON(t, setMongoField(row, "_id", int64(1))), marshalBSON(t, setMongoField(row, "_id", int64(2)))})
		reader, err := NewMongoSourceReader(bytes.NewReader(raw), expected)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = reader.Next(); err != nil {
			t.Fatal(err)
		}
		if _, err = reader.Next(); err != ErrSourceDigest {
			t.Fatal("different original source bytes not blocked")
		}
	})
	t.Run("wrong_digest", func(t *testing.T) {
		raw, expected := fixtureMongoCopy(t, [][]byte{marshalBSON(t, row)})
		expected.DataHash = strings.Repeat("c", 64)
		reader, err := NewMongoSourceReader(bytes.NewReader(raw), expected)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = reader.Next(); err != nil {
			t.Fatal(err)
		}
		if _, err = reader.Next(); err != ErrSourceDigest || reader.Receipt().Complete {
			t.Fatal("wrong full copy digest accepted")
		}
	})
	t.Run("empty_copy", func(t *testing.T) {
		raw, expected := fixtureMongoCopy(t, nil)
		reader, err := NewMongoSourceReader(bytes.NewReader(raw), expected)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = reader.Next(); err != io.EOF || !reader.Receipt().Complete || reader.Receipt().DropReady {
			t.Fatal("empty source completion invalid")
		}
	})
	t.Run("unsupported_pk_type", func(t *testing.T) {
		raw, expected := fixtureMongoCopy(t, [][]byte{marshalBSON(t, setMongoField(row, "_id", 1.0))})
		if _, err := NewMongoSourceReader(bytes.NewReader(raw), expected); err != ErrSourceProtocol {
			t.Fatal("double PK accepted")
		}
	})
}

func TestSourceKnownFramingAndProofContract(t *testing.T) {
	h := sha256.New()
	sourceFrame(h, nil, true)
	sourceFrame(h, []byte{}, false)
	sourceFrame(h, []byte("hello"), false)
	want := sha256.Sum256(append(append(independentFrame(nil, true), independentFrame([]byte{}, false)...), independentFrame([]byte("hello"), false)...))
	// Independent Python hashlib/struct golden for NULL, empty, and hello.
	const framingGolden = "aeab540a2be5d4873ea81f4c62c6f067d70d6b0c7d13baf9349551ec5672d2be"
	if hex.EncodeToString(h.Sum(nil)) != framingGolden || hex.EncodeToString(want[:]) != framingGolden {
		t.Fatal("inventory v2 frame bytes changed")
	}
	v := readOneSQL(t, fixtureSQLRow(t, wireFixture(t, "evaluation.outcome.committed"), "1"))
	entry := evidence.HistoricalReferenceEntryV1{EventID: v.EventID, EventType: v.EventType, Source: v.Source}
	if entry.Validate() == nil {
		t.Fatal("decoding alone manufactured retirement proof")
	}
	if _, err := json.Marshal(v); err == nil {
		t.Fatal("private event body accidentally persisted")
	}
	if _, err := bson.Marshal(v); err != ErrSourceSerialization {
		t.Fatal("private event body could BSON serialize")
	}
	if strings.Contains(fmt.Sprintf("%#v", v), "original-event") || strings.Contains(fmt.Sprintf("%+v", v), "historical reason") {
		t.Fatal("private source leaked in diagnostic formatting")
	}
	if reflect.DeepEqual(v.ContentDigest, v.Source.Digest) || strings.Contains(v.String(), "original-event") {
		t.Fatal("unsafe source summary")
	}
}

func TestSourceOriginalDomainBytesAreNotReserialized(t *testing.T) {
	body := wireFixture(t, "evaluation.failed")
	var pretty bytes.Buffer
	if json.Indent(&pretty, body, "", "  ") != nil {
		t.Fatal("fixture format failed")
	}
	first := readOneSQL(t, fixtureSQLRow(t, body, "1"))
	second := readOneSQL(t, fixtureSQLRow(t, pretty.Bytes(), "1"))
	if !reflect.DeepEqual(first.Failed, second.Failed) || first.ContentDigest == second.ContentDigest || first.Source.Digest == second.Source.Digest {
		t.Fatal("original bytes replaced by normalized serialization")
	}
	sum := sha256.Sum256(pretty.Bytes())
	if second.ContentDigest.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatal("content digest not original bytes")
	}
}
