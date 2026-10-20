package retryschedule

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/stretchr/testify/require"
)

func TestDispatchRequiresUnchangedDecisionAndExplicitStoredDeadlinePrecision(t *testing.T) {
	at := time.Date(2026, 10, 21, 0, 0, 0, 123499999, time.UTC)
	a, in := fixture(t, Evaluation, retrygovernance.AttemptOriginAutomatic, "", "first", at)
	d := retrygovernance.Decision{Disposition: retrygovernance.DispositionAutomatic, Attempt: 1, MaxAutomaticAttempts: 3, RemainingAutomaticAttempts: 2, NextAttemptAt: &at, PolicyVersion: "business-retry/v1", RetryEventID: in.ID}
	digest, err := DecisionDigest(&d)
	require.NoError(t, err)
	a.DecisionHash = digest
	f, err := New(a, in, at)
	require.NoError(t, err)
	require.NoError(t, f.MatchDecision(&d, at))
	for _, precision := range []string{"mysql", "mongo"} {
		t.Run(precision, func(t *testing.T) {
			stored := at.Truncate(time.Millisecond)
			if precision == "mysql" {
				stored = at.Round(time.Microsecond).Round(time.Millisecond)
			}
			original := d
			original.NextAttemptAt = &stored
			require.NoError(t, f.MatchDecision(&original, stored))
			require.ErrorIs(t, f.MatchDecision(&original, stored.Add(time.Millisecond)), ErrConflict)
			changed := original
			changed.ActionRequestID = "unapproved-action"
			require.ErrorIs(t, f.MatchDecision(&changed, stored), ErrConflict)
			changed = original
			changed.PolicyVersion = "other-policy"
			require.ErrorIs(t, f.MatchDecision(&changed, stored), ErrConflict)
			changed = original
			changed.Disposition = retrygovernance.DispositionManualRequired
			require.ErrorIs(t, f.MatchDecision(&changed, stored), ErrConflict)
			changed = original
			changed.RetryEventID = "other-first-wire"
			require.ErrorIs(t, f.MatchDecision(&changed, stored), ErrConflict)
		})
	}
}
