package evaluation

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestUTCBudgetDayNormalizesNonUTCTime(t *testing.T) {
	at := time.Date(2026, 8, 28, 1, 30, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	want := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	if got := UTCBudgetDay(at); !got.Equal(want) || got.Location() != time.UTC {
		t.Fatalf("UTC budget day = %s (%s), want %s UTC", got, got.Location(), want)
	}
}

func TestDailyCapacityReservationRequiresExactUTCWindowAndLimit(t *testing.T) {
	reservedAt := time.Date(2026, 8, 27, 23, 59, 0, 0, time.UTC)
	valid := DailyCapacityReservation{
		RunID: meta.ID(71), OrgID: 12, RequestedBy: "user:42", BudgetDay: UTCBudgetDay(reservedAt),
		ProviderInvocations: 70, DailyLimit: 140, ReservedAt: reservedAt,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid reservation: %v", err)
	}

	tests := map[string]DailyCapacityReservation{
		"different UTC day": func() DailyCapacityReservation {
			value := valid
			value.BudgetDay = value.BudgetDay.Add(-24 * time.Hour)
			return value
		}(),
		"non-midnight budget day": func() DailyCapacityReservation {
			value := valid
			value.BudgetDay = value.BudgetDay.Add(time.Second)
			return value
		}(),
		"daily limit below reservation": func() DailyCapacityReservation {
			value := valid
			value.DailyLimit = 69
			return value
		}(),
		"missing organization": func() DailyCapacityReservation {
			value := valid
			value.OrgID = 0
			return value
		}(),
	}
	for name, value := range tests {
		t.Run(name, func(t *testing.T) {
			if err := value.Validate(); err == nil {
				t.Fatal("expected invalid reservation")
			}
		})
	}
}

func TestDailyCapacityUsageRequiresAuditableExactTotal(t *testing.T) {
	day := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	usage := DailyCapacityUsage{
		OrgID: 12, BudgetDay: day, ReservedProviderInvocations: 70,
		Reservations: []DailyCapacityUsageReservation{{
			RunID: meta.ID(71), RequestedBy: "user:42", ProviderInvocations: 70, ReservedAt: day.Add(time.Hour),
		}},
	}
	if err := usage.Validate(); err != nil {
		t.Fatalf("valid usage: %v", err)
	}

	usage.ReservedProviderInvocations = 140
	if err := usage.Validate(); err == nil {
		t.Fatal("expected inconsistent reservation total to be rejected")
	}
	usage.ReservedProviderInvocations = 140
	usage.Reservations = append(usage.Reservations, usage.Reservations[0])
	if err := usage.Validate(); err == nil {
		t.Fatal("expected duplicated run reservation to be rejected")
	}
}
