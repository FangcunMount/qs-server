package retirement

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	aiFixtureRequestID  = "10000000-0000-4000-8000-000000000001"
	aiFixtureCommandID  = "20000000-0000-4000-8000-000000000001"
	aiFixtureSessionID  = "30000000-0000-4000-8000-000000000001"
	aiFixtureQuestionID = "40000000-0000-4000-8000-000000000001"
)

// The fixture invokes the actual historical encode inputs (app.Start/Change),
// matching store.go and the retained handoff. Identities/text are synthetic;
// no production bodies or imported completeness/terminal flags are accepted.
func aiFixturePayload(t *testing.T, kind string) ([]byte, string) {
	t.Helper()
	var value any
	if kind == "start" {
		value = app.Start{RequestID: aiFixtureRequestID, Actor: app.Actor{OrgID: "639678084915671598", SubjectID: "fixture-subject"}, TesteeID: "18446744073709551615", AssessmentIDs: []string{"639678084915671599"}, Goal: "private-fixture-goal <&> 中文", Evidence: []app.EvidenceItem{{AssessmentID: "639678084915671599", TesteeID: "18446744073709551615", ReportID: "fixture-report", SourceVersion: "v1", Facts: []app.Fact{{Ref: "score", Value: "private-fixture-fact"}}}}}
	} else {
		value = app.Change{CommandID: aiFixtureCommandID, SessionID: aiFixtureSessionID, Actor: app.Actor{OrgID: "639678084915671598", SubjectID: "fixture-subject"}, Action: kind, ExpectedVersion: 9007199254740993}
		if kind == "answer" {
			answer := "private-fixture-answer <&> 中文"
			change := value.(app.Change)
			change.QuestionID, change.Answer = aiFixtureQuestionID, &answer
			value = change
		}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal("fixture marshal")
	}
	return raw, sourceSHA(raw)
}

func aiFixtureColumns(table string) SQLColumns {
	// Independent exact DDL ordinal/type fixtures, rather than copying the
	// decoder's supported schema declaration (which could hide a typo there).
	wants := []aiSQLColumn{
		{"command_id", "char(36)", "ascii_bin"}, {"request_id", "char(36)", "ascii_bin"},
		{"kind", "varchar(16)", "utf8mb4"}, {"payload", "json", ""},
		{"payload_hash", "char(64)", "utf8mb4"}, {"delivered", "tinyint(1)", ""},
		{"attempts", "int", ""}, {"available_at", "datetime(6)", ""},
	}
	if table == AILegacyCommandSource {
		wants = []aiSQLColumn{
			{"command_id", "char(36)", "ascii_bin"}, {"request_id", "char(36)", "ascii_bin"},
			{"source_kind", "varchar(16)", "ascii_bin"}, {"source_payload", "mediumblob", ""},
			{"source_payload_hash", "char(64)", "ascii_bin"}, {"source_attempts", "int", ""},
			{"source_available_at", "datetime(6)", ""}, {"source_original_time", "varchar(64)", "ascii_bin"},
			{"messaging_body_sha256", "char(64)", "ascii_bin"}, {"transferred_at", "datetime(6)", ""},
		}
	}
	columns := make(SQLColumns, len(wants))
	for i, want := range wants {
		columns[i] = []*string{strptr(want.name), strptr(want.kind), strptr("NO"), nil, strptr(""), nil}
		if want.collation != "" {
			collation := want.collation
			if collation == "utf8mb4" {
				collation = "utf8mb4_0900_ai_ci"
			}
			columns[i][5] = strptr(collation)
		}
	}
	if table == AIBridgeCommandSource {
		columns[5][3], columns[6][3] = strptr("0"), strptr("0")
		columns[7][3], columns[7][4] = strptr("CURRENT_TIMESTAMP(6)"), strptr("DEFAULT_GENERATED")
	}
	return columns
}

func aiFixtureRow(t *testing.T, table, kind string) [][]byte {
	t.Helper()
	payload, writerHash := aiFixturePayload(t, kind)
	id := aiFixtureCommandID
	if kind == "start" {
		id = aiFixtureRequestID
	}
	row := [][]byte{[]byte(id), []byte(aiFixtureRequestID), []byte(kind), payload, []byte(writerHash)}
	if table == AIBridgeCommandSource {
		return append(row, []byte("0"), []byte("3"), []byte("2026-10-08 11:12:13.123456"))
	}
	original := ""
	if kind == "start" {
		original = "2026-10-08T11:12:12.123456+08:00"
	}
	return append(row, []byte("3"), []byte("2026-10-08 11:12:13.123456"), []byte(original), []byte(strings.Repeat("c", 64)), []byte("2026-10-08 11:12:14.123456"))
}

func aiFixtureCopy(t *testing.T, table string, rows [][][]byte, columns SQLColumns) ([]byte, SourceCopyExpectation) {
	t.Helper()
	if columns == nil {
		columns = aiFixtureColumns(table)
	}
	b := SourceBoundary{Database: "mysql", Name: table, Kind: "base_table", Present: true, Empty: len(rows) == 0, PKType: "ascii_string", SchemaHash: strings.Repeat("a", 64), IdentityHash: strings.Repeat("b", 64)}
	if len(rows) != 0 {
		b.UpperToken = base64.StdEncoding.EncodeToString(rows[len(rows)-1][0])
	}
	var file bytes.Buffer
	if json.NewEncoder(&file).Encode(sqlSourceHeader{Protocol: SQLSourceProtocol, Columns: columns, Boundary: b}) != nil {
		t.Fatal("fixture header")
	}
	metadata, err := json.Marshal(columns)
	if err != nil {
		t.Fatal("fixture metadata")
	}
	metadataHash := sha256.Sum256(metadata)
	hashBytes := independentFrame([]byte(hex.EncodeToString(metadataHash[:])), false)
	var size uint64
	for _, row := range rows {
		encoded := make([]*string, len(row))
		for i, raw := range row {
			hashBytes = append(hashBytes, independentFrame(raw, raw == nil)...)
			size += uint64(len(raw))
			if raw != nil {
				encoded[i] = strptr(base64.StdEncoding.EncodeToString(raw))
			}
		}
		if json.NewEncoder(&file).Encode(encoded) != nil {
			t.Fatal("fixture row")
		}
	}
	sum := sha256.Sum256(hashBytes)
	return file.Bytes(), SourceCopyExpectation{Boundary: b, Records: uint64(len(rows)), Bytes: size, DataHash: hex.EncodeToString(sum[:])}
}

func aiReadFixture(t *testing.T, table string, row [][]byte) *DecodedAICommand {
	t.Helper()
	raw, expected := aiFixtureCopy(t, table, [][][]byte{row}, nil)
	r, err := NewAISQLSourceReader(bytes.NewReader(raw), expected)
	if err != nil {
		t.Fatal("fixture reader", err)
	}
	v, err := r.Next()
	if err != nil {
		t.Fatal("fixture decode", err)
	}
	if r.Receipt().Complete {
		t.Fatal("premature complete")
	}
	if _, err = r.Next(); err != io.EOF {
		t.Fatal("fixture complete", err)
	}
	if receipt := r.Receipt(); !receipt.Complete || receipt.BusinessClosureVerified || receipt.DropReady || receipt.DataHash != expected.DataHash || receipt.Records != 1 || receipt.Bytes != expected.Bytes {
		t.Fatal("invalid source receipt")
	}
	return v
}

func TestAISourceKnownWriterKindsAndBudgets(t *testing.T) {
	for _, table := range []string{AIBridgeCommandSource, AILegacyCommandSource} {
		for _, kind := range []string{"start", "answer", "cancel"} {
			t.Run(table+"/"+kind, func(t *testing.T) {
				row := aiFixtureRow(t, table, kind)
				v := aiReadFixture(t, table, row)
				if v.Source.Digest.Kind != SQLRowDigestKind || v.Source.PrimaryKeyKind != "mysql_ascii_uuid" || v.RequestID != aiFixtureRequestID || v.OrganizationID != "639678084915671598" || v.Transport.SourceAttempts != 3 || v.Transport.HandoffBudgetFloor != 3 || v.Transport.HandoffBudgetExhausted {
					t.Fatal("source facts")
				}
				if kind == "start" && (v.CommandID != v.RequestID || v.ResourceID != v.RequestID || v.MessagingKind != "START" || v.Business.TesteeID != "18446744073709551615") {
					t.Fatal("start identities")
				}
				if kind != "start" && (v.ResourceID != aiFixtureSessionID || v.MessagingKind != "CHANGE" || v.Business.ExpectedVersion != 9007199254740993 || v.Transport.OriginalOccurredAt != "") {
					t.Fatal("change identities/clocks/precision")
				}
				if table == AIBridgeCommandSource && (v.Transport.Delivered == nil || *v.Transport.Delivered || v.PayloadBytesDigest.Kind != AIBridgePayloadBytesKind) {
					t.Fatal("bridge physical facts")
				}
				if table == AILegacyCommandSource && (v.Transport.Delivered != nil || v.PayloadBytesDigest.Kind != AILegacyPayloadBytesKind || v.Transport.MessagingBodySHA256 != strings.Repeat("c", 64)) {
					t.Fatal("handoff physical facts")
				}
				if len(v.ResolverGaps) < 2 {
					t.Fatal("closure silently proven")
				}
			})
		}
	}
	for _, attempts := range []string{"0", "8", "2147483647"} {
		row := aiFixtureRow(t, AILegacyCommandSource, "start")
		row[5] = []byte(attempts)
		v := aiReadFixture(t, AILegacyCommandSource, row)
		if v.Transport.HandoffBudgetExhausted != (attempts != "0") || v.Transport.HandoffBudgetFloor != map[string]uint32{"0": 0, "8": 8, "2147483647": 8}[attempts] {
			t.Fatal("handoff reset/fabricated failure")
		}
	}
	row := aiFixtureRow(t, AIBridgeCommandSource, "start")
	row[5] = []byte("1")
	v := aiReadFixture(t, AIBridgeCommandSource, row)
	if !*v.Transport.Delivered || len(v.ResolverGaps) == 0 {
		t.Fatal("delivered treated as terminal")
	}
}

func TestAISourceWriterContractGolden(t *testing.T) {
	goldens := map[string]string{
		"start":  `{"evidence":[{"assessment_id":"639678084915671599","testee_id":"18446744073709551615","report_id":"fixture-report","source_version":"v1","facts":[{"ref":"score","value":"private-fixture-fact"}]}],"request_id":"10000000-0000-4000-8000-000000000001","actor":{"org_id":"639678084915671598","subject_id":"fixture-subject"},"testee_id":"18446744073709551615","assessment_ids":["639678084915671599"],"goal":"private-fixture-goal \u003c\u0026\u003e 中文"}`,
		"answer": `{"command_id":"20000000-0000-4000-8000-000000000001","session_id":"30000000-0000-4000-8000-000000000001","actor":{"org_id":"639678084915671598","subject_id":"fixture-subject"},"action":"answer","expected_version":9007199254740993,"question_id":"40000000-0000-4000-8000-000000000001","answer":"private-fixture-answer \u003c\u0026\u003e 中文","skip":false}`,
		"cancel": `{"command_id":"20000000-0000-4000-8000-000000000001","session_id":"30000000-0000-4000-8000-000000000001","actor":{"org_id":"639678084915671598","subject_id":"fixture-subject"},"action":"cancel","expected_version":9007199254740993,"question_id":"","skip":false}`,
	}
	for kind, golden := range goldens {
		raw, hash := aiFixturePayload(t, kind)
		if string(raw) != golden || hash != sourceSHA([]byte(golden)) {
			t.Fatal("historical writer field order/omitempty/escaping contract changed")
		}
	}
}

func TestAISourceCanonicalWriterHashSeparateFromStoredJSONBytes(t *testing.T) {
	row := aiFixtureRow(t, AIBridgeCommandSource, "answer")
	canonical := append([]byte(nil), row[3]...)
	// MySQL may normalize key order/spacing/escaped characters. Preserve int64
	// as raw JSON, rather than losing precision through a float64 intermediary.
	var fields map[string]json.RawMessage
	if json.Unmarshal(canonical, &fields) != nil {
		t.Fatal("fixture fields")
	}
	normalized, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		t.Fatal("fixture normalize")
	}
	normalized = bytes.ReplaceAll(normalized, []byte(`\u003c`), []byte("<"))
	normalized = bytes.ReplaceAll(normalized, []byte(`\u003e`), []byte(">"))
	normalized = bytes.ReplaceAll(normalized, []byte(`\u0026`), []byte("&"))
	row[3] = normalized
	v := aiReadFixture(t, AIBridgeCommandSource, row)
	if v.WriterPayloadDigest.SHA256 != sourceSHA(canonical) || v.PayloadBytesDigest.SHA256 != sourceSHA(normalized) || v.WriterPayloadDigest.SHA256 == v.PayloadBytesDigest.SHA256 {
		t.Fatal("writer and CAST digest conflated")
	}
	legacyRow := aiFixtureRow(t, AILegacyCommandSource, "answer")
	legacyRow[3] = append([]byte(nil), normalized...)
	legacy := aiReadFixture(t, AILegacyCommandSource, legacyRow)
	if legacy.PayloadBytesDigest.Kind != AILegacyPayloadBytesKind || legacy.PayloadBytesDigest.SHA256 != sourceSHA(normalized) || legacy.WriterPayloadDigest != v.WriterPayloadDigest || ValidateAISourcePair(v, legacy) != nil {
		t.Fatal("legacy original blob conflated with writer hash")
	}
	// A raw CAST hash is not the producer hash, even if the copy's framing
	// digest independently matches the changed cells.
	row[4] = []byte(sourceSHA(normalized))
	raw, expected := aiFixtureCopy(t, AIBridgeCommandSource, [][][]byte{row}, nil)
	r, err := NewAISQLSourceReader(bytes.NewReader(raw), expected)
	if err != nil {
		t.Fatal("reader", err)
	}
	if _, err = r.Next(); err != ErrSourceDigest || r.Receipt().Complete {
		t.Fatal("accepted forged writer algorithm", err)
	}
}

func TestAISourceBusinessIsolationAndKnownProducerShape(t *testing.T) {
	for _, table := range []string{AIBridgeCommandSource, AILegacyCommandSource} {
		for _, mutation := range []string{"org_leading_zero", "org_overflow", "subject_missing", "assessment_duplicate", "answer_and_skip", "session_nil", "expected_version_fraction", "answer_null", "facts_unknown"} {
			t.Run(table+"/"+mutation, func(t *testing.T) {
				kind := "start"
				if strings.HasPrefix(mutation, "answer") || mutation == "session_nil" || mutation == "expected_version_fraction" {
					kind = "answer"
				}
				row := aiFixtureRow(t, table, kind)
				if kind == "start" {
					var original app.Start
					if json.Unmarshal(row[3], &original) != nil {
						t.Fatal("fixture start")
					}
					switch mutation {
					case "org_leading_zero":
						original.Actor.OrgID = "01"
					case "org_overflow":
						original.Actor.OrgID = "18446744073709551616"
					case "subject_missing":
						original.Actor.SubjectID = ""
					case "assessment_duplicate":
						original.AssessmentIDs = append(original.AssessmentIDs, original.AssessmentIDs[0])
					}
					payload, err := json.Marshal(original)
					if err != nil {
						t.Fatal("fixture encode")
					}
					row[3], row[4] = payload, []byte(sourceSHA(payload))
				} else {
					var original app.Change
					if json.Unmarshal(row[3], &original) != nil {
						t.Fatal("fixture change")
					}
					if mutation == "answer_and_skip" {
						original.Skip = true
					}
					if mutation == "session_nil" {
						original.SessionID = "00000000-0000-0000-0000-000000000000"
					}
					payload, err := json.Marshal(original)
					if err != nil {
						t.Fatal("fixture encode")
					}
					row[3], row[4] = payload, []byte(sourceSHA(payload))
				}
				if mutation == "expected_version_fraction" {
					row[3] = bytes.Replace(row[3], []byte(`"expected_version":9007199254740993`), []byte(`"expected_version":1.1`), 1)
				}
				if mutation == "answer_null" {
					row[3] = bytes.Replace(row[3], []byte(`"answer":"private-fixture-answer \u003c\u0026\u003e 中文"`), []byte(`"answer":null`), 1)
				}
				if mutation == "facts_unknown" {
					row[3] = bytes.Replace(row[3], []byte(`"ref":"score"`), []byte(`"complete":true,"ref":"score"`), 1)
				}
				raw, expected := aiFixtureCopy(t, table, [][][]byte{row}, nil)
				r, err := NewAISQLSourceReader(bytes.NewReader(raw), expected)
				if err != nil {
					t.Fatal("fixture reader", err)
				}
				if _, err = r.Next(); err == nil || r.Receipt().Complete {
					t.Fatal("cross-organization/unsupported producer facts accepted")
				}
				if strings.HasPrefix(mutation, "org_") || mutation == "subject_missing" {
					if err != ErrSourceOrganization {
						t.Fatal("wrong organization conflict category", err)
					}
				}
			})
		}
	}
	// A nil non-omitempty Facts slice really was emitted as null by encode.
	row := aiFixtureRow(t, AIBridgeCommandSource, "start")
	var original app.Start
	if json.Unmarshal(row[3], &original) != nil {
		t.Fatal("fixture nil facts")
	}
	original.Evidence[0].Facts = nil
	payload, err := json.Marshal(original)
	if err != nil {
		t.Fatal("fixture nil facts encode")
	}
	row[3], row[4] = payload, []byte(sourceSHA(payload))
	_ = aiReadFixture(t, AIBridgeCommandSource, row)
}

func TestAISourceRefusesUnknownWireAndAlteredFacts(t *testing.T) {
	cases := []struct {
		name, table, kind string
		mutate            func([][]byte)
		want              SourceError
	}{
		{"participant_retry", AIBridgeCommandSource, "start", func(row [][]byte) { row[2] = []byte("PARTICIPANT_RETRY") }, ErrSourceEventType},
		{"evaluation", AIBridgeCommandSource, "start", func(row [][]byte) { row[2] = []byte("EVALUATION_START") }, ErrSourceEventType},
		{"ack", AIBridgeCommandSource, "start", func(row [][]byte) { row[2] = []byte("EVENT_ACKNOWLEDGEMENT") }, ErrSourceEventType},
		{"uuid_nil", AIBridgeCommandSource, "start", func(row [][]byte) { row[0] = []byte("00000000-0000-0000-0000-000000000000") }, ErrSourceIdentity},
		{"uuid_upper", AIBridgeCommandSource, "start", func(row [][]byte) { row[0] = []byte("AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA") }, ErrSourceIdentity},
		{"request_cross", AIBridgeCommandSource, "start", func(row [][]byte) { row[1] = []byte(aiFixtureCommandID) }, ErrSourceIdentity},
		{"missing_hash", AIBridgeCommandSource, "start", func(row [][]byte) { row[4] = []byte("") }, ErrSourceDigest},
		{"different_hash", AIBridgeCommandSource, "start", func(row [][]byte) { row[4] = []byte(strings.Repeat("0", 64)) }, ErrSourceDigest},
		{"invalid_delivery", AIBridgeCommandSource, "start", func(row [][]byte) { row[5] = []byte("true") }, ErrSourceSchema},
		{"negative_attempt", AIBridgeCommandSource, "start", func(row [][]byte) { row[6] = []byte("-1") }, ErrSourceSchema},
		{"attempt_overflow", AIBridgeCommandSource, "start", func(row [][]byte) { row[6] = []byte("2147483648") }, ErrSourceSchema},
		{"attempt_noncanonical", AILegacyCommandSource, "start", func(row [][]byte) { row[5] = []byte("03") }, ErrSourceSchema},
		{"microsecond_lost", AIBridgeCommandSource, "start", func(row [][]byte) { row[7] = []byte("2026-10-08 11:12:13.123") }, ErrSourceSchema},
		{"invalid_date", AILegacyCommandSource, "start", func(row [][]byte) { row[6] = []byte("2026-02-30 11:12:13.123456") }, ErrSourceSchema},
		{"original_change_invented", AILegacyCommandSource, "answer", func(row [][]byte) { row[7] = []byte("2026-10-08T11:12:13.123456+08:00") }, ErrSourceSchema},
		{"original_time_other_zone", AILegacyCommandSource, "start", func(row [][]byte) { row[7] = []byte("2026-10-08T03:12:12.123456Z") }, ErrSourceSchema},
		{"original_time_false_precision", AILegacyCommandSource, "start", func(row [][]byte) { row[7] = []byte("2026-10-08T11:12:12.123456789+08:00") }, ErrSourceSchema},
		{"handoff_body_hash", AILegacyCommandSource, "start", func(row [][]byte) { row[8] = []byte("not-a-digest") }, ErrSourceSchema},
		{"nonnull_cell", AILegacyCommandSource, "start", func(row [][]byte) { row[7] = nil }, ErrSourceSchema},
		{"duplicate_json_key", AIBridgeCommandSource, "start", func(row [][]byte) {
			row[3] = bytes.Replace(row[3], []byte(`"request_id":`), []byte(`"request_id":"different","request_id":`), 1)
		}, ErrSourceDuplicateKey},
		{"unknown_json_field", AIBridgeCommandSource, "start", func(row [][]byte) { row[3] = append([]byte(`{"complete":true,`), row[3][1:]...) }, ErrSourceSchema},
		{"case_alias", AIBridgeCommandSource, "start", func(row [][]byte) { row[3] = bytes.Replace(row[3], []byte(`"actor"`), []byte(`"Actor"`), 1) }, ErrSourceSchema},
		{"null_actor", AIBridgeCommandSource, "start", func(row [][]byte) {
			row[3] = bytes.Replace(row[3], []byte(`{"org_id":"639678084915671598","subject_id":"fixture-subject"}`), []byte(`null`), 1)
		}, ErrSourceSchema},
		{"null_skip", AIBridgeCommandSource, "answer", func(row [][]byte) { row[3] = bytes.Replace(row[3], []byte(`"skip":false`), []byte(`"skip":null`), 1) }, ErrSourceSchema},
		{"malformed_utf8", AILegacyCommandSource, "start", func(row [][]byte) { row[3] = []byte{'"', 0xff, '"'} }, ErrSourceJSON},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			row := aiFixtureRow(t, tc.table, tc.kind)
			tc.mutate(row)
			raw, expected := aiFixtureCopy(t, tc.table, [][][]byte{row}, nil)
			r, err := NewAISQLSourceReader(bytes.NewReader(raw), expected)
			if err == nil {
				_, err = r.Next()
				if r.Receipt().Complete {
					t.Fatal("failed row complete")
				}
			}
			if err != tc.want {
				t.Fatal("wrong fixed rejection", err)
			}
		})
	}
}

func TestAISourceSchemaFullOrdinalMetadataAndApproval(t *testing.T) {
	for _, table := range []string{AIBridgeCommandSource, AILegacyCommandSource} {
		for _, mutation := range []string{"missing", "extra", "reorder", "pk_collation", "nullable", "type", "default", "extra_metadata", "tuple_short", "null_extra"} {
			t.Run(table+"/"+mutation, func(t *testing.T) {
				cols := aiFixtureColumns(table)
				switch mutation {
				case "missing":
					cols = cols[:len(cols)-1]
				case "extra":
					cols = append(cols, []*string{strptr("created_at"), strptr("datetime(6)"), strptr("NO"), nil, strptr(""), nil})
				case "reorder":
					cols[0], cols[1] = cols[1], cols[0]
				case "pk_collation":
					cols[0][5] = strptr("ascii_general_ci")
				case "nullable":
					cols[3][2] = strptr("YES")
				case "type":
					cols[3][1] = strptr("longtext")
				case "default":
					cols[0][3] = strptr("")
				case "extra_metadata":
					cols[0][4] = strptr("auto_increment")
				case "tuple_short":
					cols[0] = cols[0][:5]
				case "null_extra":
					cols[0][4] = nil
				}
				raw, expected := aiFixtureCopy(t, table, [][][]byte{aiFixtureRow(t, table, "start")}, cols)
				if _, err := NewAISQLSourceReader(bytes.NewReader(raw), expected); err != ErrSourceSchema {
					t.Fatal("unsupported schema accepted", err)
				}
			})
		}
	}
	raw, expected := aiFixtureCopy(t, AIBridgeCommandSource, [][][]byte{aiFixtureRow(t, AIBridgeCommandSource, "start")}, nil)
	for _, mutation := range []string{"database", "table", "kind", "absent", "pk_type", "schema_hash", "identity_hash", "empty", "upper_invalid", "records_cap", "bytes_cap"} {
		t.Run(mutation, func(t *testing.T) {
			bad := expected
			switch mutation {
			case "database":
				bad.Boundary.Database = "mongodb"
			case "table":
				bad.Boundary.Name = "ai_messaging_operations"
			case "kind":
				bad.Boundary.Kind = "view"
			case "absent":
				bad.Boundary.Present = false
			case "pk_type":
				bad.Boundary.PKType = "uint64"
			case "schema_hash":
				bad.Boundary.SchemaHash = strings.Repeat("c", 64)
			case "identity_hash":
				bad.Boundary.IdentityHash = strings.Repeat("c", 64)
			case "empty":
				bad.Boundary.Empty = true
			case "upper_invalid":
				bad.Boundary.UpperToken = "not_base64"
			case "records_cap":
				bad.Records = MaxSourceRecords + 1
			case "bytes_cap":
				bad.Bytes = MaxSourceBytes + 1
			}
			if _, err := NewAISQLSourceReader(bytes.NewReader(raw), bad); err == nil {
				t.Fatal("unapproved binding accepted")
			}
		})
	}
}

func TestAISourceHeaderUnknownCaseNullAndInvalidBase64(t *testing.T) {
	raw, expected := aiFixtureCopy(t, AIBridgeCommandSource, [][][]byte{aiFixtureRow(t, AIBridgeCommandSource, "start")}, nil)
	for _, mutation := range []string{"unknown_header", "unknown_boundary", "header_case", "boundary_case", "boundary_null", "duplicate_header", "protocol_old", "row_base64"} {
		t.Run(mutation, func(t *testing.T) {
			input := append([]byte(nil), raw...)
			switch mutation {
			case "unknown_header":
				input = append([]byte(`{"complete":true,`), input[1:]...)
			case "unknown_boundary":
				input = bytes.Replace(input, []byte(`"boundary":{`), []byte(`"boundary":{"complete":true,`), 1)
			case "header_case":
				input = bytes.Replace(input, []byte(`"protocol"`), []byte(`"Protocol"`), 1)
			case "boundary_case":
				input = bytes.Replace(input, []byte(`"present"`), []byte(`"Present"`), 1)
			case "boundary_null":
				input = bytes.Replace(input, []byte(`"present":true`), []byte(`"present":null`), 1)
			case "duplicate_header":
				input = append([]byte(`{"protocol":"other",`), input[1:]...)
			case "protocol_old":
				input = bytes.Replace(input, []byte(SQLSourceProtocol), []byte("mysql_cast_binary_columns_pk_order_v1"), 1)
			case "row_base64":
				newline := bytes.IndexByte(input, '\n')
				input = append(input[:newline+1], []byte(`["invalid-base64"]`+"\n")...)
			}
			r, err := NewAISQLSourceReader(bytes.NewReader(input), expected)
			if err == nil {
				_, err = r.Next()
			}
			if err == nil {
				t.Fatal("unknown/raw unapproved protocol accepted")
			}
		})
	}
}

func TestAISourceExactEOFOrderCountsAndTransportFraming(t *testing.T) {
	for _, table := range []string{AIBridgeCommandSource, AILegacyCommandSource} {
		raw, expected := aiFixtureCopy(t, table, nil, nil)
		r, err := NewAISQLSourceReader(bytes.NewReader(raw), expected)
		if err != nil {
			t.Fatal("empty source", err)
		}
		if _, err = r.Next(); err != io.EOF || !r.Receipt().Complete || r.Receipt().DropReady {
			t.Fatal("empty proof")
		}
	}
	row := aiFixtureRow(t, AIBridgeCommandSource, "start")
	raw, expected := aiFixtureCopy(t, AIBridgeCommandSource, [][][]byte{row}, nil)
	for _, mutation := range []string{"truncated", "digest", "bytes", "records", "trailing", "duplicate", "mutated_delivery", "mutated_retry"} {
		t.Run(mutation, func(t *testing.T) {
			input := append([]byte(nil), raw...)
			want := expected
			switch mutation {
			case "truncated":
				input = input[:bytes.IndexByte(input, '\n')+1]
			case "digest":
				want.DataHash = strings.Repeat("0", 64)
			case "bytes":
				want.Bytes++
			case "records":
				want.Records++
			case "trailing":
				input = append(input, []byte("{}\n")...)
			case "duplicate":
				input, want = aiFixtureCopy(t, AIBridgeCommandSource, [][][]byte{row, row}, nil)
			case "mutated_delivery":
				changed := aiFixtureRow(t, AIBridgeCommandSource, "start")
				changed[5] = []byte("1")
				input, _ = aiFixtureCopy(t, AIBridgeCommandSource, [][][]byte{changed}, nil)
			case "mutated_retry":
				changed := aiFixtureRow(t, AIBridgeCommandSource, "start")
				changed[6] = []byte("4")
				input, _ = aiFixtureCopy(t, AIBridgeCommandSource, [][][]byte{changed}, nil)
			}
			r, err := NewAISQLSourceReader(bytes.NewReader(input), want)
			if err != nil {
				t.Fatal("setup", err)
			}
			for err == nil {
				_, err = r.Next()
			}
			if err == io.EOF || r.Receipt().Complete {
				t.Fatal("incomplete/conflicting source accepted")
			}
			if _, nextErr := r.Next(); nextErr != ErrSourceIncomplete {
				t.Fatal("failed source resumed", nextErr)
			}
		})
	}
	// Row digests include delivery/retry fields, not just payload and identity.
	v1 := aiReadFixture(t, AIBridgeCommandSource, row)
	changed := aiFixtureRow(t, AIBridgeCommandSource, "start")
	changed[5] = []byte("1")
	v2 := aiReadFixture(t, AIBridgeCommandSource, changed)
	if v1.Source.Digest == v2.Source.Digest || v1.PayloadBytesDigest != v2.PayloadBytesDigest {
		t.Fatal("transport missing from framed source digest")
	}
}

func TestAISourcePairIdentityHashBudgetAndNoReplay(t *testing.T) {
	bridge := aiReadFixture(t, AIBridgeCommandSource, aiFixtureRow(t, AIBridgeCommandSource, "answer"))
	legacy := aiReadFixture(t, AILegacyCommandSource, aiFixtureRow(t, AILegacyCommandSource, "answer"))
	if err := ValidateAISourcePair(bridge, legacy); err != nil {
		t.Fatal("valid source pair", err)
	}
	for _, mutate := range []func(*DecodedAICommand){
		func(v *DecodedAICommand) { v.CommandID = aiFixtureRequestID },
		func(v *DecodedAICommand) { v.RequestID = aiFixtureSessionID },
		func(v *DecodedAICommand) { v.OrganizationID = "2" },
		func(v *DecodedAICommand) { v.SubjectID = "other" },
		func(v *DecodedAICommand) { v.ResourceID = aiFixtureQuestionID },
		func(v *DecodedAICommand) { v.WriterPayloadDigest.SHA256 = strings.Repeat("a", 64) },
		func(v *DecodedAICommand) { v.PayloadBytesDigest.SHA256 = strings.Repeat("a", 64) },
		func(v *DecodedAICommand) { v.Transport.SourceAttempts++ },
		func(v *DecodedAICommand) { v.Transport.SourceAvailableAt = "2026-10-08 11:12:13.123457" },
		func(v *DecodedAICommand) { v.Business.ExpectedVersion++ },
		func(v *DecodedAICommand) { v.Transport.MessagingBodySHA256 = "" },
	} {
		bad := *legacy
		mutate(&bad)
		if err := ValidateAISourcePair(bridge, &bad); err != ErrAISourceHandoff {
			t.Fatal("ambiguous handoff accepted", err)
		}
	}
	delivered := true
	bad := *bridge
	bad.Transport.Delivered = &delivered
	if ValidateAISourcePair(&bad, legacy) != ErrAISourceHandoff || ValidateAISourcePair(nil, legacy) != ErrAISourceHandoff {
		t.Fatal("multiple delivery owners accepted")
	}
}

func TestAISourceBodyNeverSerializedOrIncludedInDiagnostics(t *testing.T) {
	for _, kind := range []string{"start", "answer"} {
		v := aiReadFixture(t, AIBridgeCommandSource, aiFixtureRow(t, AIBridgeCommandSource, kind))
		if _, err := json.Marshal(v); !errors.Is(err, ErrSourceSerialization) {
			t.Fatal("JSON exposed private references")
		}
		if _, err := bson.Marshal(v); !errors.Is(err, ErrSourceSerialization) {
			t.Fatal("BSON exposed private references")
		}
		text := fmt.Sprintf("%v %s %#v", v, v, v)
		for _, secret := range []string{"private-fixture-goal", "private-fixture-answer", "private-fixture-fact", aiFixtureRequestID, "fixture-subject"} {
			if strings.Contains(text, secret) {
				t.Fatal("diagnostic leaked private field")
			}
		}
		fields := reflect.TypeOf(*v)
		for i := 0; i < fields.NumField(); i++ {
			if fields.Field(i).Type == reflect.TypeOf(app.Start{}) || fields.Field(i).Type == reflect.TypeOf(app.Change{}) || fields.Field(i).Type == reflect.TypeOf([]byte{}) {
				t.Fatal("decoded result retained source body")
			}
		}
	}
}
