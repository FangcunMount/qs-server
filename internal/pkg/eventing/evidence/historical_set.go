package evidence

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"unicode/utf8"
)

const (
	HistoricalReferenceMaxEntries = 128
	HistoricalReferenceMaxBytes   = 512 * 1024
)

var (
	ErrHistoricalReferenceInvalid  = errors.New("invalid historical reference")
	ErrHistoricalReferenceConflict = errors.New("conflicting historical reference")
	ErrHistoricalReferenceLimit    = errors.New("historical reference limit exceeded")
	historicalOperationID          = regexp.MustCompile(`^[0-9]{1,20}-[0-9]{1,4}$`)
)

// HistoricalSourceReferenceV1 identifies original bytes without retaining a
// message body or an arbitrary BSON primary-key value.
type HistoricalSourceReferenceV1 struct {
	Database         string `json:"database" bson:"database"`
	Object           string `json:"object" bson:"object"`
	PrimaryKeyKind   string `json:"primary_key_kind" bson:"primary_key_kind"`
	PrimaryKeySHA256 string `json:"primary_key_sha256" bson:"primary_key_sha256"`
	Digest           Digest `json:"digest" bson:"digest"`
}

type HistoricalRunReferenceV1 struct {
	RunID   string `json:"run_id" bson:"run_id"`
	Attempt uint   `json:"attempt" bson:"attempt"`
}

type HistoricalReferenceEntryV1 struct {
	EventID   string                      `json:"event_id" bson:"event_id"`
	EventType string                      `json:"event_type" bson:"event_type"`
	Source    HistoricalSourceReferenceV1 `json:"source" bson:"source"`
	Proof     *EventEvidenceV1            `json:"proof" bson:"proof"`
	Run       *HistoricalRunReferenceV1   `json:"run,omitempty" bson:"run,omitempty"`
}

// HistoricalReferenceSetV1 is a business-record-local collection of old source
// conclusions. It never replaces a standard single-event reference. Callers
// must independently verify business ownership, closure and source coverage.
type HistoricalReferenceSetV1 struct {
	Version int                          `json:"version" bson:"version"`
	Entries []HistoricalReferenceEntryV1 `json:"entries" bson:"entries"`
}

func historicalText(s string, max int, required bool) bool {
	return (!required || s != "") && len(s) <= max && utf8.ValidString(s)
}

func historicalIdentifier(s string, max int) bool {
	if !historicalText(s, max, true) {
		return false
	}
	for _, r := range s {
		if r <= ' ' || r == 127 {
			return false
		}
	}
	return true
}

func (e HistoricalReferenceEntryV1) Validate() error {
	engine := ""
	switch e.EventType {
	case "evaluation.requested", "evaluation.retry.requested", "evaluation.failed", "evaluation.outcome.committed":
		engine = "mysql"
	case "answersheet.submitted", "interpretation.report.generated":
		engine = "mongodb"
	default:
		return ErrHistoricalReferenceInvalid
	}
	if !historicalIdentifier(e.EventID, 128) || e.Source.Database != engine || e.Source.Object != "domain_event_outbox" ||
		!ValidSHA256(e.Source.PrimaryKeySHA256) || !historicalIdentifier(e.Source.Digest.Kind, 64) ||
		e.Source.Digest.Kind == SDKFingerprintKind || !ValidSHA256(e.Source.Digest.SHA256) {
		return ErrHistoricalReferenceInvalid
	}
	if (engine == "mysql" && e.Source.PrimaryKeyKind != "mysql_uint64") ||
		(engine == "mongodb" && e.Source.PrimaryKeyKind != "mongodb_objectid" && e.Source.PrimaryKeyKind != "mongodb_raw_bson_v1") {
		return ErrHistoricalReferenceInvalid
	}
	p := e.Proof
	if p == nil || (p.Class != RetiredVerified && p.Class != Unverifiable) || p.EventID != e.EventID || p.Digest != e.Source.Digest ||
		p.Origin != "retirement" || !historicalIdentifier(p.Verification.Method, 128) || !historicalIdentifier(p.Verification.Version, 64) ||
		!historicalOperationID.MatchString(p.Verification.OperationID) || !historicalText(p.Verification.Reason, 4096, false) {
		return ErrHistoricalReferenceInvalid
	}
	// BSON stores dates at millisecond precision. A narrower new contract keeps
	// JSON/BSON round trips and exact-proof idempotency equivalent.
	_, offset := p.Verification.VerifiedAt.Zone()
	if offset != 0 || p.Verification.VerifiedAt.Nanosecond()%1_000_000 != 0 {
		return ErrHistoricalReferenceInvalid
	}
	if err := p.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrHistoricalReferenceInvalid, err)
	}
	if e.Run != nil && (!historicalIdentifier(e.Run.RunID, 128) || e.Run.Attempt == 0 || uint64(e.Run.Attempt) > uint64(^uint32(0))) {
		return ErrHistoricalReferenceInvalid
	}
	return nil
}

func (e HistoricalReferenceEntryV1) Clone() HistoricalReferenceEntryV1 {
	out := e
	out.Proof = e.Proof.Clone()
	if e.Run != nil {
		run := *e.Run
		out.Run = &run
	}
	if out.Proof != nil {
		out.Proof.Verification.VerifiedAt = out.Proof.Verification.VerifiedAt.UTC().Round(0)
	}
	return out
}

func (s *HistoricalReferenceSetV1) Clone() *HistoricalReferenceSetV1 {
	if s == nil {
		return nil
	}
	out := &HistoricalReferenceSetV1{Version: s.Version, Entries: make([]HistoricalReferenceEntryV1, len(s.Entries))}
	for i, entry := range s.Entries {
		out.Entries[i] = entry.Clone()
	}
	return out
}

func (s *HistoricalReferenceSetV1) Validate() error {
	if s == nil || s.Version != 1 || len(s.Entries) == 0 {
		return ErrHistoricalReferenceInvalid
	}
	if len(s.Entries) > HistoricalReferenceMaxEntries {
		return ErrHistoricalReferenceLimit
	}
	seen := make(map[string]struct{}, len(s.Entries))
	for _, entry := range s.Entries {
		if err := entry.Validate(); err != nil {
			return err
		}
		if _, exists := seen[entry.EventID]; exists {
			return ErrHistoricalReferenceConflict
		}
		seen[entry.EventID] = struct{}{}
	}
	b, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("%w: encoding", ErrHistoricalReferenceInvalid)
	}
	if len(b) > HistoricalReferenceMaxBytes {
		return ErrHistoricalReferenceLimit
	}
	return nil
}

// Append returns a defensive copy. An identical entry is idempotent; the same
// event ID with any changed source, type, run or proof is a conflict.
func (s *HistoricalReferenceSetV1) Append(entries ...HistoricalReferenceEntryV1) (*HistoricalReferenceSetV1, error) {
	if len(entries) > HistoricalReferenceMaxEntries {
		return nil, ErrHistoricalReferenceLimit
	}
	if s != nil {
		if err := s.Validate(); err != nil {
			return nil, err
		}
	}
	out := s.Clone()
	if out == nil {
		out = &HistoricalReferenceSetV1{Version: 1}
	}
	seen := make(map[string]HistoricalReferenceEntryV1, len(out.Entries))
	for _, entry := range out.Entries {
		seen[entry.EventID] = entry
	}
	for _, entry := range entries {
		if err := entry.Validate(); err != nil {
			return nil, err
		}
		entry = entry.Clone()
		if previous, exists := seen[entry.EventID]; exists {
			if !reflect.DeepEqual(previous, entry) {
				return nil, ErrHistoricalReferenceConflict
			}
			continue
		}
		out.Entries = append(out.Entries, entry)
		seen[entry.EventID] = entry
	}
	return out, out.Validate()
}

// DecodeHistoricalReferenceSetJSON rejects unknown fields, duplicate keys and
// trailing documents before accepting an untrusted persisted set.
func DecodeHistoricalReferenceSetJSON(raw []byte) (*HistoricalReferenceSetV1, error) {
	if len(raw) == 0 || len(raw) > HistoricalReferenceMaxBytes {
		return nil, ErrHistoricalReferenceLimit
	}
	keys := json.NewDecoder(bytes.NewReader(raw))
	if err := historicalJSONValue(keys, 0); err != nil {
		return nil, ErrHistoricalReferenceInvalid
	}
	if _, err := keys.Token(); err != io.EOF {
		return nil, ErrHistoricalReferenceInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var result HistoricalReferenceSetV1
	if err := decoder.Decode(&result); err != nil {
		return nil, ErrHistoricalReferenceInvalid
	}
	return &result, result.Validate()
}

func historicalJSONValue(d *json.Decoder, depth int) error {
	if depth > 32 {
		return ErrHistoricalReferenceInvalid
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	delimiter, compound := token.(json.Delim)
	if !compound {
		return nil
	}
	seen := map[string]bool{}
	for d.More() {
		if delimiter == '{' {
			key, e := d.Token()
			name, ok := key.(string)
			if e != nil || !ok || seen[name] {
				return ErrHistoricalReferenceInvalid
			}
			seen[name] = true
		}
		if err := historicalJSONValue(d, depth+1); err != nil {
			return err
		}
	}
	end, err := d.Token()
	if err != nil || (delimiter != '{' && delimiter != '[') || (delimiter == '{' && end != json.Delim('}')) || (delimiter == '[' && end != json.Delim(']')) {
		return ErrHistoricalReferenceInvalid
	}
	return nil
}
