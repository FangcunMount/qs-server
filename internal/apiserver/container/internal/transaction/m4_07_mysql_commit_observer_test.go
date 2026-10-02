//go:build integration && (m4_07_old_chain || m4_07_new_chain)

package transaction

import (
	"context"
	"sync"
	"time"

	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
)

type m407AssessmentCommitKey struct{}

// The runner returns only after the host transaction has committed. This is a
// client observation after COMMIT success, not the database's internal commit time.
type m407CommitObserver struct {
	next apptransaction.Runner
	mu   sync.Mutex
	byID map[uint64][]time.Time
}

func newM407CommitObserver(next apptransaction.Runner) *m407CommitObserver {
	return &m407CommitObserver{next: next, byID: make(map[uint64][]time.Time)}
}

func (o *m407CommitObserver) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	err := o.next.WithinTransaction(ctx, fn)
	if err == nil {
		if id, ok := ctx.Value(m407AssessmentCommitKey{}).(uint64); ok {
			o.mu.Lock()
			o.byID[id] = append(o.byID[id], time.Now())
			o.mu.Unlock()
		}
	}
	return err
}

func (o *m407CommitObserver) times(id uint64) []time.Time {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]time.Time(nil), o.byID[id]...)
}
