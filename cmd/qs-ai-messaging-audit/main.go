// qs-ai-messaging-audit collects a read-only database snapshot for cutover review.
// It never starts a relay, reads payload content, installs schema or transfers rows.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"os"
	"time"

	"github.com/go-sql-driver/mysql"
)

var sourceSHA = "development"

var tables = []string{"ai_bridge_commands", "ai_bridge_requests", "ai_messaging_legacy_commands", "ai_messaging_outbox", "ai_messaging_operations", "ai_messaging_inbox", "ai_messaging_quarantine", "ai_messaging_failures", "ai_messaging_admission", "ai_messaging_observations"}

type pendingCommand struct {
	ID          string `json:"command_id"`
	RequestID   string `json:"request_id"`
	Kind        string `json:"kind"`
	BodyHash    string `json:"original_payload_hash"`
	Attempts    uint64 `json:"source_attempts"`
	AvailableAt string `json:"source_available_at_utc"`
	Ownership   string `json:"ownership"`
	Review      string `json:"review"`
	MQStage     string `json:"mq_stage,omitempty"`
	MQBodyHash  string `json:"mq_body_hash,omitempty"`
}
type stageCount struct {
	Kind  int    `json:"kind"`
	Stage string `json:"stage"`
	Count uint64 `json:"count"`
}
type messageMetadata struct {
	Producer        string `json:"producer"`
	Destination     string `json:"destination"`
	ID              string `json:"message_id"`
	BodyHash        string `json:"body_sha256"`
	WireHash        string `json:"wire_sha256"`
	BodyBytes       uint64 `json:"body_bytes"`
	WireBytes       uint64 `json:"wire_bytes"`
	Kind            int    `json:"kind"`
	Stage           string `json:"stage"`
	Aggregate       string `json:"aggregate_key"`
	Sequence        uint64 `json:"aggregate_sequence,string"`
	Attempts        uint64 `json:"attempts"`
	RequiresReceipt bool   `json:"requires_receipt"`
	BodyReference   bool   `json:"body_reference_under_candidate_codec"`
}
type snapshot struct {
	ReadOnly                bool              `json:"read_only"`
	SourceSHA               string            `json:"source_sha"`
	ObservedAt              string            `json:"observed_at"`
	Database                string            `json:"database"`
	MySQLVersion            string            `json:"mysql_version"`
	PresentTables           map[string]bool   `json:"present_tables"`
	MQTablesPresent         bool              `json:"mq_tables_present"`
	LegacyPending           uint64            `json:"legacy_source_pending"`
	LegacyDelivered         uint64            `json:"legacy_delivered_history"`
	UnownedPending          uint64            `json:"unowned_pending"`
	UnknownOrderAggregates  uint64            `json:"unknown_order_aggregates"`
	Pending                 []pendingCommand  `json:"pending_samples"`
	Truncated               bool              `json:"pending_samples_truncated"`
	MQStages                []stageCount      `json:"mq_outbox_stages"`
	SchemaHeadPresent       bool              `json:"schema_head_present"`
	SchemaHead              *uint64           `json:"schema_head"`
	SchemaDirty             *bool             `json:"schema_dirty"`
	RetainedReferenceBodies uint64            `json:"retained_reference_bodies"`
	UnconfirmedMessages     uint64            `json:"unconfirmed_messages"`
	MessageSamples          []messageMetadata `json:"message_samples"`
	MessageSamplesTruncated bool              `json:"message_samples_truncated"`
	ReferenceRule           string            `json:"body_reference_rule"`
	Inbox                   uint64            `json:"mq_inbox_count"`
	Quarantine              uint64            `json:"quarantine_count"`
	FailureLedger           uint64            `json:"failure_ledger_count"`
	Limit                   int               `json:"sample_limit"`
	Scope                   string            `json:"scope"`
}

func main() {
	limit := flag.Int("limit", 100, "bounded pending sample limit, 1..1000")
	flag.Parse()
	if err := run(*limit); err != nil {
		// DSN, database driver errors and payloads never appear in public output.
		fmt.Fprintln(os.Stderr, "read-only MQ database audit failed")
		os.Exit(1)
	}
}
func run(limit int) (resultErr error) {
	raw := os.Getenv("QS_AI_MESSAGING_AUDIT_DSN")
	if raw == "" {
		return errors.New("explicit database binding required")
	}
	cfg, err := mysql.ParseDSN(raw)
	if err != nil || cfg.DBName == "" {
		return errors.New("explicit database required")
	}
	cfg.ParseTime, cfg.Loc = true, time.UTC
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, db.Close()) }()
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := collect(ctx, db, limit)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}
func collect(ctx context.Context, db *sql.DB, limit int) (snapshot, error) {
	result := snapshot{ReadOnly: true, SourceSHA: sourceSHA, ObservedAt: time.Now().In(time.FixedZone("UTC+8", 28800)).Format(time.RFC3339Nano), PresentTables: map[string]bool{}, Pending: []pendingCommand{}, MQStages: []stageCount{}, MessageSamples: []messageMetadata{}, ReferenceRule: "candidate codec: retained body bytes > 32768; encrypted wire is not decrypted by this audit", Limit: limit, Scope: "database metadata only; not broker, key, workload, business or cutover acceptance"}
	if limit < 1 || limit > 1000 {
		return result, errors.New("bounded sample limit required")
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()
	return readSnapshot(ctx, tx, result)
}
func readSnapshot(ctx context.Context, tx *sql.Tx, result snapshot) (snapshot, error) {
	if err := tx.QueryRowContext(ctx, "SELECT DATABASE(),VERSION()").Scan(&result.Database, &result.MySQLVersion); err != nil {
		return result, err
	}
	var heads int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='schema_migrations'").Scan(&heads); err != nil {
		return result, err
	}
	result.SchemaHeadPresent = heads == 1
	if result.SchemaHeadPresent {
		var head uint64
		var dirty bool
		if err := tx.QueryRowContext(ctx, "SELECT version,dirty FROM schema_migrations").Scan(&head, &dirty); err != nil {
			return result, err
		}
		result.SchemaHead, result.SchemaDirty = &head, &dirty
	}
	for _, table := range tables {
		var count int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?", table).Scan(&count); err != nil {
			return result, err
		}
		result.PresentTables[table] = count == 1
	}
	result.MQTablesPresent = true
	for _, table := range tables[2:] {
		result.MQTablesPresent = result.MQTablesPresent && result.PresentTables[table]
	}
	if result.PresentTables["ai_bridge_commands"] {
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_bridge_commands WHERE delivered=FALSE").Scan(&result.LegacyPending); err != nil {
			return result, err
		}
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_bridge_commands WHERE delivered=TRUE").Scan(&result.LegacyDelivered); err != nil {
			return result, err
		}
		ownership, join := "NULL,NULL,NULL", ""
		unowned := ""
		if result.PresentTables["ai_messaging_legacy_commands"] {
			ownership = "m.command_id,NULL,NULL"
			join = " LEFT JOIN ai_messaging_legacy_commands m ON m.command_id=c.command_id"
			unowned = " AND m.command_id IS NULL"
			if result.PresentTables["ai_messaging_outbox"] {
				join += " LEFT JOIN ai_messaging_outbox b ON b.producer='qs-server' AND b.destination='qs-ai' AND b.message_id=c.command_id"
				ownership = "m.command_id,b.stage,IF(b.body_sha256=m.messaging_body_sha256 AND m.source_payload_hash=c.payload_hash AND m.request_id=c.request_id AND m.source_kind=c.kind,b.body_sha256,NULL)"
			}
		}
		counts, err := tx.QueryContext(ctx, "SELECT c.request_id,COUNT(*) FROM ai_bridge_commands c"+join+" WHERE c.delivered=FALSE"+unowned+" GROUP BY c.request_id")
		if err != nil {
			return result, err
		}
		aggregates := map[string]uint64{}
		for counts.Next() {
			var id string
			var n uint64
			if err = counts.Scan(&id, &n); err != nil {
				_ = counts.Close()
				return result, err
			}
			aggregates[id] = n
			result.UnownedPending += n
			if n > 1 {
				result.UnknownOrderAggregates++
			}
		}
		err = counts.Err()
		_ = counts.Close()
		if err != nil {
			return result, err
		}
		// Stable display order is deliberately not claimed as commit order.
		rows, err := tx.QueryContext(ctx, "SELECT c.command_id,c.request_id,c.kind,c.payload_hash,c.attempts,c.available_at,"+ownership+" FROM ai_bridge_commands c"+join+" WHERE c.delivered=FALSE ORDER BY c.request_id,c.command_id LIMIT ?", result.Limit)
		if err != nil {
			return result, err
		}
		for rows.Next() {
			var row pendingCommand
			var at time.Time
			var owned, stage, hash sql.NullString
			if err = rows.Scan(&row.ID, &row.RequestID, &row.Kind, &row.BodyHash, &row.Attempts, &at, &owned, &stage, &hash); err != nil {
				_ = rows.Close()
				return result, err
			}
			row.AvailableAt = at.UTC().Format(time.RFC3339Nano)
			row.Ownership = "legacy"
			row.Review = "single_pending_requires_original_transaction_validation"
			if aggregates[row.RequestID] > 1 {
				row.Review = "unknown_commit_order_requires_review"
			}
			if owned.Valid {
				row.Ownership = "audit_recorded"
				row.Review = "ownership_link_unverified"
				if stage.Valid && hash.Valid {
					row.Ownership = "mq"
					row.Review = "source_retained_after_handoff"
					row.MQStage = stage.String
					row.MQBodyHash = hash.String
				}
			}
			result.Pending = append(result.Pending, row)
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return result, err
		}
		result.Truncated = result.LegacyPending > uint64(len(result.Pending))
	}
	if result.PresentTables["ai_messaging_outbox"] {
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_messaging_outbox WHERE stage<>'confirmed'").Scan(&result.UnconfirmedMessages); err != nil {
			return result, err
		}
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_messaging_outbox WHERE OCTET_LENGTH(body)>?", app.InlineMessagingBody).Scan(&result.RetainedReferenceBodies); err != nil {
			return result, err
		}
		var total uint64
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM ai_messaging_outbox WHERE stage<>'confirmed' OR OCTET_LENGTH(body)>?", app.InlineMessagingBody).Scan(&total); err != nil {
			return result, err
		}
		metadata, err := tx.QueryContext(ctx, `SELECT producer,destination,message_id,body_sha256,wire_sha256,OCTET_LENGTH(body),OCTET_LENGTH(wire),kind,stage,aggregate_key,aggregate_sequence,attempts,requires_receipt FROM ai_messaging_outbox WHERE stage<>'confirmed' OR OCTET_LENGTH(body)>? ORDER BY producer,destination,message_id LIMIT ?`, app.InlineMessagingBody, result.Limit)
		if err != nil {
			return result, err
		}
		for metadata.Next() {
			var row messageMetadata
			if err = metadata.Scan(&row.Producer, &row.Destination, &row.ID, &row.BodyHash, &row.WireHash, &row.BodyBytes, &row.WireBytes, &row.Kind, &row.Stage, &row.Aggregate, &row.Sequence, &row.Attempts, &row.RequiresReceipt); err != nil {
				_ = metadata.Close()
				return result, err
			}
			row.BodyReference = row.BodyBytes > app.InlineMessagingBody
			result.MessageSamples = append(result.MessageSamples, row)
		}
		err = metadata.Err()
		_ = metadata.Close()
		if err != nil {
			return result, err
		}
		result.MessageSamplesTruncated = total > uint64(len(result.MessageSamples))
		rows, err := tx.QueryContext(ctx, "SELECT kind,stage,COUNT(*) FROM ai_messaging_outbox GROUP BY kind,stage ORDER BY kind,stage")
		if err != nil {
			return result, err
		}
		for rows.Next() {
			var row stageCount
			if err = rows.Scan(&row.Kind, &row.Stage, &row.Count); err != nil {
				_ = rows.Close()
				return result, err
			}
			result.MQStages = append(result.MQStages, row)
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return result, err
		}
	}
	for _, item := range []struct {
		table  string
		target *uint64
	}{{"ai_messaging_inbox", &result.Inbox}, {"ai_messaging_quarantine", &result.Quarantine}, {"ai_messaging_failures", &result.FailureLedger}} {
		if result.PresentTables[item.table] {
			if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+item.table).Scan(item.target); err != nil {
				return result, err
			}
		}
	}
	return result, nil
}
