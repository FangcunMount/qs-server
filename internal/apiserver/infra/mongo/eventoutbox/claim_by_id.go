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

	staleBefore := now.Add(-s.publishingStaleFor)
	sortDue := bson.D{{Key: "next_attempt_at", Value: 1}, {Key: "created_at", Value: 1}}
	sortStale := bson.D{{Key: "updated_at", Value: 1}, {Key: "created_at", Value: 1}}

	claimed := make([]outboxport.PendingEvent, 0, len(eventIDs))
	for _, eventID := range eventIDs {
		if eventID == "" {
			continue
		}
		item, found, err := s.claimOne(ctx, bson.M{
			"event_id":        eventID,
			"status":          bson.M{"$in": []string{outboxcore.StatusPending, outboxcore.StatusFailed}},
			"next_attempt_at": bson.M{"$lte": now},
		}, sortDue, now)
		if err != nil {
			return nil, err
		}
		if found {
			claimed = append(claimed, item)
			continue
		}
		item, found, err = s.claimOne(ctx, bson.M{
			"event_id":   eventID,
			"status":     outboxcore.StatusPublishing,
			"updated_at": bson.M{"$lte": staleBefore},
		}, sortStale, now)
		if err != nil {
			return nil, err
		}
		if found {
			claimed = append(claimed, item)
		}
	}
	return claimed, nil
}
