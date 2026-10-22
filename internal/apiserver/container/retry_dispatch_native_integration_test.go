//go:build integration && m3_dispatch_factory

package container

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	mongoTx "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/eventtransaction"
	mysqlTx "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventtransaction"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	catalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/pkg/messagingruntime"
	generic "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/reliable-messaging/transaction"
	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	mongooptions "go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// This entry exercises the actual default Container factory with original
// stores. It intentionally never starts senders or claims a business/Broker test.
func TestM3DefaultDispatchFactory(t *testing.T) {
	require.True(t, strings.HasPrefix(os.Getenv("QS_M3_FIXTURE_PROJECT"), "rocketmq-m2-"))
	cfg, err := driver.ParseDSN(os.Getenv("QS_M3_FACTORY_DSN"))
	require.NoError(t, err)
	require.Empty(t, cfg.DBName)
	require.Equal(t, "tcp", cfg.Net)
	host, _, err := net.SplitHostPort(cfg.Addr)
	require.NoError(t, err)
	require.True(t, net.ParseIP(host).IsLoopback())
	cfg.ParseTime, cfg.MultiStatements, cfg.Loc = true, true, time.UTC
	cfg.Params = map[string]string{"time_zone": "'+00:00'"}
	admin, err := sql.Open("mysql", cfg.FormatDSN())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, admin.Close()) })
	name := fmt.Sprintf("qs_m3_factory_%d", time.Now().UnixNano())
	_, err = admin.ExecContext(t.Context(), "CREATE DATABASE `"+name+"`")
	require.NoError(t, err)
	var dropSQLOnce sync.Once
	dropSQL := func() {
		dropSQLOnce.Do(func() {
			_, e := admin.ExecContext(context.Background(), "DROP DATABASE `"+name+"`")
			require.NoError(t, e)
		})
	}
	t.Cleanup(dropSQL)
	cfg.DBName = name
	db, err := gorm.Open(gormmysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(4)
	pool.SetMaxIdleConns(2)
	var poolOnce sync.Once
	closeSQL := func() { poolOnce.Do(func() { require.NoError(t, pool.Close()) }) }
	t.Cleanup(closeSQL)
	mongoURI := os.Getenv("QS_M3_FACTORY_MONGO_URI")
	parsedMongo, parseErr := url.Parse(mongoURI)
	require.NoError(t, parseErr)
	require.Equal(t, "mongodb", parsedMongo.Scheme)
	require.Nil(t, parsedMongo.User)
	require.True(t, net.ParseIP(parsedMongo.Hostname()).IsLoopback())
	require.Equal(t, "sdk-proof", parsedMongo.Query().Get("replicaSet"))
	require.Equal(t, "true", parsedMongo.Query().Get("directConnection"))
	client, err := mongo.Connect(t.Context(), mongooptions.Client().ApplyURI(mongoURI).SetMaxPoolSize(4).SetServerSelectionTimeout(2*time.Second))
	require.NoError(t, err)
	var disconnectOnce sync.Once
	disconnect := func() { disconnectOnce.Do(func() { require.NoError(t, client.Disconnect(context.Background())) }) }
	t.Cleanup(disconnect)
	require.NoError(t, client.Ping(t.Context(), readpref.Primary()))
	documents := client.Database(name)
	var dropMongoOnce sync.Once
	dropMongo := func() { dropMongoOnce.Do(func() { require.NoError(t, documents.Drop(context.Background())) }) }
	t.Cleanup(dropMongo)
	raw, err := os.ReadFile(os.Getenv("QS_M3_FACTORY_OPTIONS"))
	require.NoError(t, err)
	var options generic.MessagingOptions
	require.NoError(t, json.Unmarshal(raw, &options))
	require.NotNil(t, options.RocketMQ)
	confHost, _, err := net.SplitHostPort(options.RocketMQ.Endpoint)
	require.NoError(t, err)
	require.True(t, net.ParseIP(confHost).IsLoopback())
	require.Len(t, options.RocketMQ.Transactions, 2)
	source, err := catalog.Load("../../../configs/events.yaml")
	require.NoError(t, err)
	routes := catalog.NewCatalog(source)
	normal, err := messagingruntime.NewNormalWirePublisher(&options, []string{"qs.survey.lifecycle", "qs.plan.task"}, 4)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, normal.Close(context.Background())) })
	events, err := eventsubsystem.NewWithTransactionOwners(eventsubsystem.Options{MySQLDB: db, MongoDB: documents, Catalog: routes, WirePublisher: normal, PublisherMode: eventruntime.PublishModeMQ}, &options)
	require.NoError(t, err)
	sqlOwner, documentOwner := events.MySQLTransactionOwner(), events.MongoTransactionOwner()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		require.NoError(t, documentOwner.Runtime().Close(ctx))
		require.NoError(t, sqlOwner.Runtime().Close(ctx))
	})
	container := NewContainerWithOptions(db, documents, nil, ContainerOptions{EventSubsystem: events, Transactions: &EventTransactionBindings{MySQL: sqlOwner, Mongo: documentOwner, Routes: routes}, Silent: true})
	t.Cleanup(func() { require.NoError(t, container.Cleanup()) })
	countTables := func() int {
		var n int
		require.NoError(t, pool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=?", name).Scan(&n))
		return n
	}
	collections := func() []string {
		names, e := documents.ListCollectionNames(t.Context(), bson.D{})
		require.NoError(t, e)
		sort.Strings(names)
		return names
	}
	beforeTables, beforeCollections := countTables(), collections()
	factory, err := container.BuildRetryDispatchers(true)
	require.NoError(t, err)
	require.Equal(t, beforeTables, countTables())
	require.Equal(t, beforeCollections, collections())
	require.Same(t, db, container.MySQLDB())
	require.Same(t, documents, container.MongoDB())
	require.Same(t, sqlOwner, container.EvaluationTransactionOwner())
	require.Same(t, documentOwner, container.ReportTransactionOwner())
	require.False(t, sqlOwner.Runtime().Stats().Ready)
	require.False(t, documentOwner.Runtime().Stats().Ready)
	cases := map[string]bool{"original_handles_and_inert_factory": true}
	require.ErrorIs(t, factory.Validate(t.Context()), transaction.ErrSchema)
	cases["missing_schema_rejected_before_loops"] = true
	sqlMigration := func(file string) {
		body, e := os.ReadFile(filepath.Join("..", "..", "pkg", "migration", "migrations", "mysql", file))
		require.NoError(t, e)
		_, e = pool.ExecContext(t.Context(), string(body))
		require.NoError(t, e)
	}
	sqlMigration("000106_transaction_messaging.up.sql")
	sqlMigration("000107_business_retry_schedule.up.sql")
	sb, mb := sqlOwner.Binding(), documentOwner.Binding()
	_, err = pool.ExecContext(t.Context(), "INSERT INTO rm_tx_binding VALUES(?,?,?,1)", []byte(sb.Owner), []byte(sb.StoreID), []byte(sb.WriterEpoch))
	require.NoError(t, err)
	for _, file := range []string{"000041_transaction_messaging.up.json", "000042_business_retry_schedule.up.json"} {
		body, e := os.ReadFile(filepath.Join("..", "..", "pkg", "migration", "migrations", "mongodb", file))
		require.NoError(t, e)
		var commands []json.RawMessage
		require.NoError(t, json.Unmarshal(body, &commands))
		for _, command := range commands {
			var doc bson.D
			require.NoError(t, bson.UnmarshalExtJSON(command, false, &doc))
			require.NoError(t, documents.RunCommand(t.Context(), doc).Err())
		}
	}
	_, err = documents.Collection("rm_tx_binding").InsertOne(t.Context(), bson.M{"_id": mb.Owner, "store_id": mb.StoreID, "writer_epoch": mb.WriterEpoch, "schema_version": 1})
	require.NoError(t, err)
	require.NoError(t, factory.Validate(t.Context()))
	cases["explicit_dual_store_preflight"] = true
	foreignPool, err := sql.Open("mysql", cfg.FormatDSN())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, foreignPool.Close()) })
	foreignDB, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: foreignPool, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	wrongSQL := NewContainerWithOptions(foreignDB, documents, nil, ContainerOptions{EventSubsystem: events, Transactions: &EventTransactionBindings{MySQL: sqlOwner, Mongo: documentOwner, Routes: routes}, Silent: true})
	t.Cleanup(func() { require.NoError(t, wrongSQL.Cleanup()) })
	_, err = wrongSQL.BuildRetryDispatchers(true)
	require.Error(t, err)
	wrongMongo := NewContainerWithOptions(db, client.Database(name+"_foreign"), nil, ContainerOptions{EventSubsystem: events, Transactions: &EventTransactionBindings{MySQL: sqlOwner, Mongo: documentOwner, Routes: routes}, Silent: true})
	t.Cleanup(func() { require.NoError(t, wrongMongo.Cleanup()) })
	_, err = wrongMongo.BuildRetryDispatchers(true)
	require.Error(t, err)
	cases["foreign_original_handles_rejected"] = true
	duplicateSQL, duplicateErr := mysqlTx.New(db, &options, routes, []string{"qs.evaluation.lifecycle"})
	require.NoError(t, duplicateErr)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		require.NoError(t, duplicateSQL.Runtime().Close(ctx))
	})
	duplicate := NewContainerWithOptions(db, documents, nil, ContainerOptions{EventSubsystem: events, Transactions: &EventTransactionBindings{MySQL: duplicateSQL, Mongo: documentOwner, Routes: routes}, Silent: true})
	t.Cleanup(func() { require.NoError(t, duplicate.Cleanup()) })
	_, duplicateErr = duplicate.BuildRetryDispatchers(true)
	require.Error(t, duplicateErr, "same database must not authorize a second independent owner runtime")
	duplicateDocument, duplicateErr := mongoTx.New(documents, &options, []string{"qs.evaluation.lifecycle"})
	require.NoError(t, duplicateErr)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		require.NoError(t, duplicateDocument.Runtime().Close(ctx))
	})
	duplicateMongo := NewContainerWithOptions(db, documents, nil, ContainerOptions{EventSubsystem: events, Transactions: &EventTransactionBindings{MySQL: sqlOwner, Mongo: duplicateDocument, Routes: routes}, Silent: true})
	t.Cleanup(func() { require.NoError(t, duplicateMongo.Cleanup()) })
	_, duplicateErr = duplicateMongo.BuildRetryDispatchers(true)
	require.Error(t, duplicateErr, "same Mongo database must not authorize a second independent owner runtime")
	cases["duplicate_owner_runtime_refused"] = true
	_, err = pool.ExecContext(t.Context(), "UPDATE rm_tx_binding SET store_id=? WHERE owner=?", []byte("foreign"), []byte(sb.Owner))
	require.NoError(t, err)
	require.ErrorIs(t, factory.Validate(t.Context()), transaction.ErrBinding)
	_, err = pool.ExecContext(t.Context(), "UPDATE rm_tx_binding SET store_id=? WHERE owner=?", []byte(sb.StoreID), []byte(sb.Owner))
	require.NoError(t, err)
	_, err = documents.Collection("rm_tx_binding").UpdateOne(t.Context(), bson.M{"_id": mb.Owner}, bson.M{"$set": bson.M{"writer_epoch": "foreign"}})
	require.NoError(t, err)
	require.ErrorIs(t, factory.Validate(t.Context()), transaction.ErrBinding)
	_, err = documents.Collection("rm_tx_binding").UpdateOne(t.Context(), bson.M{"_id": mb.Owner}, bson.M{"$set": bson.M{"writer_epoch": mb.WriterEpoch}})
	require.NoError(t, err)
	require.NoError(t, factory.Validate(t.Context()))
	cases["fresh_authoritative_binding_reads"] = true
	require.NoError(t, hostmysql.NewUnitOfWork(db).WithinTransaction(t.Context(), func(ctx context.Context) error {
		require.ErrorIs(t, factory.Validate(ctx), transaction.ErrTransactionRequired)
		return nil
	}))
	session, err := client.StartSession()
	require.NoError(t, err)
	require.NoError(t, session.StartTransaction())
	err = mongo.WithSession(t.Context(), session, func(ctx mongo.SessionContext) error { return factory.Validate(ctx) })
	require.ErrorIs(t, err, transaction.ErrTransactionRequired)
	require.NoError(t, session.AbortTransaction(t.Context()))
	session.EndSession(t.Context())
	cases["original_transaction_context_refused"] = true
	for _, automatic := range []bool{false, true} {
		current, e := container.BuildRetryDispatchers(automatic)
		require.NoError(t, e)
		require.NoError(t, current.Validate(t.Context()))
		out, e := current.MySQL.Once(t.Context(), time.Now().UTC(), 50)
		require.NoError(t, e)
		require.Empty(t, out)
		out, e = current.Mongo.Once(t.Context(), time.Now().UTC(), 50)
		require.NoError(t, e)
		require.Empty(t, out)
	}
	var requests int
	require.NoError(t, pool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM rm_tx_request").Scan(&requests))
	require.Zero(t, requests)
	docRequests, err := documents.Collection("rm_tx_request").CountDocuments(t.Context(), bson.D{})
	require.NoError(t, err)
	require.Zero(t, docRequests)
	require.False(t, sqlOwner.Runtime().Stats().Ready)
	require.False(t, documentOwner.Runtime().Stats().Ready)
	cases["empty_factory_dispatch_never_starts_sender_or_business"] = true
	lock, err := pool.Conn(t.Context())
	require.NoError(t, err)
	_, err = lock.ExecContext(t.Context(), "LOCK TABLES rm_tx_binding WRITE")
	require.NoError(t, err)
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, e := lock.ExecContext(ctx, "UNLOCK TABLES")
			require.NoError(t, e)
			require.NoError(t, lock.Close())
		})
	}
	t.Cleanup(release)
	started := time.Now()
	err = factory.Validate(t.Context())
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(started), 4*time.Second)
	release()
	require.NoError(t, factory.Validate(t.Context()))
	cases["real_mysql_metadata_lock_respects_fresh_budget"] = true
	// Cleanup is verified before the receipt; host DB/client ownership stays here.
	require.NoError(t, sqlOwner.Runtime().Close(t.Context()))
	require.NoError(t, documentOwner.Runtime().Close(t.Context()))
	require.NoError(t, container.Cleanup())
	require.NoError(t, pool.PingContext(t.Context()))
	require.NoError(t, client.Ping(t.Context(), readpref.Primary()))
	cases["factory_cleanup_preserves_host_database_ownership"] = true
	closeSQL()
	dropSQL()
	dropMongo()
	disconnect()
	sourceSHA, err := exec.Command("git", "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	receipt, err := json.Marshal(map[string]any{"source_sha": strings.TrimSpace(string(sourceSHA)), "passed": len(cases), "failed": 0, "skipped": 0, "cases": cases, "scope": "actual default dispatch factory; original MySQL and MongoDB preflight and empty scans", "broker_or_business_dispatch_not_claimed": true, "complete_host_startup_not_claimed": true, "system_login_not_claimed": true, "owned_databases_dropped": true, "original_client_closed_by_fixture": true})
	require.NoError(t, err)
	fmt.Println("M3_QS_DISPATCH_FACTORY_RECEIPT " + string(receipt))
}
