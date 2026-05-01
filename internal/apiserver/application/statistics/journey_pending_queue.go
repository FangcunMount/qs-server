package statistics

import (
	"context"
	"encoding/json"
	"time"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

type pendingRetryQueue struct {
	repo BehaviorJourneyRepository
}

func (q pendingRetryQueue) enqueue(ctx context.Context, input BehaviorProjectEventInput, attemptCount int64, reason string) error {
	payload, err := marshalBehaviorProjectEventInput(input)
	if err != nil {
		return err
	}
	return q.repo.UpsertAnalyticsPendingEvent(ctx, input.EventID, input.EventType, payload, time.Now().Add(nextBehaviorPendingBackoff(attemptCount)), reason)
}

func (q pendingRetryQueue) listDue(ctx context.Context, limit int, now time.Time) ([]*domainStatistics.AnalyticsPendingEvent, error) {
	return q.repo.ListDueAnalyticsPendingEvents(ctx, limit, now)
}

func (q pendingRetryQueue) decode(item *domainStatistics.AnalyticsPendingEvent) (BehaviorProjectEventInput, error) {
	var input BehaviorProjectEventInput
	if item == nil {
		return input, nil
	}
	err := json.Unmarshal([]byte(item.PayloadJSON), &input)
	return input, err
}

func (q pendingRetryQueue) reschedule(ctx context.Context, eventID, lastError string, attemptCount int64) error {
	return q.repo.RescheduleAnalyticsPendingEvent(ctx, eventID, lastError, time.Now().Add(nextBehaviorPendingBackoff(attemptCount)))
}

func (q pendingRetryQueue) delete(ctx context.Context, eventID string) error {
	return q.repo.DeleteAnalyticsPendingEvent(ctx, eventID)
}

func marshalBehaviorProjectEventInput(input BehaviorProjectEventInput) (string, error) {
	bytes, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func nextBehaviorPendingBackoff(attemptCount int64) time.Duration {
	if attemptCount <= 1 {
		return defaultBehaviorPendingBackoff
	}
	backoff := defaultBehaviorPendingBackoff
	for i := int64(1); i < attemptCount && backoff < maxBehaviorPendingBackoff; i++ {
		backoff *= 2
		if backoff >= maxBehaviorPendingBackoff {
			return maxBehaviorPendingBackoff
		}
	}
	return backoff
}
