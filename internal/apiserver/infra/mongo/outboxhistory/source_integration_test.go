//go:build integration

package outboxhistory

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func TestHistoryReaderRejectsCollectionIdentityChange(t *testing.T) {
	client, db := mongodbtest.ReplicaSetDatabase(t)
	ctx := t.Context()
	require.NoError(t, db.CreateCollection(ctx, Archive))
	reader := NewReader(ctx, db)
	collection, err := reader.Collection()
	require.NoError(t, err)
	require.Equal(t, Archive, collection.Name())
	require.NoError(t, reader.Verify())
	require.NoError(t, client.Database("admin").RunCommand(ctx, bson.D{
		{Key: "renameCollection", Value: db.Name() + "." + Archive},
		{Key: "to", Value: db.Name() + ".isolated_retained_history"},
	}).Err())
	require.NoError(t, db.CreateCollection(ctx, Archive))
	require.ErrorIs(t, reader.Verify(), ErrIdentity)

	reader = NewReader(ctx, db)
	_, err = reader.Collection()
	require.NoError(t, err)
	require.NoError(t, db.CreateCollection(ctx, Original))
	require.ErrorIs(t, reader.Verify(), ErrIdentity)
	fresh, err := NewReader(ctx, db).Collection()
	require.NoError(t, err)
	require.Equal(t, Original, fresh.Name())
	t.Log("F06_MONGO_IDENTITY_PROOF replaced_uuid_refused=true changed_source_priority_refused=true")
}
