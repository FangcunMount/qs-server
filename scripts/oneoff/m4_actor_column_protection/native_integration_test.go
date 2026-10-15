//go:build integration

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	retirementDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/operatorretirement"
	retirementRepo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor/operatorretirement"
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
	"go.mongodb.org/mongo-driver/mongo/readpref"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// These are test-only owned namespaces. No force/SetVersion or direct writes to
// either migration ledger are used, and this file implements no restoration.
type actorNative struct {
	t                          *testing.T
	name, dsn, uri, tool, dir  string
	root, db, repoPool         *sql.DB
	client                     *mongo.Client
	repo                       *retirementRepo.Repository
	tasks                      map[uint64]retirementDomain.Task
	recoveries                 map[string]retirementDomain.Recovery
	sqlSource, mongoSource     fstest.MapFS
	binding                    binding
	materialPath, materialHash string
	original                   material
}

func pinnedActorSources(t *testing.T) (fstest.MapFS, fstest.MapFS) {
	t.Helper()
	out := []fstest.MapFS{{}, {}}
	var inventory [][3]string
	for i, item := range []struct {
		backend, extension string
		head               int
	}{{"mysql", "sql", 102}, {"mongodb", "json", 40}} {
		root := filepath.Join("..", "..", "..", "internal", "pkg", "migration", "migrations", item.backend)
		entries, e := os.ReadDir(root)
		if e != nil {
			t.Fatal("fixed C migration source unavailable")
		}
		pattern := regexp.MustCompile(`^([0-9]{6})_[a-z0-9_]+\.(up|down)\.` + item.extension + `$`)
		for _, entry := range entries {
			match := pattern.FindStringSubmatch(entry.Name())
			if match == nil {
				continue
			}
			n, e := strconv.Atoi(match[1])
			if e != nil || n > item.head {
				continue
			}
			raw, e := os.ReadFile(filepath.Join(root, entry.Name()))
			if e != nil || len(raw) == 0 {
				t.Fatal("fixed C migration body unavailable")
			}
			out[i][entry.Name()] = &fstest.MapFile{Data: raw, Mode: 0444}
			inventory = append(inventory, [3]string{item.backend, "migrations/" + item.backend + "/" + entry.Name(), fingerprint(raw)})
		}
	}
	raw, e := json.Marshal(inventory)
	if e != nil || len(inventory) != cResourceFiles || fingerprint(raw) != cResources {
		t.Fatal("historical C040b resource inventory differs; latest head is never a substitute")
	}
	return out[0], out[1]
}

// This top-level check is a source-byte check, not native database evidence.
func TestM4ActorColumnsPinnedResources(t *testing.T) {
	sqlSource, mongoSource := pinnedActorSources(t)
	original := sqlSource["000102_retire_actor_compatibility_columns.up.sql"]
	if len(sqlSource) != 204 || len(mongoSource) != 80 || original == nil || strings.Count(string(original.Data), "DROP COLUMN") != 2 {
		t.Fatal("fixed original two-DROP resource unavailable")
	}
}

func newActorNative(t *testing.T) *actorNative {
	t.Helper()
	if os.Getenv("QS_M4_C_OBJECT_NATIVE_REQUIRED") != "1" || os.Getenv("QS_M4_C_OBJECT_NATIVE_DISPOSABLE_ACK") != "owned_synthetic_dependencies_only" || !sourceRE.MatchString(version.GitCommit) {
		t.Fatal("required owned native dependencies and actual compiled source must be explicit")
	}
	dsn, uri := os.Getenv("QS_M4_C_OBJECT_NATIVE_MYSQL_DSN"), os.Getenv("QS_M4_C_OBJECT_NATIVE_MONGO_URI")
	cfg, e := mysql.ParseDSN(dsn)
	if e != nil || cfg.Net != "tcp" || cfg.DBName != "" {
		t.Fatal("owned administrative loopback DSN without selected database required")
	}
	host, port, e := net.SplitHostPort(cfg.Addr)
	portNumber, portErr := strconv.Atoi(port)
	if e != nil || host != "127.0.0.1" || portErr != nil || portNumber <= 1024 || portNumber > 65535 || strconv.Itoa(portNumber) != port || port == "3306" {
		t.Fatal("exclusive nondefault MySQL loopback port required")
	}
	u, e := url.Parse(uri)
	if e != nil {
		t.Fatal("owned Mongo endpoint invalid")
	}
	mongoPort, mongoPortErr := strconv.Atoi(u.Port())
	if u.Scheme != "mongodb" || u.Hostname() != "127.0.0.1" || mongoPortErr != nil || mongoPort <= 1024 || mongoPort > 65535 || strconv.Itoa(mongoPort) != u.Port() || u.Port() == "27017" || u.Query().Get("replicaSet") != "rs0" || u.Query().Get("directConnection") != "true" {
		t.Fatal("exclusive nondefault direct rs0 Mongo loopback endpoint required")
	}
	if mysql.SetLogger(&mysql.NopLogger{}) != nil {
		t.Fatal("fixture logging boundary unavailable")
	}
	cfg.MultiStatements, cfg.ParseTime = true, false
	cfg.Timeout, cfg.ReadTimeout, cfg.WriteTimeout = queryLimit, time.Minute, queryLimit
	root, e := sql.Open("mysql", cfg.FormatDSN())
	if e != nil {
		t.Fatal("owned MySQL fixture unavailable")
	}
	root.SetMaxOpenConns(1)
	h := &actorNative{t: t, name: "qs_m4_c_native_" + strconv.FormatInt(time.Now().UnixNano(), 10), root: root, uri: uri, tool: version.GitCommit}
	t.Cleanup(func() {
		if h.repoPool != nil && h.repoPool.Close() != nil {
			t.Error("owned modern repository pool not released")
		}
		if h.db != nil && h.db.Close() != nil {
			t.Error("owned SQL pool not released")
		}
		if h.client != nil {
			ctx, cancel := context.WithTimeout(context.Background(), queryLimit)
			if h.client.Database(h.name).Drop(ctx) != nil {
				t.Error("owned Mongo namespace not released")
			}
			cancel()
			ctx, cancel = context.WithTimeout(context.Background(), queryLimit)
			if h.client.Disconnect(ctx) != nil {
				t.Error("owned Mongo client not released")
			}
			cancel()
		}
		ctx, cancel := context.WithTimeout(context.Background(), queryLimit)
		if _, e := root.ExecContext(ctx, "DROP DATABASE IF EXISTS `"+h.name+"`"); e != nil {
			t.Error("owned SQL namespace not released")
		}
		cancel()
		if root.Close() != nil {
			t.Error("owned administrative SQL pool not released")
		}
	})
	var sqlVersion string
	if root.QueryRowContext(t.Context(), "SELECT VERSION()").Scan(&sqlVersion) != nil || sqlVersion != "8.0.36" {
		t.Fatal("actual MySQL8.0.36 required")
	}
	if _, e := root.ExecContext(t.Context(), "CREATE DATABASE `"+h.name+"` CHARACTER SET utf8mb4"); e != nil {
		t.Fatal("new owned SQL namespace unavailable")
	}
	var e2 error
	h.dir, e2 = filepath.EvalSymlinks(t.TempDir())
	if e2 != nil || os.Chmod(h.dir, 0700) != nil {
		t.Fatal("private owned capture directory unavailable")
	}
	cfg.DBName = h.name
	h.dsn = cfg.FormatDSN()
	h.db, e = sql.Open("mysql", h.dsn)
	if e != nil {
		t.Fatal("owned selected SQL pool unavailable")
	}
	h.db.SetMaxOpenConns(1)
	h.sqlSource, h.mongoSource = pinnedActorSources(t)
	h.migrateSQL(101)
	h.client, e = mongo.Connect(t.Context(), mongoReadOptions(uri))
	if e != nil {
		t.Fatal("owned Mongo connection unavailable")
	}
	var build struct {
		Version string `bson:"version"`
	}
	if h.client.Database("admin").RunCommand(t.Context(), bson.D{{Key: "buildInfo", Value: 1}}).Decode(&build) != nil || build.Version != "7.0.31" || h.client.Ping(t.Context(), readpref.Primary()) != nil {
		t.Fatal("actual Mongo7.0.31 primary required")
	}
	h.migrateMongo()
	h.seed()
	h.seedModernRecords()
	h.binding = h.independentBinding()
	t.Logf("actual owned dependencies mysql=%s mongo=%s rs0; Csource=%s resources=%s files=%d; tool=%s", sqlVersion, build.Version, cSource, cResources, cResourceFiles, h.tool)
	return h
}

func (h *actorNative) migrateSQL(target uint) {
	h.t.Helper()
	source, e := iofs.New(h.sqlSource, ".")
	if e != nil {
		h.t.Fatal("fixed SQL source driver unavailable")
	}
	pool, e := sql.Open("mysql", h.dsn)
	if e != nil {
		h.t.Fatal("owned migration pool unavailable")
	}
	driver, e := migratemysql.WithInstance(pool, &migratemysql.Config{DatabaseName: h.name})
	if e != nil {
		_ = source.Close()
		_ = pool.Close()
		h.t.Fatal("normal SQL provider unavailable")
	}
	m, e := migrate.NewWithInstance("iofs", source, "mysql", driver)
	if e != nil {
		_ = source.Close()
		_ = driver.Close()
		h.t.Fatal("normal SQL provider assembly failed")
	}
	e = m.Migrate(target)
	se, de := m.Close()
	if (e != nil && e != migrate.ErrNoChange) || se != nil || de != nil {
		h.t.Fatal("normal SQL migration or owned provider release failed")
	}
}

func (h *actorNative) migrateMongo() {
	h.t.Helper()
	source, e := iofs.New(h.mongoSource, ".")
	if e != nil {
		h.t.Fatal("fixed Mongo source driver unavailable")
	}
	client, e := mongo.Connect(h.t.Context(), mongoReadOptions(h.uri))
	if e != nil {
		_ = source.Close()
		h.t.Fatal("owned Mongo provider connection unavailable")
	}
	driver, e := migratemongo.WithInstance(client, &migratemongo.Config{DatabaseName: h.name, MigrationsCollection: "schema_migrations"})
	if e != nil {
		_ = source.Close()
		_ = client.Disconnect(h.t.Context())
		h.t.Fatal("normal Mongo provider unavailable")
	}
	cleanup, e := qsMigration.NewMongoDriver(client).PrepareRun(h.t.Context(), &qsMigration.Config{Database: h.name}, 0)
	if e != nil {
		_ = source.Close()
		_ = driver.Close()
		h.t.Fatal("normal historical Mongo preparation failed")
	}
	m, e := migrate.NewWithInstance("iofs", source, "mongodb", driver)
	if e != nil {
		_ = cleanup(h.t.Context())
		_ = source.Close()
		_ = driver.Close()
		h.t.Fatal("normal Mongo provider assembly failed")
	}
	e = m.Migrate(40)
	ce := cleanup(h.t.Context())
	se, de := m.Close()
	if (e != nil && e != migrate.ErrNoChange) || ce != nil || se != nil || de != nil {
		h.t.Fatal("normal Mongo migration or owned provider release failed")
	}
}

func (h *actorNative) seed() {
	h.t.Helper()
	conn, e := h.db.Conn(h.t.Context())
	if e != nil {
		h.t.Fatal("owned seed connection unavailable")
	}
	defer func() {
		if conn.Close() != nil {
			h.t.Error("owned seed connection not released")
		}
	}()
	for _, query := range []string{
		"SET SESSION sql_mode=CONCAT(@@session.sql_mode,',NO_AUTO_VALUE_ON_ZERO')",
		`INSERT INTO testee(id,org_id,name,gender,tags) VALUES(0,1,'owned-null',0,NULL),(7,1,'owned-array',0,JSON_ARRAY()),(9,1,'owned-json-null',0,'null'),(18446744073709551615,1,'owned-max',0,JSON_ARRAY('retained-original'))`,
		`INSERT INTO operators(id,org_id,user_id,roles,effective_roles,name,is_active) VALUES(0,1,100,JSON_ARRAY('qs:admin'),'null','owned-json-null',TRUE),(7,1,107,JSON_ARRAY('qs:admin'),JSON_ARRAY('owned-original-effective-role'),'owned-historical',TRUE),(9,1,109,JSON_ARRAY('qs:result_reviewer'),JSON_ARRAY(),'owned-empty',TRUE),(18446744073709551615,1,199,JSON_ARRAY('qs:admin'),JSON_ARRAY('qs:result_reviewer'),'owned-max',TRUE)`,
	} {
		if _, e := conn.ExecContext(h.t.Context(), query); e != nil {
			h.t.Fatal("owned synthetic actor seed failed")
		}
	}
}

// Normal repository writes construct valid local facts solely for these owned
// fixtures. Later assertions cover archive/task reads and immutable bytes, not
// application authorization, IAM convergence or the complete recovery journey.
func (h *actorNative) seedModernRecords() {
	h.t.Helper()
	cfg, e := mysql.ParseDSN(h.dsn)
	if e != nil {
		h.t.Fatal("owned modern repository DSN unavailable")
	}
	cfg.ParseTime, cfg.Loc = true, time.UTC
	h.repoPool, e = sql.Open("mysql", cfg.FormatDSN())
	if e != nil {
		h.t.Fatal("owned modern repository pool unavailable")
	}
	h.repoPool.SetMaxOpenConns(1)
	db, e := gorm.Open(gormmysql.New(gormmysql.Config{Conn: h.repoPool, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		h.t.Fatal("owned modern repository unavailable")
	}
	h.repo = retirementRepo.NewRepository(db)
	h.recoveries = map[string]retirementDomain.Recovery{}
	now := time.Date(2026, 10, 15, 0, 0, 0, 123000000, time.UTC)
	version := uint32(1)
	for i, request := range []string{"A", "a"} {
		at := now.Add(time.Duration(i) * time.Second)
		task, e := retirementDomain.New(7, 1, 107, 199, version, "exit-"+request, "owned valid historical exit", at)
		if e != nil || h.repo.Begin(h.t.Context(), task) != nil {
			h.t.Fatal("normal owned retirement begin failed")
		}
		policy := int64(10 + i)
		if task.MarkRevoked(policy, at) != nil || h.repo.Save(h.t.Context(), task) != nil || task.Complete(policy, false, at) != nil || h.repo.Finish(h.t.Context(), task) != nil {
			h.t.Fatal("normal owned retirement completion failed")
		}
		recovery, e := retirementDomain.NewRecovery(task, 199, version+2, policy, request, "owned valid archive construction", at)
		if e != nil || h.repo.WithOperatorLock(h.t.Context(), 1, 7, func(ctx context.Context) error { return h.repo.Recover(ctx, recovery) }) != nil {
			h.t.Fatal("normal owned recovery archive construction failed")
		}
		h.recoveries[request] = recovery
		version += 3
	}
	h.tasks = map[uint64]retirementDomain.Task{}
	for _, target := range []struct {
		id      uint64
		user    int64
		version uint32
	}{{7, 107, version}, {9, 109, 1}} {
		task, e := retirementDomain.New(target.id, 1, target.user, 199, target.version, "pending-"+strconv.FormatUint(target.id, 10), "owned valid pending exit", now.Add(3*time.Second))
		if e != nil || h.repo.Begin(h.t.Context(), task) != nil {
			h.t.Fatal("normal owned pending retirement construction failed")
		}
		h.tasks[target.id] = task
	}
	h.assertModernReads()
}

func (h *actorNative) assertModernReads() {
	h.t.Helper()
	for id, want := range h.tasks {
		got, e := h.repo.FindTask(h.t.Context(), 1, id)
		if e != nil || got == nil || *got != want {
			h.t.Fatal("modern task payload or row identity changed")
		}
	}
	for request, want := range h.recoveries {
		got, e := h.repo.FindRecovery(h.t.Context(), request)
		if e != nil || got == nil || *got != want {
			h.t.Fatal("modern recovery checksum, payload or row identity changed")
		}
	}
}

func (h *actorNative) independentBinding() binding {
	h.t.Helper()
	var uuid, selected string
	if h.db.QueryRowContext(h.t.Context(), "SELECT @@server_uuid,DATABASE()").Scan(&uuid, &selected) != nil || selected != h.name {
		h.t.Fatal("independent SQL identity unavailable")
	}
	raw, _ := json.Marshal([]any{"mysql_server_selected_v1", uuid, selected})
	b := binding{SQLName: h.name, MongoName: h.name, SQLIdentity: fingerprint(raw)}
	hello, e := h.client.Database("admin").RunCommand(h.t.Context(), bson.D{{Key: "hello", Value: 1}}).Raw()
	hosts, e2 := primaryReplicaHosts(hello)
	if e != nil || e2 != nil {
		h.t.Fatal("independent rs0 primary identity unavailable")
	}
	raw, _ = json.Marshal([]any{"mongodb_cluster_selected_v1", bson.D{{Key: "setName", Value: "rs0"}, {Key: "hosts", Value: hosts}}, h.name})
	b.MongoIdentity = fingerprint(raw)
	cur, e := h.client.Database(h.name).ListCollections(h.t.Context(), bson.D{{Key: "name", Value: "schema_migrations"}})
	if e != nil {
		h.t.Fatal("independent ledger catalog unavailable")
	}
	n, valid := 0, true
	for cur.Next(h.t.Context()) {
		subtype, value, ok := cur.Current.Lookup("info", "uuid").BinaryOK()
		if !ok || subtype != 4 || len(value) != 16 {
			valid = false
		} else {
			b.MongoUUID = fingerprint(append([]byte{subtype}, value...))
		}
		n++
	}
	re, ce := cur.Err(), cur.Close(h.t.Context())
	if re != nil || ce != nil || cur.ID() != 0 || n != 1 || !valid {
		h.t.Fatal("independent ledger UUID cursor incomplete or unreleased")
	}
	return b
}

func (h *actorNative) action(action string, extras ...string) receipt {
	h.t.Helper()
	args := []string{"--action=" + action, "--mysql-database=" + h.name, "--mongo-database=" + h.name, "--mysql-identity-sha256=" + h.binding.SQLIdentity, "--mongo-identity-sha256=" + h.binding.MongoIdentity, "--mongo-ledger-uuid-sha256=" + h.binding.MongoUUID, "--timeout=2m"}
	args = append(args, extras...)
	return run(h.t.Context(), args, func(key string) string {
		switch key {
		case "QS_M4_C_OBJECT_MYSQL_DSN":
			return h.dsn
		case "QS_M4_C_OBJECT_MONGO_URI":
			return h.uri
		}
		return ""
	}, h.tool)
}

func (h *actorNative) capture() {
	h.t.Helper()
	h.materialPath = filepath.Join(h.dir, "first-capture.json")
	r := h.action("capture", "--capture-file="+h.materialPath)
	if !r.Complete || !r.ReadOnly || !r.SQLConnectionClosed || !r.SQLPoolClosed || !r.MongoClosed || !r.MandatoryEOF || !r.SQLSnapshot || r.ApprovalConsumed {
		h.t.Fatal("formal capture failed", r.Error, r.ReleaseError)
	}
	h.materialHash = r.MaterialHash
	raw, e := readPrivate(h.materialPath, h.materialHash)
	if e != nil || strictDecode(raw, &h.original) != nil || validateMaterial(h.original, h.tool, h.binding) != nil {
		h.t.Fatal("captured original material unavailable")
	}
	if r := h.action("capture", "--capture-file="+h.materialPath); r.Complete || r.Error != "new_private_capture_required" {
		h.t.Fatal("first capture was overwritten", r.Error)
	}
}

func (h *actorNative) verify() receipt {
	return h.action("verify-target", "--material-file="+h.materialPath, "--material-sha256="+h.materialHash)
}

// Independent witnesses query the full original server representations rather
// than reusing the tool's retained-row encoding as its own expected result.
func (h *actorNative) query(query string) [][]sql.NullString {
	h.t.Helper()
	rows, e := h.db.QueryContext(h.t.Context(), query)
	if e != nil {
		h.t.Fatal("independent SQL witness query failed")
	}
	cols, e := rows.Columns()
	if e != nil {
		_ = rows.Close()
		h.t.Fatal("independent SQL witness columns unavailable")
	}
	var result [][]sql.NullString
	for rows.Next() {
		values, scan := make([]sql.NullString, len(cols)), make([]any, len(cols))
		for i := range values {
			scan[i] = &values[i]
		}
		if rows.Scan(scan...) != nil {
			_ = rows.Close()
			h.t.Fatal("independent SQL witness row invalid")
		}
		result = append(result, values)
	}
	re, ce := rows.Err(), rows.Close()
	if re != nil || ce != nil {
		h.t.Fatal("independent SQL witness incomplete or unreleased")
	}
	return result
}

func (h *actorNative) retainedRaw() [4][][]sql.NullString {
	h.t.Helper()
	var out [4][][]sql.NullString
	for i, s := range scope {
		var cols []string
		for _, row := range h.query("SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='" + s.table + "' ORDER BY ORDINAL_POSITION") {
			if row[0].String != s.target {
				cols = append(cols, "CAST(`"+row[0].String+"` AS BINARY)")
			}
		}
		out[i] = h.query("SELECT " + strings.Join(cols, ",") + " FROM `" + s.table + "` ORDER BY `" + s.key + "`")
	}
	return out
}

func (h *actorNative) mongoHead() []byte {
	h.t.Helper()
	cur, e := h.client.Database(h.name).Collection("schema_migrations").Find(h.t.Context(), bson.D{}, options.Find().SetLimit(2).SetBatchSize(2))
	if e != nil {
		h.t.Fatal("actual provider clean40 ledger unavailable")
	}
	var raw []byte
	n := 0
	for cur.Next(h.t.Context()) {
		raw = append([]byte(nil), cur.Current...)
		n++
	}
	re, ce := cur.Err(), cur.Close(h.t.Context())
	if re != nil || ce != nil || cur.ID() != 0 || n != 1 || !cleanMongo40(raw) {
		h.t.Fatal("independent original clean40 ledger incomplete or unreleased")
	}
	return raw
}

func TestM4ActorColumnsNativeCaptureUpgradeVerify(t *testing.T) {
	h := newActorNative(t)
	h.assertModernReads()
	before, mongoHead := h.retainedRaw(), h.mongoHead()
	h.capture()
	values := h.query("SELECT id,CAST(tags AS BINARY) FROM testee ORDER BY id")
	if len(values) != 4 || values[0][0].String != "0" || values[0][1].Valid || values[1][1].String != "[]" || values[2][1].String != "null" || values[3][0].String != "18446744073709551615" {
		t.Fatal("fixture did not preserve actual SQL NULL/JSON null/array/uint64 boundaries")
	}
	for i, table := range h.original.Tables {
		if table.Pages != 2 || !table.EOF || !reflect.DeepEqual(h.query("SHOW CREATE TABLE `"+scope[i].table+"`"), [][]sql.NullString{{{String: scope[i].table, Valid: true}, {String: table.DDL, Valid: true}}}) {
			t.Fatal("original DDL or complete EOF was not captured")
		}
		if i < 2 {
			rows := h.query("SELECT id,CAST(`" + scope[i].target + "` AS BINARY) FROM `" + scope[i].table + "` ORDER BY id")
			for n, row := range rows {
				v := table.Rows[n].Value
				if table.Rows[n].Key != row[0].String || (v.Kind == "json") != row[1].Valid || string(v.JSON) != row[1].String {
					t.Fatal("original server JSON bytes differ from independent witness")
				}
			}
		}
	}
	for _, request := range []string{"A", "a"} {
		rows := h.query("SELECT retirement_before_state,operator_before_state FROM operator_recovery_archives WHERE request_id='" + request + "'")
		if len(rows) != 1 || !strings.Contains(rows[0][0].String, `"effective_roles"`) || !strings.Contains(rows[0][1].String, `"effective_roles"`) {
			t.Fatal("normal archive did not retain the actual old-schema facts")
		}
	}
	h.migrateSQL(102)
	h.assertModernReads()
	r := h.verify()
	if !r.Complete || r.Error != "none" || !r.SQLConnectionClosed || !r.SQLPoolClosed || !r.MongoClosed || !r.MandatoryEOF {
		t.Fatal("formal target verification failed", r.Error)
	}
	if !reflect.DeepEqual(before, h.retainedRaw()) || !bytes.Equal(mongoHead, h.mongoHead()) || h.binding != h.independentBinding() {
		t.Fatal("actual SQL retained facts or Mongo ledger/UUID changed")
	}
	for i, s := range scope {
		if i < 2 && !reflect.DeepEqual(h.query("SHOW CREATE TABLE `"+s.table+"`"), [][]sql.NullString{{{String: s.table, Valid: true}, {String: h.original.Tables[i].RetainedDDL, Valid: true}}}) {
			t.Fatal("target changed more than its fixed column")
		}
	}
	if r := h.action("restore"); r.Complete || r.ApprovalConsumed || r.Error != "restore_not_implemented" {
		t.Fatal("native fixture activated restoration")
	}
}

func TestM4ActorColumnsNativeUnboundMaterialRejected(t *testing.T) {
	h := newActorNative(t)
	h.capture()
	before, head := h.retainedRaw(), h.mongoHead()
	for _, name := range []string{"hash", "source", "scope", "identity", "eof", "boundary"} {
		h.t.Run(name, func(t *testing.T) {
			m := cloneMaterial(t, h.original)
			switch name {
			case "source":
				m.CSource = fixtureSource
			case "scope":
				m.Kind = "m4_a_sql_null_object_protection"
			case "identity":
				m.Binding.MongoUUID = strings.Repeat("f", 64)
			case "eof":
				m.Tables[0].EOF = false
			case "boundary":
				m.Tables[0].Rows[0].Key = "00"
			}
			raw, _ := json.Marshal(m)
			path := filepath.Join(h.dir, name+".json")
			if writeExclusive(path, raw) != nil {
				t.Fatal("private negative material unavailable")
			}
			hash := fingerprint(raw)
			if name == "hash" {
				hash = strings.Repeat("0", 64)
			}
			args := []string{"--action=verify-target", "--mysql-database=" + h.name, "--mongo-database=" + h.name, "--mysql-identity-sha256=" + h.binding.SQLIdentity, "--mongo-identity-sha256=" + h.binding.MongoIdentity, "--mongo-ledger-uuid-sha256=" + h.binding.MongoUUID, "--material-file=" + path, "--material-sha256=" + hash}
			r := run(t.Context(), args, func(string) string { t.Fatal("unbound input accessed connection configuration"); return "" }, h.tool)
			if r.Complete || r.SQLPoolClosed || r.MongoClosed || r.ApprovalConsumed {
				t.Fatal("unbound material crossed the pre-connection boundary")
			}
		})
	}
	if !reflect.DeepEqual(before, h.retainedRaw()) || !bytes.Equal(head, h.mongoHead()) {
		t.Fatal("rejected material changed owned data")
	}
}

func TestM4ActorColumnsNativeRetainedAndBoundaryDrift(t *testing.T) {
	h := newActorNative(t)
	h.capture()
	h.migrateSQL(102)
	h.assertModernReads()
	original := h.query("SELECT recovery_payload FROM operator_recovery_archives WHERE request_id='A'")
	if len(original) != 1 || !original[0][0].Valid {
		t.Fatal("normal original recovery payload unavailable")
	}
	if _, e := h.db.ExecContext(t.Context(), "UPDATE operator_recovery_archives SET recovery_payload='changed-original' WHERE request_id='A'"); e != nil {
		t.Fatal("owned archive drift not installed")
	}
	if _, e := h.repo.FindRecovery(t.Context(), "A"); e == nil {
		t.Fatal("actual archive checksum reader accepted changed payload")
	}
	if r := h.verify(); r.Complete || r.Error != "target_retained_facts_rejected" {
		t.Fatal("archive drift accepted", r.Error)
	}
	if _, e := h.db.ExecContext(t.Context(), "UPDATE operator_recovery_archives SET recovery_payload=? WHERE request_id='A'", original[0][0].String); e != nil {
		t.Fatal("owned archive reset failed")
	}
	if r := h.verify(); !r.Complete {
		t.Fatal("independent original archive bytes no longer match", r.Error)
	}
	h.assertModernReads()
	if _, e := h.db.ExecContext(t.Context(), "UPDATE operator_recovery_archives SET user_id=user_id+1 WHERE request_id='a'"); e != nil {
		t.Fatal("owned row identity drift unavailable")
	}
	if _, e := h.repo.FindRecovery(t.Context(), "a"); e == nil {
		t.Fatal("actual archive reader accepted changed row identity")
	}
	if r := h.verify(); r.Complete || r.Error != "target_retained_facts_rejected" {
		t.Fatal("archive row identity drift accepted", r.Error)
	}
	if _, e := h.db.ExecContext(t.Context(), "UPDATE operator_recovery_archives SET user_id=? WHERE request_id='a'", h.recoveries["a"].UserID); e != nil {
		t.Fatal("owned row identity reset failed")
	}
	h.assertModernReads()
	if _, e := h.db.ExecContext(t.Context(), "INSERT INTO testee(id,org_id,name,gender) VALUES(10,1,'owned-new-fact',0)"); e != nil {
		t.Fatal("owned new boundary fact unavailable")
	}
	if r := h.verify(); r.Complete || r.Error != "target_retained_facts_rejected" {
		t.Fatal("new primary key accepted under old material", r.Error)
	}
}

func TestM4ActorColumnsNativeOriginalSchemaDrift(t *testing.T) {
	h := newActorNative(t)
	if _, e := h.db.ExecContext(t.Context(), "ALTER TABLE testee MODIFY COLUMN tags JSON DEFAULT NULL COMMENT 'owned definition drift'"); e != nil {
		t.Fatal("owned definition drift unavailable")
	}
	path := filepath.Join(h.dir, "rejected.json")
	r := h.action("capture", "--capture-file="+path)
	if r.Complete || r.Error != "original_C_column_definition_rejected" {
		t.Fatal("changed original column definition accepted", r.Error)
	}
	if _, e := os.Lstat(path); !os.IsNotExist(e) {
		t.Fatal("rejected original shape produced a material")
	}
}

func TestM4ActorColumnsNativeIncompletePrivilege(t *testing.T) {
	h := newActorNative(t)
	user := "qs_m4_c_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if _, e := h.root.ExecContext(t.Context(), "CREATE USER '"+user+"'@'%' IDENTIFIED BY 'owned-only-fixture'"); e != nil {
		t.Fatal("owned limited principal unavailable")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), queryLimit)
		defer cancel()
		if _, e := h.root.ExecContext(ctx, "DROP USER '"+user+"'@'%'"); e != nil {
			t.Error("owned limited principal not released")
		}
	})
	if _, e := h.root.ExecContext(t.Context(), "GRANT SELECT ON `"+h.name+"`.testee TO '"+user+"'@'%'"); e != nil {
		t.Fatal("owned partial visibility fixture unavailable")
	}
	cfg, _ := mysql.ParseDSN(h.dsn)
	cfg.User, cfg.Passwd = user, "owned-only-fixture"
	h.dsn = cfg.FormatDSN()
	r := h.action("capture", "--capture-file="+filepath.Join(h.dir, "partial.json"))
	if r.Complete || r.ApprovalConsumed || r.Error != "fixed_table_scope_or_release_unknown" {
		t.Fatal("partial metadata visibility became a complete capture", r.Error)
	}
}
