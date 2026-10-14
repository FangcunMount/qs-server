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
	t.Helper()
	dsn, uri, tool := requireNative(t)
	cfg, _ := mysql.ParseDSN(dsn)
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
	}, h.tool)
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
	name := h.name
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
	t.Log("actual MySQL COMMIT OK was dropped; tool reported unknown and did not force/down/retry")
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
