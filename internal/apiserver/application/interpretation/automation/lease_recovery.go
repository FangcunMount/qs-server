package automation

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/leasemetrics"
	domaingeneration "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	interpretationrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
)

type LeaseRecoverer interface {
	RecoverExpiredLeases(context.Context, time.Time, int) (int, error)
}

type leaseRecoverer struct {
	reader      interpretationrun.ExpiredLeaseReader
	generations domaingeneration.Repository
	automation  Service
}

func NewLeaseRecoverer(reader interpretationrun.ExpiredLeaseReader, generations domaingeneration.Repository, automation Service) LeaseRecoverer {
	return &leaseRecoverer{reader: reader, generations: generations, automation: automation}
}

func (r *leaseRecoverer) RecoverExpiredLeases(ctx context.Context, now time.Time, limit int) (int, error) {
	if r == nil || r.reader == nil || r.generations == nil || r.automation == nil {
		return 0, fmt.Errorf("interpretation lease recovery is not configured")
	}
	leases, err := r.reader.ListExpiredLeases(ctx, now, limit)
	if err != nil {
		return 0, err
	}
	leasemetrics.ObserveExpiredLeases(len(leases))
	recovered := 0
	for _, lease := range leases {
		generationRecord, err := r.generations.FindByID(ctx, lease.GenerationID)
		if err != nil {
			return recovered, err
		}
		_, err = r.automation.Generate(ctx, GenerateCommand{
			Actor: TrustedServiceActor("lease-recovery"), OutcomeID: generationRecord.Key().OutcomeID, TraceID: "lease-recovery:" + lease.RunID.String(),
		})
		if err != nil {
			if _, durable := FailureFrom(err); !durable {
				return recovered, err
			}
		}
		leasemetrics.ObserveRecovery(lease.LeaseExpiredAt, now)
		recovered++
	}
	return recovered, nil
}
