// Package evidence holds immutable event references and explicitly bounded
// historical verification conclusions. It never retains a message body.
package evidence

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Class string

const (
	StandardReferenceClass Class = "standard_reference"
	RetiredVerified        Class = "retired_verified"
	Unverifiable           Class = "unverifiable"
	SDKFingerprintKind           = "rm-fingerprint-draft-v1"
)

type StandardReference struct {
	EventID       string `json:"event_id" bson:"event_id"`
	Producer      string `json:"producer" bson:"producer"`
	Destination   string `json:"destination" bson:"destination"`
	EventType     string `json:"event_type" bson:"event_type"`
	SchemaVersion string `json:"schema_version" bson:"schema_version"`
	Scope         string `json:"scope" bson:"scope"`
	ContentType   string `json:"content_type" bson:"content_type"`
	OccurredAt    string `json:"occurred_at" bson:"occurred_at"`
	Fingerprint   string `json:"fingerprint" bson:"fingerprint"`
}

type Digest struct {
	Kind   string `json:"kind" bson:"kind"`
	SHA256 string `json:"sha256" bson:"sha256"`
}

type Verification struct {
	Method               string    `json:"method" bson:"method"`
	Version              string    `json:"version" bson:"version"`
	OperationID          string    `json:"operation_id,omitempty" bson:"operation_id,omitempty"`
	Reason               string    `json:"reason,omitempty" bson:"reason,omitempty"`
	VerifiedAt           time.Time `json:"verified_at" bson:"verified_at"`
	BusinessTerminal     bool      `json:"business_terminal" bson:"business_terminal"`
	OwnershipVerified    bool      `json:"ownership_verified" bson:"ownership_verified"`
	ResponsibilityClosed bool      `json:"responsibility_closed" bson:"responsibility_closed"`
}

type EventEvidenceV1 struct {
	Version               int                `json:"version" bson:"version"`
	Class                 Class              `json:"class" bson:"class"`
	EventID               string             `json:"event_id,omitempty" bson:"event_id,omitempty"`
	Reference             *StandardReference `json:"reference,omitempty" bson:"reference,omitempty"`
	Digest                Digest             `json:"digest" bson:"digest"`
	BusinessBindingSHA256 string             `json:"business_binding_sha256" bson:"business_binding_sha256"`
	Origin                string             `json:"origin" bson:"origin"`
	Verification          Verification       `json:"verification" bson:"verification"`
}

func ValidSHA256(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func (r StandardReference) Validate() error {
	org, err := strconv.ParseInt(strings.TrimPrefix(r.Scope, "org:"), 10, 64)
	if r.Producer != "qs-server" || r.EventID == "" || len(r.EventID) > 128 || r.Destination == "" || r.EventType == "" ||
		r.SchemaVersion != "v1" || r.ContentType != "application/json" || !strings.HasPrefix(r.Scope, "org:") || err != nil || org <= 0 || !ValidSHA256(r.Fingerprint) {
		return fmt.Errorf("invalid standard event reference")
	}
	if _, err := time.Parse(time.RFC3339Nano, r.OccurredAt); err != nil {
		return fmt.Errorf("invalid standard event time: %w", err)
	}
	return nil
}

func (e *EventEvidenceV1) Validate() error {
	if e == nil || e.Version != 1 || !ValidSHA256(e.BusinessBindingSHA256) || e.Origin == "" ||
		e.Verification.Method == "" || e.Verification.Version == "" || e.Verification.VerifiedAt.IsZero() {
		return fmt.Errorf("invalid event evidence")
	}
	switch e.Class {
	case StandardReferenceClass:
		if e.Reference == nil || e.Reference.EventID != e.EventID || e.Digest.Kind != SDKFingerprintKind || e.Digest.SHA256 != e.Reference.Fingerprint ||
			(e.Origin != "native_atomic" && e.Origin != "backfilled_existing") {
			return fmt.Errorf("invalid standard event evidence")
		}
		return e.Reference.Validate()
	case RetiredVerified, Unverifiable:
		if e.Reference != nil || e.Verification.OperationID == "" || !e.Verification.BusinessTerminal || !e.Verification.OwnershipVerified || !e.Verification.ResponsibilityClosed {
			return fmt.Errorf("historical event responsibility is not verified closed")
		}
		if e.Class == RetiredVerified && (e.EventID == "" || e.Digest.Kind == "" || e.Digest.Kind == SDKFingerprintKind || !ValidSHA256(e.Digest.SHA256)) {
			return fmt.Errorf("invalid retired event source digest")
		}
		if e.Class == Unverifiable && (e.Verification.Reason == "" || (e.Digest.SHA256 != "" && (!ValidSHA256(e.Digest.SHA256) || e.Digest.Kind == "" || e.Digest.Kind == SDKFingerprintKind))) {
			return fmt.Errorf("historical evidence gap requires an explicit reason")
		}
		return nil
	default:
		return fmt.Errorf("unknown event evidence class")
	}
}

func (e *EventEvidenceV1) Clone() *EventEvidenceV1 {
	if e == nil {
		return nil
	}
	out := *e
	if e.Reference != nil {
		ref := *e.Reference
		out.Reference = &ref
	}
	return &out
}

func NewStandard(ref StandardReference, bindingSHA string) (*EventEvidenceV1, error) {
	e := &EventEvidenceV1{Version: 1, Class: StandardReferenceClass, EventID: ref.EventID, Reference: &ref,
		Digest: Digest{Kind: SDKFingerprintKind, SHA256: ref.Fingerprint}, BusinessBindingSHA256: bindingSHA, Origin: "native_atomic",
		Verification: Verification{Method: "atomic-business-and-outbox", Version: "v1", VerifiedAt: time.Now().UTC()}}
	return e, e.Validate()
}

// BindingDigest uses a versioned, type-specific ordered list of frozen facts.
// Nil and empty are distinct; bytes are never parsed and reserialized.
func BindingDigest(eventType string, fields ...*string) string {
	h := sha256.New()
	_, _ = h.Write([]byte("qs-event-business-binding/v1\x00"))
	all := append([]*string{String(eventType)}, fields...)
	for _, value := range all {
		if value == nil {
			_, _ = h.Write([]byte{0})
			continue
		}
		_, _ = h.Write([]byte{1})
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(*value)))
		_, _ = h.Write(length[:])
		_, _ = h.Write([]byte(*value))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func String(value string) *string { return &value }
func MillisecondTime(value time.Time) string {
	return value.UTC().Truncate(time.Millisecond).Format(time.RFC3339Nano)
}
func SourceDigest(kind string, source []byte) Digest {
	hash := sha256.Sum256(source)
	return Digest{Kind: kind, SHA256: hex.EncodeToString(hash[:])}
}
