package retirement

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/google/uuid"
)

const (
	AIBridgeCommandSource                = "ai_bridge_commands"
	AILegacyCommandSource                = "ai_messaging_legacy_commands"
	AIBridgePayloadBytesKind             = "mysql_json_payload_cast_binary_sha256"
	AILegacyPayloadBytesKind             = "mysql_blob_source_payload_sha256"
	AIStartWriterDigestKind              = "go-json-marshal-aibridge-start-v1"
	AIChangeWriterDigestKind             = "go-json-marshal-aibridge-change-v1"
	ErrAISourceHandoff       SourceError = "ai_source_handoff_conflict"
)

// AICommandBusinessFacts contains references and digests, never Goal, Answer or
// evidence fact values. An original request must still independently bind its
// actor, session, testee and assessment associations before retirement.
type AICommandBusinessFacts struct {
	TesteeID        string
	AssessmentIDs   []string
	SessionID       string
	ExpectedVersion int64
	QuestionID      string
	AnswerPresent   bool
	AnswerSHA256    string
	Skip            bool
}

// AICommandTransportFacts records actual source observations. SourceAttempts is
// the legacy failed-delivery counter, not a provider invocation count. The floor
// is the existing handoff's min(attempts, 8), never an instruction to spend or
// re-inherit a budget. SQL DATETIME(6) bytes carry no timezone; retry availability
// cannot substitute for the missing original change-command timestamp.
type AICommandTransportFacts struct {
	Delivered              *bool
	SourceAttempts         uint32
	HandoffBudgetFloor     uint32
	HandoffBudgetExhausted bool
	SourceAvailableAt      string
	OriginalOccurredAt     string
	TransferredAt          string
	MessagingBodySHA256    string
}

// DecodedAICommand is private, bounded, body-free source evidence. Neither
// delivered=true nor an immutable handoff row proves business terminal state,
// an authenticated command decision, or absence of MQ/external responsibility.
type DecodedAICommand struct {
	Source              evidence.HistoricalSourceReferenceV1
	SupportedSchema     string
	CommandID           string
	RequestID           string
	SourceKind          string
	MessagingKind       string
	OrganizationID      string
	SubjectID           string
	ResourceID          string
	PayloadBytesDigest  evidence.Digest
	WriterPayloadDigest evidence.Digest
	Business            AICommandBusinessFacts
	Transport           AICommandTransportFacts
	ResolverGaps        []string
}

func (DecodedAICommand) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (DecodedAICommand) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (v DecodedAICommand) String() string {
	return "private decoded AI command (" + v.Source.Digest.SHA256 + ")"
}
func (v DecodedAICommand) GoString() string { return v.String() }

type aiSQLColumn struct{ name, kind, collation string }

// Ordinal layouts are exactly migrations 72 (8 columns, no created_at) and 93
// (10 columns). Request created_at was added to ai_bridge_requests by 83, not
// commands. No projection or extra/missing source column is accepted.
var aiBridgeColumns = []aiSQLColumn{
	{"command_id", "char(36)", "ascii_bin"}, {"request_id", "char(36)", "ascii_bin"},
	{"kind", "varchar(16)", "utf8mb4"}, {"payload", "json", ""},
	{"payload_hash", "char(64)", "utf8mb4"}, {"delivered", "tinyint(1)", ""},
	{"attempts", "int", ""}, {"available_at", "datetime(6)", ""},
}
var aiLegacyColumns = []aiSQLColumn{
	{"command_id", "char(36)", "ascii_bin"}, {"request_id", "char(36)", "ascii_bin"},
	{"source_kind", "varchar(16)", "ascii_bin"}, {"source_payload", "mediumblob", ""},
	{"source_payload_hash", "char(64)", "ascii_bin"}, {"source_attempts", "int", ""},
	{"source_available_at", "datetime(6)", ""}, {"source_original_time", "varchar(64)", "ascii_bin"},
	{"messaging_body_sha256", "char(64)", "ascii_bin"}, {"transferred_at", "datetime(6)", ""},
}

type AISQLSourceReader struct {
	scanner     *bufio.Scanner
	acc         sourceAccumulator
	columns     SQLColumns
	columnsHash string
	upper, last string
}

// NewAISQLSourceReader consumes only a v2 private source copy whose boundary,
// full-cell digest and exact counts were independently approved. A caller must
// reach clean EOF and check Receipt().Complete before any subsequent business
// CAS. This reader performs no database, filesystem, network or mutation I/O.
func NewAISQLSourceReader(input io.Reader, expected SourceCopyExpectation) (*AISQLSourceReader, error) {
	b := expected.Boundary
	if input == nil || b.Database != "mysql" || (b.Name != AIBridgeCommandSource && b.Name != AILegacyCommandSource) ||
		b.Kind != "base_table" || !b.Present || b.PKType != "ascii_string" || !evidence.ValidSHA256(b.SchemaHash) ||
		!evidence.ValidSHA256(b.IdentityHash) || !evidence.ValidSHA256(expected.DataHash) || len(b.UpperToken) > 1368 ||
		expected.Records > MaxSourceRecords || expected.Bytes > MaxSourceBytes || b.Empty != (expected.Records == 0) ||
		(b.Empty && (b.UpperToken != "" || expected.Bytes != 0)) || (!b.Empty && b.UpperToken == "") {
		return nil, ErrSourceProtocol
	}
	s := &AISQLSourceReader{acc: sourceAccumulator{expectation: expected, h: sha256.New()}}
	s.scanner = bufio.NewScanner(input)
	s.scanner.Buffer(make([]byte, 64*1024), MaxSourceRowBytes*2)
	if !s.scanner.Scan() {
		return nil, ErrSourceProtocol
	}
	raw := s.scanner.Bytes()
	if err := strictJSON(raw); err != nil {
		return nil, err
	}
	var header sqlSourceHeader
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&header) != nil {
		return nil, ErrSourceProtocol
	}
	var keys map[string]json.RawMessage
	if json.Unmarshal(raw, &keys) != nil || !aiExactKeys(keys, []string{"protocol", "columns", "boundary"}, nil) {
		return nil, ErrSourceProtocol
	}
	var boundaryKeys map[string]json.RawMessage
	if json.Unmarshal(keys["boundary"], &boundaryKeys) != nil || !aiExactKeys(boundaryKeys, []string{"database", "name", "kind", "present", "empty", "pk_type", "upper_token", "schema_hash", "identity_hash"}, nil) {
		return nil, ErrSourceProtocol
	}
	if header.Protocol != SQLSourceProtocol || !reflect.DeepEqual(header.Boundary, b) {
		return nil, ErrSourceProtocol
	}
	if err := aiValidateColumns(b.Name, header.Columns); err != nil {
		return nil, err
	}
	s.columns = header.Columns
	columnsRaw, err := json.Marshal(header.Columns)
	if err != nil {
		return nil, ErrSourceProtocol
	}
	s.columnsHash = sourceSHA(columnsRaw)
	sourceFrame(s.acc.h, []byte(s.columnsHash), false)
	if !b.Empty {
		upper, err := canonicalBase64(b.UpperToken)
		if err != nil {
			return nil, err
		}
		if !aiOriginalUUID(string(upper)) {
			return nil, ErrSourceIdentity
		}
		s.upper = string(upper)
	}
	return s, nil
}

func aiValidateColumns(table string, columns SQLColumns) error {
	wants := aiBridgeColumns
	if table == AILegacyCommandSource {
		wants = aiLegacyColumns
	}
	if len(columns) != len(wants) {
		return ErrSourceSchema
	}
	for i, c := range columns {
		want := wants[i]
		if len(c) != 6 || c[0] == nil || c[1] == nil || c[2] == nil || c[4] == nil || *c[0] != want.name || *c[1] != want.kind || *c[2] != "NO" {
			return ErrSourceSchema
		}
		for _, field := range c {
			if field != nil && !utf8.ValidString(*field) {
				return ErrSourceSchema
			}
		}
		if want.collation == "" {
			if c[5] != nil {
				return ErrSourceSchema
			}
		} else if c[5] == nil {
			return ErrSourceSchema
		} else if want.collation == "utf8mb4" {
			// The original table did not specify a collation. These known MySQL
			// defaults are supported; the complete observed metadata is still
			// included in the independently approved framing digest.
			switch *c[5] {
			case "utf8mb4_0900_ai_ci", "utf8mb4_general_ci", "utf8mb4_unicode_ci", "utf8mb4_bin":
			default:
				return ErrSourceSchema
			}
		} else if *c[5] != want.collation {
			return ErrSourceSchema
		}
		if table == AIBridgeCommandSource && i == 7 {
			if c[3] == nil || strings.ToLower(*c[3]) != "current_timestamp(6)" || *c[4] != "DEFAULT_GENERATED" {
				return ErrSourceSchema
			}
		} else {
			if *c[4] != "" {
				return ErrSourceSchema
			}
			if table == AIBridgeCommandSource && (i == 5 || i == 6) {
				if c[3] == nil || *c[3] != "0" {
					return ErrSourceSchema
				}
			} else if c[3] != nil {
				return ErrSourceSchema
			}
		}
	}
	return nil
}

func (s *AISQLSourceReader) Next() (*DecodedAICommand, error) {
	if s.acc.done {
		return nil, io.EOF
	}
	if s.acc.failed {
		return nil, ErrSourceIncomplete
	}
	if !s.scanner.Scan() {
		if s.scanner.Err() != nil {
			s.acc.failed = true
			return nil, ErrSourceIncomplete
		}
		return nil, s.acc.finish()
	}
	v, err := s.decodeLine(s.scanner.Bytes())
	if err != nil {
		s.acc.failed = true
	}
	return v, err
}

func (s *AISQLSourceReader) Receipt() SourceCopyReceipt { return s.acc.receipt(SQLSourceProtocol) }

func (s *AISQLSourceReader) decodeLine(line []byte) (*DecodedAICommand, error) {
	if err := strictJSON(line); err != nil {
		return nil, err
	}
	var encoded []*string
	if json.Unmarshal(line, &encoded) != nil || len(encoded) != len(s.columns) {
		return nil, ErrSourceSchema
	}
	cells := make([][]byte, len(encoded))
	rowHash := sha256.New()
	sourceFrame(rowHash, []byte(s.columnsHash), false)
	var size uint64
	for i, text := range encoded {
		if text == nil {
			return nil, ErrSourceSchema
		}
		raw, err := canonicalBase64(*text)
		if err != nil {
			return nil, err
		}
		cells[i] = raw
		size += uint64(len(raw))
		if size > MaxSourceRowBytes {
			return nil, ErrSourceBounds
		}
		sourceFrame(rowHash, raw, false)
	}
	id, requestID := string(cells[0]), string(cells[1])
	if !aiOriginalUUID(id) || !aiOriginalUUID(requestID) {
		return nil, ErrSourceIdentity
	}
	if s.acc.expectation.Boundary.Empty || id > s.upper || id <= s.last {
		return nil, ErrSourceOrder
	}
	kind, writerHash := string(cells[2]), string(cells[4])
	if kind != "start" && kind != "answer" && kind != "cancel" {
		return nil, ErrSourceEventType
	}
	if !evidence.ValidSHA256(writerHash) {
		return nil, ErrSourceDigest
	}
	value, err := aiDecodeBusiness(kind, id, requestID, cells[3], writerHash)
	if err != nil {
		return nil, err
	}
	table := s.acc.expectation.Boundary.Name
	bytesKind := AIBridgePayloadBytesKind
	attemptIndex, clockIndex := 6, 7
	if table == AILegacyCommandSource {
		bytesKind, attemptIndex, clockIndex = AILegacyPayloadBytesKind, 5, 6
	}
	attempts, err := strconv.ParseUint(string(cells[attemptIndex]), 10, 31)
	if err != nil || strconv.FormatUint(attempts, 10) != string(cells[attemptIndex]) {
		return nil, ErrSourceSchema
	}
	if !aiSQLMicrosecondClock(string(cells[clockIndex])) {
		return nil, ErrSourceSchema
	}
	value.Transport.SourceAttempts = uint32(attempts)
	value.Transport.HandoffBudgetFloor = uint32(min(attempts, 8))
	value.Transport.HandoffBudgetExhausted = attempts >= 8
	value.Transport.SourceAvailableAt = string(cells[clockIndex])
	if table == AIBridgeCommandSource {
		if string(cells[5]) != "0" && string(cells[5]) != "1" {
			return nil, ErrSourceSchema
		}
		delivered := string(cells[5]) == "1"
		value.Transport.Delivered = &delivered
		value.SupportedSchema = "ai-bridge-command-72-start-change-json-v1"
	} else {
		original := string(cells[7])
		if !evidence.ValidSHA256(string(cells[8])) || !aiSQLMicrosecondClock(string(cells[9])) {
			return nil, ErrSourceSchema
		}
		if original != "" {
			at, err := time.Parse(time.RFC3339Nano, original)
			_, offset := at.Zone()
			if err != nil || at.IsZero() || at.Nanosecond()%1000 != 0 || at.Format(time.RFC3339Nano) != original || offset != 8*3600 || kind != "start" {
				return nil, ErrSourceSchema
			}
		}
		value.Transport.OriginalOccurredAt = original
		value.Transport.MessagingBodySHA256 = string(cells[8])
		value.Transport.TransferredAt = string(cells[9])
		value.SupportedSchema = "ai-legacy-command-93-start-change-json-v1"
		value.ResolverGaps = append(value.ResolverGaps, "authenticated_live_operation_outbox_receipt_inbox_binding_required")
	}
	value.PayloadBytesDigest = evidence.SourceDigest(bytesKind, cells[3])
	value.Source = evidence.HistoricalSourceReferenceV1{Database: "mysql", Object: table, PrimaryKeyKind: "mysql_ascii_uuid", PrimaryKeySHA256: sourceSHA(cells[0]), Digest: evidence.Digest{Kind: SQLRowDigestKind, SHA256: hex.EncodeToString(rowHash.Sum(nil))}}
	if err := s.acc.add(size); err != nil {
		return nil, err
	}
	for _, raw := range cells {
		sourceFrame(s.acc.h, raw, false)
	}
	s.last = id
	return value, nil
}

func aiOriginalUUID(text string) bool {
	id, err := uuid.Parse(text)
	return err == nil && id != uuid.Nil && id.String() == text
}
func aiPositiveNumber(text string) bool {
	n, err := strconv.ParseUint(text, 10, 64)
	return err == nil && n > 0 && strconv.FormatUint(n, 10) == text
}
func aiSQLMicrosecondClock(text string) bool {
	if len(text) != 26 || text[19] != '.' {
		return false
	}
	at, err := time.Parse("2006-01-02 15:04:05.000000", text)
	return err == nil && !at.IsZero() && at.Format("2006-01-02 15:04:05.000000") == text
}

func aiExactKeys(fields map[string]json.RawMessage, required, optional []string) bool {
	if fields == nil {
		return false
	}
	allowed := make(map[string]bool, len(required)+len(optional))
	for _, key := range required {
		if fields[key] == nil {
			return false
		}
		allowed[key] = true
	}
	for _, key := range optional {
		allowed[key] = true
	}
	for key, raw := range fields {
		if !allowed[key] || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return false
		}
	}
	return true
}

// Validate the historical app's exact JSON shape separately from the generic
// DCE shape decoder (which intentionally has no array/bool producer contracts).
// json.Marshal typed values then reproduces the original encode hash despite
// MySQL normalizing JSON order, spaces, escapes and numeric representations.
func aiPayloadShape(raw []byte, kind string) error {
	if err := strictJSON(raw); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return ErrSourceSchema
	}
	if kind == "start" {
		if !aiExactKeys(fields, []string{"request_id", "actor", "testee_id", "assessment_ids", "goal"}, []string{"evidence"}) {
			return ErrSourceSchema
		}
		if itemsRaw, ok := fields["evidence"]; ok {
			var items []map[string]json.RawMessage
			if json.Unmarshal(itemsRaw, &items) != nil {
				return ErrSourceSchema
			}
			for _, item := range items {
				// Facts is a non-omitempty slice in the original Go producer; nil
				// is genuinely encoded as JSON null, unlike scalar field nulls.
				facts, ok := item["facts"]
				if !ok {
					return ErrSourceSchema
				}
				copy := make(map[string]json.RawMessage, len(item))
				for key, value := range item {
					copy[key] = value
				}
				copy["facts"] = json.RawMessage("[]")
				if !aiExactKeys(copy, []string{"assessment_id", "testee_id", "report_id", "source_version", "facts"}, nil) {
					return ErrSourceSchema
				}
				if !bytes.Equal(bytes.TrimSpace(facts), []byte("null")) {
					var entries []map[string]json.RawMessage
					if json.Unmarshal(facts, &entries) != nil {
						return ErrSourceSchema
					}
					for _, fact := range entries {
						if !aiExactKeys(fact, []string{"ref", "value"}, nil) {
							return ErrSourceSchema
						}
					}
				}
			}
		}
	} else if !aiExactKeys(fields, []string{"command_id", "session_id", "actor", "action", "expected_version", "question_id", "skip"}, []string{"answer"}) {
		return ErrSourceSchema
	}
	var actor map[string]json.RawMessage
	if json.Unmarshal(fields["actor"], &actor) != nil || !aiExactKeys(actor, []string{"org_id", "subject_id"}, nil) {
		return ErrSourceSchema
	}
	return nil
}

func aiDecodeBusiness(kind, id, requestID string, raw []byte, writerHash string) (*DecodedAICommand, error) {
	if err := aiPayloadShape(raw, kind); err != nil {
		return nil, err
	}
	result := &DecodedAICommand{CommandID: id, RequestID: requestID, SourceKind: kind, ResolverGaps: []string{"trusted_original_request_actor_business_binding_required", "business_terminal_and_no_unfinished_responsibility_required"}}
	var canonical []byte
	var err error
	if kind == "start" {
		var value app.Start
		if json.Unmarshal(raw, &value) != nil {
			return nil, ErrSourceSchema
		}
		if value.RequestID != id || requestID != id || !aiPositiveNumber(value.TesteeID) || len(value.AssessmentIDs) < 1 || len(value.AssessmentIDs) > 10 || value.Goal == "" || len(value.Goal) > 8000 {
			return nil, ErrSourceIdentity
		}
		seen := make(map[string]bool, len(value.AssessmentIDs))
		for _, assessment := range value.AssessmentIDs {
			if !aiPositiveNumber(assessment) || seen[assessment] {
				return nil, ErrSourceIdentity
			}
			seen[assessment] = true
		}
		result.OrganizationID, result.SubjectID, result.ResourceID, result.MessagingKind = value.Actor.OrgID, value.Actor.SubjectID, value.RequestID, "START"
		result.Business.TesteeID = value.TesteeID
		result.Business.AssessmentIDs = append([]string(nil), value.AssessmentIDs...)
		canonical, err = json.Marshal(value)
		result.WriterPayloadDigest.Kind = AIStartWriterDigestKind
	} else {
		var value app.Change
		if json.Unmarshal(raw, &value) != nil {
			return nil, ErrSourceSchema
		}
		if value.CommandID != id || value.Action != kind || !aiOriginalUUID(value.SessionID) || value.ExpectedVersion < 1 {
			return nil, ErrSourceIdentity
		}
		if kind == "answer" && (!aiOriginalUUID(value.QuestionID) || (value.Skip && value.Answer != nil) || (!value.Skip && (value.Answer == nil || *value.Answer == ""))) {
			return nil, ErrSourceIdentity
		}
		result.OrganizationID, result.SubjectID, result.ResourceID, result.MessagingKind = value.Actor.OrgID, value.Actor.SubjectID, value.SessionID, "CHANGE"
		result.Business.SessionID, result.Business.ExpectedVersion, result.Business.QuestionID, result.Business.Skip = value.SessionID, value.ExpectedVersion, value.QuestionID, value.Skip
		if value.Answer != nil {
			result.Business.AnswerPresent = true
			result.Business.AnswerSHA256 = sourceSHA([]byte(*value.Answer))
		}
		result.ResolverGaps = append(result.ResolverGaps, "original_per_command_time_unavailable", "trusted_request_session_binding_required")
		canonical, err = json.Marshal(value)
		result.WriterPayloadDigest.Kind = AIChangeWriterDigestKind
	}
	if err != nil || sourceSHA(canonical) != writerHash {
		return nil, ErrSourceDigest
	}
	if !aiPositiveNumber(result.OrganizationID) || result.SubjectID == "" || len(result.SubjectID) > 128 {
		return nil, ErrSourceOrganization
	}
	result.WriterPayloadDigest.SHA256 = writerHash
	return result, nil
}

// ValidateAISourcePair binds the existing handoff to its original bridge row.
// It grants no ownership/closure conclusion. Changed delivery ownership, source
// bytes, retry counter/clock or actor/request is an explicit conflict; do not
// infer a new transfer or replay to remove such an ambiguity.
func ValidateAISourcePair(bridge, legacy *DecodedAICommand) error {
	if !aiDecodedIdentityValid(bridge) || !aiDecodedIdentityValid(legacy) || bridge.Source.Object != AIBridgeCommandSource || legacy.Source.Object != AILegacyCommandSource || bridge.Transport.Delivered == nil || *bridge.Transport.Delivered ||
		bridge.CommandID != legacy.CommandID || bridge.RequestID != legacy.RequestID || bridge.SourceKind != legacy.SourceKind || bridge.MessagingKind != legacy.MessagingKind ||
		bridge.OrganizationID != legacy.OrganizationID || bridge.SubjectID != legacy.SubjectID || bridge.ResourceID != legacy.ResourceID ||
		bridge.WriterPayloadDigest != legacy.WriterPayloadDigest || bridge.PayloadBytesDigest.SHA256 != legacy.PayloadBytesDigest.SHA256 ||
		bridge.Transport.SourceAttempts != legacy.Transport.SourceAttempts || bridge.Transport.SourceAvailableAt != legacy.Transport.SourceAvailableAt ||
		!reflect.DeepEqual(bridge.Business, legacy.Business) || !evidence.ValidSHA256(legacy.Transport.MessagingBodySHA256) {
		return ErrAISourceHandoff
	}
	return nil
}

func aiDecodedIdentityValid(v *DecodedAICommand) bool {
	if v == nil || !aiOriginalUUID(v.CommandID) || !aiOriginalUUID(v.RequestID) || !aiOriginalUUID(v.ResourceID) || !aiPositiveNumber(v.OrganizationID) ||
		v.SubjectID == "" || len(v.SubjectID) > 128 || !utf8.ValidString(v.SubjectID) || v.Source.Database != "mysql" || v.Source.PrimaryKeyKind != "mysql_ascii_uuid" ||
		v.Source.PrimaryKeySHA256 != sourceSHA([]byte(v.CommandID)) || v.Source.Digest.Kind != SQLRowDigestKind || !evidence.ValidSHA256(v.Source.Digest.SHA256) ||
		!evidence.ValidSHA256(v.PayloadBytesDigest.SHA256) || !evidence.ValidSHA256(v.WriterPayloadDigest.SHA256) || !aiSQLMicrosecondClock(v.Transport.SourceAvailableAt) {
		return false
	}
	switch v.Source.Object {
	case AIBridgeCommandSource:
		if v.PayloadBytesDigest.Kind != AIBridgePayloadBytesKind || v.SupportedSchema != "ai-bridge-command-72-start-change-json-v1" {
			return false
		}
	case AILegacyCommandSource:
		if v.PayloadBytesDigest.Kind != AILegacyPayloadBytesKind || v.SupportedSchema != "ai-legacy-command-93-start-change-json-v1" {
			return false
		}
	default:
		return false
	}
	if v.SourceKind == "start" {
		return v.CommandID == v.RequestID && v.ResourceID == v.RequestID && v.MessagingKind == "START" && v.WriterPayloadDigest.Kind == AIStartWriterDigestKind
	}
	return (v.SourceKind == "answer" || v.SourceKind == "cancel") && v.MessagingKind == "CHANGE" && v.WriterPayloadDigest.Kind == AIChangeWriterDigestKind
}
