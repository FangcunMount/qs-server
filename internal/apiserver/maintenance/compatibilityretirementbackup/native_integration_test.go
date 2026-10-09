//go:build integration

package compatibilityretirementbackup

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	mysql "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

const nativePrivateRoot = "/private/tmp/qs-compatibility-retirement"
const nativeInspectFormat = `{"id":{{json .Id}},"name":{{json .Name}},"image":{{json .Image}},"labels":{{json .Config.Labels}},"mounts":{{json .Mounts}},"ports":{{json .NetworkSettings.Ports}},"requested_ports":{{json .HostConfig.PortBindings}},"network":{{json .HostConfig.NetworkMode}},"running":{{json .State.Running}}}`

type nativeContainer struct {
	ID     string            `json:"id"`
	Name   string            `json:"name"`
	Image  string            `json:"image"`
	Labels map[string]string `json:"labels"`
	Mounts []struct {
		Type, Name, Source, Destination string
		RW                              bool
	} `json:"mounts"`
	Ports          map[string][]struct{ HostIP, HostPort string } `json:"ports"`
	RequestedPorts map[string][]struct{ HostIP, HostPort string } `json:"requested_ports"`
	Network        string                                         `json:"network"`
	Running        bool                                           `json:"running"`
}
type nativeFixture struct {
	ContainerID   string            `json:"container_id"`
	ContainerName string            `json:"container_name"`
	Image         string            `json:"image_id"`
	Volumes       []string          `json:"volumes"`
	Labels        map[string]string `json:"labels"`
	Ready         bool              `json:"ready"`
}

func nativeCommand(ctx context.Context, env []string, program string, args ...string) ([]byte, error) {
	q, c := context.WithTimeout(ctx, 180*time.Second)
	defer c()
	cmd := exec.CommandContext(q, program, args...)
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if cmd.Run() != nil {
		// Full command output stays private (never env/argv). Only a fixed error
		// category is extracted by the caller; this checkpoint supports diagnosis.
		path := filepath.Join(nativePrivateRoot, "backup-native-command-"+primitive.NewObjectID().Hex()+".log")
		raw := append(append([]byte(nil), stdout.Bytes()...), stderr.Bytes()...)
		if len(raw) <= 2<<20 {
			_ = writePrivate(path, raw)
		}
		return stdout.Bytes(), Error("owned_native_command_failed")
	}
	if stdout.Len() > 2<<20 || stderr.Len() > 2<<20 {
		return nil, Error("owned_native_output_budget_exceeded")
	}
	return stdout.Bytes(), nil
}
func nativeDocker(ctx context.Context, args ...string) ([]byte, error) {
	return nativeCommand(ctx, os.Environ(), "docker", args...)
}
func nativeInspect(ctx context.Context, id string) (nativeContainer, error) {
	var v nativeContainer
	b, e := nativeDocker(ctx, "inspect", "--format", nativeInspectFormat, id)
	if e != nil || json.Unmarshal(b, &v) != nil {
		return v, ErrIsolation
	}
	return v, nil
}
func nativeRoot(t *testing.T, filename, port string) nativeContainer {
	t.Helper()
	raw, e := os.ReadFile(filepath.Join(nativePrivateRoot, filename))
	var f nativeFixture
	if e != nil || json.Unmarshal(raw, &f) != nil || !f.Ready || len(f.ContainerID) != 64 || !strings.HasPrefix(f.Image, "sha256:") {
		t.Fatal("owned_fixture_manifest_rejected")
	}
	actual, e := nativeInspect(context.Background(), f.ContainerID)
	if e != nil || actual.ID != f.ContainerID || strings.TrimPrefix(actual.Name, "/") != f.ContainerName || actual.Image != f.Image || !actual.Running || actual.Labels["codex.task"] == "" {
		t.Fatal("owned_fixture_identity_rejected")
	}
	seen := map[string]bool{}
	for _, m := range actual.Mounts {
		if m.Type != "volume" {
			t.Fatal("owned_fixture_mount_rejected")
		}
		seen[m.Name] = true
	}
	if len(seen) != len(f.Volumes) {
		t.Fatal("owned_fixture_volume_rejected")
	}
	for _, v := range f.Volumes {
		if !seen[v] {
			t.Fatal("owned_fixture_volume_rejected")
		}
	}
	if f.Labels != nil && !reflect.DeepEqual(actual.Labels, f.Labels) {
		t.Fatal("owned_fixture_label_rejected")
	}
	found := false
	for _, p := range actual.Ports {
		if len(p) == 1 && p[0].HostIP == "127.0.0.1" && p[0].HostPort == port {
			found = true
		}
	}
	if !found || len(actual.Ports) != 1 {
		t.Fatal("owned_fixture_loopback_rejected")
	}
	t.Cleanup(func() {
		after, e := nativeInspect(context.Background(), actual.ID)
		if e != nil || !reflect.DeepEqual(after, actual) {
			t.Error("shared_fixture_changed")
		}
	})
	return actual
}
func nativePrivateDir(t *testing.T) string {
	t.Helper()
	return nativePrivateDirWithCleanupGuard(t, nil)
}

func nativePrivateDirWithCleanupGuard(t *testing.T, cleanupAllowed func() bool) string {
	t.Helper()
	p, e := os.MkdirTemp(nativePrivateRoot, "backup-native-owned-")
	if e != nil || os.Chmod(p, 0700) != nil {
		t.Fatal("owned_private_directory_failed")
	}
	t.Cleanup(func() {
		// This in-memory guard remains effective when disk-full or permission
		// failures prevent a durable unresolved checkpoint from being written.
		if cleanupAllowed != nil && !cleanupAllowed() {
			t.Error("owned_private_material_cleanup_not_accepted_retained")
			return
		}
		if _, e := os.Lstat(filepath.Join(p, "cleanup-unresolved.private.json")); e == nil {
			t.Error("owned_cleanup_unresolved_checkpoint_retained")
			return
		} else if !os.IsNotExist(e) {
			t.Error("owned_cleanup_checkpoint_state_unknown")
			return
		}
		if os.RemoveAll(p) != nil {
			t.Error("owned_private_material_cleanup_failed")
		}
		if _, e := os.Stat(p); !os.IsNotExist(e) {
			t.Error("owned_private_material_remaining")
		}
	})
	return p
}
func nativeJSON(t *testing.T, path string, v any) string {
	t.Helper()
	raw, e := json.Marshal(v)
	if e != nil || writePrivate(path, raw) != nil {
		t.Fatal("owned_private_registration_failed")
	}
	return sha(raw)
}
func nativeEnv(t *testing.T) map[string]string {
	t.Helper()
	raw, e := os.ReadFile(filepath.Join(nativePrivateRoot, "mysql-native.env.json"))
	var env map[string]string
	if e != nil || json.Unmarshal(raw, &env) != nil || env["MYSQL_USERNAME"] == "" || env["MYSQL_HOST"] != "127.0.0.1" || env["MYSQL_PORT"] != "34306" || env["MONGODB_HOST"] != "127.0.0.1" || env["MONGODB_PORT"] != "33317" {
		t.Fatal("owned_private_environment_rejected")
	}
	return env
}
func nativeChildEnv(values map[string]string) []string {
	out := []string{}
	for _, item := range os.Environ() {
		key := strings.SplitN(item, "=", 2)[0]
		if _, ok := values[key]; !ok {
			out = append(out, item)
		}
	}
	for k, v := range values {
		out = append(out, k+"="+v)
	}
	return out
}
func nativeSQL(t *testing.T, env map[string]string, name string) *sql.DB {
	t.Helper()
	cfg := mysql.NewConfig()
	cfg.User = env["MYSQL_USERNAME"]
	cfg.Passwd = env["MYSQL_PASSWORD"]
	cfg.Net = "tcp"
	cfg.Addr = env["MYSQL_HOST"] + ":" + env["MYSQL_PORT"]
	cfg.DBName = name
	cfg.ParseTime = false
	cfg.Params = map[string]string{"charset": "utf8mb4"}
	cfg.Timeout = 5 * time.Second
	db, e := sql.Open("mysql", cfg.FormatDSN())
	if e != nil || db.PingContext(context.Background()) != nil {
		t.Fatal("owned_source_connection_failed")
	}
	t.Cleanup(func() {
		if db.Close() != nil {
			t.Error("owned_source_connection_close_failed")
		}
	})
	return db
}
func nativeExec(t *testing.T, db *sql.DB, q string, args ...any) {
	t.Helper()
	if _, e := db.ExecContext(context.Background(), q, args...); e != nil {
		if se, ok := e.(*mysql.MySQLError); ok {
			t.Fatalf("owned_source_fixture_write_failed server_code=%d", se.Number)
		}
		t.Fatal("owned_source_fixture_write_failed")
	}
}
func nativeMongo(t *testing.T, env map[string]string) *mongo.Client {
	t.Helper()
	opts := options.Client().SetHosts([]string{env["MONGODB_HOST"] + ":" + env["MONGODB_PORT"]}).SetDirect(true).SetServerSelectionTimeout(5 * time.Second)
	if env["MONGODB_USERNAME"] != "" {
		opts.SetAuth(options.Credential{Username: env["MONGODB_USERNAME"], Password: env["MONGODB_PASSWORD"], AuthSource: "admin"})
	}
	client, e := mongo.Connect(context.Background(), opts)
	if e != nil || client.Ping(context.Background(), nil) != nil {
		t.Fatal("owned_source_mongo_connection_failed")
	}
	t.Cleanup(func() {
		if client.Disconnect(context.Background()) != nil {
			t.Error("owned_source_mongo_disconnect_failed")
		}
	})
	return client
}
func nativeSourceSchemas(t *testing.T, db *sql.DB, mdb *mongo.Database) {
	t.Helper()
	nativeExec(t, db, "CREATE TABLE schema_migrations(version BIGINT NOT NULL,dirty BOOL NOT NULL)")
	nativeExec(t, db, "INSERT INTO schema_migrations VALUES(99,FALSE)")
	nativeExec(t, db, "CREATE TABLE rm_outbox(id BIGINT PRIMARY KEY,protected_fact BLOB)")
	nativeExec(t, db, "INSERT INTO rm_outbox VALUES(1,X'00FF')")
	nativeExec(t, db, "CREATE TABLE evidence_kept(id BIGINT PRIMARY KEY,proof JSON)")
	nativeExec(t, db, `INSERT INTO evidence_kept VALUES(1,'{"keep":true}')`)
	root := filepath.Clean("../../../..")
	for _, file := range []string{"000018_add_domain_event_outbox.up.sql", "000072_ai_bridge_delivery.up.sql", "000093_ai_messaging_legacy_commands.up.sql"} {
		raw, e := os.ReadFile(filepath.Join(root, "internal/pkg/migration/migrations/mysql", file))
		if e != nil {
			t.Fatal("actual_source_migration_unavailable")
		}
		var clean []string
		for _, line := range strings.Split(string(raw), "\n") {
			if !strings.HasPrefix(strings.TrimSpace(line), "--") {
				clean = append(clean, line)
			}
		}
		for _, q := range strings.Split(strings.Join(clean, "\n"), ";") {
			if strings.TrimSpace(q) != "" {
				nativeExec(t, db, q)
			}
		}
	}
	raw, e := os.ReadFile(filepath.Join(root, "internal/pkg/migration/migrations/mysql/000049_add_retry_governance.up.sql"))
	if e != nil {
		t.Fatal("actual_source_migration_unavailable")
	}
	start := strings.Index(string(raw), "ALTER TABLE `domain_event_outbox`")
	if start < 0 {
		t.Fatal("actual_source_layout_unavailable")
	}
	end := strings.Index(string(raw)[start:], ";")
	if end < 0 {
		t.Fatal("actual_source_layout_unavailable")
	}
	nativeExec(t, db, string(raw)[start:start+end])
	if _, e = mdb.Collection("schema_migrations").InsertOne(context.Background(), bson.D{{Key: "version", Value: int64(38)}, {Key: "dirty", Value: false}}); e != nil {
		t.Fatal("owned_mongo_head_setup_failed")
	}
	if _, e = mdb.Collection("rm_outbox").InsertOne(context.Background(), bson.D{{Key: "_id", Value: "protected"}, {Key: "proof", Value: true}}); e != nil {
		t.Fatal("owned_mongo_kept_setup_failed")
	}
	if e = mdb.CreateCollection(context.Background(), "domain_event_outbox", options.CreateCollection().SetValidator(bson.D{{Key: "event_id", Value: bson.D{{Key: "$type", Value: "string"}}}}).SetValidationLevel("strict")); e != nil {
		t.Fatal("owned_mongo_collection_setup_failed")
	}
	models := []mongo.IndexModel{{Keys: bson.D{{Key: "event_id", Value: 1}}, Options: options.Index().SetName("uk_event_id").SetUnique(true)}, {Keys: bson.D{{Key: "status", Value: 1}, {Key: "created_at", Value: -1}}, Options: options.Index().SetName("ordered_compound").SetPartialFilterExpression(bson.D{{Key: "status", Value: "published"}})}}
	if _, e = mdb.Collection("domain_event_outbox").Indexes().CreateMany(context.Background(), models); e != nil {
		t.Fatal("owned_mongo_indexes_setup_failed")
	}
}
func nativeSourceData(t *testing.T, db *sql.DB, mdb *mongo.Database) {
	t.Helper()
	clock := "2026-10-08 11:12:13.123"
	for start := 0; start < 2503; start += 250 {
		var tuples []string
		var args []any
		for i := start; i < start+250 && i < 2503; i++ {
			tuples = append(tuples, "(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)")
			var nullable any
			if i%2 == 0 {
				nullable = ""
			}
			args = append(args, uint64(i+1), "old-event-"+primitive.NewObjectID().Hex(), "evaluation.outcome.committed", "Evaluation", "639678084915671598", nil, "qs-evaluation", `{"private":"PRIVATE_BACKUP_SENTINEL","n":9223372036854775807}`, "published", i%7, nil, clock, nullable, nil, nil, clock, clock, nil)
		}
		nativeExec(t, db, "INSERT INTO domain_event_outbox VALUES "+strings.Join(tuples, ","), args...)
	}
	for i := 0; i < 8; i++ {
		id := uuid.NewString()
		nativeExec(t, db, "INSERT INTO ai_bridge_requests(request_id,request_hash,payload) VALUES(?,?,?)", id, strings.Repeat("a", 64), `{"owner":"private fixture"}`)
		nativeExec(t, db, "INSERT INTO ai_bridge_commands VALUES(?,?,?,?,?,?,?,?)", id, id, "start", `{"private":"PRIVATE_BACKUP_SENTINEL","number":639678084915671598}`, strings.Repeat("b", 64), i%2 == 0, 3, "2026-10-08 11:12:13.123456")
		if i < 2 {
			payload := []byte{0, 255, 16}
			if i == 1 {
				payload = []byte{}
			}
			nativeExec(t, db, "INSERT INTO ai_messaging_legacy_commands VALUES(?,?,?,?,?,?,?,?,?,?)", id, id, "start", payload, sha(payload), 3, "2026-10-08 11:12:13.123456", "2026-10-08T11:12:13.123456Z", strings.Repeat("c", 64), "2026-10-08 11:12:14.123456")
		}
	}
	for start := 0; start < 2003; start += 250 {
		var docs []any
		for i := start; i < start+250 && i < 2003; i++ {
			docs = append(docs, bson.D{{Key: "_id", Value: primitive.NewObjectID()}, {Key: "event_id", Value: uuid.NewString()}, {Key: "event_type", Value: "answersheet.submitted"}, {Key: "status", Value: "published"}, {Key: "created_at", Value: time.UnixMilli(1728000123123)}, {Key: "private", Value: "PRIVATE_BACKUP_SENTINEL"}, {Key: "large", Value: int64(639678084915671598)}, {Key: "binary", Value: primitive.Binary{Subtype: 0, Data: []byte{0, 255}}}, {Key: "nullable", Value: nil}, {Key: "array", Value: bson.A{int32(1), int64(2), nil}}})
		}
		if _, e := mdb.Collection("domain_event_outbox").InsertMany(context.Background(), docs); e != nil {
			t.Fatal("owned_mongo_data_setup_failed")
		}
	}
}
func nativeIdentities(t *testing.T, db *sql.DB, mdb *mongo.Database) (map[string]string, map[string]uint64) {
	t.Helper()
	rows, e := readSQL(context.Background(), db, "SELECT @@server_uuid,DATABASE(),VERSION()")
	if e != nil || len(rows) != 1 {
		t.Fatal("owned_source_identity_failed")
	}
	sqlID := parts("mysql_database_identity_v1", cell(rows[0], 0), cell(rows[0], 1))
	var hello bson.Raw
	if mdb.Client().Database("admin").RunCommand(context.Background(), bson.D{{Key: "hello", Value: 1}}).Decode(&hello) != nil {
		t.Fatal("owned_source_identity_failed")
	}
	cols, _, e := mongoCatalog(context.Background(), mdb)
	if e != nil {
		t.Fatal("owned_source_catalog_failed")
	}
	kind, id, ok := cols["schema_migrations"].Lookup("info", "uuid").BinaryOK()
	if !ok || kind != 4 {
		t.Fatal("owned_source_identity_failed")
	}
	stable := bson.D{}
	for _, n := range []string{"setName", "hosts", "me"} {
		v := hello.Lookup(n)
		if v.Type != 0 {
			var a any
			if v.Unmarshal(&a) != nil {
				t.Fatal("owned_source_identity_failed")
			}
			stable = append(stable, bson.E{Key: n, Value: a})
		}
	}
	raw, e := json.Marshal(stable)
	if e != nil {
		t.Fatal("owned_source_identity_failed")
	}
	mongoID := parts("mongodb_database_identity_v1", string(raw), mdb.Name(), hex.EncodeToString(id))
	return map[string]string{"mysql": sqlID, "mongodb": mongoID}, map[string]uint64{"mysql": 99, "mongodb": 38}
}
func nativeBuild(t *testing.T, dir string) (string, string) {
	t.Helper()
	ctx := context.Background()
	cli := filepath.Join(dir, "inventory-cli")
	helper := filepath.Join(dir, "restore-native.test")
	goEnv := nativeChildEnv(map[string]string{"GOTOOLCHAIN": "local"})
	if _, e := nativeCommand(ctx, goEnv, "go", "build", "-ldflags=-X main.sourceSHA="+strings.Repeat("a", 40), "-o", cli, "../../../../cmd/qs-compatibility-retirement"); e != nil {
		t.Fatal("owned_inventory_tool_compile_failed")
	}
	linux := nativeChildEnv(map[string]string{"GOOS": "linux", "GOARCH": "arm64", "CGO_ENABLED": "0", "GOTOOLCHAIN": "local"})
	if _, e := nativeCommand(ctx, linux, "go", "test", "-c", "-tags=integration", "-o", helper, "."); e != nil {
		t.Fatal("owned_restore_tool_compile_failed")
	}
	if os.Chmod(helper, 0500) != nil {
		t.Fatal("owned_restore_tool_mode_failed")
	}
	return cli, helper
}
func nativeInventory(t *testing.T, cli, dir string, env map[string]string, db *sql.DB, mdb *mongo.Database) (Inputs, Approval) {
	t.Helper()
	ids, heads := nativeIdentities(t, db, mdb)
	scope := [][3]string{{"mysql", "domain_event_outbox", "base_table"}, {"mysql", "ai_bridge_commands", "base_table"}, {"mysql", "ai_messaging_legacy_commands", "base_table"}, {"mongodb", "domain_event_outbox", "collection"}}
	limits := map[string]int{"query_seconds": 30, "total_seconds": 1500, "max_records": 1000000, "max_bytes": 2 << 30, "page_size": 1000, "max_pages": 1001}
	request := map[string]any{"format_version": 2, "kind": "readonly_inventory_boundary_request", "source_sha": strings.Repeat("a", 40), "operation_id": "123-1", "target_hash": jsonSHA(scope), "database_scope": "mysql-and-mongodb", "identity_hashes": ids, "expected_migrations": heads, "limits": limits}
	path := filepath.Join(dir, "boundary-request.json")
	hash := nativeJSON(t, path, request)
	out := filepath.Join(dir, "bounds-456-1")
	if os.Mkdir(out, 0700) != nil {
		t.Fatal("owned_inventory_directory_failed")
	}
	child := nativeChildEnv(env)
	if raw, e := nativeCommand(context.Background(), child, cli, "--mode=bounds", "--request", path, "--request-hash", hash, "--operation-id=123-1", "--run-id=456-1", "--output-directory", out); e != nil {
		var category struct {
			Error string `json:"error_category"`
		}
		if json.Unmarshal(raw, &category) == nil && safeNativeCategory(category.Error) {
			t.Fatal("actual_inventory_bounds_failed category=" + category.Error)
		}
		t.Fatal("actual_inventory_bounds_failed")
	}
	raw, e := os.ReadFile(filepath.Join(out, "boundary.private.json"))
	var bound inventory
	if e != nil || exactJSON(raw, &bound) != nil || !bound.Complete || len(bound.Targets) != 4 {
		t.Fatal("actual_inventory_bounds_incomplete")
	}
	boundaries := make([]any, 4)
	for i, s := range bound.Targets {
		boundaries[i] = s.Boundary
	}
	request["kind"] = "readonly_inventory_request"
	request["boundary_run_id"] = "456-1"
	request["boundary_report_hash"] = sha(raw)
	request["approved_boundaries"] = boundaries
	path = filepath.Join(dir, "inventory-request.json")
	hash = nativeJSON(t, path, request)
	out = filepath.Join(dir, "inventory-789-1")
	if os.Mkdir(out, 0700) != nil {
		t.Fatal("owned_inventory_directory_failed")
	}
	if _, e := nativeCommand(context.Background(), child, cli, "--mode=inventory", "--request", path, "--request-hash", hash, "--operation-id=123-1", "--run-id=789-1", "--output-directory", out); e != nil {
		t.Fatal("actual_inventory_scan_failed")
	}
	reportRaw, e := os.ReadFile(filepath.Join(out, "inventory.private.json"))
	if e != nil {
		t.Fatal("actual_inventory_report_missing")
	}
	sqlRaw, e := os.ReadFile(filepath.Join(out, "mysql-metadata.private.json"))
	if e != nil {
		t.Fatal("actual_inventory_structure_missing")
	}
	mongoRaw, e := os.ReadFile(filepath.Join(out, "mongodb-metadata.private.json"))
	if e != nil {
		t.Fatal("actual_inventory_structure_missing")
	}
	ordered, e := ReadOrderedMongoSchema(context.Background(), mdb)
	if e != nil {
		t.Fatal("actual_ordered_structure_missing")
	}
	approval := Approval{InventorySHA256: sha(reportRaw), SQLMetadataSHA256: sha(sqlRaw), MongoMetadataSHA256: sha(mongoRaw), OrderedMongoSchemaSHA256: ordered.SHA256(), SourceSHA: strings.Repeat("a", 40), OperationID: "123-1", RunID: "789-1", RequestHash: hash}
	inputs := Inputs{Inventory: bytes.NewReader(reportRaw), SQLMetadata: bytes.NewReader(sqlRaw), MongoMetadata: bytes.NewReader(mongoRaw)}
	for i, name := range sourceNames {
		f, e := openPrivateFile(filepath.Join(out, name))
		if e != nil {
			t.Fatal("actual_inventory_source_missing")
		}
		t.Cleanup(func() {
			if f.Close() != nil {
				t.Error("borrowed_fixture_source_close_failed")
			}
		})
		inputs.Sources[i] = f
	}
	return inputs, approval
}
func TestBackupNativeRestoreHelper(t *testing.T) {
	mode := os.Getenv("QS_BACKUP_NATIVE_HELPER")
	if mode == "" {
		t.Skip("owned container helper only")
	}
	ctx, c := context.WithTimeout(context.Background(), 600*time.Second)
	defer c()
	archive, e := OpenArchive(ctx, "/backup", os.Getenv("QS_BACKUP_ARCHIVE_HASH"))
	if e != nil {
		t.Fatal("owned_archive_open_failed")
	}
	var v Verification
	if mode == "mysql" {
		cfg := mysql.NewConfig()
		cfg.User = "root"
		cfg.Net = "unix"
		cfg.Addr = "/var/run/mysqld/mysqld.sock"
		cfg.Params = map[string]string{"charset": "utf8mb4"}
		db, e := sql.Open("mysql", cfg.FormatDSN())
		if e != nil {
			t.Fatal("owned_restore_connect_failed")
		}
		defer func() {
			if db.Close() != nil {
				t.Error("owned_restore_pool_close_failed")
			}
		}()
		deadline := time.Now().Add(90 * time.Second)
		for {
			ready, e := readSQL(ctx, db, "SELECT @@skip_networking")
			if e == nil && len(ready) == 1 && cell(ready[0], 0) == "0" {
				break
			}
			// The official image's temporary initialization server explicitly uses
			// --skip-networking. Wait for the final server's actual value, without
			// assuming PID 1 is mysqld (an image/runtime init may wrap that process).
			if time.Now().After(deadline) {
				t.Fatal("owned_restore_not_ready")
			}
			time.Sleep(200 * time.Millisecond)
		}
		name := "qs_restore_" + primitive.NewObjectID().Hex()
		if _, e = db.ExecContext(ctx, "CREATE DATABASE "+quote(name)+" CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); e != nil {
			t.Fatal("owned_restore_database_create_failed")
		}
		conn, e := db.Conn(ctx)
		if e != nil {
			t.Fatal("owned_restore_connection_failed")
		}
		defer func() {
			if conn.Close() != nil {
				t.Error("owned_restore_connection_close_failed")
			}
		}()
		if _, e = conn.ExecContext(ctx, "USE "+quote(name)); e != nil {
			t.Fatal("owned_restore_namespace_failed")
		}
		proof, err := RestoreSQL(ctx, conn, archive)
		e = err
		if proof != nil {
			v = proof.Summary()
		}
		if e != nil {
			if e == ErrStructure {
				for i := range archive.data.SQL {
					actual, re := sqlStructure(ctx, conn, i)
					if re != nil {
						t.Logf("safe_sql_restore_structure target_index=%d read_failed=true", i)
						continue
					}
					want := archive.data.SQL[i]
					al, wl := strings.Split(actual.DDL, "\n"), strings.Split(want.DDL, "\n")
					if len(al) == len(wl) {
						for li := range al {
							if al[li] == wl[li] {
								continue
							}
							category := "other"
							for _, k := range []string{"PRIMARY KEY", "UNIQUE KEY", " KEY ", "CONSTRAINT", "ENGINE=", "CHECK", "COMMENT", "AUTO_INCREMENT"} {
								if strings.Contains(al[li], k) || strings.Contains(wl[li], k) {
									category = strings.TrimSpace(k)
									break
								}
							}
							t.Logf("safe_sql_restore_ddl_difference target_index=%d line_index=%d category=%s actual_length=%d expected_length=%d", i, li, category, len(al[li]), len(wl[li]))
						}
					}
					t.Logf("safe_sql_restore_structure target_index=%d ddl_equal=%t columns_equal=%t auto_increment_normalized_equal=%t charsets_equal=%t environment_equal=%t semantic_ddl_equal=%t", i, actual.DDL == want.DDL, reflect.DeepEqual(actual.Columns, want.Columns), increment.ReplaceAllString(actual.DDL, "") == increment.ReplaceAllString(want.DDL, ""), reflect.DeepEqual(actual.CharacterSets, want.CharacterSets), reflect.DeepEqual(actual.ShowCreateEnvironment, want.ShowCreateEnvironment), normalizedSQLDDL(actual) == normalizedSQLDDL(want))
					for ci := range actual.Columns {
						if ci < len(want.Columns) && !reflect.DeepEqual(actual.Columns[ci], want.Columns[ci]) {
							for fi := range actual.Columns[ci] {
								if fi < len(want.Columns[ci]) && !reflect.DeepEqual(actual.Columns[ci][fi], want.Columns[ci][fi]) {
									t.Logf("safe_sql_restore_column_difference target_index=%d column_index=%d field_index=%d", i, ci, fi)
								}
							}
						}
					}
				}
			}
			t.Fatal(e)
		}
		settings, e := readSQL(ctx, conn, "SELECT @@session.foreign_key_checks")
		if e != nil || cell(settings[0], 0) != "1" {
			t.Fatal("actual_restore_fk_session_not_restored")
		}
		nativeSQLStructureNegatives(t, ctx, conn, archive)
		negativeTx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal("owned_restore_negative_transaction_failed")
		}
		if _, e = negativeTx.ExecContext(ctx, "UPDATE domain_event_outbox SET payload_json='PRIVATE_RESTORE_CORRUPTION' WHERE id=1"); e != nil {
			t.Fatal("owned_restore_negative_setup_failed")
		}
		if e = verifySQLContent(ctx, negativeTx, archive.data.SQL[0], archive.data.Inventory.Targets[0], 0); e != ErrContent {
			t.Fatal("actual_restore_corruption_not_detected")
		}
		if negativeTx.Rollback() != nil || verifySQLContent(ctx, conn, archive.data.SQL[0], archive.data.Inventory.Targets[0], 0) != nil {
			t.Fatal("owned_restore_negative_rollback_not_equal")
		}

	} else if mode == "mongodb" {
		client, e := mongo.Connect(ctx, options.Client().SetHosts([]string{"127.0.0.1:27017"}).SetDirect(true).SetServerSelectionTimeout(3*time.Second))
		if e != nil {
			t.Fatal("owned_restore_connect_failed")
		}
		defer func() {
			if client.Disconnect(context.Background()) != nil {
				t.Error("owned_restore_client_close_failed")
			}
		}()
		deadline := time.Now().Add(90 * time.Second)
		for client.Ping(ctx, nil) != nil {
			if time.Now().After(deadline) {
				t.Fatal("owned_restore_not_ready")
			}
			time.Sleep(200 * time.Millisecond)
		}
		db := client.Database("qs_restore_" + primitive.NewObjectID().Hex())
		proof, err := RestoreMongo(ctx, db, archive)
		e = err
		if proof != nil {
			v = proof.Summary()
		}
		if e != nil {
			t.Fatal(e)
		}
		schema, e := ReadOrderedMongoSchema(ctx, db)
		if e != nil || !schemaEqual(schema.data, archive.data.Mongo) {
			t.Fatal("actual_compound_key_order_not_preserved")
		}
		var original bson.Raw
		if db.Collection("domain_event_outbox").FindOne(ctx, bson.D{}).Decode(&original) != nil {
			t.Fatal("owned_restore_negative_original_read_failed")
		}
		if _, e = db.Collection("domain_event_outbox").DeleteOne(ctx, bson.D{{Key: "_id", Value: original.Lookup("_id")}}); e != nil {
			t.Fatal("owned_restore_negative_setup_failed")
		}
		if e = verifyMongoContent(ctx, db, archive.data.Inventory.Targets[3]); e != ErrContent {
			t.Fatal("actual_restore_partial_copy_not_detected")
		}
		if _, e = db.Collection("domain_event_outbox").InsertOne(ctx, original); e != nil || verifyMongoContent(ctx, db, archive.data.Inventory.Targets[3]) != nil {
			t.Fatal("owned_restore_negative_repair_not_equal")
		}

	} else {
		t.Fatal("owned_restore_helper_mode_invalid")
	}
	v.FinishedAt = time.Now().UTC()
	v.ElapsedMillis = v.FinishedAt.Sub(v.StartedAt).Milliseconds()
	raw, e := json.Marshal(v)
	if e != nil || !v.SchemaEqual || !v.ContentEqual || v.DropReady {
		t.Fatal("owned_restore_verification_missing")
	}
	t.Log("safe_restore_verification=" + string(raw))
}

type nativeOwnedRestore struct {
	ID, Name, Image, Owner, Kind, ArchiveDir, Tool, Registry string
	Volumes                                                  []string
	Labels, ContainerLabels                                  map[string]string
}

func (f *nativeOwnedRestore) check(ctx context.Context, requireRunning bool) error {
	id := f.ID
	if id == "" {
		id = f.Name
	}
	v, e := nativeInspect(ctx, id)
	if e != nil {
		return e
	}
	if (f.ID != "" && v.ID != f.ID) || len(v.ID) != 64 || strings.TrimPrefix(v.Name, "/") != f.Name || v.Image != f.Image || !reflect.DeepEqual(v.Labels, f.ContainerLabels) || v.Network != "none" || (requireRunning && !v.Running) || len(v.RequestedPorts) != 0 {
		return ErrIsolation
	}
	for _, ports := range v.Ports {
		if len(ports) > 0 {
			return ErrIsolation
		}
	}
	expected := map[string]string{}
	if f.Kind == "mysql" {
		expected[f.Volumes[0]] = "/var/lib/mysql"
	} else {
		expected[f.Volumes[0]] = "/data/db"
		expected[f.Volumes[1]] = "/data/configdb"
	}
	seen := map[string]bool{}
	binds := 0
	for _, m := range v.Mounts {
		if m.Type == "volume" && m.RW && expected[m.Name] == m.Destination && !seen[m.Name] {
			seen[m.Name] = true
		} else if m.Type == "bind" && !m.RW && ((m.Source == f.ArchiveDir && m.Destination == "/backup") || (m.Source == f.Tool && m.Destination == "/tool/native.test")) {
			binds++
		} else {
			return ErrIsolation
		}
	}
	if len(seen) != len(expected) || binds != 2 || len(v.Mounts) != len(expected)+2 {
		return ErrIsolation
	}
	f.ID = v.ID
	return nil
}
func (f *nativeOwnedRestore) cleanup(ctx context.Context) error {
	if e := f.check(ctx, false); e == nil {
		if _, e = nativeDocker(ctx, "rm", "--force", f.ID); e != nil {
			return Error("owned_restore_container_remove_failed")
		}
	} else {
		remaining, e := nativeDocker(ctx, "ps", "--all", "--filter", "name=^/"+f.Name+"$", "--format", "{{.ID}}")
		if e != nil || strings.TrimSpace(string(remaining)) != "" {
			return Error("owned_restore_cleanup_identity_unproven")
		}
	}
	for _, volume := range f.Volumes {
		raw, e := nativeDocker(ctx, "volume", "inspect", "--format", "{{json .Labels}}", volume)
		if e != nil {
			remaining, e := nativeDocker(ctx, "volume", "ls", "--filter", "name=^"+volume+"$", "--format", "{{.Name}}")
			if e != nil || strings.TrimSpace(string(remaining)) != "" {
				return Error("owned_restore_volume_absence_unproven")
			}
			continue
		}
		var labels map[string]string
		if json.Unmarshal(raw, &labels) != nil || !reflect.DeepEqual(labels, f.Labels) {
			return Error("owned_restore_volume_ownership_rejected")
		}
		if _, e = nativeDocker(ctx, "volume", "rm", volume); e != nil {
			return Error("owned_restore_volume_remove_failed")
		}
	}
	containers, e := nativeDocker(ctx, "ps", "--all", "--filter", "label=codex.owner="+f.Owner, "--format", "{{.ID}}")
	volumes, ve := nativeDocker(ctx, "volume", "ls", "--filter", "label=codex.owner="+f.Owner, "--format", "{{.Name}}")
	if e != nil || ve != nil || strings.TrimSpace(string(containers)) != "" || strings.TrimSpace(string(volumes)) != "" {
		return Error("owned_restore_resources_remaining")
	}
	return nil
}
func nativeStartRestore(t *testing.T, dir, tool, archive, kind string, root nativeContainer) *nativeOwnedRestore {
	t.Helper()
	owner := primitive.NewObjectID().Hex()
	f := &nativeOwnedRestore{Name: "qs-backup-native-" + owner, Owner: owner, Image: root.Image, Kind: kind, ArchiveDir: archive, Tool: tool, Labels: map[string]string{"codex.task": "qs-retirement-backup-native", "codex.owner": owner}}
	f.Volumes = []string{"qs-backup-data-" + owner}
	if kind == "mongodb" {
		f.Volumes = append(f.Volumes, "qs-backup-config-"+owner)
	}
	raw, e := nativeDocker(context.Background(), "image", "inspect", "--format", "{{json .Config.Labels}}", root.Image)
	if e != nil || json.Unmarshal(raw, &f.ContainerLabels) != nil {
		t.Fatal("owned_restore_image_identity_failed")
	}
	if f.ContainerLabels == nil {
		f.ContainerLabels = map[string]string{}
	}
	for k, v := range f.Labels {
		f.ContainerLabels[k] = v
	}
	arch, e := nativeDocker(context.Background(), "image", "inspect", "--format", "{{.Architecture}}", root.Image)
	if e != nil || strings.TrimSpace(string(arch)) != "arm64" {
		t.Fatal("owned_restore_image_architecture_rejected")
	}
	f.Registry = filepath.Join(dir, "restore-"+owner+".private.json")
	nativeJSON(t, f.Registry, f)
	t.Cleanup(func() {
		if e := f.cleanup(context.Background()); e != nil {
			nativeRetainCleanup(t, dir)
			t.Error(e)
		} else {
			t.Log("owned_restore_container_remaining=0 owned_restore_volumes_remaining=0")
		}
	})
	for _, volume := range f.Volumes {
		if _, e = nativeDocker(context.Background(), "volume", "create", "--label", "codex.task="+f.Labels["codex.task"], "--label", "codex.owner="+owner, volume); e != nil {
			t.Fatal("owned_restore_volume_create_failed")
		}
	}
	args := []string{"run", "--detach", "--name", f.Name, "--label", "codex.task=" + f.Labels["codex.task"], "--label", "codex.owner=" + owner, "--network", "none", "--memory", "2g", "--cpus", "2", "--mount", "type=bind,src=" + archive + ",dst=/backup,readonly", "--mount", "type=bind,src=" + tool + ",dst=/tool/native.test,readonly"}
	if kind == "mysql" {
		args = append(args, "--mount", "type=volume,src="+f.Volumes[0]+",dst=/var/lib/mysql", "--env", "MYSQL_ALLOW_EMPTY_PASSWORD=1", root.Image)
	} else {
		args = append(args, "--mount", "type=volume,src="+f.Volumes[0]+",dst=/data/db", "--mount", "type=volume,src="+f.Volumes[1]+",dst=/data/configdb", root.Image, "mongod", "--bind_ip", "127.0.0.1", "--setParameter", "ttlMonitorEnabled=false")
	}
	if _, e = nativeDocker(context.Background(), args...); e != nil {
		t.Fatal("owned_restore_container_create_failed")
	}
	if e = f.check(context.Background(), true); e != nil {
		t.Fatal(e)
	}
	nativeJSON(t, filepath.Join(dir, "restore-"+owner+"-created.private.json"), f)
	return f
}
func nativeRunRestore(t *testing.T, f *nativeOwnedRestore, a *Archive) Verification {
	t.Helper()
	if e := f.check(context.Background(), true); e != nil {
		t.Fatal(e)
	}
	before, e := nativeInspect(context.Background(), f.ID)
	if e != nil {
		t.Fatal("owned_restore_inspect_failed")
	}
	raw, e := nativeDocker(context.Background(), "exec", "--env", "QS_BACKUP_NATIVE_HELPER="+f.Kind, "--env", "QS_BACKUP_ARCHIVE_HASH="+a.digest, f.ID, "/tool/native.test", "-test.run=^TestBackupNativeRestoreHelper$", "-test.v")
	if e != nil {
		t.Fatal("owned_actual_restore_or_validation_failed")
	}
	var v Verification
	found := 0
	for _, line := range strings.Split(string(raw), "\n") {
		if i := strings.Index(line, "safe_restore_verification="); i >= 0 {
			found++
			if exactJSON([]byte(line[i+len("safe_restore_verification="):]), &v) != nil {
				t.Fatal("owned_restore_safe_receipt_invalid")
			}
		}
	}
	if found != 1 || v.Database != f.Kind || v.ArchiveSHA256 != a.digest || !v.ContentEqual || !v.SchemaEqual || v.DropReady || !hashPattern.MatchString(v.RestoreIdentityHash) || v.ElapsedMillis < 0 || v.ElapsedMillis > 600000 {
		t.Fatal("owned_restore_not_proven")
	}
	if v.ProductionBoundRestoreBudgetProven || len(v.Targets) != v.TargetCount {
		t.Fatal("native_scale_misrepresented_as_production_budget")
	}
	for _, m := range v.Targets {
		if m.Database != f.Kind || m.SourceRecords != m.RestoredRecords || m.SourceRawBytes != m.RestoredRawBytes || m.MaxBatchRecords > 250 || m.ExplicitTransactions != 0 || m.ExplicitCommits != 0 || (m.Database == "mysql" && m.AutocommitInsertBatches != m.InsertStatements) {
			t.Fatal("actual_load_metrics_not_equal")
		}
	}
	after, e := nativeInspect(context.Background(), f.ID)
	if e != nil || !reflect.DeepEqual(before, after) || f.check(context.Background(), true) != nil {
		t.Fatal("owned_restore_topology_changed")
	}
	return v
}
func TestBackupNativeFourTargetNetworkNoneRestore(t *testing.T) {
	if os.Getenv("QS_RETIREMENT_BACKUP_NATIVE") != "1" {
		if os.Getenv("QS_RETIREMENT_BACKUP_NATIVE_REQUIRED") == "1" {
			t.Fatal("owned_native_fixture_opt_in_missing")
		}
		t.Skip("owned backup restore native fixture not requested")
	}
	mysqlRoot := nativeRoot(t, "owned-mysql.json", "34306")
	mongoRoot := nativeRoot(t, "owned-mongo.json", "33317")
	env := nativeEnv(t)
	dir := nativePrivateDir(t)
	sourceFixture := nativeStartSourceRS(t, dir, mongoRoot, env)
	_ = sourceFixture
	nativeJSON(t, filepath.Join(dir, "batch.private.json"), map[string]any{"kind": "owned_native_only", "production": false, "private_source_assets": true, "credential_environment": "borrowed_private_env_not_copied", "owned_root_mysql_cid": mysqlRoot.ID, "owned_root_mongo_cid": mongoRoot.ID})
	admin := nativeSQL(t, env, "")
	name := "qs_backup_source_" + primitive.NewObjectID().Hex()
	nativeExec(t, admin, "CREATE DATABASE "+quote(name)+" CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci")
	t.Cleanup(func() {
		if _, e := admin.ExecContext(context.Background(), "DROP DATABASE "+quote(name)); e != nil {
			nativeRetainCleanup(t, dir)
			t.Error("owned_source_database_cleanup_failed")
			return
		}
		rows, e := readSQL(context.Background(), admin, "SELECT SCHEMA_NAME FROM information_schema.schemata WHERE SCHEMA_NAME=?", name)
		if e != nil || len(rows) != 0 {
			nativeRetainCleanup(t, dir)
			t.Error("owned_source_database_remaining")
		} else {
			t.Log("owned_source_sql_database_remaining=0")
		}
	})
	db := nativeSQL(t, env, name)
	client := nativeMongo(t, env)
	mdb := client.Database(name)
	t.Cleanup(func() {
		if mdb.Drop(context.Background()) != nil {
			nativeRetainCleanup(t, dir)
			t.Error("owned_source_mongo_cleanup_failed")
			return
		}
		names, e := client.ListDatabaseNames(context.Background(), bson.D{{Key: "name", Value: name}})
		if e != nil || len(names) != 0 {
			nativeRetainCleanup(t, dir)
			t.Error("owned_source_mongo_database_remaining")
		} else {
			t.Log("owned_source_mongo_database_remaining=0")
		}
	})
	nativeSourceSchemas(t, db, mdb)
	nativeSourceData(t, db, mdb)
	env["MYSQL_DATABASE"], env["MONGODB_DBNAME"] = name, name
	cli, helper := nativeBuild(t, dir)
	inputs, approval := nativeInventory(t, cli, dir, env, db, mdb)
	archiveDir := filepath.Join(dir, "archive")
	if os.Mkdir(archiveDir, 0700) != nil {
		t.Fatal("owned_archive_directory_failed")
	}
	tx, e := db.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if e != nil {
		t.Fatal("actual_host_readonly_transaction_failed")
	}
	archive, e := Capture(context.Background(), BorrowedSources{SQL: tx, Mongo: mdb}, approval, inputs, archiveDir)
	closeErr := tx.Rollback()
	if e != nil {
		t.Fatal(e)
	}
	if closeErr != nil {
		t.Fatal("borrowed_transaction_was_closed_or_committed")
	}
	if archive.Summary().DropReady {
		t.Fatal("native backup authorized production")
	}
	// Verify that existing head, standard MQ and evidence records remain intact
	// in a fresh source read, not merely in the earlier repeatable-read snapshot.
	protected, e := readSQL(context.Background(), db, "SELECT version,dirty,(SELECT COUNT(*) FROM rm_outbox),(SELECT COUNT(*) FROM evidence_kept) FROM schema_migrations")
	if e != nil || len(protected) != 1 || cell(protected[0], 0) != "99" || cell(protected[0], 1) != "0" || cell(protected[0], 2) != "1" || cell(protected[0], 3) != "1" {
		t.Fatal("source_non_target_or_head_changed")
	}
	if count, e := mdb.Collection("rm_outbox").CountDocuments(context.Background(), bson.D{}); e != nil || count != 1 {
		t.Fatal("source_standard_mq_changed")
	}
	if e = mdb.Collection("schema_migrations").FindOne(context.Background(), bson.D{{Key: "version", Value: int64(38)}, {Key: "dirty", Value: false}}).Err(); e != nil {
		t.Fatal("source_mongo_head_changed")
	}
	nativeCaptureNegatives(t, db, admin, mdb, inputs, approval, dir)
	opened, e := OpenArchive(context.Background(), archiveDir, archive.digest)
	if e != nil || opened.digest != archive.digest {
		t.Fatal("actual_archive_reopen_failed")
	}
	started := time.Now()
	mysqlRestore := nativeStartRestore(t, dir, helper, archiveDir, "mysql", mysqlRoot)
	mongoRestore := nativeStartRestore(t, dir, helper, archiveDir, "mongodb", mongoRoot)
	sqlProof := nativeRunRestore(t, mysqlRestore, archive)
	mongoProof := nativeRunRestore(t, mongoRestore, archive)
	elapsed := time.Since(started)
	if elapsed > 600*time.Second || sqlProof.TargetCount != 3 || mongoProof.TargetCount != 1 {
		t.Fatal("actual_four_target_restore_budget_or_scope_failed")
	}
	if strings.Contains(sqlProof.RestoreIdentityHash, "PRIVATE") || strings.Contains(mongoProof.RestoreIdentityHash, "PRIVATE") {
		t.Fatal("private restore proof leaked")
	}
	nativeJSON(t, filepath.Join(nativePrivateRoot, "backup-native-observation-"+primitive.NewObjectID().Hex()+".json"), map[string]any{"format_version": 1, "sql": sqlProof, "mongodb": mongoProof, "restore_wall_millis": elapsed.Milliseconds(), "timing_scope": "start_two_owned_restore_instances_then_actual_restore_negative_checks_full_reread_excludes_inventory_capture_cleanup", "production_bound_restore_budget_proven": false, "load_metrics_scope": "actual_primary_restore_only_excludes_fixture_negative_repair"})
	for _, p := range []Verification{sqlProof, mongoProof} {
		for _, m := range p.Targets {
			t.Logf("actual_target_metrics database=%s name=%s records=%d source_raw_bytes=%d restored_raw_bytes=%d source_file_bytes=%d insert_statements=%d autocommit_insert_batches=%d explicit_transactions=%d explicit_commits=%d", m.Database, m.Name, m.RestoredRecords, m.SourceRawBytes, m.RestoredRawBytes, m.SourceFileBytes, m.InsertStatements, m.AutocommitInsertBatches, m.ExplicitTransactions, m.ExplicitCommits)
		}
	}
	t.Logf("actual_inventory_v2=true targets_restored=4 network_none=true published_ports=0 full_schema_equal=true full_content_equal=true ordered_compound_preserved=true restore_millis=%d source_heads_and_non_targets_preserved=true drop_ready=false", elapsed.Milliseconds())
	// Purge only after both container tools have completed. This is local fixture
	// acceptance; it does not stand in for production business acceptance.
	if e = mysqlRestore.cleanup(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e = mongoRestore.cleanup(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e = PurgeRegistered(archiveDir, approval); e != nil {
		t.Fatal(e)
	}
	entries, e := os.ReadDir(archiveDir)
	if e != nil || len(entries) != 0 {
		t.Fatal("owned_backup_original_assets_remaining")
	}
	t.Log("registered_backup_original_assets_remaining=0 owned_restore_container_remaining=0 owned_restore_volumes_remaining=0")
}

func safeNativeCategory(s string) bool {
	if len(s) < 1 || len(s) > 96 {
		return false
	}
	for _, c := range s {
		if (c < 'a' || c > 'z') && c != '_' {
			return false
		}
	}
	return true
}

type nativeSourceRS struct {
	ID, Name, Image, Owner, Port, Replica, User, CredentialFile string
	Labels, ContainerLabels                                     map[string]string
	Volumes                                                     []string
}

func nativeDockerInput(ctx context.Context, input []byte, args ...string) ([]byte, error) {
	q, c := context.WithTimeout(ctx, 30*time.Second)
	defer c()
	cmd := exec.CommandContext(q, "docker", args...)
	cmd.Stdin = bytes.NewReader(input)
	var out, errout bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errout
	if cmd.Run() != nil {
		return nil, Error("owned_source_bootstrap_command_failed")
	}
	if out.Len() > 128<<10 || errout.Len() > 128<<10 {
		return nil, Error("owned_source_bootstrap_output_budget_exceeded")
	}
	return out.Bytes(), nil
}
func (f *nativeSourceRS) check(ctx context.Context, running bool) error {
	id := f.ID
	if id == "" {
		id = f.Name
	}
	v, e := nativeInspect(ctx, id)
	if e != nil {
		return e
	}
	if (f.ID != "" && v.ID != f.ID) || len(v.ID) != 64 || v.Image != f.Image || strings.TrimPrefix(v.Name, "/") != f.Name || !reflect.DeepEqual(v.Labels, f.ContainerLabels) || (running && !v.Running) || v.Network != "bridge" {
		return ErrIsolation
	}
	requested := v.RequestedPorts[f.Port+"/tcp"]
	if len(v.RequestedPorts) != 1 || len(requested) != 1 || requested[0].HostIP != "127.0.0.1" || requested[0].HostPort != f.Port {
		return ErrIsolation
	}
	if v.Running {
		ports := v.Ports[f.Port+"/tcp"]
		if len(ports) != 1 || ports[0].HostIP != "127.0.0.1" || ports[0].HostPort != f.Port {
			return ErrIsolation
		}
	}
	expected := map[string]string{f.Volumes[0]: "/data/db", f.Volumes[1]: "/data/configdb"}
	seen := map[string]bool{}
	for _, m := range v.Mounts {
		if m.Type != "volume" || !m.RW || expected[m.Name] != m.Destination || seen[m.Name] {
			return ErrIsolation
		}
		seen[m.Name] = true
	}
	if len(seen) != 2 || len(v.Mounts) != 2 {
		return ErrIsolation
	}
	f.ID = v.ID
	return nil
}
func (f *nativeSourceRS) cleanup(ctx context.Context) error {
	if e := f.check(ctx, false); e == nil {
		if _, e = nativeDocker(ctx, "rm", "--force", f.ID); e != nil {
			return Error("owned_source_instance_remove_failed")
		}
	} else {
		raw, e := nativeDocker(ctx, "ps", "--all", "--filter", "name=^/"+f.Name+"$", "--format", "{{.ID}}")
		if e != nil || strings.TrimSpace(string(raw)) != "" {
			return Error("owned_source_cleanup_identity_unproven")
		}
	}
	for _, name := range f.Volumes {
		raw, e := nativeDocker(ctx, "volume", "inspect", "--format", "{{json .Labels}}", name)
		if e != nil {
			absent, ae := nativeDocker(ctx, "volume", "ls", "--filter", "name=^"+name+"$", "--format", "{{.Name}}")
			if ae != nil || strings.TrimSpace(string(absent)) != "" {
				return Error("owned_source_volume_absence_unproven")
			}
			continue
		}
		var labels map[string]string
		if json.Unmarshal(raw, &labels) != nil || !reflect.DeepEqual(labels, f.Labels) {
			return Error("owned_source_volume_identity_rejected")
		}
		if _, e = nativeDocker(ctx, "volume", "rm", name); e != nil {
			return Error("owned_source_volume_remove_failed")
		}
	}
	containers, e := nativeDocker(ctx, "ps", "--all", "--filter", "label=codex.owner="+f.Owner, "--format", "{{.ID}}")
	volumes, ve := nativeDocker(ctx, "volume", "ls", "--filter", "label=codex.owner="+f.Owner, "--format", "{{.Name}}")
	if e != nil || ve != nil || strings.TrimSpace(string(containers)) != "" || strings.TrimSpace(string(volumes)) != "" {
		return Error("owned_source_resources_remaining")
	}
	return nil
}
func nativeStartSourceRS(t *testing.T, dir string, root nativeContainer, env map[string]string) *nativeSourceRS {
	t.Helper()
	return nativeStartSourceRSWithCleanupGuard(t, dir, root, env, nil)
}

func nativeStartSourceRSWithCleanupGuard(t *testing.T, dir string, root nativeContainer, env map[string]string, cleanupAllowed func() bool) *nativeSourceRS {
	t.Helper()
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal("owned_source_port_reservation_failed")
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if listener.Close() != nil {
		t.Fatal("owned_source_port_reservation_close_failed")
	}
	owner := primitive.NewObjectID().Hex()
	f := &nativeSourceRS{Name: "qs-backup-source-" + owner, Owner: owner, Image: root.Image, Port: strconv.Itoa(port), Replica: "qsbackup_" + owner, User: "source_" + primitive.NewObjectID().Hex(), Volumes: []string{"qs-backup-source-data-" + owner, "qs-backup-source-config-" + owner}, Labels: map[string]string{"codex.task": "qs-retirement-backup-source-native", "codex.owner": owner}}
	password := make([]byte, 32)
	if _, e = rand.Read(password); e != nil {
		t.Fatal("owned_source_credential_random_failed")
	}
	secret := base64.RawURLEncoding.EncodeToString(password)
	f.CredentialFile = filepath.Join(dir, "source-credentials.private.json")
	nativeJSON(t, f.CredentialFile, map[string]string{"user": f.User, "password": secret})
	imageLabels, e := nativeDocker(context.Background(), "image", "inspect", "--format", "{{json .Config.Labels}}", root.Image)
	if e != nil || json.Unmarshal(imageLabels, &f.ContainerLabels) != nil {
		t.Fatal("owned_source_image_binding_failed")
	}
	if f.ContainerLabels == nil {
		f.ContainerLabels = map[string]string{}
	}
	for k, v := range f.Labels {
		f.ContainerLabels[k] = v
	}
	nativeJSON(t, filepath.Join(dir, "source-rs-requested.private.json"), f)
	t.Cleanup(func() {
		if cleanupAllowed != nil && !cleanupAllowed() {
			nativeRetainCleanup(t, dir)
			t.Error("owned_source_producer_stop_unproven_resources_retained")
			return
		}
		if e := f.cleanup(context.Background()); e != nil {
			nativeRetainCleanup(t, dir)
			t.Error(e)
		} else {
			t.Log("owned_source_rs_remaining=0 owned_source_volumes_remaining=0 owned_source_user_persistence_remaining=0")
		}
	})
	for _, v := range f.Volumes {
		if _, e = nativeDocker(context.Background(), "volume", "create", "--label", "codex.task="+f.Labels["codex.task"], "--label", "codex.owner="+owner, v); e != nil {
			t.Fatal("owned_source_volume_create_failed")
		}
	}
	args := []string{"run", "--detach", "--name", f.Name, "--label", "codex.task=" + f.Labels["codex.task"], "--label", "codex.owner=" + owner, "--network", "bridge", "--publish", "127.0.0.1:" + f.Port + ":" + f.Port, "--mount", "type=volume,src=" + f.Volumes[0] + ",dst=/data/db", "--mount", "type=volume,src=" + f.Volumes[1] + ",dst=/data/configdb", "--user", "0", "--entrypoint", "mongod", f.Image, "--bind_ip_all", "--port", f.Port, "--replSet", f.Replica}
	if _, e = nativeDocker(context.Background(), args...); e != nil {
		t.Fatal("owned_source_rs_create_failed")
	}
	if e = f.check(context.Background(), true); e != nil {
		t.Fatal(e)
	}
	nativeJSON(t, filepath.Join(dir, "source-rs-created.private.json"), f)
	init := `const v=JSON.parse(require("fs").readFileSync(0,"utf8"));const r=db.getSiblingDB("admin").runCommand({replSetInitiate:{_id:v.replica,members:[{_id:0,host:v.endpoint}]}});if(r.ok!==1){quit(2);}`
	input, _ := json.Marshal(map[string]string{"replica": f.Replica, "endpoint": "127.0.0.1:" + f.Port})
	for i := 0; ; i++ {
		if _, e = nativeDockerInput(context.Background(), input, "exec", "-i", f.ID, "mongosh", "--quiet", "--norc", "--port", f.Port, "--eval", init); e == nil {
			break
		}
		if i >= 30 {
			t.Fatal("owned_source_replica_initiate_failed")
		}
		time.Sleep(100 * time.Millisecond)
	}
	create := `const v=JSON.parse(require("fs").readFileSync(0,"utf8"));const a=db.getSiblingDB("admin");if(a.runCommand({hello:1}).isWritablePrimary!==true){quit(3);}const r=a.runCommand({createUser:v.user,pwd:v.password,roles:[{role:"root",db:"admin"}],mechanisms:["SCRAM-SHA-256"]});if(r.ok!==1){quit(2);}`
	input, _ = json.Marshal(map[string]string{"user": f.User, "password": secret})
	for i := 0; ; i++ {
		if _, e = nativeDockerInput(context.Background(), input, "exec", "-i", f.ID, "mongosh", "--quiet", "--norc", "--port", f.Port, "--eval", create); e == nil {
			break
		}
		if i >= 30 {
			t.Fatal("owned_source_user_create_failed")
		}
		time.Sleep(100 * time.Millisecond)
	}
	env["MONGODB_HOST"], env["MONGODB_PORT"], env["MONGODB_USERNAME"], env["MONGODB_PASSWORD"] = "127.0.0.1", f.Port, f.User, secret
	// Authentication is real, but this isolated source fixture intentionally has
	// no --auth/keyfile. It proves discovery/copy bytes, not production privileges.
	return f
}

func nativeSQLStructureNegatives(t *testing.T, ctx context.Context, conn *sql.Conn, a *Archive) {
	t.Helper()
	step := 0
	exec := func(statement string) {
		step++
		if _, e := conn.ExecContext(ctx, statement); e != nil {
			if se, ok := e.(*mysql.MySQLError); ok {
				t.Fatalf("owned_structure_negative_setup_failed step=%d server_code=%d", step, se.Number)
			}
			t.Fatalf("owned_structure_negative_setup_failed step=%d server_code_unknown=true", step)
		}
	}
	check := func(i int) {
		got, e := sqlStructure(ctx, conn, i)
		if e != nil || sqlStructuresEqual(a.data.SQL[i], got) {
			t.Fatal("actual_sql_structure_change_not_refused")
		}
	}
	exec("ALTER TABLE domain_event_outbox MODIFY event_id VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL")
	check(0)
	exec("ALTER TABLE domain_event_outbox MODIFY event_id VARCHAR(64) CHARACTER SET latin1 COLLATE latin1_swedish_ci NOT NULL")
	check(0)
	exec("ALTER TABLE domain_event_outbox MODIFY event_id VARCHAR(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL")
	exec("ALTER TABLE domain_event_outbox DROP INDEX idx_status_next_attempt_at")
	check(0)
	exec("ALTER TABLE domain_event_outbox ADD INDEX idx_status_next_attempt_at(status,next_attempt_at)")
	exec("ALTER TABLE domain_event_outbox COMMENT='owned_structure_negative'")
	check(0)
	exec("ALTER TABLE domain_event_outbox COMMENT=''")
	keys, e := readSQL(ctx, conn, "SELECT CONSTRAINT_NAME FROM information_schema.key_column_usage WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='ai_bridge_commands' AND COLUMN_NAME='request_id' AND REFERENCED_TABLE_NAME='ai_bridge_requests'")
	if e != nil || len(keys) != 1 || cell(keys[0], 0) == "" {
		t.Fatal("owned_fk_negative_identity_failed")
	}
	exec("ALTER TABLE ai_bridge_commands DROP FOREIGN KEY " + quote(cell(keys[0], 0)))
	check(1)
	exec("SET SESSION FOREIGN_KEY_CHECKS=0")
	exec("ALTER TABLE ai_bridge_commands ADD CONSTRAINT " + quote(cell(keys[0], 0)) + " FOREIGN KEY(request_id) REFERENCES ai_bridge_requests(request_id)")
	exec("SET SESSION FOREIGN_KEY_CHECKS=1")
	// Replaying removed indexes can change SHOW CREATE's display order. Rebuild
	// only this owned isolated target from its original complete DDL and bytes.
	// Preserve strict remaining DDL comparison rather than ignoring index order.
	exec("DROP TABLE domain_event_outbox")
	exec(a.data.SQL[0].DDL)
	repair, e := loadSQLRows(ctx, conn, a, 0)
	if e != nil {
		t.Fatal("owned_structure_negative_content_replay_failed")
	}
	t.Logf("owned_fixture_negative_repair_insert_statements=%d autocommit_batches=%d rows=%d excluded_from_primary_load_metrics=true", repair.InsertStatements, repair.AutocommitInsertBatches, repair.RestoredRecords)
	for i := range a.data.SQL {
		got, e := sqlStructure(ctx, conn, i)
		if e != nil || !sqlStructuresEqual(a.data.SQL[i], got) || verifySQLContent(ctx, conn, got, a.data.Inventory.Targets[i], i) != nil {
			t.Fatal("owned_structure_negative_restore_not_equal")
		}
	}
	t.Log("actual_structure_negative_charset_collation_index_fk_table_option_rejected=true final_schema_and_content_equal=true")
}

func nativeCaptureNegatives(t *testing.T, db, admin *sql.DB, mdb *mongo.Database, input Inputs, approval Approval, dir string) {
	t.Helper()
	rewind := func() {
		for _, r := range append([]io.Reader{input.Inventory, input.SQLMetadata, input.MongoMetadata}, input.Sources[:]...) {
			f, ok := r.(io.Seeker)
			if !ok {
				t.Fatal("owned_rewind_not_supported")
			}
			if _, e := f.Seek(0, io.SeekStart); e != nil {
				t.Fatal("owned_rewind_failed")
			}
		}
	}
	for _, kind := range []string{"wrong_sql_identity", "ordered_schema_not_independently_approved", "partial_source", "actual_source_changed"} {
		rewind()
		ownInput := input
		ap := approval
		hostDB := db
		readOnly := true
		want := ErrApproval
		switch kind {
		case "wrong_sql_identity":
			hostDB = admin
			want = ErrIdentity
		case "ordered_schema_not_independently_approved":
			ap.OrderedMongoSchemaSHA256 = strings.Repeat("0", 64)
		case "partial_source":
			f, ok := input.Sources[0].(*os.File)
			if !ok {
				t.Fatal("owned_source_file_missing")
			}
			st, e := f.Stat()
			if e != nil || st.Size() < 32 {
				t.Fatal("owned_source_stat_failed")
			}
			ownInput.Sources[0] = io.LimitReader(f, st.Size()-32)
			want = ErrSource
		case "actual_source_changed":
			readOnly = false
			want = ErrContent
		}
		tx, e := hostDB.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: readOnly})
		if e != nil {
			t.Fatal("owned_negative_transaction_failed")
		}
		if kind == "actual_source_changed" {
			if _, e = tx.ExecContext(context.Background(), "UPDATE domain_event_outbox SET payload_json='PRIVATE_CHANGED_BACKUP_SOURCE' WHERE id=1"); e != nil {
				if tx.Rollback() != nil {
					t.Error("owned_negative_rollback_failed")
				}
				t.Fatal("owned_negative_source_change_failed")
			}
		}
		out := filepath.Join(dir, "negative-"+primitive.NewObjectID().Hex())
		if os.Mkdir(out, 0700) != nil {
			if tx.Rollback() != nil {
				t.Error("owned_negative_rollback_failed")
			}
			t.Fatal("owned_negative_directory_failed")
		}
		a, err := Capture(context.Background(), BorrowedSources{SQL: tx, Mongo: mdb}, ap, ownInput, out)
		if tx.Rollback() != nil {
			t.Fatal("capture_closed_borrowed_negative_transaction")
		}
		if a != nil || err != want {
			t.Fatalf("actual_capture_negative_refusal_not_equal category=%s", kind)
		}
		if _, e = os.Stat(filepath.Join(out, "assets.private.json")); e == nil {
			if PurgeRegistered(out, ap) != nil {
				t.Fatal("owned_rejected_assets_purge_failed")
			}
		} else if !os.IsNotExist(e) {
			t.Fatal("owned_rejected_registry_stat_failed")
		}
		if entries, e := os.ReadDir(out); e != nil || len(entries) != 0 {
			t.Fatal("owned_rejected_assets_remaining")
		}
	}
	t.Log("actual_capture_wrong_identity_ordered_approval_partial_source_changed_source_rejected=true borrowed_transactions_preserved=true rejected_original_assets_remaining=0")
}

func nativeRetainCleanup(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, "cleanup-unresolved.private.json")
	if _, e := os.Lstat(path); e == nil {
		return
	}
	if writePrivate(path, []byte(`{"format_version":1,"cleanup_unresolved":true,"resume_allowed":false}`)) != nil {
		t.Error("owned_cleanup_checkpoint_write_failed")
	}
}
