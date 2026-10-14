//go:build integration

package migration

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	migratedb "github.com/golang-migrate/migrate/v4/database"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const historicalIndexUp = "migrations/mongodb/000040_retire_historical_event_indexes.up.json"

var historicalIndexTargets = []struct{ collection, name, slot string }{
	{"answersheets", "uk_answersheet_legacy_submission_event_id", "legacy_submission_evidence"},
	{"answersheets", "idx_answersheet_legacy_submission_audit", "legacy_submission_evidence"},
	{"report_generations", "uk_generation_historical_generated_event_id", "historical_generated_evidence"},
	{"report_generations", "idx_generation_historical_generated_audit", "historical_generated_evidence"},
}

func nativeHistoricalIndexPair(t *testing.T) nativeSchemaPair {
	t.Helper()
	t.Setenv("QS_M3_REQUIRE_DATABASE", "1")
	r := migrationResourcesThrough(t, 101, 39)
	n := openNativeSchemaPairWithPrefix(t, "qs_m4_mongo_test")
	n.approve(t, r)
	n.runBoth(t, n.preflight(t, r))
	for _, collection := range []string{"answersheets", "report_generations"} {
		slot := "legacy_submission_evidence"
		if collection == "report_generations" {
			slot = "historical_generated_evidence"
		}
		_, err := n.mongoDB.Collection(collection).InsertOne(t.Context(), bson.D{
			{Key: "_id", Value: "m4-frozen-" + collection}, {Key: "domain_id", Value: int64(880001)},
			{Key: "outcome_id", Value: int64(880001)}, {Key: "report_type", Value: "assessment"}, {Key: "template_version", Value: "3"},
			{Key: "generated_event_id", Value: "m4-current-generated"},
			{Key: "durable_acceptance", Value: bson.D{{Key: "event_id", Value: "m4-current-submission"}, {Key: "schema_version", Value: 1}}},
			{Key: "report_input", Value: bson.D{{Key: "schema_version", Value: 3}, {Key: "InterpretationAssets", Value: bson.D{{Key: "frozen", Value: "历史报告🙂"}}}}},
			{Key: slot, Value: bson.D{{Key: "version", Value: 1}, {Key: "entries", Value: bson.A{bson.D{{Key: "event_id", Value: "m4-synthetic-history"}}}}}},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return n
}

func mongoIndexInventory(t *testing.T, db *mongo.Database) map[string]bson.D {
	t.Helper()
	names, err := db.ListCollectionNames(t.Context(), bson.D{})
	if err != nil {
		t.Fatal(err)
	}
	result := make(map[string]bson.D)
	for _, name := range names {
		cursor, err := db.Collection(name).Indexes().List(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		var indexes []bson.D
		if err := cursor.All(t.Context(), &indexes); err != nil {
			t.Fatal(err)
		}
		for _, index := range indexes {
			result[name+"/"+index.Map()["name"].(string)] = index
		}
	}
	return result
}

func mongoBusinessNamespaceInventory(t *testing.T, db *mongo.Database) map[string]bson.Raw {
	t.Helper()
	cursor, err := db.ListCollections(t.Context(), bson.D{})
	if err != nil {
		t.Fatal(err)
	}
	var rows []bson.Raw
	if err := cursor.All(t.Context(), &rows); err != nil {
		t.Fatal(err)
	}
	result := make(map[string]bson.Raw)
	for _, row := range rows {
		name := row.Lookup("name").StringValue()
		// The migrate provider recreates only its schema_migrations collection
		// when recording each dirty/clean version. Verify that metadata through
		// heads/VerifyComplete, while every business namespace stays identical.
		if name != "schema_migrations" {
			result[name] = row
		}
	}
	return result
}

func mongoFrozenDocuments(t *testing.T, db *mongo.Database) map[string][]bson.Raw {
	t.Helper()
	result := make(map[string][]bson.Raw)
	for _, name := range []string{"answersheets", "report_generations"} {
		cursor, err := db.Collection(name).Find(t.Context(), bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
		if err != nil {
			t.Fatal(err)
		}
		var rows []bson.Raw
		if err := cursor.All(t.Context(), &rows); err != nil {
			t.Fatal(err)
		}
		result[name] = rows
	}
	return result
}

func runHistoricalIndexCommands(t *testing.T, db *mongo.Database, raw []byte) error {
	t.Helper()
	var commands struct {
		Commands []bson.D `bson:"commands"`
	}
	if err := bson.UnmarshalExtJSON(append([]byte(`{"commands":`), append(raw, []byte(`}`)...)...), false, &commands); err != nil {
		t.Fatal(err)
	}
	for _, command := range commands.Commands {
		if err := db.RunCommand(t.Context(), command).Err(); err != nil {
			return err
		}
	}
	return nil
}

func TestHistoricalIndexNativeMongoOnlyUpgradeAndRestart(t *testing.T) {
	n := nativeHistoricalIndexPair(t)
	sqlBefore := historicalQuery(t, n.db, "SELECT version,dirty FROM schema_migrations")
	before, namespaces, documents := mongoIndexInventory(t, n.mongoDB), mongoBusinessNamespaceInventory(t, n.mongoDB), mongoFrozenDocuments(t, n.mongoDB)
	for _, target := range historicalIndexTargets {
		if before[target.collection+"/"+target.name] == nil {
			t.Fatal("required historical index missing before upgrade", target.name)
		}
		delete(before, target.collection+"/"+target.name)
	}
	r := testCurrentResources(t)
	p := n.preflight(t, r)
	if version, changed, err := NewMigrator(n.db, p.MySQLConfig(false)).Run(); err != nil || changed || version != 101 {
		t.Fatal("Mongo-only release changed SQL", version, changed, err)
	}
	if version, changed, err := NewMongoMigrator(n.client, p.MongoConfig(false)).Run(); err != nil || !changed || version != 40 {
		t.Fatal("Mongo40 upgrade failed", version, changed, err)
	}
	if after := mongoIndexInventory(t, n.mongoDB); !reflect.DeepEqual(before, after) {
		t.Errorf("Mongo40 changed unrelated index specs: before=%v after=%v", before, after)
	}
	if after := mongoBusinessNamespaceInventory(t, n.mongoDB); !reflect.DeepEqual(namespaces, after) {
		t.Error("Mongo40 changed business namespace UUIDs/options")
	}
	if after := mongoFrozenDocuments(t, n.mongoDB); !reflect.DeepEqual(documents, after) {
		t.Error("Mongo40 changed frozen/current business documents")
	}
	if !reflect.DeepEqual(sqlBefore, historicalQuery(t, n.db, "SELECT version,dirty FROM schema_migrations")) {
		t.Error("Mongo-only release changed SQL metadata")
	}
	restart := n.preflight(t, r)
	for _, migrator := range []*Migrator{NewMigrator(n.db, restart.MySQLConfig(false)), NewMongoMigrator(n.client, restart.MongoConfig(false))} {
		if _, changed, err := migrator.Run(); err != nil || changed {
			t.Fatal("clean101/40 restart must run no migration", changed, err)
		}
	}
	if err := restart.VerifyComplete(t.Context()); err != nil {
		t.Fatal(err)
	}
}

// Restore only live-captured index specs and synthetic documents in an isolated
// namespace. This is not a complete production database-pair restore.
func TestHistoricalIndexNativeExactObjectRestoreAndConstraints(t *testing.T) {
	n := nativeHistoricalIndexPair(t)
	before, documents := mongoIndexInventory(t, n.mongoDB), mongoFrozenDocuments(t, n.mongoDB)
	up, err := migrations.ReadFile(historicalIndexUp)
	if err != nil || runHistoricalIndexCommands(t, n.mongoDB, up) != nil {
		t.Fatal("scoped object rehearsal drop failed", err)
	}
	for _, target := range historicalIndexTargets {
		backup := before[target.collection+"/"+target.name]
		var spec bson.D
		for _, field := range backup {
			if field.Key != "ns" {
				spec = append(spec, field)
			}
		}
		if err := n.mongoDB.RunCommand(t.Context(), bson.D{{Key: "createIndexes", Value: target.collection}, {Key: "indexes", Value: bson.A{spec}}}).Err(); err != nil {
			t.Fatal("restore live captured key order/unique/partial/sparse/collation spec", err)
		}
	}
	if !reflect.DeepEqual(before, mongoIndexInventory(t, n.mongoDB)) || !reflect.DeepEqual(documents, mongoFrozenDocuments(t, n.mongoDB)) {
		t.Fatal("restored index specs or frozen documents differ from the live backup")
	}
	for _, target := range []struct{ collection, slot, index string }{{"answersheets", "legacy_submission_evidence", "uk_answersheet_legacy_submission_event_id"}, {"report_generations", "historical_generated_evidence", "uk_generation_historical_generated_event_id"}} {
		doc := bson.M{"domain_id": int64(880002), "outcome_id": int64(880002), "report_type": "assessment", "template_version": "3", target.slot: bson.M{"entries": bson.A{bson.M{"event_id": "m4-synthetic-history"}}}}
		if _, err := n.mongoDB.Collection(target.collection).InsertOne(t.Context(), doc); !mongo.IsDuplicateKeyError(err) || !strings.Contains(err.Error(), target.index) {
			t.Fatal("restored exact historical unique constraint did not reject collision", target.index, err)
		}
		for _, id := range []int64{880003, 880004} {
			// Missing and non-string keys are outside the original partial filter.
			doc = bson.M{"domain_id": id, "outcome_id": id, "report_type": "assessment", "template_version": "3"}
			if id == 880004 {
				doc[target.slot] = bson.M{"entries": bson.A{bson.M{"event_id": int32(7)}}}
			}
			if _, err := n.mongoDB.Collection(target.collection).InsertOne(t.Context(), doc); err != nil {
				t.Fatal("restored partial filter changed sparse/missing/non-string behavior", err)
			}
		}
	}
	if sql, dirty, mongoHead, mongoDirty := n.heads(t); sql != 101 || mongoHead != 39 || dirty || mongoDirty {
		t.Fatal("index-only rehearsal fabricated migration success", sql, dirty, mongoHead, mongoDirty)
	}
}

// This proves only an isolated, fixed-object backup replay. It does not start
// an application image or provide production freeze, permissions or RTO proof.
func TestHistoricalIndexNativeDirtyObjectBackupRestore(t *testing.T) {
	for _, name := range []string{"restore", "interrupt_before_metadata_then_restore", "transaction_write_failure_then_restore"} {
		t.Run(name, func(t *testing.T) {
			n := nativeHistoricalIndexPair(t)
			if _, err := n.mongoDB.Collection("rm_outbox").InsertOne(t.Context(), bson.D{
				{Key: "_id", Value: "m4-preserved-current-responsibility"},
				{Key: "message_id", Value: "m4-preserved-current-responsibility"},
				{Key: "payload", Value: bson.D{{Key: "frozen", Value: "current responsibility"}}},
			}); err != nil {
				t.Fatal(err)
			}
			indexes, namespaces := mongoIndexInventory(t, n.mongoDB), mongoBusinessNamespaceInventory(t, n.mongoDB)
			businessDocuments := func() map[string][]bson.Raw {
				result := make(map[string][]bson.Raw)
				for collection := range namespaces {
					cursor, err := n.mongoDB.Collection(collection).Find(t.Context(), bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
					if err != nil {
						t.Fatal(err)
					}
					var rows []bson.Raw
					if err := cursor.All(t.Context(), &rows); err != nil {
						t.Fatal(err)
					}
					result[collection] = rows
				}
				return result
			}
			documents := businessDocuments()
			sqlBefore := historicalQuery(t, n.db, "SELECT version,dirty FROM schema_migrations")
			sqlIdentity, mongoIdentity, err := pairIdentity(t.Context(), n.db, n.client, n.cfg.MySQLDatabase, n.cfg.MongoDatabase)
			if err != nil {
				t.Fatal(err)
			}
			// Capture real BSON, including its original _id, integer encoding and
			// field order. No version/dirty document is constructed for replay.
			cursor, err := n.mongoDB.Collection("schema_migrations").Find(t.Context(), bson.D{}, options.Find().SetLimit(2))
			if err != nil {
				t.Fatal(err)
			}
			var headRows []bson.Raw
			if err := cursor.All(t.Context(), &headRows); err != nil || len(headRows) != 1 {
				t.Fatal("complete clean version BSON backup requires exactly one row", err)
			}
			headBackup := append(bson.Raw(nil), headRows[0]...)
			cursor, err = n.mongoDB.ListCollections(t.Context(), bson.D{{Key: "name", Value: "schema_migrations"}})
			if err != nil {
				t.Fatal(err)
			}
			var catalogs []bson.Raw
			if err := cursor.All(t.Context(), &catalogs); err != nil || len(catalogs) != 1 {
				t.Fatal("capture complete version namespace failed", err)
			}
			var headIndexes []bson.D
			for index, spec := range indexes {
				if strings.HasPrefix(index, "schema_migrations/") {
					headIndexes = append(headIndexes, spec)
				}
			}
			if len(headIndexes) == 0 || headBackup.Lookup("_id").Type == 0 {
				t.Fatal("version object backup incomplete")
			}

			// Fixed FS seam: real MongoMigrator writes40dirty and successfully
			// drops the first two indexes before the third command fails in Mongo.
			source := embeddedMigrationSource(t)
			source[historicalIndexUp].Data = bytes.Replace(source[historicalIndexUp].Data, []byte(`"index":"uk_generation_historical_generated_event_id"`), []byte(`"index":"m4_missing_index"`), 1)
			r, err := readMigrationResources(source, declaredSchemaPairs)
			if err != nil {
				t.Fatal(err)
			}
			p := n.preflight(t, r)
			if _, _, err := NewMigrator(n.db, p.MySQLConfig(false)).Run(); err != nil {
				t.Fatal(err)
			}
			if _, _, err := NewMongoMigrator(n.client, p.MongoConfig(false)).Run(); err == nil {
				t.Fatal("real partial index migration unexpectedly succeeded")
			}
			assertDirty := func() {
				if sql, dirty, head, mongoDirty := n.heads(t); sql != 101 || dirty || head != 40 || !mongoDirty {
					t.Fatal("fixed-object replay must leave101clean/40dirty until its final step")
				}
				if _, err := preflightCompatibilityPair(t.Context(), n.db, n.client, n.cfg, r); err == nil {
					t.Fatal("fresh process adopted the interrupted dirty migration")
				}
			}
			assertDirty()
			afterFailure := mongoIndexInventory(t, n.mongoDB)
			for _, target := range historicalIndexTargets[:2] {
				if afterFailure[target.collection+"/"+target.name] != nil {
					t.Fatal("real migration did not complete the required partial drops")
				}
			}
			var dirtyHead bson.Raw
			if err := n.mongoDB.Collection("schema_migrations").FindOne(t.Context(), bson.D{}).Decode(&dirtyHead); err != nil {
				t.Fatal(err)
			}
			dirtyHead = append(bson.Raw(nil), dirtyHead...)
			cursor, err = n.mongoDB.ListCollections(t.Context(), bson.D{{Key: "name", Value: "schema_migrations"}})
			if err != nil {
				t.Fatal(err)
			}
			var dirtyCatalogs []bson.Raw
			if err := cursor.All(t.Context(), &dirtyCatalogs); err != nil || len(dirtyCatalogs) != 1 {
				t.Fatal("capture dirty version namespace failed", err)
			}
			if bytes.Equal(catalogs[0].Lookup("info").Document().Lookup("uuid").Value, dirtyCatalogs[0].Lookup("info").Document().Lookup("uuid").Value) {
				t.Fatal("real provider40 version write did not expose its new metadata UUID")
			}
			createExactIndex := func(collection string, backup bson.D) error {
				var spec bson.D
				for _, field := range backup {
					if field.Key != "ns" {
						spec = append(spec, field)
					}
				}
				return n.mongoDB.RunCommand(t.Context(), bson.D{{Key: "createIndexes", Value: collection}, {Key: "indexes", Value: bson.A{spec}}}).Err()
			}
			restore := func(beforeMetadata func() error, failWrite bool) error {
				assertDirty()
				cursor, err := n.mongoDB.ListCollections(t.Context(), bson.D{{Key: "name", Value: "schema_migrations"}})
				if err != nil {
					return err
				}
				var currentCatalogs []bson.Raw
				if err := cursor.All(t.Context(), &currentCatalogs); err != nil {
					return err
				}
				if len(currentCatalogs) != 1 || !bytes.Equal(catalogs[0].Lookup("options").Value, currentCatalogs[0].Lookup("options").Value) {
					return errors.New("version namespace options do not match the backup")
				}
				for _, spec := range headIndexes {
					if err := createExactIndex("schema_migrations", spec); err != nil {
						return err
					}
				}
				for _, target := range historicalIndexTargets {
					if err := createExactIndex(target.collection, indexes[target.collection+"/"+target.name]); err != nil {
						return err
					}
				}
				if !reflect.DeepEqual(indexes, mongoIndexInventory(t, n.mongoDB)) || !reflect.DeepEqual(namespaces, mongoBusinessNamespaceInventory(t, n.mongoDB)) || !reflect.DeepEqual(documents, businessDocuments()) || !reflect.DeepEqual(sqlBefore, historicalQuery(t, n.db, "SELECT version,dirty FROM schema_migrations")) {
					return errors.New("business objects differ before metadata replay")
				}
				a, b, err := pairIdentity(t.Context(), n.db, n.client, n.cfg.MySQLDatabase, n.cfg.MongoDatabase)
				if err != nil || a != sqlIdentity || b != mongoIdentity {
					return errors.New("source identity differs before metadata replay")
				}
				if beforeMetadata != nil {
					if err := beforeMetadata(); err != nil {
						return err
					}
				}
				// A single explicit replica-set transaction publishes the captured
				// BSON only after the complete object proof. No automatic retry and
				// no namespace drop/recreation are used for the recovery itself.
				session, err := n.client.StartSession()
				if err != nil {
					return err
				}
				defer session.EndSession(t.Context())
				if err := session.StartTransaction(); err != nil {
					return err
				}
				txn := mongo.NewSessionContext(t.Context(), session)
				if _, err = n.mongoDB.Collection("schema_migrations").DeleteMany(txn, bson.D{}); err == nil {
					_, err = n.mongoDB.Collection("schema_migrations").InsertOne(txn, headBackup)
				}
				if err == nil && failWrite {
					// A real duplicate _id error aborts these actual database writes;
					// it is not a synthetic callback or a fabricated dirty document.
					_, err = n.mongoDB.Collection("schema_migrations").InsertOne(txn, headBackup)
				}
				if err != nil {
					return errors.Join(err, session.AbortTransaction(t.Context()))
				}
				if err := session.CommitTransaction(t.Context()); err != nil {
					return errors.Join(err, session.AbortTransaction(t.Context()))
				}
				return nil
			}
			if name == "interrupt_before_metadata_then_restore" {
				if err := restore(func() error { return context.Canceled }, false); !errors.Is(err, context.Canceled) {
					t.Fatal("fixed interruption was not respected before metadata replay", err)
				}
				var stillDirty bson.Raw
				if err := n.mongoDB.Collection("schema_migrations").FindOne(t.Context(), bson.D{}).Decode(&stillDirty); err != nil || !bytes.Equal(dirtyHead, stillDirty) {
					t.Fatal("interrupted restoration changed the original40dirty BSON", err)
				}
				assertDirty()
			}
			if name == "transaction_write_failure_then_restore" {
				if err := restore(nil, true); !mongo.IsDuplicateKeyError(err) {
					t.Fatal("real transactional duplicate_id failure was not observed", err)
				}
				var stillDirty bson.Raw
				if err := n.mongoDB.Collection("schema_migrations").FindOne(t.Context(), bson.D{}).Decode(&stillDirty); err != nil || !bytes.Equal(dirtyHead, stillDirty) {
					t.Fatal("aborted transaction changed the original40dirty BSON", err)
				}
				assertDirty()
			}
			if err := restore(nil, false); err != nil {
				t.Fatal("fixed-object backup replay failed", err)
			}
			var restoredHead bson.Raw
			if err := n.mongoDB.Collection("schema_migrations").FindOne(t.Context(), bson.D{}).Decode(&restoredHead); err != nil || !bytes.Equal(headBackup, restoredHead) {
				t.Fatal("original version BSON/_id/type was not restored exactly", err)
			}
			cursor, err = n.mongoDB.ListCollections(t.Context(), bson.D{{Key: "name", Value: "schema_migrations"}})
			if err != nil {
				t.Fatal(err)
			}
			var restoredCatalogs []bson.Raw
			if err := cursor.All(t.Context(), &restoredCatalogs); err != nil || len(restoredCatalogs) != 1 || !bytes.Equal(catalogs[0].Lookup("options").Value, restoredCatalogs[0].Lookup("options").Value) {
				t.Fatal("version collection options not restored", err)
			}
			if !bytes.Equal(dirtyCatalogs[0].Lookup("info").Document().Lookup("uuid").Value, restoredCatalogs[0].Lookup("info").Document().Lookup("uuid").Value) {
				t.Fatal("transactional replay changed the existing metadata namespace UUID")
			}
			if !reflect.DeepEqual(indexes, mongoIndexInventory(t, n.mongoDB)) || !reflect.DeepEqual(namespaces, mongoBusinessNamespaceInventory(t, n.mongoDB)) || !reflect.DeepEqual(documents, businessDocuments()) {
				t.Fatal("replay changed business BSON/namespaces or exact index specs")
			}
			oldResources := migrationResourcesThrough(t, 101, 39)
			if err := n.preflight(t, oldResources).VerifyComplete(t.Context()); err != nil {
				t.Fatal("fresh101/39 preflight rejected the actual restored pair", err)
			}
		})
	}
}

func TestHistoricalIndexNativePartialFailureAndDirtyRefuseRepairByForce(t *testing.T) {
	n := nativeHistoricalIndexPair(t)
	before, documents := mongoIndexInventory(t, n.mongoDB), mongoFrozenDocuments(t, n.mongoDB)
	source := embeddedMigrationSource(t)
	source[historicalIndexUp].Data = bytes.Replace(source[historicalIndexUp].Data, []byte(`"index":"uk_generation_historical_generated_event_id"`), []byte(`"index":"m4_missing_index"`), 1)
	r, err := readMigrationResources(source, declaredSchemaPairs)
	if err != nil {
		t.Fatal(err)
	}
	p := n.preflight(t, r)
	if _, _, err := NewMigrator(n.db, p.MySQLConfig(false)).Run(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := NewMongoMigrator(n.client, p.MongoConfig(false)).Run(); err == nil {
		t.Fatal("partial index DDL did not fail")
	}
	delete(before, "answersheets/uk_answersheet_legacy_submission_event_id")
	delete(before, "answersheets/idx_answersheet_legacy_submission_audit")
	if !reflect.DeepEqual(before, mongoIndexInventory(t, n.mongoDB)) || !reflect.DeepEqual(documents, mongoFrozenDocuments(t, n.mongoDB)) {
		t.Fatal("partial failure exceeded the two completed drops or changed documents")
	}
	if _, err := preflightCompatibilityPair(t.Context(), n.db, n.client, n.cfg, testCurrentResources(t)); err == nil {
		t.Fatal("fresh process adopted40dirty")
	}
	instance, err := NewMongoDriver(n.client).CreateInstance(r.source, ensureConfigDefaults(p.MongoConfig(false)))
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []int{39, 40} {
		if err := instance.Force(version); err == nil {
			t.Fatal("force concealed partial index retirement", version)
		}
	}
	down, err := migrations.ReadFile("migrations/mongodb/000040_retire_historical_event_indexes.down.json")
	if err != nil {
		t.Fatal(err)
	}
	var commandErr mongo.CommandError
	if err := runHistoricalIndexCommands(t, n.mongoDB, down); !errors.As(err, &commandErr) || commandErr.Code != 59 {
		t.Fatal("down must explicitly refuse automatic reconstruction", err)
	}
	if sql, dirty, mongoHead, mongoDirty := n.heads(t); sql != 101 || mongoHead != 40 || dirty || !mongoDirty {
		t.Fatal("partial index retirement must retain101clean/40dirty", sql, dirty, mongoHead, mongoDirty)
	}
}

func TestHistoricalIndexNativeUnknownMetadataStopsBeforeDDL(t *testing.T) {
	n := nativeHistoricalIndexPair(t)
	before := mongoIndexInventory(t, n.mongoDB)
	if _, err := n.mongoDB.Collection("schema_migrations").InsertOne(t.Context(), bson.M{"version": 40, "dirty": false}); err != nil {
		t.Fatal(err)
	}
	if _, err := preflightCompatibilityPair(t.Context(), n.db, n.client, n.cfg, testCurrentResources(t)); err == nil {
		t.Fatal("ambiguous metadata allowed migration")
	}
	if !reflect.DeepEqual(before, mongoIndexInventory(t, n.mongoDB)) {
		t.Fatal("unknown metadata changed indexes")
	}
}

func TestHistoricalIndexNativePermissionDeniedAfterMetadataWrite(t *testing.T) {
	configPath := os.Getenv("QS_M4_TEST_MONGO_AUTH_CONFIG")
	if configPath == "" {
		t.Fatal("QS_M4_TEST_MONGO_AUTH_CONFIG required; authenticated permission test cannot skip")
	}
	raw, err := os.ReadFile(configPath)
	var cfg struct {
		URI string `json:"mongo_uri"`
	}
	if err != nil || json.Unmarshal(raw, &cfg) != nil || cfg.URI == "" {
		t.Fatal("isolated authenticated Mongo configuration unavailable")
	}
	t.Setenv("QS_SERVER_TEST_MONGO_URI", cfg.URI)
	t.Setenv("QS_SERVER_TEST_MONGO_DB_PREFIX", "qs_m4_auth_test")
	n := nativeHistoricalIndexPair(t)
	before := mongoIndexInventory(t, n.mongoDB)
	role, user := "m4_no_index_drop", "m4_scoped_migrator"
	if err := n.mongoDB.RunCommand(t.Context(), bson.D{{Key: "createRole", Value: role}, {Key: "privileges", Value: bson.A{
		bson.M{"resource": bson.M{"db": n.mongoDB.Name(), "collection": ""}, "actions": bson.A{"find", "listCollections", "listIndexes"}},
		bson.M{"resource": bson.M{"db": n.mongoDB.Name(), "collection": "schema_migrations"}, "actions": bson.A{"find", "insert", "remove", "update", "dropCollection"}},
	}}, {Key: "roles", Value: bson.A{}}}).Err(); err != nil {
		t.Fatal(err)
	}
	var passwordBytes [24]byte
	if _, err := rand.Read(passwordBytes[:]); err != nil {
		t.Fatal(err)
	}
	password := hex.EncodeToString(passwordBytes[:])
	if err := n.mongoDB.RunCommand(t.Context(), bson.D{{Key: "createUser", Value: user}, {Key: "pwd", Value: password}, {Key: "roles", Value: bson.A{role}}}).Err(); err != nil {
		t.Fatal("create scoped fixture user failed")
	}
	t.Cleanup(func() {
		for _, command := range []bson.D{{{Key: "dropUser", Value: user}}, {{Key: "dropRole", Value: role}}} {
			if err := n.mongoDB.RunCommand(context.Background(), command).Err(); err != nil {
				t.Error("remove only owned test user/role failed")
			}
		}
	})
	client, err := mongo.Connect(t.Context(), options.Client().ApplyURI(cfg.URI).SetAuth(options.Credential{AuthSource: n.mongoDB.Name(), Username: user, Password: password}))
	if err != nil {
		t.Fatal("restricted fixture connection failed")
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	r := testCurrentResources(t)
	p, err := preflightCompatibilityPair(t.Context(), n.db, client, n.cfg, r)
	if err != nil {
		t.Fatal("read/catalog preflight must be authorized before the drop denial", err)
	}
	if _, _, err := NewMigrator(n.db, p.MySQLConfig(false)).Run(); err != nil {
		t.Fatal(err)
	}
	_, _, err = NewMongoMigrator(client, p.MongoConfig(false)).Run()
	var databaseErr *migratedb.Error
	var commandErr mongo.CommandError
	if !errors.As(err, &databaseErr) || !errors.As(databaseErr.OrigErr, &commandErr) || commandErr.Code != 13 {
		t.Fatal("dropIndexes must fail with real Mongo Unauthorized13 after metadata write", err)
	}
	if sql, dirty, mongoHead, mongoDirty := n.heads(t); sql != 101 || mongoHead != 40 || dirty || !mongoDirty {
		t.Fatal("permission rejection must preserve101clean/40dirty", sql, dirty, mongoHead, mongoDirty)
	}
	if !reflect.DeepEqual(before, mongoIndexInventory(t, n.mongoDB)) {
		t.Fatal("permission rejection changed indexes")
	}
	if _, err := preflightCompatibilityPair(t.Context(), n.db, n.client, n.cfg, r); err == nil {
		t.Fatal("administrator fresh process adopted dirty unauthorized migration")
	}
}
