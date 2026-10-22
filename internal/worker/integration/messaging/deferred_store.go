package messaging

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventtransport "github.com/FangcunMount/qs-server/internal/pkg/eventing/deliveryevidence"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	rmtransport "github.com/FangcunMount/reliable-messaging/transport"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	"github.com/google/uuid"
)

var ErrDeferredClaimLost = errors.New("deferred consumer claim no longer owned")

const deferredDBBudget = 2 * time.Second

// DeferredDeliveryStore borrows the existing worker hold pool. Version2 is a
// consumption responsibility, never an Outbox or permission to publish/retry
// business work. Constructors perform no I/O or DDL; the host installs109.
type DeferredDeliveryStore struct {
	original *mysqlRetryEventHoldStore
	channel  string
	routes   map[string]rmtransport.RouteBinding
	policy   retrygovernance.Policy
}

func NewRocketMQDeferredStore(original *mysqlRetryEventHoldStore, channel string, attempts int, routes []rmtransport.RouteBinding) (*DeferredDeliveryStore, error) {
	if original == nil || original.db == nil || original.provider != "rocketmq" || channel == "" || attempts < 1 || attempts > retrygovernance.HardMaxDeliveryAttempts {
		return nil, errors.New("original RocketMQ hold pool, subscription and bounded attempts required")
	}
	if err := rmtransport.ValidateRoutes(routes); err != nil {
		return nil, err
	}
	s := &DeferredDeliveryStore{original: original, channel: channel, routes: map[string]rmtransport.RouteBinding{}, policy: original.policy}
	if s.policy.MaxAutomaticAttempts == 0 {
		s.policy = retrygovernance.DefaultOutboxPolicy
	}
	s.policy.Version = "deferred-consume/v2"
	s.policy.MaxAutomaticAttempts = min(s.policy.MaxAutomaticAttempts, attempts)
	if err := s.policy.Validate(); err != nil {
		return nil, err
	}
	owners := map[string]bool{}
	for _, r := range routes {
		if r.Topic != "qs.evaluation.lifecycle" || r.Subscription != channel {
			continue
		}
		if r.MessageType != rmtransport.TransactionMessageType || r.RouteRevision != "m3-1" || (r.Owner != "qs.mysql" && r.Owner != "qs.mongo") || r.PhysicalTopic == "" {
			return nil, errors.New("deferred consumption requires reviewed v2 transaction inputs")
		}
		if _, found := s.routes[r.PhysicalTopic]; found {
			return nil, errors.New("duplicate deferred physical input")
		}
		s.routes[r.PhysicalTopic] = r
		owners[r.Owner] = true
	}
	if !owners["qs.mysql"] || !owners["qs.mongo"] {
		return nil, errors.New("both original evaluation/report inputs required")
	}
	return s, nil
}

func (s *DeferredDeliveryStore) Validate(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, deferredDBBudget)
	defer cancel()
	for _, query := range []string{
		"SELECT protocol_version,transport_identity_hash,received_timestamp,consumed_at,used_manual_replay_request_id FROM retry_event_hold LIMIT 0",
		"SELECT identity_hash,original_wire,metadata_bytes FROM event_delivery_transport_evidence LIMIT 0",
	} {
		rows, err := s.original.db.QueryContext(ctx, query)
		if err != nil {
			return err
		}
		err = errors.Join(rows.Err(), rows.Close())
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *DeferredDeliveryStore) eventType(m rmtransport.Received) (string, error) {
	r, found := s.routes[m.PhysicalTopic]
	if !found || m.Provider != "rocketmq" || m.Channel != s.channel || m.Topic != r.Topic || m.ID == "" || len(m.ID) > 128 || len(m.Wire) == 0 || len(m.Wire) > 262144 || len(m.Payload) > 262144 || m.Attempts < 1 || m.Attempts > retrygovernance.HardMaxDeliveryAttempts {
		return "", errors.New("deferred source is outside the configured subscription")
	}
	env, err := domainwire.DecodeEnvelope(m.Payload)
	if err != nil || env.ID != m.ID || env.ID == "" || len(env.ID) > 64 {
		return "", errors.New("deferred domain identity required")
	}
	expectedOwner := ""
	switch env.EventType {
	case eventcatalog.EvaluationRequested, eventcatalog.EvaluationRetryRequested:
		expectedOwner = "qs.mysql"
	case eventcatalog.InterpretationRetryRequested:
		expectedOwner = "qs.mongo"
	default:
		return "", errors.New("event has no deferred consumption contract")
	}
	if r.Owner != expectedOwner || (m.Metadata["event_type"] != "" && m.Metadata["event_type"] != env.EventType) {
		return "", errors.New("deferred event owner or metadata conflict")
	}
	var data struct {
		AttemptOrigin string `json:"attempt_origin"`
	}
	if json.Unmarshal(env.Data, &data) != nil || data.AttemptOrigin != "automatic" {
		return "", errors.New("only already-authorized paused automatic deliveries may be held")
	}
	return env.EventType, nil
}

func (s *DeferredDeliveryStore) HoldDelivery(ctx context.Context, m rmtransport.Received, eventType string, cause error) error {
	if !errors.Is(cause, eventruntime.ErrAutomaticRetryPaused) {
		return errors.New("deferred hold requires an explicit consumer pause")
	}
	kind, err := s.eventType(m)
	if err != nil || kind != eventType {
		return errors.New("deferred event binding conflict")
	}
	identity, err := eventtransport.RocketMQDeliveryIdentity(m)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, deferredDBBudget)
	defer cancel()
	tx, err := s.original.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, org := retryEventIdentity(m.Payload)
	now := time.Now().UTC()
	// Acquire the hold row before evidence locks, matching the claim order.
	_, err = tx.ExecContext(ctx, `INSERT INTO retry_event_hold
(event_id,message_id,org_id,provider,topic_name,channel_name,payload_json,original_delivery_attempt,blocked_reason,blocked_at,status,retry_disposition,replay_attempt_count,next_attempt_at,created_at,updated_at,protocol_version,transport_identity_hash,received_timestamp)
VALUES(?,?,?,?,?,?,?,?,?,?,'blocked','automatic',0,?,?,?,2,?,?) ON DUPLICATE KEY UPDATE id=LAST_INSERT_ID(id)`,
		m.ID, m.ID, org, m.Provider, m.Topic, m.Channel, string(m.Payload), m.Attempts, "automatic_retry_paused", now, now, now, now, identity[:], m.Timestamp)
	if err != nil {
		return err
	}
	var version int
	var eventID, messageID, payload string
	var first []byte
	err = tx.QueryRowContext(ctx, `SELECT protocol_version,event_id,message_id,payload_json,transport_identity_hash FROM retry_event_hold WHERE provider=? AND topic_name=? AND channel_name=? AND message_id=? FOR UPDATE`, m.Provider, m.Topic, m.Channel, m.ID).Scan(&version, &eventID, &messageID, &payload, &first)
	if err != nil {
		return err
	}
	if version != 2 || eventID != m.ID || messageID != m.ID || payload != string(m.Payload) || len(first) != 32 {
		return errors.New("existing hold is another protocol or immutable delivery")
	}
	_, err = eventtransport.PreserveRocketMQDelivery(ctx, tx, m)
	if err != nil {
		return err
	}
	var reference [32]byte
	copy(reference[:], first)
	original, err := eventtransport.LoadRocketMQDelivery(ctx, tx, reference)
	if err != nil {
		return err
	}
	if original.ID != m.ID || !bytes.Equal(original.Wire, m.Wire) {
		return errors.New("duplicate deferred delivery changed its first wire")
	}
	return tx.Commit()
}

type deferredItem struct {
	ID                   uint64
	Token                string
	Message              rmtransport.Received
	EventType            string
	ResumeCount, Attempt int
	Manual               bool
	previousManualUse    sql.NullString
	manualRequestID      string
}

func (s *DeferredDeliveryStore) claim(ctx context.Context, now time.Time, lease time.Duration) (*deferredItem, error) {
	if now.IsZero() || lease <= 0 {
		return nil, errors.New("bounded deferred claim time and lease required")
	}
	ctx, cancel := context.WithTimeout(ctx, deferredDBBudget)
	defer cancel()
	tx, err := s.original.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var item deferredItem
	var identity []byte
	var eventID, messageID, payload string
	var attempts int
	var timestamp int64
	var grant, used sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT id,event_id,message_id,payload_json,transport_identity_hash,original_delivery_attempt,received_timestamp,replay_attempt_count,manual_replay_request_id,used_manual_replay_request_id
FROM retry_event_hold WHERE provider='rocketmq' AND channel_name=? AND protocol_version=2
AND (status IN ('blocked','failed') OR (status='consuming' AND claim_expires_at<=?)) AND retry_disposition='automatic'
AND (next_attempt_at IS NULL OR next_attempt_at<=?) AND (claim_expires_at IS NULL OR claim_expires_at<=?)
ORDER BY COALESCE(next_attempt_at,blocked_at),id LIMIT 1 FOR UPDATE SKIP LOCKED`, s.channel, now, now, now).Scan(&item.ID, &eventID, &messageID, &payload, &identity, &attempts, &timestamp, &item.ResumeCount, &grant, &used)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(identity) != 32 || attempts < 1 || attempts > retrygovernance.HardMaxDeliveryAttempts || item.ResumeCount < 0 || item.ResumeCount >= 1<<31-1 {
		return nil, errors.New("invalid deferred consumption evidence")
	}
	var reference [32]byte
	copy(reference[:], identity)
	item.Message, err = eventtransport.LoadRocketMQDelivery(ctx, tx, reference)
	if err != nil {
		return nil, err
	}
	item.Message.Attempts, item.Message.Timestamp = uint16(attempts), timestamp
	if item.Message.ID != messageID || eventID != messageID || !bytes.Equal(item.Message.Payload, []byte(payload)) {
		return nil, errors.New("deferred consumption identity conflict")
	}
	item.EventType, err = s.eventType(item.Message)
	if err != nil {
		return nil, err
	}
	item.Attempt = attempts + item.ResumeCount
	item.Manual = grant.Valid && grant.String != "" && (!used.Valid || used.String != grant.String)
	item.previousManualUse = used
	if item.Manual {
		item.manualRequestID = grant.String
	}
	if item.Attempt > s.policy.MaxAutomaticAttempts && !item.Manual {
		_, err = tx.ExecContext(ctx, `UPDATE retry_event_hold SET status='failed',retry_disposition='manual_required',next_attempt_at=NULL,claim_token=NULL,claim_expires_at=NULL,last_error='deferred_delivery_budget_exhausted',updated_at=? WHERE id=? AND protocol_version=2`, now, item.ID)
		if err != nil {
			return nil, err
		}
		return nil, tx.Commit()
	}
	item.Token = uuid.NewString()
	item.ResumeCount++
	var grantUsed any
	if item.Manual {
		grantUsed = grant.String
	} else if used.Valid {
		grantUsed = used.String
	}
	result, err := tx.ExecContext(ctx, `UPDATE retry_event_hold SET status='consuming',claim_token=?,claim_expires_at=?,replay_attempt_count=?,used_manual_replay_request_id=?,updated_at=? WHERE id=? AND protocol_version=2 AND replay_attempt_count=?`, item.Token, now.Add(lease), item.ResumeCount, grantUsed, now, item.ID, item.ResumeCount-1)
	if err != nil {
		return nil, err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return nil, err
	} else if affected != 1 {
		return nil, ErrDeferredClaimLost
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *DeferredDeliveryStore) renew(ctx context.Context, item *deferredItem, now time.Time, lease time.Duration) error {
	if item == nil || item.ID == 0 || item.Token == "" || now.IsZero() || lease <= 0 {
		return ErrDeferredClaimLost
	}
	return s.updateClaim(ctx, `UPDATE retry_event_hold SET claim_expires_at=?,updated_at=? WHERE id=? AND protocol_version=2 AND status='consuming' AND claim_token=? AND claim_expires_at>?`, now.Add(lease), now, item.ID, item.Token, now)
}

func (s *DeferredDeliveryStore) complete(ctx context.Context, item *deferredItem, now time.Time) error {
	if item == nil || item.ID == 0 || item.Token == "" || now.IsZero() {
		return ErrDeferredClaimLost
	}
	return s.updateClaim(ctx, `UPDATE retry_event_hold SET status='consumed',retry_disposition=NULL,next_attempt_at=NULL,claim_token=NULL,claim_expires_at=NULL,last_error=NULL,consumed_at=?,updated_at=? WHERE id=? AND protocol_version=2 AND status='consuming' AND claim_token=? AND claim_expires_at>?`, now, now, item.ID, item.Token, now)
}

type deferredFailure string

const (
	deferredDispatchFailed deferredFailure = "deferred_dispatch_failed"
	deferredPaused         deferredFailure = "automatic_retry_paused"
	deferredUnknownHandler deferredFailure = "deferred_unknown_handler"
)

func (s *DeferredDeliveryStore) failed(ctx context.Context, item *deferredItem, now time.Time, reason deferredFailure) error {
	if item == nil || item.ID == 0 || item.Token == "" || now.IsZero() {
		return ErrDeferredClaimLost
	}
	switch reason {
	case deferredDispatchFailed, deferredPaused, deferredUnknownHandler:
	default:
		return errors.New("known deferred failure outcome required")
	}
	decision := s.policy.DecideFailureForKey(true, item.Attempt, now, item.Message.ID)
	count := item.ResumeCount
	var manualUse any
	if item.previousManualUse.Valid {
		manualUse = item.previousManualUse.String
	}
	switch reason {
	case deferredPaused:
		count--
		decision.Disposition = retrygovernance.DispositionAutomatic
		next := now.Add(5 * time.Second)
		decision.NextAttemptAt = &next
	case deferredUnknownHandler:
		decision.Disposition = retrygovernance.DispositionManualRequired
		decision.NextAttemptAt = nil
	}
	if reason != deferredPaused && item.Manual {
		// Claim already consumed this exact technical delivery authorization.
		manualUse = item.manualRequestID
	}
	return s.updateClaim(ctx, `UPDATE retry_event_hold SET status='failed',retry_disposition=?,next_attempt_at=?,claim_token=NULL,claim_expires_at=NULL,last_error=?,replay_attempt_count=?,used_manual_replay_request_id=?,updated_at=? WHERE id=? AND protocol_version=2 AND status='consuming' AND claim_token=? AND claim_expires_at>?`, string(decision.Disposition), decision.NextAttemptAt, string(reason), count, manualUse, now, item.ID, item.Token, now)
}

func (s *DeferredDeliveryStore) updateClaim(ctx context.Context, query string, args ...any) error {
	ctx, cancel := context.WithTimeout(ctx, deferredDBBudget)
	defer cancel()
	result, err := s.original.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil {
		return err
	} else if affected != 1 {
		return ErrDeferredClaimLost
	}
	return nil
}
