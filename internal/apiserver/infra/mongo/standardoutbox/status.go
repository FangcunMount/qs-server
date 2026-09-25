//go:build reliable_messaging_m4

package standardoutbox

import (
	"context"
	"fmt"
	"time"

	appstandard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// StatusReader borrows the standard collection and never queries the old
// eventoutbox collection or exposes message payloads.
type StatusReader struct{ collection *mongo.Collection }

var _ outboxport.EventTypeStatusReader = (*StatusReader)(nil)

func NewStatusReader(collection *mongo.Collection) (*StatusReader, error) {
	if collection == nil {
		return nil, fmt.Errorf("standard Mongo status requires host collection")
	}
	return &StatusReader{collection: collection}, nil
}

func (r *StatusReader) OutboxStatusSnapshot(ctx context.Context, now time.Time) (outboxport.StatusSnapshot, error) {
	if r == nil || r.collection == nil {
		return outboxport.StatusSnapshot{}, fmt.Errorf("standard Mongo status reader is not configured")
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.D{{Key: "state", Value: bson.D{{Key: "$ne", Value: "published"}}}}}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$state"},
			{Key: "n", Value: bson.D{{Key: "$sum", Value: 1}}},
			{Key: "oldest_created_at", Value: bson.D{{Key: "$min", Value: "$created_at"}}},
			{Key: "invalid_created_at", Value: bson.D{{Key: "$sum", Value: bson.D{{Key: "$cond", Value: bson.A{
				bson.D{{Key: "$eq", Value: bson.A{bson.D{{Key: "$type", Value: "$created_at"}}, "date"}}}, 0, 1,
			}}}}}},
		}}},
	}
	cursor, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return outboxport.StatusSnapshot{}, err
	}
	defer cursor.Close(ctx)
	counts := make([]appstandard.StatusCount, 0, 4)
	for cursor.Next(ctx) {
		var row struct {
			State            string     `bson:"_id"`
			Count            int64      `bson:"n"`
			OldestCreatedAt  *time.Time `bson:"oldest_created_at"`
			InvalidCreatedAt int64      `bson:"invalid_created_at"`
		}
		if err := cursor.Decode(&row); err != nil {
			return outboxport.StatusSnapshot{}, err
		}
		if row.InvalidCreatedAt != 0 {
			return outboxport.StatusSnapshot{}, fmt.Errorf("standard Mongo state %q has %d rows without creation time", row.State, row.InvalidCreatedAt)
		}
		counts = append(counts, appstandard.StatusCount{State: row.State, Count: row.Count, OldestCreatedAt: row.OldestCreatedAt})
	}
	if err := cursor.Err(); err != nil {
		return outboxport.StatusSnapshot{}, err
	}
	return appstandard.BuildStatusSnapshot("mongo-domain-events", now, counts)
}

// OutboxStatusByEventType reports only unfinished standard intents. The
// aggregation reads identity, state and age fields, never message payloads.
func (r *StatusReader) OutboxStatusByEventType(ctx context.Context, _ time.Time) ([]outboxport.EventTypeStatusBucket, error) {
	if r == nil || r.collection == nil {
		return nil, fmt.Errorf("standard Mongo status reader is not configured")
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.D{{Key: "state", Value: bson.D{{Key: "$ne", Value: "published"}}}}}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: bson.D{{Key: "event_type", Value: "$event_type"}, {Key: "state", Value: "$state"}}},
			{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
			{Key: "oldest", Value: bson.D{{Key: "$min", Value: "$created_at"}}},
			{Key: "invalid_created_at", Value: bson.D{{Key: "$sum", Value: bson.D{{Key: "$cond", Value: bson.A{
				bson.D{{Key: "$eq", Value: bson.A{bson.D{{Key: "$type", Value: "$created_at"}}, "date"}}}, 0, 1,
			}}}}}},
		}}},
	}
	cursor, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	buckets := make([]outboxport.EventTypeStatusBucket, 0)
	for cursor.Next(ctx) {
		var row struct {
			ID struct {
				EventType string `bson:"event_type"`
				State     string `bson:"state"`
			} `bson:"_id"`
			Count            int64      `bson:"count"`
			Oldest           *time.Time `bson:"oldest"`
			InvalidCreatedAt int64      `bson:"invalid_created_at"`
		}
		if err := cursor.Decode(&row); err != nil {
			return nil, err
		}
		if row.ID.EventType == "" || !appstandard.IsUnfinishedState(row.ID.State) || row.Count <= 0 || row.InvalidCreatedAt != 0 || row.Oldest == nil || row.Oldest.IsZero() {
			return nil, fmt.Errorf("invalid standard Mongo event type status row %q/%q", row.ID.EventType, row.ID.State)
		}
		oldest := *row.Oldest
		buckets = append(buckets, outboxport.EventTypeStatusBucket{
			EventType: row.ID.EventType, Status: row.ID.State, Count: row.Count, OldestCreatedAt: &oldest,
		})
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return buckets, nil
}
