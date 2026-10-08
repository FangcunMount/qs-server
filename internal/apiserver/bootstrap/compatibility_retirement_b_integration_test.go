//go:build integration

package bootstrap

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	apiserverconfig "github.com/FangcunMount/qs-server/internal/apiserver/config"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	mysqlclient "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

type recoveryLocalPair struct {
	db        *sql.DB
	client    *mongo.Client
	name      string
	sqlConfig *mysqlclient.Config
}

func recoveryLocalDatabasePair(t *testing.T) *recoveryLocalPair {
	t.Helper()
	if os.Getenv("QS_COMPAT_B_LOCAL_ONLY") != "1" {
		if os.Getenv("QS_COMPAT_REQUIRE_DATABASE") == "1" {
			t.Fatal("recovery bootstrap requires disposable local harness")
		}
		t.Skip("disposable local recovery bootstrap is not enabled")
	}
	cfg, err := mysqlclient.ParseDSN(os.Getenv("MYSQL_DSN"))
	if err != nil {
		t.Fatal("invalid isolated recovery SQL configuration")
	}
	host, port, err := net.SplitHostPort(cfg.Addr)
	portNumber, portErr := strconv.Atoi(port)
	if err != nil || portErr != nil || host != "127.0.0.1" || cfg.Net != "tcp" || cfg.DBName != "" || portNumber < 1 || portNumber > 65535 {
		t.Fatal("non-isolated recovery SQL endpoint refused")
	}
	uri := os.Getenv("QS_SERVER_TEST_MONGO_URI")
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "mongodb" || u.Hostname() != "127.0.0.1" || strings.Contains(u.Host, ",") || u.Query().Get("directConnection") != "true" {
		t.Fatal("non-isolated recovery Mongo endpoint refused")
	}
	portNumber, err = strconv.Atoi(u.Port())
	if err != nil || portNumber < 1 || portNumber > 65535 {
		t.Fatal("non-isolated recovery Mongo port refused")
	}
	var suffix [12]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	name := "qs_compat_b_recovery_test_" + hex.EncodeToString(suffix[:])
	cfg.MultiStatements = true
	admin, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("open isolated recovery SQL admin")
	}
	if _, err := admin.ExecContext(t.Context(), "CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		_ = admin.Close()
		t.Fatal(err)
	}
	cfg.DBName = name
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal("open isolated recovery SQL fixture")
	}
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal("connect isolated recovery Mongo fixture")
	}
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		t.Fatal(err)
	}
	var hello struct {
		SetName string `bson:"setName"`
	}
	if err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello); err != nil || hello.SetName != "qs-compat-b-local" {
		t.Fatal("local recovery replica set required")
	}
	f := &recoveryLocalPair{db: db, client: client, name: name, sqlConfig: cfg}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := client.Database(name).Drop(ctx); err != nil {
			t.Errorf("drop own recovery Mongo database: %v", err)
		}
		if err := client.Disconnect(ctx); err != nil {
			t.Errorf("disconnect own recovery fixture client: %v", err)
		}
		if err := db.Close(); err != nil {
			t.Errorf("close own recovery fixture pool: %v", err)
		}
		if _, err := admin.ExecContext(ctx, "DROP DATABASE `"+name+"`"); err != nil {
			t.Errorf("drop own recovery SQL database: %v", err)
		}
		if err := admin.Close(); err != nil {
			t.Errorf("close own recovery SQL admin: %v", err)
		}
	})
	return f
}

func (f *recoveryLocalPair) seed(t *testing.T, sqlDirty, mongoDirty bool) {
	t.Helper()
	for _, statement := range []string{
		"CREATE TABLE schema_migrations(version BIGINT NOT NULL PRIMARY KEY,dirty BOOLEAN NOT NULL)",
		"CREATE TABLE b_recovery_protected(id BIGINT PRIMARY KEY,note VARCHAR(128) NOT NULL,KEY protected_note(note))",
		"INSERT INTO b_recovery_protected VALUES(1,'recovery protected original SQL fact')",
		"CREATE PROCEDURE b_recovery_protected_procedure() SELECT 7",
		"CREATE EVENT b_recovery_protected_event ON SCHEDULE AT CURRENT_TIMESTAMP + INTERVAL 1 DAY DO SELECT 1",
		"CREATE TRIGGER b_recovery_protected_trigger BEFORE INSERT ON b_recovery_protected FOR EACH ROW SET NEW.note=NEW.note",
	} {
		if _, err := f.db.ExecContext(t.Context(), statement); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.db.ExecContext(t.Context(), "INSERT INTO schema_migrations VALUES(99,?)", sqlDirty); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"domain_event_outbox", "ai_bridge_commands", "ai_messaging_legacy_commands"} {
		if _, err := f.db.ExecContext(t.Context(), "CREATE TABLE `"+name+"`(id BIGINT PRIMARY KEY,payload TEXT)"); err != nil {
			t.Fatal(err)
		}
		if _, err := f.db.ExecContext(t.Context(), "INSERT INTO `"+name+"` VALUES(1,'recovery retained original source')"); err != nil {
			t.Fatal(err)
		}
	}
	db := f.client.Database(f.name)
	if _, err := db.Collection("schema_migrations").InsertOne(t.Context(), bson.D{{Key: "_id", Value: "head"}, {Key: "version", Value: int64(38)}, {Key: "dirty", Value: mongoDirty}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Collection("b_recovery_protected").InsertOne(t.Context(), bson.D{{Key: "_id", Value: 1}, {Key: "note", Value: "recovery protected original Mongo fact"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Collection("b_recovery_protected").Indexes().CreateOne(t.Context(), mongo.IndexModel{Keys: bson.D{{Key: "note", Value: 1}}, Options: options.Index().SetName("protected_note")}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Collection("domain_event_outbox").InsertOne(t.Context(), bson.D{{Key: "_id", Value: 1}, {Key: "payload", Value: "recovery retained original source"}}); err != nil {
		t.Fatal(err)
	}
}

type recoverySnapshot struct {
	SQL   map[string][][]string
	Mongo map[string][]string
}

func recoverySQLRows(t *testing.T, db *sql.DB, query string, args ...any) [][]string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), query, args...)
	if err != nil {
		t.Fatal(err)
	}
	columns, err := rows.Columns()
	if err != nil {
		_ = rows.Close()
		t.Fatal(err)
	}
	result := [][]string{columns}
	for rows.Next() {
		values := make([]sql.NullString, len(columns))
		destinations := make([]any, len(columns))
		for i := range values {
			destinations[i] = &values[i]
		}
		if err := rows.Scan(destinations...); err != nil {
			_ = rows.Close()
			t.Fatal(err)
		}
		row := make([]string, len(columns))
		for i, value := range values {
			row[i] = fmt.Sprintf("%t:%s", value.Valid, value.String)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	return result
}

func recoveryMongoRows(t *testing.T, cursor *mongo.Cursor) []string {
	t.Helper()
	var rows []string
	for cursor.Next(t.Context()) {
		raw, err := bson.MarshalExtJSON(cursor.Current, true, false)
		if err != nil {
			_ = cursor.Close(t.Context())
			t.Fatal(err)
		}
		rows = append(rows, string(raw))
	}
	if err := cursor.Err(); err != nil {
		_ = cursor.Close(t.Context())
		t.Fatal(err)
	}
	if err := cursor.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	sort.Strings(rows)
	return rows
}

func (f *recoveryLocalPair) snapshot(t *testing.T) recoverySnapshot {
	t.Helper()
	s := recoverySnapshot{SQL: map[string][][]string{}, Mongo: map[string][]string{}}
	s.SQL["catalog"] = recoverySQLRows(t, f.db, "SELECT TABLE_NAME,TABLE_TYPE,ENGINE,TABLE_COLLATION FROM information_schema.tables WHERE table_schema=? ORDER BY TABLE_NAME", f.name)
	rows, err := f.db.QueryContext(t.Context(), "SELECT TABLE_NAME FROM information_schema.tables WHERE table_schema=? ORDER BY TABLE_NAME", f.name)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		s.SQL["create:"+name] = recoverySQLRows(t, f.db, "SHOW CREATE TABLE `"+name+"`")
		s.SQL["rows:"+name] = recoverySQLRows(t, f.db, "SELECT * FROM `"+name+"` ORDER BY 1")
	}
	s.SQL["routines"] = recoverySQLRows(t, f.db, "SELECT ROUTINE_NAME,ROUTINE_TYPE,ROUTINE_DEFINITION,SQL_DATA_ACCESS,SECURITY_TYPE,DEFINER FROM information_schema.routines WHERE routine_schema=? ORDER BY ROUTINE_NAME", f.name)
	s.SQL["triggers"] = recoverySQLRows(t, f.db, "SELECT TRIGGER_NAME,EVENT_MANIPULATION,EVENT_OBJECT_TABLE,ACTION_STATEMENT,ACTION_TIMING,DEFINER FROM information_schema.triggers WHERE trigger_schema=? ORDER BY TRIGGER_NAME", f.name)
	s.SQL["events"] = recoverySQLRows(t, f.db, "SELECT EVENT_NAME,EVENT_DEFINITION,EVENT_TYPE,EXECUTE_AT,STATUS,ON_COMPLETION,DEFINER FROM information_schema.events WHERE event_schema=? ORDER BY EVENT_NAME", f.name)
	db := f.client.Database(f.name)
	cursor, err := db.ListCollections(t.Context(), bson.D{})
	if err != nil {
		t.Fatal(err)
	}
	s.Mongo["catalog"] = recoveryMongoRows(t, cursor)
	collections, err := db.ListCollectionNames(t.Context(), bson.D{})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range collections {
		cursor, err := db.Collection(name).Find(t.Context(), bson.D{})
		if err != nil {
			t.Fatal(err)
		}
		s.Mongo["rows:"+name] = recoveryMongoRows(t, cursor)
		cursor, err = db.Collection(name).Indexes().List(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		s.Mongo["indexes:"+name] = recoveryMongoRows(t, cursor)
	}
	return s
}

func TestCompatibilityRetirementBDisabledRecoveryBootstrapDoesNotWrite(t *testing.T) {
	for _, sqlDirty := range []bool{false, true} {
		for _, mongoDirty := range []bool{false, true} {
			t.Run(fmt.Sprintf("sql_dirty_%t_mongo_dirty_%t", sqlDirty, mongoDirty), func(t *testing.T) {
				f := recoveryLocalDatabasePair(t)
				f.seed(t, sqlDirty, mongoDirty)
				before := f.snapshot(t)
				opts := apiserveroptions.NewOptions()
				opts.MigrationOptions.Enabled = false
				opts.MigrationOptions.AutoSeed = false
				opts.MySQLOptions.Host = f.sqlConfig.Addr
				opts.MySQLOptions.Username = f.sqlConfig.User
				opts.MySQLOptions.Password = f.sqlConfig.Passwd
				opts.MySQLOptions.Database = f.name
				opts.MySQLOptions.MaxOpenConnections = 1
				opts.MySQLOptions.MaxIdleConnections = 1
				opts.MySQLOptions.Location = "UTC"
				opts.MySQLOptions.SessionTimeZone = "+00:00"
				// An absent Redis configuration uses the real manager's optional
				// path. Merely clearing NewOptions.Host leaves its default port
				// configured and would connect to :6379 in the existing adapter.
				opts.RedisOptions = nil
				opts.RedisProfiles = nil
				opts.MongoDBOptions.URL = os.Getenv("QS_SERVER_TEST_MONGO_URI")
				opts.MongoDBOptions.Database = f.name
				opts.MongoDBOptions.EnableLogger = false
				cfg, err := apiserverconfig.CreateConfigFromOptions(opts)
				if err != nil {
					t.Fatal(err)
				}
				dm := NewDatabaseManager(cfg)
				closed := false
				t.Cleanup(func() {
					if !closed {
						if err := dm.Close(); err != nil {
							t.Errorf("close own recovery manager: %v", err)
						}
					}
				})
				if err := dm.Initialize(); err != nil {
					t.Fatalf("real disabled-recovery initialization failed: %v", err)
				}
				if after := f.snapshot(t); !reflect.DeepEqual(before, after) {
					t.Fatal("disabled-recovery initialization wrote heads, source data or nontarget schema")
				}
				gormDB, err := dm.GetMySQLDB()
				if err != nil {
					t.Fatal(err)
				}
				ownedPool, err := gormDB.DB()
				if err != nil {
					t.Fatal(err)
				}
				ownedClient, err := dm.GetMongoClient()
				if err != nil {
					t.Fatal(err)
				}
				if ownedPool == f.db || ownedClient == f.client {
					t.Fatal("manager borrowed fixture-owned connections")
				}
				if ownedPool.Stats().InUse != 0 || ownedPool.Stats().MaxOpenConnections != 1 {
					t.Fatal("manager pool was blocked or misconfigured")
				}
				if err := dm.HealthCheck(); err != nil {
					t.Fatal(err)
				}
				if err := dm.Close(); err != nil {
					t.Fatal(err)
				}
				closed = true
				ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
				defer cancel()
				if err := ownedPool.PingContext(ctx); err == nil {
					t.Fatal("manager did not close its owned SQL pool")
				}
				if err := ownedClient.Ping(ctx, readpref.Primary()); err == nil {
					t.Fatal("manager did not disconnect its owned Mongo client")
				}
				if err := f.db.PingContext(ctx); err != nil {
					t.Fatal("manager closed independent SQL fixture pool")
				}
				if err := f.client.Ping(ctx, readpref.Primary()); err != nil {
					t.Fatal("manager disconnected independent Mongo fixture client")
				}
				if after := f.snapshot(t); !reflect.DeepEqual(before, after) {
					t.Fatal("recovery manager shutdown modified metadata or nontarget schema")
				}
			})
		}
	}
}
