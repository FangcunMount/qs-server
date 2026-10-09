//go:build integration

package compatibilityretirementbackup

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	fence "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirementfence"
	identitymeta "github.com/FangcunMount/qs-server/internal/pkg/databaseidentity"
	"github.com/FangcunMount/qs-server/internal/pkg/migration"
	buildversion "github.com/FangcunMount/qs-server/pkg/version"
	mysql "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/sys/unix"
)

// These two opt-in parents execute a genuine archive/DDL/migration process and
// then wait for its exit before launching a separate recovery process. Approval
// data is synthetic local fixture policy. The public Linux root Window, actual
// SQL/Mongo responses, immutable files and new handles are real; no private
// Window/plan/proof is constructed. This does not prove production writer fencing,
// an A rollback image/config, unknown in-flight DDL, or a 600-second restore scale.
func TestTargetBCompleteCrossProcessNative(t *testing.T) { bCrossProcessNative(t, false) }
func TestTargetBSQLOnlyCrossProcessNative(t *testing.T)  { bCrossProcessNative(t, true) }

type bNativeHandoff struct {
	Name               string
	Directory          string
	ArchiveDirectory   string
	ArchiveSHA256      string
	WindowDirectory    string
	WindowBinding      fence.WindowBinding
	Request            TargetRecoveryRequest
	ApprovedBSourceSHA string
	SQLOnly            bool
	Resume             *TargetBRecoveryResumeRequest
}

type bNativeChildReceipt struct {
	PID                 int
	Stage               string
	SQLConnectionID     string
	WindowStartSHA256   string
	RecoverySHA256      string
	SQLHead             uint64
	MongoHead           uint64
	WriterFenceRequired bool
	ProductionAuthority bool
}

// This is a test-only transparent native Connector, not a migration result
// constructor. All queries, rows, values and Close errors come from MySQL. The
// one-shot trigger cancels the real host context only after the exact pinned
// provider's final clean100 Version row has closed successfully. This is after
// Migrator.cleanup and before Pair.Run can mark MongoAttempted. No journal field,
// head, clock, returned version or private migration capability is fabricated.
type bNativeSQLCancellation struct {
	armed  atomic.Bool
	fired  atomic.Bool
	cancel context.CancelFunc
}

type bNativeConnector struct {
	driver.Connector
	hook *bNativeSQLCancellation
}

func (c bNativeConnector) Connect(ctx context.Context) (driver.Conn, error) {
	actual, e := c.Connector.Connect(ctx)
	if e != nil {
		return nil, e
	}
	return &bNativeDriverConn{Conn: actual, hook: c.hook}, nil
}

type bNativeDriverConn struct {
	driver.Conn
	hook *bNativeSQLCancellation
}

func (c *bNativeDriverConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	actual, ok := c.Conn.(driver.QueryerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	rows, e := actual.QueryContext(ctx, query, args)
	if e != nil {
		return nil, e
	}
	if c.hook != nil && query == "SELECT version, dirty FROM `schema_migrations` LIMIT 1" {
		return &bNativeVersionRows{Rows: rows, hook: c.hook}, nil
	}
	return rows, nil
}
func (c *bNativeDriverConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	actual, ok := c.Conn.(driver.ExecerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	return actual.ExecContext(ctx, query, args)
}
func (c *bNativeDriverConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	actual, ok := c.Conn.(driver.ConnPrepareContext)
	if !ok {
		return nil, errors.New("native connector prepared context interface missing")
	}
	return actual.PrepareContext(ctx, query)
}
func (c *bNativeDriverConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	actual, ok := c.Conn.(driver.ConnBeginTx)
	if !ok {
		return nil, errors.New("native connector transaction interface missing")
	}
	return actual.BeginTx(ctx, opts)
}
func (c *bNativeDriverConn) Ping(ctx context.Context) error {
	actual, ok := c.Conn.(driver.Pinger)
	if !ok {
		return errors.New("native connector ping interface missing")
	}
	return actual.Ping(ctx)
}
func (c *bNativeDriverConn) ResetSession(ctx context.Context) error {
	actual, ok := c.Conn.(driver.SessionResetter)
	if !ok {
		return errors.New("native connector reset interface missing")
	}
	return actual.ResetSession(ctx)
}
func (c *bNativeDriverConn) IsValid() bool {
	actual, ok := c.Conn.(driver.Validator)
	return ok && actual.IsValid()
}
func (c *bNativeDriverConn) CheckNamedValue(v *driver.NamedValue) error {
	actual, ok := c.Conn.(driver.NamedValueChecker)
	if !ok {
		return driver.ErrSkip
	}
	return actual.CheckNamedValue(v)
}

type bNativeVersionRows struct {
	driver.Rows
	hook     *bNativeSQLCancellation
	clean100 bool
}

func (r *bNativeVersionRows) Next(values []driver.Value) error {
	e := r.Rows.Next(values)
	if e == nil && len(values) == 2 {
		version, vok := values[0].(int64)
		dirty, dok := values[1].(int64)
		r.clean100 = vok && dok && version == 100 && dirty == 0
	}
	return e
}
func (r *bNativeVersionRows) Close() error {
	e := r.Rows.Close()
	if e == nil && r.clean100 && r.hook.armed.Load() && r.hook.fired.CompareAndSwap(false, true) {
		r.hook.cancel()
	}
	return e
}

func bNativeSQL(t *testing.T, env map[string]string, name string, hook *bNativeSQLCancellation) *sql.DB {
	t.Helper()
	cfg := mysql.NewConfig()
	cfg.User, cfg.Passwd = env["MYSQL_USERNAME"], env["MYSQL_PASSWORD"]
	cfg.Net, cfg.Addr, cfg.DBName = "tcp", env["MYSQL_HOST"]+":"+env["MYSQL_PORT"], name
	// NewConnector consumes Config directly; DSN-only charset must not be sent
	// as a server variable through Params. Keep the real utf8mb4 connection.
	if e := cfg.Apply(mysql.Charset("utf8mb4", "")); e != nil {
		bNativePhaseFailure(t, "mysql_charset", e)
	}
	cfg.Timeout = 5 * time.Second
	// This native-only connection must execute the real embedded three-statement
	// SQL100 body. It does not split/reimplement the production migration.
	cfg.MultiStatements = true
	connector, e := mysql.NewConnector(cfg)
	if e != nil {
		bNativePhaseFailure(t, "mysql_connector", e)
	}
	db := sql.OpenDB(bNativeConnector{connector, hook})
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if db.Close() != nil {
			t.Error("native pool close failed")
		}
	})
	if e := db.PingContext(context.Background()); e != nil {
		bNativePhaseFailure(t, "mysql_ping", e)
	}
	return db
}

func bNativeRead[T any](t *testing.T, path string) T {
	t.Helper()
	f, e := openPrivateFile(path)
	if e != nil {
		t.Fatal("native handoff file rejected")
	}
	raw, re := readPrivate(f, metadataBudget)
	ce := f.Close()
	var v T
	if re != nil || ce != nil || exactJSON(raw, &v) != nil {
		t.Fatal("native handoff decode rejected")
	}
	return v
}
func bNativeHeads(t *testing.T, conn *sql.Conn, mdb *mongo.Database, sqlHead, mongoHead uint64) {
	t.Helper()
	var v uint64
	var dirty bool
	if conn.QueryRowContext(context.Background(), "SELECT version,dirty FROM schema_migrations").Scan(&v, &dirty) != nil || dirty || v != sqlHead {
		t.Fatal("actual SQL clean head mismatch")
	}
	var raw bson.Raw
	if mdb.Collection("schema_migrations").FindOne(context.Background(), bson.D{}).Decode(&raw) != nil {
		t.Fatal("actual Mongo clean head unknown")
	}
	mv := int64(0)
	if x, ok := raw.Lookup("version").Int64OK(); ok {
		mv = x
	} else if x, ok := raw.Lookup("version").Int32OK(); ok {
		mv = int64(x)
	} else {
		t.Fatal("actual Mongo version invalid")
	}
	md, ok := raw.Lookup("dirty").BooleanOK()
	if !ok || md || mv != int64(mongoHead) {
		t.Fatal("actual Mongo clean head mismatch")
	}
}
func bNativeMongoRows(t *testing.T, db *mongo.Database) []string {
	t.Helper()
	cur, e := db.Collection("domain_event_outbox").Find(context.Background(), bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetCollation(&options.Collation{Locale: "simple"}))
	if e != nil {
		t.Fatal("native BSON point read failed")
	}
	var values []string
	for cur.Next(context.Background()) {
		values = append(values, sha(cur.Current))
	}
	re, ce := cur.Err(), cur.Close(context.Background())
	if re != nil || ce != nil {
		t.Fatal("native BSON point close failed")
	}
	return values
}

// Failure-only metadata never carries an error string, query, URI or credential.
// The waiting parent reads it only after the real child has exited; it is not a
// migration result or permission to continue after an unsuccessful process.
type bNativeChildFailureReceipt struct {
	PID       int    `json:"pid"`
	Phase     string `json:"phase"`
	Stage     string `json:"stage"`
	Category  string `json:"category"`
	MySQLCode uint16 `json:"mysql_code"`
}

func bNativeFailureFieldsValid(r bNativeChildFailureReceipt) bool {
	switch r.Phase {
	case "migrate", "recover":
	default:
		return false
	}
	switch r.Stage {
	case "mysql_charset", "mysql_connector", "mysql_ping", "archive_open", "window_open", "sql_connection", "sql_connection_identity", "recovery_prepare", "targets_apply", "b_migration", "sql_only_result", "recovery_reconcile", "targets_restore":
	default:
		return false
	}
	switch r.Category {
	case "mysql_server_error":
		return r.PID > 0 && r.MySQLCode > 0
	case "context_cancelled", "context_deadline", "native_operation_failed", "native_predicate_rejected",
		string(ErrApproval), string(ErrStructure), string(ErrIdentity), string(ErrSource), string(ErrPrivate), string(ErrRead), string(ErrRestore), string(ErrContent), string(ErrIsolation), string(ErrBudget), string(ErrSerialization),
		string(ErrRecoveryAuthority), string(ErrRecoveryBinding), string(ErrRecoveryState), string(ErrRecoveryUnknown), string(ErrRecoveryHead), string(ErrRecoveryTransaction), string(ErrRecoveryJournal):
		return r.PID > 0 && r.MySQLCode == 0
	default:
		return false
	}
}

func bNativePhaseFailure(t *testing.T, stage string, e error) {
	t.Helper()
	r := bNativeChildFailureReceipt{PID: os.Getpid(), Phase: os.Getenv("QS_B_RECOVERY_CHILD_PHASE"), Stage: stage, Category: "native_operation_failed"}
	var server *mysql.MySQLError
	var known Error
	switch {
	case e == nil:
		r.Category = "native_predicate_rejected"
	case errors.As(e, &server) && server != nil && server.Number > 0:
		r.Category, r.MySQLCode = "mysql_server_error", server.Number
	case errors.Is(e, context.Canceled):
		r.Category = "context_cancelled"
	case errors.Is(e, context.DeadlineExceeded):
		r.Category = "context_deadline"
	case errors.As(e, &known):
		r.Category = string(known)
		if !bNativeFailureFieldsValid(r) {
			r.Category = "native_operation_failed"
		}
	}
	input := os.Getenv("QS_B_RECOVERY_CHILD_INPUT")
	written := false
	if bNativeFailureFieldsValid(r) && filepath.IsAbs(input) && filepath.Clean(input) == input && privateDirectory(filepath.Dir(input)) == nil {
		raw, marshalError := json.Marshal(r)
		written = marshalError == nil && writePrivate(filepath.Join(filepath.Dir(input), r.Phase+"-failure.private.json"), raw) == nil
	}
	t.Fatalf("native phase failed stage=%s category=%s mysql_code=%d diagnostic_written=%t", stage, r.Category, r.MySQLCode, written)
}

func bNativeReadFailure(input, phase string, pid int) (bNativeChildFailureReceipt, bool) {
	var r bNativeChildFailureReceipt
	if !filepath.IsAbs(input) || filepath.Clean(input) != input || privateDirectory(filepath.Dir(input)) != nil {
		return r, false
	}
	f, e := openPrivateFile(filepath.Join(filepath.Dir(input), phase+"-failure.private.json"))
	if e != nil {
		return r, false
	}
	raw, re := readPrivate(f, 4096)
	ce := f.Close()
	if re != nil || ce != nil || exactJSON(raw, &r) != nil || r.PID != pid || r.Phase != phase || !bNativeFailureFieldsValid(r) {
		return bNativeChildFailureReceipt{}, false
	}
	return r, true
}

// No child output is persisted or printed. Fixed counts/exit categories cannot
// expose credentials/body. The actual process group is killed on byte overflow
// or the parent's single deadline; Wait always reaps it before any next phase.
type bNativeOutputCounter struct {
	mu       sync.Mutex
	bytes    int64
	overflow bool
	kill     func()
}

func (w *bNativeOutputCounter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.bytes += int64(len(p))
	if w.bytes > 1<<20 {
		w.overflow = true
		if w.kill != nil {
			w.kill()
		}
		return 0, io.ErrShortWrite
	}
	return len(p), nil
}
func bNativeChild(t *testing.T, ctx context.Context, input, phase, parent string) bNativeChildReceipt {
	t.Helper()
	exe, e := os.Executable()
	if e != nil {
		t.Fatal("native executable identity unavailable")
	}
	cmd := exec.CommandContext(ctx, exe, "-test.run=^"+parent+"$", "-test.count=1", "-test.timeout=4m")
	cmd.Env = nativeChildEnv(map[string]string{"QS_B_RECOVERY_CHILD_INPUT": input, "QS_B_RECOVERY_CHILD_PHASE": phase})
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	counter := &bNativeOutputCounter{}
	kill := func() {
		if cmd.Process != nil {
			_ = unix.Kill(-cmd.Process.Pid, unix.SIGKILL)
		}
	}
	counter.kill = kill
	cmd.Stdout, cmd.Stderr = counter, counter
	cmd.Cancel = func() error { kill(); return nil }
	cmd.WaitDelay = 2 * time.Second
	if cmd.Start() != nil {
		t.Fatal("native child start failed")
	}
	pid := cmd.Process.Pid
	e = cmd.Wait()
	if e != nil || counter.overflow {
		exit := -1
		if cmd.ProcessState != nil {
			exit = cmd.ProcessState.ExitCode()
		}
		if failure, ok := bNativeReadFailure(input, phase, pid); ok {
			t.Fatalf("native child refused phase=%s exit=%d output_bytes=%d overflow=%t stage=%s category=%s mysql_code=%d", phase, exit, counter.bytes, counter.overflow, failure.Stage, failure.Category, failure.MySQLCode)
		}
		t.Fatalf("native child refused phase=%s exit=%d output_bytes=%d overflow=%t", phase, exit, counter.bytes, counter.overflow)
	}
	if cmd.ProcessState == nil || !cmd.ProcessState.Exited() || cmd.ProcessState.Pid() != pid {
		t.Fatal("native predecessor exit unproven")
	}
	if e = unix.Kill(pid, 0); e != unix.ESRCH {
		t.Fatal("native predecessor PID still present or reused")
	}
	out := bNativeRead[bNativeChildReceipt](t, filepath.Join(filepath.Dir(input), phase+".private.json"))
	if out.PID != pid || out.Stage != phase || out.ProductionAuthority || !out.WriterFenceRequired {
		t.Fatal("native child receipt overclaimed")
	}
	return out
}

func bCrossProcessNative(t *testing.T, sqlOnly bool) {
	if os.Getenv("QS_B_RECOVERY_NATIVE") != "1" {
		t.Skip("actual Linux root owned fixture not requested")
	}
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		t.Fatal("public maintenance window requires real Linux root")
	}
	wantSource := os.Getenv("QS_B_RECOVERY_NATIVE_SOURCE_SHA")
	if !sourcePattern.MatchString(wantSource) || buildversion.Get().GitCommit != wantSource {
		t.Fatal("native compiled source binding rejected")
	}
	parent := "TestTargetBCompleteCrossProcessNative"
	if sqlOnly {
		parent = "TestTargetBSQLOnlyCrossProcessNative"
	}
	if input := os.Getenv("QS_B_RECOVERY_CHILD_INPUT"); input != "" {
		bNativePhase(t, input, os.Getenv("QS_B_RECOVERY_CHILD_PHASE"), sqlOnly)
		return
	}
	// Reuse exactly the registered protected instances, no Docker creation or
	// shared topology/account/parameter changes. These helpers re-inspect on exit.
	nativeRoot(t, "owned-mysql.json", "34306")
	nativeRoot(t, "owned-mongo.json", "33317")
	env := nativeEnv(t)
	approvedRoot := os.Getenv("QS_B_RECOVERY_NATIVE_ROOT_DIRECTORY")
	if !filepath.IsAbs(approvedRoot) || filepath.Clean(approvedRoot) != approvedRoot {
		t.Fatal("native protected root not supplied")
	}
	dir, e := os.MkdirTemp(approvedRoot, "b-native-")
	if e != nil {
		t.Fatal("native owned private root creation failed")
	}
	t.Cleanup(func() {
		if t.Failed() {
			nativeRetainCleanup(t, dir)
			return
		}
		if _, e := os.Lstat(filepath.Join(dir, "cleanup-unresolved.private.json")); e == nil {
			t.Error("native cleanup unresolved; private material retained")
			return
		}
		if os.RemoveAll(dir) != nil {
			t.Error("native private namespace cleanup failed")
		}
		if _, e := os.Lstat(dir); !os.IsNotExist(e) {
			t.Error("native private namespace remaining")
		}
	})
	admin := nativeSQL(t, env, "")
	name := "qs_b_cross_recovery_" + primitive.NewObjectID().Hex()
	nativeExec(t, admin, "CREATE DATABASE "+quote(name)+" CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci")
	db := nativeSQL(t, env, name)
	client := nativeMongo(t, env)
	mdb := client.Database(name)
	t.Cleanup(func() {
		if _, e := admin.ExecContext(context.Background(), "DROP DATABASE "+quote(name)); e != nil {
			nativeRetainCleanup(t, dir)
			t.Error("native SQL cleanup failed")
		}
		if mdb.Drop(context.Background()) != nil {
			nativeRetainCleanup(t, dir)
			t.Error("native Mongo cleanup failed")
		}
		rows, e := readSQL(context.Background(), admin, "SELECT SCHEMA_NAME FROM information_schema.schemata WHERE SCHEMA_NAME=?", name)
		if e != nil || len(rows) != 0 {
			nativeRetainCleanup(t, dir)
			t.Error("native SQL namespace remains")
		}
		names, e := client.ListDatabaseNames(context.Background(), bson.D{{Key: "name", Value: name}})
		if e != nil || len(names) != 0 {
			nativeRetainCleanup(t, dir)
			t.Error("native Mongo namespace remains")
		}
	})
	// Original 99/38 fixture uses actual historical target DDL and small native
	// facts. It is not an assertion that every historical migration was run.
	nativeSourceSchemas(t, db, mdb)
	nativeTargetInstallOperations(t, db)
	nativeTargetInsertEvents(t, db)
	id := "11111111-1111-4111-8111-111111111111"
	nativeExec(t, db, "INSERT INTO ai_bridge_requests(request_id,request_hash,payload) VALUES(?,?,?)", id, strings.Repeat("a", 64), `{"native":true}`)
	nativeExec(t, db, "INSERT INTO ai_bridge_commands VALUES(?,?,?,?,?,?,?,?)", id, id, "start", `{"native":true}`, strings.Repeat("b", 64), true, 3, "2026-10-08 11:12:13.123456")
	nativeExec(t, db, "INSERT INTO ai_messaging_legacy_commands VALUES(?,?,?,?,?,?,?,?,?,?)", id, id, "start", []byte{0, 255}, sha([]byte{0, 255}), 3, "2026-10-08 11:12:13.123456", "2026-10-08T11:12:13.123456Z", strings.Repeat("c", 64), "2026-10-08 11:12:14.123456")
	if _, e = mdb.Collection("domain_event_outbox").InsertOne(context.Background(), bson.D{{Key: "_id", Value: primitive.NewObjectID()}, {Key: "event_id", Value: id}, {Key: "status", Value: "published"}, {Key: "raw", Value: primitive.Binary{Subtype: 0, Data: []byte{0, 255}}}}); e != nil {
		t.Fatal("native BSON fact creation failed")
	}
	// The namespace anchor observes real UUIDs of the fixed retained objects.
	// Create these facts only in this parent's owned database; the production
	// anchor's requirement and absence semantics remain unchanged.
	for _, kept := range []string{"answersheets", "interpret_report_artifacts", "interpretation_runs", "report_generations"} {
		if _, e = mdb.Collection(kept).InsertOne(context.Background(), bson.D{{Key: "_id", Value: "kept-fixture"}, {Key: "protected", Value: true}}); e != nil {
			t.Fatal("native retained namespace setup failed")
		}
	}
	port, e := strconv.Atoi(env["MONGODB_PORT"])
	if e != nil {
		t.Fatal("actual protected Mongo endpoint invalid")
	}
	endpoint, e := identitymeta.MongoEndpointSHA256(env["MONGODB_HOST"], port, name)
	if e != nil {
		t.Fatal("actual protected Mongo endpoint binding failed")
	}
	approvedAnchor, e := identitymeta.ObserveMongoNamespaceAnchor(context.Background(), mdb, endpoint)
	if e != nil {
		t.Fatal("actual approved kept UUID profile unavailable")
	}
	// This local fixture registers the original actual anchor before Capture.
	// It remains synthetic test approval, never a production auto-approval.
	a := nativeNamespaceArchive(t, db, mdb, dir, "801-1", "802-1", approvedAnchor.Clone())
	conn, e := db.Conn(context.Background())
	if e != nil {
		t.Fatal("native parent dedicated connection failed")
	}
	t.Cleanup(func() {
		if conn.Close() != nil {
			t.Error("native parent connection close failed")
		}
	})
	originalSQL := nativeTargetOriginalPoints(t, conn)
	originalMongo := bNativeMongoRows(t, mdb)
	kept := nativeTargetProtectedPoints(t, conn, mdb)
	delete(kept, "head")
	delete(kept, "mongo_head")
	defs, e := readSQLCatalog(context.Background(), conn)
	if e != nil {
		t.Fatal("native original SQL metadata unknown")
	}
	non, e := targetSQLNonTarget(defs, name)
	if e != nil {
		t.Fatal("native original SQL baseline unknown")
	}
	_, mdefs, e := mongoCatalog(context.Background(), mdb)
	if e != nil {
		t.Fatal("native original Mongo metadata unknown")
	}
	stable, e := targetBStableMongoNonTarget(mdefs)
	if e != nil {
		t.Fatal("native original kept UUID baseline unknown")
	}
	r := TargetRecoveryRequest{SourceSHA: a.data.Approval.SourceSHA, OperationID: a.data.Approval.OperationID, OriginalRunID: a.data.Approval.RunID, ActualRunID: "803-1", ManifestSHA256: strings.Repeat("d", 64), ArchiveSHA256: a.digest, SQLNonTargetSHA256: non, MongoNonTargetSHA256: targetMongoNonTarget(mdefs), SQLHead: 99, MongoHead: 38}
	windowDir, journal := filepath.Join(dir, "window"), filepath.Join(dir, "journal")
	if os.Mkdir(windowDir, 0700) != nil || os.Mkdir(journal, 0700) != nil {
		t.Fatal("native operation directories failed")
	}
	wb := fence.WindowBinding{TargetSHA256: fence.MaintenanceWindowTargetSHA256(), SourceSHA: r.SourceSHA, OperationID: r.OperationID, ManifestSHA256: r.ManifestSHA256, OriginalRunID: r.OriginalRunID}
	w, e := fence.StartMaintenanceWindow(context.Background(), windowDir, wb)
	if e != nil {
		t.Fatal("actual native root Window start refused")
	}
	start, e := w.Diagnostic(context.Background())
	if e != nil || start.StartSHA256 == "" || !start.BudgetOnly || start.MutationAllowed || start.DropReady {
		t.Fatal("native Window diagnostic overclaimed")
	}
	if w.Close() != nil {
		t.Fatal("native parent Window release failed")
	}
	h := bNativeHandoff{Name: name, Directory: dir, ArchiveDirectory: a.dir, ArchiveSHA256: a.digest, WindowDirectory: windowDir, WindowBinding: wb, Request: r, ApprovedBSourceSHA: wantSource, SQLOnly: sqlOnly}
	firstPath := filepath.Join(dir, "first-input.private.json")
	nativeJSON(t, firstPath, h)
	// One fixed local parent deadline, never renewed per process or restore.
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	first := bNativeChild(t, ctx, firstPath, "migrate", parent)
	mongoHead := uint64(39)
	if sqlOnly {
		mongoHead = 38
	}
	bNativeHeads(t, conn, mdb, 100, mongoHead)
	if first.SQLHead != 100 || first.MongoHead != mongoHead || first.WindowStartSHA256 != start.StartSHA256 {
		t.Fatal("native migration actual state differs")
	}
	w, e = fence.OpenMaintenanceWindow(ctx, windowDir, wb)
	if e != nil {
		t.Fatal("native original Window reopen failed")
	}
	// Seal the first real recovery budget once before material registration.
	// The next process must retain this exact record, never gain a new 600s.
	recoveryCtx, recoveryCancel, e := w.RecoveryContext(ctx)
	if e != nil || recoveryCtx == nil || recoveryCtx.Err() != nil {
		t.Fatal("native original recovery budget could not start")
	}
	recoveryCancel()
	var registration TargetRecoveryJournalSummary
	if sqlOnly {
		registration, e = InspectTargetBSQLOnlyRecoveryJournal(ctx, a, r, journal, w)
	} else {
		registration, e = InspectTargetRecoveryJournal(ctx, a, r, journal, w)
	}
	if e != nil || registration.WindowStartSHA256 != start.StartSHA256 || registration.MutationAllowed || registration.ProductionFenceVerified {
		t.Fatal("native original physical journal registration refused")
	}
	diagnostic, e := w.Diagnostic(ctx)
	if e != nil || diagnostic.StartSHA256 != start.StartSHA256 {
		t.Fatal("native original window changed")
	}
	intentRaw := bNativeReadRaw(t, filepath.Join(journal, "target-recovery-b-migration-intent.json"))
	resultRaw := bNativeReadRaw(t, filepath.Join(journal, "target-recovery-b-migration-result.json"))
	h.Resume = &TargetBRecoveryResumeRequest{Recovery: TargetRecoveryResumeRequest{Original: r, CurrentRunID: "804-1", JournalSHA256: registration.JournalSHA256, WindowStartSHA256: start.StartSHA256}, ApprovedBSourceSHA: wantSource, MigrationIntentSHA256: sha(intentRaw), MigrationResultSHA256: sha(resultRaw)}
	if w.Close() != nil {
		t.Fatal("native registration Window release failed")
	}
	secondPath := filepath.Join(dir, "second-input.private.json")
	nativeJSON(t, secondPath, h)
	second := bNativeChild(t, ctx, secondPath, "recover", parent)
	if second.SQLConnectionID == first.SQLConnectionID || second.WindowStartSHA256 != start.StartSHA256 || second.RecoverySHA256 == "" || second.RecoverySHA256 != diagnostic.RecoverySHA256 {
		t.Fatal("native new process/connection or original budget binding failed")
	}
	bNativeHeads(t, conn, mdb, 100, mongoHead)
	if !reflect.DeepEqual(originalSQL, nativeTargetOriginalPoints(t, conn)) || !reflect.DeepEqual(originalMongo, bNativeMongoRows(t, mdb)) {
		t.Fatal("actual original raw target data not restored")
	}
	after := nativeTargetProtectedPoints(t, conn, mdb)
	delete(after, "head")
	delete(after, "mongo_head")
	if !reflect.DeepEqual(kept, after) {
		t.Fatal("actual current MQ/evidence/non-target fact changed")
	}
	_, afterDefs, e := mongoCatalog(ctx, mdb)
	if e != nil {
		t.Fatal("native final Mongo catalog unknown")
	}
	afterStable, e := targetBStableMongoNonTarget(afterDefs)
	if e != nil || afterStable != stable {
		t.Fatal("native final kept UUID/index/options changed")
	}
	// Restored targets remain intentionally incompatible with ordinary B Up.
	if _, e = migration.PreflightCompatibilityPair(ctx, db, client, migration.PairConfig{MySQLDatabase: name, MongoDatabase: name, ExpectedSourceSHA: wantSource}); e == nil {
		t.Fatal("ordinary startup adopted restored compatibility targets")
	}
}

func bNativeReadRaw(t *testing.T, path string) []byte {
	t.Helper()
	f, e := openPrivateFile(path)
	if e != nil {
		t.Fatal("native raw binding file rejected")
	}
	raw, re := readPrivate(f, metadataBudget)
	ce := f.Close()
	if re != nil || ce != nil {
		t.Fatal("native raw binding read failed")
	}
	return raw
}

func bNativePhase(t *testing.T, input, phase string, sqlOnly bool) {
	t.Helper()
	nativeRoot(t, "owned-mysql.json", "34306")
	nativeRoot(t, "owned-mongo.json", "33317")
	h := bNativeRead[bNativeHandoff](t, input)
	if h.SQLOnly != sqlOnly || h.ApprovedBSourceSHA != buildversion.Get().GitCommit || h.Name == "" || h.WindowBinding.SourceSHA != h.Request.SourceSHA || (h.Resume != nil && phase != "recover") || (phase != "migrate" && phase != "recover") {
		t.Fatal("native phase binding rejected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	a, e := OpenArchive(ctx, h.ArchiveDirectory, h.ArchiveSHA256)
	if e != nil {
		bNativePhaseFailure(t, "archive_open", e)
	}
	w, e := fence.OpenMaintenanceWindow(ctx, h.WindowDirectory, h.WindowBinding)
	if e != nil {
		bNativePhaseFailure(t, "window_open", e)
	}
	closed := false
	t.Cleanup(func() {
		if !closed && w.Close() != nil {
			t.Error("native phase Window close failed")
		}
	})
	hook := &bNativeSQLCancellation{cancel: cancel}
	db := bNativeSQL(t, nativeEnv(t), h.Name, hook)
	conn, e := db.Conn(ctx)
	if e != nil {
		bNativePhaseFailure(t, "sql_connection", e)
	}
	t.Cleanup(func() {
		if conn.Close() != nil {
			t.Error("native phase borrowed connection release failed")
		}
	})
	client := nativeMongo(t, nativeEnv(t))
	mdb := client.Database(h.Name)
	var connectionID string
	if e = conn.QueryRowContext(ctx, "SELECT CAST(CONNECTION_ID() AS CHAR)").Scan(&connectionID); e != nil {
		bNativePhaseFailure(t, "sql_connection_identity", e)
	}
	if phase == "migrate" {
		p, e := PrepareTargetRecovery(ctx, a, TargetRecoveryBorrowed{conn, mdb}, h.Request, filepath.Join(h.Directory, "journal"), w)
		if e != nil {
			bNativePhaseFailure(t, "recovery_prepare", e)
		}
		applied, e := ApplyTargets(ctx, p)
		if e != nil || applied == nil || applied.Summary().Targets.NativeDropProofs != 4 || applied.Summary().Targets.DropReady {
			bNativePhaseFailure(t, "targets_apply", e)
		}
		if sqlOnly {
			hook.armed.Store(true)
		}
		proof, e := RunTargetBMigration(ctx, p, h.ApprovedBSourceSHA)
		if sqlOnly {
			if e == nil || proof != nil || !hook.fired.Load() {
				bNativePhaseFailure(t, "b_migration", e)
			}
			// Use a fresh read context after the original canceled migration ctx.
			bNativeHeads(t, conn, mdb, 100, 38)
			result := bNativeRead[targetBMigrationResult](t, filepath.Join(h.Directory, "journal", "target-recovery-b-migration-result.json"))
			if e = targetValidateSQLOnlyAttempt(result); e != nil {
				bNativePhaseFailure(t, "sql_only_result", e)
			}
			if _, e = migration.PreflightCompatibilityPair(context.Background(), db, client, migration.PairConfig{MySQLDatabase: h.Name, MongoDatabase: h.Name, ExpectedSourceSHA: h.ApprovedBSourceSHA}); e == nil {
				t.Fatal("ordinary restart completed an unknown partial pair")
			}
		} else {
			if e != nil || proof == nil || proof.Observation().SQLAfter != 100 || proof.Observation().MongoAfter != 39 {
				bNativePhaseFailure(t, "b_migration", e)
			}
			bNativeHeads(t, conn, mdb, 100, 39)
		}
	} else {
		if h.Resume == nil {
			t.Fatal("native independently registered expected materials absent")
		}
		var reconciled *TargetRecoveryReconciliation
		if sqlOnly {
			reconciled, e = ReconcileTargetBSQLOnlyRecovery(ctx, a, TargetRecoveryBorrowed{conn, mdb}, *h.Resume, filepath.Join(h.Directory, "journal"), w)
		} else {
			reconciled, e = ReconcileTargetBRecovery(ctx, a, TargetRecoveryBorrowed{conn, mdb}, *h.Resume, filepath.Join(h.Directory, "journal"), w)
		}
		if e != nil || reconciled == nil {
			bNativePhaseFailure(t, "recovery_reconcile", e)
		}
		s := reconciled.Summary()
		if s.Unresolved != 0 || s.MutationAllowed || s.ProductionAuthorityIntegrated || s.DropReady || !s.WriterFenceRequired {
			t.Fatal("native reconciliation silently minted writer/production authority")
		}
		restored, e := ResumeTargetRecovery(ctx, reconciled, TargetRecoveryBorrowed{conn, mdb}, w)
		if e != nil || restored == nil || restored.Summary().Targets.NativeDropProofs != 0 || restored.Summary().Targets.DropReady || restored.Summary().Targets.ProductionAuthorityIntegrated || restored.Summary().Targets.WholeWriterFenceProven {
			bNativePhaseFailure(t, "targets_restore", e)
		}
		if len(restored.Summary().Targets.Targets) != 4 {
			t.Fatal("native recovery omitted a target")
		}
		for i, target := range restored.Summary().Targets.Targets {
			if target.Database != targetDatabase(i) || target.Name != targetNames[i] || target.State != "restored_exact" || target.Records != a.data.Inventory.Targets[i].Records {
				t.Fatal("native full-source restore readback incomplete")
			}
		}
	}
	// Cancellation may be intentional for SQL-only; Window diagnostics use a
	// live new read context but the exact original kernel start/deadline records.
	diagnostic, e := w.Diagnostic(context.Background())
	if e != nil || !diagnostic.BudgetOnly || diagnostic.DropReady {
		t.Fatal("native original budget diagnostic unavailable")
	}
	mongoHead := uint64(39)
	if sqlOnly {
		mongoHead = 38
	}
	receipt := bNativeChildReceipt{PID: os.Getpid(), Stage: phase, SQLConnectionID: connectionID, WindowStartSHA256: diagnostic.StartSHA256, RecoverySHA256: diagnostic.RecoverySHA256, SQLHead: 100, MongoHead: mongoHead, WriterFenceRequired: true}
	if w.Close() != nil {
		t.Fatal("actual native Window close failed")
	}
	closed = true
	nativeJSON(t, filepath.Join(h.Directory, phase+".private.json"), receipt)
	// Cleanup callbacks and process exit still must succeed; the waiting parent
	// treats this metadata file as no success until the child is actually reaped.
}

// Keep Go's pinned native driver ABI explicit; delegation is checked by the
// compiler when root validates this candidate. None of these are mock drivers.
var _ driver.Connector = bNativeConnector{}
var _ driver.QueryerContext = (*bNativeDriverConn)(nil)
var _ driver.ExecerContext = (*bNativeDriverConn)(nil)
var _ driver.ConnBeginTx = (*bNativeDriverConn)(nil)
var _ driver.ConnPrepareContext = (*bNativeDriverConn)(nil)
var _ driver.Pinger = (*bNativeDriverConn)(nil)
var _ driver.SessionResetter = (*bNativeDriverConn)(nil)
var _ driver.Validator = (*bNativeDriverConn)(nil)
var _ driver.NamedValueChecker = (*bNativeDriverConn)(nil)
