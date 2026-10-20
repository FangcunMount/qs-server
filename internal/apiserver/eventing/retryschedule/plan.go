// Package retryschedule freezes a future, already-authorized business retry.
// It has no publish-attempt state and performs no database or Broker I/O.
package retryschedule

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"
	"unicode/utf8"

	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	outcome "github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
	payload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/FangcunMount/reliable-messaging/message"
)

var (
	ErrInvalid  = errors.New("invalid frozen business retry schedule")
	ErrConflict = errors.New("business retry authorization input conflict")
)

const (
	Evaluation     = "evaluation"
	Interpretation = "interpretation"
	LifecycleTopic = "qs.evaluation.lifecycle"
)

// Authority identifies the original failed run and the host's explicit retry
// decision. DecisionHash binds policy/input or operator permission/reason;
// generated event clocks and wire bytes belong to the reservation winner.
type Authority struct {
	Owner               string
	OrgID               int64
	Kind                string
	SubjectID           string
	RunID               string
	AssessmentID        uint64
	OutcomeID           uint64
	TesteeID            uint64
	ExpectedAttempt     int
	Origin              retrygovernance.AttemptOrigin
	RequestID           string
	DecisionHash        [32]byte
	AuthorizationSHA256 string `json:",omitempty"`
	Authorization       []byte `json:",omitempty"`
}

type document struct {
	Version   int
	Authority Authority
	DueAt     time.Time
	Message   message.Input
}

// Frozen owns both the first message bytes and its future business deadline.
// Accessors return copies; a store must persist Bytes verbatim.
type Frozen struct {
	value document
	raw   []byte
}

func New(a Authority, in message.Input, due time.Time) (Frozen, error) {
	m, err := message.New(in)
	if err != nil {
		return Frozen{}, ErrInvalid
	}
	a.Authorization = bytes.Clone(a.Authorization)
	d := document{Version: 1, Authority: a, DueAt: due.UTC(), Message: m.Input()}
	if err := validate(d); err != nil {
		return Frozen{}, err
	}
	raw, err := json.Marshal(d)
	if err != nil || len(raw) > 1<<20 {
		return Frozen{}, ErrInvalid
	}
	f := Frozen{value: d, raw: raw}
	// Reserve room for the complete nested dispatch artifact, its longest
	// timestamp and any admissible revision. Reject before authorization is
	// committed, rather than persist a future plan the host cannot send.
	if _, err := FreezeDispatch(Record{Plan: f, State: Ready, Version: math.MaxInt64 - 1}, time.Date(9999, 12, 31, 23, 59, 59, 999999999, time.UTC)); err != nil {
		return Frozen{}, ErrInvalid
	}
	return f, nil
}

func Decode(raw []byte) (Frozen, error) {
	if len(raw) == 0 || len(raw) > 1<<20 {
		return Frozen{}, ErrInvalid
	}
	var d document
	if json.Unmarshal(raw, &d) != nil {
		return Frozen{}, ErrInvalid
	}
	f, err := New(d.Authority, d.Message, d.DueAt)
	if err != nil || d.Version != 1 || !bytes.Equal(f.raw, raw) {
		return Frozen{}, ErrInvalid
	}
	return f, nil
}

func (f Frozen) Valid() bool   { return len(f.raw) > 0 }
func (f Frozen) Bytes() []byte { return bytes.Clone(f.raw) }
func (f Frozen) Authority() Authority {
	a := f.value.Authority
	a.Authorization = bytes.Clone(a.Authorization)
	return a
}
func (f Frozen) DueAt() time.Time         { return f.value.DueAt }
func (f Frozen) Message() message.Message { m, _ := message.New(f.value.Message); return m }
func (f Frozen) Digest() [32]byte         { return sha256.Sum256(f.raw) }

// KeyIdentity retains original fields alongside the digest index. A hash
// collision or changed input must never silently replace the first plan.
func (f Frozen) KeyIdentity() []byte {
	a := f.value.Authority
	request := "automatic"
	if a.Origin != retrygovernance.AttemptOriginAutomatic {
		request = "action:" + a.RequestID
	}
	raw, _ := json.Marshal([]any{"qs-retry-schedule/v1", a.Owner, a.OrgID, a.Kind, a.SubjectID, a.RunID, request})
	return raw
}
func (f Frozen) Key() [32]byte { return sha256.Sum256(f.KeyIdentity()) }
func (f Frozen) ID() string    { k := f.Key(); return hex.EncodeToString(k[:]) }
func (f Frozen) InputHash() [32]byte {
	raw, _ := json.Marshal(f.value.Authority)
	return sha256.Sum256(raw)
}

func (f Frozen) MatchInput(candidate Frozen) error {
	if !f.Valid() || !candidate.Valid() || !bytes.Equal(f.KeyIdentity(), candidate.KeyIdentity()) || f.InputHash() != candidate.InputHash() {
		return ErrConflict
	}
	return nil
}

// IndexDue rounds UP for a storage index. The original deadline and wire
// remain unchanged; precision loss must never make a future retry due early.
func (f Frozen) IndexDue(precision time.Duration) (time.Time, error) {
	if !f.Valid() || (precision != time.Microsecond && precision != time.Millisecond) {
		return time.Time{}, ErrInvalid
	}
	at := f.DueAt().Truncate(precision)
	if at.Before(f.DueAt()) {
		at = at.Add(precision)
	}
	if at.Year() > 9999 {
		return time.Time{}, ErrInvalid
	}
	return at, nil
}

func bounded(s string, n int) bool { return s != "" && len(s) <= n && utf8.ValidString(s) }
func canonicalID(s string) bool {
	n, err := strconv.ParseUint(s, 10, 64)
	return err == nil && n > 0 && strconv.FormatUint(n, 10) == s
}

func validate(d document) error {
	a, in := d.Authority, d.Message
	if a.OrgID <= 0 || !canonicalID(a.SubjectID) || !bounded(a.RunID, 128) || a.AssessmentID == 0 || a.AssessmentID > math.MaxInt64 || a.TesteeID == 0 || a.ExpectedAttempt < 1 || a.ExpectedAttempt > math.MaxInt32 || a.DecisionHash == [32]byte{} || d.DueAt.IsZero() || d.DueAt.Year() < 1970 || d.DueAt.Year() > 9999 {
		return ErrInvalid
	}
	switch a.Origin {
	case retrygovernance.AttemptOriginAutomatic:
		if a.RequestID != "" || a.AuthorizationSHA256 != "" || len(a.Authorization) > 0 {
			return ErrInvalid
		}
	case retrygovernance.AttemptOriginManual, retrygovernance.AttemptOriginForce:
		if !bounded(a.RequestID, 255) {
			return ErrInvalid
		}
		decoded, err := hex.DecodeString(a.AuthorizationSHA256)
		if err != nil || len(decoded) != 32 || a.AuthorizationSHA256 != hex.EncodeToString(decoded) || bytes.Equal(decoded, make([]byte, 32)) {
			return ErrInvalid
		}
		digest := sha256.Sum256(a.Authorization)
		if len(a.Authorization) == 0 || !json.Valid(a.Authorization) || bytes.Equal(a.Authorization, []byte("null")) || !bytes.Equal(decoded, digest[:]) {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	if in.Producer != "qs-server" || in.Scope != fmt.Sprintf("org:%d", a.OrgID) || in.Destination != LifecycleTopic || len(in.Payload) > 262144 {
		return ErrInvalid
	}
	m, err := message.New(in)
	if err != nil {
		return ErrInvalid
	}
	ref := standard.ReferenceFromMessage(m)
	envelope, err := standard.VerifyReference(in, ref.Fingerprint, ref)
	if err != nil {
		return ErrInvalid
	}
	if envelope.AggregateID != a.SubjectID {
		return ErrInvalid
	}
	switch a.Kind {
	case Evaluation:
		if a.Owner != "qs.mysql" || a.SubjectID != strconv.FormatUint(a.AssessmentID, 10) || a.RunID != fmt.Sprintf("%s:%d", a.SubjectID, a.ExpectedAttempt) || a.OutcomeID != 0 || in.EventType != catalog.EvaluationRetryRequested || envelope.AggregateType != "Evaluation" {
			return ErrInvalid
		}
		var body payload.EvaluationRequestedData
		if json.Unmarshal(envelope.Data, &body) != nil || body.OrgID != a.OrgID || body.AssessmentID != int64(a.AssessmentID) || body.TesteeID != a.TesteeID || body.ExpectedAttempt != a.ExpectedAttempt || body.AttemptOrigin != string(a.Origin) || body.ActionRequestID != a.RequestID || body.Mode != "next_attempt" || !body.RequestedAt.Equal(d.DueAt) {
			return ErrInvalid
		}
	case Interpretation:
		if a.Owner != "qs.mongo" || !canonicalID(a.RunID) || a.OutcomeID == 0 || in.EventType != catalog.InterpretationRetryRequested || envelope.AggregateType != "ReportGeneration" {
			return ErrInvalid
		}
		var body outcome.InterpretationRetryRequestedPayload
		if json.Unmarshal(envelope.Data, &body) != nil || body.OrgID != a.OrgID || body.GenerationID != a.SubjectID || body.RunID != a.RunID || body.AssessmentID != strconv.FormatUint(a.AssessmentID, 10) || body.OutcomeID != strconv.FormatUint(a.OutcomeID, 10) || body.TesteeID != a.TesteeID || body.ExpectedAttempt != a.ExpectedAttempt || body.AttemptOrigin != string(a.Origin) || body.ActionRequestID != a.RequestID || body.Mode != "next_attempt" || !body.RequestedAt.Equal(d.DueAt) {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}
