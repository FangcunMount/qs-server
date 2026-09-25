package transport

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	basemessaging "github.com/FangcunMount/component-base/pkg/messaging"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	drivermysql "github.com/go-sql-driver/mysql"
)

type DeadLetterRecord struct {
	// MessageID is the component-base message UUID. For enveloped domain events
	// it is the EventID, not the physical NSQ broker message ID.
	MessageID string
	// TransportMessageID distinguishes physical broker deliveries of one
	// logical message. Empty identifies an older handoff without this field.
	TransportMessageID string
	EventID            string
	OrgID              *int64
	Provider           string
	Topic              string
	Channel            string
	DeliveryAttempts   int
	Payload            []byte
	LastError          string
	FailedAt           time.Time
}

type DeadLetterRecorder interface {
	RecordDeadLetter(context.Context, DeadLetterRecord) error
}

type SQLDeadLetterRecorder struct {
	db    *sql.DB
	owned bool
}

func NewSQLDeadLetterRecorder(db *sql.DB) (*SQLDeadLetterRecorder, error) {
	if db == nil {
		return nil, fmt.Errorf("dead-letter audit store is not configured")
	}
	return &SQLDeadLetterRecorder{db: db}, nil
}

func OpenMySQLDeadLetterRecorder(options *genericoptions.MySQLOptions) (*SQLDeadLetterRecorder, error) {
	if options == nil || options.Host == "" || options.Database == "" {
		return nil, fmt.Errorf("dead-letter audit store is not configured")
	}
	locationName := options.Location
	if locationName == "" {
		locationName = "Asia/Shanghai"
	}
	location, err := time.LoadLocation(locationName)
	if err != nil {
		return nil, fmt.Errorf("invalid dead-letter mysql location %q: %w", locationName, err)
	}
	sessionTimeZone := options.SessionTimeZone
	if sessionTimeZone == "" {
		sessionTimeZone = "+08:00"
	}
	if _, err := time.Parse("-07:00", sessionTimeZone); err != nil {
		return nil, fmt.Errorf("invalid dead-letter mysql session time zone %q: %w", sessionTimeZone, err)
	}
	cfg := drivermysql.NewConfig()
	cfg.Net = "tcp"
	cfg.Addr = options.Host
	cfg.User = options.Username
	cfg.Passwd = options.Password
	cfg.DBName = options.Database
	cfg.ParseTime = true
	cfg.Loc = location
	cfg.Params = map[string]string{"time_zone": "'" + sessionTimeZone + "'"}
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("open dead-letter audit store: %w", err)
	}
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(options.MaxConnectionLifeTime)
	return &SQLDeadLetterRecorder{db: db, owned: true}, nil
}

func (r *SQLDeadLetterRecorder) Close() error {
	if r == nil || r.db == nil || !r.owned {
		return nil
	}
	return r.db.Close()
}

func FailedMessageHandler(recorder DeadLetterRecorder) basemessaging.FailedMessageHandler {
	return func(ctx context.Context, failed basemessaging.FailedMessage) error {
		if recorder == nil || failed.Message == nil {
			return fmt.Errorf("dead-letter audit store is not configured")
		}
		lastError := "transport delivery exhausted"
		if failed.Cause != nil {
			lastError = failed.Cause.Error()
		}
		return recorder.RecordDeadLetter(ctx, deadLetterRecord(
			failed.Provider, failed.Topic, failed.Channel, failed.Attempts,
			failed.Message.UUID, failed.Message.TransportMessageID, failed.Message.Payload, lastError,
		))
	}
}

// NewUnknownEventRecorder preserves an unsupported event before its Worker
// delivery is acknowledged. A failed database write must leave it unsettled.
func NewUnknownEventRecorder(provider string, recorder DeadLetterRecorder) func(context.Context, *basemessaging.Message, string) error {
	return func(ctx context.Context, msg *basemessaging.Message, eventType string) error {
		if recorder == nil || msg == nil || provider == "" || eventType == "" {
			return fmt.Errorf("unknown-event audit store or identity is not configured")
		}
		return recorder.RecordDeadLetter(ctx, deadLetterRecord(
			provider, msg.Topic, msg.Channel, max(int(msg.Attempts), 1), msg.UUID, msg.TransportMessageID, msg.Payload,
			"unknown event type: "+eventType,
		))
	}
}

func (r *SQLDeadLetterRecorder) RecordDeadLetter(ctx context.Context, record DeadLetterRecord) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("dead-letter audit store is not configured")
	}
	if record.MessageID == "" || len(record.TransportMessageID) > 64 || record.Provider == "" || record.Topic == "" || record.Channel == "" || record.DeliveryAttempts < 1 {
		return fmt.Errorf("invalid dead-letter record")
	}
	if record.FailedAt.IsZero() {
		record.FailedAt = time.Now()
	}
	// The physical transport ID separates new replay deliveries while the
	// empty value preserves deduplication for pre-upgrade failed handoffs.
	// The transaction prevents an identity collision from silently ACKing a
	// different tenant or payload under an existing failure row.
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
INSERT INTO event_delivery_dead_letter
  (message_id, transport_message_id, event_id, org_id, provider, topic_name, channel_name, delivery_attempts,
   payload_json, last_error, retry_disposition, failed_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'manual_required', ?, ?, ?)
ON DUPLICATE KEY UPDATE id=id`,
		record.MessageID, record.TransportMessageID, nullableString(record.EventID), record.OrgID,
		record.Provider, record.Topic, record.Channel, record.DeliveryAttempts, string(record.Payload),
		nullableString(record.LastError), record.FailedAt, record.FailedAt, record.FailedAt,
	); err != nil {
		return err
	}
	var rowID uint64
	var eventID sql.NullString
	var orgID sql.NullInt64
	var payload string
	var disposition string
	var replayRequestID sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT id,event_id,org_id,payload_json,retry_disposition,replay_request_id
FROM event_delivery_dead_letter
WHERE provider=? AND topic_name=? AND channel_name=? AND message_id=? AND transport_message_id=? FOR UPDATE`,
		record.Provider, record.Topic, record.Channel, record.MessageID, record.TransportMessageID,
	).Scan(&rowID, &eventID, &orgID, &payload, &disposition, &replayRequestID)
	if err != nil {
		return err
	}
	if eventID.String != record.EventID || eventID.Valid != (record.EventID != "") ||
		orgID.Valid != (record.OrgID != nil) || (orgID.Valid && orgID.Int64 != *record.OrgID) ||
		payload != string(record.Payload) {
		return fmt.Errorf("dead-letter delivery identity conflicts with existing event, organization, or payload")
	}
	if disposition == "manual_required" && !replayRequestID.Valid {
		if _, err := tx.ExecContext(ctx, `UPDATE event_delivery_dead_letter
SET delivery_attempts=GREATEST(delivery_attempts,?), last_error=?, failed_at=?, updated_at=? WHERE id=?`,
			record.DeliveryAttempts, nullableString(record.LastError), record.FailedAt, record.FailedAt, rowID,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func deadLetterRecord(provider, topic, channel string, attempts int, messageID, transportMessageID string, payload []byte, lastError string) DeadLetterRecord {
	record := DeadLetterRecord{
		MessageID: messageID, TransportMessageID: transportMessageID, Provider: provider, Topic: topic, Channel: channel,
		DeliveryAttempts: attempts, Payload: append([]byte(nil), payload...), LastError: lastError, FailedAt: time.Now(),
	}
	var envelope struct {
		ID   string `json:"id"`
		Data struct {
			OrgID int64 `json:"org_id"`
		} `json:"data"`
	}
	if json.Unmarshal(payload, &envelope) == nil {
		record.EventID = envelope.ID
		if envelope.Data.OrgID != 0 {
			orgID := envelope.Data.OrgID
			record.OrgID = &orgID
		}
	}
	if record.EventID == "" {
		record.EventID = messageID
	}
	return record
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
