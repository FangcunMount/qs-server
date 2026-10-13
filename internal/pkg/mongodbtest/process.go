//go:build integration

package mongodbtest

import (
	"context"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

const processDatabaseEnv = "QS_SERVER_TEST_PROCESS_MONGO_DATABASE"

// RunTestProcess starts the already compiled test binary in a separate process.
// Only the parent owns the disposable database and drops it after every child exits.
func RunTestProcess(t testing.TB, db *mongo.Database, testName string, env ...string) {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "-test.run=^"+regexp.QuoteMeta(testName)+"$", "-test.v", "-test.count=1")
	cmd.Env = append(os.Environ(), processDatabaseEnv+"="+db.Name(), "QS_SERVER_TEST_PROCESS_PARENT_PID="+strconv.Itoa(os.Getpid()))
	cmd.Env = append(cmd.Env, env...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("restarted test process: %v\n%s", err, output)
	}
	t.Logf("restarted test process:\n%s", output)
}

// ProcessDatabase reconnects a child to the parent's isolated database. It
// deliberately has no Drop cleanup; the parent remains the only database owner.
func ProcessDatabase(t testing.TB) *mongo.Database {
	t.Helper()
	parentPID, err := strconv.Atoi(os.Getenv("QS_SERVER_TEST_PROCESS_PARENT_PID"))
	if err != nil || parentPID <= 0 || parentPID == os.Getpid() {
		t.Fatal("test must run in a distinct child process")
	}
	t.Logf("child pid=%d parent pid=%d", os.Getpid(), parentPID)
	name := os.Getenv(processDatabaseEnv)
	if name == "" || (!strings.Contains(name, "test") && !strings.Contains(name, "contract")) || databaseToken.MatchString(name) {
		t.Fatal("a parent-owned test or contract database is required")
	}
	uri := strings.TrimSpace(os.Getenv(testURIEnv))
	if uri == "" {
		t.Fatalf("%s is required; SKIP is not allowed", testURIEnv)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := client.Disconnect(ctx); err != nil {
			t.Errorf("disconnect child test Mongo: %v", err)
		}
	})
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		t.Fatal(err)
	}
	var hello struct {
		SetName string `bson:"setName"`
	}
	if err := client.Database("admin").RunCommand(ctx, map[string]any{"hello": 1}).Decode(&hello); err != nil || hello.SetName == "" {
		t.Fatalf("child requires a real Replica Set: set=%q err=%v", hello.SetName, err)
	}
	return client.Database(name)
}
