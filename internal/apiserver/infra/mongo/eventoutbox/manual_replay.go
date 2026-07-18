package eventoutbox

import (
	"context"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/outboxcore"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"go.mongodb.org/mongo-driver/bson"
)

func (s *Store) AuthorizeManualReplay(ctx context.Context, orgID int64, requestID string, targets []outboxport.ManualReplayTarget, authorizedAt time.Time) ([]outboxport.ManualReplayResult, error) {
	results := make([]outboxport.ManualReplayResult, 0, len(targets))
	for _, target := range targets {
		result := outboxport.ManualReplayResult{EventID: target.EventID}
		filter := bson.M{
			"event_id": target.EventID, "org_id": orgID, "status": outboxcore.StatusFailed,
			"retry_disposition": string(retrygovernance.DispositionManualRequired), "attempt_count": target.ExpectedAttemptCount,
		}
		update := bson.M{"$set": bson.M{
			"retry_disposition": string(retrygovernance.DispositionAutomatic), "next_attempt_at": authorizedAt,
			"manual_replay_request_id": requestID, "updated_at": authorizedAt,
		}}
		updated, err := s.coll.UpdateOne(ctx, filter, update)
		if err != nil {
			return nil, err
		}
		if updated.ModifiedCount == 1 {
			result.Authorized = true
		} else {
			result.Reason = "not_found_or_conflict"
		}
		results = append(results, result)
	}
	return results, nil
}

var _ outboxport.ManualReplayAuthorizer = (*Store)(nil)
