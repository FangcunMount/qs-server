// Package admission owns Interpretation lifecycle-front admission evidence.
// Failures here happen before Generation/Run creation and must not pollute
// business lifecycle collections.
package admission

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// Kind classifies why Interpretation refused to start a Generation.
type Kind string

const (
	KindOutcomeNotFound     Kind = "outcome_not_found"
	KindOutcomeUnauthorized Kind = "outcome_unauthorized"
	KindPayloadDecode       Kind = "payload_decode"
	KindReportInputDecode   Kind = "report_input_decode"
	KindMapping             Kind = "mapping"
	KindFrozenIdentity      Kind = "frozen_identity"
	KindRuntimeSpecInvalid  Kind = "runtime_spec_invalid"
	KindUnknown             Kind = "unknown"
)

func (k Kind) IsValid() bool {
	switch k {
	case KindOutcomeNotFound, KindOutcomeUnauthorized, KindPayloadDecode, KindReportInputDecode,
		KindMapping, KindFrozenIdentity, KindRuntimeSpecInvalid, KindUnknown:
		return true
	default:
		return false
	}
}

// Failure is durable evidence for a rejected Interpretation admission.
type Failure struct {
	id           meta.ID
	outcomeID    meta.ID
	orgID        int64
	assessmentID meta.ID
	testeeID     uint64
	eventID      string
	traceID      string
	kind         Kind
	code         string
	safeMessage  string
	retryable    bool
	fingerprint  string
	occurredAt   time.Time
}

// Input constructs one admission failure.
type Input struct {
	ID           meta.ID
	OutcomeID    meta.ID
	OrgID        int64
	AssessmentID meta.ID
	TesteeID     uint64
	EventID      string
	TraceID      string
	Kind         Kind
	Code         string
	SafeMessage  string
	Retryable    bool
	OccurredAt   time.Time
}

// NewFailure validates and builds durable admission evidence.
func NewFailure(input Input) (*Failure, error) {
	if input.ID.IsZero() {
		return nil, fmt.Errorf("admission failure id is required")
	}
	if !input.Kind.IsValid() {
		return nil, fmt.Errorf("admission failure kind is invalid")
	}
	if input.Code == "" || input.SafeMessage == "" {
		return nil, fmt.Errorf("admission failure code and safe message are required")
	}
	if input.OccurredAt.IsZero() {
		return nil, fmt.Errorf("admission failure occurred_at is required")
	}
	fingerprint := Fingerprint(input.EventID, input.OutcomeID, input.Kind, input.Code)
	return &Failure{
		id: input.ID, outcomeID: input.OutcomeID, orgID: input.OrgID, assessmentID: input.AssessmentID,
		testeeID: input.TesteeID, eventID: input.EventID, traceID: input.TraceID, kind: input.Kind,
		code: input.Code, safeMessage: input.SafeMessage, retryable: input.Retryable,
		fingerprint: fingerprint, occurredAt: input.OccurredAt,
	}, nil
}

// Fingerprint is the idempotency key for admission evidence.
func Fingerprint(eventID string, outcomeID meta.ID, kind Kind, code string) string {
	if eventID != "" {
		return "event:" + eventID
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s", outcomeID.String(), kind, code)))
	return "hash:" + hex.EncodeToString(sum[:16])
}

func (f *Failure) ID() meta.ID           { return f.id }
func (f *Failure) OutcomeID() meta.ID    { return f.outcomeID }
func (f *Failure) OrgID() int64          { return f.orgID }
func (f *Failure) AssessmentID() meta.ID { return f.assessmentID }
func (f *Failure) TesteeID() uint64      { return f.testeeID }
func (f *Failure) EventID() string       { return f.eventID }
func (f *Failure) TraceID() string       { return f.traceID }
func (f *Failure) Kind() Kind            { return f.kind }
func (f *Failure) Code() string          { return f.code }
func (f *Failure) SafeMessage() string   { return f.safeMessage }
func (f *Failure) Retryable() bool       { return f.retryable }
func (f *Failure) Fingerprint() string   { return f.fingerprint }
func (f *Failure) OccurredAt() time.Time { return f.occurredAt }
