package eventoutbox

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/outboxcore"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (s *Store) ListPendingEventRefs(ctx context.Context, limit int, now time.Time) ([]outboxport.PendingEventRef, error) {
	if s == nil || s.coll == nil || limit <= 0 {
		return nil, nil
	}
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	cursor, err := s.coll.Find(ctx, bson.M{
		"status":          bson.M{"$in": []string{outboxcore.StatusPending, outboxcore.StatusFailed}},
		"next_attempt_at": bson.M{"$lte": now},
	}, options.Find().
		SetProjection(bson.M{"event_id": 1, "event_type": 1, "next_attempt_at": 1}).
		SetSort(bson.D{{Key: "created_at", Value: 1}}).
		SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	rows := make([]OutboxPO, 0)
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}
	refs := make([]outboxport.PendingEventRef, 0, len(rows))
	for _, row := range rows {
		refs = append(refs, outboxport.PendingEventRef{
			EventID:       row.EventID,
			EventType:     row.EventType,
			NextAttemptAt: row.NextAttemptAt,
		})
	}
	return refs, nil
}
