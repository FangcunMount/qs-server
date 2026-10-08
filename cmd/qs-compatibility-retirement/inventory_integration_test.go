//go:build integration

package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// This test refuses every production endpoint; the harness creates disposable
// loopback-only MySQL 8 / Mongo 7 instances with synthetic payloads.
func TestIsolatedRealInventoryPresenceStates(t *testing.T) {
	if os.Getenv("QS_RETIREMENT_LOCAL_INTEGRATION") != "1" {
		t.Skip("isolated databases not requested")
	}
	if os.Getenv("MYSQL_HOST") != "127.0.0.1" || os.Getenv("MONGODB_HOST") != "127.0.0.1" || os.Getenv("MYSQL_DATABASE") != "qs_retirement_inventory_test" || os.Getenv("MONGODB_DBNAME") != "qs_retirement_inventory_test" {
		t.Fatal("non-isolated target rejected")
	}
	sourceSHA = strings.Repeat("a", 40)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	db, e := mysqlOpen(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if e := db.Close(); e != nil {
			t.Error(e)
		}
	}()
	client, e := mongoOpen(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if e := client.Disconnect(context.Background()); e != nil {
			t.Error(e)
		}
	}()
	mdb := client.Database(os.Getenv("MONGODB_DBNAME"))
	for _, statement := range []string{"CREATE TABLE schema_migrations(version BIGINT NOT NULL,dirty BOOL NOT NULL)", "INSERT INTO schema_migrations VALUES(95,FALSE)", "CREATE TABLE kept_fact(id BIGINT PRIMARY KEY,note TEXT)", "CREATE TABLE domain_event_outbox(id BIGINT UNSIGNED PRIMARY KEY,event_type VARCHAR(128),status VARCHAR(32),payload_json LONGTEXT)", "CREATE TABLE ai_bridge_commands(command_id CHAR(36) PRIMARY KEY,kind VARCHAR(32),delivered BOOL,payload JSON)", "CREATE TABLE ai_messaging_legacy_commands(command_id CHAR(36) PRIMARY KEY,source_kind VARCHAR(32),source_payload MEDIUMBLOB)", "INSERT INTO domain_event_outbox VALUES(1,'footprint.entry_opened','published','{\"sentinel\":\"private SQL bytes\"}'),(2,'TEST_PRIVATE_UNKNOWN_TYPE','pending',NULL)", "INSERT INTO ai_bridge_commands VALUES('command-a','request',TRUE,'{\"value\":1}')", "INSERT INTO ai_messaging_legacy_commands VALUES('command-b','prepare',X'00FF10')"} {
		if _, e = db.ExecContext(ctx, statement); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = db.ExecContext(ctx, "ALTER TABLE domain_event_outbox CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); e != nil {
		t.Fatal(e)
	}
	if _, e = db.ExecContext(ctx, "INSERT INTO domain_event_outbox VALUES(3,'Footprint.entry_opened','published',NULL),(4,'footprint.entry_opened ','published',NULL)"); e != nil {
		t.Fatal(e)
	}
	if _, e = mdb.Collection("schema_migrations").InsertOne(ctx, bson.D{{Key: "version", Value: int64(36)}, {Key: "dirty", Value: false}}); e != nil {
		t.Fatal(e)
	}
	if _, e = mdb.Collection("kept_fact").InsertOne(ctx, bson.D{{Key: "_id", Value: 1}, {Key: "note", Value: "private Mongo bytes"}}); e != nil {
		t.Fatal(e)
	}
	if e = mdb.CreateCollection(ctx, "domain_event_outbox", options.CreateCollection().SetCollation(&options.Collation{Locale: "en", Strength: 2})); e != nil {
		t.Fatal(e)
	}
	if _, e = mdb.Collection("domain_event_outbox").InsertOne(ctx, bson.D{{Key: "_id", Value: "event-a"}, {Key: "event_type", Value: "interpretation.ai_explanation.generated"}, {Key: "status", Value: "published"}, {Key: "payload", Value: bson.D{{Key: "sentinel", Value: "private original BSON"}}}}); e != nil {
		t.Fatal(e)
	}
	if _, e = mdb.Collection("domain_event_outbox").InsertOne(ctx, bson.D{{Key: "_id", Value: "event-b"}, {Key: "event_type", Value: "Interpretation.ai_explanation.generated"}, {Key: "status", Value: "published"}}); e != nil {
		t.Fatal(e)
	}
	var uuid, schema string
	if e = db.QueryRowContext(ctx, "SELECT @@server_uuid,DATABASE()").Scan(&uuid, &schema); e != nil {
		t.Fatal(e)
	}
	collections, _, e := mongoSchemas(ctx, mdb)
	if e != nil {
		t.Fatal(e)
	}
	_, bytes := collections["schema_migrations"].Lookup("info", "uuid").Binary()
	stable, _ := json.Marshal(bson.D{})
	identities := map[string]string{"mysql": hashParts("mysql_database_identity_v1", uuid, schema), "mongodb": hashParts("mongodb_database_identity_v1", string(stable), os.Getenv("MONGODB_DBNAME"), hex.EncodeToString(bytes))}
	r := request{FormatVersion: 1, Kind: "readonly_inventory_request", OperationID: "123-1", SourceSHA: sourceSHA, TargetHash: digest(targets), DatabaseScope: "mysql-and-mongodb", Identities: identities, Migrations: map[string]uint64{"mysql": 95, "mongodb": 36}}
	r.Limits.QuerySeconds = querySeconds
	r.Limits.TotalSeconds = totalSeconds
	r.Limits.MaxRecords = maxRecords
	r.Limits.MaxBytes = maxBytes
	for state := 0; state < 3; state++ {
		if state == 1 {
			if _, e = db.ExecContext(ctx, "DROP TABLE ai_bridge_commands"); e != nil {
				t.Fatal(e)
			}
			if e = mdb.Collection("domain_event_outbox").Drop(ctx); e != nil {
				t.Fatal(e)
			}
		}
		if state == 2 {
			for _, name := range []string{"domain_event_outbox", "ai_messaging_legacy_commands"} {
				if _, e = db.ExecContext(ctx, "DROP TABLE "+quote(name)); e != nil {
					t.Fatal(e)
				}
			}
		}
		dir := t.TempDir()
		dir, e = filepath.EvalSymlinks(dir)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.Chmod(dir, 0700); e != nil {
			t.Fatal(e)
		}
		reqPath := filepath.Join(dir, "inventory-request.json")
		raw, _ := json.Marshal(r)
		if e = os.WriteFile(reqPath, raw, 0600); e != nil {
			t.Fatal(e)
		}
		out := filepath.Join(dir, "out")
		if e = os.Mkdir(out, 0700); e != nil {
			t.Fatal(e)
		}
		identity := identityRequest{FormatVersion: 1, Kind: "readonly_identity_discovery_request", SourceSHA: sourceSHA, OperationID: "123-1", TargetHash: digest(targets), DatabaseScope: "mysql-and-mongodb", Protocols: identityProtocols()}
		identity.Limits.QuerySeconds, identity.Limits.TotalSeconds = 15, 90
		identityRaw, _ := json.Marshal(identity)
		identityPath := filepath.Join(dir, "identity-request.json")
		if e = os.WriteFile(identityPath, identityRaw, 0600); e != nil {
			t.Fatal(e)
		}
		identityOut := filepath.Join(dir, "identity-out")
		if e = os.Mkdir(identityOut, 0700); e != nil {
			t.Fatal(e)
		}
		discovery, discoverErr := runIdentity(identityPath, digestRaw(identityRaw), "123-1", "456-1", identityOut)
		if discoverErr != nil || !discovery.Complete || discovery.DropReady || !discovery.DiagnosticOnly || len(discovery.Histograms) != 4 {
			t.Fatalf("identity state %d: %v", state, discoverErr)
		}
		for name, binding := range discovery.States {
			if binding.IdentityHash != identities[name] || !binding.Clean || !binding.PermissionsSufficient {
				t.Fatal("identity protocol or clean head mismatch")
			}
		}
		observed := 0
		for _, histogram := range discovery.Histograms {
			if !histogram.Complete || histogram.Present == nil || !histogram.DiagnosticOnly {
				t.Fatal("histogram incomplete")
			}
			if *histogram.Present {
				observed++
			}
		}
		if observed != []int{4, 2, 0}[state] {
			t.Fatal("diagnostic presence mismatch")
		}
		if state == 0 {
			if len(discovery.Histograms[0].Buckets) != 4 || len(discovery.Histograms[3].Buckets) != 2 {
				t.Fatal("case/space collation hid unknown type")
			}
			count := uint64(0)
			for _, h := range discovery.Histograms {
				for _, b := range h.Buckets {
					count += b.Records
				}
			}
			if count != 8 {
				t.Fatal("full diagnostic count mismatch")
			}
		}
		discoveryBytes, _ := json.Marshal(identitySummary(discovery))
		for _, private := range []string{"TEST_PRIVATE_UNKNOWN_TYPE", "private SQL bytes", "private original BSON", "event-a", "command-a"} {
			if strings.Contains(string(discoveryBytes), private) {
				t.Fatal("diagnostic exposes private source")
			}
		}
		identityReportBytes, readErr := os.ReadFile(filepath.Join(identityOut, "identity.private.json"))
		if readErr != nil || digestRaw(identityReportBytes) != identitySummary(discovery)["private_report_hash"] {
			t.Fatal("identity private report not bound")
		}
		result, e := run(reqPath, digestRaw(raw), "123-1", "456-1", out)
		if e != nil || !result.Complete || result.DropReady || len(result.Targets) != 4 {
			t.Fatalf("state %d failed: %v / %#v", state, e, result.DatabaseBindings)
		}
		expected := []int{4, 2, 0}[state]
		present := 0
		for _, s := range result.Targets {
			if s.Present {
				present++
				if !s.Complete || s.DataHash == "" || s.SchemaHash == "" || s.SourceFile == "" {
					t.Fatal("missing full source evidence")
				}
			}
		}
		if present != expected {
			t.Fatalf("presence=%d want=%d", present, expected)
		}
		summary := safeSummary(result)
		data, _ := json.Marshal(summary)
		for _, sentinel := range []string{"private SQL bytes", "private original BSON", "private Mongo bytes", "command-a"} {
			if strings.Contains(string(data), sentinel) {
				t.Fatal("private source in safe summary")
			}
		}
		rawReport, e := os.ReadFile(filepath.Join(out, "inventory.private.json"))
		if e != nil || digestRaw(rawReport) != summary["private_report_hash"] {
			t.Fatal("private hash binding failed")
		}
	}
	if _, e = db.ExecContext(ctx, "UPDATE schema_migrations SET dirty=TRUE"); e != nil {
		t.Fatal(e)
	}
	dir := t.TempDir()
	d, _, e := mysqlInventory(ctx, r, dir)
	if e == nil || !d.Dirty {
		t.Fatal("dirty head accepted")
	}
	if identity, discoverErr := discoverMySQL(ctx); discoverErr == nil || identity.Clean || identity.Dirty == nil || !*identity.Dirty {
		t.Fatal("identity discovery accepted dirty SQL")
	}
	if _, e = db.ExecContext(ctx, "UPDATE schema_migrations SET dirty=FALSE"); e != nil {
		t.Fatal(e)
	}
	wrong := r
	wrong.Identities = map[string]string{"mysql": strings.Repeat("0", 64), "mongodb": identities["mongodb"]}
	if _, _, e = mysqlInventory(ctx, wrong, dir); e == nil {
		t.Fatal("wrong database accepted")
	}
	if _, e = db.ExecContext(ctx, "CREATE VIEW domain_event_outbox AS SELECT id,note FROM kept_fact"); e != nil {
		t.Fatal(e)
	}
	if _, _, e = mysqlInventory(ctx, r, dir); e == nil || e.Error() != "target_type_rejected" {
		t.Fatal("wrong MySQL namespace kind accepted")
	}
	if _, e = db.ExecContext(ctx, "DROP VIEW domain_event_outbox"); e != nil {
		t.Fatal(e)
	}
	if e = mdb.CreateView(ctx, "domain_event_outbox", "kept_fact", []bson.D{}); e != nil {
		t.Fatal(e)
	}
	if _, _, e = mongoInventory(ctx, r, dir); e == nil || e.Error() != "target_type_rejected" {
		t.Fatal("wrong Mongo namespace kind accepted")
	}
	if e = mdb.Collection("domain_event_outbox").Drop(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e = mdb.Collection("schema_migrations").UpdateOne(ctx, bson.D{}, bson.D{{Key: "$set", Value: bson.D{{Key: "dirty", Value: true}}}}); e != nil {
		t.Fatal(e)
	}
	if identity, discoverErr := discoverMongo(ctx); discoverErr == nil || identity.Clean || identity.Dirty == nil || !*identity.Dirty {
		t.Fatal("identity discovery accepted dirty Mongo")
	}
	if _, e = mdb.Collection("schema_migrations").UpdateOne(ctx, bson.D{}, bson.D{{Key: "$set", Value: bson.D{{Key: "dirty", Value: false}}}}); e != nil {
		t.Fatal(e)
	}
	if e = mdb.Collection("schema_migrations").Drop(ctx); e != nil {
		t.Fatal(e)
	}
	if identity, discoverErr := discoverMongo(ctx); discoverErr == nil || identity.IdentityObserved {
		t.Fatal("identity discovery accepted absent database UUID")
	}
	if _, e = mdb.Collection("schema_migrations").InsertOne(ctx, bson.D{{Key: "version", Value: int64(36)}, {Key: "dirty", Value: false}}); e != nil {
		t.Fatal(e)
	}
	if _, e = db.ExecContext(ctx, "CREATE TABLE domain_event_outbox(id BIGINT PRIMARY KEY,event_type VARCHAR(128),status VARCHAR(32))"); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 129; i++ {
		if _, e = db.ExecContext(ctx, "INSERT INTO domain_event_outbox VALUES(?,?,'published')", i, "unknown-distinct-"+strconv.Itoa(i)); e != nil {
			t.Fatal(e)
		}
	}
	tx, e := db.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	histogram, histogramErr := sqlHistogram(ctx, tx, "domain_event_outbox")
	if closeErr := tx.Rollback(); closeErr != nil {
		t.Fatal(closeErr)
	}
	if histogramErr == nil || histogramErr.Error() != "histogram_bucket_bound_exceeded" || histogram.Complete || len(histogram.Buckets) != 0 {
		t.Fatal("bounded histogram accepted excess distinct types")
	}
	if _, e = db.ExecContext(ctx, "CREATE USER 'local_inventory_limited'@'%' IDENTIFIED BY 'local_inventory_limited_password'"); e != nil {
		t.Fatal(e)
	}
	if _, e = db.ExecContext(ctx, "GRANT SELECT ON qs_retirement_inventory_test.* TO 'local_inventory_limited'@'%'"); e != nil {
		t.Fatal(e)
	}
	t.Setenv("MYSQL_USERNAME", "local_inventory_limited")
	t.Setenv("MYSQL_PASSWORD", "local_inventory_limited_password")
	if _, _, e = mysqlInventory(ctx, r, dir); e == nil || e.Error() != "mysql_global_metadata_visibility_unproven" {
		t.Fatal("partial metadata permission accepted")
	}
	if identity, discoverErr := discoverMySQL(ctx); discoverErr == nil || identity.PermissionsSufficient {
		t.Fatal("identity discovery accepted partial metadata permission")
	}
}
