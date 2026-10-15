//go:build integration

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	qsMigration "github.com/FangcunMount/qs-server/internal/pkg/migration"
	"github.com/FangcunMount/qs-server/pkg/version"
	"github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	migratemongo "github.com/golang-migrate/migrate/v4/database/mongodb"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type nativeFixture struct {
	t                          *testing.T
	name, dsn, uri, tool       string
	root, db                   *sql.DB
	client                     *mongo.Client
	dir                        string
	materialPath, materialHash string
	material                   material
}

func requireNative(t *testing.T) (string, string, string) {
	t.Helper()
	if os.Getenv("QS_M4_OBJECT_NATIVE_REQUIRED") != "1" {
		t.Skip("owned native dependencies not selected")
	}
	dsn, uri := os.Getenv("QS_M4_OBJECT_NATIVE_MYSQL_DSN"), os.Getenv("QS_M4_OBJECT_NATIVE_MONGO_URI")
	cfg, e := mysql.ParseDSN(dsn)
	if e != nil || dsn == "" || cfg.Net != "tcp" {
		t.Fatal("required owned MySQL DSN missing")
	}
	host, _, e := net.SplitHostPort(cfg.Addr)
	if e != nil || (host != "127.0.0.1" && host != "localhost") {
		t.Fatal("native endpoint must be owned loopback")
	}
	u, e := url.Parse(uri)
	if e != nil || uri == "" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") || u.Query().Get("replicaSet") != "rs0" {
		t.Fatal("required owned rs0 loopback URI missing")
	}
	if os.Getenv("QS_M4_OBJECT_NATIVE_DISPOSABLE_ACK") != "owned_synthetic_dependencies_only" || !sourceRE.MatchString(version.GitCommit) {
		t.Fatal("owned fixture acknowledgment and actual compiled source required")
	}
	return dsn, uri, version.GitCommit
}
func nativeSource(t *testing.T, backend string, through int) fstest.MapFS {
	t.Helper()
	root := "../../../internal/pkg/migration/migrations/" + backend
	out := fstest.MapFS{}
	entries, e := os.ReadDir(root)
	if e != nil {
		t.Fatal(e)
	}
	for _, entry := range entries {
		name := entry.Name()
		if len(name) < 7 || (!strings.Contains(name, ".up.") && !strings.Contains(name, ".down.")) {
			continue
		}
		n, e := strconv.Atoi(name[:6])
		if e != nil || n > through {
			continue
		}
		raw, e := os.ReadFile(filepath.Join(root, name))
		if e != nil {
			t.Fatal(e)
		}
		out[name] = &fstest.MapFile{Data: raw, Mode: 0444}
	}
	return out
}
func newNative(t *testing.T) *nativeFixture {
	return newNativeOriginalCharset(t, "utf8mb4")
}

func newNativeOriginalCharset(t *testing.T, charset string) *nativeFixture {
	t.Helper()
	if charset != "utf8" && charset != "utf8mb4" {
		t.Fatal("fixed original fixture charset required")
	}
	dsn, uri, tool := requireNative(t)
	cfg, _ := mysql.ParseDSN(dsn)
	if cfg.Params == nil {
		cfg.Params = map[string]string{}
	}
	cfg.Params["charset"] = charset
	cfg.MultiStatements = true
	cfg.ParseTime = false
	root, e := sql.Open("mysql", cfg.FormatDSN())
	if e != nil {
		t.Fatal(e)
	}
	root.SetMaxOpenConns(1)
	name := fmt.Sprintf("qs_m4_object_test_%d", time.Now().UnixNano())
	if _, e = root.ExecContext(t.Context(), "CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4"); e != nil {
		_ = root.Close()
		t.Fatal(e)
	}
	h := &nativeFixture{t: t, name: name, root: root, uri: uri, tool: tool, dir: privateTestDir(t)}
	t.Cleanup(func() {
		if h.db != nil {
			if e := h.db.Close(); e != nil {
				t.Error("fixture SQL pool release", e)
			}
		}
		if h.client != nil {
			ctx, c := context.WithTimeout(context.Background(), 5*time.Second)
			if e := h.client.Database(name).Drop(ctx); e != nil {
				t.Error("owned Mongo namespace cleanup", e)
			}
			if e := h.client.Disconnect(ctx); e != nil {
				t.Error("fixture Mongo release", e)
			}
			c()
		}
		if _, e := root.Exec("DROP DATABASE `" + name + "`"); e != nil {
			t.Error("owned SQL namespace cleanup", e)
		}
		if e := root.Close(); e != nil {
			t.Error("fixture root release", e)
		}
	})
	var actual string
	if root.QueryRow("SELECT VERSION()").Scan(&actual) != nil || actual != "8.0.36" {
		t.Fatal("actual MySQL8.0.36 required")
	}
	cfg.DBName = name
	h.dsn = cfg.FormatDSN()
	h.db, e = sql.Open("mysql", h.dsn)
	if e != nil {
		t.Fatal(e)
	}
	h.db.SetMaxOpenConns(1)
	h.migrateSQL(nativeSource(t, "mysql", 100), 100)
	h.client, e = mongo.Connect(t.Context(), options.Client().ApplyURI(uri).SetMaxPoolSize(1).SetTimeout(5*time.Second))
	if e != nil {
		t.Fatal(e)
	}
	var build struct {
		Version string `bson:"version"`
	}
	if h.client.Database("admin").RunCommand(t.Context(), bson.D{{Key: "buildInfo", Value: 1}}).Decode(&build) != nil || build.Version != "7.0.31" {
		t.Fatal("actual Mongo7.0.31 required")
	}
	h.migrateMongo(nativeSource(t, "mongodb", 39), 39)
	for _, id := range []uint64{4101, 4102} {
		if _, e = h.db.Exec("INSERT INTO assessment(id,org_id,testee_id,questionnaire_code,questionnaire_version,answer_sheet_id,origin_type,status,evaluation_model_kind,evaluation_model_code,evaluation_model_version,primary_score_value) VALUES(?,1,2,'owned-m4','1',?,'direct','evaluated','scale','owned-model','1',11.5)", id, id); e != nil {
			t.Fatal(e)
		}
		if _, e = h.db.Exec("INSERT INTO evaluation_outcome(id,org_id,assessment_id,testee_id,evaluation_run_id,model_kind,model_code,model_version,payload_json,report_input_json,evaluated_at,committed_event_id,committed_event_evidence) VALUES(?,1,?,2,?,'scale','owned-model','1',?,?,'2026-10-13 00:00:00',?,?)", id+1000, id, fmt.Sprintf("owned-%d", id), `{"fixture":"NULL remains distinct from empty"}`, `{"fixture":"frozen retain"}`, fmt.Sprintf("event-%d", id), `{"version":1}`); e != nil {
			t.Fatal(e)
		}
	}
	t.Logf("actual dependencies MySQL=%s Mongo=%s rs0; owned namespace=%s; compiled source=%s", actual, build.Version, name, tool)
	return h
}

func (h *nativeFixture) toolCharset(charset string) {
	h.t.Helper()
	if charset != "utf8mb3" && charset != "utf8mb4" {
		h.t.Fatal("fixed opposite fixture charset required")
	}
	cfg, e := mysql.ParseDSN(h.dsn)
	if e != nil {
		h.t.Fatal(e)
	}
	if cfg.Params == nil {
		cfg.Params = map[string]string{}
	}
	cfg.Params["charset"] = charset
	h.dsn = cfg.FormatDSN()
	pool, e := sql.Open("mysql", h.dsn)
	if e != nil {
		h.t.Fatal(e)
	}
	defer func() {
		if e := pool.Close(); e != nil {
			h.t.Error("opposite charset witness release", e)
		}
	}()
	var client, connection string
	if e = pool.QueryRowContext(h.t.Context(), "SELECT @@character_set_client,@@character_set_connection").Scan(&client, &connection); e != nil || client != charset || connection != charset {
		h.t.Fatal("actual opposite connection charset not installed", e)
	}
	h.t.Logf("actual tool DSN connection charset client=%s connection=%s", client, connection)
}

func (h *nativeFixture) fullDDL() [2]string {
	h.t.Helper()
	var out [2]string
	for i, object := range objects {
		var e error
		out[i], e = ddl(h.t.Context(), h.db, object.table)
		if e != nil {
			h.t.Fatal(e)
		}
	}
	return out
}

func TestM4SQLObjectNativeCharsetBoundRecovery(t *testing.T) {
	for _, tc := range []struct {
		name, original, opposite string
		encoding                 originalEncoding
		drops                    int
		clean                    bool
	}{
		{"UTF8MB3DirtyFirstDDLToolMB4", "utf8", "utf8mb4", originalUTF8MB3, 1, false},
		{"UTF8MB3CleanBothDDLToolMB4", "utf8", "utf8mb4", originalUTF8MB3, 2, true},
		{"UTF8MB4DirtyFirstDDLToolMB3", "utf8mb4", "utf8mb3", originalUTF8MB4, 1, false},
		{"UTF8MB4CleanBothDDLToolMB3", "utf8mb4", "utf8mb3", originalUTF8MB4, 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newNativeOriginalCharset(t, tc.original)
			original := h.fullDDL()
			for i := range objects {
				encoding, e := validateOriginalObjectDefinition(original[i], i)
				if e != nil || encoding != tc.encoding {
					t.Fatal("real original 099 encoding did not match creation connection", e)
				}
			}
			h.toolCharset(tc.opposite)
			h.capture()
			for i := range objects {
				if h.material.Tables[i].OriginalDDL != original[i] {
					t.Fatal("opposite capture connection changed original SHOW CREATE")
				}
			}
			h.upgrade(tc.drops, tc.clean)
			args, _ := h.restoreInput()
			r := h.action("restore", args...)
			if !r.Complete || !r.SQLConnectionClosed || !r.SQLPoolClosed || !r.MongoClosed || r.TransactionReleaseUnproven {
				t.Fatal("cross-charset restore not complete/released", r)
			}
			if h.fullDDL() != original {
				t.Fatal("full original SHOW CREATE not byte-identical after restoration")
			}
			h.assertHead(100, false)
			r = h.action("inspect")
			if !r.Complete {
				t.Fatal("fresh clean10039 inspect failed", r)
			}
		})
	}
}

func TestM4SQLObjectNativeExistingEncodingMismatchRefusedBeforeClaim(t *testing.T) {
	h := newNativeOriginalCharset(t, "utf8")
	h.toolCharset("utf8mb4")
	h.capture()
	h.upgrade(1, false)
	other, e := originalCheckDefinition(1, originalUTF8MB4)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = h.db.Exec("ALTER TABLE evaluation_outcome DROP CHECK chk_outcome_historical_committed, ADD " + other); e != nil {
		t.Fatal("owned known-alternate CHECK setup failed", e)
	}
	before := h.fullDDL()
	args, _ := h.restoreInput()
	r := h.action("restore", args...)
	if r.Complete || r.Error != "existing_original_check_encoding_changed" || !r.SQLConnectionClosed || !r.SQLPoolClosed || !r.MongoClosed {
		t.Fatal("known alternate encoding was not rejected before claim", r)
	}
	if h.fullDDL() != before {
		t.Fatal("early mismatch refusal changed an object")
	}
	h.assertHead(101, true)
	assertOwnedRecoveryUnclaimed(t, h, args)
}
func (h *nativeFixture) migrateSQL(source fs.FS, target uint) {
	h.t.Helper()
	sd, e := iofs.New(source, ".")
	if e != nil {
		h.t.Fatal(e)
	}
	cfg, _ := mysql.ParseDSN(h.dsn)
	pool, e := sql.Open("mysql", cfg.FormatDSN())
	if e != nil {
		h.t.Fatal(e)
	}
	dd, e := migratemysql.WithInstance(pool, &migratemysql.Config{DatabaseName: h.name})
	if e != nil {
		_ = pool.Close()
		h.t.Fatal(e)
	}
	m, e := migrate.NewWithInstance("iofs", sd, "mysql", dd)
	if e != nil {
		h.t.Fatal(e)
	}
	e = m.Migrate(target)
	se, de := m.Close()
	if e != nil && e != migrate.ErrNoChange {
		h.t.Fatal(e)
	}
	if se != nil || de != nil {
		h.t.Fatal("migration driver release failed")
	}
}
func (h *nativeFixture) migrateMongo(source fs.FS, target uint) {
	h.t.Helper()
	sd, e := iofs.New(source, ".")
	if e != nil {
		h.t.Fatal(e)
	}
	client, e := mongo.Connect(h.t.Context(), options.Client().ApplyURI(h.uri))
	if e != nil {
		h.t.Fatal(e)
	}
	dd, e := migratemongo.WithInstance(client, &migratemongo.Config{DatabaseName: h.name, MigrationsCollection: "schema_migrations", TransactionMode: false})
	if e != nil {
		_ = client.Disconnect(h.t.Context())
		h.t.Fatal(e)
	}
	cleanup, e := qsMigration.NewMongoDriver(client).PrepareRun(h.t.Context(), &qsMigration.Config{Database: h.name}, 0)
	if e != nil {
		h.t.Fatal("normal historical preparation failed", e)
	}
	m, e := migrate.NewWithInstance("iofs", sd, "mongodb", dd)
	if e != nil {
		h.t.Fatal(e)
	}
	e = m.Migrate(target)
	cleanupErr := cleanup(h.t.Context())
	se, de := m.Close()
	if e != nil && e != migrate.ErrNoChange {
		h.t.Fatal(e)
	}
	if se != nil || de != nil || cleanupErr != nil {
		h.t.Fatal("Mongo migration driver release failed")
	}
}
func (h *nativeFixture) action(action string, extra ...string) receipt {
	h.t.Helper()
	args := []string{"--action=" + action, "--mysql-database=" + h.name, "--mongo-database=" + h.name, "--timeout=2m"}
	args = append(args, extra...)
	return run(h.t.Context(), args, func(key string) string {
		if key == "QS_M4_OBJECT_MYSQL_DSN" {
			return h.dsn
		}
		if key == "QS_M4_OBJECT_MONGO_URI" {
			return h.uri
		}
		return ""
	}, h.tool, nil)
}
func (h *nativeFixture) capture() {
	h.t.Helper()
	h.materialPath = filepath.Join(h.dir, "first-capture.json")
	r := h.action("capture", "--capture-file="+h.materialPath)
	if !r.Complete || !r.SQLConnectionClosed || !r.SQLPoolClosed || !r.MongoClosed || r.ProductionFreezeCertified {
		h.t.Fatal(r)
	}
	h.materialHash = r.MaterialHash
	raw, e := readPrivate(h.materialPath, h.materialHash, maxMaterialBytes)
	if e != nil || strictDecode(raw, &h.material) != nil {
		h.t.Fatal("actual first capture unavailable", e)
	}
	for _, table := range h.material.Tables {
		if len(table.Rows) != 2 || !table.EOF || !table.SQLNullOnly {
			h.t.Fatal("incomplete real capture")
		}
	}
}
func (h *nativeFixture) upgrade(drops int, clean bool) {
	h.t.Helper()
	raw, e := os.ReadFile("../../../internal/pkg/migration/migrations/mysql/000101_retire_historical_event_evidence.up.sql")
	if e != nil {
		h.t.Fatal(e)
	}
	if clean {
		h.migrateSQL(nativeSource(h.t, "mysql", 101), 101)
		return
	}
	parts := strings.Split(string(raw), "ALTER TABLE `evaluation_outcome`")
	if len(parts) != 2 {
		h.t.Fatal("actual101resource split failed")
	}
	fault := ""
	if drops > 0 {
		fault = parts[0]
	}
	if drops > 1 {
		fault += "ALTER TABLE `evaluation_outcome`" + parts[1]
	}
	fault += "\nSIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='owned fixture interrupted101';\n"
	source := nativeSource(h.t, "mysql", 100)
	source["000101_owned_fault.up.sql"] = &fstest.MapFile{Data: []byte(fault)}
	source["000101_owned_fault.down.sql"] = &fstest.MapFile{Data: []byte("SELECT 1;")}
	sd, e := iofs.New(source, ".")
	if e != nil {
		h.t.Fatal(e)
	}
	pool, e := sql.Open("mysql", h.dsn)
	if e != nil {
		h.t.Fatal(e)
	}
	dd, e := migratemysql.WithInstance(pool, &migratemysql.Config{DatabaseName: h.name})
	if e != nil {
		h.t.Fatal(e)
	}
	m, e := migrate.NewWithInstance("iofs", sd, "mysql", dd)
	if e != nil {
		h.t.Fatal(e)
	}
	e = m.Steps(1)
	se, de := m.Close()
	if e == nil || se != nil || de != nil {
		h.t.Fatal("actual interruption/release not observed", e, se, de)
	}
	h.assertHead(101, true)
}
func (h *nativeFixture) assertHead(want uint64, dirty bool) {
	h.t.Helper()
	var v uint64
	var d bool
	if h.db.QueryRow("SELECT version,dirty FROM schema_migrations").Scan(&v, &d) != nil || v != want || d != dirty {
		h.t.Fatalf("actual ledger=%d dirty=%t; expected=%d/%t", v, d, want, dirty)
	}
}
func (h *nativeFixture) restoreInput() ([]string, string) {
	h.t.Helper()
	now := time.Now().UTC()
	a := approval{Format: 1, Kind: "m4_a_pre_start_sql_object_restore", Operation: fmt.Sprintf("%d-1", now.UnixNano()), ToolSource: h.tool, ASource: aSource, AResources: aResources, AResourceFiles: aResourceFiles, MaterialHash: h.materialHash, FreezeHash: strings.Repeat("a", 64), ResponsibilityHash: strings.Repeat("b", 64), SQLIdentity: h.material.SQLIdentity, MongoIdentity: h.material.MongoIdentity, RuntimeFrozen: true, RuntimeNotStarted: true, Marker: filepath.Join(h.dir, "runtime-command.marker"), WindowStarted: now.Add(-time.Minute).Format(time.RFC3339Nano), RecoveryDecision: now.Format(time.RFC3339Nano), Deadline: now.Add(time.Minute).Format(time.RFC3339Nano), Expires: now.Add(2 * time.Minute).Format(time.RFC3339Nano), RetentionDays: 30, DataOwner: "system (10001)", RecoveryOwner: "system (10001)"}
	raw, _ := json.Marshal(a)
	p := filepath.Join(h.dir, a.Operation+"-approval.json")
	if e := writeExclusive(p, raw); e != nil {
		h.t.Fatal(e)
	}
	return []string{"--material-file=" + h.materialPath, "--material-sha256=" + h.materialHash, "--approval-file=" + p, "--approval-sha256=" + fingerprint(raw)}, a.Marker
}
func (h *nativeFixture) limited(withoutAlter bool) func() {
	h.t.Helper()
	name := strings.Replace(h.name, "qs_m4_object_test_", "qs_m4_u_", 1)
	password := "owned-only-synthetic"
	if _, e := h.root.Exec("CREATE USER '" + name + "'@'%' IDENTIFIED BY '" + password + "'"); e != nil {
		h.t.Fatal(e)
	}
	grants := "SELECT, ALTER"
	if withoutAlter {
		grants = "SELECT"
	}
	if _, e := h.root.Exec("GRANT " + grants + " ON `" + h.name + "`.* TO '" + name + "'@'%'"); e != nil {
		h.t.Fatal(e)
	}
	original := h.dsn
	cfg, _ := mysql.ParseDSN(original)
	cfg.User = name
	cfg.Passwd = password
	h.dsn = cfg.FormatDSN()
	return func() {
		h.dsn = original
		if _, e := h.root.Exec("DROP USER '" + name + "'@'%'"); e != nil {
			h.t.Error("owned principal cleanup", e)
		}
	}
}
func TestM4SQLObjectNativeCaptureAndTargetVerification(t *testing.T) {
	h := newNative(t)
	h.capture()
	h.upgrade(2, true)
	r := h.action("verify-target", "--material-file="+h.materialPath, "--material-sha256="+h.materialHash)
	if !r.Complete || !r.ReadOnly || !r.SQLConnectionClosed || !r.MongoClosed {
		t.Fatal(r)
	}
	if _, e := h.db.Exec("UPDATE assessment SET primary_score_value=99 WHERE id=4101"); e != nil {
		t.Fatal(e)
	}
	r = h.action("verify-target", "--material-file="+h.materialPath, "--material-sha256="+h.materialHash)
	if r.Complete || r.Error != "retained_structure_or_fact_changed" {
		t.Fatal(r)
	}
}

func TestM4SQLObjectNativeOriginalDefinitionDriftRefused(t *testing.T) {
	for _, kind := range []string{"CheckChanged", "ColumnMoved"} {
		t.Run(kind, func(t *testing.T) {
			h := newNative(t)
			h.capture()
			statement := "ALTER TABLE assessment DROP CHECK chk_assessment_historical_lifecycle, ADD CONSTRAINT chk_assessment_historical_lifecycle CHECK (historical_lifecycle_evidence IS NULL)"
			if kind == "ColumnMoved" {
				statement = "ALTER TABLE assessment MODIFY COLUMN historical_lifecycle_evidence JSON NULL FIRST"
			}
			if _, e := h.db.Exec(statement); e != nil {
				t.Fatal("owned schema drift fixture failed", e)
			}
			r := h.action("capture", "--capture-file="+filepath.Join(h.dir, "drift-rejected.json"))
			if r.Complete || r.Error != "original_099_object_definition_rejected" || !r.SQLConnectionClosed || !r.SQLPoolClosed || !r.MongoClosed {
				t.Fatal(r)
			}
			if _, e := os.Stat(filepath.Join(h.dir, "drift-rejected.json")); !os.IsNotExist(e) {
				t.Fatal("drifted original capture material was created")
			}
			h.upgrade(0, false)
			args, _ := h.restoreInput()
			var before, after string
			var table string
			if e := h.db.QueryRow("SHOW CREATE TABLE assessment").Scan(&table, &before); e != nil {
				t.Fatal(e)
			}
			r = h.action("restore", args...)
			if r.Complete || r.Error != "original_099_object_definition_rejected" || !r.SQLConnectionClosed || !r.SQLPoolClosed || !r.MongoClosed {
				t.Fatal(r)
			}
			if e := h.db.QueryRow("SHOW CREATE TABLE assessment").Scan(&table, &after); e != nil || after != before {
				t.Fatal("early refusal changed the drifted original structure", e)
			}
			h.assertHead(101, true)
			assertOwnedRecoveryUnclaimed(t, h, args)
		})
	}
}
func TestM4SQLObjectNativeRecoveryMatrix(t *testing.T) {
	for _, tc := range []struct {
		name  string
		drops int
		clean bool
	}{{"DirtyBeforeAnyDDL", 0, false}, {"DirtyAfterFirstDDL", 1, false}, {"DirtyAfterBothDDL", 2, false}, {"Clean101BeforeFirstRuntime", 2, true}} {
		t.Run(tc.name, func(t *testing.T) {
			h := newNative(t)
			h.capture()
			h.upgrade(tc.drops, tc.clean)
			args, _ := h.restoreInput()
			r := h.action("restore", args...)
			if !r.Complete || r.ReadOnly || !r.SQLConnectionClosed || !r.SQLPoolClosed || !r.MongoClosed {
				t.Fatal(r)
			}
			h.assertHead(100, false)
			r = h.action("inspect")
			if !r.Complete {
				t.Fatal(r)
			}
			r = h.action("restore", args...)
			if r.Complete {
				t.Fatal("consumed restore accepted again")
			}
		})
	}
}
func TestM4SQLObjectNativePermissionAndLedgerFailure(t *testing.T) {
	for _, noAlter := range []bool{true, false} {
		name := "LedgerDeleteDeniedAfterObjectsVerified"
		if noAlter {
			name = "DDLPermissionDenied"
		}
		t.Run(name, func(t *testing.T) {
			h := newNative(t)
			h.capture()
			h.upgrade(2, false)
			args, _ := h.restoreInput()
			undo := h.limited(noAlter)
			r := h.action("restore", args...)
			undo()
			if r.Complete || !r.SQLConnectionClosed || !r.SQLPoolClosed || !r.MongoClosed {
				t.Fatal(r)
			}
			expected := "ledger_transaction_replay_failed"
			if noAlter {
				expected = "object_ddl_restore_failed_or_unknown"
			}
			if r.Error != expected {
				t.Fatal(r)
			}
			h.assertHead(101, true)
			for _, o := range objects {
				var n int
				if h.db.QueryRow("SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=? AND TABLE_NAME=? AND COLUMN_NAME=?", h.name, o.table, o.column).Scan(&n) != nil || n != boolInt(!noAlter) {
					t.Fatal("actual object state differs after failure")
				}
			}
		})
	}
}
func TestM4SQLObjectNativePostStartNewFactsAndNonNullRefused(t *testing.T) {
	for _, kind := range []string{"FirstRuntimeCommandMarker", "RetainedFactChanged", "NewPersistentFact", "NonNullCapture"} {
		t.Run(kind, func(t *testing.T) {
			h := newNative(t)
			if kind == "NonNullCapture" {
				schema := map[string]any{"version": 1, "entries": []any{map[string]any{"event_id": "owned-event", "event_type": "evaluation.requested", "source": map[string]any{"database": "mysql", "object": "domain_event_outbox", "primary_key_kind": "mysql_uint64", "primary_key_sha256": strings.Repeat("a", 64), "digest": map[string]any{"kind": "owned", "sha256": strings.Repeat("b", 64)}}, "proof": map[string]any{"version": 1, "class": "retired_verified", "event_id": "owned-event", "digest": map[string]any{"kind": "owned", "sha256": strings.Repeat("b", 64)}, "business_binding_sha256": strings.Repeat("c", 64), "origin": "retirement", "verification": map[string]any{"method": "owned", "version": "1", "operation_id": "123-1", "verified_at": "2026-10-15T00:00:00Z", "business_terminal": true, "ownership_verified": true, "responsibility_closed": true}}}}}
				raw, _ := json.Marshal(schema)
				if _, e := h.db.Exec("UPDATE assessment SET historical_lifecycle_evidence=? WHERE id=4101", raw); e != nil {
					t.Fatal("actual original CHECK rejected valid synthetic evidence", e)
				}
				r := h.action("capture", "--capture-file="+filepath.Join(h.dir, "rejected.json"))
				if r.Complete || r.Error != "non_null_or_retained_row_rejected" {
					t.Fatal(r)
				}
				return
			}

			h.capture()
			h.upgrade(1, false)
			args, marker := h.restoreInput()
			if kind == "FirstRuntimeCommandMarker" {
				if e := writeExclusive(marker, []byte("issued")); e != nil {
					t.Fatal(e)
				}
			} else if kind == "NewPersistentFact" {
				if _, e := h.db.Exec("INSERT INTO assessment(id,org_id,testee_id,questionnaire_code,questionnaire_version,answer_sheet_id,origin_type,status,evaluation_model_kind,evaluation_model_code,evaluation_model_version,primary_score_value) VALUES(4103,1,2,'owned-new','1',4103,'direct','evaluated','scale','owned-new','1',11.5)"); e != nil {
					t.Fatal(e)
				}
			} else {
				if _, e := h.db.Exec("UPDATE assessment SET primary_score_value=98 WHERE id=4101"); e != nil {
					t.Fatal(e)
				}
			}
			r := h.action("restore", args...)
			if r.Complete {
				t.Fatal(r)
			}
			h.assertHead(101, true)
			if _, e := os.Stat(marker); kind == "FirstRuntimeCommandMarker" && e != nil {
				t.Fatal("runtime marker removed")
			}
		})
	}
}

type commitLossConn struct {
	net.Conn
	mu          sync.Mutex
	armed, drop bool
	observed    *bool
}

func (c *commitLossConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	if len(p) > 5 && p[4] == 3 {
		s := strings.ToUpper(strings.TrimSpace(string(p[5:])))
		if strings.HasPrefix(s, "DELETE FROM SCHEMA_MIGRATIONS") {
			c.armed = true
		}
		if c.armed && s == "COMMIT" {
			c.drop = true
		}
	}
	c.mu.Unlock()
	return c.Conn.Write(p)
}
func (c *commitLossConn) Read(p []byte) (int, error) {
	c.mu.Lock()
	drop := c.drop
	c.drop = false
	c.mu.Unlock()
	if !drop {
		return c.Conn.Read(p)
	}
	var header [4]byte
	if _, e := io.ReadFull(c.Conn, header[:]); e != nil {
		return 0, e
	}
	size := int(header[0]) | int(header[1])<<8 | int(header[2])<<16
	if size < 1 || size > 65536 {
		return 0, errors.New("owned response size rejected")
	}
	raw := make([]byte, size)
	if _, e := io.ReadFull(c.Conn, raw); e != nil {
		return 0, e
	}
	if raw[0] == 0 {
		*c.observed = true
	}
	return 0, io.EOF
}
func TestM4SQLObjectNativeCommitResponseLossRemainsUnknown(t *testing.T) {
	h := newNative(t)
	h.capture()
	h.upgrade(2, false)
	args, _ := h.restoreInput()
	directDSN := h.dsn
	cfg, _ := mysql.ParseDSN(h.dsn)
	actualAck := false
	network := h.name
	mysql.RegisterDialContext(network, func(ctx context.Context, address string) (net.Conn, error) {
		c, e := (&net.Dialer{}).DialContext(ctx, "tcp", address)
		if e != nil {
			return nil, e
		}
		return &commitLossConn{Conn: c, observed: &actualAck}, nil
	})
	defer mysql.DeregisterDialContext(network)
	cfg.Net = network
	h.dsn = cfg.FormatDSN()
	r := h.action("restore", args...)
	if r.Complete || r.Error != "ledger_commit_outcome_unknown" || !actualAck {
		t.Fatal("actual committed reply loss was not safely refused", r, actualAck)
	}
	h.assertHead(100, false)
	// This is a new normal TCP client, not the connection that lost the ACK.
	h.dsn = directDSN
	stateBefore := ownedRecoveryStateBytes(t, h.dir)
	verified := h.action("verify-restored", args...)
	if !verified.Complete || !verified.ReadOnly || !verified.SQLConnectionClosed || !verified.SQLPoolClosed || !verified.MongoClosed || verified.ReleaseError != "" || verified.TransactionReleaseUnproven {
		t.Fatal("fresh read-only original-state verification failed", verified)
	}
	if !reflect.DeepEqual(stateBefore, ownedRecoveryStateBytes(t, h.dir)) {
		t.Fatal("read-only verification changed retained recovery files")
	}
	// A real retained fact change must make the same full verification fail.
	if _, e := h.db.Exec("UPDATE assessment SET primary_score_value=primary_score_value+1 WHERE id=4101"); e != nil {
		t.Fatal("owned retained-fact change unavailable", e)
	}
	drifted := h.action("verify-restored", args...)
	if drifted.Complete || !drifted.ReadOnly || drifted.Error != "retained_structure_or_fact_changed" || !drifted.SQLConnectionClosed || !drifted.SQLPoolClosed || !drifted.MongoClosed || drifted.ReleaseError != "" || drifted.TransactionReleaseUnproven {
		t.Fatal("fresh verification accepted drift or failed to release", drifted)
	}
	h.assertHead(100, false)
	if !reflect.DeepEqual(stateBefore, ownedRecoveryStateBytes(t, h.dir)) || r.Complete || r.Error != "ledger_commit_outcome_unknown" {
		t.Fatal("verification changed the original unknown receipt or recovery state")
	}
	t.Log("actual MySQL COMMIT OK was dropped; independent fresh full verification proved original state, retained drift then refused; no replay/new claim and unknown receipt unchanged")
}

func ownedRecoveryStateBytes(t *testing.T, directory string) map[string]string {
	t.Helper()
	entries, e := os.ReadDir(directory)
	if e != nil {
		t.Fatal("owned recovery directory unavailable")
	}
	out := map[string]string{}
	for _, entry := range entries {
		if entry.IsDir() {
			t.Fatal("unexpected directory in owned recovery state")
		}
		raw, e := os.ReadFile(filepath.Join(directory, entry.Name()))
		if e != nil {
			t.Fatal("owned recovery state unavailable")
		}
		out[entry.Name()] = fingerprint(raw)
	}
	return out
}

func TestM4SQLObjectNativeUnsignedBoundaryAndMandatoryEOF(t *testing.T) {
	h := newNative(t)
	if _, e := h.db.Exec("SET SESSION sql_mode=CONCAT(@@session.sql_mode,',NO_AUTO_VALUE_ON_ZERO')"); e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{"0", "18446744073709551615"} {
		if _, e := h.db.Exec("INSERT INTO assessment(id,org_id,testee_id,questionnaire_code,questionnaire_version,answer_sheet_id,origin_type,status,evaluation_model_kind,evaluation_model_code,evaluation_model_version,primary_score_value) VALUES(CAST(? AS DECIMAL(20,0)),1,2,'owned-boundary','1',CAST(? AS DECIMAL(20,0)),'direct','evaluated','scale','owned-boundary','1',11.5)", id, id); e != nil {
			t.Fatal(e)
		}
	}
	p := filepath.Join(h.dir, "first-boundary-capture.json")
	r := h.action("capture", "--capture-file="+p)
	if !r.Complete {
		t.Fatal(r)
	}
	raw, e := readPrivate(p, r.MaterialHash, maxMaterialBytes)
	if e != nil {
		t.Fatal(e)
	}
	var m material
	if strictDecode(raw, &m) != nil {
		t.Fatal("boundary material invalid")
	}
	rows := m.Tables[0].Rows
	if len(rows) != 4 || rows[0].ID != 0 || rows[3].ID != ^uint64(0) || !m.Tables[0].EOF || m.Tables[0].Pages != 2 {
		t.Fatal("highest unsigned key omitted or no independent empty EOF", m.Tables[0].Pages, len(rows))
	}
}

func TestM4SQLObjectNativeRestoreDDLDeadlines(t *testing.T) {
	for _, name := range []string{"pending_over_five_seconds_then_complete", "earlier_approval_deadline", "earlier_approval_expiry"} {
		t.Run(name, func(t *testing.T) {
			h := newNativeOriginalCharset(t, "utf8")
			h.capture()
			original := h.fullDDL()
			h.upgrade(2, true)
			for _, query := range []string{
				"UPDATE performance_schema.setup_instruments SET ENABLED='YES',TIMED='YES' WHERE NAME IN ('wait/lock/metadata/sql/mdl','statement/sql/alter_table')",
				"UPDATE performance_schema.setup_consumers SET ENABLED='YES' WHERE NAME IN ('global_instrumentation','thread_instrumentation','events_statements_current')",
			} {
				if _, e := h.root.ExecContext(t.Context(), query); e != nil {
					t.Fatal("owned metadata-lock instrumentation unavailable")
				}
			}
			blockerContext, stopBlocker := context.WithTimeout(t.Context(), 45*time.Second)
			defer stopBlocker()
			blocker, e := h.db.BeginTx(blockerContext, &sql.TxOptions{ReadOnly: true})
			if e != nil {
				t.Fatal("owned metadata-lock blocker begin failed")
			}
			var count int
			if blocker.QueryRowContext(blockerContext, "SELECT COUNT(*) FROM assessment").Scan(&count) != nil || count != 2 {
				_ = blocker.Rollback()
				t.Fatal("owned metadata-lock blocker read failed")
			}
			released := false
			release := func() {
				if !released {
					if e := blocker.Rollback(); e != nil {
						t.Error("owned metadata-lock blocker release failed")
					}
					released = true
				}
			}
			now := time.Now().UTC()
			a := testApproval(t, h.material, h.materialHash)
			a.ToolSource = h.tool
			a.Operation = fmt.Sprintf("%d-1", now.UnixNano())
			a.Deadline, a.Expires = now.Add(30*time.Second).Format(time.RFC3339Nano), now.Add(30*time.Second).Format(time.RFC3339Nano)
			switch name {
			case "earlier_approval_deadline":
				a.Deadline = now.Add(4 * time.Second).Format(time.RFC3339Nano)
			case "earlier_approval_expiry":
				a.Expires = now.Add(4 * time.Second).Format(time.RFC3339Nano)
			}
			if e := validateApproval(a, h.material, h.materialHash, h.tool, time.Now()); e != nil {
				release()
				t.Fatal("owned deadline approval does not bind this actual compiled tool", e.Error())
			}
			raw, _ := json.Marshal(a)
			approvalPath := filepath.Join(h.dir, "ddl-deadline-approval.json")
			if writeExclusive(approvalPath, raw) != nil {
				release()
				t.Fatal("owned deadline approval could not be frozen")
			}
			action, cancelAction := context.WithCancel(t.Context())
			response := make(chan receipt, 1)
			readResponse := false
			defer func() {
				release()
				cancelAction()
				if !readResponse {
					select {
					case <-response:
					case <-time.After(10 * time.Second):
						t.Error("owned restore client did not finish during cleanup")
					}
				}
				if !waitOwnedRestoreDDL(t, h, false, 10*time.Second) {
					t.Error("owned ALTER termination remained unknown after releasing the blocker")
				}
			}()
			go func() {
				response <- run(action, []string{"--action=restore", "--mysql-database=" + h.name, "--mongo-database=" + h.name, "--timeout=30s", "--material-file=" + h.materialPath, "--material-sha256=" + h.materialHash, "--approval-file=" + approvalPath, "--approval-sha256=" + fingerprint(raw)}, func(key string) string {
					switch key {
					case "QS_M4_OBJECT_MYSQL_DSN":
						return h.dsn
					case "QS_M4_OBJECT_MONGO_URI":
						return h.uri
					}
					return ""
				}, h.tool, nil)
			}()
			pending, early := probeOwnedRestoreDDL(t, h, true, 3*time.Second, response)
			if early != nil {
				readResponse = true
				t.Fatalf("restore returned before an actual MDL wait: category=%s complete=%t connection_closed=%t pool_closed=%t mongo_closed=%t", early.Error, early.Complete, early.SQLConnectionClosed, early.SQLPoolClosed, early.MongoClosed)
			}
			if !pending {
				logOwnedRestoreDDLProbe(t, h)
				t.Fatal("independent probe did not observe this ALTER actually pending on MDL")
			}
			pendingAt := time.Now()
			var r receipt
			if name == "pending_over_five_seconds_then_complete" {
				timer := time.NewTimer(6 * time.Second)
				defer timer.Stop()
				select {
				case r = <-response:
					readResponse = true
					t.Fatal("restore returned before the observed six-second MDL wait was released", r.Error)
				case <-timer.C:
				}
				release()
				select {
				case r = <-response:
					readResponse = true
				case <-time.After(10 * time.Second):
					t.Fatal("restore did not finish after releasing its observed MDL wait")
				}
				if !r.Complete || !r.SQLConnectionClosed || !r.SQLPoolClosed || !r.MongoClosed || r.TransactionReleaseUnproven || h.fullDDL() != original {
					t.Fatal("long pending ALTER did not preserve complete original objects and release", r.Error)
				}
				h.assertHead(100, false)
				if !h.action("inspect").Complete {
					t.Fatal("fresh10039 after the long ALTER was not accepted")
				}
			} else {
				select {
				case r = <-response:
					readResponse = true
				case <-time.After(6 * time.Second):
					t.Fatal("restore ignored the independently approved earlier context bound")
				}
				if r.Complete || !r.BudgetExceeded || r.Error != "object_ddl_restore_failed_or_unknown" {
					t.Fatal("approved deadline did not stop this actually pending ALTER", r.Error)
				}
				var version uint64
				var dirty bool
				q, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				e = h.root.QueryRowContext(q, "SELECT version,dirty FROM `"+h.name+"`.schema_migrations").Scan(&version, &dirty)
				cancel()
				if e != nil || version != 101 || dirty {
					t.Fatal("cancelled ALTER replayed the original ledger before the objects were proved")
				}
				release()
				if !waitOwnedRestoreDDL(t, h, false, 10*time.Second) {
					t.Fatal("actual statement termination unknown after releasing the owned lock")
				}
				h.assertHead(101, false)
				var actualColumns int
				if h.root.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=? AND ((TABLE_NAME='assessment' AND COLUMN_NAME='historical_lifecycle_evidence') OR (TABLE_NAME='evaluation_outcome' AND COLUMN_NAME='historical_committed_evidence'))", h.name).Scan(&actualColumns) != nil {
					t.Fatal("independent post-failure object observation unavailable")
				}
				t.Logf("post-timeout actual historical-column count=%d; no rollback inferred; ledger remains101clean", actualColumns)
			}
			t.Logf("actual MDL pending observed; elapsed_after_pending=%.3fs complete=%t category=%s connection_closed=%t pool_closed=%t mongo_closed=%t", time.Since(pendingAt).Seconds(), r.Complete, r.Error, r.SQLConnectionClosed, r.SQLPoolClosed, r.MongoClosed)
		})
	}
}

func waitOwnedRestoreDDL(t *testing.T, h *nativeFixture, pending bool, limit time.Duration) bool {
	t.Helper()
	observed, _ := probeOwnedRestoreDDL(t, h, pending, limit, nil)
	return observed
}

func probeOwnedRestoreDDL(t *testing.T, h *nativeFixture, pending bool, limit time.Duration, response <-chan receipt) (bool, *receipt) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		select {
		case r := <-response:
			return false, &r
		default:
		}
		q, cancel := context.WithTimeout(t.Context(), time.Second)
		query := "SELECT COUNT(*) FROM performance_schema.events_statements_current s JOIN performance_schema.threads p ON p.THREAD_ID=s.THREAD_ID WHERE p.PROCESSLIST_DB=? AND s.EVENT_NAME='statement/sql/alter_table' AND s.END_EVENT_ID IS NULL"
		if pending {
			query += " AND EXISTS(SELECT 1 FROM performance_schema.metadata_locks m WHERE m.OWNER_THREAD_ID=s.THREAD_ID AND m.OBJECT_SCHEMA=? AND m.OBJECT_NAME='assessment' AND m.LOCK_STATUS='PENDING')"
		}
		args := []any{h.name}
		if pending {
			args = append(args, h.name)
		}
		var n int
		e := h.root.QueryRowContext(q, query, args...).Scan(&n)
		cancel()
		if e != nil {
			return false, nil
		}
		if (pending && n > 0) || (!pending && n == 0) {
			return true, nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false, nil
}

func logOwnedRestoreDDLProbe(t *testing.T, h *nativeFixture) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	rows, e := h.root.QueryContext(ctx, "SELECT COALESCE(s.EVENT_NAME,'none'),s.END_EVENT_ID IS NULL,p.PROCESSLIST_DB=?,COALESCE(m.LOCK_TYPE,'none'),COALESCE(m.LOCK_STATUS,'none') FROM performance_schema.threads p LEFT JOIN performance_schema.events_statements_current s ON s.THREAD_ID=p.THREAD_ID LEFT JOIN performance_schema.metadata_locks m ON m.OWNER_THREAD_ID=p.THREAD_ID AND m.OBJECT_SCHEMA=? AND m.OBJECT_NAME='assessment' WHERE p.PROCESSLIST_DB=? LIMIT 65", h.name, h.name, h.name)
	if e != nil {
		t.Log("owned DDL probe diagnostics unavailable")
		return
	}
	defer func() {
		if rows.Close() != nil {
			t.Error("owned DDL probe diagnostic rows release unproved")
		}
	}()
	for n := 0; rows.Next(); n++ {
		var event, lockType, lockStatus string
		var unfinished, selectedDatabase bool
		if n >= 64 || rows.Scan(&event, &unfinished, &selectedDatabase, &lockType, &lockStatus) != nil || len(event) > 128 || len(lockType) > 64 || len(lockStatus) > 64 {
			t.Log("owned DDL probe diagnostics incomplete")
			return
		}
		t.Logf("owned DDL probe event_name=%s unfinished=%t selected_database=%t lock_type=%s lock_status=%s", event, unfinished, selectedDatabase, lockType, lockStatus)
	}
	if rows.Err() != nil {
		t.Log("owned DDL probe diagnostics incomplete")
	}
}

func TestM4SQLObjectNativeRestoreContinuation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		clean bool
		acks  int
	}{
		{"dirty_after_first_add_ack", false, 1},
		{"dirty_after_second_add_ack", false, 2},
		{"clean_after_first_add_ack", true, 1},
		{"clean_after_second_add_ack", true, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newNativeOriginalCharset(t, "utf8")
			h.capture()
			original := h.fullDDL()
			h.upgrade(2, tc.clean)
			ledgerBefore := readOwnedLedger(t, h)
			args, marker := h.restoreInput()
			action, cancel := context.WithCancel(t.Context())
			defer cancel()
			acks := []int{}
			r := run(action, append([]string{"--action=restore", "--mysql-database=" + h.name, "--mongo-database=" + h.name, "--timeout=1m"}, args...), func(key string) string {
				switch key {
				case "QS_M4_OBJECT_MYSQL_DSN":
					return h.dsn
				case "QS_M4_OBJECT_MONGO_URI":
					return h.uri
				}
				return ""
			}, h.tool, func(i int) {
				acks = append(acks, i)
				if len(acks) == tc.acks {
					cancel()
				}
			})
			if r.Complete || !r.SQLConnectionClosed || !r.SQLPoolClosed || !r.MongoClosed || r.TransactionReleaseUnproven || len(acks) != tc.acks || acks[0] != 1 || (tc.acks == 2 && acks[1] != 0) {
				t.Fatal("explicit interruption did not follow successful reverse ADD acknowledgments", r.Error, acks)
			}
			if !reflect.DeepEqual(readOwnedLedger(t, h), ledgerBefore) {
				t.Fatal("101 ledger bytes or DDL changed before full object restoration")
			}
			h.assertHead(101, !tc.clean)
			actual := h.fullDDL()
			for i := range objects {
				present := strings.Contains(actual[i], "`"+objects[i].column+"` json DEFAULT NULL")
				wantPresent := i == 1 || tc.acks == 2
				if present != wantPresent || (present && actual[i] != original[i]) {
					t.Fatal("acknowledged ADD did not leave an exact admitted DROP prefix", i, present)
				}
			}
			if tc.clean {
				verified := h.action("verify-target", "--material-file="+h.materialPath, "--material-sha256="+h.materialHash)
				if verified.Complete || verified.Error != "target_objects_still_present" {
					t.Fatal("restore admission leaked into verify-target", verified.Error)
				}
			}
			old := h.action("restore", args...)
			if old.Complete || old.Error != "restore_approval_consumption_failed" || !reflect.DeepEqual(readOwnedLedger(t, h), ledgerBefore) || h.fullDDL() != actual {
				t.Fatal("consumed approval reused or changed restored facts", old.Error)
			}
			next := nextOwnedRestoreApproval(t, h, args, marker)
			resumed := h.action("restore", next...)
			if !resumed.Complete || !resumed.SQLConnectionClosed || !resumed.SQLPoolClosed || !resumed.MongoClosed || resumed.TransactionReleaseUnproven || h.fullDDL() != original {
				t.Fatal("new explicit approval could not finish the admitted prefix", resumed.Error)
			}
			h.assertHead(100, false)
			if !h.action("inspect").Complete {
				t.Fatal("fresh10039 rejected after explicit continuation")
			}
			assertOwnedRetainedCapture(t, h)
			t.Logf("actual successful ADD acknowledgments=%d; original101 ledger remained unchanged; new approval restored original DDL/facts/ledger and released clients", tc.acks)
		})
	}
}

func nextOwnedRestoreApproval(t *testing.T, h *nativeFixture, original []string, marker string) []string {
	t.Helper()
	var path, hash string
	for _, arg := range original {
		if strings.HasPrefix(arg, "--approval-file=") {
			path = strings.TrimPrefix(arg, "--approval-file=")
		}
		if strings.HasPrefix(arg, "--approval-sha256=") {
			hash = strings.TrimPrefix(arg, "--approval-sha256=")
		}
	}
	raw, e := readPrivate(path, hash, 32768)
	var first approval
	if e != nil || strictDecode(raw, &first) != nil || first.Marker != marker {
		t.Fatal("first independently frozen approval unavailable for explicit continuation")
	}
	next := first
	next.Operation = fmt.Sprintf("%d-2", time.Now().UnixNano())
	if next.Operation == first.Operation {
		t.Fatal("continuation operation was not independent")
	}
	// Keep the original freeze, material, marker, decision and absolute expiry/
	// deadline unchanged. A new attempt cannot reset its recovery budget.
	raw, e = json.Marshal(next)
	path = filepath.Join(h.dir, next.Operation+"-approval.json")
	if e != nil || writeExclusive(path, raw) != nil {
		t.Fatal("new explicit continuation approval could not be frozen")
	}
	args := append([]string(nil), original...)
	for i, arg := range args {
		if strings.HasPrefix(arg, "--approval-file=") {
			args[i] = "--approval-file=" + path
		}
		if strings.HasPrefix(arg, "--approval-sha256=") {
			args[i] = "--approval-sha256=" + fingerprint(raw)
		}
	}
	return args
}

type ownedLedger struct {
	DDL  string
	Rows []ledgerRow
}

func readOwnedLedger(t *testing.T, h *nativeFixture) ownedLedger {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), queryLimit)
	defer cancel()
	text, e := ddl(ctx, h.db, "schema_migrations")
	if e != nil {
		t.Fatal("actual ledger DDL unavailable")
	}
	out := ownedLedger{DDL: text}
	rows, e := h.db.QueryContext(ctx, "SELECT version,dirty FROM schema_migrations ORDER BY version LIMIT 2")
	if e != nil {
		t.Fatal("actual ledger rows unavailable")
	}
	for rows.Next() {
		var row ledgerRow
		if rows.Scan(&row.Version, &row.Dirty) != nil {
			_ = rows.Close()
			t.Fatal("actual ledger row invalid")
		}
		out.Rows = append(out.Rows, row)
	}
	re, ce := rows.Err(), rows.Close()
	if re != nil || ce != nil || len(out.Rows) != 1 {
		t.Fatal("complete actual ledger or row release unproved")
	}
	return out
}

func assertOwnedRetainedCapture(t *testing.T, h *nativeFixture) {
	t.Helper()
	p := filepath.Join(h.dir, "independent-final-capture.json")
	r := h.action("capture", "--capture-file="+p)
	if !r.Complete {
		t.Fatal("fresh independent complete capture failed", r.Error)
	}
	raw, e := readPrivate(p, r.MaterialHash, maxMaterialBytes)
	var final material
	if e != nil || strictDecode(raw, &final) != nil || compareRetained(h.material, final) != nil || !reflect.DeepEqual(h.material.Ledger, final.Ledger) || h.material.LedgerDDL != final.LedgerDDL {
		t.Fatal("fresh capture did not preserve all original facts and full ledger")
	}
}

func TestM4SQLObjectNativeCompleteCountBudgets(t *testing.T) {
	for _, name := range []string{"pending_over_five_seconds_then_complete", "earlier_action_deadline", "actual_count_mismatch"} {
		t.Run(name, func(t *testing.T) {
			h := newNativeOriginalCharset(t, "utf8")
			h.capture()
			original := h.fullDDL()
			for _, query := range []string{
				"UPDATE performance_schema.setup_instruments SET ENABLED='YES',TIMED='YES' WHERE NAME IN ('wait/lock/metadata/sql/mdl','statement/sql/select')",
				"UPDATE performance_schema.setup_consumers SET ENABLED='YES' WHERE NAME IN ('global_instrumentation','thread_instrumentation','events_statements_current')",
			} {
				if _, e := h.root.ExecContext(t.Context(), query); e != nil {
					t.Fatal("owned COUNT instrumentation unavailable")
				}
			}
			limit := 30 * time.Second
			if name == "earlier_action_deadline" {
				limit = 4 * time.Second
			}
			action, cancelAction := context.WithTimeout(t.Context(), limit)
			defer cancelAction()
			cfg, e := mysql.ParseDSN(h.dsn)
			if e != nil || !configureMySQLAction(action, cfg) {
				t.Fatal("actual action connection configuration failed")
			}
			pool, e := sql.Open("mysql", cfg.FormatDSN())
			if e != nil {
				t.Fatal("owned COUNT pool unavailable")
			}
			pool.SetMaxOpenConns(1)
			pool.SetMaxIdleConns(1)
			defer func() {
				if !closeSQLPool(pool) {
					t.Error("owned COUNT pool release unproved")
				}
			}()
			connect, cancelConnect := context.WithTimeout(action, queryLimit)
			conn, e := pool.Conn(connect)
			cancelConnect()
			if e != nil {
				t.Fatal("owned COUNT connection unavailable")
			}
			defer func() {
				if e := conn.Close(); e != nil && !errors.Is(e, sql.ErrConnDone) {
					t.Error("owned COUNT connection release unproved")
				}
			}()
			tx, e := conn.BeginTx(action, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
			if e != nil {
				t.Fatal("owned COUNT RR snapshot unavailable")
			}
			defer func() {
				if e := tx.Rollback(); e != nil && !errors.Is(e, sql.ErrTxDone) {
					t.Error("owned COUNT RR release unproved")
				}
			}()
			q, cancel := context.WithTimeout(action, queryLimit)
			var head uint64
			e = tx.QueryRowContext(q, "SELECT version FROM schema_migrations").Scan(&head)
			cancel()
			if e != nil || head != 100 {
				t.Fatal("owned COUNT snapshot did not begin on the actual clean100 ledger")
			}
			if name == "actual_count_mismatch" {
				if e := verifyCompleteRangeCount(action, tx, 0, 3); e == nil || e.Error() != "complete_range_count_mismatch" {
					t.Fatal("actual two-row COUNT did not reject expected three", e)
				}
			} else {
				runOwnedBlockedCount(t, h, action, tx, name == "pending_over_five_seconds_then_complete")
			}
			if e := tx.Rollback(); e != nil && !errors.Is(e, sql.ErrTxDone) {
				t.Fatal("COUNT snapshot release failed")
			}
			h.assertHead(100, false)
			if h.fullDDL() != original {
				t.Fatal("read-only COUNT changed original objects")
			}
			assertOwnedRetainedCapture(t, h)
		})
	}
}

func runOwnedBlockedCount(t *testing.T, h *nativeFixture, action context.Context, tx *sql.Tx, complete bool) {
	t.Helper()
	blockerCtx, cancelBlocker := context.WithTimeout(t.Context(), 35*time.Second)
	defer cancelBlocker()
	blocker, e := h.db.Conn(blockerCtx)
	if e != nil {
		t.Fatal("owned COUNT blocker connection unavailable")
	}
	released := false
	release := func() {
		if released {
			return
		}
		q, cancel := context.WithTimeout(t.Context(), queryLimit)
		_, unlockErr := blocker.ExecContext(q, "UNLOCK TABLES")
		cancel()
		closeErr := blocker.Close()
		released = true
		if unlockErr != nil || closeErr != nil {
			t.Error("owned COUNT blocker release unproved")
		}
	}
	defer release()
	q, cancel := context.WithTimeout(blockerCtx, queryLimit)
	_, e = blocker.ExecContext(q, "LOCK TABLES assessment WRITE")
	cancel()
	if e != nil {
		t.Fatal("owned COUNT MDL blocker not installed")
	}
	response := make(chan error, 1)
	go func() { response <- verifyCompleteRangeCount(action, tx, 0, 2) }()
	readResponse := false
	defer func() {
		release()
		if !readResponse {
			select {
			case <-response:
			case <-time.After(10 * time.Second):
				t.Error("owned COUNT client termination unproved")
			}
		}
		if !waitOwnedCountStatement(t, h, false, 10*time.Second) {
			t.Error("actual COUNT termination remained unknown after unlocking")
		}
	}()
	deadline := time.Now().Add(3 * time.Second)
	for !waitOwnedCountStatement(t, h, true, 50*time.Millisecond) {
		select {
		case e := <-response:
			readResponse = true
			t.Fatal("COUNT returned before independent MDL evidence", e)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("independent probe did not observe the actual COUNT pending on MDL")
		}
	}
	pendingAt := time.Now()
	if complete {
		timer := time.NewTimer(6 * time.Second)
		defer timer.Stop()
		select {
		case e := <-response:
			readResponse = true
			t.Fatal("COUNT did not survive the observed six-second MDL wait", e)
		case <-timer.C:
		}
		release()
	}
	select {
	case e = <-response:
		readResponse = true
	case <-time.After(8 * time.Second):
		t.Fatal("COUNT did not obey its bounded action after the observed MDL wait")
	}
	if complete {
		if e != nil {
			t.Fatal("bounded COUNT did not complete after unlocking", e)
		}
	} else if e == nil || (e.Error() != "complete_range_count_deadline_exceeded" && e.Error() != "complete_range_count_server_timeout") {
		t.Fatal("earlier action deadline was not a classified COUNT failure", e)
	}
	release()
	h.assertHead(100, false)
	if !waitOwnedCountStatement(t, h, false, 10*time.Second) {
		t.Fatal("COUNT still active after releasing the owned MDL blocker")
	}
	t.Logf("actual COUNT MDL pending observed; elapsed_after_pending=%.3fs complete=%t; no rollback inferred", time.Since(pendingAt).Seconds(), complete)
}

func waitOwnedCountStatement(t *testing.T, h *nativeFixture, pending bool, limit time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		q, cancel := context.WithTimeout(t.Context(), time.Second)
		query := "SELECT COUNT(*) FROM performance_schema.events_statements_current s JOIN performance_schema.threads p ON p.THREAD_ID=s.THREAD_ID WHERE p.PROCESSLIST_DB=? AND s.EVENT_NAME='statement/sql/select' AND s.END_EVENT_ID IS NULL AND LOCATE('COUNT(*) FROM `assessment`',s.SQL_TEXT)>0"
		args := []any{h.name}
		if pending {
			query += " AND EXISTS(SELECT 1 FROM performance_schema.metadata_locks m WHERE m.OWNER_THREAD_ID=s.THREAD_ID AND m.OBJECT_SCHEMA=? AND m.OBJECT_NAME='assessment' AND m.LOCK_STATUS='PENDING')"
			args = append(args, h.name)
		}
		var n int
		e := h.root.QueryRowContext(q, query, args...).Scan(&n)
		cancel()
		if e != nil {
			return false
		}
		if (pending && n == 1) || (!pending && n == 0) {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func assertOwnedRecoveryUnclaimed(t *testing.T, h *nativeFixture, args []string) {
	t.Helper()
	var path, hash string
	for _, arg := range args {
		if strings.HasPrefix(arg, "--approval-file=") {
			path = strings.TrimPrefix(arg, "--approval-file=")
		}
		if strings.HasPrefix(arg, "--approval-sha256=") {
			hash = strings.TrimPrefix(arg, "--approval-sha256=")
		}
	}
	raw, e := readPrivate(path, hash, 32768)
	var approved approval
	if e != nil || strictDecode(raw, &approved) != nil {
		t.Fatal("actual frozen approval unavailable for claim observation")
	}
	boundary, claim, e := recoveryPaths(h.material, h.materialHash, approved.Operation)
	if e != nil {
		t.Fatal("actual fixed recovery paths unavailable")
	}
	for _, ownedPath := range []string{boundary, claim} {
		if _, e := os.Stat(ownedPath); !os.IsNotExist(e) {
			t.Fatal("early refusal persisted original boundary or operation claim")
		}
	}
}
