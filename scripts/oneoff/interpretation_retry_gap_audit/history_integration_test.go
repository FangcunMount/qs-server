//go:build integration

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/outboxhistory"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestRetryGapAuditReadsOriginalThenArchive(t *testing.T) {
	client, db := mongodbtest.ReplicaSetDatabase(t)
	ctx := t.Context()
	t.Setenv("MONGO_URI", os.Getenv("QS_SERVER_TEST_MONGO_URI"))
	t.Setenv("MONGO_DB", db.Name())
	due := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	runs := db.Collection("interpretation_runs")
	generations := db.Collection("report_generations")
	live := db.Collection(outboxhistory.Original)
	archive := db.Collection(outboxhistory.Archive)
	for _, index := range []struct {
		collection *mongo.Collection
		keys       bson.D
		name       string
	}{
		{runs, bson.D{{Key: "domain_id", Value: 1}}, "uk_interpretation_run_domain_id"},
		{runs, bson.D{{Key: "generation_id", Value: 1}, {Key: "attempt", Value: 1}}, "uk_interpretation_run_generation_attempt"},
		{generations, bson.D{{Key: "domain_id", Value: 1}}, "uk_generation_domain_id"},
		{live, bson.D{{Key: "message_id", Value: 1}}, "original_message"},
	} {
		_, err := index.collection.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: index.keys, Options: options.Index().SetName(index.name)})
		require.NoError(t, err)
	}
	_, err := runs.InsertOne(ctx, bson.M{"domain_id": 12, "generation_id": 11, "attempt": 1,
		"status": "failed", "retry_disposition": "automatic", "retry_event_id": "original", "next_attempt_at": due})
	require.NoError(t, err)
	_, err = generations.InsertOne(ctx, bson.M{"domain_id": 11, "status": "failed", "latest_run_id": 12})
	require.NoError(t, err)
	original := bson.D{
		{Key: "_id", Value: bson.D{{Key: "producer", Value: "qs-server"}, {Key: "message_id", Value: "original"}, {Key: "destination", Value: "qs.evaluation.lifecycle"}}},
		{Key: "message_id", Value: "original"}, {Key: "event_type", Value: "interpretation.retry.requested"},
		{Key: "state", Value: "published"}, {Key: "transport_confirmed_at", Value: due},
	}
	for _, collection := range []*mongo.Collection{live, archive} {
		_, err := collection.InsertOne(ctx, original)
		require.NoError(t, err)
	}
	audit := func(exit int, want string) {
		t.Helper()
		var output bytes.Buffer
		require.Equal(t, exit, runCLI(ctx, []string{"--after-id=10", "--upper-id=12", "--due-before=" + due.Format(time.RFC3339)}, nil, &output, io.Discard))
		if exit == 1 {
			require.Empty(t, output.String())
			return
		}
		var got report
		require.NoError(t, json.Unmarshal(output.Bytes(), &got))
		require.True(t, got.Complete)
		require.Equal(t, map[string]int{want: 1}, got.Counts)
		require.Empty(t, got.Findings)
	}
	audit(2, suspectedDeliveryGap)
	_, err = live.DeleteOne(ctx, bson.M{"message_id": "original"})
	require.NoError(t, err)
	audit(2, manualRequired)
	_, err = live.InsertOne(ctx, original)
	require.NoError(t, err)
	require.NoError(t, client.Database("admin").RunCommand(ctx, bson.D{
		{Key: "renameCollection", Value: db.Name() + "." + live.Name()},
		{Key: "to", Value: db.Name() + ".isolated_original"},
	}).Err())
	audit(1, "")
	_, err = archive.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "message_id", Value: 1}}})
	require.NoError(t, err)
	audit(2, suspectedDeliveryGap)
	require.NoError(t, archive.Drop(ctx))
	audit(1, "")
	t.Log("F06_INTERPRETATION_AUDIT_PROOF archive_classification_equal=true live_row_missing_reported=true missing_index_refused=true both_sources_missing_refused=true")
}
