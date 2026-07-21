package attentionprojection

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// MemoryStore is an in-memory Store for handler and projector tests.
type MemoryStore struct {
	mu      sync.Mutex
	records map[string]*Record
	now     func() time.Time
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		records: make(map[string]*Record),
		now:     time.Now,
	}
}

func (s *MemoryStore) EnsurePending(_ context.Context, input PendingInput) (bool, error) {
	if input.EventID == "" {
		return false, fmt.Errorf("event_id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.records[input.EventID]
	if ok {
		if existing.Status == StatusSucceeded {
			return true, nil
		}
		existing.ReportID = input.ReportID
		existing.AssessmentID = input.AssessmentID
		existing.TesteeID = input.TesteeID
		existing.RiskLevel = input.RiskLevel
		existing.MarkKeyFocus = input.MarkKeyFocus
		existing.UpdatedAt = s.now()
		return false, nil
	}

	now := s.now()
	s.records[input.EventID] = &Record{
		EventID:      input.EventID,
		ReportID:     input.ReportID,
		AssessmentID: input.AssessmentID,
		TesteeID:     input.TesteeID,
		RiskLevel:    input.RiskLevel,
		MarkKeyFocus: input.MarkKeyFocus,
		Status:       StatusPending,
		Attempt:      0,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	return false, nil
}

func (s *MemoryStore) MarkSucceeded(_ context.Context, eventID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.records[eventID]
	if !ok {
		return fmt.Errorf("attention projection not found: %s", eventID)
	}
	rec.Status = StatusSucceeded
	rec.LastError = ""
	rec.UpdatedAt = s.now()
	return nil
}

func (s *MemoryStore) RecordFailure(_ context.Context, eventID string, errMsg string, maxAttempts int) (Status, error) {
	if maxAttempts <= 0 {
		maxAttempts = DefaultMaxAttempts
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.records[eventID]
	if !ok {
		return "", fmt.Errorf("attention projection not found: %s", eventID)
	}
	rec.Attempt++
	rec.LastError = errMsg
	rec.UpdatedAt = s.now()
	if rec.Attempt >= maxAttempts {
		rec.Status = StatusManualRequired
		return StatusManualRequired, nil
	}
	rec.Status = StatusFailed
	return StatusFailed, nil
}

func (s *MemoryStore) GetByEventID(_ context.Context, eventID string) (*Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec, ok := s.records[eventID]
	if !ok {
		return nil, fmt.Errorf("attention projection not found: %s", eventID)
	}
	copy := *rec
	return &copy, nil
}

func (s *MemoryStore) ListRetryable(_ context.Context, maxAttempts int, limit int) ([]Record, error) {
	if maxAttempts <= 0 {
		maxAttempts = DefaultMaxAttempts
	}
	if limit <= 0 {
		limit = 100
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	items := make([]Record, 0)
	for _, rec := range s.records {
		if rec.Status != StatusPending && rec.Status != StatusFailed {
			continue
		}
		if rec.Attempt >= maxAttempts {
			continue
		}
		items = append(items, *rec)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].UpdatedAt.Before(items[j].UpdatedAt)
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

var _ Store = (*MemoryStore)(nil)
