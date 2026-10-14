//go:build integration

package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/database"
	"github.com/FangcunMount/qs-server/internal/apiserver/config"
	"github.com/FangcunMount/qs-server/internal/apiserver/options"
	"github.com/FangcunMount/qs-server/internal/pkg/migration"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	drivermysql "github.com/go-sql-driver/mysql"
	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Only adapts test-owned live clients into the existing registry.
type nativeGuardConnection struct {
	kind   database.DatabaseType
	client any
}

func (c nativeGuardConnection) Type() database.DatabaseType     { return c.kind }
func (nativeGuardConnection) Connect() error                    { return nil }
func (nativeGuardConnection) Close() error                      { return nil }
func (nativeGuardConnection) HealthCheck(context.Context) error { return nil }
func (c nativeGuardConnection) GetClient() any                  { return c.client }

func disabledNativeManager(t *testing.T, installed bool) (*DatabaseManager, *sql.DB, *mongo.Database) {
	t.Helper()
	dsn := os.Getenv("QS_M3_TEST_MYSQL_DSN")
	if dsn == "" {
		if os.Getenv("QS_M3_REQUIRE_DATABASE") == "1" {
			t.Fatal("QS_M3_TEST_MYSQL_DSN required")
		}
		t.Skip("isolated M3 MySQL not configured")
	}
	sqlConfig, err := drivermysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal("invalid isolated MySQL configuration")
	}
	sqlConfig.DBName, sqlConfig.MultiStatements = "", true
	server, err := sql.Open("mysql", sqlConfig.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("qs_m3_bootstrap_test_%d", time.Now().UnixNano())
	if _, err := server.ExecContext(t.Context(), "CREATE DATABASE `"+name+"`"); err != nil {
		_ = server.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = server.ExecContext(context.Background(), "DROP DATABASE IF EXISTS `"+name+"`")
		_ = server.Close()
	})
	sqlConfig.DBName = name
	db, err := sql.Open("mysql", sqlConfig.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	client, mongoDB := mongodbtest.ReplicaSetDatabase(t)
	if installed {
		sqlHead, mongoHead, _, err := migration.EmbeddedMigrationResources()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(t.Context(), fmt.Sprintf("CREATE TABLE schema_migrations (version BIGINT NOT NULL PRIMARY KEY, dirty BOOLEAN NOT NULL); INSERT INTO schema_migrations VALUES(%d,FALSE)", sqlHead)); err != nil {
			t.Fatal(err)
		}
		if _, err := mongoDB.Collection("schema_migrations").InsertOne(t.Context(), bson.M{"version": int64(mongoHead), "dirty": false}); err != nil {
			t.Fatal(err)
		}
	}
	orm, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	opts := options.NewOptions()
	opts.MigrationOptions.Enabled = false
	opts.MySQLOptions.Database, opts.MongoDBOptions.Database = name, mongoDB.Name()
	dm := NewDatabaseManager(&config.Config{Options: opts})
	if err := dm.registry.Register(database.MySQL, nil, nativeGuardConnection{database.MySQL, orm}); err != nil {
		t.Fatal(err)
	}
	if err := dm.registry.Register(database.MongoDB, nil, nativeGuardConnection{database.MongoDB, client}); err != nil {
		t.Fatal(err)
	}
	previous := viper.Get("mongodb.database")
	viper.Set("mongodb.database", mongoDB.Name())
	t.Cleanup(func() { viper.Set("mongodb.database", previous) })
	return dm, db, mongoDB
}

func TestDisabledDDLNativeValidatesCurrentHeadsAndNeverCreatesMetadata(t *testing.T) {
	for _, installed := range []bool{false, true} {
		t.Run(fmt.Sprintf("installed_%v", installed), func(t *testing.T) {
			dm, db, mongoDB := disabledNativeManager(t, installed)
			err := dm.runMigrations(t.Context())
			if (err == nil) != installed {
				t.Fatalf("installed=%v error=%v", installed, err)
			}
			var tables int
			if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE()").Scan(&tables); err != nil {
				t.Fatal(err)
			}
			names, err := mongoDB.ListCollectionNames(t.Context(), bson.D{})
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if installed {
				want = 1
			}
			if tables != want || len(names) != want {
				t.Fatalf("disabled DDL mutated namespaces: SQL=%d Mongo=%v", tables, names)
			}
		})
	}
}

func TestDisabledDDLNativeRejectsDirtyPartialOldAndUnknown(t *testing.T) {
	dm, db, mongoDB := disabledNativeManager(t, true)
	sqlHead, _, _, err := migration.EmbeddedMigrationResources()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name            string
		change, restore func() error
	}{
		{"previous_declared_release", func() error { _, e := db.Exec("UPDATE schema_migrations SET version=100"); return e }, func() error {
			_, e := db.Exec(fmt.Sprintf("UPDATE schema_migrations SET version=%d", sqlHead))
			return e
		}},
		{"sql_dirty", func() error { _, e := db.Exec("UPDATE schema_migrations SET dirty=TRUE"); return e }, func() error { _, e := db.Exec("UPDATE schema_migrations SET dirty=FALSE"); return e }},
		{"mongo_dirty", func() error {
			_, e := mongoDB.Collection("schema_migrations").UpdateOne(t.Context(), bson.D{}, bson.M{"$set": bson.M{"dirty": true}})
			return e
		}, func() error {
			_, e := mongoDB.Collection("schema_migrations").UpdateOne(t.Context(), bson.D{}, bson.M{"$set": bson.M{"dirty": false}})
			return e
		}},
		{"partial", func() error { _, e := db.Exec("UPDATE schema_migrations SET version=99"); return e }, func() error {
			_, e := db.Exec(fmt.Sprintf("UPDATE schema_migrations SET version=%d", sqlHead))
			return e
		}},
		{"unknown_future", func() error {
			_, e := db.Exec(fmt.Sprintf("UPDATE schema_migrations SET version=%d", sqlHead+1))
			return e
		}, func() error {
			_, e := db.Exec(fmt.Sprintf("UPDATE schema_migrations SET version=%d", sqlHead))
			return e
		}},
		{"retired_sql", func() error { _, e := db.Exec("CREATE VIEW domain_event_outbox AS SELECT 1 AS id"); return e }, func() error { _, e := db.Exec("DROP VIEW domain_event_outbox"); return e }},
		{"retired_mongo", func() error { return mongoDB.CreateCollection(t.Context(), "domain_event_outbox") }, func() error { return mongoDB.Collection("domain_event_outbox").Drop(t.Context()) }},
		{"selected_identity", func() error { dm.config.MySQLOptions.Database += "_wrong"; return nil }, func() error {
			dm.config.MySQLOptions.Database = dm.config.MySQLOptions.Database[:len(dm.config.MySQLOptions.Database)-6]
			return nil
		}},
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
			if err := dm.runMigrations(t.Context()); err == nil {
				t.Fatal("disabled DDL bypassed failed read-only guard")
			}
		})
	}
}
