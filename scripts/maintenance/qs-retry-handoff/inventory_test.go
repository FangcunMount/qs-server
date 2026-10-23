package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	model "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
)

type inventoryHostFixture struct {
	hostFixture
	snapshot    model.LegacyInventory
	inspectErr  error
	selected    int
	inspections int
}

func (h *inventoryHostFixture) Inspect(_ context.Context, selected []model.Frozen) (model.LegacyInventory, error) {
	h.selected = len(selected)
	h.inspections++
	return h.snapshot, h.inspectErr
}

func TestInventoryKeepsUnreviewedHeldAndPendingResponsibilitiesVisible(t *testing.T) {
	for _, tc := range []struct {
		name     string
		counts   map[string]uint64
		selected bool
		verified uint64
		pending  bool
	}{
		{"old-pending", map[string]uint64{"pending": 1}, false, 0, true},
		{"held-unreviewed", map[string]uint64{"held": 1}, false, 0, true},
		{"held-reviewed", map[string]uint64{"held": 1}, true, 1, false},
		{"empty", nil, false, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			review, args, receipt := fixture(t)
			snapshot, err := model.Inventory(review.Binding(), tc.counts, tc.verified, 0)
			require.NoError(t, err)
			h := &inventoryHostFixture{snapshot: snapshot}
			closed := 0
			open := func(context.Context, arguments, func(string) string) (executor, func() error, error) {
				return h, func() error { closed++; return nil }, nil
			}
			selection := args[4:]
			if tc.selected {
				selection = args
			}
			inv := append(append([]string(nil), selection...), "--inventory", "--database", "original-db", "--receipt", receipt)
			var output bytes.Buffer
			err = run(t.Context(), inv, noEnvironment, &output, open)
			if tc.pending {
				require.ErrorIs(t, err, errInventoryPending)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, 1, closed)
			require.Equal(t, 1, h.inspections)
			require.Zero(t, h.calls, "inventory must never invoke transfer")
			if tc.selected {
				require.Equal(t, 1, h.selected)
			} else {
				require.Zero(t, h.selected)
			}
			raw, err := os.ReadFile(receipt)
			require.NoError(t, err)
			lines := bytes.Split(bytes.TrimSpace(raw), []byte("\n"))
			require.Len(t, lines, 2)
			var header, final map[string]any
			require.NoError(t, json.Unmarshal(lines[0], &header))
			require.NoError(t, json.Unmarshal(lines[1], &final))
			require.Equal(t, true, header["read_only"])
			require.Equal(t, false, final["cutover_authorized"])
			require.NotContains(t, output.String(), "private-event-id")
			require.NotContains(t, output.String(), "original-primary")
		})
	}
}

func TestInventoryUnavailableOrWrongBindingNeverBecomesEmptySuccess(t *testing.T) {
	for _, failure := range []string{"database", "binding", "cleanup"} {
		t.Run(failure, func(t *testing.T) {
			review, args, receipt := fixture(t)
			binding := review.Binding()
			if failure == "binding" {
				binding.StoreID = "wrong-primary"
			}
			snapshot, err := model.Inventory(binding, nil, 0, 0)
			require.NoError(t, err)
			h := &inventoryHostFixture{snapshot: snapshot}
			if failure == "database" {
				h.inspectErr = context.DeadlineExceeded
			}
			closed := 0
			open := func(context.Context, arguments, func(string) string) (executor, func() error, error) {
				return h, func() error {
					closed++
					if failure == "cleanup" {
						return errors.New("controlled cleanup failure")
					}
					return nil
				}, nil
			}
			var output bytes.Buffer
			err = run(t.Context(), append(append([]string(nil), args[4:]...), "--inventory", "--database", "original-db", "--receipt", receipt), noEnvironment, &output, open)
			require.Error(t, err)
			if failure == "binding" {
				require.ErrorIs(t, err, transaction.ErrBinding)
			}
			require.Empty(t, output.String())
			require.Equal(t, 1, closed)
			raw, err := os.ReadFile(receipt)
			require.NoError(t, err)
			require.Contains(t, string(raw), "inventory_unavailable")
		})
	}
}
