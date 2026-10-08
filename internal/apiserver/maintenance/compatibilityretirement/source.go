// Package retirement decodes private historical inventory sources. Decoding
// proves neither business closure nor permission to retire an object.
package retirement

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	eventoutcome "github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
)

// SourceError is a fixed, payload-free category. Never include raw fields in an
// error returned by this package: its input can contain private message bodies.
type SourceError string

func (e SourceError) Error() string { return string(e) }

const (
	ErrSourceProtocol      SourceError = "source_protocol_rejected"
	ErrSourceSchema        SourceError = "source_schema_unsupported"
	ErrSourceJSON          SourceError = "source_json_invalid"
	ErrSourceDuplicateKey  SourceError = "source_json_duplicate_key"
	ErrSourceUnknownField  SourceError = "source_json_unknown_field"
	ErrSourcePrecision     SourceError = "source_numeric_precision_loss"
	ErrSourceIdentity      SourceError = "source_identity_conflict"
	ErrSourceOrganization  SourceError = "source_organization_conflict"
	ErrSourceDigest        SourceError = "source_digest_conflict"
	ErrSourceBSON          SourceError = "source_bson_invalid"
	ErrSourceBounds        SourceError = "source_bound_exceeded"
	ErrSourceOrder         SourceError = "source_primary_key_order_invalid"
	ErrSourceEventType     SourceError = "source_event_type_unsupported"
	ErrSourceIncomplete    SourceError = "source_copy_incomplete"
	ErrSourceSerialization SourceError = "private_decoded_event_serialization_forbidden"
)

const (
	SQLSourceProtocol   = "mysql_cast_binary_columns_pk_order_v2"
	MongoSourceProtocol = "mongodb_server_bson_pk_order_v2"
	SQLRowDigestKind    = "mysql-cast-binary-row-v2"
	MongoRowDigestKind  = "mongodb-server-bson-row-v2"
	ContentDigestKind   = "legacy-domain-json-bytes-v1"
	// This is a per-row memory ceiling, not a production scan budget.
	MaxSourceRowBytes = 32 << 20
	MaxSourceRecords  = 1_000_000
	MaxSourceBytes    = 2 << 30
)

// SourceBoundary mirrors the exact inventory v2 boundary. The caller must get
// the expected value from an independently approved private request/receipt,
// not from the source header being decoded.
type SourceBoundary struct {
	Database     string `json:"database"`
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	Present      bool   `json:"present"`
	Empty        bool   `json:"empty"`
	PKType       string `json:"pk_type"`
	UpperToken   string `json:"upper_token"`
	SchemaHash   string `json:"schema_hash"`
	IdentityHash string `json:"identity_hash"`
}

// SourceCopyExpectation is the *approved* inventory pass's exact byte/record
// counts and framing digest. A SourceReader verifies these only at clean EOF.
// Hosts must verify the whole private copy before applying any business CAS.
type SourceCopyExpectation struct {
	Boundary SourceBoundary
	DataHash string
	Records  uint64
	Bytes    uint64
}

type SourceCopyReceipt struct {
	Protocol                string `json:"protocol"`
	Records                 uint64 `json:"records"`
	Bytes                   uint64 `json:"source_bytes"`
	DataHash                string `json:"data_hash"`
	Complete                bool   `json:"complete"`
	BusinessClosureVerified bool   `json:"business_closure_verified"`
	DropReady               bool   `json:"drop_ready"`
}

type OriginalRunFacts struct {
	RunID   string
	Attempt *uint32
	Missing []string
}

// DecodedSourceEvent contains private, in-memory typed facts. It deliberately
// cannot be JSON-marshaled. Source/content digests are not SDK fingerprints or
// business bindings; this layer cannot produce either of those proofs.
type DecodedSourceEvent struct {
	Source           evidence.HistoricalSourceReferenceV1
	PrimaryKeyToken  string
	SupportedSchema  string
	ContentDigest    evidence.Digest
	EventID          string
	EventType        string
	AggregateType    string
	AggregateID      string
	OrgID            uint64
	OuterOrgID       *int64
	OccurredAt       time.Time
	BusinessAt       time.Time
	BusinessIDs      map[string]string
	OriginalRun      OriginalRunFacts
	ResolverGaps     []string
	Transport        TransportFacts
	Requested        *eventpayload.EvaluationRequestedData
	Failed           *eventpayload.EvaluationFailedData
	OutcomeCommitted *eventpayload.EvaluationOutcomeCommittedData
	Submitted        *eventpayload.AnswerSheetSubmittedData
	Generated        *eventoutcome.ReportGeneratedPayload
}

func (DecodedSourceEvent) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (DecodedSourceEvent) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (v DecodedSourceEvent) String() string {
	return "private decoded event (" + v.Source.Database + ", " + v.Source.Digest.SHA256 + ")"
}
func (v DecodedSourceEvent) GoString() string { return v.String() }

// TransportFacts preserves NULL/absent vs empty strings. It is not an ACK or a
// business-terminal decision. Original delivery mutation remains in Source.
type TransportFacts struct {
	Status       string
	AttemptCount uint64
	TopicName    string
	Fields       map[string]*string
}

// BusinessTimeEqual compares the representation stored in DATETIME(3)/BSON
// dates. It never overwrites the original event clock or claims those distinct
// event/business clocks were equal in the legacy producer.
func BusinessTimeEqual(a, b time.Time) bool {
	return !a.IsZero() && !b.IsZero() && a.UTC().Truncate(time.Millisecond).Equal(b.UTC().Truncate(time.Millisecond))
}

func sourceSHA(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }

// sourceFrame matches cmd paging.go/main.go exactly: NULL is nine zero bytes;
// non-NULL is 1 + big-endian uint64 length + bytes. Empty differs from NULL.
func sourceFrame(h hash.Hash, raw []byte, null bool) {
	var prefix [9]byte
	if !null {
		prefix[0] = 1
		binary.BigEndian.PutUint64(prefix[1:], uint64(len(raw)))
	}
	_, _ = h.Write(prefix[:]) // crypto hash Write never fails
	if !null {
		_, _ = h.Write(raw)
	}
}

func identifier(s string, max int) bool {
	if s == "" || len(s) > max || !utf8.ValidString(s) {
		return false
	}
	return !strings.ContainsFunc(s, func(r rune) bool { return r <= ' ' || r == 127 })
}

func validExpectation(e SourceCopyExpectation, engine string) error {
	b := e.Boundary
	kind := "base_table"
	if engine == "mongodb" {
		kind = "collection"
	}
	if b.Database != engine || b.Name != "domain_event_outbox" || b.Kind != kind || !b.Present ||
		!evidence.ValidSHA256(b.SchemaHash) || !evidence.ValidSHA256(b.IdentityHash) || !evidence.ValidSHA256(e.DataHash) ||
		len(b.UpperToken) > 1368 ||
		e.Records > MaxSourceRecords || e.Bytes > MaxSourceBytes || (b.Empty != (e.Records == 0)) ||
		(b.Empty && (b.UpperToken != "" || e.Bytes != 0)) || (!b.Empty && b.UpperToken == "") {
		return ErrSourceProtocol
	}
	return nil
}

type sourceAccumulator struct {
	expectation    SourceCopyExpectation
	h              hash.Hash
	records, bytes uint64
	done, failed   bool
	identities     SourceIdentityTracker
}

// SourceIdentityTracker can be shared across the two private source readers by
// a host verifier. Identical observations are idempotent; the same EventID with
// any changed source identity/digest or type is an explicit conflict. It holds
// only bounded identities/digests, never bodies or inferred business state.
type SourceIdentityTracker struct{ seen map[string]sourceObservation }
type sourceObservation struct {
	eventType string
	source    evidence.HistoricalSourceReferenceV1
}

func (t *SourceIdentityTracker) Observe(event *DecodedSourceEvent) error {
	if event == nil || !identifier(event.EventID, 64) || !evidence.ValidSHA256(event.Source.Digest.SHA256) {
		return ErrSourceIdentity
	}
	if t.seen == nil {
		t.seen = make(map[string]sourceObservation)
	}
	current := sourceObservation{eventType: event.EventType, source: event.Source}
	if prior, ok := t.seen[event.EventID]; ok {
		if prior != current {
			return ErrSourceDigest
		}
		return nil
	}
	if len(t.seen) >= MaxSourceRecords {
		return ErrSourceBounds
	}
	t.seen[event.EventID] = current
	return nil
}

func (s *sourceAccumulator) add(size uint64) error {
	if s.done || s.failed || s.records >= s.expectation.Records || s.bytes > s.expectation.Bytes || size > s.expectation.Bytes-s.bytes {
		s.failed = true
		return ErrSourceBounds
	}
	s.records++
	s.bytes += size
	return nil
}

func (s *sourceAccumulator) finish() error {
	if s.failed {
		return ErrSourceIncomplete
	}
	if s.records != s.expectation.Records || s.bytes != s.expectation.Bytes {
		s.failed = true
		return ErrSourceIncomplete
	}
	if hex.EncodeToString(s.h.Sum(nil)) != s.expectation.DataHash {
		s.failed = true
		return ErrSourceDigest
	}
	s.done = true
	return io.EOF
}

func (s *sourceAccumulator) receipt(protocol string) SourceCopyReceipt {
	return SourceCopyReceipt{Protocol: protocol, Records: s.records, Bytes: s.bytes, DataHash: hex.EncodeToString(s.h.Sum(nil)), Complete: s.done && !s.failed}
}
