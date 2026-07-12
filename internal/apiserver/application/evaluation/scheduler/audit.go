// Package scheduler contains read-only Evaluation maintenance use cases.
package scheduler

import (
	"context"

	legacy "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/consistency"
)

type Service interface {
	AuditOnce(context.Context, int) (int, error)
}

type service struct{ legacy legacy.Service }

func NewService(legacy legacy.Service) Service { return &service{legacy: legacy} }
func (s *service) AuditOnce(ctx context.Context, limit int) (int, error) {
	return s.legacy.ReconcileOnce(ctx, limit)
}
