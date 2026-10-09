package compatibilityretirementbackup

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	retirement "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirement"
	"go.mongodb.org/mongo-driver/bson"
	"hash"
	"io"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const metadataBudget = 32 << 20

func readerMissing(r io.Reader) bool {
	if r == nil {
		return true
	}
	v := reflect.ValueOf(r)
	return v.Kind() == reflect.Pointer && v.IsNil()
}
func readPrivate(r io.Reader, limit int64) ([]byte, error) {
	if readerMissing(r) {
		return nil, ErrPrivate
	}
	b, e := io.ReadAll(io.LimitReader(r, limit+1))
	if e != nil || int64(len(b)) > limit {
		return nil, ErrPrivate
	}
	return b, nil
}

// Duplicate JSON keys, case aliases and trailing tokens must not weaken an
// independently approved report or source header. Raw errors are discarded.
func exactJSON(raw []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 64 {
			return ErrSource
		}
		t, e := d.Token()
		if e != nil {
			return ErrSource
		}
		if delim, ok := t.(json.Delim); ok {
			switch delim {
			case '{':
				seen := map[string]bool{}
				for d.More() {
					k, e := d.Token()
					s, ok := k.(string)
					if e != nil || !ok || seen[s] || !utf8.ValidString(s) {
						return ErrSource
					}
					seen[s] = true
					if e = walk(depth + 1); e != nil {
						return e
					}
				}
			case '[':
				for d.More() {
					if e = walk(depth + 1); e != nil {
						return e
					}
				}
			default:
				return ErrSource
			}
			if _, e = d.Token(); e != nil {
				return ErrSource
			}
		}
		return nil
	}
	if e := walk(0); e != nil {
		return e
	}
	if _, e := d.Token(); e != io.EOF {
		return ErrSource
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		return ErrSource
	}
	return exactNames(raw, reflect.TypeOf(out).Elem())
}
func exactNames(raw []byte, t reflect.Type) error {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == reflect.TypeOf(time.Time{}) {
		return nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	switch t.Kind() {
	case reflect.Struct:
		var m map[string]json.RawMessage
		if json.Unmarshal(raw, &m) != nil {
			return ErrSource
		}
		allowed := map[string]reflect.Type{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			tag, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if tag == "-" {
				continue
			}
			if tag == "" {
				tag = f.Name
			}
			allowed[tag] = f.Type
		}
		for k, v := range m {
			ft, ok := allowed[k]
			if !ok {
				return ErrSource
			}
			if e := exactNames(v, ft); e != nil {
				return e
			}
		}
	case reflect.Array, reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return nil
		}
		var a []json.RawMessage
		if json.Unmarshal(raw, &a) != nil {
			return ErrSource
		}
		for _, v := range a {
			if e := exactNames(v, t.Elem()); e != nil {
				return e
			}
		}
	case reflect.Map:
		if t.Elem().Kind() == reflect.Struct {
			var m map[string]json.RawMessage
			if json.Unmarshal(raw, &m) != nil {
				return ErrSource
			}
			for _, v := range m {
				if e := exactNames(v, t.Elem()); e != nil {
					return e
				}
			}
		}
	}
	return nil
}
func base64Raw(s string) ([]byte, error) {
	b, e := base64.StdEncoding.Strict().DecodeString(s)
	if e != nil || base64.StdEncoding.EncodeToString(b) != s {
		return nil, ErrSource
	}
	return b, nil
}

type sqlHeader struct {
	Protocol string                    `json:"protocol"`
	Columns  retirement.SQLColumns     `json:"columns"`
	Boundary retirement.SourceBoundary `json:"boundary"`
}
type rawRow struct {
	cells [][]byte
	nulls []bool
	bson  bson.Raw
}
type rawReader struct {
	ctx           context.Context
	index         int
	expected      SourceSnapshot
	columns       retirement.SQLColumns
	scanner       *bufio.Scanner
	input         io.Reader
	hash          hash.Hash
	records, size uint64
	last          []byte
	lastID        uint64
	upperID       uint64
	upper         bson.RawValue
	done          bool
}

func supportedColumns(index int, columns retirement.SQLColumns) bool {
	layouts := [][]string{{"id", "event_id", "event_type", "aggregate_type", "aggregate_id", "org_id", "topic_name", "payload_json", "status", "attempt_count", "retry_disposition", "next_attempt_at", "last_error", "last_error_kind", "manual_replay_request_id", "created_at", "updated_at", "published_at"}, {"command_id", "request_id", "kind", "payload", "payload_hash", "delivered", "attempts", "available_at"}, {"command_id", "request_id", "source_kind", "source_payload", "source_payload_hash", "source_attempts", "source_available_at", "source_original_time", "messaging_body_sha256", "transferred_at"}}
	if index < 0 || index > 2 || len(columns) != len(layouts[index]) {
		return false
	}
	for i, c := range columns {
		if len(c) != 6 || c[0] == nil || *c[0] != layouts[index][i] || c[1] == nil || c[2] == nil || c[4] == nil {
			return false
		}
		for _, v := range c {
			if v != nil && !utf8.ValidString(*v) {
				return false
			}
		}
	}
	return true
}
func newRawReader(ctx context.Context, r io.Reader, index int, expected SourceSnapshot, columns retirement.SQLColumns) (*rawReader, error) {
	if ctx == nil || ctx.Err() != nil || readerMissing(r) || index < 0 || index > 3 {
		return nil, ErrSource
	}
	b := expected.Boundary
	if !expected.Present || !expected.Complete || expected.NextCycle || expected.Passes != 2 || !b.Present || b.Empty != (expected.Records == 0) || expected.Records > retirement.MaxSourceRecords || expected.Bytes > retirement.MaxSourceBytes || !hashPattern.MatchString(expected.DataHash) || b.IdentityHash != expected.IdentityHash || b.SchemaHash != expected.SchemaHash || (b.Empty && (b.UpperToken != "" || expected.Bytes != 0)) || (!b.Empty && b.UpperToken == "") {
		return nil, ErrSource
	}
	x := &rawReader{ctx: ctx, index: index, expected: expected, columns: columns, input: r, hash: sha256.New()}
	if index < 3 {
		x.scanner = bufio.NewScanner(r)
		x.scanner.Buffer(make([]byte, 64<<10), 2*retirement.MaxSourceRowBytes)
		if !x.scanner.Scan() {
			return nil, ErrSource
		}
		var hdr sqlHeader
		if exactJSON(x.scanner.Bytes(), &hdr) != nil || hdr.Protocol != retirement.SQLSourceProtocol || !reflect.DeepEqual(hdr.Columns, columns) || !reflect.DeepEqual(hdr.Boundary, b) || !supportedColumns(index, columns) {
			return nil, ErrStructure
		}
		frame(x.hash, []byte(jsonSHA(columns)), false)
		if !b.Empty {
			upper, e := base64Raw(b.UpperToken)
			if e != nil {
				return nil, e
			}
			x.last = nil
			if index == 0 {
				x.upperID, e = strconv.ParseUint(string(upper), 10, 64)
				if e != nil || x.upperID == 0 || strconv.FormatUint(x.upperID, 10) != string(upper) {
					return nil, ErrSource
				}
			} else {
				x.last = nil
			}
		}
	} else if !b.Empty {
		upper, e := base64Raw(b.UpperToken)
		if e != nil {
			return nil, e
		}
		doc := bson.Raw(upper)
		if doc.Validate() != nil {
			return nil, ErrSource
		}
		es, e := doc.Elements()
		if e != nil || len(es) != 1 || es[0].Key() != "_id" {
			return nil, ErrSource
		}
		x.upper = es[0].Value()
		if pkKind(x.upper) != b.PKType {
			return nil, ErrSource
		}
	}
	return x, nil
}
func (r *rawReader) next() (*rawRow, error) {
	if r.done {
		return nil, io.EOF
	}
	if r.ctx.Err() != nil {
		return nil, ErrBudget
	}
	row := &rawRow{}
	var size uint64
	if r.index < 3 {
		if !r.scanner.Scan() {
			if r.scanner.Err() != nil {
				return nil, ErrSource
			}
			return nil, r.finish()
		}
		var enc []*string
		if exactJSON(r.scanner.Bytes(), &enc) != nil || len(enc) != len(r.columns) {
			return nil, ErrSource
		}
		row.cells = make([][]byte, len(enc))
		row.nulls = make([]bool, len(enc))
		for i, v := range enc {
			row.nulls[i] = v == nil
			if v == nil {
				if *r.columns[i][2] != "YES" {
					return nil, ErrSource
				}
			} else {
				b, e := base64Raw(*v)
				if e != nil {
					return nil, e
				}
				row.cells[i] = b
				size += uint64(len(b))
			}
			if size > retirement.MaxSourceRowBytes {
				return nil, ErrSource
			}
			frame(r.hash, row.cells[i], v == nil)
		}
		if row.nulls[0] || r.expected.Boundary.Empty {
			return nil, ErrSource
		}
		if r.index == 0 {
			id, e := strconv.ParseUint(string(row.cells[0]), 10, 64)
			if e != nil || id == 0 || strconv.FormatUint(id, 10) != string(row.cells[0]) || id <= r.lastID || id > r.upperID {
				return nil, ErrSource
			}
			r.lastID = id
		} else {
			upper, e := base64Raw(r.expected.Boundary.UpperToken)
			if e != nil || len(row.cells[0]) == 0 || bytes.Compare(row.cells[0], upper) > 0 || (r.last != nil && bytes.Compare(row.cells[0], r.last) <= 0) {
				return nil, ErrSource
			}
			r.last = append([]byte(nil), row.cells[0]...)
		}
	} else {
		var length [8]byte
		n, e := io.ReadFull(r.input, length[:])
		if e == io.EOF && n == 0 {
			return nil, r.finish()
		}
		if e != nil {
			return nil, ErrSource
		}
		size = binary.BigEndian.Uint64(length[:])
		if size < 5 || size > 16<<20 {
			return nil, ErrSource
		}
		row.bson = make(bson.Raw, size)
		if _, e = io.ReadFull(r.input, row.bson); e != nil || row.bson.Validate() != nil {
			return nil, ErrSource
		}
		es, e := row.bson.Elements()
		if e != nil {
			return nil, ErrSource
		}
		seen := map[string]bool{}
		for _, v := range es {
			if seen[v.Key()] || !utf8.ValidString(v.Key()) {
				return nil, ErrSource
			}
			seen[v.Key()] = true
		}
		id := row.bson.Lookup("_id")
		if pkKind(id) != r.expected.Boundary.PKType || r.expected.Boundary.Empty {
			return nil, ErrSource
		}
		cmp, e := comparePK(id, r.upper)
		if e != nil || cmp > 0 {
			return nil, ErrSource
		}
		if r.last != nil {
			prior := bson.Raw(r.last).Lookup("_id")
			cmp, e = comparePK(id, prior)
			if e != nil || cmp <= 0 {
				return nil, ErrSource
			}
		}
		token, e := bson.Marshal(bson.D{{Key: "_id", Value: id}})
		if e != nil {
			return nil, ErrSource
		}
		r.last = token
		frame(r.hash, row.bson, false)
	}
	if r.records >= r.expected.Records || r.size > r.expected.Bytes || size > r.expected.Bytes-r.size {
		return nil, ErrSource
	}
	r.records++
	r.size += size
	return row, nil
}
func (r *rawReader) finish() error {
	if r.records != r.expected.Records || r.size != r.expected.Bytes || hex.EncodeToString(r.hash.Sum(nil)) != r.expected.DataHash {
		return ErrSource
	}
	r.done = true
	return io.EOF
}
func pkKind(v bson.RawValue) string {
	switch v.Type {
	case bson.TypeObjectID:
		return "objectId"
	case bson.TypeString:
		if v.StringValue() != "" && utf8.ValidString(v.StringValue()) {
			return "string"
		}
	case bson.TypeInt32:
		return "int"
	case bson.TypeInt64:
		return "long"
	}
	return ""
}
func comparePK(a, b bson.RawValue) (int, error) {
	if a.Type != b.Type || pkKind(a) == "" {
		return 0, ErrSource
	}
	switch a.Type {
	case bson.TypeObjectID:
		return bytes.Compare(a.Value, b.Value), nil
	case bson.TypeString:
		return bytes.Compare([]byte(a.StringValue()), []byte(b.StringValue())), nil
	case bson.TypeInt32:
		if a.Int32() < b.Int32() {
			return -1, nil
		}
		if a.Int32() > b.Int32() {
			return 1, nil
		}
	case bson.TypeInt64:
		if a.Int64() < b.Int64() {
			return -1, nil
		}
		if a.Int64() > b.Int64() {
			return 1, nil
		}
	}
	return 0, nil
}
