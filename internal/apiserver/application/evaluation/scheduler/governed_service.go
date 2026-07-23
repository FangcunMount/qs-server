package scheduler

import (
	"context"
	"time"
)

// NewGovernedService composes the existing read-only consistency audit with
// bounded Evaluation and Interpretation lease recovery under the same HA
// scheduler tick. Recovery keeps each original attempt number.
func NewGovernedService(base Service, recoverers ...LeaseRecoverer) Service {
	return NewGovernedServiceWithAuditors(base, nil, recoverers...)
}

// ConsistencyAuditor contributes a read-only audit to the shared HA scheduler.
type ConsistencyAuditor interface {
	AuditOnce(context.Context, int) (int, error)
}

// NewGovernedServiceWithAuditors composes read-only consistency audits before
// bounded lease recovery under one leader-elected scheduler tick.
func NewGovernedServiceWithAuditors(base Service, auditors []ConsistencyAuditor, recoverers ...LeaseRecoverer) Service {
	filtered := make([]LeaseRecoverer, 0, len(recoverers))
	for _, recoverer := range recoverers {
		if recoverer != nil {
			filtered = append(filtered, recoverer)
		}
	}
	filteredAuditors := make([]ConsistencyAuditor, 0, len(auditors))
	for _, auditor := range auditors {
		if auditor != nil {
			filteredAuditors = append(filteredAuditors, auditor)
		}
	}
	if len(filtered) == 0 && len(filteredAuditors) == 0 {
		return base
	}
	return &governedService{base: base, auditors: filteredAuditors, recoverers: filtered, now: time.Now}
}

type governedService struct {
	base       Service
	auditors   []ConsistencyAuditor
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
	for _, auditor := range s.auditors {
		count, err := auditor.AuditOnce(ctx, limit)
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
