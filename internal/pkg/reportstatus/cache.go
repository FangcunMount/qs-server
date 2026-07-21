package reportstatus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	redis "github.com/redis/go-redis/v9"
)

var ErrCacheUnavailable = errors.New("report status cache unavailable")

type Cache struct {
	opsHandle *redisruntime.Handle
}

func NewCache(opsHandle *redisruntime.Handle) *Cache {
	return &Cache{opsHandle: opsHandle}
}

func (c *Cache) Get(ctx context.Context, assessmentID string) (*Snapshot, error) {
	client := c.redisClient()
	if client == nil {
		IncStatusGet("unavailable", "")
		return nil, ErrCacheUnavailable
	}
	raw, err := client.Get(ctx, c.keyspace().ReportStatus(assessmentID)).Result()
	if err != nil {
		if err == redis.Nil {
			IncStatusGet("miss", "")
			return nil, nil
		}
		IncStatusGetFailed("")
		return nil, err
	}
	var snapshot Snapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		IncStatusGetFailed("")
		return nil, fmt.Errorf("unmarshal report status: %w", err)
	}
	IncStatusGet("hit", snapshot.Status)
	return &snapshot, nil
}

func (c *Cache) Set(ctx context.Context, snapshot *Snapshot, ttl time.Duration) error {
	if snapshot == nil {
		return nil
	}
	client := c.redisClient()
	if client == nil {
		IncStatusSetFailed("")
		return ErrCacheUnavailable
	}
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	if snapshot.UpdatedAt.IsZero() {
		snapshot.UpdatedAt = time.Now().UTC()
	}
	body, err := json.Marshal(snapshot)
	if err != nil {
		IncStatusSetFailed(snapshot.Status)
		return fmt.Errorf("marshal report status: %w", err)
	}
	if err := client.Set(ctx, c.keyspace().ReportStatus(snapshot.AssessmentID), body, ttl).Err(); err != nil {
		IncStatusSetFailed(snapshot.Status)
		return err
	}
	IncStatusSet(snapshot.Status)
	return nil
}

func (c *Cache) SetIfHigherPriority(ctx context.Context, snapshot *Snapshot, ttl time.Duration) error {
	if snapshot == nil {
		return nil
	}
	current, err := c.Get(ctx, snapshot.AssessmentID)
	if err != nil && !errors.Is(err, ErrCacheUnavailable) {
		return err
	}
	if current != nil && !shouldOverride(current.Status, snapshot.Status) {
		return nil
	}
	return c.Set(ctx, snapshot, ttl)
}

func (c *Cache) SetQueued(ctx context.Context, assessmentID, answerSheetID string, ttl time.Duration) error {
	return c.SetIfHigherPriority(ctx, &Snapshot{
		AssessmentID:  assessmentID,
		AnswerSheetID: answerSheetID,
		Status:        "queued",
		Stage:         "queued",
		Message:       "报告排队生成中",
		UpdatedAt:     time.Now().UTC(),
	}, ttl)
}

func (c *Cache) SetProcessing(ctx context.Context, assessmentID, answerSheetID, stage string, ttl time.Duration) error {
	if stage == "" {
		stage = "processing"
	}
	return c.SetIfHigherPriority(ctx, &Snapshot{
		AssessmentID:  assessmentID,
		AnswerSheetID: answerSheetID,
		Status:        "processing",
		Stage:         stage,
		Message:       "报告生成中",
		UpdatedAt:     time.Now().UTC(),
	}, ttl)
}

func (c *Cache) SetCompleted(ctx context.Context, assessmentID, answerSheetID, reportID string, ttl time.Duration) error {
	return c.SetIfHigherPriority(ctx, &Snapshot{
		AssessmentID:  assessmentID,
		AnswerSheetID: answerSheetID,
		ReportID:      reportID,
		Status:        "completed",
		Stage:         "completed",
		Message:       "报告已生成",
		UpdatedAt:     time.Now().UTC(),
	}, ttl)
}

func (c *Cache) SetFailed(ctx context.Context, assessmentID, answerSheetID, reason, message string, ttl time.Duration) error {
	if message == "" {
		message = "报告生成失败"
	}
	return c.SetIfHigherPriority(ctx, &Snapshot{
		AssessmentID:  assessmentID,
		AnswerSheetID: answerSheetID,
		Status:        "failed",
		Stage:         "failed",
		Reason:        reason,
		Message:       message,
		UpdatedAt:     time.Now().UTC(),
	}, ttl)
}

func (c *Cache) SetTemporarilyUnavailable(ctx context.Context, assessmentID, answerSheetID, reason, message string, ttl time.Duration) error {
	if message == "" {
		message = "报告暂不可用，请稍后重试"
	}
	return c.SetIfHigherPriority(ctx, &Snapshot{
		AssessmentID:  assessmentID,
		AnswerSheetID: answerSheetID,
		Status:        "temporarily_unavailable",
		Stage:         "temporarily_unavailable",
		Reason:        reason,
		Message:       message,
		UpdatedAt:     time.Now().UTC(),
	}, ttl)
}

func (c *Cache) redisClient() redis.UniversalClient {
	if c == nil || c.opsHandle == nil {
		return nil
	}
	return c.opsHandle.Client
}

func (c *Cache) keyspace() keyspace {
	namespace := ""
	if c != nil && c.opsHandle != nil {
		namespace = c.opsHandle.Namespace
	}
	return newKeyspace(namespace)
}

func shouldOverride(current, incoming string) bool {
	priority := map[string]int{
		"submitted":                 1,
		"queued":                    2,
		"processing":                3,
		"scoring":                   4,
		"interpreting":              5,
		"temporarily_unavailable":   90,
		"completed":                 100,
		"failed":                    100,
	}
	cur, okCur := priority[current]
	in, okIn := priority[incoming]
	if !okCur || !okIn {
		return true
	}
	return in >= cur
}
