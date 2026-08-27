package generation

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

func TestParticipantDailyCapacityReservationValidate(t *testing.T) {
	at := time.Date(2026, 8, 27, 13, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	reservation := ParticipantDailyCapacityReservation{
		ReservationID: ParticipantCapacityReservationID(meta.FromUint64(71), 1),
		GenerationID:  meta.FromUint64(71), Attempt: 1, Origin: retrygovernance.AttemptOriginInitial,
		OrgID: 12, UserID: "user-42", AssessmentID: meta.FromUint64(501),
		BudgetDay: ParticipantUTCBudgetDay(at), ProviderInvocations: ParticipantProviderInvocationsPerGenerationV1,
		Policy: ParticipantCapacityPolicy{
			DailyProviderInvocationBudgetPerOrg: 500, DailyProviderInvocationBudgetPerUser: 5,
			DailyProviderInvocationBudgetPerAssessment: 3, MaxActiveProviderExecutionsPerOrg: 10,
			MaxActiveProviderExecutionsPerUser: 2, MaxActiveProviderExecutionsPerAssessment: 1,
		},
		ReservedAt: at,
	}
	if err := reservation.Validate(); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*ParticipantDailyCapacityReservation)
	}{
		{name: "missing user", mutate: func(value *ParticipantDailyCapacityReservation) { value.UserID = "" }},
		{name: "wrong day", mutate: func(value *ParticipantDailyCapacityReservation) { value.BudgetDay = value.BudgetDay.AddDate(0, 0, -1) }},
		{name: "more than one invocation", mutate: func(value *ParticipantDailyCapacityReservation) { value.ProviderInvocations = 2 }},
		{name: "org below user", mutate: func(value *ParticipantDailyCapacityReservation) { value.Policy.DailyProviderInvocationBudgetPerOrg = 4 }},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			invalid := reservation
			testCase.mutate(&invalid)
			if err := invalid.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestParticipantActiveCapacityContractsValidate(t *testing.T) {
	at := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	policy := ParticipantCapacityPolicy{
		DailyProviderInvocationBudgetPerOrg: 500, DailyProviderInvocationBudgetPerUser: 5,
		DailyProviderInvocationBudgetPerAssessment: 3, MaxActiveProviderExecutionsPerOrg: 10,
		MaxActiveProviderExecutionsPerUser: 2, MaxActiveProviderExecutionsPerAssessment: 1,
	}
	slot := ParticipantActiveSlot{
		GenerationID: meta.FromUint64(71), RunID: meta.FromUint64(72), OrgID: 12, UserID: "user-42",
		AssessmentID: meta.FromUint64(501), Policy: policy, AcquiredAt: at,
	}
	if err := slot.Validate(); err != nil {
		t.Fatal(err)
	}
	release := ParticipantActiveSlotRelease{
		GenerationID: slot.GenerationID, RunID: slot.RunID, OrgID: slot.OrgID, UserID: slot.UserID,
		AssessmentID: slot.AssessmentID, ReleasedAt: at.Add(time.Second),
	}
	if err := release.Validate(); err != nil {
		t.Fatal(err)
	}
	usage := ParticipantActiveCapacityUsage{
		OrgID: 12, ActiveExecutions: 1,
		Reservations: []ParticipantActiveCapacityUsageReservation{{
			GenerationID: slot.GenerationID, RunID: slot.RunID, UserID: slot.UserID,
			AssessmentID: slot.AssessmentID, AcquiredAt: slot.AcquiredAt,
		}},
	}
	if err := usage.Validate(); err != nil {
		t.Fatal(err)
	}
	usage.ActiveExecutions = 2
	if err := usage.Validate(); err == nil {
		t.Fatal("expected active capacity drift to fail")
	}
}

func TestParticipantUTCBudgetDay(t *testing.T) {
	at := time.Date(2026, 8, 27, 1, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	want := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	if got := ParticipantUTCBudgetDay(at); !got.Equal(want) || got.Location() != time.UTC {
		t.Fatalf("budget day = %s, want %s", got, want)
	}
}

func TestParticipantDailyCapacityUsageRejectsLedgerDrift(t *testing.T) {
	day := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	usage := ParticipantDailyCapacityUsage{
		OrgID: 12, BudgetDay: day, ReservedProviderInvocations: 1,
		Reservations: []ParticipantDailyCapacityUsageReservation{{
			ReservationID: ParticipantCapacityReservationID(meta.FromUint64(71), 1),
			GenerationID:  meta.FromUint64(71), Attempt: 1, Origin: retrygovernance.AttemptOriginInitial,
			UserID: "user-42", AssessmentID: meta.FromUint64(501),
			ProviderInvocations: 1, ReservedAt: day.Add(time.Hour),
		}},
	}
	if err := usage.Validate(); err != nil {
		t.Fatal(err)
	}
	usage.ReservedProviderInvocations = 2
	if err := usage.Validate(); err == nil {
		t.Fatal("expected inconsistent total to fail")
	}
}

func TestParticipantDailyCapacityUsageAcceptsRedactedReservationsWithoutRefund(t *testing.T) {
	day := ParticipantUTCBudgetDay(time.Now())
	usage := ParticipantDailyCapacityUsage{
		OrgID: 8, BudgetDay: day, ReservedProviderInvocations: 2, RedactedProviderInvocations: 1,
		Reservations: []ParticipantDailyCapacityUsageReservation{{
			ReservationID: "reservation-2", GenerationID: meta.FromUint64(2), Attempt: 1,
			Origin: retrygovernance.AttemptOriginInitial, UserID: "user-2", AssessmentID: meta.FromUint64(20),
			ProviderInvocations: 1, ReservedAt: day.Add(time.Hour),
		}},
	}
	if err := usage.Validate(); err != nil {
		t.Fatalf("redacted participant capacity usage: %v", err)
	}
	usage.RedactedProviderInvocations = 2
	if err := usage.Validate(); err == nil {
		t.Fatal("redacted plus retained reservations exceeding total must fail")
	}
}
