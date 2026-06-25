package eventoutbox

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/outboxcore"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type eventTypeStatusRow struct {
	ID struct {
		EventType string `bson:"event_type"`
		Status    string `bson:"status"`
	} `bson:"_id"`
	Count  int64     `bson:"count"`
	Oldest time.Time `bson:"oldest"`
}

func (s *Store) OutboxStatusByEventType(ctx context.Context, now time.Time) ([]outboxport.EventTypeStatusBucket, error) {
	if s == nil || s.coll == nil {
		return nil, nil
	}
	_ = now
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"status": bson.M{"$in": outboxcore.UnfinishedStatuses()}}}},
		{{Key: "$group", Value: bson.M{
			"_id":    bson.M{"event_type": "$event_type", "status": "$status"},
			"count":  bson.M{"$sum": 1},
			"oldest": bson.M{"$min": "$created_at"},
		}}},
	}
	cursor, err := s.coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	rows := make([]eventTypeStatusRow, 0)
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}
	buckets := make([]outboxport.EventTypeStatusBucket, 0, len(rows))
	for _, row := range rows {
		oldest := row.Oldest
		buckets = append(buckets, outboxport.EventTypeStatusBucket{
			EventType:       row.ID.EventType,
			Status:          row.ID.Status,
			Count:           row.Count,
			OldestCreatedAt: &oldest,
		})
	}
	return buckets, nil
}
