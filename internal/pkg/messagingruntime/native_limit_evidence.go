package messagingruntime

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sort"
)

var ErrNativeLimitEvidence = errors.New("exact read-only native transaction-limit evidence required")

// NativeLimitEvidence is a deny-only audit reference. It proves neither a
// business outcome nor operator authorization. A recorder must use its own
// configured primary binding; the report cannot select a database or owner.
type NativeLimitEvidence struct {
	digest  [32]byte
	indices [][32]byte
}

func (e NativeLimitEvidence) valid() bool {
	if e.digest == [32]byte{} || len(e.indices) == 0 || len(e.indices) > 64 {
		return false
	}
	seen := map[[32]byte]bool{}
	for _, index := range e.indices {
		if index == [32]byte{} || seen[index] {
			return false
		}
		seen[index] = true
	}
	return true
}
func (e NativeLimitEvidence) Digest() [32]byte { return e.digest }
func (e NativeLimitEvidence) IndexCount() int  { return len(e.indices) }

type nativeLimitIndex struct {
	MessageID       string `json:"broker_message_id"`
	QueueID         *int64 `json:"queue_id"`
	QueueOffset     *int64 `json:"queue_offset"`
	CommitlogOffset *int64 `json:"commitlog_offset"`
	IndexHour       *int64 `json:"index_hour"`
}
type nativeLimitReport struct {
	Topic        string             `json:"topic"`
	KeySHA       string             `json:"key_sha256"`
	Records      []nativeLimitIndex `json:"records"`
	AdminSHA     string             `json:"raw_admin_sha256"`
	ReadOnly     *bool              `json:"read_only"`
	BodyExported *bool              `json:"body_exported"`
	Truncated    *bool              `json:"truncated_possible"`
	Alert        string             `json:"alert"`
	Severity     string             `json:"severity"`
}

// uniqueAuditJSON rejects duplicate fields at every level before typed decode.
// The report has only object/array/index depth; no unbounded recursive input.
func uniqueAuditJSON(d *json.Decoder, depth int) error {
	if depth > 4 {
		return ErrNativeLimitEvidence
	}
	token, err := d.Token()
	if err != nil {
		return ErrNativeLimitEvidence
	}
	if delimiter, ok := token.(json.Delim); ok {
		switch delimiter {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				key, e := d.Token()
				if e != nil {
					return ErrNativeLimitEvidence
				}
				field, ok := key.(string)
				if !ok || seen[field] {
					return ErrNativeLimitEvidence
				}
				seen[field] = true
				if uniqueAuditJSON(d, depth+1) != nil {
					return ErrNativeLimitEvidence
				}
			}
			end, e := d.Token()
			if e != nil || end != json.Delim('}') {
				return ErrNativeLimitEvidence
			}
		case '[':
			for d.More() {
				if uniqueAuditJSON(d, depth+1) != nil {
					return ErrNativeLimitEvidence
				}
			}
			end, e := d.Token()
			if e != nil || end != json.Delim(']') {
				return ErrNativeLimitEvidence
			}
		default:
			return ErrNativeLimitEvidence
		}
	}
	return nil
}

// ParseNativeLimitEvidence preserves the digest of the exact private export.
// The expected digest must be supplied independently by the host's controlled
// audit/handoff, not reconstructed by JSON remarshal or taken from this report.
func ParseNativeLimitEvidence(raw []byte, expected [32]byte) (NativeLimitEvidence, error) {
	reject := func() (NativeLimitEvidence, error) { return NativeLimitEvidence{}, ErrNativeLimitEvidence }
	if len(raw) == 0 || len(raw) > 65536 || expected == [32]byte{} || sha256.Sum256(raw) != expected {
		return reject()
	}
	check := json.NewDecoder(bytes.NewReader(raw))
	check.UseNumber()
	if uniqueAuditJSON(check, 0) != nil {
		return reject()
	}
	if _, err := check.Token(); err != io.EOF {
		return reject()
	}
	var report nativeLimitReport
	decode := json.NewDecoder(bytes.NewReader(raw))
	decode.DisallowUnknownFields()
	if decode.Decode(&report) != nil || report.Topic != "TRANS_CHECK_MAX_TIME_TOPIC" || report.Alert != "native_transaction_check_limit_reached" || report.Severity != "critical" ||
		report.ReadOnly == nil || !*report.ReadOnly || report.BodyExported == nil || *report.BodyExported || report.Truncated == nil ||
		len(report.Records) < 1 || len(report.Records) > 64 || *report.Truncated != (len(report.Records) == 64) {
		return reject()
	}
	for _, value := range []string{report.KeySHA, report.AdminSHA} {
		b, err := hex.DecodeString(value)
		if err != nil || len(b) != 32 || value != hex.EncodeToString(b) {
			return reject()
		}
	}
	indices := make([][32]byte, 0, len(report.Records))
	keyHash, _ := hex.DecodeString(report.KeySHA)
	for _, row := range report.Records {
		if len(row.MessageID) < 32 || len(row.MessageID) > 64 {
			return reject()
		}
		id, err := hex.DecodeString(row.MessageID)
		if err != nil {
			return reject()
		}
		for _, number := range []*int64{row.QueueID, row.QueueOffset, row.CommitlogOffset, row.IndexHour} {
			if number == nil || *number < 0 {
				return reject()
			}
		}

		h := sha256.New()
		_, _ = h.Write([]byte("m3-native-limit-index-v1\x00"))
		for _, field := range [][]byte{[]byte(report.Topic), keyHash, id} {
			nativeNoticeField(h, field)
		}
		for _, number := range []*int64{row.QueueID, row.QueueOffset, row.CommitlogOffset, row.IndexHour} {
			var value [8]byte
			binary.BigEndian.PutUint64(value[:], uint64(*number))
			_, _ = h.Write(value[:])
		}
		var fingerprint [32]byte
		copy(fingerprint[:], h.Sum(nil))
		indices = append(indices, fingerprint)
	}
	sort.Slice(indices, func(i, j int) bool { return bytes.Compare(indices[i][:], indices[j][:]) < 0 })
	// A client ID can survive a native copy. Deduplicate the full frozen
	// index fingerprint; keep its v1 framing and every existing notice key.
	evidence := NativeLimitEvidence{digest: expected, indices: indices}
	if !evidence.valid() {
		return reject()
	}
	return evidence, nil
}
