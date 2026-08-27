package generation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

const (
	ParticipantProviderInvocationsPerAttemptV1    = 1
	ParticipantProviderInvocationsPerGenerationV1 = ParticipantProviderInvocationsPerAttemptV1
)

// ParticipantCapacityPolicy is the conservative UTC-day admission policy for
// participant Provider attempts. Exact semantic reuse never reserves another
// invocation; every governed retry must reserve its next attempt first.
type ParticipantCapacityPolicy struct {
	DailyProviderInvocationBudgetPerOrg        int
	DailyProviderInvocationBudgetPerUser       int
	DailyProviderInvocationBudgetPerAssessment int
	MaxActiveProviderExecutionsPerOrg          int
	MaxActiveProviderExecutionsPerUser         int
	MaxActiveProviderExecutionsPerAssessment   int
}

func (p ParticipantCapacityPolicy) Validate() error {
	if p.DailyProviderInvocationBudgetPerOrg < ParticipantProviderInvocationsPerGenerationV1 ||
		p.DailyProviderInvocationBudgetPerUser < ParticipantProviderInvocationsPerGenerationV1 ||
		p.DailyProviderInvocationBudgetPerAssessment < ParticipantProviderInvocationsPerGenerationV1 ||
		p.DailyProviderInvocationBudgetPerOrg < p.DailyProviderInvocationBudgetPerUser ||
		p.DailyProviderInvocationBudgetPerOrg < p.DailyProviderInvocationBudgetPerAssessment ||
		p.MaxActiveProviderExecutionsPerOrg < 1 || p.MaxActiveProviderExecutionsPerUser < 1 ||
		p.MaxActiveProviderExecutionsPerAssessment < 1 ||
		p.MaxActiveProviderExecutionsPerOrg < p.MaxActiveProviderExecutionsPerUser ||
		p.MaxActiveProviderExecutionsPerOrg < p.MaxActiveProviderExecutionsPerAssessment {
		return fmt.Errorf("AI explanation participant capacity policy is invalid")
	}
	return nil
}

// ParticipantDailyCapacityReservation is committed atomically with one new
// Generation and its requested Outbox event. BudgetDay is always UTC midnight.
type ParticipantDailyCapacityReservation struct {
	ReservationID       string
	GenerationID        meta.ID
	Attempt             int
	Origin              retrygovernance.AttemptOrigin
	OrgID               int64
	UserID              string
	AssessmentID        meta.ID
	BudgetDay           time.Time
	ProviderInvocations int
	Policy              ParticipantCapacityPolicy
	ReservedAt          time.Time
}

func (r ParticipantDailyCapacityReservation) Validate() error {
	reservationID := strings.TrimSpace(r.ReservationID)
	userID := strings.TrimSpace(r.UserID)
	if reservationID == "" || len(reservationID) > 512 || r.GenerationID.IsZero() || r.Attempt < 1 || !r.Origin.IsValid() ||
		r.OrgID <= 0 || userID == "" || len(userID) > 256 || r.AssessmentID.IsZero() ||
		r.ProviderInvocations != ParticipantProviderInvocationsPerAttemptV1 || r.ReservedAt.IsZero() ||
		r.BudgetDay.Location() != time.UTC || !r.BudgetDay.Equal(ParticipantUTCBudgetDay(r.BudgetDay)) ||
		!r.BudgetDay.Equal(ParticipantUTCBudgetDay(r.ReservedAt)) {
		return fmt.Errorf("AI explanation participant daily capacity reservation is invalid")
	}
	return r.Policy.Validate()
}

// ParticipantCapacityRepository owns the exact organization/user/Assessment
// reservation ledger. Reserve must run in the same Mongo transaction as the
// Generation and requested Outbox event.
type ParticipantCapacityRepository interface {
	EnsureParticipantDailyBucket(context.Context, int64, time.Time, time.Time) error
	ReserveParticipantDailyProviderInvocations(context.Context, ParticipantDailyCapacityReservation) error
}

type ParticipantDailyCapacityUsage struct {
	OrgID                       int64
	BudgetDay                   time.Time
	ReservedProviderInvocations int
	RedactedProviderInvocations int
	Reservations                []ParticipantDailyCapacityUsageReservation
}

type ParticipantDailyCapacityUsageReservation struct {
	ReservationID       string
	GenerationID        meta.ID
	Attempt             int
	Origin              retrygovernance.AttemptOrigin
	UserID              string
	AssessmentID        meta.ID
	ProviderInvocations int
	ReservedAt          time.Time
}

func (u ParticipantDailyCapacityUsage) Validate() error {
	if u.OrgID <= 0 || u.BudgetDay.Location() != time.UTC || !u.BudgetDay.Equal(ParticipantUTCBudgetDay(u.BudgetDay)) ||
		u.ReservedProviderInvocations < 0 || u.RedactedProviderInvocations < 0 ||
		u.RedactedProviderInvocations > u.ReservedProviderInvocations {
		return fmt.Errorf("AI explanation participant daily capacity usage is invalid")
	}
	total := 0
	seenReservations := make(map[string]struct{}, len(u.Reservations))
	seenAttempts := make(map[string]struct{}, len(u.Reservations))
	for _, reservation := range u.Reservations {
		reservationID := strings.TrimSpace(reservation.ReservationID)
		userID := strings.TrimSpace(reservation.UserID)
		if reservationID == "" || len(reservationID) > 512 || reservation.GenerationID.IsZero() || reservation.Attempt < 1 || !reservation.Origin.IsValid() ||
			userID == "" || len(userID) > 256 || reservation.AssessmentID.IsZero() ||
			reservation.ProviderInvocations != ParticipantProviderInvocationsPerAttemptV1 || reservation.ReservedAt.IsZero() ||
			!u.BudgetDay.Equal(ParticipantUTCBudgetDay(reservation.ReservedAt)) {
			return fmt.Errorf("AI explanation participant daily capacity usage reservation is invalid")
		}
		if _, duplicate := seenReservations[reservationID]; duplicate {
			return fmt.Errorf("AI explanation participant daily capacity usage reservation is duplicated")
		}
		attemptKey := fmt.Sprintf("%s:%d", reservation.GenerationID, reservation.Attempt)
		if _, duplicate := seenAttempts[attemptKey]; duplicate {
			return fmt.Errorf("AI explanation participant daily capacity attempt is duplicated")
		}
		seenReservations[reservationID] = struct{}{}
		seenAttempts[attemptKey] = struct{}{}
		total += reservation.ProviderInvocations
	}
	if total+u.RedactedProviderInvocations != u.ReservedProviderInvocations {
		return fmt.Errorf("AI explanation participant daily capacity usage total is inconsistent")
	}
	return nil
}

func ParticipantCapacityReservationID(generationID meta.ID, attempt int) string {
	return fmt.Sprintf("ai-explanation:%s:attempt:%d", generationID, attempt)
}

type ParticipantCapacityReader interface {
	FindParticipantDailyCapacityUsage(context.Context, int64, time.Time) (ParticipantDailyCapacityUsage, bool, error)
}

func ParticipantUTCBudgetDay(at time.Time) time.Time {
	at = at.UTC()
	return time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC)
}
