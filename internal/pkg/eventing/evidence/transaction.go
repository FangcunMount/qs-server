package evidence

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/FangcunMount/reliable-messaging/transaction"
)

const TransactionReferenceClass Class = "transaction_reference"

// EventEvidenceV2 references the exact reserved SDK branch. This value alone
// proves neither a database commit nor a Broker decision. Persist it only with
// the business fact and CommitWitness in the guarded original root transaction.
// V1 historical references remain V1; they must never be promoted to a witness.
type EventEvidenceV2 struct {
	Version               int               `json:"version" bson:"version"`
	Class                 Class             `json:"class" bson:"class"`
	Reference             StandardReference `json:"reference" bson:"reference"`
	TransactionReference  []byte            `json:"transaction_reference" bson:"transaction_reference"`
	BusinessBindingSHA256 string            `json:"business_binding_sha256" bson:"business_binding_sha256"`
	Origin                string            `json:"origin" bson:"origin"`
	RecordedAt            time.Time         `json:"recorded_at" bson:"recorded_at"`
}

func NewTransactional(ref StandardReference, tx transaction.Reference, bindingSHA string, at time.Time) (*EventEvidenceV2, error) {
	raw, err := transaction.EncodeReference(tx)
	if err != nil {
		return nil, err
	}
	e := &EventEvidenceV2{Version: 2, Class: TransactionReferenceClass, Reference: ref, TransactionReference: raw, BusinessBindingSHA256: bindingSHA, Origin: "native_atomic", RecordedAt: at.UTC()}
	return e, e.Validate()
}

func (e *EventEvidenceV2) Validate() error {
	if e == nil || e.Version != 2 || e.Class != TransactionReferenceClass || e.Origin != "native_atomic" || e.RecordedAt.IsZero() || !ValidSHA256(e.BusinessBindingSHA256) {
		return fmt.Errorf("invalid transaction event evidence")
	}
	if err := e.Reference.Validate(); err != nil {
		return err
	}
	r, err := transaction.DecodeReference(e.TransactionReference)
	if err != nil {
		return fmt.Errorf("invalid transaction reference: %w", err)
	}
	canonical, err := transaction.EncodeReference(r)
	if err != nil || !bytes.Equal(canonical, e.TransactionReference) {
		return fmt.Errorf("transaction reference must retain canonical SDK bytes")
	}
	if (r.Owner != "qs.mysql" && r.Owner != "qs.mongo") || r.RouteRevision != "m3-1" || r.MessageID != e.Reference.EventID || r.Scope != e.Reference.Scope || r.LogicalTopic != e.Reference.Destination || hex.EncodeToString(r.MessageFingerprint[:]) != e.Reference.Fingerprint || r.WireHash == [32]byte{} || r.PlanDigest == [32]byte{} {
		return fmt.Errorf("transaction event identity differs")
	}
	return nil
}

func (e *EventEvidenceV2) Clone() *EventEvidenceV2 {
	if e == nil {
		return nil
	}
	out := *e
	out.TransactionReference = append([]byte(nil), e.TransactionReference...)
	return &out
}

// Record is an explicit versioned read result. Neither branch is a decision
// oracle; only the authority store may resolve the exact transaction reference.
type Record struct {
	V1 *EventEvidenceV1
	V2 *EventEvidenceV2
}

// MarshalJSON keeps the existing flat, explicitly versioned JSON column shape.
// The union is a read/write codec, never a committed-transaction decision.
func (r Record) MarshalJSON() ([]byte, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	if r.V1 != nil {
		return json.Marshal(r.V1)
	}
	return json.Marshal(r.V2)
}

func (r Record) Validate() error {
	if (r.V1 == nil) == (r.V2 == nil) {
		return fmt.Errorf("exactly one event evidence version required")
	}
	if r.V1 != nil {
		return r.V1.Validate()
	}
	return r.V2.Validate()
}

func (r *Record) UnmarshalJSON(data []byte) error {
	decoded, err := DecodeRecord(data)
	if err != nil {
		return err
	}
	*r = decoded
	return nil
}

func (r Record) Clone() Record { return Record{V1: r.V1.Clone(), V2: r.V2.Clone()} }

func (r Record) EventID() string {
	if r.V1 != nil && r.V2 == nil {
		return r.V1.EventID
	}
	if r.V2 != nil && r.V1 == nil {
		return r.V2.Reference.EventID
	}
	return ""
}

func DecodeRecord(data []byte) (Record, error) {
	var version struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &version); err != nil {
		return Record{}, err
	}
	switch version.Version {
	case 1:
		var e EventEvidenceV1
		if err := json.Unmarshal(data, &e); err != nil {
			return Record{}, err
		}
		if err := e.Validate(); err != nil {
			return Record{}, err
		}
		return Record{V1: &e}, nil
	case 2:
		var e EventEvidenceV2
		if err := json.Unmarshal(data, &e); err != nil {
			return Record{}, err
		}
		if err := e.Validate(); err != nil {
			return Record{}, err
		}
		return Record{V2: &e}, nil
	default:
		return Record{}, fmt.Errorf("unsupported event evidence version")
	}
}
