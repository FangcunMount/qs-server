//go:build integration

package runtimeclosure

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	migrationpkg "github.com/FangcunMount/qs-server/internal/pkg/migration"
	buildversion "github.com/FangcunMount/qs-server/pkg/version"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func runtimeClosureCompiledSource(t *testing.T) string {
	t.Helper()
	source := buildversion.Get().GitCommit
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(source) {
		t.Fatal("runtime closure requires the actual checkout SHA linked into pkg/version.GitCommit")
	}
	return source
}

// This pure test can be executed without any database to independently verify
// that the test build really received its checkout identity through the linker.
func TestRuntimeClosureCompiledSourceIdentity(t *testing.T) {
	source := runtimeClosureCompiledSource(t)
	if expected := os.Getenv("QS_RUNTIME_CLOSURE_EXPECTED_SOURCE_SHA"); expected != "" && source != expected {
		t.Fatalf("compiled source=%s, expected checkout=%s", source, expected)
	}
	t.Log("actual compiled runtime closure source", source)
}

// Only this test host's newly created random SQL and disposable Mongo namespaces
// are approved. The private file approves cold bootstrap; the genuine public
// preflight still observes both empty catalogs, identities and permissions.
// It does not create historical qualification, writer fencing or a DROP permit.
func runtimeClosurePristinePair(t *testing.T, sqlDB *sql.DB, sqlName string, client *mongo.Client, mongoName string) *migrationpkg.PairPreflight {
	t.Helper()
	if !regexp.MustCompile(`^qs_runtime_closure_[0-9]{1,20}$`).MatchString(sqlName) || (!strings.Contains(mongoName, "test") && !strings.Contains(mongoName, "contract")) {
		t.Fatal("pristine approval is limited to the newly allocated runtime test namespaces")
	}
	source := runtimeClosureCompiledSource(t)
	var uuid, selected string
	if err := sqlDB.QueryRowContext(t.Context(), "SELECT @@server_uuid,DATABASE()").Scan(&uuid, &selected); err != nil || selected != sqlName {
		t.Fatal("actual runtime SQL selected identity unavailable")
	}
	var hello struct {
		SetName string   `bson:"setName"`
		Hosts   []string `bson:"hosts"`
	}
	if err := client.Database("admin").RunCommand(t.Context(), bson.D{{Key: "hello", Value: 1}}).Decode(&hello); err != nil || hello.SetName == "" || len(hello.Hosts) == 0 {
		t.Fatal("actual runtime Mongo replica identity unavailable")
	}
	sort.Strings(hello.Hosts)
	for i, host := range hello.Hosts {
		if host == "" || (i > 0 && host == hello.Hosts[i-1]) {
			t.Fatal("actual runtime Mongo hosts malformed")
		}
	}
	hash := func(raw []byte) string {
		h := sha256.Sum256(raw)
		return hex.EncodeToString(h[:])
	}
	sqlIdentity, err := json.Marshal([]any{"mysql_server_selected_v1", uuid, selected})
	if err != nil {
		t.Fatal(err)
	}
	mongoIdentity, err := json.Marshal([]any{"mongodb_cluster_selected_v1", bson.D{{Key: "setName", Value: hello.SetName}, {Key: "hosts", Value: hello.Hosts}}, mongoName})
	if err != nil {
		t.Fatal(err)
	}
	approval := map[string]any{
		"format_version": 1, "kind": "qs_compatibility_retirement_b_pristine_bootstrap",
		"operation_id":        fmt.Sprintf("%d-1", time.Now().UnixNano()),
		"expires_at":          time.Now().UTC().Add(15 * time.Minute).Format(time.RFC3339),
		"approved_source_sha": source, "mysql_database": sqlName, "mongo_database": mongoName,
		"mysql_identity_sha256": hash(sqlIdentity), "mongo_cluster_sha256": hash(mongoIdentity),
		"mysql_version": 100, "mongo_version": 39,
		"migration_resources_sha256": migrationpkg.CompatibilityMigrationResourcesSHA256(),
	}
	summary, err := json.Marshal([]any{"owned-pristine-runtime-test-approval-v1", t.Name(), approval})
	if err != nil {
		t.Fatal(err)
	}
	approval["approval_summary_sha256"] = hash(summary)
	raw, err := json.Marshal(approval)
	if err != nil {
		t.Fatal(err)
	}
	// Resolve the temp root once: the production reader rejects symlink ancestors.
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "pristine-bootstrap.json")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	n, writeErr := f.Write(raw)
	if writeErr == nil && n != len(raw) {
		writeErr = fmt.Errorf("incomplete runtime bootstrap approval write")
	}
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatal("runtime bootstrap approval persistence failed", writeErr, closeErr)
	}
	pair, err := migrationpkg.PreflightCompatibilityPair(t.Context(), sqlDB, client, migrationpkg.PairConfig{
		MySQLDatabase: sqlName, MongoDatabase: mongoName, ExpectedSourceSHA: source,
		BootstrapAuthorizationFile: path, BootstrapAuthorizationSHA256: hash(raw),
	})
	if err != nil {
		t.Fatalf("preflight actual pristine runtime pair: %v", err)
	}
	return pair
}
