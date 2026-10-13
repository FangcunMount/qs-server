//go:build integration

package migration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	drivermysql "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type nativeSchemaPair struct {
	db      *sql.DB
	client  *mongo.Client
	mongoDB *mongo.Database
	cfg     PairConfig
}

func openNativeSchemaPair(t *testing.T) nativeSchemaPair {
	t.Helper()
	dsn := os.Getenv("QS_M3_TEST_MYSQL_DSN")
	if dsn == "" {
		if os.Getenv("QS_M3_REQUIRE_DATABASE") == "1" {
			t.Fatal("QS_M3_TEST_MYSQL_DSN required")
		}
		t.Skip("isolated M3 MySQL is not configured")
	}
	config, err := drivermysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal("invalid isolated MySQL configuration")
	}
	config.DBName, config.MultiStatements = "", true
	server, err := sql.Open("mysql", config.FormatDSN())
	if err != nil {
		t.Fatal("isolated MySQL unavailable")
	}
	name := fmt.Sprintf("qs_m3_test_%d", time.Now().UnixNano())
	if _, err := server.ExecContext(t.Context(), "CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		_ = server.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = server.ExecContext(context.Background(), "DROP DATABASE IF EXISTS `"+name+"`")
		_ = server.Close()
	})
	config.DBName = name
	db, err := sql.Open("mysql", config.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	client, mongoDB := mongodbtest.ReplicaSetDatabase(t)
	return nativeSchemaPair{db, client, mongoDB, PairConfig{MySQLDatabase: name, MongoDatabase: mongoDB.Name(), ExpectedSourceSHA: strings.Repeat("a", 40)}}
}

func (n *nativeSchemaPair) approve(t *testing.T, r *migrationResources) {
	t.Helper()
	cfg, a := testBootstrapApproval(t, r)
	cfg.MySQLDatabase, cfg.MongoDatabase = n.cfg.MySQLDatabase, n.cfg.MongoDatabase
	a.MySQLDatabase, a.MongoDatabase = cfg.MySQLDatabase, cfg.MongoDatabase
	var err error
	a.MySQLIdentitySHA256, a.MongoClusterSHA256, err = pairIdentity(t.Context(), n.db, n.client, cfg.MySQLDatabase, cfg.MongoDatabase)
	if err != nil {
		t.Fatal(err)
	}
	writeBootstrapApproval(t, &cfg, a)
	n.cfg = cfg
}

func (n nativeSchemaPair) preflight(t *testing.T, r *migrationResources) *PairPreflight {
	t.Helper()
	p, err := preflightCompatibilityPair(t.Context(), n.db, n.client, n.cfg, r)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func (n nativeSchemaPair) runBoth(t *testing.T, p *PairPreflight) {
	t.Helper()
	if version, _, err := NewMigrator(n.db, p.MySQLConfig(false)).Run(); err != nil || version != p.resources.sql.head {
		t.Fatalf("SQL version=%d error=%v", version, err)
	}
	if version, _, err := NewMongoMigrator(n.client, p.MongoConfig(false)).Run(); err != nil || version != p.resources.mongo.head {
		t.Fatalf("Mongo version=%d error=%v", version, err)
	}
	if err := p.VerifyComplete(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := n.db.PingContext(t.Context()); err != nil {
		t.Fatal("borrowed SQL pool closed", err)
	}
	if err := n.client.Ping(t.Context(), nil); err != nil {
		t.Fatal("borrowed Mongo client closed", err)
	}
}

func (n nativeSchemaPair) heads(t *testing.T) (uint, bool, uint, bool) {
	t.Helper()
	var sqlVersion uint
	var sqlDirty bool
	if err := n.db.QueryRowContext(t.Context(), "SELECT version,dirty FROM schema_migrations").Scan(&sqlVersion, &sqlDirty); err != nil {
		t.Fatal(err)
	}
	var mongoHead struct {
		Version uint `bson:"version"`
		Dirty   bool `bson:"dirty"`
	}
	if err := n.mongoDB.Collection("schema_migrations").FindOne(t.Context(), bson.D{}).Decode(&mongoHead); err != nil {
		t.Fatal(err)
	}
	return sqlVersion, sqlDirty, mongoHead.Version, mongoHead.Dirty
}

func currentNativeSchemaPair(t *testing.T) (nativeSchemaPair, *migrationResources) {
	t.Helper()
	r, err := compiledMigrationResources()
	if err != nil {
		t.Fatal(err)
	}
	n := openNativeSchemaPair(t)
	n.approve(t, r)
	n.runBoth(t, n.preflight(t, r))
	return n, r
}

func TestSchemaPairNativeColdBootstrapAndRestart(t *testing.T) {
	for _, future := range []bool{false, true} {
		t.Run(fmt.Sprintf("future_%v", future), func(t *testing.T) {
			r, err := compiledMigrationResources()
			if future {
				r, err = readMigrationResources(futureMigrationSource(t), []schemaVersionPair{{100, 39}, {101, 39}, {101, 40}})
			}
			if err != nil {
				t.Fatal(err)
			}
			n := openNativeSchemaPair(t)
			if _, err := preflightCompatibilityPair(t.Context(), n.db, n.client, n.cfg, r); err == nil {
				t.Fatal("unapproved pristine pair accepted")
			}
			n.approve(t, r)
			p := n.preflight(t, r)
			if err := p.VerifyComplete(t.Context()); err == nil {
				t.Fatal("pristine pair allowed with disabled DDL")
			}
			if _, err := os.Stat(n.cfg.BootstrapAuthorizationFile + ".consumed"); !os.IsNotExist(err) {
				t.Fatal("read-only verification consumed approval")
			}
			n.runBoth(t, p)
			if _, err := os.Stat(n.cfg.BootstrapAuthorizationFile + ".consumed"); err != nil {
				t.Fatal("cold approval was not consumed", err)
			}
			n.runBoth(t, n.preflight(t, r))
			if sql, dirty, mongo, mongoDirty := n.heads(t); sql != r.sql.head || mongo != r.mongo.head || dirty || mongoDirty {
				t.Fatalf("heads=%d/%d dirty=%v/%v", sql, mongo, dirty, mongoDirty)
			}
		})
	}
}

func TestSchemaPairNativeIndependentReleaseRestart(t *testing.T) {
	n, _ := currentNativeSchemaPair(t)
	r, err := readMigrationResources(futureMigrationSource(t), []schemaVersionPair{{100, 39}, {101, 39}, {101, 40}})
	if err != nil {
		t.Fatal(err)
	}
	p := n.preflight(t, r)
	if _, _, err := NewMongoMigrator(n.client, p.MongoConfig(false)).Run(); err == nil {
		t.Fatal("Mongo ran before SQL success")
	}
	// A failed live pair is poisoned even though the installed start was clean.
	if _, _, err := NewMigrator(n.db, p.MySQLConfig(false)).Run(); err == nil {
		t.Fatal("failed live pair reused")
	}
	p = n.preflight(t, r)
	if version, _, err := NewMigrator(n.db, p.MySQLConfig(false)).Run(); err != nil || version != 101 {
		t.Fatal(version, err)
	}
	if err := p.VerifyComplete(t.Context()); err == nil {
		t.Fatal("101/39 claimed to be final 101/40")
	}
	// 101/39 is an independently reviewed release, so a new process may continue.
	n.runBoth(t, n.preflight(t, r))
	assertMySQLTable(t, n.db, n.cfg.MySQLDatabase, "migration_future_probe", true)
	names, err := n.mongoDB.ListCollectionNames(t.Context(), bson.M{"name": "migration_future_probe"})
	if err != nil || len(names) != 1 {
		t.Fatal(names, err)
	}
}

func TestSchemaPairNativeLegacyBoundaryRejectsFreshPartial(t *testing.T) {
	n := openNativeSchemaPair(t)
	if _, _, err := NewMigrator(n.db, &Config{Enabled: true, Database: n.cfg.MySQLDatabase}).run(99); err != nil {
		t.Fatal(err)
	}
	if _, _, err := NewMongoMigrator(n.client, &Config{Enabled: true, Database: n.cfg.MongoDatabase}).run(38); err != nil {
		t.Fatal(err)
	}
	for _, name := range retirementSQLNames {
		if _, err := n.db.ExecContext(t.Context(), "DROP TABLE `"+name+"`"); err != nil {
			t.Fatal(err)
		}
	}
	if err := n.mongoDB.Collection("domain_event_outbox").Drop(t.Context()); err != nil {
		t.Fatal(err)
	}
	r, err := compiledMigrationResources()
	if err != nil {
		t.Fatal(err)
	}
	p := n.preflight(t, r)
	if _, _, err := NewMigrator(n.db, p.MySQLConfig(false)).Run(); err != nil {
		t.Fatal(err)
	}
	if _, err := preflightCompatibilityPair(t.Context(), n.db, n.client, n.cfg, r); err == nil {
		t.Fatal("fresh process adopted 100/38")
	}
	if _, _, err := NewMongoMigrator(n.client, p.MongoConfig(false)).Run(); err != nil {
		t.Fatal(err)
	}
	if err := p.VerifyComplete(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name            string
		change, restore func() error
	}{
		{"sql_dirty", func() error { _, e := n.db.Exec("UPDATE schema_migrations SET dirty=TRUE"); return e }, func() error { _, e := n.db.Exec("UPDATE schema_migrations SET dirty=FALSE"); return e }},
		{"mongo_dirty", func() error {
			_, e := n.mongoDB.Collection("schema_migrations").UpdateOne(t.Context(), bson.D{}, bson.M{"$set": bson.M{"dirty": true}})
			return e
		}, func() error {
			_, e := n.mongoDB.Collection("schema_migrations").UpdateOne(t.Context(), bson.D{}, bson.M{"$set": bson.M{"dirty": false}})
			return e
		}},
		{"future_head", func() error { _, e := n.db.Exec("UPDATE schema_migrations SET version=101"); return e }, func() error { _, e := n.db.Exec("UPDATE schema_migrations SET version=100"); return e }},
		{"old_sql_reappears", func() error { _, e := n.db.Exec("CREATE TABLE domain_event_outbox (id BIGINT PRIMARY KEY)"); return e }, func() error { _, e := n.db.Exec("DROP TABLE domain_event_outbox"); return e }},
		{"old_mongo_reappears", func() error { return n.mongoDB.CreateCollection(t.Context(), "domain_event_outbox") }, func() error { return n.mongoDB.Collection("domain_event_outbox").Drop(t.Context()) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.change(); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := test.restore(); err != nil {
					t.Error(err)
				}
			}()
			if _, err := preflightCompatibilityPair(t.Context(), n.db, n.client, n.cfg, r); err == nil {
				t.Fatal("unsafe installed pair accepted")
			}
		})
	}
}

func TestSchemaPairNativeFailedFutureDDLDoesNotAdvanceOtherBackend(t *testing.T) {
	for _, failedBackend := range []Backend{BackendMySQL, BackendMongo} {
		t.Run(string(failedBackend), func(t *testing.T) {
			n, _ := currentNativeSchemaPair(t)
			source := futureMigrationSource(t)
			if failedBackend == BackendMySQL {
				source["migrations/mysql/000101_future_test.up.sql"].Data = []byte("CREATE TABLE migration_future_probe (id BIGINT PRIMARY KEY); SELECT * FROM missing_m3_probe;")
			} else {
				source["migrations/mongodb/000040_future_test.up.json"].Data = []byte(`[{"create":"migration_future_probe"},{"unknown_m3_command":1}]`)
			}
			r, err := readMigrationResources(source, []schemaVersionPair{{100, 39}, {101, 39}, {101, 40}})
			if err != nil {
				t.Fatal(err)
			}
			p := n.preflight(t, r)
			_, _, sqlErr := NewMigrator(n.db, p.MySQLConfig(false)).Run()
			if failedBackend == BackendMySQL && sqlErr == nil {
				t.Fatal("invalid SQL did not fail")
			}
			if failedBackend == BackendMongo && sqlErr != nil {
				t.Fatal(sqlErr)
			}
			if _, _, err := NewMongoMigrator(n.client, p.MongoConfig(false)).Run(); err == nil {
				t.Fatal("failed/unknown phase accepted")
			}
			sqlVersion, sqlDirty, mongoVersion, mongoDirty := n.heads(t)
			if sqlVersion != 101 || sqlDirty != (failedBackend == BackendMySQL) || mongoVersion != map[Backend]uint{BackendMySQL: 39, BackendMongo: 40}[failedBackend] || mongoDirty != (failedBackend == BackendMongo) {
				t.Fatalf("heads=%d/%d dirty=%v/%v", sqlVersion, mongoVersion, sqlDirty, mongoDirty)
			}
			if _, err := preflightCompatibilityPair(t.Context(), n.db, n.client, n.cfg, r); err == nil {
				t.Fatal("dirty failed pair adopted")
			}
		})
	}
}
