//go:build integration

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/outboxhistory"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestOriginalRetryHistoryInspectAndApplyBoundary(t *testing.T) {
	client, db := mongodbtest.ReplicaSetDatabase(t)
	ctx := t.Context()
	t.Setenv("MONGO_URI", os.Getenv("QS_SERVER_TEST_MONGO_URI"))
	t.Setenv("MONGO_DB", db.Name())
	t.Setenv("M6_RETRY_NSQ_ADDRESS", "127.0.0.1:1")
	source, now := validSource(t)
	runs := db.Collection("interpretation_runs")
	for _, index := range []mongo.IndexModel{
		{Keys: bson.D{{Key: "domain_id", Value: 1}}, Options: options.Index().SetName("uk_interpretation_run_domain_id")},
		{Keys: bson.D{{Key: "generation_id", Value: 1}, {Key: "attempt", Value: 1}}, Options: options.Index().SetName("uk_interpretation_run_generation_attempt")},
	} {
		_, err := runs.Indexes().CreateOne(ctx, index)
		require.NoError(t, err)
	}
	generations := db.Collection("report_generations")
	_, err := generations.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "domain_id", Value: 1}}, Options: options.Index().SetName("uk_generation_domain_id")})
	require.NoError(t, err)
	_, err = runs.InsertOne(ctx, source.Run)
	require.NoError(t, err)
	_, err = generations.InsertOne(ctx, source.Generation)
	require.NoError(t, err)

	row, err := bson.Marshal(source.Intent)
	require.NoError(t, err)
	var original bson.D
	require.NoError(t, bson.Unmarshal(row, &original))
	id := bson.D{{Key: "producer", Value: source.Intent.Producer}, {Key: "message_id", Value: source.Intent.ID}, {Key: "destination", Value: source.Intent.Destination}}
	original = append(bson.D{{Key: "_id", Value: id}}, original...)
	live := db.Collection(outboxhistory.Original)
	archive := db.Collection(outboxhistory.Archive)
	for _, collection := range []*mongo.Collection{live, archive} {
		_, err := collection.InsertOne(ctx, original)
		require.NoError(t, err)
	}
	inspect := func(expected int) []byte {
		t.Helper()
		var output bytes.Buffer
		require.Equal(t, expected, runCLI(ctx, []string{"--run-id=1002", "--org-id=7"}, &output, io.Discard))
		return output.Bytes()
	}
	baseline := inspect(0)
	_, err = live.DeleteOne(ctx, bson.M{"_id": id})
	require.NoError(t, err)
	require.Empty(t, inspect(1), "live collection must not borrow a missing row from the archive")
	_, err = live.InsertOne(ctx, original)
	require.NoError(t, err)
	require.NoError(t, client.Database("admin").RunCommand(ctx, bson.D{
		{Key: "renameCollection", Value: db.Name() + "." + live.Name()},
		{Key: "to", Value: db.Name() + ".isolated_original"},
	}).Err())
	require.Equal(t, baseline, inspect(0))

	plan, err := captureOriginal(ctx, db, outboxhistory.NewReader(ctx, db), 1002, 7, now)
	require.NoError(t, err)
	require.Equal(t, source.Intent.Payload, plan.message.Input().Payload)
	var inspected recoveryPlan
	require.NoError(t, json.Unmarshal(baseline, &inspected))
	require.Equal(t, inspected.SourceFingerprint, plan.SourceFingerprint)
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0700))
	var output, diagnostic bytes.Buffer
	require.Equal(t, 1, runCLI(ctx, []string{"--mode=apply", "--run-id=1002", "--org-id=7",
		"--source-fingerprint=" + plan.SourceFingerprint, "--audit-dir=" + dir,
		"--request-id=" + uuid.NewString(), "--operator=isolated-fixture", "--reason=controlled test",
		"--external-result-reviewed"}, &output, &diagnostic))
	require.Contains(t, diagnostic.String(), "source rejected or unavailable")
	require.Empty(t, output.String())
	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, files)

	_, err = archive.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"fingerprint": []byte("corrupt")}})
	require.NoError(t, err)
	require.Empty(t, inspect(1))
	t.Log("F06_INTERPRETATION_RECOVERY_PROOF inspect_archive_wire_equal=true live_row_missing_refused=true corrupt_archive_refused=true apply_archive_refused_before_journal=true")
}
