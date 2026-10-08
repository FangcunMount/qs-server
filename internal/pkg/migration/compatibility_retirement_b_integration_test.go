//go:build integration

package migration

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	mysqlclient "github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	migratemongo "github.com/golang-migrate/migrate/v4/database/mongodb"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/event"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

var bNativeSQLTargets = []string{"domain_event_outbox", "ai_bridge_commands", "ai_messaging_legacy_commands"}

type bNativeFixture struct {
	sqlDB     *sql.DB
	client    *mongo.Client
	mongoDB   *mongo.Database
	sqlName   string
	mongoName string
}

// No test in this file accepts an application endpoint. Each case owns unique
// database names inside the harness's disposable authenticated local servers.
func bNativeDatabasePair(t *testing.T, monitors ...*event.CommandMonitor) *bNativeFixture {
	t.Helper()
	if os.Getenv("QS_COMPAT_B_LOCAL_ONLY") != "1" {
		if os.Getenv("QS_COMPAT_REQUIRE_DATABASE") == "1" {
			t.Fatal("B native tests require the disposable local harness")
		}
		t.Skip("B native local harness is not enabled")
	}
	cfg, err := mysqlclient.ParseDSN(os.Getenv("MYSQL_DSN"))
	if err != nil {
		t.Fatal("invalid isolated SQL configuration")
	}
	host, port, err := net.SplitHostPort(cfg.Addr)
	if err != nil || cfg.Net != "tcp" || host != "127.0.0.1" || !bNativeValidPort(port) || cfg.DBName != "" {
		t.Fatal("non-isolated SQL endpoint refused")
	}
	uri := os.Getenv("QS_SERVER_TEST_MONGO_URI")
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "mongodb" || u.Hostname() != "127.0.0.1" || !bNativeValidPort(u.Port()) || strings.Contains(u.Host, ",") || u.Query().Get("directConnection") != "true" {
		t.Fatal("non-isolated Mongo endpoint refused")
	}
	var suffix [12]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	name := "qs_compat_b_test_" + hex.EncodeToString(suffix[:])
	cfg.MultiStatements = true
	admin, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("open isolated SQL admin")
	}
	if _, err := admin.ExecContext(t.Context(), "CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		_ = admin.Close()
		t.Fatal(err)
	}
	cfg.DBName = name
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		_, _ = admin.ExecContext(t.Context(), "DROP DATABASE `"+name+"`")
		_ = admin.Close()
		t.Fatal("open isolated SQL fixture")
	}
	// A second checkout while the migration provider owns its connection would
	// deadlock this test. It also makes ownership leaks visible after Run.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	clientOptions := options.Client().ApplyURI(uri)
	if len(monitors) > 0 {
		clientOptions.SetMonitor(monitors[0])
	}
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		t.Fatal("connect isolated Mongo fixture")
	}
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		t.Fatal(err)
	}
	var hello struct {
		SetName string `bson:"setName"`
	}
	if err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello); err != nil || hello.SetName != "qs-compat-b-local" {
		t.Fatal("isolated authenticated replica set required")
	}
	fixture := &bNativeFixture{sqlDB: db, client: client, mongoDB: client.Database(name), sqlName: name, mongoName: name}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		if err := fixture.mongoDB.Drop(cleanupCtx); err != nil {
			t.Errorf("drop own local Mongo fixture: %v", err)
		}
		if err := client.Disconnect(cleanupCtx); err != nil {
			t.Errorf("disconnect own local Mongo client: %v", err)
		}
		if err := db.Close(); err != nil {
			t.Errorf("close own local SQL pool: %v", err)
		}
		if _, err := admin.ExecContext(cleanupCtx, "DROP DATABASE `"+name+"`"); err != nil {
			t.Errorf("drop own local SQL fixture: %v", err)
		}
		if err := admin.Close(); err != nil {
			t.Errorf("close own local SQL admin pool: %v", err)
		}
	})
	return fixture
}

func bNativeValidPort(port string) bool {
	n, err := strconv.Atoi(port)
	return err == nil && n > 0 && n <= 65535
}

func (f *bNativeFixture) config() PairConfig {
	return PairConfig{MySQLDatabase: f.sqlName, MongoDatabase: f.mongoName, ExpectedSourceSHA: strings.Repeat("a", 40)}
}

func (f *bNativeFixture) exec(t *testing.T, statement string, args ...any) {
	t.Helper()
	if _, err := f.sqlDB.ExecContext(t.Context(), statement, args...); err != nil {
		t.Fatal(err)
	}
}

func (f *bNativeFixture) installed(t *testing.T, sqlVersion, mongoVersion uint) {
	t.Helper()
	f.exec(t, "CREATE TABLE schema_migrations(version BIGINT NOT NULL PRIMARY KEY,dirty BOOLEAN NOT NULL)")
	f.exec(t, "INSERT INTO schema_migrations(version,dirty) VALUES(?,FALSE)", sqlVersion)
	f.exec(t, "CREATE TABLE b_protected(id BIGINT PRIMARY KEY,note VARCHAR(128) NOT NULL)")
	f.exec(t, "INSERT INTO b_protected VALUES(1,'B native protected SQL fact')")
	if _, err := f.mongoDB.Collection("schema_migrations").InsertOne(t.Context(), bson.D{{Key: "version", Value: int64(mongoVersion)}, {Key: "dirty", Value: false}}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.mongoDB.Collection("b_protected").InsertOne(t.Context(), bson.D{{Key: "_id", Value: 1}, {Key: "note", Value: "B native protected Mongo fact"}}); err != nil {
		t.Fatal(err)
	}
}

func (f *bNativeFixture) targets(t *testing.T, present int, nonempty bool) {
	t.Helper()
	for i, name := range bNativeSQLTargets {
		if present&(1<<i) != 0 {
			f.exec(t, "CREATE TABLE `"+name+"`(id BIGINT PRIMARY KEY,payload TEXT)")
			if nonempty {
				f.exec(t, "INSERT INTO `"+name+"` VALUES(1,'B native retained original source')")
			}
		}
	}
	if present&8 != 0 {
		if err := f.mongoDB.CreateCollection(t.Context(), "domain_event_outbox"); err != nil {
			t.Fatal(err)
		}
		if nonempty {
			if _, err := f.mongoDB.Collection("domain_event_outbox").InsertOne(t.Context(), bson.D{{Key: "_id", Value: 1}, {Key: "payload", Value: "B native retained original source"}}); err != nil {
				t.Fatal(err)
			}
		}
	}
}

type bNativeSnapshot struct {
	SQLTables   map[string]string
	SQLCounts   map[string]int64
	SQLHead     []string
	MongoSchema []string
	MongoCounts map[string]int64
	MongoHead   []string
}

func (f *bNativeFixture) snapshot(t *testing.T) bNativeSnapshot {
	t.Helper()
	out := bNativeSnapshot{SQLTables: map[string]string{}, SQLCounts: map[string]int64{}, MongoCounts: map[string]int64{}}
	for _, name := range mysqlTableNames(t, f.sqlDB, f.sqlName) {
		// MySQL returns two columns for a table and four for a view. Preserve
		// both exact definitions without treating the diagnostic snapshot's
		// result shape as an application preflight failure.
		rows, err := f.sqlDB.QueryContext(t.Context(), "SHOW CREATE TABLE `"+name+"`")
		if err != nil {
			t.Fatal(err)
		}
		columns, err := rows.Columns()
		if err != nil || len(columns) < 2 || !rows.Next() {
			_ = rows.Close()
			t.Fatal("SHOW CREATE diagnostic shape is incomplete")
		}
		values := make([]string, len(columns))
		destinations := make([]any, len(columns))
		for i := range values {
			destinations[i] = &values[i]
		}
		if err := rows.Scan(destinations...); err != nil {
			_ = rows.Close()
			t.Fatal(err)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			t.Fatal(err)
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
		out.SQLTables[name] = strings.Join(values, "\x00")
		var count int64
		if err := f.sqlDB.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM `"+name+"`").Scan(&count); err != nil {
			t.Fatal(err)
		}
		out.SQLCounts[name] = count
		if name == "schema_migrations" {
			rows, err := f.sqlDB.QueryContext(t.Context(), "SELECT version,dirty FROM schema_migrations ORDER BY version")
			if err != nil {
				t.Fatal(err)
			}
			for rows.Next() {
				var version, dirty sql.NullString
				if err := rows.Scan(&version, &dirty); err != nil {
					_ = rows.Close()
					t.Fatal(err)
				}
				out.SQLHead = append(out.SQLHead, fmt.Sprintf("%t:%s/%t:%s", version.Valid, version.String, dirty.Valid, dirty.String))
			}
			if err := rows.Err(); err != nil {
				_ = rows.Close()
				t.Fatal(err)
			}
			if err := rows.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
	cursor, err := f.mongoDB.ListCollections(t.Context(), bson.D{})
	if err != nil {
		t.Fatal(err)
	}
	for cursor.Next(t.Context()) {
		name, ok := cursor.Current.Lookup("name").StringValueOK()
		if !ok {
			_ = cursor.Close(t.Context())
			t.Fatal("Mongo diagnostic namespace name malformed")
		}
		if name == "system.views" {
			// Mongo forbids the CountDocuments aggregation on system.views,
			// including for this root-role fixture. Preserve the complete view
			// definitions in the catalog snapshot and mark its count unknown.
			out.MongoCounts[name] = -1
		} else {
			count, err := f.mongoDB.Collection(name).CountDocuments(t.Context(), bson.D{})
			if err != nil {
				_ = cursor.Close(t.Context())
				t.Fatal(err)
			}
			out.MongoCounts[name] = count
		}
		raw, err := bson.MarshalExtJSON(cursor.Current, true, false)
		if err != nil {
			_ = cursor.Close(t.Context())
			t.Fatal(err)
		}
		out.MongoSchema = append(out.MongoSchema, string(raw))
	}
	if err := cursor.Err(); err != nil {
		_ = cursor.Close(t.Context())
		t.Fatal(err)
	}
	if err := cursor.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	sort.Strings(out.MongoSchema)
	cursor, err = f.mongoDB.Collection("schema_migrations").Find(t.Context(), bson.D{})
	if err != nil {
		t.Fatal(err)
	}
	for cursor.Next(t.Context()) {
		raw, err := bson.MarshalExtJSON(cursor.Current, true, false)
		if err != nil {
			_ = cursor.Close(t.Context())
			t.Fatal(err)
		}
		out.MongoHead = append(out.MongoHead, string(raw))
	}
	if err := cursor.Err(); err != nil {
		_ = cursor.Close(t.Context())
		t.Fatal(err)
	}
	if err := cursor.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	sort.Strings(out.MongoHead)
	return out
}

func (f *bNativeFixture) assertPoolsReusable(t *testing.T) {
	t.Helper()
	if stats := f.sqlDB.Stats(); stats.InUse != 0 {
		t.Fatalf("migration leaked its SQL checkout: InUse=%d", stats.InUse)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err := f.sqlDB.PingContext(ctx); err != nil {
		t.Fatalf("borrowed SQL pool was closed or blocked: %v", err)
	}
	if err := f.client.Ping(ctx, readpref.Primary()); err != nil {
		t.Fatalf("borrowed Mongo client was disconnected: %v", err)
	}
}

func (f *bNativeFixture) assertProtected(t *testing.T) {
	t.Helper()
	var note string
	if err := f.sqlDB.QueryRowContext(t.Context(), "SELECT note FROM b_protected WHERE id=1").Scan(&note); err != nil || note != "B native protected SQL fact" {
		t.Fatalf("protected SQL fact changed: %v", err)
	}
	var row struct {
		Note string `bson:"note"`
	}
	if err := f.mongoDB.Collection("b_protected").FindOne(t.Context(), bson.D{{Key: "_id", Value: 1}}).Decode(&row); err != nil || row.Note != "B native protected Mongo fact" {
		t.Fatalf("protected Mongo fact changed: %v", err)
	}
}

func (f *bNativeFixture) assertTargetsAbsent(t *testing.T) {
	t.Helper()
	for _, name := range bNativeSQLTargets {
		assertMySQLTable(t, f.sqlDB, f.sqlName, name, false)
	}
	names, err := f.mongoDB.ListCollectionNames(t.Context(), bson.D{{Key: "name", Value: "domain_event_outbox"}})
	if err != nil || len(names) != 0 {
		t.Fatalf("Mongo compatibility target remains: names=%v error=%v", names, err)
	}
}

func TestCompatibilityRetirementBInstalledSixteenPresenceCombinations(t *testing.T) {
	for present := 0; present < 16; present++ {
		t.Run(fmt.Sprintf("mask_%02d", present), func(t *testing.T) {
			f := bNativeDatabasePair(t)
			f.installed(t, 99, 38)
			f.targets(t, present, false)
			before := f.snapshot(t)
			pair, err := PreflightCompatibilityPair(t.Context(), f.sqlDB, f.client, f.config())
			if present != 0 {
				if err == nil || pair != nil {
					t.Fatal("present installed compatibility namespace was accepted")
				}
				if after := f.snapshot(t); !reflect.DeepEqual(before, after) {
					t.Fatal("rejected pair mutated metadata, source objects or counts")
				}
			} else {
				if err != nil || pair == nil {
					t.Fatalf("absent installed pair refused: %v", err)
				}
				version, changed, err := NewMigrator(f.sqlDB, pair.MySQLConfig(false)).Run()
				if err != nil || version != 100 || !changed {
					t.Fatalf("SQL installed forward: version=%d changed=%v error=%v", version, changed, err)
				}
				f.assertPoolsReusable(t)
				version, changed, err = NewMongoMigrator(f.client, pair.MongoConfig(false)).Run()
				if err != nil || version != 39 || !changed {
					t.Fatalf("Mongo installed forward: version=%d changed=%v error=%v", version, changed, err)
				}
				f.assertTargetsAbsent(t)
			}
			f.assertProtected(t)
			f.assertPoolsReusable(t)
		})
	}
}

func TestCompatibilityRetirementBPartialForwardAndRestart(t *testing.T) {
	f := bNativeDatabasePair(t)
	f.installed(t, 100, 38)
	for iteration := 0; iteration < 2; iteration++ {
		pair, err := PreflightCompatibilityPair(t.Context(), f.sqlDB, f.client, f.config())
		if err != nil {
			t.Fatalf("clean partial/latest preflight: %v", err)
		}
		version, changed, err := NewMigrator(f.sqlDB, pair.MySQLConfig(false)).Run()
		if err != nil || version != 100 || changed {
			t.Fatalf("SQL already-forward: version=%d changed=%v error=%v", version, changed, err)
		}
		version, changed, err = NewMongoMigrator(f.client, pair.MongoConfig(false)).Run()
		if err != nil || version != 39 || changed != (iteration == 0) {
			t.Fatalf("Mongo resume/restart: version=%d changed=%v error=%v", version, changed, err)
		}
		f.assertPoolsReusable(t)
		f.assertProtected(t)
		f.assertTargetsAbsent(t)
	}
}

func TestCompatibilityRetirementBRejectsInvalidInstalledPairWithoutWrites(t *testing.T) {
	cases := []struct {
		name   string
		change func(*testing.T, *bNativeFixture)
	}{
		{"sql_dirty", func(t *testing.T, f *bNativeFixture) { f.exec(t, "UPDATE schema_migrations SET dirty=TRUE") }},
		{"mongo_dirty", func(t *testing.T, f *bNativeFixture) {
			if _, err := f.mongoDB.Collection("schema_migrations").UpdateOne(t.Context(), bson.D{}, bson.D{{Key: "$set", Value: bson.D{{Key: "dirty", Value: true}}}}); err != nil {
				t.Fatal(err)
			}
		}},
		{"sql_old_head", func(t *testing.T, f *bNativeFixture) { f.exec(t, "UPDATE schema_migrations SET version=97") }},
		{"mongo_old_head", func(t *testing.T, f *bNativeFixture) {
			if _, err := f.mongoDB.Collection("schema_migrations").UpdateOne(t.Context(), bson.D{}, bson.D{{Key: "$set", Value: bson.D{{Key: "version", Value: int64(36)}}}}); err != nil {
				t.Fatal(err)
			}
		}},
		{"sql_multiple_heads", func(t *testing.T, f *bNativeFixture) { f.exec(t, "INSERT INTO schema_migrations VALUES(100,FALSE)") }},
		{"sql_VARCHAR_version", func(t *testing.T, f *bNativeFixture) {
			f.exec(t, "DROP TABLE schema_migrations")
			f.exec(t, "CREATE TABLE schema_migrations(version VARCHAR(32) NOT NULL PRIMARY KEY,dirty BOOLEAN NOT NULL)")
			f.exec(t, "INSERT INTO schema_migrations VALUES('99',FALSE)")
		}},
		{"sql_VARCHAR_dirty", func(t *testing.T, f *bNativeFixture) {
			f.exec(t, "DROP TABLE schema_migrations")
			f.exec(t, "CREATE TABLE schema_migrations(version BIGINT NOT NULL PRIMARY KEY,dirty VARCHAR(32) NOT NULL)")
			f.exec(t, "INSERT INTO schema_migrations VALUES(99,'false')")
		}},
		{"sql_dirty_two", func(t *testing.T, f *bNativeFixture) { f.exec(t, "UPDATE schema_migrations SET dirty=2") }},
		{"mongo_multiple_heads", func(t *testing.T, f *bNativeFixture) {
			if _, err := f.mongoDB.Collection("schema_migrations").InsertOne(t.Context(), bson.D{{Key: "version", Value: int64(39)}, {Key: "dirty", Value: false}}); err != nil {
				t.Fatal(err)
			}
		}},
		{"reverse_partial", func(t *testing.T, f *bNativeFixture) {
			if _, err := f.mongoDB.Collection("schema_migrations").UpdateOne(t.Context(), bson.D{}, bson.D{{Key: "$set", Value: bson.D{{Key: "version", Value: int64(39)}}}}); err != nil {
				t.Fatal(err)
			}
		}},
		{"sql_target_view", func(t *testing.T, f *bNativeFixture) {
			f.exec(t, "CREATE VIEW domain_event_outbox AS SELECT id,note FROM b_protected")
		}},
		{"mongo_target_view", func(t *testing.T, f *bNativeFixture) {
			if err := f.mongoDB.CreateView(t.Context(), "domain_event_outbox", "b_protected", mongo.Pipeline{}); err != nil {
				t.Fatal(err)
			}
		}},
		{"sql_source_nonempty", func(t *testing.T, f *bNativeFixture) { f.targets(t, 1, true) }},
		{"mongo_source_nonempty", func(t *testing.T, f *bNativeFixture) { f.targets(t, 8, true) }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			f := bNativeDatabasePair(t)
			f.installed(t, 99, 38)
			test.change(t, f)
			before := f.snapshot(t)
			pair, err := PreflightCompatibilityPair(t.Context(), f.sqlDB, f.client, f.config())
			if err == nil || pair != nil {
				t.Fatal("invalid installed pair was accepted")
			}
			if after := f.snapshot(t); !reflect.DeepEqual(before, after) {
				t.Fatal("refused preflight changed database objects or heads")
			}
			f.assertProtected(t)
			f.assertPoolsReusable(t)
		})
	}
}

func TestCompatibilityRetirementBStandaloneMigratorCannotCrossTail(t *testing.T) {
	f := bNativeDatabasePair(t)
	f.installed(t, 99, 38)
	f.targets(t, 15, true)
	before := f.snapshot(t)
	if _, _, err := NewMigrator(f.sqlDB, &Config{Enabled: true, Database: f.sqlName}).Run(); err == nil {
		t.Fatal("unpaired SQL migrator crossed B tail")
	}
	if _, _, err := NewMongoMigrator(f.client, &Config{Enabled: true, Database: f.mongoName}).Run(); err == nil {
		t.Fatal("unpaired Mongo migrator crossed B tail")
	}
	if after := f.snapshot(t); !reflect.DeepEqual(before, after) {
		t.Fatal("unpaired Run modified heads or retained sources")
	}
	f.assertProtected(t)
	f.assertPoolsReusable(t)
}

func TestCompatibilityRetirementBLowLevelForceAndDownCannotRewriteRetirementHeads(t *testing.T) {
	for _, backend := range []Backend{BackendMySQL, BackendMongo} {
		for _, action := range []string{"force_low", "force_current", "force_unknown", "down"} {
			for _, dirty := range []bool{false, true} {
				for _, useProof := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s_%s_dirty_%t_proof_%t", backend, action, dirty, useProof), func(t *testing.T) {
						f := bNativeDatabasePair(t)
						f.installed(t, 100, 39)
						pair, err := PreflightCompatibilityPair(t.Context(), f.sqlDB, f.client, f.config())
						if err != nil {
							t.Fatal(err)
						}
						// Even a previously valid opaque pair cannot authorize a manual
						// Force, Down or dirty-head cleanup on the raw migrate instance.
						f.targets(t, 15, true)
						if dirty {
							f.exec(t, "UPDATE schema_migrations SET dirty=TRUE")
							if _, err := f.mongoDB.Collection("schema_migrations").UpdateOne(t.Context(), bson.D{}, bson.D{{Key: "$set", Value: bson.D{{Key: "dirty", Value: true}}}}); err != nil {
								t.Fatal(err)
							}
						}
						before := f.snapshot(t)
						var instance *migrate.Migrate
						var sqlDriver *MySQLDriver
						current := 100
						if backend == BackendMySQL {
							cfg := ensureConfigDefaults(&Config{Enabled: true, Database: f.sqlName})
							if useProof {
								cfg = ensureConfigDefaults(pair.MySQLConfig(false))
							}
							sqlDriver = NewMySQLDriver(f.sqlDB)
							instance, err = sqlDriver.CreateInstance(migrations, cfg)
						} else {
							cfg := ensureConfigDefaults(&Config{Enabled: true, Database: f.mongoName})
							if useProof {
								cfg = ensureConfigDefaults(pair.MongoConfig(false))
							}
							instance, err = NewMongoDriver(f.client).CreateInstance(migrations, cfg)
							current = 39
						}
						if err != nil {
							t.Fatal(err)
						}
						switch action {
						case "force_low":
							err = instance.Force(current - 1)
						case "force_current":
							err = instance.Force(current)
						case "force_unknown":
							err = instance.Force(current + 1)
						case "down":
							err = instance.Down()
						}
						if sqlDriver != nil {
							if closeErr := sqlDriver.finishRun(); closeErr != nil {
								t.Fatal(closeErr)
							}
						}
						if err == nil {
							t.Fatal("raw Force/Down rewrote a retirement head or cleared dirty state")
						}
						if after := f.snapshot(t); !reflect.DeepEqual(before, after) {
							t.Fatal("rejected raw Force/Down changed head, source objects or data")
						}
						f.assertProtected(t)
						f.assertPoolsReusable(t)
					})
				}
			}
		}
	}
}

func TestCompatibilityRetirementBOrderAndMetadataOverridesRefuseBeforeWrites(t *testing.T) {
	for _, scenario := range []string{"Mongo_before_SQL_100", "SQL_metadata_override", "Mongo_metadata_override"} {
		t.Run(scenario, func(t *testing.T) {
			f := bNativeDatabasePair(t)
			f.installed(t, 99, 38)
			pair, err := PreflightCompatibilityPair(t.Context(), f.sqlDB, f.client, f.config())
			if err != nil {
				t.Fatal(err)
			}
			before := f.snapshot(t)
			switch scenario {
			case "Mongo_before_SQL_100":
				_, _, err = NewMongoMigrator(f.client, pair.MongoConfig(false)).Run()
			case "SQL_metadata_override":
				cfg := pair.MySQLConfig(false)
				cfg.MigrationsTable = "unapproved_schema_migrations"
				_, _, err = NewMigrator(f.sqlDB, cfg).Run()
			case "Mongo_metadata_override":
				cfg := pair.MongoConfig(false)
				cfg.MigrationsCollection = "unapproved_schema_migrations"
				_, _, err = NewMongoMigrator(f.client, cfg).Run()
			}
			if err == nil {
				t.Fatal("migration order or metadata override bypassed paired proof")
			}
			if after := f.snapshot(t); !reflect.DeepEqual(before, after) {
				t.Fatal("order/override refusal wrote metadata")
			}
			f.assertPoolsReusable(t)
		})
	}
}

func TestCompatibilityRetirementBLateNamespaceReappearanceRefuses(t *testing.T) {
	for _, mask := range []int{1, 2, 4, 8} {
		for _, nonempty := range []bool{false, true} {
			t.Run(fmt.Sprintf("mask_%d_nonempty_%t", mask, nonempty), func(t *testing.T) {
				f := bNativeDatabasePair(t)
				f.installed(t, 99, 38)
				pair, err := PreflightCompatibilityPair(t.Context(), f.sqlDB, f.client, f.config())
				if err != nil {
					t.Fatal(err)
				}
				if mask == 8 {
					if version, changed, err := NewMigrator(f.sqlDB, pair.MySQLConfig(false)).Run(); err != nil || version != 100 || !changed {
						t.Fatalf("prepare ordered late Mongo source: %d %t %v", version, changed, err)
					}
				}
				f.targets(t, mask, nonempty)
				if mask&8 == 0 {
					if _, _, err := NewMigrator(f.sqlDB, pair.MySQLConfig(false)).Run(); err == nil {
						t.Fatal("late SQL source namespace was dropped")
					}
					for i, name := range bNativeSQLTargets {
						if mask&(1<<i) != 0 {
							assertMySQLTable(t, f.sqlDB, f.sqlName, name, true)
						}
					}
				} else {
					if _, _, err := NewMongoMigrator(f.client, pair.MongoConfig(false)).Run(); err == nil {
						t.Fatal("late Mongo source namespace was dropped")
					}
					names, err := f.mongoDB.ListCollectionNames(t.Context(), bson.D{{Key: "name", Value: "domain_event_outbox"}})
					if err != nil || len(names) != 1 {
						t.Fatal("late Mongo source was changed")
					}
				}
				f.assertProtected(t)
				f.assertPoolsReusable(t)
			})
		}
	}
}

func TestCompatibilityRetirementBPairCannotAuthorizeDifferentConnectionsOrNames(t *testing.T) {
	for _, scenario := range []string{"different_SQL_pool", "different_Mongo_client", "different_SQL_name", "different_Mongo_name"} {
		t.Run(scenario, func(t *testing.T) {
			f := bNativeDatabasePair(t)
			f.installed(t, 99, 38)
			other := bNativeDatabasePair(t)
			other.installed(t, 99, 38)
			pair, err := PreflightCompatibilityPair(t.Context(), f.sqlDB, f.client, f.config())
			if err != nil {
				t.Fatal(err)
			}
			before, otherBefore := f.snapshot(t), other.snapshot(t)
			switch scenario {
			case "different_SQL_pool":
				_, _, err = NewMigrator(other.sqlDB, pair.MySQLConfig(false)).Run()
			case "different_Mongo_client":
				_, _, err = NewMongoMigrator(other.client, pair.MongoConfig(false)).Run()
			case "different_SQL_name":
				cfg := pair.MySQLConfig(false)
				cfg.Database = other.sqlName
				_, _, err = NewMigrator(f.sqlDB, cfg).Run()
			case "different_Mongo_name":
				cfg := pair.MongoConfig(false)
				cfg.Database = other.mongoName
				_, _, err = NewMongoMigrator(f.client, cfg).Run()
			}
			if err == nil {
				t.Fatal("paired proof was reused with a different borrowed connection or selected name")
			}
			if after := f.snapshot(t); !reflect.DeepEqual(before, after) {
				t.Fatal("misbound proof wrote its original database metadata")
			}
			if after := other.snapshot(t); !reflect.DeepEqual(otherBefore, after) {
				t.Fatal("misbound proof wrote the alternate database metadata")
			}
			f.assertPoolsReusable(t)
			other.assertPoolsReusable(t)
		})
	}
}

// The fixture constructs a fresh approval independently of the application
// path. Its expected raw hash is passed explicitly, rather than auto-approved
// after the application discovers that a namespace happens to be empty.
func (f *bNativeFixture) bootstrapConfig(t *testing.T) PairConfig {
	t.Helper()
	sqlIdentity, mongoIdentity, err := pairIdentity(t.Context(), f.sqlDB, f.client, f.sqlName, f.mongoName)
	if err != nil {
		t.Fatal(err)
	}
	authorization := bootstrapAuthorization{
		FormatVersion: 1, Kind: "qs_compatibility_retirement_b_pristine_bootstrap",
		OperationID: "123-1", ExpiresAt: time.Now().UTC().Add(30 * time.Minute).Format(time.RFC3339),
		ApprovedSourceSHA: strings.Repeat("a", 40), ApprovalSummarySHA256: strings.Repeat("b", 64),
		MySQLDatabase: f.sqlName, MongoDatabase: f.mongoName, MySQLIdentitySHA256: sqlIdentity,
		MongoClusterSHA256: mongoIdentity, MySQLVersion: 100, MongoVersion: 39,
		MigrationResourcesSHA256: CompatibilityMigrationResourcesSHA256(),
	}
	raw, err := json.Marshal(authorization)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "approved-pristine-bootstrap.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(raw)
	cfg := f.config()
	cfg.BootstrapAuthorizationFile, cfg.BootstrapAuthorizationSHA256 = path, hex.EncodeToString(hash[:])
	return cfg
}

func TestCompatibilityRetirementBPristinePairNeedsHashBoundAuthorization(t *testing.T) {
	f := bNativeDatabasePair(t)
	before := f.snapshot(t)
	if pair, err := PreflightCompatibilityPair(t.Context(), f.sqlDB, f.client, f.config()); err == nil || pair != nil {
		t.Fatal("unapproved empty pair was accepted")
	}
	if after := f.snapshot(t); !reflect.DeepEqual(before, after) {
		t.Fatal("unapproved pair created migration metadata")
	}
	cfg := f.bootstrapConfig(t)
	cfg.BootstrapAuthorizationSHA256 = strings.Repeat("c", 64)
	if pair, err := PreflightCompatibilityPair(t.Context(), f.sqlDB, f.client, cfg); err == nil || pair != nil {
		t.Fatal("wrong raw authorization hash was accepted")
	}
	if after := f.snapshot(t); !reflect.DeepEqual(before, after) {
		t.Fatal("hash mismatch created migration metadata")
	}
	f.assertPoolsReusable(t)
}

func TestCompatibilityRetirementBAuthorizedColdFullHistoryAndRestart(t *testing.T) {
	f := bNativeDatabasePair(t)
	cfg := f.bootstrapConfig(t)
	pair, err := PreflightCompatibilityPair(t.Context(), f.sqlDB, f.client, cfg)
	if err != nil {
		t.Fatalf("authorized pristine preflight: %v", err)
	}
	version, changed, err := NewMigrator(f.sqlDB, pair.MySQLConfig(false)).Run()
	if err != nil || version != 100 || !changed {
		t.Fatalf("authorized cold SQL: version=%d changed=%v error=%v", version, changed, err)
	}
	info, err := os.Lstat(cfg.BootstrapAuthorizationFile + ".consumed")
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		t.Fatal("cold SQL did not durably consume its authorization")
	}
	if _, err := readBootstrapAuthorization(cfg); err == nil {
		t.Fatal("consumed cold authorization was reusable")
	}
	f.assertPoolsReusable(t)
	version, changed, err = NewMongoMigrator(f.client, pair.MongoConfig(false)).Run()
	if err != nil || version != 39 || !changed {
		t.Fatalf("authorized cold Mongo: version=%d changed=%v error=%v", version, changed, err)
	}
	f.assertTargetsAbsent(t)
	assertMySQLTable(t, f.sqlDB, f.sqlName, "rm_outbox", true)
	for iteration := 0; iteration < 2; iteration++ {
		pair, err = PreflightCompatibilityPair(t.Context(), f.sqlDB, f.client, f.config())
		if err != nil {
			t.Fatal(err)
		}
		version, changed, err = NewMigrator(f.sqlDB, pair.MySQLConfig(false)).Run()
		if err != nil || version != 100 || changed {
			t.Fatalf("cold SQL restart: version=%d changed=%v error=%v", version, changed, err)
		}
		version, changed, err = NewMongoMigrator(f.client, pair.MongoConfig(false)).Run()
		if err != nil || version != 39 || changed {
			t.Fatalf("cold Mongo restart: version=%d changed=%v error=%v", version, changed, err)
		}
		f.assertTargetsAbsent(t)
		f.assertPoolsReusable(t)
	}
}

func (f *bNativeFixture) limitedSQL(t *testing.T, grants []string) *sql.DB {
	t.Helper()
	user := "b_native_" + f.sqlName[len(f.sqlName)-12:]
	password := "only-local-B-synthetic-password"
	f.exec(t, "CREATE USER '"+user+"'@'%' IDENTIFIED BY '"+password+"'")
	for _, grant := range grants {
		f.exec(t, grant+" TO '"+user+"'@'%'")
	}
	cfg, err := mysqlclient.ParseDSN(os.Getenv("MYSQL_DSN"))
	if err != nil {
		t.Fatal("parse isolated limited SQL configuration")
	}
	cfg.User, cfg.Passwd, cfg.DBName = user, password, f.sqlName
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("open isolated limited SQL pool")
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close own limited SQL pool: %v", err)
		}
		if _, err := f.sqlDB.ExecContext(context.Background(), "DROP USER '"+user+"'@'%'"); err != nil {
			t.Errorf("drop own limited SQL user: %v", err)
		}
	})
	return db
}

func (f *bNativeFixture) limitedMongo(t *testing.T, actions []string, collections ...string) *mongo.Client {
	t.Helper()
	user := "b_native_" + f.mongoName[len(f.mongoName)-12:]
	role := user + "_role"
	password := "only-local-B-synthetic-password"
	admin := f.client.Database("admin")
	collection := "schema_migrations"
	if len(collections) > 0 {
		collection = collections[0]
	}
	privilege := bson.D{{Key: "resource", Value: bson.D{{Key: "db", Value: f.mongoName}, {Key: "collection", Value: collection}}}, {Key: "actions", Value: actions}}
	if err := admin.RunCommand(t.Context(), bson.D{{Key: "createRole", Value: role}, {Key: "privileges", Value: bson.A{privilege}}, {Key: "roles", Value: bson.A{}}}).Err(); err != nil {
		t.Fatal(err)
	}
	if err := admin.RunCommand(t.Context(), bson.D{{Key: "createUser", Value: user}, {Key: "pwd", Value: password}, {Key: "roles", Value: bson.A{bson.D{{Key: "role", Value: role}, {Key: "db", Value: "admin"}}}}}).Err(); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(os.Getenv("QS_SERVER_TEST_MONGO_URI"))
	if err != nil {
		t.Fatal("parse isolated limited Mongo configuration")
	}
	u.User = url.UserPassword(user, password)
	client, err := mongo.Connect(t.Context(), options.Client().ApplyURI(u.String()))
	if err != nil {
		t.Fatal("connect own limited Mongo client")
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := client.Disconnect(ctx); err != nil {
			t.Errorf("disconnect own limited Mongo client: %v", err)
		}
		if err := admin.RunCommand(ctx, bson.D{{Key: "dropUser", Value: user}}).Err(); err != nil {
			t.Errorf("drop own limited Mongo user: %v", err)
		}
		if err := admin.RunCommand(ctx, bson.D{{Key: "dropRole", Value: role}}).Err(); err != nil {
			t.Errorf("drop own limited Mongo role: %v", err)
		}
	})
	return client
}

func TestCompatibilityRetirementBIncompleteVisibilityRefusesWithoutWrites(t *testing.T) {
	t.Run("SQL_table_only_select", func(t *testing.T) {
		f := bNativeDatabasePair(t)
		f.installed(t, 99, 38)
		db := f.limitedSQL(t, []string{"GRANT SELECT ON `" + f.sqlName + "`.schema_migrations"})
		before := f.snapshot(t)
		if pair, err := PreflightCompatibilityPair(t.Context(), db, f.client, f.config()); err == nil || pair != nil {
			t.Fatal("partial SQL catalog visibility was accepted")
		}
		if after := f.snapshot(t); !reflect.DeepEqual(before, after) {
			t.Fatal("partial SQL visibility failure wrote metadata")
		}
		f.assertPoolsReusable(t)
	})
	t.Run("Mongo_collection_only_find", func(t *testing.T) {
		f := bNativeDatabasePair(t)
		f.installed(t, 99, 38)
		client := f.limitedMongo(t, []string{"find"})
		before := f.snapshot(t)
		if pair, err := PreflightCompatibilityPair(t.Context(), f.sqlDB, client, f.config()); err == nil || pair != nil {
			t.Fatal("partial Mongo catalog visibility was accepted")
		}
		if after := f.snapshot(t); !reflect.DeepEqual(before, after) {
			t.Fatal("partial Mongo visibility failure wrote metadata")
		}
		f.assertPoolsReusable(t)
	})
	t.Run("pristine_hidden_SQL_routine", func(t *testing.T) {
		f := bNativeDatabasePair(t)
		cfg := f.bootstrapConfig(t)
		f.exec(t, "CREATE PROCEDURE hidden_local_routine() SELECT 1")
		db := f.limitedSQL(t, []string{"GRANT SELECT ON `" + f.sqlName + "`.*"})
		if pair, err := PreflightCompatibilityPair(t.Context(), db, f.client, cfg); err == nil || pair != nil {
			t.Fatal("incomplete routine visibility was mistaken for pristine")
		}
		var count int
		if err := f.sqlDB.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM information_schema.routines WHERE routine_schema=?", f.sqlName).Scan(&count); err != nil || count != 1 {
			t.Fatal("hidden routine was changed")
		}
		if names := mysqlTableNames(t, f.sqlDB, f.sqlName); len(names) != 0 {
			t.Fatal("routine visibility failure wrote SQL metadata")
		}
		if names := mongoCollectionNames(t, f.mongoDB); len(names) != 0 {
			t.Fatal("routine visibility failure wrote Mongo metadata")
		}
		f.assertPoolsReusable(t)
	})
	t.Run("pristine_hidden_SQL_event", func(t *testing.T) {
		f := bNativeDatabasePair(t)
		cfg := f.bootstrapConfig(t)
		f.exec(t, "CREATE EVENT hidden_local_event ON SCHEDULE AT CURRENT_TIMESTAMP + INTERVAL 1 DAY DO SELECT 1")
		db := f.limitedSQL(t, []string{"GRANT SELECT ON `" + f.sqlName + "`.*"})
		if pair, err := PreflightCompatibilityPair(t.Context(), db, f.client, cfg); err == nil || pair != nil {
			t.Fatal("incomplete event visibility was mistaken for pristine")
		}
		var count int
		if err := f.sqlDB.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM information_schema.events WHERE event_schema=?", f.sqlName).Scan(&count); err != nil || count != 1 {
			t.Fatal("hidden event was changed")
		}
		if names := mysqlTableNames(t, f.sqlDB, f.sqlName); len(names) != 0 {
			t.Fatal("event visibility failure wrote SQL metadata")
		}
		if names := mongoCollectionNames(t, f.mongoDB); len(names) != 0 {
			t.Fatal("event visibility failure wrote Mongo metadata")
		}
		f.assertPoolsReusable(t)
	})
}

func bNativeProfileState(t *testing.T, db *mongo.Database) map[string]bson.RawValue {
	t.Helper()
	var state bson.Raw
	if err := db.RunCommand(t.Context(), bson.D{{Key: "profile", Value: -1}}).Decode(&state); err != nil {
		t.Fatal(err)
	}
	result := make(map[string]bson.RawValue)
	for _, key := range []string{"was", "slowms", "sampleRate", "filter"} {
		if value := state.Lookup(key); value.Type != 0 {
			result[key] = value
		}
	}
	return result
}

func TestCompatibilityRetirementBPristineProfilingVisibilityIsReadOnlyAndRequired(t *testing.T) {
	for _, scenario := range []string{"fresh_read_only", "enabled_profile", "disabled_profile_with_filter", "catalog_visibility_without_profile_permission"} {
		t.Run(scenario, func(t *testing.T) {
			f := bNativeDatabasePair(t)
			cfg := f.bootstrapConfig(t)
			client := f.client
			switch scenario {
			case "enabled_profile":
				if err := f.mongoDB.RunCommand(t.Context(), bson.D{{Key: "profile", Value: 1}, {Key: "slowms", Value: int32(2147483647)}, {Key: "sampleRate", Value: 0}}).Err(); err != nil {
					t.Fatal(err)
				}
			case "disabled_profile_with_filter":
				if err := f.mongoDB.RunCommand(t.Context(), bson.D{{Key: "profile", Value: 0}, {Key: "filter", Value: bson.D{{Key: "ns", Value: bson.D{{Key: "$exists", Value: true}}}}}}).Err(); err != nil {
					t.Fatal(err)
				}
			case "catalog_visibility_without_profile_permission":
				client = f.limitedMongo(t, []string{"listCollections"}, "")
				cursor, err := client.Database(f.mongoName).ListCollections(t.Context(), bson.D{}, options.ListCollections().SetAuthorizedCollections(false).SetNameOnly(false))
				if err != nil {
					t.Fatalf("fixture lacks complete catalog visibility: %v", err)
				}
				var namespaces []bson.Raw
				if err := cursor.All(t.Context(), &namespaces); err != nil || len(namespaces) != 0 {
					t.Fatal("fixture catalog is not visibly empty")
				}
				if err := client.Database(f.mongoName).RunCommand(t.Context(), bson.D{{Key: "profile", Value: -1}}).Err(); err == nil {
					t.Fatal("fixture unexpectedly has profiling visibility")
				}
			}
			if scenario == "enabled_profile" || scenario == "disabled_profile_with_filter" {
				t.Cleanup(func() {
					ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
					defer cancel()
					if err := f.mongoDB.RunCommand(ctx, bson.D{{Key: "profile", Value: 0}, {Key: "filter", Value: "unset"}}).Err(); err != nil {
						t.Errorf("reset own fixture profiling: %v", err)
					}
				})
			}
			profileBefore := bNativeProfileState(t, f.mongoDB)
			if scenario == "enabled_profile" && profileBefore["was"].Int32() != 1 {
				t.Fatal("fixture profiling was not enabled")
			}
			if scenario == "disabled_profile_with_filter" && profileBefore["filter"].Type == 0 {
				t.Fatal("fixture lacks a profiling filter")
			}
			if scenario != "enabled_profile" && len(mongoCollectionNames(t, f.mongoDB)) != 0 {
				t.Fatal("pristine profiling fixture already has a namespace")
			}
			before := f.snapshot(t)
			pair, err := PreflightCompatibilityPair(t.Context(), f.sqlDB, client, cfg)
			if scenario == "fresh_read_only" {
				if err != nil || pair == nil {
					t.Fatalf("read-only pristine profiling preflight failed: %v", err)
				}
			} else if err == nil || pair != nil {
				t.Fatal("unsafe or unknown profiling state was accepted as pristine")
			}
			if after := f.snapshot(t); !reflect.DeepEqual(before, after) {
				t.Fatal("profiling preflight changed catalog or metadata")
			}
			if profileAfter := bNativeProfileState(t, f.mongoDB); !reflect.DeepEqual(profileBefore, profileAfter) {
				t.Fatal("read-only preflight changed profiling state")
			}
			if _, err := os.Lstat(cfg.BootstrapAuthorizationFile + ".consumed"); !os.IsNotExist(err) {
				t.Fatal("profiling preflight consumed cold authorization")
			}
			f.assertPoolsReusable(t)
		})
	}
}

func TestCompatibilityRetirementBPristineAuthorizationRejectsTamperingWithoutConsumption(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*testing.T, *PairConfig)
	}{
		{"expired", func(t *testing.T, cfg *PairConfig) {
			bNativeRewriteAuthorization(t, cfg, func(a *bootstrapAuthorization) { a.ExpiresAt = time.Now().UTC().Add(-time.Minute).Format(time.RFC3339) })
		}},
		{"wrong_SQL_identity", func(t *testing.T, cfg *PairConfig) {
			bNativeRewriteAuthorization(t, cfg, func(a *bootstrapAuthorization) { a.MySQLIdentitySHA256 = strings.Repeat("d", 64) })
		}},
		{"wrong_Mongo_cluster", func(t *testing.T, cfg *PairConfig) {
			bNativeRewriteAuthorization(t, cfg, func(a *bootstrapAuthorization) { a.MongoClusterSHA256 = strings.Repeat("d", 64) })
		}},
		{"wrong_SQL_selected_name", func(t *testing.T, cfg *PairConfig) {
			bNativeRewriteAuthorization(t, cfg, func(a *bootstrapAuthorization) { a.MySQLDatabase += "_typo" })
		}},
		{"wrong_Mongo_selected_name", func(t *testing.T, cfg *PairConfig) {
			bNativeRewriteAuthorization(t, cfg, func(a *bootstrapAuthorization) { a.MongoDatabase += "_typo" })
		}},
		{"wrong_resources", func(t *testing.T, cfg *PairConfig) {
			bNativeRewriteAuthorization(t, cfg, func(a *bootstrapAuthorization) { a.MigrationResourcesSHA256 = strings.Repeat("e", 64) })
		}},
		{"wrong_version", func(t *testing.T, cfg *PairConfig) {
			bNativeRewriteAuthorization(t, cfg, func(a *bootstrapAuthorization) { a.MySQLVersion = 99 })
		}},
		{"wrong_approved_source", func(t *testing.T, cfg *PairConfig) {
			bNativeRewriteAuthorization(t, cfg, func(a *bootstrapAuthorization) { a.ApprovedSourceSHA = strings.Repeat("c", 40) })
		}},
		{"expected_source_empty", func(_ *testing.T, cfg *PairConfig) {
			cfg.ExpectedSourceSHA = ""
		}},
		{"expected_source_unknown", func(_ *testing.T, cfg *PairConfig) {
			cfg.ExpectedSourceSHA = "unknown"
		}},
		{"expected_source_invalid", func(_ *testing.T, cfg *PairConfig) {
			cfg.ExpectedSourceSHA = strings.Repeat("z", 40)
		}},
		{"expected_source_mismatch", func(_ *testing.T, cfg *PairConfig) {
			cfg.ExpectedSourceSHA = strings.Repeat("c", 40)
		}},
		{"world_readable_file", func(t *testing.T, cfg *PairConfig) {
			if err := os.Chmod(cfg.BootstrapAuthorizationFile, 0644); err != nil {
				t.Fatal(err)
			}
		}},
		{"symlink_file", func(t *testing.T, cfg *PairConfig) {
			link := cfg.BootstrapAuthorizationFile + ".link"
			if err := os.Symlink(cfg.BootstrapAuthorizationFile, link); err != nil {
				t.Fatal(err)
			}
			cfg.BootstrapAuthorizationFile = link
		}},
		{"hardlinked_file", func(t *testing.T, cfg *PairConfig) {
			if err := os.Link(cfg.BootstrapAuthorizationFile, cfg.BootstrapAuthorizationFile+".link"); err != nil {
				t.Fatal(err)
			}
		}},
		{"duplicate_JSON_key", func(t *testing.T, cfg *PairConfig) {
			raw, err := os.ReadFile(cfg.BootstrapAuthorizationFile)
			if err != nil {
				t.Fatal(err)
			}
			raw = append([]byte(`{"format_version":1,`), raw[1:]...)
			bNativeWriteAuthorization(t, cfg, raw)
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			f := bNativeDatabasePair(t)
			cfg := f.bootstrapConfig(t)
			test.mutate(t, &cfg)
			before := f.snapshot(t)
			if pair, err := PreflightCompatibilityPair(t.Context(), f.sqlDB, f.client, cfg); err == nil || pair != nil {
				t.Fatal("invalid cold authorization was accepted")
			}
			if _, err := os.Lstat(cfg.BootstrapAuthorizationFile + ".consumed"); !os.IsNotExist(err) {
				t.Fatal("invalid authorization was consumed")
			}
			if after := f.snapshot(t); !reflect.DeepEqual(before, after) {
				t.Fatal("invalid authorization created metadata")
			}
			f.assertPoolsReusable(t)
		})
	}
}

func bNativeRewriteAuthorization(t *testing.T, cfg *PairConfig, mutate func(*bootstrapAuthorization)) {
	t.Helper()
	raw, err := os.ReadFile(cfg.BootstrapAuthorizationFile)
	if err != nil {
		t.Fatal(err)
	}
	var a bootstrapAuthorization
	if err := json.Unmarshal(raw, &a); err != nil {
		t.Fatal(err)
	}
	mutate(&a)
	raw, err = json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	bNativeWriteAuthorization(t, cfg, raw)
}

func bNativeWriteAuthorization(t *testing.T, cfg *PairConfig, raw []byte) {
	t.Helper()
	if err := os.WriteFile(cfg.BootstrapAuthorizationFile, raw, 0600); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(raw)
	cfg.BootstrapAuthorizationSHA256 = hex.EncodeToString(hash[:])
}

func bNativeMongoTailWrapper(t *testing.T, f *bNativeFixture, pair *PairPreflight) *retirementDatabaseDriver {
	t.Helper()
	cfg := ensureConfigDefaults(pair.MongoConfig(false))
	provider, err := migratemongo.WithInstance(f.client, &migratemongo.Config{DatabaseName: f.mongoName, MigrationsCollection: defaultTable})
	if err != nil {
		t.Fatal(err)
	}
	return newRetirementDatabaseDriver(provider, BackendMongo, cfg, nil, f.client, migrations)
}

func bNativeMongoUpBytes(t *testing.T) []byte {
	t.Helper()
	raw, err := migrations.ReadFile("migrations/mongodb/000039_retire_compatibility_message_storage.up.json")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestCompatibilityRetirementBNativeMongoWrapperBoundaries(t *testing.T) {
	for _, scenario := range []string{"installed_absent_no_drop", "pristine_empty_one_drop", "pristine_nonempty_retained", "canonical_body_wrong_version", "tail_wrong_body", "ordinary_missing_namespace_error"} {
		t.Run(scenario, func(t *testing.T) {
			var dropCommands, createCommands atomic.Int64
			monitor := &event.CommandMonitor{Started: func(_ context.Context, command *event.CommandStartedEvent) {
				if name, ok := command.Command.Lookup("drop").StringValueOK(); ok && name == "domain_event_outbox" {
					dropCommands.Add(1)
				}
				if name, ok := command.Command.Lookup("create").StringValueOK(); ok && name == "domain_event_outbox" {
					createCommands.Add(1)
				}
			}}
			f := bNativeDatabasePair(t, monitor)
			var pair *PairPreflight
			var err error
			if strings.HasPrefix(scenario, "pristine_") {
				pair, err = PreflightCompatibilityPair(t.Context(), f.sqlDB, f.client, f.bootstrapConfig(t))
				if err != nil {
					t.Fatal(err)
				}
				if version, changed, err := NewMigrator(f.sqlDB, pair.MySQLConfig(false)).Run(); err != nil || version != 100 || !changed {
					t.Fatalf("SQL full-up before narrow cold wrapper: %d %t %v", version, changed, err)
				}
				if err := pair.validateStart(t.Context(), BackendMongo); err != nil {
					t.Fatal(err)
				}
			} else {
				f.installed(t, 100, 38)
				pair, err = PreflightCompatibilityPair(t.Context(), f.sqlDB, f.client, f.config())
				if err != nil {
					t.Fatal(err)
				}
			}
			wrapper := bNativeMongoTailWrapper(t, f, pair)
			if strings.HasPrefix(scenario, "pristine_") || scenario == "canonical_body_wrong_version" {
				f.targets(t, 8, scenario != "pristine_empty_one_drop")
			}
			dropCommands.Store(0)
			createCommands.Store(0)
			version := 39
			if scenario == "canonical_body_wrong_version" || scenario == "ordinary_missing_namespace_error" {
				version = 38
			}
			if err := wrapper.SetVersion(version, true); err != nil {
				t.Fatal(err)
			}
			body := bNativeMongoUpBytes(t)
			if scenario == "tail_wrong_body" {
				body = []byte(`[{"drop":"b_protected"}]`)
			}
			if scenario == "ordinary_missing_namespace_error" {
				body = []byte(`[{"collMod":"b_nonretirement_missing_namespace","validator":{}}]`)
			}
			err = wrapper.Run(bytes.NewReader(body))
			switch scenario {
			case "installed_absent_no_drop":
				if err != nil || dropCommands.Load() != 0 || createCommands.Load() != 0 {
					t.Fatalf("absent exact tail did not remain no-op: drops=%d creates=%d error=%v", dropCommands.Load(), createCommands.Load(), err)
				}
				f.assertTargetsAbsent(t)
			case "pristine_empty_one_drop":
				if err != nil || dropCommands.Load() != 1 || createCommands.Load() != 0 {
					t.Fatalf("empty proven-pristine exact tail: drops=%d creates=%d error=%v", dropCommands.Load(), createCommands.Load(), err)
				}
				f.assertTargetsAbsent(t)
			case "pristine_nonempty_retained", "canonical_body_wrong_version":
				if err == nil || dropCommands.Load() != 0 {
					t.Fatal("nonempty/wrong-version source was dropped")
				}
				count, readErr := f.mongoDB.Collection("domain_event_outbox").CountDocuments(t.Context(), bson.D{})
				if readErr != nil || count != 1 {
					t.Fatal("nonempty source evidence was lost")
				}
			case "tail_wrong_body":
				if err == nil {
					t.Fatal("wrong tail command body was delegated")
				}
				f.assertProtected(t)
			case "ordinary_missing_namespace_error":
				originalErr := wrapper.Driver.Run(bytes.NewReader(body))
				if err == nil || originalErr == nil || err.Error() != originalErr.Error() {
					t.Fatal("ordinary namespace-not-found was globally swallowed")
				}
				f.assertProtected(t)
			}
			f.assertPoolsReusable(t)
		})
	}
}

func TestCompatibilityRetirementBNativeMongoUUIDReplacementBeforeDropRefuses(t *testing.T) {
	var armed atomic.Bool
	var mutationFinished atomic.Bool
	var dropCommands atomic.Int64
	mutationErrors := make(chan error, 1)
	var mutator *mongo.Client
	var databaseName string
	monitor := &event.CommandMonitor{Started: func(ctx context.Context, command *event.CommandStartedEvent) {
		if name, ok := command.Command.Lookup("drop").StringValueOK(); ok && name == "domain_event_outbox" {
			dropCommands.Add(1)
		}
		if name, ok := command.Command.Lookup("aggregate").StringValueOK(); !ok || name != "domain_event_outbox" || !armed.CompareAndSwap(true, false) {
			return
		}
		// A different local connection replaces the empty namespace while the
		// guarded count has not reached the server. Its zero result must not
		// authorize deleting the replacement collection's different UUID.
		db := mutator.Database(databaseName)
		if err := db.Collection("domain_event_outbox").Drop(ctx); err != nil {
			mutationErrors <- err
			return
		}
		if err := db.CreateCollection(ctx, "domain_event_outbox"); err != nil {
			mutationErrors <- err
			return
		}
		mutationFinished.Store(true)
	}}
	f := bNativeDatabasePair(t, monitor)
	databaseName = f.mongoName
	var err error
	mutator, err = mongo.Connect(t.Context(), options.Client().ApplyURI(os.Getenv("QS_SERVER_TEST_MONGO_URI")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := mutator.Disconnect(ctx); err != nil {
			t.Errorf("disconnect own UUID mutation client: %v", err)
		}
	})
	pair, err := PreflightCompatibilityPair(t.Context(), f.sqlDB, f.client, f.bootstrapConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	if version, changed, err := NewMigrator(f.sqlDB, pair.MySQLConfig(false)).Run(); err != nil || version != 100 || !changed {
		t.Fatalf("cold SQL before UUID replacement: %d %t %v", version, changed, err)
	}
	if err := pair.validateStart(t.Context(), BackendMongo); err != nil {
		t.Fatal(err)
	}
	wrapper := bNativeMongoTailWrapper(t, f, pair)
	f.targets(t, 8, false)
	before, err := mongoRetirementNamespace(t.Context(), f.mongoDB)
	if err != nil || len(before) != 1 {
		t.Fatal("old UUID fixture missing")
	}
	beforeUUID, err := mongoRetirementUUID(before[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := wrapper.SetVersion(39, true); err != nil {
		t.Fatal(err)
	}
	dropCommands.Store(0)
	armed.Store(true)
	if err := wrapper.Run(bytes.NewReader(bNativeMongoUpBytes(t))); err == nil {
		t.Fatal("count on replacement UUID authorized a DROP")
	}
	select {
	case err := <-mutationErrors:
		t.Fatalf("local UUID replacement failed: %v", err)
	default:
	}
	if !mutationFinished.Load() {
		t.Fatal("actual guarded count did not exercise the deterministic namespace replacement")
	}
	if dropCommands.Load() != 0 {
		t.Fatal("wrapper issued DROP after namespace UUID replacement")
	}
	after, err := mongoRetirementNamespace(t.Context(), f.mongoDB)
	if err != nil || len(after) != 1 {
		t.Fatal("replacement namespace was removed")
	}
	afterUUID, err := mongoRetirementUUID(after[0])
	if err != nil || reflect.DeepEqual(beforeUUID, afterUUID) {
		t.Fatal("actual namespace UUID was not replaced")
	}
	f.assertPoolsReusable(t)
}

func TestCompatibilityRetirementBHeadlessOrMixedPairIsNotPristine(t *testing.T) {
	cases := []struct {
		name   string
		change func(*testing.T, *bNativeFixture)
	}{
		{"headless_sql_fact", func(t *testing.T, f *bNativeFixture) {
			f.exec(t, "CREATE TABLE unexpected_fact(id BIGINT PRIMARY KEY)")
		}},
		{"headless_mongo_fact", func(t *testing.T, f *bNativeFixture) {
			if err := f.mongoDB.CreateCollection(t.Context(), "unexpected_fact"); err != nil {
				t.Fatal(err)
			}
		}},
		{"empty_sql_metadata", func(t *testing.T, f *bNativeFixture) {
			f.exec(t, "CREATE TABLE schema_migrations(version BIGINT NOT NULL PRIMARY KEY,dirty BOOLEAN NOT NULL)")
		}},
		{"empty_mongo_metadata", func(t *testing.T, f *bNativeFixture) {
			if err := f.mongoDB.CreateCollection(t.Context(), "schema_migrations"); err != nil {
				t.Fatal(err)
			}
		}},
		{"installed_sql_pristine_mongo", func(t *testing.T, f *bNativeFixture) {
			f.exec(t, "CREATE TABLE schema_migrations(version BIGINT NOT NULL PRIMARY KEY,dirty BOOLEAN NOT NULL)")
			f.exec(t, "INSERT INTO schema_migrations VALUES(99,FALSE)")
		}},
		{"pristine_sql_installed_mongo", func(t *testing.T, f *bNativeFixture) {
			if _, err := f.mongoDB.Collection("schema_migrations").InsertOne(t.Context(), bson.D{{Key: "version", Value: int64(38)}, {Key: "dirty", Value: false}}); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			f := bNativeDatabasePair(t)
			cfg := f.bootstrapConfig(t)
			test.change(t, f)
			before := f.snapshot(t)
			if pair, err := PreflightCompatibilityPair(t.Context(), f.sqlDB, f.client, cfg); err == nil || pair != nil {
				t.Fatal("headless/partial pair was adopted as pristine")
			}
			if after := f.snapshot(t); !reflect.DeepEqual(before, after) {
				t.Fatal("headless/partial refusal mutated metadata")
			}
			f.assertPoolsReusable(t)
		})
	}
}
