package retryschedule

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/reliable-messaging/transaction"
)

type State string

const (
	Ready             State = "ready"
	DispatchCommitted State = "dispatch_committed"
	Canceled          State = "canceled"
	Held              State = "held"
)

// Record tracks the future business dispatch, not Broker delivery or consumer
// completion. Only an original-root SDK witness may support its terminal proof.
type Record struct {
	Plan    Frozen
	State   State
	Version int64
	Proof   *evidence.EventEvidenceV2
}

func (r Record) Validate() error {
	if !r.Plan.Valid() || r.Version < 1 {
		return ErrInvalid
	}
	switch r.State {
	case Ready, Canceled, Held:
		if r.Proof != nil {
			return ErrInvalid
		}
	case DispatchCommitted:
		if err := r.Plan.ValidateDispatchProof(r.Proof); err != nil {
			return err
		}
	default:
		return ErrInvalid
	}
	return nil
}

func (f Frozen) BindingDigest() string { digest := f.Digest(); return hex.EncodeToString(digest[:]) }

// ValidateDispatchProof verifies exact frozen identity and bytes. This is not
// a commit oracle: callers still read the original authority's exact witnesses.
// The SDK reference fingerprint includes the private dispatch artifact, while
// WireHash binds the unchanged public wire.
func (f Frozen) ValidateDispatchProof(proof *evidence.EventEvidenceV2) error {
	if !f.Valid() || proof.Validate() != nil || proof.BusinessBindingSHA256 != f.BindingDigest() {
		return ErrInvalid
	}
	ref, err := transaction.DecodeReference(proof.TransactionReference)
	if err != nil {
		return ErrInvalid
	}
	in := f.Message().Input()
	actual := proof.Reference
	if ref.Owner != f.Authority().Owner || ref.WireHash != sha256.Sum256(in.Payload) || actual.Producer != in.Producer || actual.EventID != in.ID || actual.Destination != in.Destination || actual.EventType != in.EventType || actual.SchemaVersion != in.SchemaVersion || actual.Scope != in.Scope || actual.ContentType != in.ContentType || actual.OccurredAt != in.OccurredAt {
		return ErrInvalid
	}
	return nil
}
