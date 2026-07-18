package scheduler

import (
	"context"
	"time"
)

// NewGovernedService composes the existing read-only consistency audit with
// bounded Evaluation and Interpretation lease recovery under the same HA
// scheduler tick. Recovery keeps each original attempt number.
func NewGovernedService(base Service, recoverers ...LeaseRecoverer) Service {
	filtered := make([]LeaseRecoverer, 0, len(recoverers))
	for _, recoverer := range recoverers {
		if recoverer != nil {
			filtered = append(filtered, recoverer)
		}
	}
	if len(filtered) == 0 {
		return base
	}
	return &governedService{base: base, recoverers: filtered, now: time.Now}
}

type governedService struct {
	base       Service
	recoverers []LeaseRecoverer
	now        func() time.Time
}

func (s *governedService) AuditOnce(ctx context.Context, limit int) (int, error) {
	total := 0
	if s.base != nil {
		count, err := s.base.AuditOnce(ctx, limit)
		if err != nil {
			return total, err
		}
		total += count
	}
	for _, recoverer := range s.recoverers {
		count, err := recoverer.RecoverExpiredLeases(ctx, s.now(), limit)
		if err != nil {
			return total, err
		}
		total += count
	}
	return total, nil
}
