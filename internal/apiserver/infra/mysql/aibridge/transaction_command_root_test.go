package aibridge

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"github.com/FangcunMount/reliable-messaging/wire/protected"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type commandExecuteFunc func(context.Context, transaction.Execution, transaction.LocalExecutor) (transaction.Result, error)

func (f commandExecuteFunc) Execute(ctx context.Context, e transaction.Execution, local transaction.LocalExecutor) (transaction.Result, error) {
	return f(ctx, e, local)
}

func commandExecutionFixture(t *testing.T) transaction.Execution {
	t.Helper()
	c := orderCommandFixture(t)
	body, err := c.Body()
	require.NoError(t, err)
	header, raw, err := app.PrepareMessaging(c.Kind(), c.ID(), c.Aggregate(), "", body)
	require.NoError(t, err)
	header.OriginalOccurredAt = "2026-10-21T03:00:00Z"
	wire, err := legacy.Encode(legacy.Envelope{UUID: c.ID(), Metadata: map[string]string{"secure_profile": protected.Profile}, Payload: []byte("opaque-unit-wire")}, legacy.Revision2)
	require.NoError(t, err)
	plan, err := c.Freeze(&app.PreparedMessaging{Envelope: header, Body: raw, Wire: wire, Topic: app.CommandsTopic}, 1)
	require.NoError(t, err)
	return transaction.Execution{Plan: plan, Fence: 1, WriterEpoch: "test-epoch", Token: [32]byte{1}, LeaseUntil: time.Now().Add(time.Minute)}
}

func TestCommandExecutorWaitsForActualRootAfterUpstreamCancellation(t *testing.T) {
	e := commandExecutionFixture(t)
	started, finish, returned := make(chan struct{}), make(chan struct{}), make(chan transaction.Result, 1)
	sender := commandExecuteFunc(func(ctx context.Context, actual transaction.Execution, local transaction.LocalExecutor) (transaction.Result, error) {
		go func() { _, _ = local(ctx, actual) }()
		<-started
		return transaction.InitialResult(actual), context.Canceled
	})
	go func() {
		r, _ := executeCommand(t.Context(), sender, e, func(context.Context, transaction.Execution) (transaction.LocalOutcome, error) {
			close(started)
			<-finish
			return transaction.LocalCommitted, nil
		})
		returned <- r
	}()
	<-started
	select {
	case <-returned:
		t.Fatal("released a genuinely running original root")
	case <-time.After(20 * time.Millisecond):
	}
	close(finish)
	select {
	case result := <-returned:
		require.Equal(t, transaction.LocalCommitted, result.Local)
	case <-time.After(time.Second):
		t.Fatal("root completion did not release its join")
	}
}

func TestCommandExecutorRefusesLateDuplicateAndWrongExecutionCallbacks(t *testing.T) {
	for _, name := range []string{"late", "duplicate", "wrong-fence", "wrong-token", "wrong-epoch"} {
		t.Run(name, func(t *testing.T) {
			e := commandExecutionFixture(t)
			var calls atomic.Int32
			var late transaction.LocalExecutor
			sender := commandExecuteFunc(func(ctx context.Context, actual transaction.Execution, local transaction.LocalExecutor) (transaction.Result, error) {
				if name == "late" {
					late = local
					return transaction.InitialResult(actual), context.Canceled
				}
				switch name {
				case "wrong-fence":
					actual.Fence++
				case "wrong-token":
					actual.Token[0]++
				case "wrong-epoch":
					actual.WriterEpoch = "another-epoch"
				}
				state, err := local(ctx, actual)
				if name == "duplicate" {
					require.NoError(t, err)
					require.Equal(t, transaction.LocalCommitted, state)
					state, err = local(ctx, actual)
				}
				require.ErrorIs(t, err, transaction.ErrStaleExecution)
				require.Equal(t, transaction.LocalUnknown, state)
				return transaction.InitialResult(actual), err
			})
			r, _ := executeCommand(t.Context(), sender, e, func(context.Context, transaction.Execution) (transaction.LocalOutcome, error) {
				calls.Add(1)
				return transaction.LocalCommitted, nil
			})
			if late != nil {
				_, err := late(t.Context(), e)
				require.ErrorIs(t, err, transaction.ErrStaleExecution)
			}
			if name == "duplicate" {
				require.EqualValues(t, 1, calls.Load())
				require.Equal(t, transaction.LocalCommitted, r.Local)
			} else {
				require.Zero(t, calls.Load())
			}
		})
	}
}

func TestCommandExecutorDoesNotPromoteCallbackErrorToRollbackProof(t *testing.T) {
	e := commandExecutionFixture(t)
	fault := errors.New("commit reply lost")
	sender := commandExecuteFunc(func(ctx context.Context, actual transaction.Execution, local transaction.LocalExecutor) (transaction.Result, error) {
		state, err := local(ctx, actual)
		r := transaction.InitialResult(actual)
		r.Local = state
		return r, err
	})
	r, err := executeCommand(t.Context(), sender, e, func(context.Context, transaction.Execution) (transaction.LocalOutcome, error) {
		return transaction.LocalUnknown, fault
	})
	require.ErrorIs(t, err, fault)
	require.Equal(t, transaction.LocalUnknown, r.Local)
}

func TestCommandWinnerReconstructsEveryStartFactAndOwnsMutableFields(t *testing.T) {
	input := app.Start{RequestID: orderAggregate, Actor: app.Actor{OrgID: "7", SubjectID: "operator:9"}, TesteeID: "19", AssessmentIDs: []string{"21"}, Goal: "frozen goal", Evidence: []app.EvidenceItem{{AssessmentID: "21", TesteeID: "19", ReportID: orderSession, SourceVersion: "v1", Facts: []app.Fact{{Ref: "fact:1", Value: "original"}}}}}
	body := startMessagingBody(input)
	raw, err := proto.Marshal(body)
	require.NoError(t, err)
	winner := new(pb.MessagingBody)
	require.NoError(t, proto.Unmarshal(raw, winner))
	actual := originalStart(winner.GetStart())
	require.Equal(t, input, actual)
	actual.AssessmentIDs[0] = "changed"
	actual.Evidence[0].Facts[0].Value = "changed"
	require.Equal(t, "21", winner.GetStart().AssessmentIds[0])
	require.Equal(t, "original", winner.GetStart().Evidence[0].Facts[0].Value)
}
