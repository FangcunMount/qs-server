//go:build reliable_messaging_m4

package standardoutbox

import (
	"context"
	"fmt"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (r *StatusReader) ReadOutboxGovernance(ctx context.Context, orgID int64) (app.OutboxGovernanceSummary, error) {
	var summary app.OutboxGovernanceSummary
	if r == nil || r.collection == nil || orgID <= 0 {
		return summary, fmt.Errorf("standard Mongo governance requires a store and organization")
	}
	scope := fmt.Sprintf("org:%d", orgID)
	authorized := bson.M{"$expr": bson.M{"$eq": bson.A{"$manual_replay_version", "$version"}},
		"manual_replay_request_id": bson.M{"$ne": nil}}
	for _, item := range []struct {
		filter bson.M
		count  *int64
	}{
		{bson.M{"scope": scope, "state": "retry_wait", "$or": automaticReplayClauses()}, &summary.Automatic},
		{bson.M{"scope": scope, "state": "quarantined", "last_error_code": "publish_unknown"}, &summary.ManualRequired},
		{bson.M{"scope": scope, "state": "retry_wait", "manual_replay_request_id": authorized["manual_replay_request_id"], "$expr": authorized["$expr"]}, &summary.Authorized},
		{bson.M{"scope": scope, "state": "quarantined", "last_error_code": bson.M{"$ne": "publish_unknown"}}, &summary.Terminal},
		{bson.M{"scope": scope, "state": "quarantined", "last_error_code": "publish_unknown",
			"event_type": bson.M{"$in": bson.A{"evaluation.retry.requested", "interpretation.retry.requested"}}}, &summary.BlockedRetryEvents},
	} {
		count, err := r.collection.CountDocuments(ctx, item.filter)
		if err != nil {
			return app.OutboxGovernanceSummary{}, err
		}
		*item.count = count
	}
	return summary, nil
}

func (r *StatusReader) ListOutboxCandidates(ctx context.Context, orgID int64, limit int) ([]app.RetryCandidate, error) {
	if r == nil || r.collection == nil || orgID <= 0 || limit < 1 || limit > 10101 {
		return nil, fmt.Errorf("invalid standard Mongo candidate query")
	}
	filter := bson.M{"scope": fmt.Sprintf("org:%d", orgID), "$or": bson.A{
		bson.M{"state": "retry_wait", "$or": automaticReplayClauses()},
		bson.M{"state": "quarantined", "last_error_code": "publish_unknown"},
	}}
	find := options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}, {Key: "_id", Value: 1}}).
		SetLimit(int64(limit)).SetProjection(bson.M{
		"message_id": 1, "failure_count": 1, "state": 1, "next_attempt_at": 1,
		"last_error_code": 1, "updated_at": 1,
	})
	cur, err := r.collection.Find(ctx, filter, find)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var rows []struct {
		EventID       string    `bson:"message_id"`
		FailureCount  uint64    `bson:"failure_count"`
		State         string    `bson:"state"`
		NextAttemptAt time.Time `bson:"next_attempt_at"`
		LastErrorCode string    `bson:"last_error_code"`
		UpdatedAt     time.Time `bson:"updated_at"`
	}
	if err := cur.All(ctx, &rows); err != nil {
		return nil, err
	}
	items := make([]app.RetryCandidate, 0, len(rows))
	for _, row := range rows {
		if row.EventID == "" || row.UpdatedAt.IsZero() || row.FailureCount > uint64(int(^uint(0)>>1)) {
			return nil, fmt.Errorf("standard Mongo candidate has invalid identity, update time or failure count")
		}
		item := app.RetryCandidate{
			Kind: "outbox", Store: "mongo-domain-events", ResourceID: row.EventID,
			Attempt: int(row.FailureCount), LastErrorKind: row.LastErrorCode,
			UpdatedAt: row.UpdatedAt.UTC(),
		}
		switch row.State {
		case "retry_wait":
			if row.NextAttemptAt.IsZero() {
				return nil, fmt.Errorf("standard Mongo automatic candidate has no due time")
			}
			item.Disposition = "automatic"
			next := row.NextAttemptAt.UTC()
			item.NextAttemptAt = &next
		case "quarantined":
			item.Disposition = "manual_required"
		default:
			return nil, fmt.Errorf("invalid standard Mongo candidate state %q", row.State)
		}
		items = append(items, item)
	}
	return items, nil
}

func automaticReplayClauses() bson.A {
	return bson.A{
		bson.M{"manual_replay_request_id": nil},
		bson.M{"manual_replay_version": nil},
		bson.M{"$expr": bson.M{"$ne": bson.A{"$manual_replay_version", "$version"}}},
	}
}

var _ app.OutboxGovernanceReader = (*StatusReader)(nil)
