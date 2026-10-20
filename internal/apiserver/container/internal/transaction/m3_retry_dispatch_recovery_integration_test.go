//go:build integration && (m3_evaluation_broker || m3_report_integration)

package transaction_test

import (
	"context"
	"errors"
	"testing"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing/retrydispatch"
	model "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	events "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/retryschedule"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
)

type rejectAfterRealAuthority struct{ original port.AuthorityValidator }

func (v rejectAfterRealAuthority) ValidateDispatchAuthority(ctx context.Context, plan model.Frozen) error {
	if err := v.original.ValidateDispatchAuthority(ctx, plan); err != nil {
		return err
	}
	// This is a controlled root rollback, not a lost database reply or network
	// fault. The following assertions read the real persisted Aborted witnesses.
	return errors.New("fixture refuses dispatch inside the original guarded root")
}

func recoverAbortedDueDispatch(t *testing.T, store port.DispatchStore, owner events.Committer, authority port.AuthorityValidator, protocol transaction.Store, frozen model.Frozen) app.Outcome {
	record, err := store.Find(t.Context(), frozen)
	require.NoError(t, err)
	before := record.Plan.Bytes()
	paused, err := app.New(store, owner, authority, app.WithAutomaticRetrySwitch(func() bool { return true }))
	require.NoError(t, err)
	outcomes, err := paused.Once(t.Context(), time.Now().UTC(), 10)
	require.NoError(t, err)
	require.Len(t, outcomes, 1)
	require.ErrorIs(t, outcomes[0].Err, eventruntime.ErrAutomaticRetryPaused)
	_, err = owner.Find(t.Context(), frozen.Message().Input().Scope, model.DispatchKey(frozen))
	require.ErrorIs(t, err, transaction.ErrNotFound, "pause must not reserve or prepare a half")
	rejecting, err := app.New(store, owner, rejectAfterRealAuthority{authority})
	require.NoError(t, err)
	outcomes, err = rejecting.Once(t.Context(), time.Now().UTC(), 10)
	require.NoError(t, err)
	require.Len(t, outcomes, 1)
	require.ErrorIs(t, outcomes[0].Err, events.ErrUnresolved)
	previous, err := owner.Find(t.Context(), frozen.Message().Input().Scope, model.DispatchKey(frozen))
	require.NoError(t, err)
	// A callback error is not durable rollback evidence. Wait for the real
	// lease, then compete over the SAME execution via the protocol reconciler.
	// No synthetic exception or altered database clock stands in for this proof.
	if owner.Confirm(t.Context(), previous.Execution) != transaction.LocalAborted {
		wait := time.NewTimer(time.Until(previous.Execution.LeaseUntil.Add(2 * time.Millisecond)))
		select {
		case <-wait.C:
		case <-t.Context().Done():
			wait.Stop()
			t.Fatal("real protocol lease wait cancelled")
		}
		_, err = protocol.ReconcileOnce(t.Context(), 100)
		require.NoError(t, err)
	}
	require.Equal(t, transaction.LocalAborted, owner.Confirm(t.Context(), previous.Execution))
	exact, err := store.Find(t.Context(), frozen)
	require.NoError(t, err)
	require.Equal(t, model.Ready, exact.State)
	require.Equal(t, record.Version, exact.Version)
	require.Equal(t, before, exact.Plan.Bytes())
	healthy, err := app.New(store, owner, authority)
	require.NoError(t, err)
	outcomes, err = healthy.Once(t.Context(), time.Now().UTC(), 10)
	require.NoError(t, err)
	require.Len(t, outcomes, 1)
	require.ErrorIs(t, outcomes[0].Err, events.ErrUnresolved)
	same, err := owner.Find(t.Context(), frozen.Message().Input().Scope, model.DispatchKey(frozen))
	require.NoError(t, err)
	require.Equal(t, previous.Execution.Fence, same.Execution.Fence, "scan cannot implicitly grant another fence")
	out := healthy.RetryAborted(t.Context(), *exact, previous.Execution, time.Minute)
	require.NoError(t, out.Err)
	require.Equal(t, transaction.LocalCommitted, out.Transaction.Local)
	current, err := owner.Find(t.Context(), frozen.Message().Input().Scope, model.DispatchKey(frozen))
	require.NoError(t, err)
	require.Equal(t, previous.Execution.Fence+1, current.Execution.Fence)
	require.Equal(t, previous.Execution.Plan.Digest(), current.Execution.Plan.Digest())
	require.Equal(t, frozen.Message().Input().Payload, current.Execution.Plan.Input().Branches[0].Wire)
	for _, ref := range previous.Execution.References() {
		require.Equal(t, transaction.Rollback, (transaction.StoreChecker{Store: protocol}).Check(t.Context(), ref), "old fence remains retained and Aborted")
	}
	require.Equal(t, transaction.LocalCommitted, owner.Confirm(t.Context(), current.Execution))
	return out
}
