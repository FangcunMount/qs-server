package eventoutbox

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/outboxcore"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (s *Store) claimBatchByFilter(
	ctx context.Context,
	filter bson.M,
	sort bson.D,
	limit int,
	now time.Time,
) ([]outboxport.PendingEvent, error) {
	if limit <= 0 {
		return nil, nil
	}

	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()

	cursor, err := s.coll.Find(ctx, filter, options.Find().
		SetProjection(bson.M{"event_id": 1}).
		SetSort(sort).
		SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	var rows []OutboxPO
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}
	ids := eventIDsFromRows(rows)
	if len(ids) == 0 {
		return nil, nil
	}

	claimToken := uuid.NewString()
	claimFilter := andFilter(filter, bson.M{"event_id": bson.M{"$in": ids}})
	res, err := s.coll.UpdateMany(ctx, claimFilter, bson.M{
		"$set": bson.M{
			"status":      outboxcore.StatusPublishing,
			"updated_at":  now,
			"claim_token": claimToken,
		},
	})
	if err != nil {
		return nil, err
	}
	if res.ModifiedCount == 0 {
		return nil, nil
	}

	cursor, err = s.coll.Find(ctx, bson.M{
		"claim_token": claimToken,
		"status":      outboxcore.StatusPublishing,
	}, options.Find().SetSort(sort))
	if err != nil {
		return nil, err
	}
	var claimedRows []OutboxPO
	if err := cursor.All(ctx, &claimedRows); err != nil {
		return nil, err
	}
	return s.pendingFromDocuments(ctx, claimedRows)
}

func (s *Store) claimPendingBatch(ctx context.Context, limit int, now time.Time) ([]outboxport.PendingEvent, error) {
	if limit <= 0 {
		return nil, nil
	}

	claimed := make([]outboxport.PendingEvent, 0, limit)
	for _, query := range pendingClaimQueries(now, s.priorityTiers) {
		if len(claimed) >= limit {
			break
		}
		batch, err := s.claimBatchByFilter(
			ctx,
			query.filter,
			query.sort,
			limit-len(claimed),
			now,
		)
		if err != nil {
			return nil, err
		}
		claimed = append(claimed, batch...)
	}
	return claimed, nil
}

func (s *Store) pendingFromDocuments(ctx context.Context, docs []OutboxPO) ([]outboxport.PendingEvent, error) {
	claimed := make([]outboxport.PendingEvent, 0, len(docs))
	for _, po := range docs {
		pending, err := outboxcore.DecodePendingEvent(po.EventID, po.PayloadJSON)
		if err != nil {
			transition := outboxcore.NewDecodeFailureTransition(err, time.Now())
			_ = s.markPermanentFailure(ctx, po.EventID, transition.LastError, "encoding", time.Now())
			continue
		}
		claimed = append(claimed, pending)
	}
	return claimed, nil
}

func andFilter(filter bson.M, extra bson.M) bson.M {
	return bson.M{
		"$and": []bson.M{filter, extra},
	}
}

func compactUniqueEventIDs(eventIDs []string) []string {
	if len(eventIDs) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(eventIDs))
	unique := make([]string, 0, len(eventIDs))
	for _, eventID := range eventIDs {
		if eventID == "" {
			continue
		}
		if _, ok := seen[eventID]; ok {
			continue
		}
		seen[eventID] = struct{}{}
		unique = append(unique, eventID)
	}
	return unique
}

func eventIDsFromRows(rows []OutboxPO) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.EventID != "" {
			ids = append(ids, row.EventID)
		}
	}
	return ids
}
