package retirement

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"reflect"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
)

// SQLColumns is exactly information_schema columns in paging.go: name,
// column_type, is_nullable, default, extra, collation, in ordinal order.
type SQLColumns [][]*string

type sqlSourceHeader struct {
	Protocol string         `json:"protocol"`
	Columns  SQLColumns     `json:"columns"`
	Boundary SourceBoundary `json:"boundary"`
}

// The only supported SQL source schema is the retired PO's full 18+49 schema.
// Earlier/missing/extra columns are explicitly unsupported, even if enough
// fields could be projected to reconstruct a superficially valid event.
var sqlSourceColumns = []struct{ name, kind, nullable string }{
	{"id", "bigint unsigned", "NO"}, {"event_id", "varchar(64)", "NO"}, {"event_type", "varchar(128)", "NO"},
	{"aggregate_type", "varchar(64)", "NO"}, {"aggregate_id", "varchar(64)", "NO"}, {"org_id", "bigint", "YES"},
	{"topic_name", "varchar(128)", "NO"}, {"payload_json", "longtext", "NO"}, {"status", "varchar(32)", "NO"},
	{"attempt_count", "int unsigned", "NO"}, {"retry_disposition", "varchar(32)", "YES"}, {"next_attempt_at", "datetime(3)", "NO"},
	{"last_error", "text", "YES"}, {"last_error_kind", "varchar(32)", "YES"}, {"manual_replay_request_id", "varchar(64)", "YES"},
	{"created_at", "datetime(3)", "NO"}, {"updated_at", "datetime(3)", "NO"}, {"published_at", "datetime(3)", "YES"},
}

type SQLSourceReader struct {
	scanner     *bufio.Scanner
	acc         sourceAccumulator
	columns     SQLColumns
	columnsHash string
	upper, last uint64
}

func NewSQLSourceReader(input io.Reader, expected SourceCopyExpectation) (*SQLSourceReader, error) {
	if input == nil || validExpectation(expected, "mysql") != nil || expected.Boundary.PKType != "uint64" {
		return nil, ErrSourceProtocol
	}
	s := &SQLSourceReader{acc: sourceAccumulator{expectation: expected, h: sha256.New()}}
	// Do not turn a byte budget into a synthetic EOF: trailing bytes must be
	// observed and rejected. Per-line and exact aggregate raw-cell budgets are
	// enforced instead, before a receipt can become complete.
	s.scanner = bufio.NewScanner(input)
	s.scanner.Buffer(make([]byte, 64*1024), MaxSourceRowBytes*2)
	if !s.scanner.Scan() {
		return nil, ErrSourceProtocol
	}
	headerBytes := s.scanner.Bytes()
	if err := strictJSON(headerBytes); err != nil {
		return nil, err
	}
	var header sqlSourceHeader
	d := json.NewDecoder(bytes.NewReader(headerBytes))
	d.DisallowUnknownFields()
	if d.Decode(&header) != nil {
		return nil, ErrSourceProtocol
	}
	var keys map[string]json.RawMessage
	if json.Unmarshal(headerBytes, &keys) != nil || len(keys) != 3 || keys["protocol"] == nil || keys["columns"] == nil || keys["boundary"] == nil {
		return nil, ErrSourceProtocol
	}
	// Check the boundary's exact spelling as well: encoding/json accepts case
	// aliases. The approved boundary must be independent of this header.
	var boundaryKeys map[string]json.RawMessage
	if json.Unmarshal(keys["boundary"], &boundaryKeys) != nil || len(boundaryKeys) != 9 {
		return nil, ErrSourceProtocol
	}
	for _, name := range []string{"database", "name", "kind", "present", "empty", "pk_type", "upper_token", "schema_hash", "identity_hash"} {
		if boundaryKeys[name] == nil || bytes.Equal(bytes.TrimSpace(boundaryKeys[name]), []byte("null")) {
			return nil, ErrSourceProtocol
		}
	}
	if header.Protocol != SQLSourceProtocol || !reflect.DeepEqual(header.Boundary, expected.Boundary) {
		return nil, ErrSourceProtocol
	}
	if len(header.Columns) != len(sqlSourceColumns) {
		return nil, ErrSourceSchema
	}
	for i, c := range header.Columns {
		want := sqlSourceColumns[i]
		if len(c) != 6 || c[0] == nil || c[1] == nil || c[2] == nil || c[4] == nil || *c[0] != want.name || *c[1] != want.kind || *c[2] != want.nullable {
			return nil, ErrSourceSchema
		}
		for _, v := range c {
			if v != nil && !utf8.ValidString(*v) {
				return nil, ErrSourceSchema
			}
		}
	}
	s.columns = header.Columns
	columnsRaw, err := json.Marshal(header.Columns)
	if err != nil {
		return nil, ErrSourceProtocol
	}
	s.columnsHash = sourceSHA(columnsRaw)
	sourceFrame(s.acc.h, []byte(s.columnsHash), false)
	if !expected.Boundary.Empty {
		raw, err := canonicalBase64(expected.Boundary.UpperToken)
		if err != nil {
			return nil, err
		}
		s.upper, err = positiveSQLID(raw)
		if err != nil {
			return nil, err
		}
	}
	return s, nil
}

func canonicalBase64(value string) ([]byte, error) {
	raw, err := base64.StdEncoding.Strict().DecodeString(value)
	if err != nil || base64.StdEncoding.EncodeToString(raw) != value {
		return nil, ErrSourceProtocol
	}
	return raw, nil
}

func positiveSQLID(raw []byte) (uint64, error) {
	n, err := strconv.ParseUint(string(raw), 10, 64)
	if err != nil || n == 0 || strconv.FormatUint(n, 10) != string(raw) {
		return 0, ErrSourceIdentity
	}
	return n, nil
}

func sqlTransportClock(raw []byte) bool {
	v, err := time.Parse("2006-01-02 15:04:05.999999999", string(raw))
	return err == nil && !v.IsZero() && v.Nanosecond()%1_000_000 == 0
}

func (s *SQLSourceReader) Next() (*DecodedSourceEvent, error) {
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
		return nil, err
	}
	return v, nil
}

func (s *SQLSourceReader) decodeLine(line []byte) (*DecodedSourceEvent, error) {
	if err := strictJSON(line); err != nil {
		return nil, err
	}
	var encoded []*string
	if json.Unmarshal(line, &encoded) != nil || len(encoded) != len(s.columns) {
		return nil, ErrSourceSchema
	}
	cells := make([][]byte, len(encoded))
	size := uint64(0)
	rowHash := sha256.New()
	sourceFrame(rowHash, []byte(s.columnsHash), false)
	for i, v := range encoded {
		if v == nil {
			if sqlSourceColumns[i].nullable != "YES" {
				return nil, ErrSourceSchema
			}
		} else {
			raw, err := canonicalBase64(*v)
			if err != nil {
				return nil, err
			}
			cells[i] = raw
			size += uint64(len(raw))
		}
		if size > MaxSourceRowBytes {
			return nil, ErrSourceBounds
		}
		sourceFrame(rowHash, cells[i], v == nil)
	}
	id, err := positiveSQLID(cells[0])
	if err != nil {
		return nil, err
	}
	if s.acc.expectation.Boundary.Empty || id > s.upper || id <= s.last {
		return nil, ErrSourceOrder
	}
	text := func(index int) string { return string(cells[index]) }
	for _, i := range []int{1, 2, 3, 4, 6, 8, 10, 12, 13, 14} {
		if cells[i] != nil && !utf8.Valid(cells[i]) {
			return nil, ErrSourceSchema
		}
	}
	if !identifier(text(1), 64) || !identifier(text(2), 128) || !identifier(text(3), 64) || !identifier(text(4), 64) || !identifier(text(6), 128) {
		return nil, ErrSourceIdentity
	}
	var org *int64
	if cells[5] != nil {
		n, err := strconv.ParseInt(text(5), 10, 64)
		if err != nil || strconv.FormatInt(n, 10) != text(5) {
			return nil, ErrSourceSchema
		}
		org = &n
	}
	attempt, err := strconv.ParseUint(text(9), 10, 32)
	if err != nil || strconv.FormatUint(attempt, 10) != text(9) {
		return nil, ErrSourceSchema
	}
	if !sourceStatus(text(8)) {
		return nil, ErrSourceSchema
	}
	for _, i := range []int{11, 15, 16, 17} {
		if cells[i] != nil && !sqlTransportClock(cells[i]) {
			return nil, ErrSourceSchema
		}
	}
	v, err := decodeDomain(cells[7], outerEventFacts{engine: "mysql", id: text(1), eventType: text(2), aggregateType: text(3), aggregateID: text(4), org: org})
	if err != nil {
		return nil, err
	}
	v.Source = evidence.HistoricalSourceReferenceV1{Database: "mysql", Object: "domain_event_outbox", PrimaryKeyKind: "mysql_uint64", PrimaryKeySHA256: sourceSHA(cells[0]), Digest: evidence.Digest{Kind: SQLRowDigestKind, SHA256: hex.EncodeToString(rowHash.Sum(nil))}}
	v.PrimaryKeyToken = base64.StdEncoding.EncodeToString(cells[0])
	v.Transport = TransportFacts{Status: text(8), AttemptCount: attempt, TopicName: text(6), Fields: map[string]*string{}}
	for _, i := range []int{10, 11, 12, 13, 14, 15, 16, 17} {
		if cells[i] == nil {
			v.Transport.Fields[sqlSourceColumns[i].name] = nil
		} else {
			copy := text(i)
			v.Transport.Fields[sqlSourceColumns[i].name] = &copy
		}
	}
	if err := s.acc.add(size); err != nil {
		return nil, err
	}
	if err := s.acc.identities.Observe(v); err != nil {
		return nil, err
	}
	for i, raw := range cells {
		sourceFrame(s.acc.h, raw, encoded[i] == nil)
	}
	s.last = id
	return v, nil
}

func sourceStatus(value string) bool {
	return value == "pending" || value == "publishing" || value == "published" || value == "failed"
}

func (s *SQLSourceReader) Receipt() SourceCopyReceipt { return s.acc.receipt(SQLSourceProtocol) }
