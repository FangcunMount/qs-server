package eventoutbox

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/outboxcore"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
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
		"event_id":          eventID,
		"status":            bson.M{"$in": []string{outboxcore.StatusPending, outboxcore.StatusFailed}},
		"next_attempt_at":   bson.M{"$lte": now},
		"retry_disposition": bson.M{"$ne": retrygovernance.DispositionManualRequired},
	}).Decode(&po)
	if err == mongo.ErrNoDocuments {
		return outboxport.PendingEvent{}, false, nil
	}
	if err != nil {
		return outboxport.PendingEvent{}, false, err
	}
	pending, err := outboxcore.DecodePendingEvent(po.EventID, po.PayloadJSON)
	if err != nil {
		_ = s.markPermanentFailure(ctx, po.EventID, "decode outbox payload: "+err.Error(), "encoding", now)
		return outboxport.PendingEvent{}, false, err
	}
	return pending, true, nil
}

func (s *Store) markPermanentFailure(ctx context.Context, eventID, lastError, errorKind string, failedAt time.Time) error {
	result, err := s.coll.UpdateOne(ctx, bson.M{"event_id": eventID}, bson.M{
		"$set": bson.M{"status": outboxcore.StatusFailed, "last_error": lastError, "last_error_kind": errorKind,
			"retry_disposition": retrygovernance.DispositionManualRequired, "next_attempt_at": failedAt, "updated_at": failedAt},
		"$inc": bson.M{"attempt_count": 1}, "$unset": bson.M{"claim_token": ""},
	})
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return fmt.Errorf("outbox event %q not found", eventID)
	}
	return nil
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
		"$unset": bson.M{"claim_token": "", "retry_disposition": ""},
	})
	return err
}

func (s *Store) MarkEventsFailedGoverned(ctx context.Context, failures []outboxport.FailedMark, failedAt time.Time) ([]outboxport.GovernedFailedMark, error) {
	if s == nil || s.coll == nil || len(failures) == 0 {
		return nil, nil
	}
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	results := make([]outboxport.GovernedFailedMark, 0, len(failures))
	for _, failure := range failures {
		marked, err := s.markEventFailedGoverned(ctx, failure, failedAt)
		if err != nil {
			return nil, err
		}
		results = append(results, marked)
	}
	return results, nil
}

func (s *Store) markEventFailedGoverned(ctx context.Context, failure outboxport.FailedMark, failedAt time.Time) (outboxport.GovernedFailedMark, error) {
	for conflict := 0; conflict < 3; conflict++ {
		var row OutboxPO
		if err := s.coll.FindOne(ctx, bson.M{"event_id": failure.EventID}).Decode(&row); err != nil {
			return outboxport.GovernedFailedMark{}, err
		}
		nextCount := row.AttemptCount + 1
		decision := retrygovernance.OutboxPolicy().DecideFailureForKey(true, nextCount, failedAt, failure.EventID)
		nextAt := failedAt
		if decision.NextAttemptAt != nil {
			nextAt = *decision.NextAttemptAt
		}
		update, err := s.coll.UpdateOne(ctx, bson.M{"event_id": failure.EventID, "attempt_count": row.AttemptCount}, bson.M{
			"$set": bson.M{
				"status": outboxcore.StatusFailed, "last_error": failure.LastError,
				"last_error_kind": "publish", "attempt_count": nextCount,
				"retry_disposition": decision.Disposition, "next_attempt_at": nextAt,
				"updated_at": failedAt,
			},
			"$unset": bson.M{"claim_token": ""},
		})
		if err != nil {
			return outboxport.GovernedFailedMark{}, err
		}
		if update.ModifiedCount == 1 {
			return outboxport.GovernedFailedMark{EventID: failure.EventID, EventType: failure.EventType, AttemptCount: nextCount, Disposition: decision.Disposition, NextAttemptAt: decision.NextAttemptAt}, nil
		}
	}
	return outboxport.GovernedFailedMark{}, fmt.Errorf("outbox event %q failure transition conflicted", failure.EventID)
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
