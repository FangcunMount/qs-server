package messaging

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	basemessaging "github.com/FangcunMount/component-base/pkg/messaging"
	drivermysql "github.com/go-sql-driver/mysql"

	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
)

type DeadLetterRecord struct {
	MessageID        string
	EventID          string
	OrgID            *int64
	Provider         string
	Topic            string
	Channel          string
	DeliveryAttempts int
	Payload          []byte
	LastError        string
	FailedAt         time.Time
}

type DeadLetterRecorder interface {
	RecordDeadLetter(context.Context, DeadLetterRecord) error
}

func failedMessageHandler(recorder DeadLetterRecorder) basemessaging.FailedMessageHandler {
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
			failed.Message.UUID, failed.Message.Payload, lastError,
		))
	}
}

type mysqlDeadLetterRecorder struct{ db *sql.DB }

func NewMySQLDeadLetterRecorder(options *genericoptions.MySQLOptions) (DeadLetterRecorder, error) {
	if options == nil || options.Host == "" || options.Database == "" {
		return nil, nil
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
		return nil, fmt.Errorf("open dead-letter audit store: %w", err)
	}
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(options.MaxConnectionLifeTime)
	return &mysqlDeadLetterRecorder{db: db}, nil
}

func (r *mysqlDeadLetterRecorder) RecordDeadLetter(ctx context.Context, record DeadLetterRecord) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("dead-letter audit store is not configured")
	}
	if record.MessageID == "" || record.Provider == "" || record.Topic == "" || record.Channel == "" || record.DeliveryAttempts < 1 {
		return fmt.Errorf("invalid dead-letter record")
	}
	if record.FailedAt.IsZero() {
		record.FailedAt = time.Now()
	}
	_, err := r.db.ExecContext(ctx, `
INSERT INTO event_delivery_dead_letter
  (message_id, event_id, org_id, provider, topic_name, channel_name, delivery_attempts,
   payload_json, last_error, retry_disposition, failed_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'manual_required', ?, ?, ?)
ON DUPLICATE KEY UPDATE
  event_id = VALUES(event_id), org_id = VALUES(org_id),
  delivery_attempts = GREATEST(delivery_attempts, VALUES(delivery_attempts)),
  payload_json = VALUES(payload_json), last_error = VALUES(last_error),
  retry_disposition = 'manual_required', failed_at = VALUES(failed_at), updated_at = VALUES(updated_at)`,
		record.MessageID, nullableString(record.EventID), record.OrgID, record.Provider, record.Topic, record.Channel,
		record.DeliveryAttempts, string(record.Payload), nullableString(record.LastError), record.FailedAt, record.FailedAt, record.FailedAt,
	)
	return err
}

func deadLetterRecord(provider, topic, channel string, attempts int, messageID string, payload []byte, lastError string) DeadLetterRecord {
	record := DeadLetterRecord{
		MessageID: messageID, Provider: provider, Topic: topic, Channel: channel,
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
