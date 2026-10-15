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
)

// All namespaces are created by this fixture on an explicitly acknowledged,
// non-default loopback pair. Providers create every ledger; no Force, SetVersion
// or fixture ledger writes manufacture the source or target states.
type maintenanceNative struct {
	t                          *testing.T
	name, dsn, uri, tool, dir  string
	root, db                   *sql.DB
	client                     *mongo.Client
	sqlSource, mongoSource     fstest.MapFS
	binding                    binding
	materialPath, materialHash string
	original                   material
}

func pinnedMaintenanceSources(t *testing.T) (fstest.MapFS, fstest.MapFS) {
	t.Helper()
	out := []fstest.MapFS{{}, {}}
	var inventory [][3]string
	for i, item := range []struct {
		backend, extension string
		head               int
	}{{"mysql", "sql", 103}, {"mongodb", "json", 40}} {
		root := filepath.Join("..", "..", "..", "internal", "pkg", "migration", "migrations", item.backend)
		entries, e := os.ReadDir(root)
		if e != nil {
			t.Fatal("fixed D migration source unavailable")
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
				t.Fatal("fixed D migration body unavailable")
			}
			out[i][entry.Name()] = &fstest.MapFile{Data: raw, Mode: 0444}
			inventory = append(inventory, [3]string{item.backend, "migrations/" + item.backend + "/" + entry.Name(), fingerprint(raw)})
		}
	}
	raw, e := json.Marshal(inventory)
	if e != nil || len(inventory) != dResourceFiles || fingerprint(raw) != dResources {
		t.Fatal("historical Dc335 resource inventory differs; latest head is never a substitute")
	}
	return out[0], out[1]
}

// Missing/unknown dependencies fail every selected native entry; there is no SKIP path.
func newMaintenanceNative(t *testing.T) *maintenanceNative {
	t.Helper()
	if os.Getenv("QS_M4_D_OBJECT_NATIVE_REQUIRED") != "1" || os.Getenv("QS_M4_D_OBJECT_NATIVE_DISPOSABLE_ACK") != "owned_synthetic_dependencies_only" || !sourceRE.MatchString(version.GitCommit) {
		t.Fatal("required owned native dependencies and actual compiled source must be explicit")
	}
	dsn, uri := os.Getenv("QS_M4_D_OBJECT_NATIVE_MYSQL_DSN"), os.Getenv("QS_M4_D_OBJECT_NATIVE_MONGO_URI")
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
	h := &maintenanceNative{t: t, name: "qs_m4_d_native_" + strconv.FormatInt(time.Now().UnixNano(), 10), root: root, uri: uri, tool: version.GitCommit}
	t.Cleanup(func() {
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
	private, e := privateParent(filepath.Join(h.dir, "first-capture.json"))
	if e != nil {
		t.Fatal("trusted private TMPDIR path required; production parent rule is not weakened")
	}
	if private.Close() != nil {
		t.Fatal("owned private parent release unproven")
	}
	cfg.DBName = h.name
	h.dsn = cfg.FormatDSN()
	h.db, e = sql.Open("mysql", h.dsn)
	if e != nil {
		t.Fatal("owned selected SQL pool unavailable")
	}
	h.db.SetMaxOpenConns(1)
	h.sqlSource, h.mongoSource = pinnedMaintenanceSources(t)
	h.migrateSQL(102)
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
	h.binding = h.independentBinding()
	t.Logf("actual owned dependencies mysql=%s mongo=%s rs0; Dsource=%s resources=%s files=%d; tool=%s", sqlVersion, build.Version, dSource, dResources, dResourceFiles, h.tool)
	return h
}

func (h *maintenanceNative) migrateSQL(target uint) {
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

func (h *maintenanceNative) migrateMongo() {
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

func (h *maintenanceNative) independentBinding() binding {
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

func (h *maintenanceNative) action(action string, extras ...string) receipt {
	h.t.Helper()
	args := []string{"--action=" + action, "--mysql-database=" + h.name, "--mongo-database=" + h.name, "--mysql-identity-sha256=" + h.binding.SQLIdentity, "--mongo-identity-sha256=" + h.binding.MongoIdentity, "--mongo-ledger-uuid-sha256=" + h.binding.MongoUUID, "--timeout=2m"}
	args = append(args, extras...)
	return run(h.t.Context(), args, func(key string) string {
		switch key {
		case "QS_M4_D_OBJECT_MYSQL_DSN":
			return h.dsn
		case "QS_M4_D_OBJECT_MONGO_URI":
			return h.uri
		}
		return ""
	}, h.tool)
}

func (h *maintenanceNative) query(query string) [][]sql.NullString {
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

func (h *maintenanceNative) mongoHead() []byte {
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

func (h *maintenanceNative) seed() {
	h.t.Helper()
	// LONGTEXT whitespace/order and its already-stored checksum are immutable
	// fixture facts; no removed historical CLI or checksum interpretation is run.
	payload := []byte("{ \"z\": 1, \"a\":\"四字节😀\", \"z\": 2, \"nested\": [null, []] }")
	before := []byte("{\"operator_id\": 0, \"roles\": [\"old\"], \"note\": \"😀\" }")
	for _, row := range []struct {
		query string
		args  []any
	}{
		{"INSERT INTO operator_retirement_manifests(migration_id,stage,payload,payload_sha256,created_at,updated_at) VALUES(?,?,?,?,?,?)", []any{"A0", "applied_unchanged", payload, fingerprint(payload), "2026-10-15 01:02:03.123456", "2026-10-15 01:02:04.654321"}},
		{"INSERT INTO clinician_operator_binding_archive(clinician_id,org_id,operator_id,migration_id,before_record,record_sha256,archived_at) VALUES(0,1,0,'A0',?,?,'2026-10-15 01:02:03.123456'),(18446744073709551615,1,18446744073709551615,'A0',?,?,'2026-10-15 01:02:03.654321')", []any{before, fingerprint(before), before, fingerprint(before)}},
		{"INSERT INTO testee_store_migration_manifests(migration_id,state,before_hash,after_hash,rollback_hash,items_hash,item_count,actor_id,created_at,completed_at,rolled_back_at) VALUES('','applied',?,'','',?,6,10001,'2026-10-15 01:02:03.123456',NULL,NULL),('A0','applied',?,?,'',?,0,10001,'2026-10-15 01:02:03.123456','2026-10-15 01:02:04.654321',NULL)", []any{fingerprint(payload), fingerprint(before), fingerprint(before), fingerprint(payload), fingerprint(before)}},
		{"INSERT INTO testee_store_migration_items(migration_id,testee_id,org_id,before_store_id,target_store_id,before_version,applied_version,history_id,disposition,reason) VALUES('',0,1,NULL,NULL,0,0,NULL,'deferred',''),('',18446744073709551615,1,0,18446744073709551615,0,4294967295,18446744073709551615,'candidate','😀;null'),('A',0,1,NULL,0,1,2,0,'candidate','upper'),('Z',0,1,NULL,0,1,2,0,'candidate','binary-before-a'),('a',18446744073709551615,1,0,NULL,2,3,NULL,'deferred','lower'),('😀',0,1,NULL,NULL,0,0,NULL,'deferred','四字节')", nil},
		{"INSERT INTO testee_store_migration_reversions(migration_id,testee_id,org_id,from_store_id,original_history_id,restored_version,actor_id,created_at) VALUES('',0,1,0,0,0,10001,'2026-10-15 01:02:03.123456'),('',18446744073709551615,1,18446744073709551615,18446744073709551615,4294967295,10001,'2026-10-15 01:02:03.654321')", nil},
	} {
		if _, e := h.db.ExecContext(h.t.Context(), row.query, row.args...); e != nil {
			h.t.Fatal("owned historical synthetic seed failed")
		}
	}
}
func (h *maintenanceNative) capture() {
	h.t.Helper()
	h.materialPath = filepath.Join(h.dir, "first-capture.json")
	r := h.action("capture", "--capture-file="+h.materialPath)
	if !r.Complete || !r.ReadOnly || !r.SQLConnectionClosed || !r.SQLPoolClosed || !r.MongoClosed || !r.MandatoryEOF || !r.SQLSnapshot || r.ProductionFreezeCertified || r.CrossDatabaseAtomicRR {
		h.t.Fatal("formal scoped capture failed", r.Error, r.ReleaseError)
	}
	h.materialHash = r.MaterialHash
	raw, e := readPrivate(h.materialPath, h.materialHash)
	if e != nil || strictDecode(raw, &h.original) != nil || validateMaterial(h.original, h.tool, h.binding) != nil {
		h.t.Fatal("formal original material unavailable")
	}
}
func (h *maintenanceNative) verify() receipt {
	return h.action("verify-target", "--material-file="+h.materialPath, "--material-sha256="+h.materialHash)
}

// Independent witnesses preserve server values and NULLs directly; they do not
// compute their expected data with the tool's row frame or material decoder.
func (h *maintenanceNative) originalRaw() [5][][]sql.NullString {
	h.t.Helper()
	var out [5][][]sql.NullString
	for i, s := range scope {
		cols := h.query("SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='" + s.name + "' ORDER BY ORDINAL_POSITION")
		var selected []string
		for _, c := range cols {
			selected = append(selected, "CAST(`"+c[0].String+"` AS BINARY)")
		}
		order := "CAST(`migration_id` AS BINARY)"
		if !s.binaryFirst {
			order = "clinician_id"
		}
		if len(s.keys) == 2 {
			order += ",testee_id"
		}
		out[i] = h.query("SELECT " + strings.Join(selected, ",") + " FROM `" + s.name + "` ORDER BY " + order)
	}
	return out
}
func (h *maintenanceNative) assertCaptureRaw(raw [5][][]sql.NullString) {
	h.t.Helper()
	for i, rows := range raw {
		table := h.original.Tables[i]
		ddlRows := h.query("SHOW CREATE TABLE `" + scope[i].name + "`")
		if len(ddlRows) != 1 || len(ddlRows[0]) != 2 || ddlRows[0][1].String != table.DDL || len(rows) != len(table.Rows) || !table.EOF || table.Pages != (len(rows)+pageSize-1)/pageSize+1 {
			h.t.Fatal("DDL/full raw range/EOF mismatch")
		}
		keys := h.query("SELECT COLUMN_NAME FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='" + scope[i].name + "' AND INDEX_NAME='PRIMARY' ORDER BY SEQ_IN_INDEX")
		if len(keys) != len(table.Keys) {
			h.t.Fatal("original complete primary key missing")
		}
		for n, key := range keys {
			if key[0].String != table.Keys[n] {
				h.t.Fatal("primary key order changed")
			}
		}
		for n, row := range rows {
			if len(row) != len(table.Rows[n].Values) {
				h.t.Fatal("original column omitted")
			}
			for c, v := range row {
				got := table.Rows[n].Values[c]
				if got.SQLNull == v.Valid || !bytes.Equal(got.Bytes, []byte(v.String)) {
					h.t.Fatal("original server bytes/NULL changed")
				}
			}
		}
	}
	ledger := h.query("SELECT version,dirty FROM schema_migrations ORDER BY version")
	if !reflect.DeepEqual(ledger, [][]sql.NullString{{{String: "102", Valid: true}, {String: "0", Valid: true}}}) || len(h.original.Ledger) != 1 || h.original.Ledger[0] != (ledgerRow{102, false}) {
		h.t.Fatal("original real provider ledger missing")
	}
	ld := h.query("SHOW CREATE TABLE schema_migrations")
	if len(ld) != 1 || ld[0][1].String != h.original.LedgerDDL {
		h.t.Fatal("original full ledger DDL missing")
	}
}
func (h *maintenanceNative) assertReadOnlyReleased(r receipt) {
	h.t.Helper()
	if !r.ReadOnly || !r.SQLConnectionClosed || !r.SQLPoolClosed || !r.MongoClosed || r.ReleaseError != "" || r.TransactionReleaseUnproven || r.ProductionFreezeCertified || r.CrossDatabaseAtomicRR {
		h.t.Fatal("read-only owned release unproven", r.Error, r.ReleaseError)
	}
}
func TestM4MaintenanceTablesNativeCaptureUpgradeVerify(t *testing.T) {
	h := newMaintenanceNative(t)
	before, mongoBefore, bindingBefore := h.originalRaw(), h.mongoHead(), h.independentBinding()
	if r := h.action("inspect"); !r.Complete || r.MandatoryEOF {
		t.Fatal("metadata-only inspect failed or claimed EOF", r.Error)
	}
	h.capture()
	h.assertCaptureRaw(before)
	if !reflect.DeepEqual(before, h.originalRaw()) || !bytes.Equal(mongoBefore, h.mongoHead()) || bindingBefore != h.independentBinding() {
		t.Fatal("capture changed original facts or identity")
	}
	h.migrateSQL(103)
	if !reflect.DeepEqual(h.query("SELECT version,dirty FROM schema_migrations ORDER BY version"), [][]sql.NullString{{{String: "103", Valid: true}, {String: "0", Valid: true}}}) {
		t.Fatal("actual normal103 provider ledger differs")
	}
	r := h.verify()
	h.assertReadOnlyReleased(r)
	if !r.Complete || r.Error != "none" || r.MandatoryEOF || !r.SQLSnapshot {
		t.Fatal("formal target verification failed or claimed nonexistent row scan", r.Error)
	}
	if !bytes.Equal(mongoBefore, h.mongoHead()) || bindingBefore != h.independentBinding() {
		t.Fatal("unaffected Mongo original BSON/UUID changed")
	}
	if got := h.query("SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ('testee_store_migration_reversions','testee_store_migration_items','testee_store_migration_manifests','clinician_operator_binding_archive','operator_retirement_manifests')"); len(got) != 0 {
		t.Fatal("not all five original tables were dropped")
	}
	if r := h.action("restore"); r.Complete || r.Error != "restore_not_implemented" || r.SQLPoolClosed || r.MongoClosed {
		t.Fatal("unsupported restore crossed ENV/DB boundary")
	}
}
func TestM4MaintenanceTablesNativeBinaryPagesAndRawEOF(t *testing.T) {
	h := newMaintenanceNative(t)
	var values []string
	var args []any
	for i := 1; i <= 501; i++ {
		values = append(values, "(?,?,?,?,?,?,?,?,?,?)")
		args = append(args, "page", i, 1, nil, 0, 0, 0, nil, "deferred", "owned")
	}
	query := "INSERT INTO testee_store_migration_items(migration_id,testee_id,org_id,before_store_id,target_store_id,before_version,applied_version,history_id,disposition,reason) VALUES" + strings.Join(values, ",")
	if _, e := h.db.ExecContext(h.t.Context(), query, args...); e != nil {
		t.Fatal("owned page fixture insert failed")
	}
	before := h.originalRaw()
	h.capture()
	h.assertCaptureRaw(before)
	items := h.original.Tables[1]
	if len(items.Rows) != 507 || items.Pages != 3 || !items.EOF || items.Rows[0].Key[0] != "" || items.Rows[0].Key[1] != "0" || items.Rows[1].Key[1] != "18446744073709551615" || items.Rows[2].Key[0] != "41" || items.Rows[3].Key[0] != "5A" || items.Rows[4].Key[0] != "61" {
		t.Fatal("binary compound/0/MAX/cross-page EOF evidence incomplete")
	}
	if r := h.action("capture", "--capture-file="+h.materialPath); r.Complete || r.Error != "new_private_capture_required" {
		t.Fatal("original material was overwritten", r.Error)
	}
	if !reflect.DeepEqual(before, h.originalRaw()) {
		t.Fatal("read-only pagination changed values")
	}
}
func TestM4MaintenanceTablesNativeOversizeHashRefusesCapture(t *testing.T) {
	h := newMaintenanceNative(t)
	if _, e := h.db.ExecContext(h.t.Context(), "UPDATE operator_retirement_manifests SET payload=JSON_QUOTE(REPEAT('x',?))", maxValueBytes+1); e != nil {
		t.Fatal("owned valid oversized LONGTEXT unavailable")
	}
	before := h.query("SELECT OCTET_LENGTH(payload),payload_sha256 FROM operator_retirement_manifests ORDER BY migration_id")
	head := h.mongoHead()
	rows, e := h.db.QueryContext(h.t.Context(), pageSQL(4, false))
	if e != nil {
		t.Fatal("bounded real page query failed")
	}
	if !rows.Next() {
		_ = rows.Close()
		t.Fatal("oversized row unavailable")
	}
	cols, e := rows.Columns()
	if e != nil {
		_ = rows.Close()
		t.Fatal("bounded page columns unavailable")
	}
	raw := make([]any, len(cols))
	dest := make([]any, len(cols))
	for n := range dest {
		dest[n] = &raw[n]
	}
	scanErr := rows.Scan(dest...)
	extra := rows.Next()
	re, ce := rows.Err(), rows.Close()
	if scanErr != nil || extra || re != nil || ce != nil || raw[len(raw)-1] != nil {
		t.Fatal("over-limit real server hash was not NULL or cursor unreleased")
	}
	for n := range scope[4].columns {
		if raw[len(scope[4].keys)+len(scope[4].columns)+n] != nil {
			t.Fatal("over-limit value branch materialized bytes")
		}
	}
	path := filepath.Join(h.dir, "oversized.json")
	r := h.action("capture", "--capture-file="+path)
	h.assertReadOnlyReleased(r)
	if r.Complete || r.MaterialHash != "" || r.Error != "cell_value_limit_or_null_rejected" {
		t.Fatal("oversized capture was not explicitly refused", r.Error)
	}
	if _, e := os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("over-limit material was persisted")
	}
	if !reflect.DeepEqual(before, h.query("SELECT OCTET_LENGTH(payload),payload_sha256 FROM operator_retirement_manifests ORDER BY migration_id")) || !bytes.Equal(head, h.mongoHead()) {
		t.Fatal("oversize rejection changed original data/ledger")
	}
}
func TestM4MaintenanceTablesNativeUnboundMaterialRejected(t *testing.T) {
	h := newMaintenanceNative(t)
	h.capture()
	before, head := h.originalRaw(), h.mongoHead()
	for _, name := range []string{"hash", "source", "identity", "eof", "first_path"} {
		t.Run(name, func(t *testing.T) {
			m := cloneMaterial(h.original)
			path := filepath.Join(h.dir, "bad-"+name+".json")
			m.CapturePath = path
			switch name {
			case "source":
				m.DSource = strings.Repeat("f", 40)
			case "identity":
				m.Binding.MongoUUID = strings.Repeat("f", 64)
			case "eof":
				m.Tables[1].EOF = false
			case "first_path":
				m.CapturePath = h.materialPath
			}
			raw, e := json.Marshal(m)
			if e != nil || writeExclusive(path, raw) != nil {
				t.Fatal("owned invalid material unavailable")
			}
			hash := fingerprint(raw)
			if name == "hash" {
				hash = strings.Repeat("f", 64)
			}
			r := h.action("verify-target", "--material-file="+path, "--material-sha256="+hash)
			if r.Complete || r.SQLPoolClosed || r.MongoClosed || r.MaterialHash != "" {
				t.Fatal("invalid material reached connection input")
			}
		})
	}
	if !reflect.DeepEqual(before, h.originalRaw()) || !bytes.Equal(head, h.mongoHead()) {
		t.Fatal("input rejection changed original facts")
	}
	wrong := h.action("inspect", "--mongo-ledger-uuid-sha256="+strings.Repeat("f", 64))
	h.assertReadOnlyReleased(wrong)
	if wrong.Complete {
		t.Fatal("actual independent UUID mismatch accepted")
	}
}
func TestM4MaintenanceTablesNativeSchemaAndReappearanceRejected(t *testing.T) {
	for _, name := range []string{"primary_prefix", "nullable", "reappeared"} {
		t.Run(name, func(t *testing.T) {
			h := newMaintenanceNative(t)
			if name == "reappeared" {
				h.capture()
				h.migrateSQL(103)
				raw := h.sqlSource["000075_operator_retirement_manifests.up.sql"].Data
				first := strings.Split(string(raw), "CREATE TABLE clinician_operator_binding_archive")
				if len(first) != 2 {
					t.Fatal("fixed original075 resource unavailable")
				}
				if _, e := h.db.ExecContext(t.Context(), first[0]); e != nil {
					t.Fatal("owned original table reappearance unavailable")
				}
				r := h.verify()
				h.assertReadOnlyReleased(r)
				if r.Complete || r.Error != "fixed_table_presence_or_release_rejected" {
					t.Fatal("reappeared legacy object accepted", r.Error)
				}
				return
			}
			query := "ALTER TABLE testee_store_migration_items DROP PRIMARY KEY, ADD PRIMARY KEY(migration_id(8),testee_id)"
			if name == "nullable" {
				query = "ALTER TABLE testee_store_migration_items MODIFY before_version INT UNSIGNED NULL"
			}
			if _, e := h.db.ExecContext(t.Context(), query); e != nil {
				t.Fatal("owned schema drift unavailable")
			}
			path := filepath.Join(h.dir, "drift.json")
			r := h.action("capture", "--capture-file="+path)
			h.assertReadOnlyReleased(r)
			if r.Complete || r.MaterialHash != "" {
				t.Fatal("drifted fixed schema accepted")
			}
			if _, e := os.Stat(path); !os.IsNotExist(e) {
				t.Fatal("drift material was persisted")
			}
		})
	}
}
