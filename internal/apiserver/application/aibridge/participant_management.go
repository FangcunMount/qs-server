package aibridge

import (
	"context"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"unicode/utf8"
)

type ParticipantCapacityQuery struct {
	SubjectID    string `json:"subject_id"`
	AssessmentID string `json:"assessment_id"`
}
type ParticipantCapacityPolicy struct {
	DailyOrg         int64 `json:"daily_org"`
	DailyUser        int64 `json:"daily_user"`
	DailyAssessment  int64 `json:"daily_assessment"`
	ActiveOrg        int64 `json:"active_org"`
	ActiveUser       int64 `json:"active_user"`
	ActiveAssessment int64 `json:"active_assessment"`
}
type ParticipantCapacityUsage struct {
	Identity        string `json:"identity"`
	DailyReserved   int64  `json:"daily_reserved"`
	DailyRemaining  int64  `json:"daily_remaining"`
	Active          int64  `json:"active"`
	ActiveRemaining int64  `json:"active_remaining"`
}
type ParticipantReservation struct {
	RunID         string   `json:"run_id"`
	SessionID     string   `json:"session_id"`
	SubjectID     string   `json:"subject_id"`
	AssessmentIDs []string `json:"assessment_ids"`
	BudgetDay     string   `json:"budget_day"`
	ReservedAt    string   `json:"reserved_at"`
	Active        bool     `json:"active"`
	AcquiredAt    string   `json:"acquired_at"`
}
type ParticipantCapacity struct {
	OrganizationID     int64                     `json:"organization_id"`
	BudgetDay          string                    `json:"budget_day"`
	Policy             ParticipantCapacityPolicy `json:"policy"`
	Organization       ParticipantCapacityUsage  `json:"organization"`
	Subject            *ParticipantCapacityUsage `json:"subject,omitempty"`
	Assessment         *ParticipantCapacityUsage `json:"assessment,omitempty"`
	DailyReservations  []ParticipantReservation  `json:"daily_reservations"`
	ActiveReservations []ParticipantReservation  `json:"active_reservations"`
	DailyTruncated     bool                      `json:"daily_truncated"`
	ActiveTruncated    bool                      `json:"active_truncated"`
}
type ParticipantManagementGateway interface {
	GetParticipantCapacity(context.Context, DraftScope, ParticipantCapacityQuery) (ParticipantCapacity, error)
	GetParticipantExecution(context.Context, DraftScope, string) (ParticipantExecution, error)
	RetryParticipant(context.Context, DraftScope, string, ParticipantRetry) (Receipt, error)
	GetParticipantRetryReceipt(context.Context, DraftScope, string) (Receipt, error)
}
type ParticipantAdministration struct{ Gateway ParticipantManagementGateway }

func (s *ParticipantAdministration) Capacity(ctx context.Context, scope DraftScope, query ParticipantCapacityQuery) (ParticipantCapacity, error) {
	if scope.OrganizationID <= 0 || scope.OperatorUserID <= 0 || len(query.SubjectID) > 128 || !utf8.ValidString(query.SubjectID) ||
		(query.AssessmentID != "" && !validNumber(query.AssessmentID)) {
		return ParticipantCapacity{}, ErrInvalid
	}
	for _, c := range query.SubjectID {
		if c < 32 {
			return ParticipantCapacity{}, ErrInvalid
		}
	}
	snapshot, ok := authz.FromContext(ctx)
	if !ok || !authz.DecideCapability(snapshot, authz.CapabilityOrgAdmin).Allowed {
		return ParticipantCapacity{}, ErrGovernanceDenied
	}
	if s == nil || s.Gateway == nil {
		return ParticipantCapacity{}, ErrManagementUnavailable
	}
	return s.Gateway.GetParticipantCapacity(ctx, scope, query)
}
