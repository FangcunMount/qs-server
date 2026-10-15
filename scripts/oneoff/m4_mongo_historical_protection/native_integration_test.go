//go:build integration

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
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
)

type nativeFixture struct {
	t                                                     *testing.T
	name, dsn, uri, tool, dir, materialPath, materialHash string
	root, db                                              *sql.DB
	client                                                *mongo.Client
	original                                              material
}

func nativeSources(t *testing.T, backend string, through int) fstest.MapFS {
	t.Helper()
	root := "../../../internal/pkg/migration/migrations/" + backend
	entries, e := os.ReadDir(root)
	if e != nil {
		t.Fatal(e)
	}
	result := fstest.MapFS{}
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
		result[name] = &fstest.MapFile{Data: raw, Mode: 0444}
	}
	return result
}
func checkNativeBResources(t *testing.T) {
	t.Helper()
	var inventory [][3]string
	for _, item := range []struct {
		backend string
		through int
	}{{"mysql", 101}, {"mongodb", 40}} {
		source := nativeSources(t, item.backend, item.through)
		names := make([]string, 0, len(source))
		for name := range source {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			inventory = append(inventory, [3]string{item.backend, "migrations/" + item.backend + "/" + name, fingerprint(source[name].Data)})
		}
	}
	raw, _ := json.Marshal(inventory)
	if len(inventory) != bResourceFiles || fingerprint(raw) != bResources {
		t.Fatal("fixture does not contain the exact frozen282-file B resource bundle")
	}
}
func newNative(t *testing.T) *nativeFixture {
	t.Helper()
	if os.Getenv("QS_M4_OBJECT_NATIVE_REQUIRED") != "1" {
		t.Skip("owned native pair not selected")
	}
	cfg, e := mysql.ParseDSN(os.Getenv("QS_M4_OBJECT_NATIVE_MYSQL_DSN"))
	if e != nil || cfg.Net != "tcp" {
		t.Fatal("owned MySQL input required")
	}
	host, port, e := net.SplitHostPort(cfg.Addr)
	if e != nil || (host != "127.0.0.1" && host != "localhost") || port == "3306" {
		t.Fatal("owned random MySQL loopback required")
	}
	uri := os.Getenv("QS_M4_OBJECT_NATIVE_MONGO_URI")
	u, e := url.Parse(uri)
	if e != nil || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") || u.Port() == "" || u.Port() == "27017" || u.Query().Get("replicaSet") != "rs0" || u.Query().Get("directConnection") != "true" {
		t.Fatal("owned direct loopback rs0 required")
	}
	if os.Getenv("QS_M4_OBJECT_NATIVE_DISPOSABLE_ACK") != "owned_synthetic_dependencies_only" || !sourceRE.MatchString(version.GitCommit) {
		t.Fatal("owned fixture acknowledgment and actual compiled source required")
	}
	checkNativeBResources(t)
	cfg.MultiStatements = true
	cfg.ParseTime = false
	root, e := sql.Open("mysql", cfg.FormatDSN())
	if e != nil {
		t.Fatal(e)
	}
	root.SetMaxOpenConns(1)
	h := &nativeFixture{t: t, name: fmt.Sprintf("qs_m4_mongo_object_%d", time.Now().UnixNano()), root: root, uri: uri, tool: version.GitCommit, dir: privateDir(t)}
	t.Cleanup(func() {
		q, c := context.WithTimeout(context.Background(), 30*time.Second)
		defer c()
		if h.client != nil {
			if e := h.client.Database(h.name).Drop(q); e != nil {
				t.Error("owned Mongo namespace cleanup failed")
			}
			if h.client.Disconnect(q) != nil {
				t.Error("owned fixture Mongo client release failed")
			}
		}
		if h.db != nil && h.db.Close() != nil {
			t.Error("owned fixture SQL pool release failed")
		}
		if _, e := root.ExecContext(q, "DROP DATABASE IF EXISTS `"+h.name+"`"); e != nil {
			t.Error("owned SQL namespace cleanup failed")
		}
		if root.Close() != nil {
			t.Error("owned root pool release failed")
		}
	})
	var actual string
	if root.QueryRowContext(t.Context(), "SELECT VERSION()").Scan(&actual) != nil || actual != "8.0.36" {
		t.Fatal("actual MySQL8.0.36 required")
	}
	if _, e = root.ExecContext(t.Context(), "CREATE DATABASE `"+h.name+"` CHARACTER SET utf8mb4"); e != nil {
		t.Fatal(e)
	}
	cfg.DBName = h.name
	h.dsn = cfg.FormatDSN()
	h.db, e = sql.Open("mysql", h.dsn)
	if e != nil {
		t.Fatal(e)
	}
	h.db.SetMaxOpenConns(1)
	h.migrateSQL(nativeSources(t, "mysql", 101), 101)
	h.client, e = mongo.Connect(t.Context(), options.Client().ApplyURI(uri).SetMaxPoolSize(1).SetTimeout(queryLimit))
	if e != nil {
		t.Fatal(e)
	}
	var build struct {
		Version string `bson:"version"`
	}
	var hello struct {
		Set     string `bson:"setName"`
		Primary bool   `bson:"isWritablePrimary"`
	}
	if h.client.Database("admin").RunCommand(t.Context(), bson.D{{Key: "buildInfo", Value: 1}}).Decode(&build) != nil || build.Version != "7.0.31" || h.client.Database("admin").RunCommand(t.Context(), bson.D{{Key: "hello", Value: 1}}).Decode(&hello) != nil || hello.Set != "rs0" || !hello.Primary {
		t.Fatal("actual primary Mongo7.0.31/rs0 required")
	}
	h.migrateMongo(nativeSources(t, "mongodb", 39), 39, false)
	for _, collection := range []string{"answersheets", "report_generations"} {
		slot := "legacy_submission_evidence"
		if collection == "report_generations" {
			slot = "historical_generated_evidence"
		}
		for i := int64(1); i <= 2; i++ {
			_, e = h.client.Database(h.name).Collection(collection).InsertOne(t.Context(), bson.D{
				{Key: "_id", Value: fmt.Sprintf("owned-%s-%d", collection, i)}, {Key: "domain_id", Value: 880000 + i}, {Key: "outcome_id", Value: 880000 + i}, {Key: "report_type", Value: "assessment"}, {Key: "template_version", Value: "3"},
				{Key: "generated_event_id", Value: fmt.Sprintf("current-generated-%d", i)}, {Key: "durable_acceptance", Value: bson.D{{Key: "event_id", Value: fmt.Sprintf("current-submission-%d", i)}, {Key: "schema_version", Value: int32(1)}}},
				{Key: "report_input", Value: bson.D{{Key: "schema_version", Value: int32(3)}, {Key: "InterpretationAssets", Value: bson.D{{Key: "frozen", Value: "synthetic report 🙂"}}}}},
				{Key: slot, Value: bson.D{{Key: "version", Value: int32(1)}, {Key: "entries", Value: bson.A{bson.D{{Key: "event_id", Value: fmt.Sprintf("owned-history-%d", i)}}}}}},
			})
			if e != nil {
				t.Fatal(e)
			}
		}
	}
	if _, e = h.client.Database(h.name).Collection("rm_outbox").InsertOne(t.Context(), bson.D{{Key: "_id", Value: "owned-retained-responsibility"}, {Key: "message_id", Value: "owned-retained-responsibility"}, {Key: "payload", Value: bson.D{{Key: "frozen", Value: true}}}}); e != nil {
		t.Fatal(e)
	}
	t.Logf("actual MySQL=%s Mongo=%s rs0; source=%s; owned namespace=%s", actual, build.Version, h.tool, h.name)
	return h
}
func (h *nativeFixture) migrateSQL(source fs.FS, target uint) {
	h.t.Helper()
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
		h.t.Fatal("SQL provider release failed")
	}
}
func (h *nativeFixture) migrateMongo(source fs.FS, target uint, wantFailure bool) {
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
	before, dirty, e := dd.Version()
	if e != nil || dirty || before < migrate.NilVersion {
		h.t.Fatal("real provider current version required before normal preparation")
	}
	versionBefore := uint(0)
	if before != migrate.NilVersion {
		versionBefore = uint(before)
	}
	// Initial preparation sees0; installed39 must not recreate namespaces that
	// historical retirement migrations already dropped.
	cleanup, e := qsMigration.NewMongoDriver(client).PrepareRun(h.t.Context(), &qsMigration.Config{Database: h.name}, versionBefore)
	if e != nil {
		h.t.Fatal(e)
	}
	m, e := migrate.NewWithInstance("iofs", sd, "mongodb", dd)
	if e != nil {
		h.t.Fatal(e)
	}
	e = m.Migrate(target)
	ce := cleanup(h.t.Context())
	se, de := m.Close()
	if (e != nil) != wantFailure && e != migrate.ErrNoChange {
		h.t.Fatal("unexpected Mongo provider outcome", e)
	}
	if wantFailure && e == nil {
		h.t.Fatal("fault migration unexpectedly succeeded")
	}
	if ce != nil || se != nil || de != nil {
		h.t.Fatal("Mongo provider release failed")
	}
}
func (h *nativeFixture) upgrade(prefix int, clean bool) {
	h.t.Helper()
	source := nativeSources(h.t, "mongodb", 40)
	if !clean {
		name := "000040_retire_historical_event_indexes.up.json"
		var commands []json.RawMessage
		if json.Unmarshal(source[name].Data, &commands) != nil || len(commands) != 4 || prefix < 0 || prefix > 4 {
			h.t.Fatal("fixed original40 command prefix unavailable")
		}
		fault := append([]json.RawMessage(nil), commands[:prefix]...)
		fault = append(fault, json.RawMessage(`{"dropIndexes":"answersheets","index":"owned_native_missing_index"}`))
		raw, e := json.Marshal(fault)
		if e != nil {
			h.t.Fatal(e)
		}
		source[name] = &fstest.MapFile{Data: raw, Mode: 0444}
	}
	h.migrateMongo(source, 40, !clean)
	raw, _ := h.head()
	v, dirty, e := ledgerVersion(raw)
	if e != nil || v != 40 || dirty == clean {
		h.t.Fatal("real provider40 dirty/clean state unavailable")
	}
	missing := 0
	for _, ns := range h.catalog() {
		for _, target := range targets {
			if target.collection == ns.Name {
				if _, ok := indexSpecs(ns)[target.name]; !ok {
					missing++
				}
			}
		}
	}
	if missing != prefix {
		h.t.Fatal("real40 provider did not execute the exact original DROP prefix", missing, prefix)
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
func requirePhysicalReleased(t *testing.T, r receipt) {
	t.Helper()
	if r.SQLConnectionClosed == nil || !*r.SQLConnectionClosed || r.SQLPoolClosed == nil || !*r.SQLPoolClosed || r.MongoClosed == nil || !*r.MongoClosed || r.MongoCursorsClosed == nil || !*r.MongoCursorsClosed || r.TransactionReleaseUnproven {
		t.Fatal("owned physical resources not released", r)
	}
}
func requireReleased(t *testing.T, r receipt) {
	t.Helper()
	requirePhysicalReleased(t, r)
	if r.ReleaseError != "" || r.MongoTransactionReleaseUnproven {
		t.Fatal("transaction release not proven", r)
	}
}

func (h *nativeFixture) capture() {
	h.t.Helper()
	h.materialPath = filepath.Join(h.dir, "first-capture.json")
	r := h.action("capture", "--capture-file="+h.materialPath)
	requireReleased(h.t, r)
	if !r.Complete || !r.ReadOnly || r.FreezeCertified || r.MaterialHash == "" {
		h.t.Fatal("capture incomplete", r)
	}
	h.materialHash = r.MaterialHash
	raw, e := readPrivate(h.materialPath, h.materialHash, maxMaterialBytes)
	if e != nil || strictDecode(raw, &h.original) != nil || validateMaterial(h.original, h.tool, h.name, h.name) != nil {
		h.t.Fatal("first genuine material unavailable")
	}
}
func (h *nativeFixture) boundInput() []string {
	return []string{"--material-file=" + h.materialPath, "--material-sha256=" + h.materialHash}
}
func (h *nativeFixture) approval() ([]string, string, string) {
	h.t.Helper()
	now := time.Now().UTC()
	captured, _ := time.Parse(time.RFC3339Nano, h.original.CapturedAt)
	a := approval{Format: 1, Kind: "m4_b_pre_start_mongo_object_restore", Operation: fmt.Sprintf("%d-1", now.UnixNano()), ToolSource: h.tool, BSource: bSource, BResources: bResources, BResourceFiles: bResourceFiles, MaterialHash: h.materialHash, FreezeHash: strings.Repeat("a", 64), ResponsibilityHash: strings.Repeat("b", 64), SQLIdentity: h.original.SQLIdentity, MongoIdentity: h.original.MongoIdentity, RuntimeFrozen: true, RuntimeNotStarted: true, Marker: filepath.Join(h.dir, "first-runtime.marker"), WindowStarted: captured.Add(-time.Minute).Format(time.RFC3339Nano), RecoveryDecision: now.Format(time.RFC3339Nano), Deadline: now.Add(time.Minute).Format(time.RFC3339Nano), Expires: now.Add(2 * time.Minute).Format(time.RFC3339Nano), RetentionDays: 30, DataOwner: "system (10001)", RecoveryOwner: "system (10001)"}
	raw, _ := json.Marshal(a)
	path := filepath.Join(h.dir, a.Operation+"-approval.json")
	if e := writeExclusive(path, raw); e != nil {
		h.t.Fatal(e)
	}
	return append(h.boundInput(), "--approval-file="+path, "--approval-sha256="+fingerprint(raw)), a.Marker, path
}
func (h *nativeFixture) head() ([]byte, []byte) {
	h.t.Helper()
	var raw bson.Raw
	if h.client.Database(h.name).Collection("schema_migrations").FindOne(h.t.Context(), bson.D{}).Decode(&raw) != nil {
		h.t.Fatal("independent real ledger read failed")
	}
	cur, e := h.client.Database(h.name).ListCollections(h.t.Context(), bson.D{{Key: "name", Value: "schema_migrations"}})
	if e != nil {
		h.t.Fatal(e)
	}
	var rows []bson.Raw
	if cur.All(h.t.Context(), &rows) != nil || len(rows) != 1 {
		h.t.Fatal("independent ledger UUID read failed")
	}
	_, uuid, ok := rows[0].Lookup("info", "uuid").BinaryOK()
	if !ok {
		h.t.Fatal("UUID missing")
	}
	return append([]byte(nil), raw...), append([]byte(nil), uuid...)
}
func (h *nativeFixture) catalog() [3]namespaceProof {
	h.t.Helper()
	d := database{mongo: h.client, mongoName: h.name}
	out, e := d.catalog(h.t.Context())
	if e != nil || d.cursorReleaseUnproven {
		h.t.Fatal("independent actual catalog failed", e)
	}
	return out
}

// Full raw synthetic BSON is read only by this isolated test, never the tool.
func (h *nativeFixture) frozenDocuments() map[string][]bson.Raw {
	h.t.Helper()
	db := h.client.Database(h.name)
	names, e := db.ListCollectionNames(h.t.Context(), bson.D{})
	if e != nil {
		h.t.Fatal(e)
	}
	out := map[string][]bson.Raw{}
	for _, name := range names {
		if name == "schema_migrations" {
			continue
		}
		cur, e := db.Collection(name).Find(h.t.Context(), bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
		if e != nil {
			h.t.Fatal(e)
		}
		var rows []bson.Raw
		if cur.All(h.t.Context(), &rows) != nil {
			h.t.Fatal("full synthetic BSON witness failed")
		}
		out[name] = rows
	}
	return out
}
func (h *nativeFixture) sqlLedger() string {
	h.t.Helper()
	rows, e := h.db.QueryContext(h.t.Context(), "SELECT version,dirty FROM schema_migrations ORDER BY version")
	if e != nil {
		h.t.Fatal(e)
	}
	var result []string
	for rows.Next() {
		var v uint64
		var dirty bool
		if rows.Scan(&v, &dirty) != nil {
			h.t.Fatal("SQL ledger read failed")
		}
		result = append(result, fmt.Sprintf("%d/%t", v, dirty))
	}
	re, ce := rows.Err(), rows.Close()
	if re != nil || ce != nil || len(result) != 1 {
		h.t.Fatal("SQL ledger complete read/release failed")
	}
	return strings.Join(result, ",")
}

// Independent native witness covers all business namespace metadata and every
// unrelated index, not only the two fixed target collections. Ledger UUID is
// checked separately because the real provider legitimately recreates it.
func (h *nativeFixture) retainedMetadata() map[string][]byte {
	h.t.Helper()
	db := h.client.Database(h.name)
	cur, e := db.ListCollections(h.t.Context(), bson.D{})
	if e != nil {
		h.t.Fatal(e)
	}
	var catalogs []bson.Raw
	if cur.All(h.t.Context(), &catalogs) != nil {
		h.t.Fatal("independent full namespace inventory failed")
	}
	result := map[string][]byte{}
	for _, raw := range catalogs {
		name := raw.Lookup("name").StringValue()
		if name == "schema_migrations" {
			continue
		}
		result["catalog/"+name] = append([]byte(nil), raw...)
	}
	for name, raw := range h.independentIndexes(false) {
		result["index/"+name] = raw
	}
	return result
}
func (h *nativeFixture) independentIndexes(includeTargets bool) map[string][]byte {
	h.t.Helper()
	db := h.client.Database(h.name)
	names, e := db.ListCollectionNames(h.t.Context(), bson.D{})
	if e != nil {
		h.t.Fatal(e)
	}
	result := map[string][]byte{}
	for _, name := range names {
		if name == "schema_migrations" {
			continue
		}
		cur, e := db.Collection(name).Indexes().List(h.t.Context())
		if e != nil {
			h.t.Fatal(e)
		}
		var specs []bson.Raw
		if cur.All(h.t.Context(), &specs) != nil {
			h.t.Fatal("independent full index inventory failed")
		}
		for _, raw := range specs {
			n := raw.Lookup("name").StringValue()
			if includeTargets || targetPosition(name, n) < 0 {
				result[name+"/"+n] = append([]byte(nil), raw...)
			}
		}
	}
	return result
}
func assertMetadata(t *testing.T, before, after map[string][]byte) {
	t.Helper()
	if !reflect.DeepEqual(before, after) {
		t.Fatal("business UUID/options/unrelated indexes changed")
	}
}
func (h *nativeFixture) assertOriginalTargetSpecs() {
	h.t.Helper()
	indexes := h.independentIndexes(true)
	for i, target := range targets {
		if !bytes.Equal(indexes[target.collection+"/"+target.name], targetSpec(h.t, i)) {
			h.t.Fatal("actual original target index spec not restored byte for byte")
		}
	}
}

func assertDocuments(t *testing.T, before, after map[string][]bson.Raw) {
	t.Helper()
	if !reflect.DeepEqual(before, after) {
		t.Fatal("tool changed retained full synthetic business BSON")
	}
}

func TestM4MongoObjectNativeCaptureUpgradeVerify(t *testing.T) {
	h := newNative(t)
	docs := h.frozenDocuments()
	metadata := h.retainedMetadata()
	sql := h.sqlLedger()
	r := h.action("inspect")
	if !r.Complete {
		t.Fatal(r)
	}
	h.capture()
	h.upgrade(4, true)
	r = h.action("verify-target", h.boundInput()...)
	requireReleased(t, r)
	if !r.Complete || r.MongoVersion != 40 || !r.ReadOnly {
		t.Fatal(r)
	}
	if h.action("inspect").Complete {
		t.Fatal("clean40 treated as original39")
	}
	assertDocuments(t, docs, h.frozenDocuments())
	assertMetadata(t, metadata, h.retainedMetadata())
	if h.sqlLedger() != sql {
		t.Fatal("SQL ledger changed")
	}
	if _, e := readPrivate(h.materialPath, h.materialHash, maxMaterialBytes); e != nil {
		t.Fatal("original material changed")
	}
}
func TestM4MongoObjectNativeRestoreMatrix(t *testing.T) {
	cases := []struct {
		name   string
		prefix int
		clean  bool
	}{{"DirtyPrefix0", 0, false}, {"DirtyPrefix1", 1, false}, {"DirtyPrefix2", 2, false}, {"DirtyPrefix3", 3, false}, {"DirtyPrefix4", 4, false}, {"CleanAll4", 4, true}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newNative(t)
			docs := h.frozenDocuments()
			metadata := h.retainedMetadata()
			sql := h.sqlLedger()
			originalIndexes := h.independentIndexes(true)
			h.capture()
			h.upgrade(tc.prefix, tc.clean)
			_, currentUUID := h.head()
			if bytes.Equal(currentUUID, h.original.Ledger.Namespace.UUID) {
				t.Fatal("provider40 did not change ledger UUID")
			}
			args, _, _ := h.approval()
			r := h.action("restore", args...)
			requireReleased(t, r)
			if !r.Complete || !r.LedgerCommitIssued || !r.LedgerCommitAcknowledged || r.LedgerCommitOutcomeUnknown || r.MongoTransactionReleaseUnproven {
				t.Fatal("restore incomplete", r)
			}
			raw, uuid := h.head()
			if !bytes.Equal(raw, h.original.Ledger.Document) || !bytes.Equal(uuid, currentUUID) {
				t.Fatal("original BSON or provider40 UUID lost")
			}
			o := observation{Collections: [2]namespaceProof{h.catalog()[0], h.catalog()[1]}}
			for i := range o.Collections {
				n := int64(len(docs[o.Collections[i].Name]))
				o.Collections[i].Count = &n
			}
			if !sameBusinessNamespaces(h.original, o) {
				t.Fatal("four specs/UUID/options/unrelated indexes not restored exactly")
			}
			if !reflect.DeepEqual(originalIndexes, h.independentIndexes(true)) {
				t.Fatal("complete original index inventory changed")
			}
			assertDocuments(t, docs, h.frozenDocuments())
			assertMetadata(t, metadata, h.retainedMetadata())
			if h.sqlLedger() != sql {
				t.Fatal("SQL ledger changed")
			}
			fresh := h.action("inspect")
			requireReleased(t, fresh)
			if !fresh.Complete || fresh.MongoVersion != 39 {
				t.Fatal("fresh10139 guard failed", fresh)
			}
		})
	}
}
func TestM4MongoObjectNativeIndexConflictKeepsDirty(t *testing.T) {
	h := newNative(t)
	h.capture()
	h.upgrade(2, false)
	// Deliberately violate external freeze before restore: exact counts stay the
	// same while legacy unique values collide. Never delete a business document
	// to make index recreation pass. Tool scope cannot certify unchanged bodies.
	_, e := h.client.Database(h.name).Collection("answersheets").UpdateOne(t.Context(), bson.D{{Key: "_id", Value: "owned-answersheets-2"}}, bson.D{{Key: "$set", Value: bson.D{{Key: "legacy_submission_evidence.entries.0.event_id", Value: "owned-history-1"}}}})
	if e != nil {
		t.Fatal(e)
	}
	docs := h.frozenDocuments()
	metadata := h.retainedMetadata()
	head, uuid := h.head()
	sql := h.sqlLedger()
	args, _, _ := h.approval()
	r := h.action("restore", args...)
	requireReleased(t, r)
	if r.Complete || r.Error != "index_restore_failed_or_unknown" || r.LedgerCommitIssued {
		t.Fatal("unique conflict advanced ledger", r)
	}
	after, afterUUID := h.head()
	if !bytes.Equal(head, after) || !bytes.Equal(uuid, afterUUID) {
		t.Fatal("failed unique rebuild changed dirty ledger")
	}
	assertDocuments(t, docs, h.frozenDocuments())
	assertMetadata(t, metadata, h.retainedMetadata())
	if h.sqlLedger() != sql {
		t.Fatal("SQL ledger changed")
	}
}
func TestM4MongoObjectNativePreclaimBoundaries(t *testing.T) {
	for _, kind := range []string{"FirstRuntimeMarker", "ChangedExternalMaterial", "ExpiredApproval", "RepeatedUse", "FutureSQL102"} {
		t.Run(kind, func(t *testing.T) {
			h := newNative(t)
			h.capture()
			h.upgrade(2, false)
			args, marker, path := h.approval()
			switch kind {
			case "FirstRuntimeMarker":
				if writeExclusive(marker, []byte("start intent issued")) != nil {
					t.Fatal("marker setup")
				}
			case "ChangedExternalMaterial":
				if os.WriteFile(h.materialPath, []byte(`{"changed":true}`), 0600) != nil {
					t.Fatal("material fault")
				}
			case "ExpiredApproval":
				raw, e := readPrivate(path, fingerprintMustRead(t, path), 32768)
				var a approval
				if e != nil || strictDecode(raw, &a) != nil {
					t.Fatal("approval fault input")
				}
				a.Expires = time.Now().Add(-time.Second).Format(time.RFC3339Nano)
				raw, _ = json.Marshal(a)
				if os.WriteFile(path, raw, 0600) != nil {
					t.Fatal("approval fault")
				}
				args[len(args)-1] = "--approval-sha256=" + fingerprint(raw)
			case "RepeatedUse":
				r := h.action("restore", args...)
				if !r.Complete {
					t.Fatal("first actual approval use failed", r)
				}
				if _, e := os.Stat(path + ".consumed"); e != nil {
					t.Fatal("actual claim missing")
				}
			case "FutureSQL102":
				h.migrateSQL(nativeSources(t, "mysql", 102), 102)
			}
			docs := h.frozenDocuments()
			metadata := h.retainedMetadata()
			head, uuid := h.head()
			catalog := h.catalog()
			sql := h.sqlLedger()
			r := h.action("restore", args...)
			if r.Complete || r.LedgerCommitIssued {
				t.Fatal("unapproved restore completed", r)
			}
			after, afterUUID := h.head()
			if !bytes.Equal(head, after) || !bytes.Equal(uuid, afterUUID) || !catalogEqual(catalog, h.catalog()) || h.sqlLedger() != sql {
				t.Fatal("preclaim rejection changed protected state")
			}
			assertDocuments(t, docs, h.frozenDocuments())
			assertMetadata(t, metadata, h.retainedMetadata())
			if kind != "RepeatedUse" {
				if _, e := os.Stat(path + ".consumed"); !os.IsNotExist(e) {
					t.Fatal("preclaim failure consumed approval")
				}
			}
		})
	}
}
func fingerprintMustRead(t *testing.T, path string) string {
	t.Helper()
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	return fingerprint(raw)
}
