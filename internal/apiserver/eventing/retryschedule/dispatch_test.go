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
