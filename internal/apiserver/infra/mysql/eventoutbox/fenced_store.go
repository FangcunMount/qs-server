package eventoutbox

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/outboxcore"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
)

// FencedStore is the isolated M4 MySQL delivery seam. It is not wired into
// either QS delivery loop until the matching schema and governance proofs pass.
type FencedStore struct{ db *sql.DB }

type FencedClaim struct {
	RecordID      uint64
	EventID       string
	TopicName     string
	PayloadJSON   string
	Token         string
	Version       uint64
	LeaseUntil    time.Time
	DeliveryCount uint64
	FailureCount  int
}

var ErrStaleDeliveryClaim = errors.New("outbox delivery claim expired or superseded")

const fencedTimeLayout = "2006-01-02 15:04:05.000000"

var fencedLocation = time.FixedZone("UTC+8", 8*3600)

// MySQL's UTC clock plus a fixed offset matches the existing QS DATETIME
// contract even if a connection's session time_zone is changed independently.
const fencedDBNow = "UTC_TIMESTAMP(6) + INTERVAL 8 HOUR"

func NewFencedStore(db *sql.DB) (*FencedStore, error) {
	if db == nil {
		return nil, errors.New("MySQL database required")
	}
	return &FencedStore{db: db}, nil
}

func (s *FencedStore) ClaimDue(ctx context.Context, limit int, lease, legacyStaleAfter time.Duration) ([]FencedClaim, error) {
	if s == nil || s.db == nil || limit < 1 || limit > 1000 || lease < time.Microsecond || lease > 24*time.Hour || legacyStaleAfter < time.Microsecond {
		return nil, errors.New("invalid claim bounds")
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	var nowText string
	if err := tx.QueryRowContext(ctx, "SELECT DATE_FORMAT("+fencedDBNow+", '%Y-%m-%d %H:%i:%s.%f')").Scan(&nowText); err != nil {
		return nil, err
	}
	now, err := time.ParseInLocation(fencedTimeLayout, nowText, fencedLocation)
	if err != nil {
		return nil, fmt.Errorf("decode MySQL UTC+8 clock: %w", err)
	}
	staleBefore := now.Add(-legacyStaleAfter).Format(fencedTimeLayout)
	rows, err := tx.QueryContext(ctx, `SELECT id,event_id,topic_name,payload_json,attempt_count,delivery_claim_version,delivery_attempt_count
 FROM domain_event_outbox
 WHERE ((status IN ('pending','failed') AND next_attempt_at<=?)
     OR (status='publishing' AND delivery_claim_token IS NOT NULL AND delivery_lease_until<=?)
     OR (status='publishing' AND delivery_claim_token IS NULL AND updated_at<=?))
   AND (retry_disposition IS NULL OR retry_disposition<>'manual_required')
 ORDER BY next_attempt_at,id LIMIT ? FOR UPDATE SKIP LOCKED`, nowText, nowText, staleBefore, limit)
	if err != nil {
		return nil, err
	}
	var claims []FencedClaim
	for rows.Next() {
		var c FencedClaim
		if err := rows.Scan(&c.RecordID, &c.EventID, &c.TopicName, &c.PayloadJSON, &c.FailureCount, &c.Version, &c.DeliveryCount); err != nil {
			return nil, errors.Join(err, rows.Close())
		}
		claims = append(claims, c)
	}
	err = errors.Join(rows.Err(), rows.Close())
	if err != nil {
		return nil, err
	}
	for i := range claims {
		c := &claims[i]
		var token [32]byte
		if _, err := rand.Read(token[:]); err != nil {
			return nil, err
		}
		c.Token = hex.EncodeToString(token[:])
		c.LeaseUntil = now.Add(lease).Truncate(time.Microsecond)
		result, err := tx.ExecContext(ctx, `UPDATE domain_event_outbox SET status='publishing',delivery_claim_token=?,delivery_lease_until=?,delivery_claim_version=delivery_claim_version+1,delivery_attempt_count=delivery_attempt_count+1,updated_at=? WHERE id=? AND delivery_claim_version=?`, c.Token, c.LeaseUntil.Format(fencedTimeLayout), nowText, c.RecordID, c.Version)
		if err != nil {
			return nil, err
		}
		n, err := result.RowsAffected()
		if err != nil || n != 1 {
			return nil, fmt.Errorf("claim update affected %d rows: %w", n, err)
		}
		c.Version++
		c.DeliveryCount++
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return claims, nil
}

func (s *FencedStore) Confirm(ctx context.Context, c FencedClaim) error {
	if !validFencedClaim(c) {
		return ErrStaleDeliveryClaim
	}
	r, err := s.db.ExecContext(ctx, `UPDATE domain_event_outbox SET status='published',published_at=`+fencedDBNow+`,updated_at=`+fencedDBNow+`,retry_disposition=NULL,delivery_claim_token=NULL,delivery_lease_until=NULL,delivery_claim_version=delivery_claim_version+1
 WHERE id=? AND event_id=? AND status='publishing' AND delivery_claim_token=? AND delivery_claim_version=? AND delivery_lease_until>`+fencedDBNow, c.RecordID, c.EventID, c.Token, c.Version)
	return fencedResult(r, err)
}

func (s *FencedStore) MarkFailedGoverned(ctx context.Context, c FencedClaim, reason string) (retrygovernance.Decision, error) {
	if !validFencedClaim(c) || reason == "" {
		return retrygovernance.Decision{}, ErrStaleDeliveryClaim
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return retrygovernance.Decision{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var nowText, token, leaseText, status, eventID string
	var version uint64
	var failures int
	if err := tx.QueryRowContext(ctx, `SELECT DATE_FORMAT(`+fencedDBNow+`, '%Y-%m-%d %H:%i:%s.%f'),event_id,status,COALESCE(delivery_claim_token,''),delivery_claim_version,COALESCE(DATE_FORMAT(delivery_lease_until,'%Y-%m-%d %H:%i:%s.%f'),''),attempt_count FROM domain_event_outbox WHERE id=? FOR UPDATE`, c.RecordID).Scan(&nowText, &eventID, &status, &token, &version, &leaseText, &failures); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return retrygovernance.Decision{}, ErrStaleDeliveryClaim
		}
		return retrygovernance.Decision{}, err
	}
	if eventID != c.EventID || status != outboxcore.StatusPublishing || token != c.Token || version != c.Version || leaseText <= nowText || failures != c.FailureCount {
		return retrygovernance.Decision{}, ErrStaleDeliveryClaim
	}
	now, err := time.ParseInLocation(fencedTimeLayout, nowText, fencedLocation)
	if err != nil {
		return retrygovernance.Decision{}, err
	}
	decision := retrygovernance.OutboxPolicy().DecideFailureForKey(true, failures+1, now, c.EventID)
	nextAt := now
	if decision.NextAttemptAt != nil {
		nextAt = *decision.NextAttemptAt
	}
	r, err := tx.ExecContext(ctx, `UPDATE domain_event_outbox SET status='failed',attempt_count=attempt_count+1,last_error=?,last_error_kind='publish',retry_disposition=?,next_attempt_at=?,updated_at=?,delivery_claim_token=NULL,delivery_lease_until=NULL,delivery_claim_version=delivery_claim_version+1
 WHERE id=? AND event_id=? AND status='publishing' AND delivery_claim_token=? AND delivery_claim_version=? AND delivery_lease_until>?`, reason, string(decision.Disposition), nextAt.Format(fencedTimeLayout), nowText, c.RecordID, c.EventID, c.Token, c.Version, nowText)
	if err := fencedResult(r, err); err != nil {
		return retrygovernance.Decision{}, err
	}
	if err := tx.Commit(); err != nil {
		return retrygovernance.Decision{}, err
	}
	return decision, nil
}

func (s *FencedStore) Quarantine(ctx context.Context, c FencedClaim, reason string) error {
	if !validFencedClaim(c) || reason == "" {
		return ErrStaleDeliveryClaim
	}
	r, err := s.db.ExecContext(ctx, `UPDATE domain_event_outbox SET status='failed',attempt_count=attempt_count+1,last_error=?,last_error_kind='encoding',retry_disposition='manual_required',updated_at=`+fencedDBNow+`,delivery_claim_token=NULL,delivery_lease_until=NULL,delivery_claim_version=delivery_claim_version+1
 WHERE id=? AND event_id=? AND status='publishing' AND delivery_claim_token=? AND delivery_claim_version=? AND delivery_lease_until>`+fencedDBNow, reason, c.RecordID, c.EventID, c.Token, c.Version)
	return fencedResult(r, err)
}

func validFencedClaim(c FencedClaim) bool {
	return c.RecordID > 0 && c.EventID != "" && len(c.Token) == 64 && c.Version > 0
}

func fencedResult(r sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("%w: affected %s rows", ErrStaleDeliveryClaim, strconv.FormatInt(n, 10))
	}
	return nil
}
