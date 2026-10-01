package container

import (
	"context"
	"testing"

	actorreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
	"github.com/stretchr/testify/require"
)

type reminderTesteeReadStub struct {
	row *actorreadmodel.TesteeRow
}

func (s reminderTesteeReadStub) GetTestee(context.Context, uint64) (*actorreadmodel.TesteeRow, error) {
	return s.row, nil
}

func TestReminderTesteeLookupNeedsNoOperatorSnapshot(t *testing.T) {
	profileID := uint64(42)
	reader := reminderTesteeLookup{reader: reminderTesteeReadStub{row: &actorreadmodel.TesteeRow{
		ID: 12, OrgID: 7, ProfileID: &profileID, Source: "online_form", Name: "private name",
	}}}
	result, err := reader.GetByID(context.Background(), 12)
	require.NoError(t, err)
	require.Equal(t, uint64(12), result.ID)
	require.Equal(t, int64(7), result.OrgID)
	require.Equal(t, &profileID, result.ProfileID)
	require.Equal(t, "online_form", result.Source)
	require.Empty(t, result.Name, "background reminder only needs identity and mock-source facts")
}

func TestReminderTesteeLookupRejectsUnexpectedIdentity(t *testing.T) {
	reader := reminderTesteeLookup{reader: reminderTesteeReadStub{row: &actorreadmodel.TesteeRow{ID: 99}}}
	_, err := reader.GetByID(context.Background(), 12)
	require.Error(t, err)
}
