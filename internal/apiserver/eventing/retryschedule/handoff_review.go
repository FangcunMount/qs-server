package retryschedule

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/FangcunMount/reliable-messaging/transaction"
)

const handoffReviewLimit = 1 << 20

// HandoffReview freezes a bounded, single-primary maintenance selection. It is
// private evidence, not authorization: a host must separately verify stopped
// writers, source identity and approval before opening the original DB root.
// It contains original message bytes and must never be logged or served by HTTP.
type HandoffReview struct {
	document handoffReviewDocument
	plans    []Frozen
	raw      []byte
}

type handoffReviewDocument struct {
	Version   int                 `json:"version"`
	Binding   transaction.Binding `json:"binding"`
	SourceSHA string              `json:"source_sha"`
	Actor     string              `json:"actor"`
	Reason    string              `json:"reason"`
	Plans     []json.RawMessage   `json:"plans"`
}

// ReviewHandoff performs no I/O and does not claim that a legacy row is still
// eligible. TransferLegacy must recheck original facts, authorization and the
// database clock in the host root. No half-message or new business is planned.
func ReviewHandoff(binding transaction.Binding, sourceSHA, actor, reason string, plans []Frozen) (HandoffReview, error) {
	if !binding.Valid() || (binding.Owner != "qs.mysql" && binding.Owner != "qs.mongo") ||
		!canonicalHex(sourceSHA, 20) || !reviewText(actor, 128) || !reviewText(reason, 1024) ||
		len(plans) == 0 || len(plans) > 100 {
		return HandoffReview{}, ErrInvalid
	}
	d := handoffReviewDocument{Version: 1, Binding: binding, SourceSHA: sourceSHA, Actor: actor, Reason: reason}
	owned := make([]Frozen, 0, len(plans))
	seen := make(map[[32]byte]bool, len(plans))
	total := 0
	for _, plan := range plans {
		if !plan.Valid() || plan.Authority().Owner != binding.Owner || seen[plan.Key()] {
			return HandoffReview{}, ErrInvalid
		}
		seen[plan.Key()] = true
		raw := plan.Bytes()
		total += len(raw)
		if total > handoffReviewLimit {
			return HandoffReview{}, ErrInvalid
		}
		// Decode owns the first bytes even if future Frozen implementation changes.
		copy, err := Decode(raw)
		if err != nil {
			return HandoffReview{}, err
		}
		owned = append(owned, copy)
		d.Plans = append(d.Plans, json.RawMessage(raw))
	}
	raw, err := json.Marshal(d)
	if err != nil || len(raw) > handoffReviewLimit {
		return HandoffReview{}, ErrInvalid
	}
	return HandoffReview{document: d, plans: owned, raw: raw}, nil
}

// DecodeHandoffReview requires an independently supplied digest and canonical
// bytes. Duplicate/unknown fields and alternative encodings cannot be accepted
// as the reviewed file. The file cannot choose a live database connection.
func DecodeHandoffReview(raw []byte, expectedSHA string) (HandoffReview, error) {
	if len(raw) == 0 || len(raw) > handoffReviewLimit || !canonicalHex(expectedSHA, sha256.Size) {
		return HandoffReview{}, ErrInvalid
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != expectedSHA {
		return HandoffReview{}, ErrInvalid
	}
	var d handoffReviewDocument
	if json.Unmarshal(raw, &d) != nil || d.Version != 1 || len(d.Plans) == 0 || len(d.Plans) > 100 {
		return HandoffReview{}, ErrInvalid
	}
	plans := make([]Frozen, 0, len(d.Plans))
	for _, rawPlan := range d.Plans {
		plan, err := Decode(rawPlan)
		if err != nil {
			return HandoffReview{}, err
		}
		plans = append(plans, plan)
	}
	review, err := ReviewHandoff(d.Binding, d.SourceSHA, d.Actor, d.Reason, plans)
	if err != nil || !bytes.Equal(review.raw, raw) {
		return HandoffReview{}, ErrInvalid
	}
	return review, nil
}

func (r HandoffReview) Valid() bool                  { return len(r.raw) > 0 }
func (r HandoffReview) Bytes() []byte                { return bytes.Clone(r.raw) }
func (r HandoffReview) Digest() [32]byte             { return sha256.Sum256(r.raw) }
func (r HandoffReview) Binding() transaction.Binding { return r.document.Binding }
func (r HandoffReview) SourceSHA() string            { return r.document.SourceSHA }
func (r HandoffReview) Actor() string                { return r.document.Actor }
func (r HandoffReview) Reason() string               { return r.document.Reason }
func (r HandoffReview) Plans() []Frozen              { return append([]Frozen(nil), r.plans...) }

func canonicalHex(value string, size int) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == size && value == hex.EncodeToString(raw)
}

func reviewText(value string, limit int) bool {
	return utf8.ValidString(value) && strings.TrimSpace(value) != "" && len(value) <= limit &&
		strings.IndexFunc(value, unicode.IsControl) < 0
}
