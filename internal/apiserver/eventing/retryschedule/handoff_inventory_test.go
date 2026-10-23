package retryschedule

import (
	"math"
	"testing"

	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
)

func TestInventoryDoesNotIgnoreHeldUnknownOrUnconfirmedResponsibilities(t *testing.T) {
	binding := transaction.Binding{Owner: "qs.mysql", StoreID: "original", WriterEpoch: "m3-1"}
	for _, state := range []string{"pending", "retry_wait", "publishing", "quarantined", "held"} {
		v, err := Inventory(binding, map[string]uint64{state: 1, "published": 5}, 0, 0)
		require.NoError(t, err)
		require.False(t, v.ResponsibilitiesAccounted)
		require.EqualValues(t, 6, v.Total)
	}
	v, err := Inventory(binding, map[string]uint64{"held": 1, "published": 5}, 1, 0)
	require.NoError(t, err)
	require.True(t, v.ResponsibilitiesAccounted)
	require.EqualValues(t, 5, v.Counts["published"], "historical rows are retained")
	v, err = Inventory(binding, map[string]uint64{"held": 1, "published": 5}, 1, 1)
	require.NoError(t, err)
	require.False(t, v.ResponsibilitiesAccounted)
	_, err = Inventory(binding, map[string]uint64{"unknown": 1}, 0, 0)
	require.ErrorIs(t, err, ErrInvalid)
	_, err = Inventory(binding, map[string]uint64{"published": 1}, 1, 0)
	require.ErrorIs(t, err, ErrInvalid)
	_, err = Inventory(binding, map[string]uint64{"published": 1}, 0, 2)
	require.ErrorIs(t, err, ErrInvalid)
	_, err = Inventory(binding, map[string]uint64{"published": math.MaxInt64, "pending": 1}, 0, 0)
	require.ErrorIs(t, err, ErrInvalid)
}

func TestInventorySelectionRequiresBoundedUniqueOriginalPlans(t *testing.T) {
	f := reviewFixture(t, Evaluation, "original")
	binding := transaction.Binding{Owner: "qs.mysql", StoreID: "original", WriterEpoch: "m3-1"}
	require.NoError(t, ValidateHandoffSelection(binding, nil))
	require.NoError(t, ValidateHandoffSelection(binding, []Frozen{f}))
	for _, plans := range [][]Frozen{{f, f}, {{}}, make([]Frozen, 101), {reviewFixture(t, Interpretation, "wrong-owner")}} {
		require.ErrorIs(t, ValidateHandoffSelection(binding, plans), ErrInvalid)
	}
}
