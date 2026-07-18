package scheduler

import (
	"context"
	"fmt"
	"time"

	evaluationworker "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/worker"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
)

type LeaseRecoverer interface {
	RecoverExpiredLeases(context.Context, time.Time, int) (int, error)
}

type leaseRecoverer struct {
	reader evaluationrun.ExpiredLeaseReader
	worker evaluationworker.Service
}

func NewLeaseRecoverer(reader evaluationrun.ExpiredLeaseReader, worker evaluationworker.Service) LeaseRecoverer {
	return &leaseRecoverer{reader: reader, worker: worker}
}

func (r *leaseRecoverer) RecoverExpiredLeases(ctx context.Context, now time.Time, limit int) (int, error) {
	if r == nil || r.reader == nil || r.worker == nil {
		return 0, fmt.Errorf("evaluation lease recovery is not configured")
	}
	leases, err := r.reader.ListExpiredLeases(ctx, now, limit)
	if err != nil {
		return 0, err
	}
	recovered := 0
	for _, lease := range leases {
		if _, err := r.worker.Execute(ctx, evaluationworker.Command{AssessmentID: lease.AssessmentID}); err != nil {
			return recovered, err
		}
		recovered++
	}
	return recovered, nil
}
