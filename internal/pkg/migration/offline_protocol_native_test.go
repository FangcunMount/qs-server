//go:build integration

package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	txmongo "github.com/FangcunMount/reliable-messaging/storage/transaction/mongo"
	txmysql "github.com/FangcunMount/reliable-messaging/storage/transaction/mysql"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestSchemaPairNativeM5ProtocolReadOnly(t *testing.T) {
	for _, version := range []uint{110, 111} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			r, err := compiledMigrationResources()
			if err != nil {
				t.Fatal(err)
			}
			if version == 110 {
				r = migrationResourcesThrough(t, 110, 44)
			}
			n := openNativeSchemaPairWithPrefix(t, "qs_m5_protocol_test")
			n.approve(t, r)
			n.runBoth(t, n.preflight(t, r))
			assertM5ProtocolReadOnly(t, n)
		})
	}
	if t.Failed() {
		return
	}
	if path := os.Getenv("QS_M5_PROTOCOL_PROOF"); path != "" {
		raw, err := json.MarshalIndent(map[string]any{"fixture_only": true, "source_sha": os.Getenv("GITHUB_SHA"), "mysql_versions": []uint{110, 111}, "mongo_version": 44, "both_primaries_read": true, "zero_metadata_observed": true, "reserved_executions_not_hidden": true, "original_full_rows_and_documents_unchanged": true, "invalid_and_missing_binding_unavailable": true, "sender_or_business_runtime_started": false, "broker_pending_or_freeze_certified": false}, "", "  ")
		if err != nil || os.WriteFile(path, append(raw, '\n'), 0600) != nil {
			t.Fatal("owned protocol proof write failed")
		}
	}
}

func assertM5ProtocolReadOnly(t *testing.T, n nativeSchemaPair) {
	t.Helper()
	n.cfg.BootstrapAuthorizationFile, n.cfg.BootstrapAuthorizationSHA256 = "", ""
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	sqlBinding := transaction.Binding{Owner: "qs.mysql", StoreID: "owned-sql", WriterEpoch: "m3-1"}
	mongoBinding := transaction.Binding{Owner: "qs.mongo", StoreID: "owned-mongo", WriterEpoch: "m3-1"}
	if _, err := n.db.ExecContext(ctx, "INSERT INTO rm_tx_binding VALUES(?,?,?,1)", []byte(sqlBinding.Owner), []byte(sqlBinding.StoreID), []byte(sqlBinding.WriterEpoch)); err != nil {
		t.Fatal(err)
	}
	if _, err := n.mongoDB.Collection("rm_tx_binding").InsertOne(ctx, bson.M{"_id": mongoBinding.Owner, "store_id": mongoBinding.StoreID, "writer_epoch": mongoBinding.WriterEpoch, "schema_version": 1}); err != nil {
		t.Fatal(err)
	}
	zero, err := ObserveOfflineProtocol(ctx, n.db, n.client, n.cfg)
	if err != nil || !zero.Available || !zero.NoObservedUnresolved || zero.RuntimeFrozen || zero.BrokerPendingKnown {
		t.Fatal("empty primary metadata not observed", err)
	}
	s, err := txmysql.New(n.db, sqlBinding)
	if err != nil {
		t.Fatal(err)
	}
	ms, err := txmongo.New(n.mongoDB, mongoBinding)
	if err != nil {
		t.Fatal(err)
	}
	for _, owner := range []struct {
		binding transaction.Binding
		store   transaction.PreparedStore
	}{{sqlBinding, s}, {mongoBinding, ms}} {
		p := m3Plan(t, owner.binding.Owner, "read-only-protocol")
		if r, err := owner.store.Reserve(ctx, transaction.RequestFor(p), p, transaction.ReserveOptions{Lease: time.Minute}); err != nil || !r.Acquired {
			t.Fatal("owned unresolved execution fixture", err)
		}
	}
	// Compare original stored rows/documents. No sender, Checker, business
	// process, schema installer or reconciliation is started by observation.
	sqlFacts := func() any {
		return []any{
			historicalQuery(t, n.db, "SELECT * FROM schema_migrations"),
			historicalQuery(t, n.db, "SELECT * FROM rm_tx_binding ORDER BY owner"),
			historicalQuery(t, n.db, "SELECT * FROM rm_tx_request ORDER BY request_key"),
			historicalQuery(t, n.db, "SELECT * FROM rm_tx_execution ORDER BY execution_key"),
			historicalQuery(t, n.db, "SELECT * FROM rm_tx_witness ORDER BY execution_key,branch_key"),
		}
	}
	mongoFacts := func() []bson.Raw {
		var result []bson.Raw
		for _, name := range []string{"schema_migrations", "rm_tx_binding", "rm_tx_request", "rm_tx_execution", "messaging_native_limit_notices"} {
			cursor, err := n.mongoDB.Collection(name).Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
			if err != nil {
				t.Fatal(err)
			}
			var rows []bson.Raw
			if err := cursor.All(ctx, &rows); err != nil {
				t.Fatal(err)
			}
			result = append(result, rows...)
		}
		return result
	}
	beforeSQL, beforeMongo := sqlFacts(), mongoFacts()
	pending, err := ObserveOfflineProtocol(ctx, n.db, n.client, n.cfg)
	if err != nil || !pending.Available || pending.NoObservedUnresolved || pending.MySQL.Protocol.ReservedExecutions != 1 || pending.Mongo.Protocol.ReservedExecutions != 1 {
		t.Fatal("reserved execution was hidden", err)
	}
	if !reflect.DeepEqual(beforeSQL, sqlFacts()) || !reflect.DeepEqual(beforeMongo, mongoFacts()) {
		t.Fatal("observation changed original stored facts")
	}
	if _, err := n.mongoDB.Collection("rm_tx_binding").UpdateByID(ctx, mongoBinding.Owner, bson.M{"$set": bson.M{"schema_version": 99}}); err != nil {
		t.Fatal(err)
	}
	unavailable, err := ObserveOfflineProtocol(ctx, n.db, n.client, n.cfg)
	if err == nil || unavailable.Available || unavailable.NoObservedUnresolved || unavailable.Mongo != nil || unavailable.Phase != "qs_mongo_protocol" {
		t.Fatal("invalid owner metadata became zero")
	}
	if _, err := n.db.ExecContext(ctx, "DELETE FROM rm_tx_binding WHERE owner=?", []byte(sqlBinding.Owner)); err != nil {
		t.Fatal(err)
	}
	missing, err := ObserveOfflineProtocol(ctx, n.db, n.client, n.cfg)
	if err == nil || missing.Available || missing.NoObservedUnresolved || missing.MySQL != nil || missing.Phase != "qs_mysql_binding" {
		t.Fatal("missing original binding became zero")
	}
}
