package taskopen

import (
	"context"
	"testing"
	"time"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	shared "github.com/FangcunMount/qs-server/internal/apiserver/eventing/transactionplan"
	repository "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/plan"
	eventport "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/tasktransaction"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
)

type openingBoundaryStub struct {
	e       transaction.Execution
	local   transaction.LocalOutcome
	commits int
}

func (b *openingBoundaryStub) Reserve(_ context.Context, q eventport.Request) (transaction.Reservation, error) {
	p, err := shared.Freeze(q, taskRoutes{}, "qs.mysql", "m3-1")
	if err != nil {
		return transaction.Reservation{}, err
	}
	b.e = transaction.Execution{Plan: p, Fence: 1, WriterEpoch: "m3-1", Token: [32]byte{1}, LeaseUntil: time.Now().Add(time.Minute)}
	return transaction.Reservation{Execution: b.e, Acquired: true}, nil
}
func (*openingBoundaryStub) Find(context.Context, string, string) (transaction.Reservation, error) {
	return transaction.Reservation{}, transaction.ErrNotFound
}
func (*openingBoundaryStub) Artifact(p transaction.FrozenPlan) ([]byte, error) {
	return shared.Artifact(p, "qs.mysql", "m3-1")
}
func (b *openingBoundaryStub) Commit(_ context.Context, e transaction.Execution, _ func(context.Context) error) (transaction.Result, error) {
	b.commits++
	r := transaction.InitialResult(e)
	r.Local = b.local
	return r, nil
}
func (*openingBoundaryStub) Confirm(context.Context, transaction.Execution) transaction.LocalOutcome {
	return transaction.LocalUnknown
}

type openingWriterStub struct{ calls int }

func (w *openingWriterStub) SaveOpenedSnapshot(context.Context, *repository.AssessmentTaskPO, time.Time, uint64) error {
	w.calls++
	return nil
}

func TestOpeningSenderUnknownDoesNotWriteOrReportSuccess(t *testing.T) {
	for _, local := range []transaction.LocalOutcome{transaction.LocalNotStarted, transaction.LocalUnknown, transaction.LocalAborted, transaction.LocalCommitted} {
		t.Run(string(local), func(t *testing.T) {
			b := &openingBoundaryStub{local: local}
			writer := &openingWriterStub{}
			c := &Committer{boundary: b, writer: writer}
			result, err := c.CommitOpened(t.Context(), port.OpenRequest{Candidate: openedTask(t)})
			require.Equal(t, 1, b.commits)
			require.Zero(t, writer.calls, "stub boundary never admitted local callback")
			if local == transaction.LocalCommitted {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Empty(t, result.Events())
			} else {
				require.ErrorIs(t, err, eventport.ErrUnresolved)
				require.Nil(t, result)
			}
		})
	}
}

func TestOpeningReadOnlyPlanRetainsReferenceWithoutExecutionToken(t *testing.T) {
	b := &openingBoundaryStub{}
	q, err := freeze(openedTask(t), 1)
	require.NoError(t, err)
	r, err := b.Reserve(t.Context(), q)
	require.NoError(t, err)
	r.Execution.Token = [32]byte{}
	require.False(t, r.Execution.Valid())
	f, err := (&Committer{boundary: b}).readExecution(r.Execution)
	require.NoError(t, err)
	require.Equal(t, domain.TaskStatusOpened.String(), f.Task.Status)
	require.Equal(t, [32]byte{}, r.Execution.Token)
}
