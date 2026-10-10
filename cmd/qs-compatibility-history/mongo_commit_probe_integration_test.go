//go:build integration

package main

import (
	"context"
	"errors"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/event"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// The original owned inventory launcher supplies both credentials and its
// independently allocated loopback port. Never adopt an ordinary local or
// production database just because credentials happen to be available.
func TestOwnedMongoCommitProbeUsesLiveServerTransaction(t *testing.T) {
	if os.Getenv("QS_RETIREMENT_LOCAL_INTEGRATION") != "1" {
		t.Skip("owned inventory launcher not selected")
	}
	port, err := strconv.Atoi(os.Getenv("MONGODB_PORT"))
	if err != nil || port < 1024 || port > 65535 || os.Getenv("MONGODB_HOST") != "127.0.0.1" || os.Getenv("MONGODB_DBNAME") != "qs_retirement_inventory_test" {
		t.Fatal("owned loopback fixture required")
	}
	// nativeToken contributes 24 ASCII bytes; keep the actual database name
	// below MongoDB's 64-byte limit, including this independently owned prefix.
	namespace := "qs_history_commit_probe_" + nativeToken(t)
	var mu sync.Mutex
	requests := map[int64]bool{}
	var responses int
	monitor := &event.CommandMonitor{
		Started: func(_ context.Context, e *event.CommandStartedEvent) {
			if e.CommandName == "find" && e.DatabaseName == namespace {
				mu.Lock()
				requests[e.RequestID] = true
				mu.Unlock()
			}
		},
		Succeeded: func(_ context.Context, e *event.CommandSucceededEvent) {
			mu.Lock()
			if requests[e.RequestID] {
				responses++
			}
			mu.Unlock()
		},
	}
	opts := options.Client().SetHosts([]string{"127.0.0.1:" + strconv.Itoa(port)}).
		SetAuth(options.Credential{Username: os.Getenv("MONGODB_USERNAME"), Password: os.Getenv("MONGODB_PASSWORD"), AuthSource: "admin"}).
		SetConnectTimeout(5 * time.Second).SetServerSelectionTimeout(5 * time.Second).SetMonitor(monitor)
	client, err := mongo.Connect(t.Context(), opts)
	if err != nil || client.Ping(t.Context(), readpref.Primary()) != nil {
		t.Fatal("owned Mongo connection failed")
	}
	db := client.Database(namespace)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if dropErr := db.Drop(ctx); dropErr != nil {
			var serverError mongo.CommandError
			if errors.As(dropErr, &serverError) {
				t.Errorf("owned probe namespace cleanup failed: server_code=%d", serverError.Code)
			} else {
				t.Error("owned probe namespace cleanup failed")
			}
		}
		names, e := client.ListDatabaseNames(ctx, bson.D{{Key: "name", Value: namespace}})
		if e != nil || len(names) != 0 {
			t.Error("owned probe namespace remaining")
		}
		if client.Disconnect(ctx) != nil {
			t.Error("owned probe client close failed")
		}
	})
	if err = db.CreateCollection(t.Context(), "rm_outbox"); err != nil {
		var serverError mongo.CommandError
		if errors.As(err, &serverError) {
			t.Fatalf("owned ledger creation failed: server_code=%d", serverError.Code)
		}
		t.Fatal("owned ledger creation failed")
	}
	for _, empty := range []bool{true, false} {
		name := "empty_ledger"
		if !empty {
			name = "nonempty_ledger"
			if _, err = db.Collection("rm_outbox").InsertOne(t.Context(), bson.D{{Key: "_id", Value: int64(1)}}); err != nil {
				t.Fatal("owned ledger insert failed")
			}
		}
		t.Run(name, func(t *testing.T) {
			session, e := client.StartSession()
			if e != nil {
				t.Fatal("owned session start failed")
			}
			defer session.EndSession(context.Background())
			if e = session.StartTransaction(options.Transaction().SetReadConcern(readconcern.Snapshot()).SetReadPreference(readpref.Primary())); e != nil {
				t.Fatal("owned transaction start failed")
			}
			if e = probePreparedMongo(t.Context(), db, session, "required"); e != nil {
				t.Fatal("actual server probe failed", e)
			}
			if e = requireMongoCommitTransaction(session, "required"); e != nil {
				t.Fatal("actual transaction did not enter InProgress")
			}
			if session.CommitTransaction(t.Context()) != nil {
				t.Fatal("actual server commit failed")
			}
		})
	}
	t.Run("server_killed_transaction_is_not_client_state_success", func(t *testing.T) {
		session, e := client.StartSession()
		if e != nil {
			t.Fatal("owned session start failed")
		}
		defer session.EndSession(context.Background())
		if session.StartTransaction(options.Transaction().SetReadConcern(readconcern.Snapshot()).SetReadPreference(readpref.Primary())) != nil || probePreparedMongo(t.Context(), db, session, "required") != nil {
			t.Fatal("initial actual transaction probe failed")
		}
		if e = client.Database("admin").RunCommand(t.Context(), bson.D{{Key: "killSessions", Value: bson.A{session.ID()}}}).Err(); e != nil {
			t.Fatal("owned server session kill failed")
		}
		if requireMongoCommitTransaction(session, "required") != nil {
			t.Fatal("test did not preserve the misleading local InProgress state")
		}
		if probePreparedMongo(t.Context(), db, session, "required") == nil {
			t.Fatal("server-killed transaction admitted before SQL commit")
		}
	})
	mu.Lock()
	defer mu.Unlock()
	if responses != 3 || len(requests) < 4 {
		t.Fatal("actual server command responses not observed")
	}
	t.Logf("actual_server_probe_find_responses=%d actual_find_attempts=%d", responses, len(requests))
}
