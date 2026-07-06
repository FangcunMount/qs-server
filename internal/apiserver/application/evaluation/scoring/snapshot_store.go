package scoring

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

// ScoringSnapshotStore persists canonical scoring outcomes between async phases.
type ScoringSnapshotStore interface {
	Save(ctx context.Context, assessmentID uint64, outcome *assessment.AssessmentOutcome) error
	Load(ctx context.Context, assessmentID uint64) (*assessment.AssessmentOutcome, error)
	Delete(ctx context.Context, assessmentID uint64) error
}

// MemoryScoringSnapshotStore is an in-process snapshot store for tests.
type MemoryScoringSnapshotStore struct {
	mu    sync.RWMutex
	items map[uint64][]byte
}

func NewMemoryScoringSnapshotStore() *MemoryScoringSnapshotStore {
	return &MemoryScoringSnapshotStore{items: make(map[uint64][]byte)}
}

func (s *MemoryScoringSnapshotStore) Save(_ context.Context, assessmentID uint64, outcome *assessment.AssessmentOutcome) error {
	if assessmentID == 0 {
		return fmt.Errorf("assessment id is required")
	}
	if outcome == nil {
		return fmt.Errorf("assessment outcome is required")
	}
	payload, err := json.Marshal(outcome)
	if err != nil {
		return fmt.Errorf("marshal scoring snapshot: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[assessmentID] = payload
	return nil
}

func (s *MemoryScoringSnapshotStore) Load(_ context.Context, assessmentID uint64) (*assessment.AssessmentOutcome, error) {
	if assessmentID == 0 {
		return nil, fmt.Errorf("assessment id is required")
	}
	s.mu.RLock()
	payload, ok := s.items[assessmentID]
	s.mu.RUnlock()
	if !ok {
		return nil, nil
	}
	var outcome assessment.AssessmentOutcome
	if err := json.Unmarshal(payload, &outcome); err != nil {
		return nil, fmt.Errorf("unmarshal scoring snapshot: %w", err)
	}
	return &outcome, nil
}

func (s *MemoryScoringSnapshotStore) Delete(_ context.Context, assessmentID uint64) error {
	if assessmentID == 0 {
		return fmt.Errorf("assessment id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, assessmentID)
	return nil
}
