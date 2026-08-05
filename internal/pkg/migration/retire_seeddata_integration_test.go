//go:build integration

package migration

import (
	"database/sql"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

var retiredMySQLControlTables = []string{
	"seed_backfill_rollback_phase_attempt",
	"seed_backfill_rollback_resource",
	"seed_backfill_rollback_operation",
	"seed_backfill_stage_attempt",
	"seed_backfill_stage",
}

var retiredMongoCollections = []string{
	"published_assessment_models",
	"interpret_reports",
	"evaluation_rule_sets",
	"scales",
}

func TestRetireSeedBackfillControlFrom63AndDown(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		t.Fatal("MYSQL_DSN is required for migration integration tests; SKIP is not allowed")
	}
	db, databaseName := openStatisticsMigrationDatabase(t, dsn)
	config := ensureConfigDefaults(&Config{Enabled: true, Database: databaseName})
	instance, err := NewMySQLDriver(db).CreateInstance(migrations, config)
	if err != nil {
		t.Fatal(err)
	}
	if err := instance.Migrate(63); err != nil {
		t.Fatalf("migrate MySQL 0 -> 63: %v", err)
	}
	wantSchema := mysqlCreateTableSnapshot(t, db, retiredMySQLControlTables)
	seedRetiredMySQLControlRows(t, db)
	before := mysqlTableNames(t, db, databaseName)

	if err := instance.Migrate(64); err != nil {
		t.Fatalf("migrate MySQL 63 -> 64: %v", err)
	}
	after := mysqlTableNames(t, db, databaseName)
	assertExactRemovedNames(t, before, after, retiredMySQLControlTables)
	for _, table := range retiredMySQLControlTables {
		assertMySQLTable(t, db, databaseName, table, false)
	}
	assertMySQLTable(t, db, databaseName, "assessment_task", true)
	assertMySQLColumn(t, db, databaseName, "assessment_task", "business_created_at", true)

	if err := instance.Steps(-1); err != nil {
		t.Fatalf("MySQL 64 down: %v", err)
	}
	for _, table := range retiredMySQLControlTables {
		assertMySQLTable(t, db, databaseName, table, true)
		var count int64
		if err := db.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM `"+table+"`").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("down migration restored data in %s: count=%d", table, count)
		}
	}
	gotSchema := mysqlCreateTableSnapshot(t, db, retiredMySQLControlTables)
	if !reflect.DeepEqual(gotSchema, wantSchema) {
		t.Fatalf("MySQL 64 Down schema mismatch\ngot:  %#v\nwant: %#v", gotSchema, wantSchema)
	}
}

func TestRetireSeedBackfillControlColdStart(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		t.Fatal("MYSQL_DSN is required for migration integration tests; SKIP is not allowed")
	}
	db, databaseName := openStatisticsMigrationDatabase(t, dsn)
	migrator := NewMigrator(db, &Config{Enabled: true, Database: databaseName})
	version, changed, err := migrator.Run()
	wantVersion := latestEmbeddedMySQLMigrationVersion(t)
	if err != nil || !changed || version != wantVersion {
		t.Fatalf("migrate MySQL 0 -> latest: version=%d changed=%v err=%v want_version=%d", version, changed, err, wantVersion)
	}
	for _, table := range retiredMySQLControlTables {
		assertMySQLTable(t, db, databaseName, table, false)
	}
	assertMySQLColumn(t, db, databaseName, "assessment_task", "business_created_at", true)
}

func TestRetireLegacyMongoCollectionsFrom19AndDown(t *testing.T) {
	client, db := mongodbtest.ReplicaSetDatabase(t)
	config := ensureConfigDefaults(&Config{Enabled: true, Database: db.Name()})
	driver := NewMongoDriver(client)
	instance, err := driver.CreateInstance(migrations, config)
	if err != nil {
		t.Fatal(err)
	}
	cleanup, err := driver.PrepareRun(t.Context(), config, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := instance.Migrate(19); err != nil {
		t.Fatalf("migrate MongoDB 0 -> 19: %v", err)
	}
	if err := cleanup(t.Context()); err != nil {
		t.Fatalf("cleanup temporary migration index: %v", err)
	}
	wantIndexes := mongoIndexSnapshots(t, db, retiredMongoCollections)
	for _, name := range retiredMongoCollections {
		if _, err := db.Collection(name).InsertOne(t.Context(), bson.M{"proof": name}); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	before := mongoCollectionNames(t, db)

	if err := instance.Migrate(20); err != nil {
		t.Fatalf("migrate MongoDB 19 -> 20: %v", err)
	}
	after := mongoCollectionNames(t, db)
	assertExactRemovedNames(t, before, after, retiredMongoCollections)

	if err := instance.Steps(-1); err != nil {
		t.Fatalf("MongoDB 20 down: %v", err)
	}
	for _, name := range retiredMongoCollections {
		count, err := db.Collection(name).CountDocuments(t.Context(), bson.M{})
		if err != nil {
			t.Fatalf("count recreated %s: %v", name, err)
		}
		if count != 0 {
			t.Fatalf("down migration restored documents in %s: count=%d", name, count)
		}
	}
	gotIndexes := mongoIndexSnapshots(t, db, retiredMongoCollections)
	if !reflect.DeepEqual(gotIndexes, wantIndexes) {
		t.Fatalf("MongoDB 20 Down index mismatch\ngot:  %#v\nwant: %#v", gotIndexes, wantIndexes)
	}
}

func TestRetireLegacyMongoCollectionsColdStart(t *testing.T) {
	client, db := mongodbtest.ReplicaSetDatabase(t)
	migrator := NewMongoMigrator(client, &Config{Enabled: true, Database: db.Name()})
	wantVersion := latestEmbeddedMongoMigrationVersion(t)
	version, changed, err := migrator.Run()
	if err != nil || !changed || version != wantVersion {
		t.Fatalf("migrate MongoDB 0 -> %d: version=%d changed=%v err=%v", wantVersion, version, changed, err)
	}
	for _, name := range retiredMongoCollections {
		if containsString(mongoCollectionNames(t, db), name) {
			t.Fatalf("retired collection %s exists after cold start", name)
		}
	}
	for _, name := range []string{"answersheets", "questionnaires", "assessment_models", "assessment_norms", "interpret_report_artifacts"} {
		if !containsString(mongoCollectionNames(t, db), name) {
			t.Fatalf("canonical collection %s is missing after cold start", name)
		}
	}
}

func seedRetiredMySQLControlRows(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.ExecContext(t.Context(), `
INSERT INTO seed_backfill_stage
(id,org_id,batch_id,scenario_id,stage,payload_hash,status,business_at,resource_type,resource_id)
VALUES (1,7,'batch-proof','scenario-proof','stage-proof',REPEAT('a',64),'completed',UTC_TIMESTAMP(6),'proof','1');
INSERT INTO seed_backfill_stage_attempt
(id,org_id,batch_id,scenario_id,stage,attempt_no,context_hash,status,business_at,started_at,finished_at)
VALUES (1,7,'batch-proof','scenario-proof','stage-proof',1,REPEAT('b',64),'completed',UTC_TIMESTAMP(6),UTC_TIMESTAMP(6),UTC_TIMESTAMP(6));
INSERT INTO seed_backfill_rollback_operation
(org_id,batch_id,manifest_hash,scope_hash,backup_suffix,phase,status,started_at)
VALUES (7,'batch-proof',REPEAT('c',64),REPEAT('d',64),'proof','prepared','running',UTC_TIMESTAMP(6));
SET @operation_id = LAST_INSERT_ID();
INSERT INTO seed_backfill_rollback_resource
(operation_id,storage,resource_type,resource_id) VALUES (@operation_id,'mysql','proof','1');
INSERT INTO seed_backfill_rollback_phase_attempt
(operation_id,phase,attempt_no,status,started_at) VALUES (@operation_id,'prepared',1,'completed',UTC_TIMESTAMP(6));`)
	if err != nil {
		t.Fatal(err)
	}
}

func mysqlTableNames(t *testing.T, db *sql.DB, databaseName string) []string {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), `SELECT table_name FROM information_schema.tables WHERE table_schema=? ORDER BY table_name`, databaseName)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return names
}

func mysqlCreateTableSnapshot(t *testing.T, db *sql.DB, tables []string) map[string]string {
	t.Helper()
	out := make(map[string]string, len(tables))
	for _, table := range tables {
		var name, statement string
		if err := db.QueryRowContext(t.Context(), "SHOW CREATE TABLE `"+table+"`").Scan(&name, &statement); err != nil {
			t.Fatalf("SHOW CREATE TABLE %s: %v", table, err)
		}
		out[name] = statement
	}
	return out
}

func mongoCollectionNames(t *testing.T, db *mongo.Database) []string {
	t.Helper()
	names, err := db.ListCollectionNames(t.Context(), bson.M{})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	return names
}

type mongoIndexSpec struct {
	Key                     bson.D `bson:"key"`
	Name                    string `bson:"name"`
	Unique                  *bool  `bson:"unique,omitempty"`
	Sparse                  *bool  `bson:"sparse,omitempty"`
	PartialFilterExpression bson.D `bson:"partialFilterExpression,omitempty"`
	ExpireAfterSeconds      *int64 `bson:"expireAfterSeconds,omitempty"`
	Hidden                  *bool  `bson:"hidden,omitempty"`
	Collation               bson.D `bson:"collation,omitempty"`
}

func mongoIndexSnapshots(t *testing.T, db *mongo.Database, collections []string) map[string][]string {
	t.Helper()
	out := make(map[string][]string, len(collections))
	for _, name := range collections {
		cursor, err := db.Collection(name).Indexes().List(t.Context())
		if err != nil {
			t.Fatalf("list indexes for %s: %v", name, err)
		}
		var indexes []string
		for cursor.Next(t.Context()) {
			var spec mongoIndexSpec
			if err := cursor.Decode(&spec); err != nil {
				_ = cursor.Close(t.Context())
				t.Fatalf("decode index for %s: %v", name, err)
			}
			raw, err := bson.MarshalExtJSON(spec, true, false)
			if err != nil {
				_ = cursor.Close(t.Context())
				t.Fatalf("canonicalize index %s.%s: %v", name, spec.Name, err)
			}
			indexes = append(indexes, string(raw))
		}
		if err := cursor.Err(); err != nil {
			_ = cursor.Close(t.Context())
			t.Fatalf("iterate indexes for %s: %v", name, err)
		}
		if err := cursor.Close(t.Context()); err != nil {
			t.Fatalf("close index cursor for %s: %v", name, err)
		}
		sort.Strings(indexes)
		out[name] = indexes
	}
	return out
}

func assertExactRemovedNames(t *testing.T, before, after, want []string) {
	t.Helper()
	afterSet := make(map[string]struct{}, len(after))
	for _, name := range after {
		afterSet[name] = struct{}{}
	}
	var removed []string
	for _, name := range before {
		if _, ok := afterSet[name]; !ok {
			removed = append(removed, name)
		}
	}
	sort.Strings(removed)
	want = append([]string(nil), want...)
	sort.Strings(want)
	if len(removed) != len(want) {
		t.Fatalf("removed=%v want=%v", removed, want)
	}
	for i := range want {
		if removed[i] != want[i] {
			t.Fatalf("removed=%v want=%v", removed, want)
		}
	}
}

func latestEmbeddedMongoMigrationVersion(t *testing.T) uint {
	t.Helper()
	entries, err := migrations.ReadDir("migrations/mongodb")
	if err != nil {
		t.Fatal(err)
	}
	var latest uint64
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".up.json") {
			continue
		}
		prefix, _, ok := strings.Cut(name, "_")
		if !ok {
			continue
		}
		version, parseErr := strconv.ParseUint(prefix, 10, 64)
		if parseErr != nil {
			t.Fatalf("parse migration version from %s: %v", name, parseErr)
		}
		if version > latest {
			latest = version
		}
	}
	if latest == 0 {
		t.Fatal("no embedded MongoDB migrations found")
	}
	return uint(latest)
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
