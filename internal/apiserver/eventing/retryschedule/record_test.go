package retryschedule

import (
	"bytes"
	"testing"
	"time"

	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	shared "github.com/FangcunMount/qs-server/internal/apiserver/eventing/transactionplan"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
)

func TestPreparedRetryUsesExactOriginalWireAndBoundPrivateArtifact(t *testing.T) {
	at := time.Date(2026, 10, 21, 0, 0, 0, 123499999, time.UTC)
	a, in := fixture(t, Evaluation, retrygovernance.AttemptOriginAutomatic, "", "retry-first", at)
	f, err := New(a, in, at)
	require.NoError(t, err)
	q := port.Request{Key: "retry-dispatch/" + f.ID(), InputHash: f.Digest(), Artifact: f.Bytes(), Prepared: []message.Input{f.Message().Input()}}
	p, err := shared.Freeze(q, routes{}, a.Owner, "m3-1")
	require.NoError(t, err)
	branch := p.Input().Branches[0]
	require.Equal(t, in.Payload, branch.Wire)
	require.NotEqual(t, in.Payload, branch.Message.Input().Payload, "the private artifact must not replace public wire")
	raw, err := shared.Artifact(p, a.Owner, "m3-1")
	require.NoError(t, err)
	require.Equal(t, f.Bytes(), raw)
	refs := (transaction.Execution{Plan: p, Fence: 1, WriterEpoch: "m3-1"}).References()
	proof, err := evidence.NewTransactional(standard.ReferenceFromMessage(branch.Message), refs[0], f.BindingDigest(), at)
	require.NoError(t, err)
	require.NoError(t, (Record{Plan: f, State: DispatchCommitted, Version: 2, Proof: proof}).Validate())
	proof.BusinessBindingSHA256 = string(bytes.Repeat([]byte{'0'}, 64))
	require.ErrorIs(t, (Record{Plan: f, State: DispatchCommitted, Version: 2, Proof: proof}).Validate(), ErrInvalid)
	q.Events = []event.DomainEvent{nil}
	_, err = shared.Freeze(q, routes{}, a.Owner, "m3-1")
	require.ErrorIs(t, err, transaction.ErrInvalidPlan)
	q.Events = nil
	q.Prepared[0].Scope = "org:8"
	_, err = shared.Freeze(q, routes{}, a.Owner, "m3-1")
	require.Error(t, err)
}

func TestDispatchProofForAnotherFirstWireCannotCloseOriginalSchedule(t *testing.T) {
	at := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	a, first := fixture(t, Interpretation, retrygovernance.AttemptOriginAutomatic, "", "first", at)
	f, err := New(a, first, at)
	require.NoError(t, err)
	_, other := fixture(t, Interpretation, retrygovernance.AttemptOriginAutomatic, "", "second", at)
	p, err := shared.Freeze(port.Request{Key: "retry-dispatch/" + f.ID(), InputHash: f.Digest(), Artifact: f.Bytes(), Prepared: []message.Input{other}}, routes{}, a.Owner, "m3-1")
	require.NoError(t, err)
	b := p.Input().Branches[0]
	ref := (transaction.Execution{Plan: p, Fence: 1, WriterEpoch: "m3-1"}).References()[0]
	proof, err := evidence.NewTransactional(standard.ReferenceFromMessage(b.Message), ref, f.BindingDigest(), at)
	require.NoError(t, err)
	require.ErrorIs(t, f.ValidateDispatchProof(proof), ErrInvalid)
	require.ErrorIs(t, (Record{Plan: f, State: DispatchCommitted, Version: 1}).Validate(), ErrInvalid)
	require.NoError(t, (Record{Plan: f, State: Ready, Version: 1}).Validate())
	require.ErrorIs(t, (Record{Plan: f, State: Ready, Version: 1, Proof: proof}).Validate(), ErrInvalid)
}
