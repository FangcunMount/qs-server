//go:build integration

package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	qsMigration "github.com/FangcunMount/qs-server/internal/pkg/migration"
	"github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	migratemongo "github.com/golang-migrate/migrate/v4/database/mongodb"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// These tests exercise the real three-leaf engine, not an implemented whole
// cohort graph or a production execution approval. Excluded roots are real
// provider-created objects, and must remain byte-for-byte unchanged.
type cohortNative struct {
	t                    *testing.T
	name                 string
	root                 *sql.DB
	db                   *sql.DB
	client               *mongo.Client
	store                *liveStore
	binding              binding
	sqlOwned, mongoOwned bool
}

func cohortNativeSources(t *testing.T, backend string, through int) fs.FS {
	t.Helper()
	root := filepath.Join("../../../internal/pkg/migration/migrations", backend)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	out := fstest.MapFS{}
	for _, entry := range entries {
		name := entry.Name()
		if len(name) < 7 || (!strings.Contains(name, ".up.") && !strings.Contains(name, ".down.")) {
			continue
		}
		n, err := strconv.Atoi(name[:6])
		if err != nil || n > through {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		out[name] = &fstest.MapFile{Data: raw, Mode: 0444}
	}
	if len(out) == 0 {
		t.Fatal("original migration resources missing")
	}
	return out
}

func newCohortNative(t *testing.T) *cohortNative {
	t.Helper()
	if os.Getenv("QS_MOCK_NATIVE_REQUIRED") != "1" || os.Getenv("QS_MOCK_NATIVE_DISPOSABLE_ACK") != "owned_synthetic_dependencies_only" {
		t.Fatal("mandatory owned native acknowledgment is required; this lane never skips")
	}
	if !regexpSource(toolSource) {
		t.Fatal("exact compiled main.toolSource is required")
	}
	cfg, err := mysql.ParseDSN(os.Getenv("QS_MOCK_NATIVE_MYSQL_DSN"))
	if err != nil || cfg.Net != "tcp" || cfg.DBName != "" {
		t.Fatal("database-free owned MySQL input required")
	}
	host, port, err := net.SplitHostPort(cfg.Addr)
	if err != nil || host != "127.0.0.1" || port == "3306" || port == "" {
		t.Fatal("owned non-default MySQL loopback required")
	}
	uri := os.Getenv("QS_MOCK_NATIVE_MONGO_URI")
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "mongodb" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.Port() == "27017" || u.Query().Get("replicaSet") == "" || u.Query().Get("directConnection") != "true" {
		t.Fatal("owned direct primary replica-set loopback required")
	}
	cfg.MultiStatements, cfg.ParseTime = true, false
	cfg.Timeout, cfg.ReadTimeout, cfg.WriteTimeout = 5*time.Second, 15*time.Second, 15*time.Second
	root, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("owned SQL open failed")
	}
	root.SetMaxOpenConns(1)
	h := &cohortNative{t: t, name: fmt.Sprintf("qs_mock_cohort_%d", time.Now().UnixNano()), root: root}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if h.store != nil && h.store.sql != nil {
			if err := h.store.sql.Close(); err != nil {
				t.Error("owned SQL connection close failed")
			}
		}
		if h.db != nil {
			if err := h.db.Close(); err != nil {
				t.Error("owned SQL pool close failed")
			}
		}
		if h.client != nil {
			if h.mongoOwned {
				if err := h.client.Database(h.name).Drop(ctx); err != nil {
					t.Error("exact owned Mongo namespace cleanup failed")
				}
			}
			if err := h.client.Disconnect(ctx); err != nil {
				t.Error("owned Mongo client close failed")
			}
		}
		if h.sqlOwned {
			if _, err := h.root.ExecContext(ctx, "DROP DATABASE `"+h.name+"`"); err != nil {
				t.Error("exact owned SQL namespace cleanup failed")
			}
		}
		if err := h.root.Close(); err != nil {
			t.Error("owned root pool close failed")
		}
	})
	var sqlVersion string
	if root.QueryRowContext(t.Context(), "SELECT VERSION()").Scan(&sqlVersion) != nil || sqlVersion != "8.0.36" {
		t.Fatal("actual MySQL8.0.36 required")
	}
	if _, err = root.ExecContext(t.Context(), "CREATE DATABASE `"+h.name+"` CHARACTER SET utf8mb4"); err != nil {
		t.Fatal("owned SQL namespace creation failed")
	}
	h.sqlOwned = true
	cfg.DBName = h.name
	dsn := cfg.FormatDSN()
	h.db, err = sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal("owned SQL database open failed")
	}
	h.db.SetMaxOpenConns(1)
	h.migrateSQL(dsn)
	h.client, err = mongo.Connect(t.Context(), options.Client().ApplyURI(uri).SetReadPreference(readpref.Primary()).SetMaxPoolSize(1).SetRetryReads(false).SetRetryWrites(false).SetTimeout(10*time.Second))
	if err != nil {
		t.Fatal("owned Mongo connect failed")
	}
	var build struct {
		Version string `bson:"version"`
	}
	var hello struct {
		Set     string   `bson:"setName"`
		Hosts   []string `bson:"hosts"`
		Primary bool     `bson:"isWritablePrimary"`
		Msg     string   `bson:"msg"`
	}
	if h.client.Database("admin").RunCommand(t.Context(), bson.D{{Key: "buildInfo", Value: 1}}).Decode(&build) != nil || build.Version != "7.0.31" {
		t.Fatal("actual Mongo7.0.31 required")
	}
	if h.client.Database("admin").RunCommand(t.Context(), bson.D{{Key: "hello", Value: 1}}).Decode(&hello) != nil || hello.Set != u.Query().Get("replicaSet") || !hello.Primary || hello.Msg != "" || len(hello.Hosts) == 0 {
		t.Fatal("actual owned primary role required")
	}
	names, err := h.client.ListDatabaseNames(t.Context(), bson.D{{Key: "name", Value: h.name}})
	if err != nil || len(names) != 0 {
		t.Fatal("fresh Mongo namespace ownership precheck failed")
	}
	if err = h.client.Database(h.name).CreateCollection(t.Context(), "_qs_mock_owner"); err != nil {
		t.Fatal("fresh Mongo namespace ownership claim failed")
	}
	h.mongoOwned = true
	h.migrateMongo(uri)
	conn, err := h.db.Conn(t.Context())
	if err != nil {
		t.Fatal("owned dedicated SQL connection failed")
	}
	h.store = &liveStore{sql: conn, mongo: h.client, mongoDB: h.name}
	var uuid string
	if conn.QueryRowContext(t.Context(), "SELECT @@server_uuid").Scan(&uuid) != nil {
		t.Fatal("SQL UUID read failed")
	}
	sort.Strings(hello.Hosts)
	h.binding = binding{SQLUUID: uuid, SQLDatabase: h.name, MongoDatabase: h.name, SQLVersion: 101, MongoVersion: 39, MongoIdentity: digest(encode([]any{"mongodb_cluster_selected_v1", bson.D{{Key: "setName", Value: hello.Set}, {Key: "hosts", Value: hello.Hosts}}, h.name}))}
	if err = h.store.identity(t.Context(), h.binding); err != nil {
		t.Fatal("real provider clean101/39 identity failed:", err)
	}
	t.Logf("actual source=%s MySQL=%s Mongo=%s replica=%s namespace=%s primitive_only=true whole_cohort_complete=false", toolSource, sqlVersion, build.Version, hello.Set, h.name)
	return h
}

func regexpSource(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			if c < 'a' || c > 'f' {
				return false
			}
		}
	}
	return true
}

func (h *cohortNative) migrateSQL(dsn string) {
	h.t.Helper()
	source, err := iofs.New(cohortNativeSources(h.t, "mysql", 101), ".")
	if err != nil {
		h.t.Fatal(err)
	}
	pool, err := sql.Open("mysql", dsn)
	if err != nil {
		h.t.Fatal("provider SQL pool open failed")
	}
	driver, err := migratemysql.WithInstance(pool, &migratemysql.Config{DatabaseName: h.name})
	if err != nil {
		_ = pool.Close()
		h.t.Fatal("original SQL driver initialization failed")
	}
	m, err := migrate.NewWithInstance("iofs", source, "mysql", driver)
	if err != nil {
		_, _ = driver.Close(), source.Close()
		h.t.Fatal("original SQL provider initialization failed")
	}
	err = m.Migrate(101)
	se, de := m.Close()
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		h.t.Fatal("original SQL migration to101 failed:", err)
	}
	if se != nil || de != nil {
		h.t.Fatal("SQL provider release failed")
	}
}

func (h *cohortNative) migrateMongo(uri string) {
	h.t.Helper()
	source, err := iofs.New(cohortNativeSources(h.t, "mongodb", 39), ".")
	if err != nil {
		h.t.Fatal(err)
	}
	client, err := mongo.Connect(h.t.Context(), options.Client().ApplyURI(uri))
	if err != nil {
		h.t.Fatal("Mongo provider connect failed")
	}
	driver, err := migratemongo.WithInstance(client, &migratemongo.Config{DatabaseName: h.name, MigrationsCollection: "schema_migrations", TransactionMode: false})
	if err != nil {
		_ = client.Disconnect(h.t.Context())
		h.t.Fatal("Mongo provider driver failed")
	}
	cleanup, err := qsMigration.NewMongoDriver(client).PrepareRun(h.t.Context(), &qsMigration.Config{Database: h.name}, 0)
	if err != nil {
		_ = driver.Close()
		h.t.Fatal("original Mongo namespace preparation failed")
	}
	m, err := migrate.NewWithInstance("iofs", source, "mongodb", driver)
	if err != nil {
		_ = driver.Close()
		h.t.Fatal("original Mongo provider initialization failed")
	}
	err = m.Migrate(39)
	ce := cleanup(h.t.Context())
	se, de := m.Close()
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		h.t.Fatal("original Mongo migration to39 failed:", err)
	}
	if ce != nil || se != nil || de != nil {
		h.t.Fatal("Mongo provider release failed")
	}
}

func (h *cohortNative) exec(query string, args ...any) {
	h.t.Helper()
	if _, err := h.store.sql.ExecContext(h.t.Context(), query, args...); err != nil {
		h.t.Fatal("fixed owned fixture SQL failed:", err)
	}
}
func nativeMongoObject(id any) object {
	raw, err := bson.Marshal(bson.D{{Key: "_id", Value: id}})
	if err != nil {
		panic(err)
	}
	v := bson.Raw(raw).Lookup("_id")
	return object{Store: "mongo", Name: "report_query_catalog", Key: []cell{{Name: "_id", Type: fmt.Sprintf("bson:%02x", byte(v.Type)), Raw: append([]byte{}, v.Value...)}}}
}
func (h *cohortNative) mustRead(o object) record {
	h.t.Helper()
	r, ok, err := h.store.read(h.t.Context(), o)
	if err != nil || !ok {
		h.t.Fatal("owned exact object read failed:", o.Store, o.Name, err)
	}
	return r
}
func (h *cohortNative) seeded() (group, protected, []object) {
	h.t.Helper()
	h.exec("INSERT INTO testee(id,org_id,name,gender,source,tags) VALUES(42,1,'synthetic-system',1,'retained',JSON_ARRAY()),(43,1,'synthetic-clack',1,'retained',JSON_ARRAY()),(44,1,'synthetic-cohort',1,'daily_simulation',JSON_ARRAY()),(45,1,'synthetic-real',1,'retained',JSON_ARRAY())")
	h.exec("INSERT INTO assessment_plan(id,org_id,scale_code,schedule_type,total_times) VALUES(500,1,'synthetic','by_day',1)")
	h.exec("INSERT INTO plan_enrollment(id,org_id,plan_id,testee_id,round,start_date,status,joined_at,record_origin) VALUES(501,1,500,44,1,'2026-01-01','active','2026-01-01','native'),(502,1,500,45,1,'2026-01-01','active','2026-01-01','native')")
	// Ordinary DEFAULT_GENERATED timestamps are original data, not computed
	// columns. Use an old instant so a restore that omits them cannot pass merely
	// because capture and restore happened in the same wall-clock second.
	h.exec("INSERT INTO assessment_score(id,assessment_id,testee_id,factor_code,factor_name,raw_score,risk_level,created_at,updated_at) VALUES(1,101,44,'synthetic','synthetic',2.5,'low','2026-01-01 01:02:03','2026-01-01 01:02:03')")
	h.exec("INSERT INTO interpretation_attention_projection(event_id,report_id,assessment_id,testee_id,risk_level,status,created_at,updated_at) VALUES('evt-native','401','101',44,'low','complete','2026-01-01','2026-01-01')")
	ctx := h.t.Context()
	db := h.client.Database(h.name)
	catalog := nativeMongoObject(primitive.NewObjectID())
	key, err := mongoKey(catalog)
	if err != nil {
		h.t.Fatal(err)
	}
	_, err = db.Collection("report_query_catalog").InsertOne(ctx, append(key, bson.E{Key: "assessment_id", Value: int64(101)}, bson.E{Key: "source_id", Value: int64(401)}, bson.E{Key: "org_id", Value: int64(1)}, bson.E{Key: "testee_id", Value: int64(44)}, bson.E{Key: "synthetic", Value: bson.D{{Key: "null", Value: nil}, {Key: "binary", Value: primitive.Binary{Subtype: 0x80, Data: []byte{0, 255, 1}}}, {Key: "empty", Value: bson.A{}}}}))
	if err != nil {
		h.t.Fatal("catalog seed failed:", err)
	}
	_, err = db.Collection("report_query_catalog").InsertOne(ctx, bson.D{{Key: "_id", Value: int64(42)}, {Key: "assessment_id", Value: int64(102)}, {Key: "source_id", Value: int64(402)}})
	if err != nil {
		h.t.Fatal("numeric physical identity seed failed:", err)
	}
	sheet := nativeMongoObject(primitive.NewObjectID())
	sheet.Name = "answersheets"
	sheetKey, _ := mongoKey(sheet)
	_, err = db.Collection("answersheets").InsertOne(ctx, append(sheetKey, bson.E{Key: "domain_id", Value: int64(700)}, bson.E{Key: "testee_id", Value: int64(44)}, bson.E{Key: "org_id", Value: int64(1)}, bson.E{Key: "filler_id", Value: int64(800)}, bson.E{Key: "attribution", Value: bson.D{{Key: "origin_type", Value: "adhoc"}}}))
	if err != nil {
		h.t.Fatal("independent AnswerSheet seed failed:", err)
	}
	guards := []object{keySQL("assessment_plan", "500"), keySQL("plan_enrollment", "501"), keySQL("plan_enrollment", "502"), sheet, nativeMongoObject(int64(42))}
	p := protected{Format: 1, Kind: "mock_cohort_protected", IAM: []byte(`{"user_ids":[],"profile_ids":[],"login_identity_ids":[]}`), QS: protectionScope{System: []object{keySQL("testee", "42")}, Clack: []object{keySQL("testee", "43")}, Other: guards}}
	g := group{Format: 1, Kind: "mock_cohort", Operation: "native-" + strings.TrimPrefix(h.name, "qs_mock_cohort_"), OwnerSHA: strings.Repeat("a", 64), ProtectedSHA: digest(encode(p)), IAM: []byte(`{"user_ids":[],"profile_ids":[],"login_identity_ids":[]}`), QS: qsScope{Selector: selection{Source: "daily_simulation", OrgID: "1", From: "2026-01-01T00:00:00+08:00", Through: "2026-01-02T00:00:00+08:00", Evidence: "synthetic dedicated primitive fixture; whole graph excluded"}, Binding: h.binding, Objects: []object{keySQL("assessment_score", "1"), {Store: "mysql", Name: "interpretation_attention_projection", Key: []cell{{Name: "event_id", Type: "text", Raw: []byte("evt-native")}}}, catalog, keySQL("assessment_plan", "500"), sheet}}}
	// Excluded objects are guarded, so the planner must not treat them as deletion
	// targets. They enter the proposal only after this shared-root test snapshot.
	g.QS.Objects = g.QS.Objects[:3]
	return g, p, []object{keySQL("assessment_plan", "500"), sheet}
}
func keySQL(table, id string) object {
	return object{Store: "mysql", Name: table, Key: []cell{{Name: "id", Type: "uint64", Raw: []byte(id)}}}
}

func (h *cohortNative) archived(g group, p protected) (group, material, string) {
	h.t.Helper()
	planned, err := plan(h.t.Context(), h.store, g, p)
	if err != nil {
		h.t.Fatal(err)
	}
	dir := privateTestDir(h.t)
	m, err := capture(h.t.Context(), h.store, planned, p, dir, digest(encode(planned)), toolSource)
	if err != nil {
		h.t.Fatal(err)
	}
	return planned, m, dir
}
func (h *cohortNative) assertGuards() {
	h.t.Helper()
	// Full byte equality is checked by protect/verify against the archive; this
	// independent assertion also checks both participants still share one Plan.
	var n int
	if h.store.sql.QueryRowContext(h.t.Context(), "SELECT COUNT(*) FROM plan_enrollment WHERE plan_id=500 AND testee_id IN (44,45)").Scan(&n) != nil || n != 2 {
		h.t.Fatal("shared Plan enrollment changed")
	}
}

type nativeOutcomeSeam struct {
	dataStore
	failStore string
	fired     bool
	writes    int
}

func (s *nativeOutcomeSeam) mutate(ctx context.Context, r []record, restore bool) error {
	s.writes++
	if err := s.dataStore.mutate(ctx, r, restore); err != nil {
		return err
	}
	if !s.fired && r[0].Object.Store == s.failStore {
		s.fired = true
		return errors.New("test response-layer fault after actual acknowledged commit")
	}
	return nil
}

func TestMockCohortNativeOwnedPrimitives(t *testing.T) {
	// The selected integration lane is mandatory, including its dependency gate.
	if os.Getenv("QS_MOCK_NATIVE_REQUIRED") != "1" {
		t.Fatal("QS_MOCK_NATIVE_REQUIRED=1 is mandatory")
	}
	t.Run("RealLeafRoundTripRetainsSharedPlanAndIndependentAnswerSheet", func(t *testing.T) {
		h := newCohortNative(t)
		g, p, excluded := h.seeded()
		// The current engine cannot clear these roots. Test exclusions independently
		// from the protection-intersection check, never by relaxing the latter.
		proposal := g
		proposal.QS.Objects = append(append([]object{}, g.QS.Objects...), excluded...)
		pOnly := p
		pOnly.QS.Other = []object{}
		proposal.ProtectedSHA = digest(encode(pOnly))
		planned, err := plan(t.Context(), h.store, proposal, pOnly)
		if err != nil {
			t.Fatal(err)
		}
		if len(planned.QS.Exclusions) != 2 || len(planned.QS.Objects) != 3 {
			t.Fatal("unsupported whole-graph roots were not excluded")
		}
		g, m, dir := h.archived(g, p)
		a := approval{Expires: time.Now().Add(2 * time.Minute)}
		if err = mutate(t.Context(), h.store, g, m, dir, a, strings.Repeat("1", 64), false); err != nil {
			t.Fatal(err)
		}
		if err = verify(t.Context(), h.store, m, dir, false); err != nil {
			t.Fatal(err)
		}
		if err = mutate(t.Context(), h.store, g, m, dir, a, strings.Repeat("2", 64), true); err != nil {
			t.Fatal(err)
		}
		if err = verify(t.Context(), h.store, m, dir, true); err != nil {
			t.Fatal(err)
		}
		h.assertGuards()
	})
	t.Run("PhysicalKeyAliasesCannotReachProtectedRows", func(t *testing.T) {
		h := newCohortNative(t)
		g, p, _ := h.seeded()
		original := h.mustRead(keySQL("assessment_score", "1"))
		p.QS.Other = append(p.QS.Other, keySQL("assessment_score", "1"), object{Store: "mysql", Name: "interpretation_attention_projection", Key: []cell{{Name: "event_id", Type: "text", Raw: []byte("evt-native")}}})
		g.ProtectedSHA = digest(encode(p))
		rejectAlias := func(o object) {
			t.Helper()
			proposal := g
			proposal.QS.Objects = []object{o}
			if _, err := plan(t.Context(), h.store, proposal, p); err == nil {
				t.Fatal("alias entered a planned deletion scope for a protected physical row")
			}
		}
		for _, typ := range []string{"text", "bytes", "int64"} {
			o := keySQL("assessment_score", "1")
			o.Key[0].Type = typ
			o.Key[0].Raw = []byte("01")
			rejectAlias(o)
		}
		text := object{Store: "mysql", Name: "interpretation_attention_projection", Key: []cell{{Name: "event_id", Type: "text", Raw: []byte("EVT-NATIVE")}}}
		rejectAlias(text)
		rejectAlias(nativeMongoObject(int32(42)))
		if !sameRecord(original, h.mustRead(keySQL("assessment_score", "1"))) {
			t.Fatal("alias checks changed SQL row")
		}
		_ = h.mustRead(nativeMongoObject(int64(42)))
	})
	t.Run("CrossStoreCommittedResponseFaultRequiresReconciliation", func(t *testing.T) {
		h := newCohortNative(t)
		g, p, _ := h.seeded()
		g, m, dir := h.archived(g, p)
		s := &nativeOutcomeSeam{dataStore: h.store, failStore: "mysql"}
		a := approval{Expires: time.Now().Add(2 * time.Minute)}
		if err := mutate(t.Context(), s, g, m, dir, a, strings.Repeat("3", 64), false); err == nil || !s.fired {
			t.Fatal("real commit plus response-layer fault was not retained")
		}
		before := s.writes
		if before != 2 {
			t.Fatal("expected one Mongo commit and one SQL commit before the fixed response fault")
		}
		if err := mutate(t.Context(), s, g, m, dir, a, strings.Repeat("4", 64), false); err == nil || s.writes != before {
			t.Fatal("unknown chunk replayed without reconciliation")
		}
		// The whole verify may remain negative because later chunks were not sent;
		// it still reconciles only exact original/absent chunks, without DB writes.
		if err := verify(t.Context(), h.store, m, dir, false); err == nil {
			t.Fatal("partially dispatched cross-store operation was reported complete")
		}
		if err := mutate(t.Context(), s, g, m, dir, a, strings.Repeat("5", 64), false); err != nil {
			t.Fatal(err)
		}
		if err := verify(t.Context(), h.store, m, dir, false); err != nil {
			t.Fatal(err)
		}
		if err := mutate(t.Context(), s, g, m, dir, a, strings.Repeat("6", 64), true); err != nil {
			t.Fatal(err)
		}
		if err := verify(t.Context(), h.store, m, dir, true); err != nil {
			for _, ref := range m.Records {
				original, archiveErr := archived(dir, ref)
				current, exists, readErr := h.store.read(t.Context(), ref.Object)
				if archiveErr != nil || readErr != nil {
					t.Logf("restore comparison store=%s object=%s key_sha=%s archive_read_ok=%t current_read_ok=%t", ref.Object.Store, ref.Object.Name, objectID(ref.Object), archiveErr == nil, readErr == nil)
					continue
				}
				if exists && sameRecord(original, current) {
					continue
				}
				changed := []string{}
				for _, before := range original.Columns {
					matched := false
					for _, after := range current.Columns {
						if before.Name == after.Name {
							matched = digest(encode(before)) == digest(encode(after))
							break
						}
					}
					if !matched {
						changed = append(changed, before.Name)
					}
				}
				t.Logf("restore mismatch store=%s object=%s key_sha=%s exists=%t original_sha=%s current_sha=%s definition_same=%t changed_columns=%v", ref.Object.Store, ref.Object.Name, objectID(ref.Object), exists, fingerprint(original), fingerprint(current), digest(original.Definition) == digest(current.Definition), changed)
			}
			t.Fatal(err)
		}
		h.assertGuards()
	})
	t.Run("RestoreConflictPreservesNewPhysicalFact", func(t *testing.T) {
		h := newCohortNative(t)
		g, p, _ := h.seeded()
		g, m, dir := h.archived(g, p)
		a := approval{Expires: time.Now().Add(2 * time.Minute)}
		if err := mutate(t.Context(), h.store, g, m, dir, a, strings.Repeat("7", 64), false); err != nil {
			t.Fatal(err)
		}
		h.exec("INSERT INTO assessment_score(id,assessment_id,testee_id,factor_code,factor_name,raw_score,risk_level) VALUES(1,999,45,'retained','new retained fact',999,'high')")
		fresh := h.mustRead(keySQL("assessment_score", "1"))
		if err := mutate(t.Context(), h.store, g, m, dir, a, strings.Repeat("8", 64), true); err == nil {
			t.Fatal("restore overwrote a conflicting new fact")
		}
		if !sameRecord(fresh, h.mustRead(keySQL("assessment_score", "1"))) {
			t.Fatal("negative restore changed retained fact")
		}
		if err := protect(t.Context(), h.store, m, dir); err != nil {
			t.Fatal(err)
		}
	})
}
