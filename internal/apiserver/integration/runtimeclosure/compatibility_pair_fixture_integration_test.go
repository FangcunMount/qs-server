//go:build integration

package runtimeclosure

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"
	"time"

	migrationpkg "github.com/FangcunMount/qs-server/internal/pkg/migration"
	buildversion "github.com/FangcunMount/qs-server/pkg/version"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"gorm.io/gorm"
)

const runtimeFixtureSQLHead uint = 99
const runtimeFixtureMongoHead uint = 38

// This prepares only the random databases allocated by the runtime test. The
// test supplies an independently pinned build SHA and consumes the same private
// authorization protocol as B startup; it never changes the product guard.
func migrateRuntimeCompatibilityPair(t *testing.T, sqlDB *sql.DB, sqlName string, client *mongo.Client, mongoDB *mongo.Database) {
	t.Helper()
	sourceSHA, err := runtimeFixtureSourceSHA(buildversion.Get().GitCommit, os.Getenv("GITHUB_SHA"), os.Getenv("QS_RUNTIME_CLOSURE_APPROVED_SOURCE_SHA"))
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^qs_runtime_closure_[0-9]+$`).MatchString(sqlName) || mongoDB.Client() != client {
		t.Fatal("runtime fixture selected database ownership mismatch")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	requireRuntimePristineCatalog(t, ctx, sqlDB, mongoDB)
	cfg := migrationpkg.PairConfig{MySQLDatabase: sqlName, MongoDatabase: mongoDB.Name(), ExpectedSourceSHA: sourceSHA}
	// The product's complete catalog/permission checks must reach only the
	// missing-authorization gate; no migration metadata may be created here.
	if _, err := migrationpkg.PreflightCompatibilityPair(ctx, sqlDB, client, cfg); err == nil || err.Error() != "compatibility retirement: pristine bootstrap explicit authorization required" {
		t.Fatalf("unapproved pristine pair must fail only at authorization gate: %v", err)
	}
	requireRuntimePristineCatalog(t, ctx, sqlDB, mongoDB)
	sqlIdentity, mongoIdentity := runtimeFixtureIdentities(t, ctx, sqlDB, sqlName, client, mongoDB.Name())
	op := fmt.Sprintf("%d-1", time.Now().UnixNano())
	resources := migrationpkg.CompatibilityMigrationResourcesSHA256()
	approval, err := json.Marshal([]any{"runtime_closure_random_pristine_fixture_v1", sourceSHA, op, sqlName, mongoDB.Name(), sqlIdentity, mongoIdentity, runtimeFixtureSQLHead, runtimeFixtureMongoHead, resources})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(struct {
		FormatVersion            int    `json:"format_version"`
		Kind                     string `json:"kind"`
		OperationID              string `json:"operation_id"`
		ExpiresAt                string `json:"expires_at"`
		ApprovedSourceSHA        string `json:"approved_source_sha"`
		ApprovalSummarySHA256    string `json:"approval_summary_sha256"`
		MySQLDatabase            string `json:"mysql_database"`
		MongoDatabase            string `json:"mongo_database"`
		MySQLIdentitySHA256      string `json:"mysql_identity_sha256"`
		MongoClusterSHA256       string `json:"mongo_cluster_sha256"`
		MySQLVersion             uint   `json:"mysql_version"`
		MongoVersion             uint   `json:"mongo_version"`
		MigrationResourcesSHA256 string `json:"migration_resources_sha256"`
	}{1, "qs_compatibility_retirement_b_pristine_bootstrap", op, time.Now().UTC().Add(30 * time.Minute).Format(time.RFC3339), sourceSHA, runtimeFixtureHash(approval), sqlName, mongoDB.Name(), sqlIdentity, mongoIdentity, runtimeFixtureSQLHead, runtimeFixtureMongoHead, resources})
	if err != nil {
		t.Fatal(err)
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	cfg.BootstrapAuthorizationFile = filepath.Join(dir, "runtime-pristine-authorization.json")
	cfg.BootstrapAuthorizationSHA256 = runtimeFixtureHash(raw)
	writeRuntimeFixtureAuthorization(t, cfg.BootstrapAuthorizationFile, raw)
	pair, err := migrationpkg.PreflightCompatibilityPair(ctx, sqlDB, client, cfg)
	if err != nil {
		t.Fatalf("approved pristine pair preflight: %v", err)
	}
	sqlVersion, sqlChanged, err := migrationpkg.NewMigrator(sqlDB, pair.MySQLConfig(false)).Run()
	if err != nil || !sqlChanged || sqlVersion != runtimeFixtureSQLHead {
		t.Fatalf("paired pristine MySQL Up: version=%d changed=%v err=%v", sqlVersion, sqlChanged, err)
	}
	consumed, err := os.ReadFile(cfg.BootstrapAuthorizationFile + ".consumed")
	wantConsumed, marshalErr := json.Marshal([]string{op, cfg.BootstrapAuthorizationSHA256})
	if err != nil || marshalErr != nil || !bytes.Equal(consumed, wantConsumed) {
		t.Fatal("private bootstrap authorization consumption proof mismatch")
	}
	mongoVersion, mongoChanged, err := migrationpkg.NewMongoMigrator(client, pair.MongoConfig(false)).Run()
	if err != nil || !mongoChanged || mongoVersion != runtimeFixtureMongoHead {
		t.Fatalf("paired pristine MongoDB Up: version=%d changed=%v err=%v", mongoVersion, mongoChanged, err)
	}
	// A restarted application must reobserve installed heads and absence of all
	// four targets. Reusing the cached pristine authorization is not a restart.
	fresh, err := migrationpkg.PreflightCompatibilityPair(t.Context(), sqlDB, client, migrationpkg.PairConfig{MySQLDatabase: sqlName, MongoDatabase: mongoDB.Name(), ExpectedSourceSHA: sourceSHA})
	if err != nil {
		t.Fatalf("fresh installed pair preflight: %v", err)
	}
	if version, changed, err := migrationpkg.NewMigrator(sqlDB, fresh.MySQLConfig(false)).Run(); err != nil || changed || version != runtimeFixtureSQLHead {
		t.Fatalf("repeat installed MySQL Up: version=%d changed=%v err=%v", version, changed, err)
	}
	if version, changed, err := migrationpkg.NewMongoMigrator(client, fresh.MongoConfig(false)).Run(); err != nil || changed || version != runtimeFixtureMongoHead {
		t.Fatalf("repeat installed MongoDB Up: version=%d changed=%v err=%v", version, changed, err)
	}
	t.Log("runtime fixture: real pristine pair approved/consumed; SQL99/Mongo38 clean and four retired objects absent; fresh installed startup passed")
}

func runtimeFixtureSourceSHA(compiled, githubSHA, approvedSHA string) (string, error) {
	valid := regexp.MustCompile(`^[0-9a-f]{40}$`)
	if !valid.MatchString(compiled) || (githubSHA == "" && approvedSHA == "") || (githubSHA != "" && (!valid.MatchString(githubSHA) || githubSHA != compiled)) || (approvedSHA != "" && (!valid.MatchString(approvedSHA) || approvedSHA != compiled)) {
		return "", fmt.Errorf("runtime fixture requires compiled GitCommit bound to independently approved exact source SHA")
	}
	return compiled, nil
}

func requireRuntimePristineCatalog(t *testing.T, ctx context.Context, db *sql.DB, mongoDB *mongo.Database) {
	t.Helper()
	var count int64
	query := "SELECT (SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE())+(SELECT COUNT(*) FROM information_schema.routines WHERE routine_schema=DATABASE())+(SELECT COUNT(*) FROM information_schema.triggers WHERE trigger_schema=DATABASE())+(SELECT COUNT(*) FROM information_schema.events WHERE event_schema=DATABASE())"
	if err := db.QueryRowContext(ctx, query).Scan(&count); err != nil || count != 0 {
		t.Fatalf("test MySQL must have a truly empty catalog: count=%d err=%v", count, err)
	}
	cursor, err := mongoDB.ListCollections(ctx, bson.D{}, options.ListCollections().SetNameOnly(false).SetAuthorizedCollections(false))
	if err != nil {
		t.Fatalf("complete test Mongo catalog: %v", err)
	}
	var collections []bson.Raw
	readErr := cursor.All(ctx, &collections)
	closeErr := cursor.Close(ctx)
	if readErr != nil || closeErr != nil || len(collections) != 0 {
		t.Fatalf("test Mongo must have a truly empty catalog: count=%d read_err=%v close_err=%v", len(collections), readErr, closeErr)
	}
	var profile bson.Raw
	if err := mongoDB.RunCommand(ctx, bson.D{{Key: "profile", Value: -1}}).Decode(&profile); err != nil {
		t.Fatalf("read test Mongo pristine profiling state: %v", err)
	}
	zero := false
	if value, ok := profile.Lookup("was").Int32OK(); ok {
		zero = value == 0
	} else if value, ok := profile.Lookup("was").Int64OK(); ok {
		zero = value == 0
	}
	if !zero || profile.Lookup("filter").Type != 0 {
		t.Fatal("test Mongo pristine profiling state is not zero/unfiltered")
	}
}

func runtimeFixtureIdentities(t *testing.T, ctx context.Context, db *sql.DB, sqlName string, client *mongo.Client, mongoName string) (string, string) {
	t.Helper()
	var uuid, selected string
	if err := db.QueryRowContext(ctx, "SELECT @@server_uuid,DATABASE()").Scan(&uuid, &selected); err != nil || selected != sqlName || !regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`).MatchString(uuid) {
		t.Fatal("test MySQL stable selected identity cannot be proved")
	}
	sqlRaw, err := json.Marshal([]any{"mysql_server_selected_v1", uuid, sqlName})
	if err != nil {
		t.Fatal(err)
	}
	var hello struct {
		SetName string   `bson:"setName"`
		Hosts   []string `bson:"hosts"`
	}
	if err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello); err != nil || hello.SetName == "" || len(hello.Hosts) == 0 {
		t.Fatal("test Mongo stable replica-set identity cannot be proved")
	}
	sort.Strings(hello.Hosts)
	for i, host := range hello.Hosts {
		if host == "" || (i > 0 && hello.Hosts[i-1] == host) {
			t.Fatal("test Mongo replica-set hosts are malformed")
		}
	}
	mongoRaw, err := json.Marshal([]any{"mongodb_cluster_selected_v1", bson.D{{Key: "setName", Value: hello.SetName}, {Key: "hosts", Value: hello.Hosts}}, mongoName})
	if err != nil {
		t.Fatal(err)
	}
	return runtimeFixtureHash(sqlRaw), runtimeFixtureHash(mongoRaw)
}

func writeRuntimeFixtureAuthorization(t *testing.T, path string, raw []byte) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := f.Write(raw)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		t.Fatalf("persist private fixture authorization: write=%v sync=%v close=%v", writeErr, syncErr, closeErr)
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	syncErr = dir.Sync()
	closeErr = dir.Close()
	if syncErr != nil || closeErr != nil {
		t.Fatalf("persist private fixture authorization directory: sync=%v close=%v", syncErr, closeErr)
	}
}

func runtimeFixtureHash(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func assertRuntimeRetiredSQLNamespaceAbsent(t *testing.T, db *gorm.DB) {
	t.Helper()
	var count int64
	if err := db.WithContext(t.Context()).Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND BINARY table_name=BINARY ?", "domain_event_outbox").Scan(&count).Error; err != nil || count != 0 {
		t.Fatalf("retired MySQL namespace must remain absent: count=%d err=%v", count, err)
	}
}

func assertRuntimeRetiredMongoNamespaceAbsent(t *testing.T, db *mongo.Database) {
	t.Helper()
	cursor, err := db.ListCollections(t.Context(), bson.D{{Key: "name", Value: "domain_event_outbox"}}, options.ListCollections().SetNameOnly(false).SetAuthorizedCollections(false))
	if err != nil {
		t.Fatalf("read retired Mongo namespace: %v", err)
	}
	var namespaces []bson.Raw
	readErr := cursor.All(t.Context(), &namespaces)
	closeErr := cursor.Close(t.Context())
	if readErr != nil || closeErr != nil || len(namespaces) != 0 {
		t.Fatalf("retired Mongo namespace must remain absent: count=%d read_err=%v close_err=%v", len(namespaces), readErr, closeErr)
	}
}

func TestRuntimeFixtureRejectsUnboundSourceSHA(t *testing.T) {
	sha := "6dc0b8c056cbc768546e4ef9d1722eb9324d3485"
	other := "81f53bf8a377c76347d8a325ed52633232474f4d"
	for _, tc := range []struct {
		name, compiled, github, approved string
		valid                            bool
	}{
		{"CI exact", sha, sha, "", true},
		{"local independently pinned", sha, "", sha, true},
		{"both agree", sha, sha, sha, true},
		{"unknown build", "$Format:%H$", sha, "", false},
		{"no approval", sha, "", "", false},
		{"CI mismatch", sha, other, "", false},
		{"approval mismatch", sha, "", other, false},
		{"conflicting approval", sha, sha, other, false},
		{"malformed approval", sha, "", "unknown", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := runtimeFixtureSourceSHA(tc.compiled, tc.github, tc.approved)
			if (err == nil) != tc.valid || (tc.valid && got != sha) {
				t.Fatalf("source binding accepted=%v want=%v", err == nil, tc.valid)
			}
		})
	}
}
