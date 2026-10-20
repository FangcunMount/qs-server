package retryschedule

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"math"
	"time"

	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	shared "github.com/FangcunMount/qs-server/internal/apiserver/eventing/transactionplan"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transaction"
)

// Dispatch is frozen before preparing halves. SelectedAt is the first due
// selection, not a callback clock. A duplicate must reuse this whole artifact.
type Dispatch struct {
	Version    int
	Schedule   []byte
	Revision   int64
	SelectedAt time.Time
}

func DispatchKey(f Frozen) string { return "retry-dispatch/" + f.ID() }

func dispatchHash(f Frozen, revision int64) [32]byte {
	raw, _ := json.Marshal(struct {
		Version  int
		Plan     [32]byte
		Revision int64
	}{1, f.Digest(), revision})
	return sha256.Sum256(raw)
}

func FreezeDispatch(r Record, at time.Time) (port.Request, error) {
	if r.Validate() != nil || r.State != Ready || r.Version == math.MaxInt64 || at.IsZero() || at.Before(r.Plan.DueAt()) || at.Year() > 9999 {
		return port.Request{}, ErrInvalid
	}
	raw, err := json.Marshal(Dispatch{1, r.Plan.Bytes(), r.Version, at.UTC()})
	if err != nil {
		return port.Request{}, err
	}
	return port.Request{Key: DispatchKey(r.Plan), InputHash: dispatchHash(r.Plan, r.Version), Artifact: raw, Prepared: []message.Input{r.Plan.Message().Input()}}, nil
}

// ReadDispatch validates the full frozen request and the exact public first
// wire. It works on readonly recovery plans; an execution token is not needed.
func ReadDispatch(r Record, p transaction.FrozenPlan) (Dispatch, error) {
	if !r.Plan.Valid() || r.Version < 1 || r.Version == math.MaxInt64 {
		return Dispatch{}, ErrInvalid
	}
	raw, err := shared.Artifact(p, r.Plan.Authority().Owner, "m3-1")
	if err != nil {
		return Dispatch{}, err
	}
	var d Dispatch
	if json.Unmarshal(raw, &d) != nil {
		return d, ErrInvalid
	}
	canonical, err := json.Marshal(d)
	if err != nil || !bytes.Equal(canonical, raw) || d.Version != 1 || d.Revision != r.Version || !bytes.Equal(d.Schedule, r.Plan.Bytes()) || d.SelectedAt.IsZero() || d.SelectedAt.Location() != time.UTC || d.SelectedAt.Before(r.Plan.DueAt()) || d.SelectedAt.Year() > 9999 {
		return d, ErrInvalid
	}
	in := p.Input()
	if err := p.MatchRequest(r.Plan.Authority().Owner, r.Plan.Message().Input().Scope, DispatchKey(r.Plan), dispatchHash(r.Plan, r.Version)); err != nil {
		return d, err
	}
	if len(in.Branches) != 1 || in.OperationID != r.Plan.Message().Input().ID {
		return d, ErrInvalid
	}
	b := in.Branches[0]
	if !bytes.Equal(b.Wire, r.Plan.Message().Input().Payload) {
		return d, ErrInvalid
	}
	consumer := b.Message.Input()
	consumer.Payload = bytes.Clone(b.Wire)
	actual, err := message.New(consumer)
	if err != nil || actual.Fingerprint() != r.Plan.Message().Fingerprint() {
		return d, ErrInvalid
	}
	return d, nil
}

func DispatchProof(r Record, e transaction.Execution) (*evidence.EventEvidenceV2, error) {
	d, err := ReadDispatch(r, e.Plan)
	if err != nil {
		return nil, err
	}
	refs := e.References()
	if len(refs) != 1 {
		return nil, ErrInvalid
	}
	proof, err := evidence.NewTransactional(standard.ReferenceFromMessage(e.Plan.Input().Branches[0].Message), refs[0], r.Plan.BindingDigest(), d.SelectedAt)
	if err != nil {
		return nil, err
	}
	return proof, r.Plan.ValidateDispatchProof(proof)
}
