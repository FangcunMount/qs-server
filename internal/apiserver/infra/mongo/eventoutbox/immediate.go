package eventoutbox

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/outboxcore"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func (s *Store) GetPublishableEvent(ctx context.Context, eventID string, now time.Time) (outboxport.PendingEvent, bool, error) {
	if s == nil || s.coll == nil || eventID == "" {
		return outboxport.PendingEvent{}, false, nil
	}
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return outboxport.PendingEvent{}, false, err
	}
	defer release()

	var po OutboxPO
	err = s.coll.FindOne(ctx, bson.M{
		"event_id":        eventID,
		"status":          bson.M{"$in": []string{outboxcore.StatusPending, outboxcore.StatusFailed}},
		"next_attempt_at": bson.M{"$lte": now},
	}).Decode(&po)
	if err == mongo.ErrNoDocuments {
		return outboxport.PendingEvent{}, false, nil
	}
	if err != nil {
		return outboxport.PendingEvent{}, false, err
	}
	pending, err := outboxcore.DecodePendingEvent(po.EventID, po.PayloadJSON)
	if err != nil {
		return outboxport.PendingEvent{}, false, err
	}
	return pending, true, nil
}

func (s *Store) MarkEventsPublished(ctx context.Context, eventIDs []string, publishedAt time.Time) error {
	if s == nil || s.coll == nil || len(eventIDs) == 0 {
		return nil
	}
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()

	transition := outboxcore.NewPublishedTransition(publishedAt)
	_, err = s.coll.UpdateMany(ctx, bson.M{"event_id": bson.M{"$in": eventIDs}}, bson.M{
		"$set": bson.M{
			"status":       transition.Status,
			"published_at": transition.PublishedAt,
			"updated_at":   transition.UpdatedAt,
		},
		"$unset": bson.M{"claim_token": ""},
	})
	return err
}

func (s *Store) MarkEventsFailed(ctx context.Context, failures []outboxport.FailedMark, nextAttemptAt time.Time) error {
	if s == nil || s.coll == nil || len(failures) == 0 {
		return nil
	}
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()

	now := time.Now()
	models := make([]mongo.WriteModel, 0, len(failures))
	for _, failure := range failures {
		if failure.EventID == "" {
			continue
		}
		transition := outboxcore.NewFailedTransition(failure.LastError, nextAttemptAt, now)
		models = append(models, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"event_id": failure.EventID}).
			SetUpdate(bson.M{
				"$set": bson.M{
					"status":          transition.Status,
					"last_error":      transition.LastError,
					"next_attempt_at": transition.NextAttemptAt,
					"updated_at":      transition.UpdatedAt,
				},
				"$inc":   bson.M{"attempt_count": transition.AttemptIncrement},
				"$unset": bson.M{"claim_token": ""},
			}))
	}
	if len(models) == 0 {
		return nil
	}
	_, err = s.coll.BulkWrite(ctx, models)
	return err
}
