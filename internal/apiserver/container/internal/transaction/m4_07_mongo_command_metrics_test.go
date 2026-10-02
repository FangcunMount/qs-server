//go:build integration && (m4_07_old_chain || m4_07_new_chain)

package transaction

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/bsontype"
	"go.mongodb.org/mongo-driver/event"
)

// m407MongoCommandMetrics observes only the disposable test Outbox collection.
// It does not issue Mongo commands or change command results.
type m407MongoCommandMetrics struct {
	database   string
	collection string
	mu         sync.Mutex
	started    map[string]string
	completed  []m407MongoCommandSample
}

type m407MongoCommandSample struct {
	Operation  string `json:"operation"`
	DurationNS int64  `json:"duration_ns"`
	Failed     bool   `json:"failed"`
}

func newM407MongoCommandMetrics(database, collection string) *m407MongoCommandMetrics {
	return &m407MongoCommandMetrics{
		database: database, collection: collection,
		started: make(map[string]string),
	}
}

func (m *m407MongoCommandMetrics) Monitor() *event.CommandMonitor {
	return &event.CommandMonitor{
		Started: func(_ context.Context, e *event.CommandStartedEvent) {
			if e.DatabaseName != m.database {
				return
			}
			collection, ok := e.Command.Lookup(e.CommandName).StringValueOK()
			if !ok || collection != m.collection {
				return
			}
			operation := e.CommandName
			if operation == "update" {
				var command struct {
					Updates []struct {
						Upsert bool          `bson:"upsert"`
						Multi  bool          `bson:"multi"`
						Update bson.RawValue `bson:"u"`
					} `bson:"updates"`
				}
				if err := bson.Unmarshal(e.Command, &command); err == nil && len(command.Updates) == 1 {
					update := command.Updates[0]
					state := ""
					if document, ok := update.Update.DocumentOK(); ok {
						state, _ = document.Lookup("$set", "status").StringValueOK()
					}
					switch {
					case update.Upsert:
						operation = "stage_upsert"
					case state == "published":
						operation = "confirm_update"
					case state == "publishing":
						operation = "claim_many"
					case update.Multi:
						operation = "update_many_other"
					default:
						operation = "confirm_update"
					}
				}
			}
			key := m407CommandKey(e.ConnectionID, e.RequestID)
			m.mu.Lock()
			m.started[key] = operation
			m.mu.Unlock()
		},
		Succeeded: func(_ context.Context, e *event.CommandSucceededEvent) {
			m.finish(e.ConnectionID, e.RequestID, e.Duration, false, e.Reply)
		},
		Failed: func(_ context.Context, e *event.CommandFailedEvent) {
			m.finish(e.ConnectionID, e.RequestID, e.Duration, true, nil)
		},
	}
}

func m407CommandKey(connection string, request int64) string {
	return connection + "/" + strconv.FormatInt(request, 10)
}

func (m *m407MongoCommandMetrics) finish(connection string, request int64, duration time.Duration, failed bool, reply bson.Raw) {
	key := m407CommandKey(connection, request)
	m.mu.Lock()
	defer m.mu.Unlock()
	operation, found := m.started[key]
	if !found {
		return
	}
	delete(m.started, key)
	if operation == "findAndModify" && !failed {
		if reply.Lookup("value").Type == bsontype.Null {
			operation = "claim_empty"
		} else {
			operation = "claim_success"
		}
	}
	m.completed = append(m.completed, m407MongoCommandSample{
		Operation: operation, DurationNS: duration.Nanoseconds(), Failed: failed,
	})
}

func (m *m407MongoCommandMetrics) Save(path string) error {
	m.mu.Lock()
	samples := append([]m407MongoCommandSample(nil), m.completed...)
	unfinished := len(m.started)
	m.mu.Unlock()
	if unfinished != 0 {
		return fmt.Errorf("%d Mongo Outbox commands did not finish", unfinished)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(f)
	for _, sample := range samples {
		if err := encoder.Encode(sample); err != nil {
			_ = f.Close()
			return err
		}
	}
	return f.Close()
}

func TestM407MongoMonitorSeparatesBatchClaimAndConfirmation(t *testing.T) {
	for _, tc := range []struct {
		state string
		want  string
	}{
		{state: "publishing", want: "claim_many"},
		{state: "published", want: "confirm_update"},
	} {
		command, err := bson.Marshal(bson.D{
			{Key: "update", Value: "domain_event_outbox"},
			{Key: "updates", Value: bson.A{bson.D{
				{Key: "q", Value: bson.D{}},
				{Key: "u", Value: bson.D{{Key: "$set", Value: bson.D{{Key: "status", Value: tc.state}}}}},
				{Key: "multi", Value: true},
			}}},
		})
		if err != nil {
			t.Fatal(err)
		}
		metrics := newM407MongoCommandMetrics("test", "domain_event_outbox")
		monitor := metrics.Monitor()
		monitor.Started(t.Context(), &event.CommandStartedEvent{
			Command: command, DatabaseName: "test", CommandName: "update", RequestID: 1, ConnectionID: "one",
		})
		monitor.Succeeded(t.Context(), &event.CommandSucceededEvent{CommandFinishedEvent: event.CommandFinishedEvent{
			Duration: time.Millisecond, DatabaseName: "test", CommandName: "update", RequestID: 1, ConnectionID: "one",
		}})
		if len(metrics.completed) != 1 || metrics.completed[0].Operation != tc.want {
			t.Fatalf("state %s classified as %+v; want %s", tc.state, metrics.completed, tc.want)
		}
	}
}
