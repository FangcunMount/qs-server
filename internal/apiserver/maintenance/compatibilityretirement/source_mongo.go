package retirement

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"io"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsontype"
)

type MongoSourceReader struct {
	input       io.Reader
	acc         sourceAccumulator
	upper, last bson.RawValue
	hasLast     bool
}

func NewMongoSourceReader(input io.Reader, expected SourceCopyExpectation) (*MongoSourceReader, error) {
	if input == nil || validExpectation(expected, "mongodb") != nil {
		return nil, ErrSourceProtocol
	}
	s := &MongoSourceReader{input: input, acc: sourceAccumulator{expectation: expected, h: sha256.New()}}
	if expected.Boundary.Empty {
		if expected.Boundary.PKType != "" {
			return nil, ErrSourceProtocol
		}
		return s, nil
	}
	raw, err := canonicalBase64(expected.Boundary.UpperToken)
	if err != nil {
		return nil, err
	}
	fields, err := exactBSONFields(raw)
	if err != nil || len(fields) != 1 {
		return nil, ErrSourceProtocol
	}
	v, ok := fields["_id"]
	if !ok || mongoPKType(v) == "" || mongoPKType(v) != expected.Boundary.PKType {
		return nil, ErrSourceProtocol
	}
	s.upper = v
	return s, nil
}

func exactBSONFields(raw []byte) (map[string]bson.RawValue, error) {
	doc := bson.Raw(raw)
	if len(raw) < 5 || int64(int32(binary.LittleEndian.Uint32(raw[:4]))) != int64(len(raw)) || doc.Validate() != nil {
		return nil, ErrSourceBSON
	}
	es, err := doc.Elements()
	if err != nil {
		return nil, ErrSourceBSON
	}
	fields := make(map[string]bson.RawValue, len(es))
	for _, element := range es {
		name := element.Key()
		if !utf8.ValidString(name) {
			return nil, ErrSourceBSON
		}
		if _, duplicate := fields[name]; duplicate {
			return nil, ErrSourceBSON
		}
		fields[name] = element.Value()
	}
	return fields, nil
}

func mongoPKType(v bson.RawValue) string {
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

// compareMongoPK matches inventory's one-type, simple-collation ordered _id
// predicate. Values never cross BSON types or turn numeric IDs into floats.
func compareMongoPK(a, b bson.RawValue) (int, error) {
	if mongoPKType(a) == "" || a.Type != b.Type {
		return 0, ErrSourceSchema
	}
	switch a.Type {
	case bson.TypeObjectID:
		return bytes.Compare(a.Value, b.Value), nil
	case bson.TypeString:
		return bytes.Compare([]byte(a.StringValue()), []byte(b.StringValue())), nil
	case bson.TypeInt32:
		left, right := a.Int32(), b.Int32()
		if left < right {
			return -1, nil
		}
		if left > right {
			return 1, nil
		}
	case bson.TypeInt64:
		left, right := a.Int64(), b.Int64()
		if left < right {
			return -1, nil
		}
		if left > right {
			return 1, nil
		}
	default:
		return 0, ErrSourceSchema
	}
	return 0, nil
}

func (s *MongoSourceReader) Next() (*DecodedSourceEvent, error) {
	if s.acc.done {
		return nil, io.EOF
	}
	if s.acc.failed {
		return nil, ErrSourceIncomplete
	}
	var prefix [8]byte
	n, err := io.ReadFull(s.input, prefix[:])
	if err == io.EOF && n == 0 {
		return nil, s.acc.finish()
	}
	if err != nil {
		s.acc.failed = true
		return nil, ErrSourceIncomplete
	}
	size := binary.BigEndian.Uint64(prefix[:])
	if size < 5 || size > 16<<20 || s.acc.records >= s.acc.expectation.Records || s.acc.bytes > s.acc.expectation.Bytes || size > s.acc.expectation.Bytes-s.acc.bytes {
		s.acc.failed = true
		return nil, ErrSourceBounds
	}
	raw := make([]byte, int(size))
	if _, err := io.ReadFull(s.input, raw); err != nil {
		s.acc.failed = true
		return nil, ErrSourceIncomplete
	}
	v, err := s.decodeRaw(raw)
	if err != nil {
		s.acc.failed = true
		return nil, err
	}
	if err := s.acc.add(size); err != nil {
		return nil, err
	}
	if err := s.acc.identities.Observe(v); err != nil {
		s.acc.failed = true
		return nil, err
	}
	sourceFrame(s.acc.h, raw, false)
	return v, nil
}

var mongoRequired = map[string]bsontype.Type{"event_id": bson.TypeString, "event_type": bson.TypeString, "aggregate_type": bson.TypeString, "aggregate_id": bson.TypeString, "topic_name": bson.TypeString, "payload_json": bson.TypeString, "status": bson.TypeString, "next_attempt_at": bson.TypeDateTime, "created_at": bson.TypeDateTime, "updated_at": bson.TypeDateTime}
var mongoOptional = map[string]bsontype.Type{"org_id": bson.TypeInt64, "retry_disposition": bson.TypeString, "last_error": bson.TypeString, "last_error_kind": bson.TypeString, "manual_replay_request_id": bson.TypeString, "published_at": bson.TypeDateTime, "claim_token": bson.TypeString}

func (s *MongoSourceReader) decodeRaw(raw []byte) (*DecodedSourceEvent, error) {
	fields, err := exactBSONFields(raw)
	if err != nil {
		return nil, err
	}
	for name, kind := range mongoRequired {
		v, ok := fields[name]
		if !ok || v.Type != kind {
			return nil, ErrSourceSchema
		}
	}
	for name, v := range fields {
		if name == "_id" || name == "attempt_count" {
			continue
		}
		kind, ok := mongoRequired[name]
		if !ok {
			kind, ok = mongoOptional[name]
		}
		if !ok {
			return nil, ErrSourceUnknownField
		}
		if kind != v.Type {
			return nil, ErrSourceSchema
		}
		if kind == bson.TypeString && !utf8.ValidString(v.StringValue()) {
			return nil, ErrSourceBSON
		}
	}
	pk, ok := fields["_id"]
	if !ok || mongoPKType(pk) != s.acc.expectation.Boundary.PKType || s.acc.expectation.Boundary.Empty {
		return nil, ErrSourceSchema
	}
	order, err := compareMongoPK(pk, s.upper)
	if err != nil || order > 0 {
		return nil, ErrSourceOrder
	}
	if s.hasLast {
		order, err = compareMongoPK(pk, s.last)
		if err != nil || order <= 0 {
			return nil, ErrSourceOrder
		}
	}
	attemptRaw, ok := fields["attempt_count"]
	if !ok {
		return nil, ErrSourceSchema
	}
	var attempt int64
	switch attemptRaw.Type {
	case bson.TypeInt32:
		attempt = int64(attemptRaw.Int32())
	case bson.TypeInt64:
		attempt = attemptRaw.Int64()
	default:
		return nil, ErrSourceSchema
	}
	if attempt < 0 {
		return nil, ErrSourceSchema
	}
	text := func(name string) string { return fields[name].StringValue() }
	if !identifier(text("event_id"), 64) || !identifier(text("event_type"), 128) || !identifier(text("aggregate_type"), 64) || !identifier(text("aggregate_id"), 64) || !identifier(text("topic_name"), 128) || !sourceStatus(text("status")) {
		return nil, ErrSourceIdentity
	}
	var org *int64
	if v, ok := fields["org_id"]; ok {
		n := v.Int64()
		org = &n
	}
	v, err := decodeDomain([]byte(text("payload_json")), outerEventFacts{engine: "mongodb", id: text("event_id"), eventType: text("event_type"), aggregateType: text("aggregate_type"), aggregateID: text("aggregate_id"), org: org})
	if err != nil {
		return nil, err
	}
	// RawValue preserves the original scalar bytes; no Extended JSON or scalar
	// coercion is used to create the one-field BSON inventory token.
	token, err := bson.Marshal(bson.D{{Key: "_id", Value: pk}})
	if err != nil {
		return nil, ErrSourceBSON
	}
	rowHash := sha256.New()
	sourceFrame(rowHash, raw, false)
	kind := "mongodb_raw_bson_v1"
	if pk.Type == bson.TypeObjectID {
		kind = "mongodb_objectid"
	}
	v.Source = evidence.HistoricalSourceReferenceV1{Database: "mongodb", Object: "domain_event_outbox", PrimaryKeyKind: kind, PrimaryKeySHA256: sourceSHA(token), Digest: evidence.Digest{Kind: MongoRowDigestKind, SHA256: hex.EncodeToString(rowHash.Sum(nil))}}
	v.PrimaryKeyToken = base64.StdEncoding.EncodeToString(token)
	v.Transport = TransportFacts{Status: text("status"), AttemptCount: uint64(attempt), TopicName: text("topic_name"), Fields: map[string]*string{}}
	for _, name := range []string{"retry_disposition", "last_error", "last_error_kind", "manual_replay_request_id", "claim_token", "next_attempt_at", "created_at", "updated_at", "published_at"} {
		field, ok := fields[name]
		if !ok {
			v.Transport.Fields[name] = nil
			continue
		}
		value := ""
		if field.Type == bson.TypeString {
			value = field.StringValue()
		} else {
			value = time.UnixMilli(field.DateTime()).UTC().Format(time.RFC3339Nano)
		}
		v.Transport.Fields[name] = &value
	}
	s.last = bson.RawValue{Type: pk.Type, Value: append([]byte(nil), pk.Value...)}
	s.hasLast = true
	return v, nil
}

func (s *MongoSourceReader) Receipt() SourceCopyReceipt { return s.acc.receipt(MongoSourceProtocol) }

// ValidateSourceRowDigest lets the future resolver bind an independently
// recorded row digest without conflating it with content/binding/fingerprint.
func ValidateSourceRowDigest(event *DecodedSourceEvent, expected evidence.Digest) error {
	if event == nil || !evidence.ValidSHA256(expected.SHA256) || expected != event.Source.Digest {
		return ErrSourceDigest
	}
	return nil
}

// Keep scalar decimal formatting explicit for host adapters and tests.
func SourceSQLIDToken(id uint64) string {
	return base64.StdEncoding.EncodeToString([]byte(strconv.FormatUint(id, 10)))
}
