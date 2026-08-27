package generation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// ParticipantActiveSlot is acquired only when a pending Generation starts its
// Provider Run. It remains held across process crashes and lease recovery until
// that exact Run reaches a persisted success or failure terminal state.
type ParticipantActiveSlot struct {
	GenerationID meta.ID
	RunID        meta.ID
	OrgID        int64
	UserID       string
	AssessmentID meta.ID
	Policy       ParticipantCapacityPolicy
	AcquiredAt   time.Time
}

func (s ParticipantActiveSlot) Validate() error {
	userID := strings.TrimSpace(s.UserID)
	if s.GenerationID.IsZero() || s.RunID.IsZero() || s.OrgID <= 0 || userID == "" || len(userID) > 256 ||
		s.AssessmentID.IsZero() || s.AcquiredAt.IsZero() {
		return fmt.Errorf("AI explanation participant active slot is invalid")
	}
	return s.Policy.Validate()
}

type ParticipantActiveSlotRelease struct {
	GenerationID meta.ID
	RunID        meta.ID
	OrgID        int64
	UserID       string
	AssessmentID meta.ID
	ReleasedAt   time.Time
}

func (r ParticipantActiveSlotRelease) Validate() error {
	userID := strings.TrimSpace(r.UserID)
	if r.GenerationID.IsZero() || r.RunID.IsZero() || r.OrgID <= 0 || userID == "" || len(userID) > 256 ||
		r.AssessmentID.IsZero() || r.ReleasedAt.IsZero() {
		return fmt.Errorf("AI explanation participant active slot release is invalid")
	}
	return nil
}

// ParticipantActiveCapacityRepository is the exact distributed concurrency
// ledger. Acquire and Release run inside the same Mongo transactions as their
// corresponding Generation/Run state transitions.
type ParticipantActiveCapacityRepository interface {
	EnsureParticipantActiveBucket(context.Context, int64, time.Time) error
	AcquireParticipantActiveSlot(context.Context, ParticipantActiveSlot) error
	ReleaseParticipantActiveSlot(context.Context, ParticipantActiveSlotRelease) error
}

type ParticipantActiveCapacityUsage struct {
	OrgID            int64
	ActiveExecutions int
	Reservations     []ParticipantActiveCapacityUsageReservation
}

type ParticipantActiveCapacityUsageReservation struct {
	GenerationID meta.ID
	RunID        meta.ID
	UserID       string
	AssessmentID meta.ID
	AcquiredAt   time.Time
}

func (u ParticipantActiveCapacityUsage) Validate() error {
	if u.OrgID <= 0 || u.ActiveExecutions < 0 || u.ActiveExecutions != len(u.Reservations) {
		return fmt.Errorf("AI explanation participant active capacity usage is invalid")
	}
	seenGenerations := make(map[meta.ID]struct{}, len(u.Reservations))
	seenRuns := make(map[meta.ID]struct{}, len(u.Reservations))
	for _, reservation := range u.Reservations {
		userID := strings.TrimSpace(reservation.UserID)
		if reservation.GenerationID.IsZero() || reservation.RunID.IsZero() || userID == "" || len(userID) > 256 ||
			reservation.AssessmentID.IsZero() || reservation.AcquiredAt.IsZero() {
			return fmt.Errorf("AI explanation participant active capacity reservation is invalid")
		}
		if _, duplicate := seenGenerations[reservation.GenerationID]; duplicate {
			return fmt.Errorf("AI explanation participant active Generation reservation is duplicated")
		}
		if _, duplicate := seenRuns[reservation.RunID]; duplicate {
			return fmt.Errorf("AI explanation participant active Run reservation is duplicated")
		}
		seenGenerations[reservation.GenerationID] = struct{}{}
		seenRuns[reservation.RunID] = struct{}{}
	}
	return nil
}

type ParticipantActiveCapacityReader interface {
	FindParticipantActiveCapacityUsage(context.Context, int64) (ParticipantActiveCapacityUsage, bool, error)
}
