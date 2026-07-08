package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	outcomescoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/scoring"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	redis "github.com/redis/go-redis/v9"
)

const scoringSnapshotTTL = 7 * 24 * time.Hour

// RedisScoringSnapshotStore persists async scoring outcomes in Redis.
type RedisScoringSnapshotStore struct {
	client redis.UniversalClient
}

func NewRedisScoringSnapshotStore(client redis.UniversalClient) outcomescoring.SnapshotStore {
	return &RedisScoringSnapshotStore{client: client}
}

func (s *RedisScoringSnapshotStore) Save(ctx context.Context, assessmentID uint64, outcome *assessment.AssessmentOutcome) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("redis scoring snapshot store is not configured")
	}
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
	return s.client.Set(ctx, scoringSnapshotKey(assessmentID), payload, scoringSnapshotTTL).Err()
}

func (s *RedisScoringSnapshotStore) Load(ctx context.Context, assessmentID uint64) (*assessment.AssessmentOutcome, error) {
	if s == nil || s.client == nil {
		return nil, fmt.Errorf("redis scoring snapshot store is not configured")
	}
	if assessmentID == 0 {
		return nil, fmt.Errorf("assessment id is required")
	}
	payload, err := s.client.Get(ctx, scoringSnapshotKey(assessmentID)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load scoring snapshot: %w", err)
	}
	var outcome assessment.AssessmentOutcome
	if err := json.Unmarshal(payload, &outcome); err != nil {
		return nil, fmt.Errorf("unmarshal scoring snapshot: %w", err)
	}
	return &outcome, nil
}

func (s *RedisScoringSnapshotStore) Delete(ctx context.Context, assessmentID uint64) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("redis scoring snapshot store is not configured")
	}
	if assessmentID == 0 {
		return fmt.Errorf("assessment id is required")
	}
	return s.client.Del(ctx, scoringSnapshotKey(assessmentID)).Err()
}

func scoringSnapshotKey(assessmentID uint64) string {
	return fmt.Sprintf("evaluation:scoring_snapshot:%d", assessmentID)
}
