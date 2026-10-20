package execution

import (
	"testing"
	"time"

	run "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	"github.com/stretchr/testify/require"
)

func TestCloneRunPreservesOriginalLeaseRecoveryHistory(t *testing.T) {
	at := time.Date(2026, 10, 20, 1, 2, 3, 0, time.UTC)
	r, err := run.NewPending(2, 1, 1)
	require.NoError(t, err)
	require.NoError(t, r.StartWithLease(at.Add(-time.Minute), "original-worker", at.Add(-time.Second)))
	require.NoError(t, r.ReclaimExpiredLease(at, "recovery-worker", at.Add(time.Minute)))
	copy, err := cloneRun(r)
	require.NoError(t, err)
	require.Equal(t, r.ClaimHistory(), copy.ClaimHistory())
	require.Equal(t, r.RecoveryCount(), copy.RecoveryCount())
	require.True(t, r.LastReclaimedAt().Equal(*copy.LastReclaimedAt()))
	require.Equal(t, r.TraceID(), copy.TraceID())
	require.Equal(t, r.Attempt(), copy.Attempt())
	require.True(t, r.LeaseExpiresAt().Equal(*copy.LeaseExpiresAt()))
	history := copy.ClaimHistory()
	history[0].TraceID = "mutated"
	require.Equal(t, "recovery-worker", r.ClaimHistory()[0].TraceID)
}
