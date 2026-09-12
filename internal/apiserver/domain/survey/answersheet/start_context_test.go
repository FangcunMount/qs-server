package answersheet

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestStartContextFreezesOwnershipAndDistinguishesUnknown(t *testing.T) {
	require.Equal(t, StartLegacy, (StartContext{}).State())
	at := time.Date(2026, 9, 12, 9, 0, 0, 0, time.FixedZone("CST", 8*3600))
	store := uint64(10)
	c, err := NewStartContext(1, at, &store, 2)
	require.NoError(t, err)
	store = 20
	require.Equal(t, uint64(10), *c.StoreID())
	*c.StoreID() = 30
	require.Equal(t, uint64(10), *c.StoreID())
	require.Equal(t, StartCaptured, c.State())
	require.True(t, c.StartedAt().Equal(at))
	missing, err := NewStartContext(2, at, nil, 0)
	require.NoError(t, err)
	require.Equal(t, StartUnassigned, missing.State())
	_, err = RestoreStartContext(1, at, nil, 0, 2)
	require.Error(t, err)
	zero := uint64(0)
	_, err = NewStartContext(1, at, &zero, 0)
	require.Error(t, err)
}
