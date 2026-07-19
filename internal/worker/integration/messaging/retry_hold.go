package messaging

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	drivermysql "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"

	"github.com/FangcunMount/component-base/pkg/eventcodec"
	basemessaging "github.com/FangcunMount/component-base/pkg/messaging"
	eventobservability "github.com/FangcunMount/qs-server/internal/pkg/eventing/observe"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

type RetryEventHoldRecorder interface {
	Hold(context.Context, *basemessaging.Message, string, error) error
}

type mysqlRetryEventHoldStore struct {
	db       *sql.DB
	provider string
	policy   retrygovernance.Policy
}

func NewMySQLRetryEventHoldStore(options *genericoptions.MySQLOptions, provider string, policies ...retrygovernance.Policy) (*mysqlRetryEventHoldStore, error) {
	if options == nil || options.Host == "" || options.Database == "" || provider == "" {
		return nil, fmt.Errorf("retry event hold store is not configured")
	}
	cfg := drivermysql.NewConfig()
	cfg.Net = "tcp"
	cfg.Addr = options.Host
	cfg.User = options.Username
	cfg.Passwd = options.Password
	cfg.DBName = options.Database
	cfg.ParseTime = true
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("open retry event hold store: %w", err)
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(options.MaxConnectionLifeTime)
	policy := retrygovernance.DefaultOutboxPolicy
	if len(policies) > 0 {
		policy = policies[0]
	}
	if err := policy.Validate(); err != nil || policy.MaxAutomaticAttempts > retrygovernance.HardMaxOutboxAttempts {
		_ = db.Close()
		return nil, fmt.Errorf("invalid retry hold policy")
	}
	return &mysqlRetryEventHoldStore{db: db, provider: provider, policy: policy}, nil
}

func (s *mysqlRetryEventHoldStore) Hold(ctx context.Context, message *basemessaging.Message, eventType string, cause error) error {
	if s == nil || s.db == nil || message == nil || message.UUID == "" || message.Topic == "" || message.Channel == "" {
		return fmt.Errorf("invalid retry event hold")
	}
	eventID, orgID := retryEventIdentity(message.Payload)
	if eventID == "" {
		eventID = message.UUID
	}
	reason := "automatic retry paused"
	if cause != nil {
		reason = cause.Error()
	}
	if len(reason) > 255 {
		reason = reason[:255]
	}
	now := time.Now()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO retry_event_hold
  (event_id, message_id, org_id, provider, topic_name, channel_name, payload_json,
   original_delivery_attempt, blocked_reason, blocked_at, status, retry_disposition,
   replay_attempt_count, next_attempt_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'blocked', 'automatic', 0, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  event_id=VALUES(event_id), org_id=VALUES(org_id), payload_json=VALUES(payload_json),
  original_delivery_attempt=LEAST(original_delivery_attempt, VALUES(original_delivery_attempt)),
  blocked_reason=VALUES(blocked_reason), blocked_at=VALUES(blocked_at), status='blocked',
  retry_disposition='automatic', next_attempt_at=VALUES(next_attempt_at), claim_token=NULL,
  claim_expires_at=NULL, last_error=NULL, replayed_at=NULL, updated_at=VALUES(updated_at)`,
		eventID, message.UUID, orgID, s.provider, message.Topic, message.Channel, string(message.Payload),
		max(int(message.Attempts), 1), reason, now, now, now, now,
	)
	_ = eventType // event type remains inside the canonical payload/metadata.
	return err
}

type heldEvent struct {
	ID                 uint64
	EventID            string
	MessageID          string
	Topic              string
	Channel            string
	Payload            []byte
	ReplayAttemptCount int
	ClaimToken         string
}

type retryEventHoldStore interface {
	claim(context.Context, time.Time, time.Duration) (*heldEvent, error)
	markReplayed(context.Context, *heldEvent, time.Time) error
	markReplayFailed(context.Context, *heldEvent, error, time.Time) error
}

func (s *mysqlRetryEventHoldStore) claim(ctx context.Context, now time.Time, lease time.Duration) (*heldEvent, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var item heldEvent
	var payload string
	err = tx.QueryRowContext(ctx, `
SELECT id, event_id, message_id, topic_name, channel_name, payload_json, replay_attempt_count
FROM retry_event_hold
WHERE (status IN ('blocked','failed') OR (status='replaying' AND claim_expires_at<=?))
  AND retry_disposition='automatic'
  AND (next_attempt_at IS NULL OR next_attempt_at<=?)
  AND (claim_expires_at IS NULL OR claim_expires_at<=?)
ORDER BY COALESCE(next_attempt_at, blocked_at), id
LIMIT 1 FOR UPDATE SKIP LOCKED`, now, now, now).Scan(&item.ID, &item.EventID, &item.MessageID, &item.Topic, &item.Channel, &payload, &item.ReplayAttemptCount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	item.ClaimToken = uuid.NewString()
	result, err := tx.ExecContext(ctx, `UPDATE retry_event_hold
SET status='replaying', claim_token=?, claim_expires_at=?, updated_at=?
WHERE id=? AND (status IN ('blocked','failed') OR (status='replaying' AND claim_expires_at<=?))`, item.ClaimToken, now.Add(lease), now, item.ID, now)
	if err != nil {
		return nil, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return nil, nil
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	item.Payload = []byte(payload)
	return &item, nil
}

func (s *mysqlRetryEventHoldStore) markReplayed(ctx context.Context, item *heldEvent, now time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE retry_event_hold
SET status='replayed', retry_disposition=NULL, next_attempt_at=NULL, claim_token=NULL,
    claim_expires_at=NULL, last_error=NULL, replay_attempt_count=replay_attempt_count+1,
    replayed_at=?, updated_at=?
WHERE id=? AND status='replaying' AND claim_token=?`, now, now, item.ID, item.ClaimToken)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return fmt.Errorf("retry hold replay claim lost")
	}
	return nil
}

func (s *mysqlRetryEventHoldStore) markReplayFailed(ctx context.Context, item *heldEvent, cause error, now time.Time) error {
	policy := s.policy
	if policy.MaxAutomaticAttempts == 0 {
		policy = retrygovernance.DefaultOutboxPolicy
	}
	attempt := item.ReplayAttemptCount + 1
	decision := policy.DecideFailureForKey(true, attempt, now, item.EventID)
	status := "failed"
	disposition := string(decision.Disposition)
	if decision.Disposition == retrygovernance.DispositionAutomatic {
		status = "failed"
	}
	result, err := s.db.ExecContext(ctx, `UPDATE retry_event_hold
SET status=?, retry_disposition=?, next_attempt_at=?, claim_token=NULL, claim_expires_at=NULL,
    replay_attempt_count=?, last_error=?, updated_at=?
WHERE id=? AND status='replaying' AND claim_token=?`, status, disposition, decision.NextAttemptAt, attempt, cause.Error(), now, item.ID, item.ClaimToken)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return fmt.Errorf("retry hold replay claim lost")
	}
	return nil
}

func retryEventIdentity(payload []byte) (string, any) {
	var envelope struct {
		ID   string `json:"id"`
		Data struct {
			OrgID int64 `json:"org_id"`
		} `json:"data"`
	}
	if json.Unmarshal(payload, &envelope) != nil {
		return "", nil
	}
	if envelope.Data.OrgID == 0 {
		return envelope.ID, nil
	}
	return envelope.ID, envelope.Data.OrgID
}

type RetryEventHoldReplayer struct {
	store     retryEventHoldStore
	publisher basemessaging.Publisher
	interval  time.Duration
	lease     time.Duration
	cancel    context.CancelFunc
	done      chan struct{}
	stopOnce  sync.Once
	observer  eventobservability.Observer
}

func NewRetryEventHoldReplayer(store retryEventHoldStore, publisher basemessaging.Publisher) *RetryEventHoldReplayer {
	return &RetryEventHoldReplayer{store: store, publisher: publisher, interval: 5 * time.Second, lease: time.Minute, done: make(chan struct{}), observer: eventobservability.DefaultObserver()}
}

func (r *RetryEventHoldReplayer) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	go func() {
		defer close(r.done)
		_ = r.RunOnce(ctx, time.Now())
		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				_ = r.RunOnce(ctx, now)
			}
		}
	}()
}

func (r *RetryEventHoldReplayer) RunOnce(ctx context.Context, now time.Time) error {
	if r == nil || r.store == nil || r.publisher == nil {
		return fmt.Errorf("retry hold replayer is not configured")
	}
	for {
		item, err := r.store.claim(ctx, now, r.lease)
		if err != nil || item == nil {
			return err
		}
		message := basemessaging.NewMessage(item.MessageID, item.Payload)
		if err := r.publisher.PublishMessage(ctx, item.Topic, message); err != nil {
			r.observe(ctx, item, eventobservability.ConsumeOutcomeHoldReplayFailed)
			if markErr := r.store.markReplayFailed(ctx, item, err, now); markErr != nil {
				return errors.Join(err, markErr)
			}
			continue
		}
		if err := r.store.markReplayed(ctx, item, now); err != nil {
			return err
		}
		r.observe(ctx, item, eventobservability.ConsumeOutcomeHoldReplayed)
	}
}

func (r *RetryEventHoldReplayer) observe(ctx context.Context, item *heldEvent, outcome eventobservability.ConsumeOutcome) {
	if r == nil || r.observer == nil || item == nil {
		return
	}
	eventType := ""
	if envelope, err := eventcodec.DecodeEnvelope(item.Payload); err == nil {
		eventType = envelope.EventType
	}
	r.observer.ObserveConsume(ctx, eventobservability.ConsumeEvent{Service: "retry-hold-replayer", Topic: item.Topic, EventType: eventType, Outcome: outcome})
}

func (r *RetryEventHoldReplayer) Stop() {
	if r == nil {
		return
	}
	r.stopOnce.Do(func() {
		if r.cancel != nil {
			r.cancel()
			<-r.done
		}
	})
}
