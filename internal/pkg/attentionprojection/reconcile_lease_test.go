package attentionprojection

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
)

type reconcileRunnerStub struct {
	acquired bool
	err      error
	calls    int
	workload locklease.WorkloadID
	key      string
}

func (s *reconcileRunnerStub) Run(
	ctx context.Context,
	workload locklease.WorkloadID,
	key string,
	_ time.Duration,
	body func(context.Context) error,
) (locklease.RunResult, error) {
	s.calls++
	s.workload = workload
	s.key = key
	if s.err != nil || !s.acquired {
		return locklease.RunResult{}, s.err
	}
	return locklease.RunResult{Acquired: true}, body(ctx)
}
