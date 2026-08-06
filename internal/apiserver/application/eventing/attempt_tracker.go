package eventing

import (
	"sync"
	"time"
)

const defaultOutboxAttemptTrackingTTL = 10 * time.Minute

// OutboxAttemptTracker correlates the best-effort immediate publish path with
// the durable relay so observability can classify the first relay delivery as
// a retry when an immediate attempt already happened. It does not influence
// delivery, persistence, or retry scheduling.
type OutboxAttemptTracker struct {
	mu        sync.Mutex
	attempted map[string]time.Time
	ttl       time.Duration
}

func NewOutboxAttemptTracker() *OutboxAttemptTracker {
	return &OutboxAttemptTracker{
		attempted: make(map[string]time.Time),
		ttl:       defaultOutboxAttemptTrackingTTL,
	}
}

func (t *OutboxAttemptTracker) Mark(eventID string) {
	if t == nil || eventID == "" {
		return
	}
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pruneLocked(now)
	t.attempted[eventID] = now
}

func (t *OutboxAttemptTracker) Consume(eventID string) bool {
	if t == nil || eventID == "" {
		return false
	}
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pruneLocked(now)
	_, found := t.attempted[eventID]
	delete(t.attempted, eventID)
	return found
}

func (t *OutboxAttemptTracker) Forget(eventID string) {
	if t == nil || eventID == "" {
		return
	}
	t.mu.Lock()
	delete(t.attempted, eventID)
	t.mu.Unlock()
}

func (t *OutboxAttemptTracker) pruneLocked(now time.Time) {
	for eventID, attemptedAt := range t.attempted {
		if now.Sub(attemptedAt) > t.ttl {
			delete(t.attempted, eventID)
		}
	}
}
