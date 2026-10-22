package scheduler

import (
	"context"
	"errors"
	"time"

	"github.com/FangcunMount/component-base/pkg/log"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing/retrydispatch"
	opts "github.com/FangcunMount/qs-server/internal/apiserver/options"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
)

type DueDispatch func(context.Context, time.Time, int) ([]app.Outcome, error)

type RetryDispatchRunner struct {
	runnerLifecycle
	owner  string
	once   DueDispatch
	ready  func() bool
	limits opts.RetryDispatchOptions
}

// NewRetryDispatchRunner creates no work. It borrows the existing dispatcher
// and its owner's admission; it cannot authorize a retry or invoke business work.
func NewRetryDispatchRunner(owner string, once DueDispatch, ready func() bool, limits opts.RetryDispatchOptions) (*RetryDispatchRunner, error) {
	if (owner != "qs.mysql" && owner != "qs.mongo") || once == nil || ready == nil || limits.Interval <= 0 || limits.Budget <= 0 || limits.Batch < 1 || limits.Batch > 50 {
		return nil, errors.New("explicit owner, due dispatcher, admission and bounded scan limits required")
	}
	return &RetryDispatchRunner{owner: owner, once: once, ready: ready, limits: limits}, nil
}
func (r *RetryDispatchRunner) Name() string { return "transaction_retry_dispatch_" + r.owner }
func (r *RetryDispatchRunner) Start(ctx context.Context) {
	if r == nil {
		return
	}
	r.launch(func() {
		ticker := time.NewTicker(r.limits.Interval)
		defer ticker.Stop()
		for {
			if ctx.Err() != nil {
				return
			}
			if r.ready() {
				scan, cancel := context.WithTimeout(ctx, r.limits.Budget)
				outcomes, err := r.once(scan, time.Now().UTC(), r.limits.Batch)
				cancel()
				if ctx.Err() == nil {
					failed := 0
					for _, out := range outcomes {
						if out.Err != nil && !errors.Is(out.Err, eventruntime.ErrAutomaticRetryPaused) {
							failed++
						}
					}
					if err != nil || failed > 0 {
						log.Warnf("transaction retry dispatch incomplete (owner=%s failed=%d)", r.owner, failed)
					}
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	})
}
