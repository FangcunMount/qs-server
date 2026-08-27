package evaluation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// DailyCapacityReservation is the conservative cost reservation committed
// with one Prompt evaluation start. ProviderInvocations is the maximum number
// of generation plus semantic calls the frozen run can make, not an estimate
// derived from later receipts.
type DailyCapacityReservation struct {
	RunID               meta.ID
	OrgID               int64
	RequestedBy         string
	BudgetDay           time.Time
	ProviderInvocations int
	DailyLimit          int
	ReservedAt          time.Time
}

func (r DailyCapacityReservation) Validate() error {
	requestedBy := strings.TrimSpace(r.RequestedBy)
	if r.RunID.IsZero() || r.OrgID <= 0 || requestedBy == "" || len(requestedBy) > 256 ||
		r.ProviderInvocations < 1 || r.DailyLimit < r.ProviderInvocations || r.ReservedAt.IsZero() ||
		r.BudgetDay.Location() != time.UTC || r.BudgetDay.Hour() != 0 || r.BudgetDay.Minute() != 0 ||
		r.BudgetDay.Second() != 0 || r.BudgetDay.Nanosecond() != 0 || !sameUTCDay(r.BudgetDay, r.ReservedAt) {
		return fmt.Errorf("AI explanation Prompt evaluation daily capacity reservation is invalid")
	}
	return nil
}

// CapacityRepository owns the durable per-organization UTC-day budget. The
// empty bucket may be ensured outside a transaction; Reserve must execute in
// the same Mongo transaction as PromptEvaluationRun and its first Outbox.
type CapacityRepository interface {
	EnsureDailyBucket(context.Context, int64, time.Time, time.Time) error
	ReserveDailyProviderInvocations(context.Context, DailyCapacityReservation) error
}

// DailyCapacityUsage is the authoritative persisted reservation projection
// for one organization and UTC day. The configured daily limit is deliberately
// not persisted here; readers combine this ledger with the current validated
// runtime policy and expose both when config changes.
type DailyCapacityUsage struct {
	OrgID                       int64
	BudgetDay                   time.Time
	ReservedProviderInvocations int
	Reservations                []DailyCapacityUsageReservation
}

type DailyCapacityUsageReservation struct {
	RunID               meta.ID
	RequestedBy         string
	ProviderInvocations int
	ReservedAt          time.Time
}

func (u DailyCapacityUsage) Validate() error {
	if u.OrgID <= 0 || u.BudgetDay.Location() != time.UTC || !u.BudgetDay.Equal(UTCBudgetDay(u.BudgetDay)) || u.ReservedProviderInvocations < 0 {
		return fmt.Errorf("AI explanation Prompt evaluation daily capacity usage is invalid")
	}
	total := 0
	seen := make(map[meta.ID]struct{}, len(u.Reservations))
	for _, reservation := range u.Reservations {
		requestedBy := strings.TrimSpace(reservation.RequestedBy)
		if reservation.RunID.IsZero() || requestedBy == "" || len(requestedBy) > 256 || reservation.ProviderInvocations < 1 || reservation.ReservedAt.IsZero() ||
			!sameUTCDay(u.BudgetDay, reservation.ReservedAt) {
			return fmt.Errorf("AI explanation Prompt evaluation daily capacity usage reservation is invalid")
		}
		if _, duplicate := seen[reservation.RunID]; duplicate {
			return fmt.Errorf("AI explanation Prompt evaluation daily capacity usage reservation is duplicated")
		}
		seen[reservation.RunID] = struct{}{}
		total += reservation.ProviderInvocations
	}
	if total != u.ReservedProviderInvocations {
		return fmt.Errorf("AI explanation Prompt evaluation daily capacity usage total is inconsistent")
	}
	return nil
}

type CapacityReader interface {
	FindDailyCapacityUsage(context.Context, int64, time.Time) (DailyCapacityUsage, bool, error)
}

func UTCBudgetDay(at time.Time) time.Time {
	at = at.UTC()
	return time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC)
}

func sameUTCDay(day, at time.Time) bool {
	return day.Equal(UTCBudgetDay(at))
}
