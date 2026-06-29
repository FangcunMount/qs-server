package eventoutbox

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/outboxcore"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"go.mongodb.org/mongo-driver/bson"
)

func (s *Store) ClaimEventsByIDs(ctx context.Context, eventIDs []string, now time.Time) ([]outboxport.PendingEvent, error) {
	if s == nil || s.coll == nil || len(eventIDs) == 0 {
		return nil, nil
	}
	eventIDs = compactUniqueEventIDs(eventIDs)
	if len(eventIDs) == 0 {
		return nil, nil
	}

	staleBefore := now.Add(-s.publishingStaleFor)
	dueFilter := bson.M{
		"event_id": bson.M{"$in": eventIDs},
		"$or": []bson.M{
			{
				"status":          outboxcore.StatusPending,
				"next_attempt_at": bson.M{"$lte": now},
			},
			{
				"status":          outboxcore.StatusFailed,
				"next_attempt_at": bson.M{"$lte": now},
			},
			{
				"status":     outboxcore.StatusPublishing,
				"updated_at": bson.M{"$lte": staleBefore},
			},
		},
	}
	return s.claimBatchByFilter(ctx, dueFilter, bson.D{{Key: "created_at", Value: 1}}, len(eventIDs), now)
}
