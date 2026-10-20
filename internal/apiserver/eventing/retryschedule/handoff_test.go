package retryschedule

import (
	"math"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/stretchr/testify/require"
)

func TestLegacyTransferRetainsExactIdentityAndRejectsAnyPriorPublishResponsibility(t *testing.T) {
	at := time.Date(2026, 10, 21, 2, 0, 0, 123456789, time.UTC)
	a, in := fixture(t, Evaluation, retrygovernance.AttemptOriginAutomatic, "", "original", at)
	f, err := New(a, in, at)
	require.NoError(t, err)
	fp := f.Message().Fingerprint()
	for _, precision := range []time.Duration{time.Microsecond, time.Millisecond} {
		due := at.Truncate(precision)

		base := LegacySource{Input: f.Message().Input(), Fingerprint: fp[:], State: "pending", NextAttemptAt: due}
		already, err := MatchLegacy(f, base, precision)
		require.NoError(t, err)
		require.False(t, already)
		base.State = "held"
		base.Code = HandoffMarker(f)
		already, err = MatchLegacy(f, base, precision)
		require.NoError(t, err)
		require.True(t, already)
		require.True(t, IsHandoffMarker(base.Code))
		require.LessOrEqual(t, len(base.Code), 128)
		base.State = "pending"
		base.Code = ""
		for _, change := range []struct {
			name  string
			apply func(*LegacySource)
		}{
			{"attempted", func(r *LegacySource) { r.Attempts = 1 }},
			{"failed", func(r *LegacySource) { r.Failures = 1 }},
			{"claimed", func(r *LegacySource) { r.HasClaim = true }},
			{"leased", func(r *LegacySource) { r.HasLease = true }},
			{"confirmed", func(r *LegacySource) { r.HasConfirmation = true }},
			{"manual-replay", func(r *LegacySource) { r.HasManualReplay = true }},
			{"unknown", func(r *LegacySource) { r.State = "quarantined"; r.Code = "publish_unknown" }},
			{"claimed-state", func(r *LegacySource) { r.State = "publishing" }},
			{"wrong-owner-marker", func(r *LegacySource) { r.State = "held"; r.Code = HandoffMarker(f) + "0" }},
			{"exhausted-version", func(r *LegacySource) { r.Version = math.MaxInt64 }},
			{"changed-due", func(r *LegacySource) { r.NextAttemptAt = r.NextAttemptAt.Add(precision) }},
			{"changed-wire", func(r *LegacySource) { r.Input.Payload = []byte("changed") }},
			{"changed-scope", func(r *LegacySource) { r.Input.Scope = "org:8" }},
			{"changed-id", func(r *LegacySource) { r.Input.ID = "replacement" }},
			{"changed-hash", func(r *LegacySource) { r.Fingerprint = []byte{1} }},
		} {
			t.Run(precision.String()+"/"+change.name, func(t *testing.T) {
				r := base
				change.apply(&r)
				_, err := MatchLegacy(f, r, precision)
				require.ErrorIs(t, err, ErrLegacyOwnership)
			})
		}
	}
	for _, code := range []string{"", handoffPrefix, handoffPrefix + "zz", handoffPrefix + "AABB", HandoffMarker(f) + "\n"} {
		require.False(t, IsHandoffMarker(code))
	}
}
