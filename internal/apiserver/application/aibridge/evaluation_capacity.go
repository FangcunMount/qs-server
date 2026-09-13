package aibridge

import (
	"context"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
)

type EvaluationCapacityReservation struct {
	RunID         string `json:"run_id"`
	ProviderCalls int64  `json:"provider_calls"`
	RequestedBy   string `json:"requested_by"`
	ReservedAt    string `json:"reserved_at"`
}
type EvaluationCapacity struct {
	OrganizationID         int64                           `json:"organization_id"`
	BudgetDay              string                          `json:"budget_day"`
	DailyProviderCalls     int64                           `json:"daily_provider_calls"`
	ReservedProviderCalls  int64                           `json:"reserved_provider_calls"`
	RemainingProviderCalls int64                           `json:"remaining_provider_calls"`
	FullRunProviderCalls   int64                           `json:"full_run_provider_calls"`
	RemainingFullRuns      int64                           `json:"remaining_full_runs"`
	MaxActiveRuns          int64                           `json:"max_active_runs"`
	ActiveRuns             int64                           `json:"active_runs"`
	ReservationCount       int64                           `json:"reservation_count"`
	Reservations           []EvaluationCapacityReservation `json:"reservations"`
	ReservationsTruncated  bool                            `json:"reservations_truncated"`
}
type EvaluationCapacityGateway interface {
	GetEvaluationCapacity(context.Context, DraftScope) (EvaluationCapacity, error)
}

func (s *EvaluationAdministration) Capacity(ctx context.Context, scope DraftScope) (EvaluationCapacity, error) {
	if scope.OrganizationID <= 0 || scope.OperatorUserID <= 0 {
		return EvaluationCapacity{}, ErrInvalid
	}
	snapshot, ok := authz.FromContext(ctx)
	if !ok || !authz.DecideCapability(snapshot, authz.CapabilityOrgAdmin).Allowed {
		return EvaluationCapacity{}, ErrGovernanceDenied
	}
	if s == nil {
		return EvaluationCapacity{}, ErrManagementUnavailable
	}
	gateway, ok := s.Gateway.(EvaluationCapacityGateway)
	if !ok {
		return EvaluationCapacity{}, ErrManagementUnavailable
	}
	return gateway.GetEvaluationCapacity(ctx, scope)
}
