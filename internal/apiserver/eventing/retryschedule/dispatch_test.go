package retryschedule

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	shared "github.com/FangcunMount/qs-server/internal/apiserver/eventing/transactionplan"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport"
	"github.com/stretchr/testify/require"
)

func TestDueDispatchFreezesSelectionSeparatelyFromBusinessAuthorization(t *testing.T) {
	at := time.Date(2026, 10, 21, 0, 0, 0, 123499999, time.UTC)
	a, in := fixture(t, Evaluation, retrygovernance.AttemptOriginAutomatic, "", "retry-first", at)
	f, err := New(a, in, at)
	require.NoError(t, err)
	r := Record{Plan: f, State: Ready, Version: 1}
	_, err = FreezeDispatch(r, at.Add(-time.Nanosecond))
	require.ErrorIs(t, err, ErrInvalid)
	first, err := FreezeDispatch(r, at)
	require.NoError(t, err)
	later, err := FreezeDispatch(r, at.Add(time.Hour))
	require.NoError(t, err)
	require.Equal(t, first.Key, later.Key)
	require.Equal(t, first.InputHash, later.InputHash)
	require.NotEqual(t, first.Artifact, later.Artifact)
	p, err := shared.Freeze(first, routes{}, a.Owner, "m3-1")
	require.NoError(t, err)
	recovered, err := ReadDispatch(r, p)
	require.NoError(t, err)
	require.True(t, recovered.SelectedAt.Equal(at))
	require.Equal(t, in.Payload, p.Input().Branches[0].Wire)
	proof, err := DispatchProof(r, transaction.Execution{Plan: p, Fence: 1, WriterEpoch: "m3-1"})
	require.NoError(t, err)
	require.True(t, proof.RecordedAt.Equal(at))
	require.NoError(t, MatchDispatchProof(r, p, proof))
	changedTime := *proof
	changedTime.RecordedAt = changedTime.RecordedAt.Add(time.Nanosecond)
	require.ErrorIs(t, MatchDispatchProof(r, p, &changedTime), ErrInvalid)

	require.NoError(t, f.ValidateDispatchProof(proof))
	for _, mode := range []string{"early", "schedule", "revision", "wire", "key", "hash", "route"} {
		t.Run(mode, func(t *testing.T) {
			q := first
			q.Prepared = append([]message.Input(nil), first.Prepared...)
			var d Dispatch
			require.NoError(t, json.Unmarshal(q.Artifact, &d))
			switch mode {
			case "early":
				d.SelectedAt = at.Add(-time.Nanosecond)
			case "schedule":
				d.Schedule = []byte("substituted")
			case "revision":
				d.Revision++
			case "wire":
				_, other := fixture(t, Evaluation, retrygovernance.AttemptOriginAutomatic, "", "other", at)
				q.Prepared[0] = other
			case "key":
				q.Key += "-other"
			case "hash":
				q.InputHash = [32]byte{2}
			}
			q.Artifact, err = json.Marshal(d)
			require.NoError(t, err)
			changed, err := shared.Freeze(q, routes{}, a.Owner, "m3-1")
			require.NoError(t, err)
			if mode == "route" {
				input := changed.Input()
				input.Branches[0].RouteRevision = "m3-2"
				changed, err = transaction.NewPlan(input)
				require.NoError(t, err)
			}
			_, err = ReadDispatch(r, changed)
			require.Error(t, err)
		})
	}
	r.Version = math.MaxInt64
	_, err = FreezeDispatch(r, at)
	require.ErrorIs(t, err, ErrInvalid)
	r.Version = 1
	r.State = Held
	_, err = FreezeDispatch(r, at)
	require.ErrorIs(t, err, ErrInvalid)
}

func TestDueDispatchBindsNewManifestWithoutChangingFutureAuthorityOrFirstWire(t *testing.T) {
	at := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	a, in := fixture(t, Evaluation, retrygovernance.AttemptOriginAutomatic, "", "readable-retry", at)
	future, err := New(a, in, at)
	require.NoError(t, err)
	original := future.Bytes()
	record := Record{Plan: future, State: Ready, Version: 1}
	request, err := FreezeDispatch(record, at)
	require.NoError(t, err)
	physical, err := transport.PhysicalTopicV3(in.Destination, transport.TransactionMessageType, a.Owner, "m3-1")
	require.NoError(t, err)
	route := transport.RouteBinding{Topic: in.Destination, PhysicalTopic: physical, MessageType: transport.TransactionMessageType, Owner: a.Owner, RouteRevision: "m3-1", NamingScheme: transport.ReadableNamingV1}
	policy, err := transport.NewRouteResolver([]transport.RouteBinding{route}, nil)
	require.NoError(t, err)
	plan, err := shared.FreezeWithRoutes(request, routes{}, a.Owner, "m3-1", policy)
	require.NoError(t, err)
	read := func(p transaction.FrozenPlan) ([]byte, error) {
		return shared.ArtifactWithRoutes(p, a.Owner, "m3-1", policy)
	}
	_, err = ReadDispatch(record, plan, read)
	require.NoError(t, err)
	proof, err := DispatchProof(record, transaction.Execution{Plan: plan, Fence: 1, WriterEpoch: "m3-1"}, read)
	require.NoError(t, err)
	require.NoError(t, MatchDispatchProof(record, plan, proof, read))
	require.Equal(t, original, future.Bytes())
	require.Equal(t, in.ID, plan.Input().Branches[0].Message.Input().ID)
	require.Equal(t, in.Payload, plan.Input().Branches[0].Wire)
	require.Equal(t, physical, plan.Input().Branches[0].PhysicalTopic)
	_, err = ReadDispatch(record, plan, nil)
	require.Error(t, err, "missing host manifest reader must fail closed")
}
