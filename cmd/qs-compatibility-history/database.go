package main

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	retirement "github.com/FangcunMount/qs-server/internal/apiserver/maintenance/compatibilityretirement"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	identitymeta "github.com/FangcunMount/qs-server/internal/pkg/databaseidentity"
	drivermysql "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Only this executable owns connections/sessions/transaction lifecycle. Every
// maintenance adapter receives an already active borrowed paired read scope.
type historyDatabase struct {
	sqlPool         *sql.DB
	sql             *gorm.DB
	mongoClient     *mongo.Client
	mongo           *mongo.Database
	mongoConfig     retirement.MongoOwnerConfig
	sqlHead         uint64
	namespaceAnchor *identitymeta.MongoNamespaceAnchor
}

func connectionValue(key string) (string, error) {
	value := os.Getenv(key)
	if value == "" || strings.ContainsAny(value, "\x00\r\n") {
		return "", fixedError("history_connection_input_rejected")
	}
	return value, nil
}
func connectionPort(key string, fallback int) (int, error) {
	if os.Getenv(key) == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(os.Getenv(key))
	if err != nil || value < 1 || value > 65535 {
		return 0, fixedError("history_connection_input_rejected")
	}
	return value, nil
}
func openDatabases(ctx context.Context, a *approvedInputs) (db *historyDatabase, result error) {
	if ctx == nil || a == nil || ctx.Err() != nil {
		return nil, fixedError("history_connection_input_rejected")
	}
	db = &historyDatabase{sqlHead: a.inventory.Migrations["mysql"]}
	defer func() {
		if result != nil {
			_ = db.close()
			db = nil
		}
	}()
	host, err := connectionValue("MYSQL_HOST")
	if err != nil {
		return db, err
	}
	user, err := connectionValue("MYSQL_USERNAME")
	if err != nil {
		return db, err
	}
	password, err := connectionValue("MYSQL_PASSWORD")
	if err != nil {
		return db, err
	}
	namespace, err := connectionValue("MYSQL_DATABASE")
	if err != nil {
		return db, err
	}
	port, err := connectionPort("MYSQL_PORT", 3306)
	if err != nil {
		return db, err
	}
	c := drivermysql.NewConfig()
	c.User = user
	c.Passwd = password
	c.Net = "tcp"
	c.Addr = net.JoinHostPort(host, strconv.Itoa(port))
	c.DBName = namespace
	c.ParseTime = true
	c.Loc = time.UTC
	c.Timeout = 10 * time.Second
	c.ReadTimeout = 30 * time.Second
	c.WriteTimeout = 30 * time.Second
	c.MultiStatements = false
	db.sqlPool, err = sql.Open("mysql", c.FormatDSN())
	if err != nil {
		return db, fixedError("history_sql_connect_failed")
	}
	db.sqlPool.SetMaxOpenConns(1)
	db.sqlPool.SetMaxIdleConns(1)
	if db.sqlPool.PingContext(ctx) != nil {
		return db, fixedError("history_sql_connect_failed")
	}
	db.sql, err = gorm.Open(gormmysql.New(gormmysql.Config{Conn: db.sqlPool, SkipInitializeWithVersion: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableAutomaticPing: true, SkipDefaultTransaction: true})
	if err != nil {
		return db, fixedError("history_sql_connect_failed")
	}
	host, err = connectionValue("MONGODB_HOST")
	if err != nil {
		return db, err
	}
	user, err = connectionValue("MONGODB_USERNAME")
	if err != nil {
		return db, err
	}
	password, err = connectionValue("MONGODB_PASSWORD")
	if err != nil {
		return db, err
	}
	namespace, err = connectionValue("MONGODB_DBNAME")
	if err != nil {
		return db, err
	}
	port, err = connectionPort("MONGODB_PORT", 27017)
	if err != nil {
		return db, err
	}
	if a.inventory.MongoNamespaceAnchor != nil {
		endpoint, endpointErr := identitymeta.MongoEndpointSHA256(host, port, namespace)
		if endpointErr != nil || a.inventory.MongoNamespaceAnchor.Validate() != nil || a.inventory.MongoNamespaceAnchor.Database != namespace || a.inventory.MongoNamespaceAnchor.EndpointSHA256 != endpoint {
			return db, fixedError("history_database_binding_rejected")
		}
		db.namespaceAnchor = a.inventory.MongoNamespaceAnchor.Clone()
	}
	opts := options.Client().SetHosts([]string{net.JoinHostPort(host, strconv.Itoa(port))}).SetAuth(options.Credential{Username: user, Password: password, AuthSource: "admin"}).SetConnectTimeout(10 * time.Second).SetServerSelectionTimeout(10 * time.Second).SetSocketTimeout(30 * time.Second).SetMaxPoolSize(1).SetReadPreference(readpref.Primary()).SetReadConcern(readconcern.Majority())
	db.mongoClient, err = mongo.Connect(ctx, opts)
	if err != nil {
		return db, fixedError("history_mongo_connect_failed")
	}
	if db.mongoClient.Ping(ctx, readpref.Primary()) != nil {
		return db, fixedError("history_mongo_connect_failed")
	}
	db.mongo = db.mongoClient.Database(namespace)
	mongoHead := a.inventory.Migrations["mongodb"]
	if mongoHead == 0 || mongoHead > 1<<63-1 {
		return db, fixedError("history_migration_binding_rejected")
	}
	db.mongoConfig = retirement.MongoOwnerConfig{ExpectedIdentityHash: a.inventory.Identities["mongodb"], ExpectedMigrationVersion: int64(mongoHead)}
	return db, nil
}
func (d *historyDatabase) close() error {
	if d == nil {
		return nil
	}
	var result error
	if d.mongoClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if d.mongoClient.Disconnect(ctx) != nil {
			result = fixedError("history_mongo_close_failed")
		}
		cancel()
		d.mongoClient = nil
	}
	if d.sqlPool != nil {
		if d.sqlPool.Close() != nil && result == nil {
			result = fixedError("history_sql_close_failed")
		}
		d.sqlPool = nil
	}
	return result
}
func (d *historyDatabase) epoch(ctx context.Context, fn func(context.Context) error) (result error) {
	if d == nil || d.sql == nil || d.mongo == nil || d.mongoClient == nil || ctx == nil || ctx.Err() != nil || fn == nil {
		return fixedError("history_host_epoch_rejected")
	}
	if err := d.validateNamespaceAnchor(ctx); err != nil {
		return err
	}
	// Register before the transaction cleanup defers so metadata is read after
	// actual abort/rollback, without replacing an original pipeline error.
	defer func() {
		if err := d.validateNamespaceAnchor(ctx); result == nil && err != nil {
			result = err
		}
	}()
	session, err := d.mongoClient.StartSession()
	if err != nil {
		return fixedError("history_host_epoch_rejected")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		session.EndSession(cleanup)
		cancel()
	}()
	if session.StartTransaction(options.Transaction().SetReadConcern(readconcern.Snapshot()).SetReadPreference(readpref.Primary())) != nil {
		return fixedError("history_host_epoch_rejected")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if session.AbortTransaction(cleanup) != nil && result == nil {
			result = fixedError("history_host_mongo_abort_failed")
		}
		cancel()
	}()
	tx := d.sql.WithContext(ctx).Begin(&sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if tx.Error != nil {
		return fixedError("history_host_epoch_rejected")
	}
	defer func() {
		if e := tx.Rollback().Error; e != nil && !errors.Is(e, sql.ErrTxDone) && result == nil {
			result = fixedError("history_host_sql_rollback_failed")
		}
	}()
	q, cancel := context.WithTimeout(ctx, 30*time.Second)
	rows, err := tx.WithContext(q).Raw("SELECT CAST(version AS BINARY),CAST(dirty AS BINARY) FROM schema_migrations ORDER BY version LIMIT 2").Rows()
	if err != nil {
		cancel()
		return fixedError("history_migration_binding_rejected")
	}
	var versions []string
	for rows.Next() {
		var version, dirty string
		if rows.Scan(&version, &dirty) != nil || dirty != "0" {
			_ = rows.Close()
			cancel()
			return fixedError("history_migration_binding_rejected")
		}
		versions = append(versions, version)
	}
	failed := rows.Err() != nil
	closeFailed := rows.Close() != nil
	cancel()
	if failed || closeFailed || len(versions) != 1 || versions[0] != strconv.FormatUint(d.sqlHead, 10) {
		return fixedError("history_migration_binding_rejected")
	}
	paired := mongo.NewSessionContext(hostmysql.WithTx(ctx, tx), session)
	return fn(paired)
}

func (d *historyDatabase) validateNamespaceAnchor(ctx context.Context) error {
	if d.namespaceAnchor == nil {
		return nil
	}
	observed, err := identitymeta.ObserveMongoNamespaceAnchor(ctx, d.mongo, d.namespaceAnchor.EndpointSHA256)
	if err != nil || !identitymeta.MatchMongoNamespaceAnchors(d.namespaceAnchor, observed) {
		return fixedError("history_database_binding_rejected")
	}
	return nil
}
