//go:build integration

package migration

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/event"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func offlineCurrentNativePair(t *testing.T) (nativeSchemaPair, *migrationResources) {
	t.Helper()
	r, err := compiledMigrationResources()
	if err != nil {
		t.Fatal(err)
	}
	n := openNativeSchemaPairWithPrefix(t, "qs_offline_test")
	n.approve(t, r)
	n.runBoth(t, n.preflight(t, r))
	return n, r
}

func offlineNativeResources(t *testing.T, sqlBody, mongoBody string) *migrationResources {
	t.Helper()
	current, err := compiledMigrationResources()
	if err != nil {
		t.Fatal(err)
	}
	source := make(fstest.MapFS)
	if err := fs.WalkDir(migrations, "migrations", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		raw, err := migrations.ReadFile(path)
		if err != nil {
			return err
		}
		source[path] = &fstest.MapFile{Data: raw}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	source[fmt.Sprintf("migrations/mysql/%06d_offline_fixture.up.sql", current.sql.head+1)] = &fstest.MapFile{Data: []byte(sqlBody)}
	source[fmt.Sprintf("migrations/mongodb/%06d_offline_fixture.up.json", current.mongo.head+1)] = &fstest.MapFile{Data: []byte(mongoBody)}
	pairs := append([]schemaVersionPair{}, declaredSchemaPairs...)
	pairs = append(pairs, schemaVersionPair{current.sql.head + 1, current.mongo.head}, schemaVersionPair{current.sql.head + 1, current.mongo.head + 1})
	r, err := readMigrationResources(source, pairs)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func offlineNativeApproval(t *testing.T, n nativeSchemaPair, r *migrationResources) OfflineConfig {
	t.Helper()
	pair := n.cfg
	pair.BootstrapAuthorizationFile, pair.BootstrapAuthorizationSHA256 = "", ""
	_, snapshot, err := inspectOffline(t.Context(), n.db, n.client, pair, r)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := offlineApprovalFixture(t, snapshot)
	cfg.Pair = pair
	return cfg
}

func TestOfflineNativeUpgradeAndFreshNoop(t *testing.T) {
	t.Setenv("QS_M3_REQUIRE_DATABASE", "1")
	n, current := offlineCurrentNativePair(t)
	r := offlineNativeResources(t, "CREATE TABLE offline_sql_probe(id BIGINT PRIMARY KEY);", `[{"create":"offline_mongo_probe"}]`)
	cfg := offlineNativeApproval(t, n, r)
	before := n.db.Stats().InUse
	result, err := runOffline(t.Context(), n.db, n.client, cfg, r)
	if err != nil || !result.Verified || result.MySQLChanged == nil || !*result.MySQLChanged || result.MongoChanged == nil || !*result.MongoChanged || !result.AuthorizationConsumed {
		t.Fatal(result, err)
	}
	if sql, dirty, mongo, mdirty := n.heads(t); sql != r.sql.head || mongo != r.mongo.head || dirty || mdirty {
		t.Fatal(sql, dirty, mongo, mdirty)
	}
	if n.db.Stats().InUse != before || n.db.PingContext(t.Context()) != nil || n.client.Ping(t.Context(), nil) != nil {
		t.Fatal("borrowed resources were leaked or closed")
	}
	if _, err := runOffline(t.Context(), n.db, n.client, cfg, r); err == nil {
		t.Fatal("consumed approval reused")
	}
	// A new process/approval binds the new genuine clean pair; no old initial
	// pair is silently adopted and no forced metadata is used in this fixture.
	fresh := offlineNativeApproval(t, n, r)
	result, err = runOffline(t.Context(), n.db, n.client, fresh, r)
	if err != nil || !result.Verified || result.MySQLChanged == nil || *result.MySQLChanged || result.MongoChanged == nil || *result.MongoChanged {
		t.Fatal(result, err)
	}
	if _, _, err := inspectOffline(t.Context(), n.db, n.client, fresh.Pair, current); err == nil {
		t.Fatal("old embedded head accepted future installed versions")
	}
}

func TestOfflineNativeSQLPartialFailureRemainsDirty(t *testing.T) {
	t.Setenv("QS_M3_REQUIRE_DATABASE", "1")
	n, current := offlineCurrentNativePair(t)
	r := offlineNativeResources(t, "CREATE TABLE offline_sql_first(id BIGINT PRIMARY KEY); SELECT * FROM offline_missing_table;", `[{"create":"offline_mongo_probe"}]`)
	cfg := offlineNativeApproval(t, n, r)
	result, err := runOffline(t.Context(), n.db, n.client, cfg, r)
	if err == nil || result.Verified || result.Stage != "mysql" || !result.AuthorizationConsumed {
		t.Fatal(result, err)
	}
	if sql, dirty, mongo, mdirty := n.heads(t); sql != r.sql.head || mongo != current.mongo.head || !dirty || mdirty {
		t.Fatal(sql, dirty, mongo, mdirty)
	}
	var rows int
	if err := n.db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='offline_sql_first'").Scan(&rows); err != nil || rows != 1 {
		t.Fatal("first actual DDL not observed", rows, err)
	}
	assertOfflineDirtyRestartRefused(t, n, cfg, r)
}

func TestOfflineNativeMongoPartialFailureRemainsDirty(t *testing.T) {
	t.Setenv("QS_M3_REQUIRE_DATABASE", "1")
	n, _ := offlineCurrentNativePair(t)
	r := offlineNativeResources(t, "CREATE TABLE offline_sql_probe(id BIGINT PRIMARY KEY);", `[{"create":"offline_mongo_first"},{"offline_unknown_command":1}]`)
	cfg := offlineNativeApproval(t, n, r)
	result, err := runOffline(t.Context(), n.db, n.client, cfg, r)
	if err == nil || result.Verified || result.Stage != "mongodb" || !result.AuthorizationConsumed {
		t.Fatal(result, err)
	}
	if sql, dirty, mongo, mdirty := n.heads(t); sql != r.sql.head || mongo != r.mongo.head || dirty || !mdirty {
		t.Fatal(sql, dirty, mongo, mdirty)
	}
	names, err := n.mongoDB.ListCollectionNames(t.Context(), bson.D{{Key: "name", Value: "offline_mongo_first"}})
	if err != nil || len(names) != 1 {
		t.Fatal("first actual Mongo command not observed", names, err)
	}
	assertOfflineDirtyRestartRefused(t, n, cfg, r)
}

func TestOfflineNativeMongoCancellationRetainsPartialState(t *testing.T) {
	t.Setenv("QS_M3_REQUIRE_DATABASE", "1")
	n, _ := offlineCurrentNativePair(t)
	r := offlineNativeResources(t, "CREATE TABLE offline_sql_probe(id BIGINT PRIMARY KEY);", `[{"create":"offline_mongo_first"},{"create":"offline_mongo_second"}]`)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	client, err := mongo.Connect(t.Context(), options.Client().ApplyURI(os.Getenv("QS_SERVER_TEST_MONGO_URI")).SetMonitor(&event.CommandMonitor{
		Started: func(_ context.Context, command *event.CommandStartedEvent) {
			if command.DatabaseName == n.mongoDB.Name() && command.CommandName == "create" && command.Command.Lookup("create").StringValue() == "offline_mongo_second" {
				// Interrupt this client's second actual body command. No server-wide
				// failpoint or shared database setting is changed.
				cancel()
			}
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	n.client = client
	cfg := offlineNativeApproval(t, n, r)
	result, err := runOffline(ctx, n.db, n.client, cfg, r)
	if err == nil || result.Stage != "mongodb" || result.Verified || !result.MigrationOutcomeUnknown || result.MongoChanged != nil {
		t.Fatal(result, err)
	}
	if sql, dirty, head, mdirty := n.heads(t); sql != r.sql.head || dirty || head != r.mongo.head || !mdirty {
		t.Fatal(sql, dirty, head, mdirty)
	}
	names, err := n.mongoDB.ListCollectionNames(t.Context(), bson.D{{Key: "name", Value: "offline_mongo_first"}})
	if err != nil || len(names) != 1 {
		t.Fatal("completed first command disappeared", names, err)
	}
	assertOfflineDirtyRestartRefused(t, n, cfg, r)
}

func assertOfflineDirtyRestartRefused(t *testing.T, n nativeSchemaPair, cfg OfflineConfig, r *migrationResources) {
	t.Helper()
	// Give the fresh attempt a different path; dirty detection must stop before
	// the file is consumed, regardless of the existence of an approval file.
	fresh := cfg
	fresh.AuthorizationFile = filepath.Join(filepath.Dir(cfg.AuthorizationFile), "unused.json")
	raw, err := os.ReadFile(cfg.AuthorizationFile)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fresh.AuthorizationFile, raw, 0600); err != nil {
		t.Fatal(err)
	}
	result, err := runOffline(t.Context(), n.db, n.client, fresh, r)
	if err == nil || result.Stage != "preflight" || result.AuthorizationConsumed {
		t.Fatal("fresh dirty attempt adopted", result, err)
	}
	if _, err := os.Stat(fresh.AuthorizationFile + ".consumed"); !os.IsNotExist(err) {
		t.Fatal("dirty attempt touched approval", err)
	}
}

func TestOfflineNativeSQLDeadlineRetainsUnknownPartialState(t *testing.T) {
	t.Setenv("QS_M3_REQUIRE_DATABASE", "1")
	n, current := offlineCurrentNativePair(t)
	r := offlineNativeResources(t, "CREATE TABLE offline_sql_first(id BIGINT PRIMARY KEY); DO SLEEP(5);", `[{"create":"offline_mongo_probe"}]`)
	cfg := offlineNativeApproval(t, n, r)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	started := time.Now()
	result, err := runOffline(ctx, n.db, n.client, cfg, r)
	if err == nil || result.Stage != "mysql" || result.Verified || !result.AuthorizationConsumed || time.Since(started) > 4*time.Second {
		t.Fatal(result, err, time.Since(started))
	}
	if sql, dirty, mongo, mdirty := n.heads(t); sql != r.sql.head || mongo != current.mongo.head || !dirty || mdirty {
		t.Fatal(sql, dirty, mongo, mdirty)
	}
	assertOfflineDirtyRestartRefused(t, n, cfg, r)
}

func TestOfflineNativeMissingRecoveryAndPristineRefusedBeforeDDL(t *testing.T) {
	t.Setenv("QS_M3_REQUIRE_DATABASE", "1")
	n, r := offlineCurrentNativePair(t)
	cfg := offlineNativeApproval(t, n, r)
	_, s, err := inspectOffline(t.Context(), n.db, n.client, cfg.Pair, r)
	if err != nil {
		t.Fatal(err)
	}
	a, err := readOfflineAuthorization(cfg, s)
	if err != nil {
		t.Fatal(err)
	}
	a.BackupPairLocation = ""
	writeOfflineApproval(t, &cfg, a)
	result, err := runOffline(t.Context(), n.db, n.client, cfg, r)
	if err == nil || result.Stage != "approval" || result.AuthorizationConsumed {
		t.Fatal(result, err)
	}
	if sql, dirty, mongo, mdirty := n.heads(t); sql != r.sql.head || mongo != r.mongo.head || dirty || mdirty {
		t.Fatal("rejected approval changed schema metadata")
	}
	pristine := openNativeSchemaPairWithPrefix(t, "qs_offline_test")
	cfg.Pair = pristine.cfg
	result, err = runOffline(t.Context(), pristine.db, pristine.client, cfg, r)
	if err == nil || result.Stage != "preflight" || result.AuthorizationConsumed {
		t.Fatal("offline bootstrap accepted", result, err)
	}
}

func TestOfflineNativeMissingBoundResourcesRefusedBeforeDDL(t *testing.T) {
	t.Setenv("QS_M3_REQUIRE_DATABASE", "1")
	for _, backend := range []Backend{BackendMySQL, BackendMongo} {
		t.Run(string(backend), func(t *testing.T) {
			n, current := offlineCurrentNativePair(t)
			r := offlineNativeResources(t, "CREATE TABLE offline_sql_probe(id BIGINT PRIMARY KEY);", `[{"create":"offline_mongo_probe"}]`)
			cfg := offlineNativeApproval(t, n, r)
			// Remove one body after the reviewed inventory and approval were bound.
			// The real migrators must reject the changed source before dirty/DDL.
			source := r.source.(fstest.MapFS)
			path := fmt.Sprintf("migrations/mysql/%06d_offline_fixture.up.sql", r.sql.head)
			if backend == BackendMongo {
				path = fmt.Sprintf("migrations/mongodb/%06d_offline_fixture.up.json", r.mongo.head)
			}
			delete(source, path)
			before := n.db.Stats().InUse
			result, err := runOffline(t.Context(), n.db, n.client, cfg, r)
			if err == nil || result.Stage != "mysql" || !result.AuthorizationConsumed || !result.MigrationOutcomeUnknown || result.Verified || result.MySQLChanged != nil || result.MongoChanged != nil {
				t.Fatal("missing approved body accepted", result, err)
			}
			if sql, dirty, mongo, mdirty := n.heads(t); sql != current.sql.head || mongo != current.mongo.head || dirty || mdirty {
				t.Fatal("missing resource changed version metadata", sql, dirty, mongo, mdirty)
			}
			assertMySQLTable(t, n.db, n.cfg.MySQLDatabase, "offline_sql_probe", false)
			names, err := n.mongoDB.ListCollectionNames(t.Context(), bson.D{{Key: "name", Value: "offline_mongo_probe"}})
			if err != nil || len(names) != 0 {
				t.Fatal("missing resource executed Mongo body", names, err)
			}
			if n.db.Stats().InUse != before || n.db.PingContext(t.Context()) != nil || n.client.Ping(t.Context(), nil) != nil {
				t.Fatal("missing resource leaked run connection or closed borrowed pools")
			}
		})
	}
}
