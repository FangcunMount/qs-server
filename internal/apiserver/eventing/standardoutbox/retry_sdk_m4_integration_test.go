//go:build reliable_messaging_m4 && reliable_messaging_m4_integration

package standardoutbox

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/outbox"
	"github.com/FangcunMount/reliable-messaging/relay"
	sdkmongo "github.com/FangcunMount/reliable-messaging/storage/mongo"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	"github.com/FangcunMount/reliable-messaging/transport"
	mysqldriver "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	driver "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type fixedPublisher struct{ outcome transport.Outcome }

func (p fixedPublisher) Publish(context.Context, message.Message) transport.Result {
	return transport.Result{Outcome: p.outcome}
}

func runCandidateRelayOnce(t *testing.T, store outbox.Store, outcome transport.Outcome) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	wrote := false
	r, err := relay.New(store, fixedPublisher{outcome}, relay.Config{
		Concurrency: 1, PollInterval: 10 * time.Millisecond, Lease: 2 * time.Second,
		PublishTimeout: 100 * time.Millisecond, WriteTimeout: 100 * time.Millisecond,
		Retry: SDKRetryPolicy(), Observe: func(evt relay.Event) {
			if evt.Kind == "write_succeeded" {
				wrote = true
				cancel()
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Run(ctx); err != nil || !wrote {
		t.Fatalf("relay write not observed: err=%v wrote=%t ctx=%v", err, wrote, ctx.Err())
	}
}

func m4RetryMessage(t *testing.T, id string) message.Message {
	t.Helper()
	m, err := message.New(message.Input{
		Producer: "qs-server", ID: id, Destination: "qs.evaluation.lifecycle",
		EventType: "evaluation.requested", SchemaVersion: "v1", Scope: "org:1",
		ContentType: "application/json", OccurredAt: "2026-09-23T10:00:00+08:00",
		Payload: []byte(`{"event_id":"` + id + `"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestCandidateRetryBudgetMySQLRelay(t *testing.T) {
	dsn := os.Getenv("RM_QS_RETRY_MYSQL_DSN")
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil || parsed.Net != "tcp" || parsed.Addr != "127.0.0.1:3306" || parsed.DBName != "m4_qs_retry" {
		t.Fatal("disposable m4_qs_retry MySQL in a local test container required")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS rm_outbox"); err != nil {
		t.Fatal(err)
	}
	defer db.ExecContext(context.Background(), "DROP TABLE IF EXISTS rm_outbox")
	if _, err := db.ExecContext(ctx, sdkmysql.Schema); err != nil {
		t.Fatal(err)
	}
	store, err := sdkmysql.New(db)
	if err != nil {
		t.Fatal(err)
	}
	appendMessage := func(id string, failures uint64) {
		t.Helper()
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		a, err := sdkmysql.Bind(tx)
		if err != nil {
			t.Fatal(err)
		}
		if err := a.Append(ctx, m4RetryMessage(t, id), time.Now().Add(-time.Minute)); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE rm_outbox SET failure_count=? WHERE message_id=?", failures, id); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	read := func(id string, wantState, wantCode string, wantFailures uint64) {
		t.Helper()
		var state, code string
		var failures uint64
		if err := db.QueryRowContext(ctx, "SELECT state,last_error_code,failure_count FROM rm_outbox WHERE message_id=?", id).Scan(&state, &code, &failures); err != nil {
			t.Fatal(err)
		}
		if state != wantState || code != wantCode || failures != wantFailures {
			t.Fatalf("%s state=%s code=%s failures=%d; want %s/%s/%d", id, state, code, failures, wantState, wantCode, wantFailures)
		}
	}
	appendMessage("near-budget", 28)
	runCandidateRelayOnce(t, store, transport.Unknown)
	read("near-budget", "retry_wait", "publish_unknown", 29)
	if _, err := db.ExecContext(ctx, "UPDATE rm_outbox SET next_attempt_at=UTC_TIMESTAMP(6)-INTERVAL 1 SECOND WHERE message_id='near-budget'"); err != nil {
		t.Fatal(err)
	}
	runCandidateRelayOnce(t, store, transport.Unknown)
	read("near-budget", "quarantined", "publish_unknown", 30)
	appendMessage("rejected", 0)
	runCandidateRelayOnce(t, store, transport.Rejected)
	read("rejected", "quarantined", "publish_rejected", 1)
}

func TestCandidateRetryBudgetMongoRelay(t *testing.T) {
	uri := os.Getenv("RM_QS_RETRY_MONGO_URI")
	if !strings.HasPrefix(uri, "mongodb://mongo:27017/") || !strings.Contains(uri, "replicaSet=rm-test") {
		t.Fatal("disposable rm-test replica-set Mongo URI required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := driver.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Disconnect(context.Background())
	db := client.Database("m4_qs_retry")
	defer db.Drop(context.Background())
	coll := db.Collection("rm_outbox")
	if _, err := coll.Indexes().CreateMany(ctx, sdkmongo.Indexes()); err != nil {
		t.Fatal(err)
	}
	store, err := sdkmongo.New(coll)
	if err != nil {
		t.Fatal(err)
	}
	appendMessage := func(id string, failures int64) {
		t.Helper()
		m := m4RetryMessage(t, id)
		session, err := client.StartSession()
		if err != nil {
			t.Fatal(err)
		}
		defer session.EndSession(ctx)
		_, err = session.WithTransaction(ctx, func(sc driver.SessionContext) (interface{}, error) {
			appender, err := sdkmongo.Bind(sc, coll)
			if err != nil {
				return nil, err
			}
			return nil, appender.Append(m, time.Now().Add(-time.Minute))
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := coll.UpdateOne(ctx, bson.M{"message_id": id}, bson.M{"$set": bson.M{"failure_count": failures}}); err != nil {
			t.Fatal(err)
		}
	}
	read := func(id string, wantState, wantCode string, wantFailures uint64) {
		t.Helper()
		var row struct {
			State        string `bson:"state"`
			Code         string `bson:"last_error_code"`
			FailureCount uint64 `bson:"failure_count"`
		}
		if err := coll.FindOne(ctx, bson.M{"message_id": id}).Decode(&row); err != nil {
			t.Fatal(err)
		}
		if row.State != wantState || row.Code != wantCode || row.FailureCount != wantFailures {
			t.Fatalf("%s state=%s code=%s failures=%d; want %s/%s/%d", id, row.State, row.Code, row.FailureCount, wantState, wantCode, wantFailures)
		}
	}
	appendMessage("near-budget", 28)
	runCandidateRelayOnce(t, store, transport.Unknown)
	read("near-budget", "retry_wait", "publish_unknown", 29)
	if _, err := coll.UpdateOne(ctx, bson.M{"message_id": "near-budget"}, bson.M{"$set": bson.M{"next_attempt_at": time.Now().Add(-time.Minute)}}); err != nil {
		t.Fatal(err)
	}
	runCandidateRelayOnce(t, store, transport.Unknown)
	read("near-budget", "quarantined", "publish_unknown", 30)
	appendMessage("rejected", 0)
	runCandidateRelayOnce(t, store, transport.Rejected)
	read("rejected", "quarantined", "publish_rejected", 1)
}
