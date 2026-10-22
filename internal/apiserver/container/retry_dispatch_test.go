package container

import (
	"context"
	"errors"
	"testing"
	"time"

	port "github.com/FangcunMount/qs-server/internal/apiserver/port/retryschedule"
	"github.com/FangcunMount/reliable-messaging/transaction"
)

// Embedding the unused port methods makes any accidental preflight reservation,
// read of due plans or write fail immediately instead of simulating a database.
type dispatchPreflightStore struct {
	port.Store
	validate func(context.Context) error
}

func (s dispatchPreflightStore) Validate(ctx context.Context) error { return s.validate(ctx) }

func TestRetryDispatchPreflightHasFreshBudgetAndStopsOnBindingError(t *testing.T) {
	wantErr := transaction.ErrBinding
	var first context.Context
	called := 0
	d := &RetryDispatchers{stores: []port.Store{
		dispatchPreflightStore{validate: func(ctx context.Context) error {
			first = ctx
			deadline, bounded := ctx.Deadline()
			if !bounded || time.Until(deadline) > 2*time.Second {
				t.Fatal("unbounded authoritative preflight")
			}
			called++
			return nil
		}},
		dispatchPreflightStore{validate: func(ctx context.Context) error {
			if first.Err() != context.Canceled || ctx.Err() != nil {
				t.Fatal("store read reused previous boundary or leaked its budget")
			}
			called++
			return wantErr
		}},
		dispatchPreflightStore{validate: func(context.Context) error { t.Fatal("scan continued after invalid binding"); return nil }},
	}}
	if err := d.Validate(context.Background()); !errors.Is(err, wantErr) || called != 2 {
		t.Fatal("preflight failure lost", err, called)
	}
}

func TestRetryDispatchRequiresExplicitHostOwnersAndOriginalStores(t *testing.T) {
	for _, host := range []*Container{nil, newBaseContainer(nil, nil, nil), {transactions: &EventTransactionBindings{}}} {
		if _, err := host.BuildRetryDispatchers(true); !errors.Is(err, transaction.ErrBinding) {
			t.Fatal("missing original host accepted", err)
		}
	}
}
