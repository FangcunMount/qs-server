// Package aimessaginghandoff transfers reviewed legacy commands without a relay.
// The caller stops all delivery claimers; the persistent gate serializes intake.
package aimessaginghandoff

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	store "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/aibridge"
	"github.com/google/uuid"
)

const MaxBatch = 20

var (
	ErrManifestDrift = errors.New("reviewed messaging handoff facts changed")
	ErrMaintenance   = errors.New("messaging handoff requires closed reviewed intake and stopped relays")
	ErrManifest      = errors.New("invalid messaging handoff manifest")
)

// Source excludes bodies, mutable business projections and transmission wire.
// The two byte digests bind the actual retained source in addition to business hashes.
type Source struct {
	CommandID          string  `json:"command_id"`
	RequestID          string  `json:"request_id"`
	Kind               string  `json:"kind"`
	BusinessHash       string  `json:"business_hash"`
	PayloadBytesSHA256 string  `json:"payload_bytes_sha256"`
	RequestHash        string  `json:"request_hash"`
	RequestBytesSHA256 string  `json:"request_bytes_sha256"`
	Attempts           int     `json:"attempts"`
	Delivered          bool    `json:"source_delivered"`
	AvailableAt        string  `json:"available_at_utc"`
	OriginalCreatedAt  *string `json:"original_created_at_utc"`
}

type Manifest struct {
	Revision               int      `json:"manifest_revision"`
	Database               string   `json:"database"`
	SchemaHead             uint64   `json:"schema_head"`
	AdmissionRevision      uint64   `json:"admission_revision"`
	UnknownOrderAggregates uint64   `json:"unknown_order_aggregates"`
	Rows                   []Source `json:"rows"`
}

func (m Manifest) SHA256() string {
	raw, _ := json.Marshal(m) // Only finite integers, fixed text and ordered rows.
	return digest(raw)
}

type RowResult struct {
	CommandID  string `json:"command_id"`
	Outcome    string `json:"outcome"`
	WireSHA256 string `json:"wire_sha256,omitempty"`
}

// An error can follow a committed row. The result explicitly preserves partial
// progress; commit_unknown is resolved by reading/reapplying the same manifest.
type Result struct {
	ManifestSHA256 string      `json:"manifest_sha256"`
	Rows           []RowResult `json:"rows"`
}

type Tool struct {
	DB      *sql.DB
	Handoff *store.MessagingLegacyHandoff
}

func digest(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }

func validateIDs(ids []string) error {
	if len(ids) < 1 || len(ids) > MaxBatch {
		return ErrManifest
	}
	seen := map[string]bool{}
	for _, id := range ids {
		value, err := uuid.Parse(id)
		if err != nil || value == uuid.Nil || value.String() != id || seen[id] {
			return ErrManifest
		}
		seen[id] = true
	}
	return nil
}

func header(ctx context.Context, tx *sql.Tx, lock bool) (Manifest, error) {
	m := Manifest{Revision: 1, Rows: []Source{}}
	var dirty bool
	if err := tx.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&m.Database); err != nil {
		return m, err
	}
	if err := tx.QueryRowContext(ctx, "SELECT version,dirty FROM schema_migrations").Scan(&m.SchemaHead, &dirty); err != nil {
		return m, err
	}
	if dirty || m.SchemaHead < 95 {
		return m, ErrManifest
	}
	gate := store.MessagingAdmission{}
	state, err := gate.Inspect(ctx, tx)
	if lock {
		state, err = gate.Read(ctx, tx)
	}
	if err != nil {
		return m, err
	}
	if !state.Closed {
		return m, ErrMaintenance
	}
	m.AdmissionRevision = state.Revision
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM (SELECT c.request_id FROM ai_bridge_commands c WHERE c.delivered=FALSE AND NOT EXISTS (SELECT 1 FROM ai_messaging_legacy_commands m WHERE m.command_id=c.command_id) GROUP BY c.request_id HAVING COUNT(*)>1) ambiguous`).Scan(&m.UnknownOrderAggregates)
	return m, err
}

func readSource(ctx context.Context, tx *sql.Tx, id string, lock bool) (Source, error) {
	s := Source{CommandID: id}
	var payload, request []byte
	var available time.Time
	var created sql.NullTime
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	err := tx.QueryRowContext(ctx, "SELECT request_id,kind,payload_hash,CAST(payload AS BINARY),attempts,delivered,available_at FROM ai_bridge_commands WHERE command_id=?"+suffix, id).Scan(&s.RequestID, &s.Kind, &s.BusinessHash, &payload, &s.Attempts, &s.Delivered, &available)
	if err != nil {
		return s, err
	}
	err = tx.QueryRowContext(ctx, "SELECT request_hash,CAST(payload AS BINARY),created_at FROM ai_bridge_requests WHERE request_id=?"+suffix, s.RequestID).Scan(&s.RequestHash, &request, &created)
	if err != nil {
		return s, err
	}
	if s.Attempts < 0 || (s.Kind != "start" && s.Kind != "answer" && s.Kind != "cancel") {
		return s, ErrManifest
	}
	s.PayloadBytesSHA256, s.RequestBytesSHA256 = digest(payload), digest(request)
	s.AvailableAt = available.UTC().Format(time.RFC3339Nano)
	if created.Valid {
		value := created.Time.UTC().Format(time.RFC3339Nano)
		s.OriginalCreatedAt = &value
	}
	return s, nil
}

// DryRun is always database-enforced READ ONLY. IDs are explicitly selected by
// the caller; display/retry order is never interpreted as commit order.
func (t *Tool) DryRun(ctx context.Context, ids []string) (Manifest, error) {
	var m Manifest
	if t == nil || t.DB == nil {
		return m, ErrManifest
	}
	if err := validateIDs(ids); err != nil {
		return m, err
	}
	tx, err := t.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return m, err
	}
	defer func() { _ = tx.Rollback() }()
	m, err = header(ctx, tx, false)
	if err != nil {
		return m, err
	}
	for _, id := range ids {
		s, err := readSource(ctx, tx, id, false)
		if err != nil {
			return m, err
		}
		m.Rows = append(m.Rows, s)
	}
	return m, nil
}

func (t *Tool) Apply(ctx context.Context, m Manifest, reviewedSHA256 string, relaysStopped bool) (Result, error) {
	r := Result{ManifestSHA256: m.SHA256(), Rows: []RowResult{}}
	if t == nil || t.DB == nil || t.Handoff == nil || m.Revision != 1 || reviewedSHA256 != r.ManifestSHA256 {
		return r, ErrManifest
	}
	ids := make([]string, len(m.Rows))
	for i, s := range m.Rows {
		ids[i] = s.CommandID
	}
	if err := validateIDs(ids); err != nil {
		return r, err
	}
	if !relaysStopped {
		return r, ErrMaintenance
	}
	if m.UnknownOrderAggregates != 0 {
		return r, store.ErrLegacyCommandOrderUnknown
	}
	for _, source := range m.Rows {
		row, err := t.applyOne(ctx, m, source)
		if row.Outcome != "" {
			r.Rows = append(r.Rows, row)
		}
		if err != nil {
			return r, err
		}
	}
	return r, nil
}

func (t *Tool) applyOne(ctx context.Context, m Manifest, source Source) (RowResult, error) {
	row := RowResult{CommandID: source.CommandID}
	tx, err := t.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return row, err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := header(ctx, tx, true)
	if err != nil {
		return row, err
	}
	if current.Database != m.Database || current.SchemaHead != m.SchemaHead || current.AdmissionRevision != m.AdmissionRevision || current.UnknownOrderAggregates != 0 {
		return row, ErrManifestDrift
	}
	actual, err := readSource(ctx, tx, source.CommandID, true)
	if err != nil {
		return row, err
	}
	if !reflect.DeepEqual(actual, source) {
		return row, ErrManifestDrift
	}
	transferred, err := t.Handoff.StageSingle(ctx, tx, source.CommandID)
	if err != nil {
		return row, err
	}
	var wire string
	var raw []byte
	err = tx.QueryRowContext(ctx, "SELECT wire_sha256,wire FROM ai_messaging_outbox WHERE producer='qs-server' AND destination='qs-ai' AND message_id=?", source.CommandID).Scan(&wire, &raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return row, err
	}
	if (transferred && len(wire) != 64) || (err == nil && digest(raw) != wire) {
		return row, fmt.Errorf("messaging handoff wire unavailable")
	}
	if err = tx.Commit(); err != nil {
		row.Outcome = "commit_unknown"
		return row, err
	}
	row.Outcome = "source_retained_no_new_transfer"
	if transferred {
		row.Outcome = "transferred"
	}
	row.WireSHA256 = wire
	return row, nil
}
