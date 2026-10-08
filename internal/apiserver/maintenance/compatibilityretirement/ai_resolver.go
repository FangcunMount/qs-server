package retirement

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"github.com/FangcunMount/reliable-messaging/wire/protected"
	jose "github.com/go-jose/go-jose/v4"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

const (
	ErrAILocalBinding        SourceError = "ai_local_binding_rejected"
	ErrAILocalTransaction    SourceError = "ai_local_original_transaction_required"
	ErrAILocalRead           SourceError = "ai_local_fact_read_failed"
	ErrAILocalChanged        SourceError = "ai_local_fact_baseline_changed"
	ErrAILocalRelation       SourceError = "ai_local_fact_relation_conflict"
	ErrAILocalTerminal       SourceError = "ai_local_business_terminal_unproven"
	ErrAILocalResponsibility SourceError = "ai_local_unfinished_responsibility"
	ErrAILocalUnknown        SourceError = "ai_local_responsibility_unknown"
	ErrAILocalBounds         SourceError = "ai_local_fact_bound_exceeded"
	aiLocalMaxRows                       = 1024
	aiLocalMaxBytes                      = 64 << 20
)

// AIResolverBinding comes from the independently approved v2 scan. Identity
// uses the inventory's framed hashParts, table schema uses SHOW CREATE JSON,
// and Columns is its exact six-tuple ordinal metadata. No closure flags or
// inferred business facts can be imported through this constructor.
type AIResolverBinding struct {
	DatabaseIdentityHash string
	MigrationVersion     uint64
	SourceSHA            string
	OperationID          string
	AdmissionRevision    uint64
	BridgeBoundary       SourceBoundary
	BridgeColumns        SQLColumns
	LegacyBoundary       SourceBoundary
	LegacyColumns        SQLColumns
}

// AILocalResolver only borrows a supported original transaction. Locking SELECTs
// give current reads, including Recheck on a host RR transaction. It never
// opens, commits, rolls back or closes a connection and never submits messages.
type AILocalResolver struct {
	pool    gorm.ConnPool
	binding AIResolverBinding
}

type AILocalQualification struct {
	mu             sync.Mutex
	binding        AIResolverBinding
	bridge, legacy *DecodedAICommand
	baseline       string
	nodes          uint64
	gaps           []string
	qualified      bool
	invalid        bool
}

// AILocalSummary deliberately carries hashes/fixed categories only. Local point
// graph qualification is not a retired conclusion or global reverse audit.
type AILocalSummary struct {
	Version               int      `json:"version"`
	Scope                 string   `json:"scope"`
	LocalQualified        bool     `json:"local_qualified"`
	DatabaseIdentityHash  string   `json:"database_identity_hash"`
	SourceSHA             string   `json:"source_sha"`
	OperationID           string   `json:"operation_id"`
	BaselineSHA256        string   `json:"baseline_sha256"`
	ObservedNodes         uint64   `json:"observed_nodes"`
	Gaps                  []string `json:"gaps"`
	ExternalClosure       string   `json:"external_closure"`
	GlobalReverseCoverage string   `json:"global_reverse_coverage"`
	RetirementConclusion  string   `json:"retirement_conclusion"`
	DropReady             bool     `json:"drop_ready"`
}

func (*AILocalQualification) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*AILocalQualification) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*AILocalQualification) String() string               { return "opaque AI local point-graph qualification" }
func (q *AILocalQualification) GoString() string           { return q.String() }
func (q *AILocalQualification) Summary() AILocalSummary {
	if q == nil {
		return AILocalSummary{ExternalClosure: "unknown", GlobalReverseCoverage: "unknown", RetirementConclusion: "not_generated"}
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return AILocalSummary{Version: 1, Scope: "qs-local-point-graph-only", LocalQualified: q.qualified && !q.invalid, DatabaseIdentityHash: q.binding.DatabaseIdentityHash, SourceSHA: q.binding.SourceSHA, OperationID: q.binding.OperationID, BaselineSHA256: q.baseline, ObservedNodes: q.nodes, Gaps: append([]string(nil), q.gaps...), ExternalClosure: "unknown", GlobalReverseCoverage: "unknown", RetirementConclusion: "not_generated"}
}

func aiFramedParts(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		sourceFrame(h, []byte(part), false)
	}
	return hex.EncodeToString(h.Sum(nil))
}
func aiJSONHash(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return sourceSHA(raw)
}
func aiCloneColumns(columns SQLColumns) SQLColumns {
	out := make(SQLColumns, len(columns))
	for i, row := range columns {
		out[i] = make([]*string, len(row))
		for j, v := range row {
			if v != nil {
				copy := *v
				out[i][j] = &copy
			}
		}
	}
	return out
}
func aiCloneBinding(b AIResolverBinding) AIResolverBinding {
	b.BridgeColumns = aiCloneColumns(b.BridgeColumns)
	b.LegacyColumns = aiCloneColumns(b.LegacyColumns)
	return b
}
func aiCloneCommand(v *DecodedAICommand) *DecodedAICommand {
	if v == nil {
		return nil
	}
	out := *v
	out.Business.AssessmentIDs = append([]string(nil), v.Business.AssessmentIDs...)
	out.ResolverGaps = append([]string(nil), v.ResolverGaps...)
	if v.Transport.Delivered != nil {
		value := *v.Transport.Delivered
		out.Transport.Delivered = &value
	}
	return &out
}
func aiValidBinding(b AIResolverBinding) bool {
	sha, err := hex.DecodeString(b.SourceSHA)
	if err != nil || len(sha) != 20 || hex.EncodeToString(sha) != b.SourceSHA || !evidenceHash(b.DatabaseIdentityHash) || b.MigrationVersion == 0 || !aiLocalOperationID(b.OperationID) {
		return false
	}
	for _, entry := range []struct {
		table    string
		boundary SourceBoundary
		columns  SQLColumns
	}{{AIBridgeCommandSource, b.BridgeBoundary, b.BridgeColumns}, {AILegacyCommandSource, b.LegacyBoundary, b.LegacyColumns}} {
		boundary := entry.boundary
		if boundary.Database != "mysql" || boundary.Name != entry.table || boundary.Kind != "base_table" || !boundary.Present || boundary.PKType != "ascii_string" || !evidenceHash(boundary.SchemaHash) || boundary.IdentityHash != aiFramedParts("mysql-object-v1", entry.table, boundary.SchemaHash) || aiValidateColumns(entry.table, entry.columns) != nil {
			return false
		}
		if boundary.Empty {
			if boundary.UpperToken != "" {
				return false
			}
		} else {
			raw, e := canonicalBase64(boundary.UpperToken)
			if e != nil || !aiOriginalUUID(string(raw)) {
				return false
			}
		}
	}
	return true
}
func evidenceHash(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == 32 && hex.EncodeToString(raw) == value
}
func aiLocalOperationID(value string) bool {
	parts := strings.Split(value, "-")
	if len(parts) != 2 || len(parts[0]) < 1 || len(parts[0]) > 20 || len(parts[1]) < 1 || len(parts[1]) > 4 {
		return false
	}
	for _, part := range parts {
		for _, c := range part {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}

func NewAILocalResolver(ctx context.Context, tx *gorm.DB, binding AIResolverBinding) (*AILocalResolver, error) {
	if !aiValidBinding(binding) {
		return nil, ErrAILocalBinding
	}
	if tx == nil || tx.Error != nil || tx.Statement == nil || tx.Statement.ConnPool == nil {
		return nil, ErrAILocalTransaction
	}
	if prepared, ok := tx.Statement.ConnPool.(*gorm.PreparedStmtTX); ok && prepared == nil {
		return nil, ErrAILocalTransaction
	}
	if _, err := sdkmysql.BindGORM(tx); err != nil {
		return nil, ErrAILocalTransaction
	}
	r := &AILocalResolver{pool: tx.Statement.ConnPool, binding: aiCloneBinding(binding)}
	if err := r.verifyBinding(ctx); err != nil {
		return nil, err
	}
	return r, nil
}

// Every query is fixed or uses identifiers from this compile-time closed list.
// All raw values stay in memory; errors never embed SQL, IDs, DSNs or bodies.
func (r *AILocalResolver) read(ctx context.Context, q string, args ...any) (out [][][]byte, result error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rows, err := r.pool.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, ErrAILocalRead
	}
	defer func() {
		if rows.Close() != nil && result == nil {
			result = ErrAILocalRead
		}
	}()
	cols, err := rows.Columns()
	if err != nil {
		return nil, ErrAILocalRead
	}
	var size uint64
	for rows.Next() {
		if len(out) >= aiLocalMaxRows {
			return nil, ErrAILocalBounds
		}
		cells := make([][]byte, len(cols))
		dest := make([]any, len(cols))
		for i := range cells {
			dest[i] = &cells[i]
		}
		if rows.Scan(dest...) != nil {
			return nil, ErrAILocalRead
		}
		for _, raw := range cells {
			size += uint64(len(raw))
			if size > aiLocalMaxBytes {
				return nil, ErrAILocalBounds
			}
		}
		out = append(out, cells)
	}
	if rows.Err() != nil {
		return nil, ErrAILocalRead
	}
	return out, nil
}

func (r *AILocalResolver) verifyBinding(ctx context.Context) error {
	identity, err := r.read(ctx, "SELECT @@server_uuid,DATABASE(),VERSION()")
	if err != nil {
		return err
	}
	if len(identity) != 1 || len(identity[0]) != 3 || len(identity[0][0]) == 0 || len(identity[0][1]) == 0 || !strings.HasPrefix(string(identity[0][2]), "8.") || aiFramedParts("mysql_database_identity_v1", string(identity[0][0]), string(identity[0][1])) != r.binding.DatabaseIdentityHash {
		return ErrAILocalBinding
	}
	head, err := r.read(ctx, "SELECT CAST(version AS BINARY),CAST(dirty AS BINARY) FROM schema_migrations FOR UPDATE")
	if err != nil {
		return err
	}
	if len(head) != 1 || len(head[0]) != 2 || string(head[0][0]) != strconv.FormatUint(r.binding.MigrationVersion, 10) || string(head[0][1]) != "0" {
		return ErrAILocalBinding
	}
	for _, target := range []struct {
		table    string
		boundary SourceBoundary
		columns  SQLColumns
	}{{AIBridgeCommandSource, r.binding.BridgeBoundary, r.binding.BridgeColumns}, {AILegacyCommandSource, r.binding.LegacyBoundary, r.binding.LegacyColumns}} {
		ddl, err := r.read(ctx, "SHOW CREATE TABLE `"+target.table+"`")
		if err != nil {
			return err
		}
		if len(ddl) != 1 || len(ddl[0]) != 2 || ddl[0][0] == nil || ddl[0][1] == nil {
			return ErrAILocalBinding
		}
		encoded := SQLColumns{{ptrString(string(ddl[0][0])), ptrString(string(ddl[0][1]))}}
		if aiJSONHash(encoded) != target.boundary.SchemaHash {
			return ErrAILocalBinding
		}
		metadata, err := r.read(ctx, "SELECT COLUMN_NAME,COLUMN_TYPE,IS_NULLABLE,COLUMN_DEFAULT,EXTRA,COLLATION_NAME FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? ORDER BY ORDINAL_POSITION", target.table)
		if err != nil {
			return err
		}
		columns := make(SQLColumns, len(metadata))
		for i, cells := range metadata {
			columns[i] = make([]*string, len(cells))
			for j, raw := range cells {
				if raw != nil {
					columns[i][j] = ptrString(string(raw))
				}
			}
		}
		if !reflect.DeepEqual(columns, target.columns) {
			return ErrAILocalBinding
		}
	}
	return nil
}
func ptrString(value string) *string { return &value }

const aiRequestQuery = `SELECT CAST(request_id AS BINARY),CAST(request_hash AS BINARY),CAST(payload AS BINARY),CAST(session_id AS BINARY),CAST(version AS BINARY),CAST(status AS BINARY),CAST(projection AS BINARY),CAST(organization_id AS BINARY),CAST(subject_id AS BINARY),CAST(testee_id AS BINARY),CAST(created_at AS BINARY),CAST(updated_at AS BINARY) FROM ai_bridge_requests WHERE request_id=? OR session_id=? ORDER BY request_id LIMIT 1025 FOR UPDATE`
const aiBridgeQuery = `SELECT CAST(command_id AS BINARY),CAST(request_id AS BINARY),CAST(kind AS BINARY),CAST(payload AS BINARY),CAST(payload_hash AS BINARY),CAST(delivered AS BINARY),CAST(attempts AS BINARY),CAST(available_at AS BINARY) FROM ai_bridge_commands WHERE command_id=? OR request_id=? ORDER BY command_id LIMIT 1025 FOR UPDATE`
const aiLegacyQuery = `SELECT CAST(command_id AS BINARY),CAST(request_id AS BINARY),CAST(source_kind AS BINARY),CAST(source_payload AS BINARY),CAST(source_payload_hash AS BINARY),CAST(source_attempts AS BINARY),CAST(source_available_at AS BINARY),CAST(source_original_time AS BINARY),CAST(messaging_body_sha256 AS BINARY),CAST(transferred_at AS BINARY) FROM ai_messaging_legacy_commands WHERE command_id=? OR request_id=? ORDER BY command_id LIMIT 1025 FOR UPDATE`
const aiOperationQuery = `SELECT CAST(command_id AS BINARY),CAST(kind AS BINARY),CAST(body_sha256 AS BINARY),CAST(organization_id AS BINARY),CAST(subject_id AS BINARY),CAST(resource_id AS BINARY),CAST(aggregate_key AS BINARY),CAST(aggregate_sequence AS BINARY),CAST(decision AS BINARY),CAST(code AS BINARY),CAST(receipt_id AS BINARY),CAST(receipt AS BINARY),CAST(created_at AS BINARY),CAST(decided_at AS BINARY),CAST(retired AS BINARY),CAST(retirement_evidence AS BINARY),CAST(retired_at AS BINARY) FROM ai_messaging_operations WHERE command_id=? OR aggregate_key=? OR resource_id=? ORDER BY command_id LIMIT 1025 FOR UPDATE`
const aiOutboxQuery = `SELECT CAST(producer AS BINARY),CAST(destination AS BINARY),CAST(message_id AS BINARY),CAST(body_sha256 AS BINARY),CAST(body AS BINARY),CAST(wire AS BINARY),CAST(wire_sha256 AS BINARY),CAST(kind AS BINARY),CAST(organization_id AS BINARY),CAST(topic AS BINARY),CAST(aggregate_key AS BINARY),CAST(aggregate_sequence AS BINARY),CAST(ordered AS BINARY),CAST(requires_receipt AS BINARY),CAST(stage AS BINARY),CAST(attempts AS BINARY),CAST(available_at AS BINARY),CAST(created_at AS BINARY),CAST(published_at AS BINARY),CAST(confirmed_at AS BINARY),CAST(error_code AS BINARY) FROM ai_messaging_outbox WHERE aggregate_key=? OR message_id=? OR message_id=? ORDER BY producer,destination,message_id LIMIT 1025 FOR UPDATE`
const aiInboxBase = `SELECT CAST(producer AS BINARY),CAST(message_id AS BINARY),CAST(body_sha256 AS BINARY),CAST(body AS BINARY),CAST(wire_sha256 AS BINARY),CAST(kind AS BINARY),CAST(aggregate_key AS BINARY),CAST(ack_id AS BINARY),CAST(received_at AS BINARY),CAST(outcome AS BINARY) FROM ai_messaging_inbox WHERE aggregate_key=? OR message_id IN (`
const aiFailureBase = `SELECT CAST(producer AS BINARY),CAST(message_id AS BINARY),CAST(body_sha256 AS BINARY),CAST(kind AS BINARY),CAST(aggregate_key AS BINARY),CAST(wire AS BINARY),CAST(attempts AS BINARY),CAST(first_seen_at AS BINARY),CAST(last_seen_at AS BINARY) FROM ai_messaging_failures WHERE aggregate_key=? OR message_id IN (`

type aiLocalSnapshot struct {
	sets  [][][][]byte
	names []string
	nodes uint64
	size  uint64
}

func (s *aiLocalSnapshot) add(name string, rows [][][]byte) error {
	for _, row := range rows {
		s.nodes++
		for _, cell := range row {
			s.size += uint64(len(cell))
			if s.size > aiLocalMaxBytes {
				return ErrAILocalBounds
			}
		}
	}
	s.names = append(s.names, name)
	s.sets = append(s.sets, rows)
	return nil
}
func (s *aiLocalSnapshot) hash() string {
	h := sha256.New()
	for i, rows := range s.sets {
		sourceFrame(h, []byte(s.names[i]), false)
		sourceFrame(h, []byte(strconv.Itoa(len(rows))), false)
		for _, row := range rows {
			sourceFrame(h, []byte(strconv.Itoa(len(row))), false)
			for _, cell := range row {
				sourceFrame(h, cell, cell == nil)
			}
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}
func aiIDsClause(base string, ids []string, suffix string) (string, []any) {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return base + strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",") + ")" + suffix, args
}
func aiText(row [][]byte, index int) string { return string(row[index]) }
func aiUint(raw []byte) (uint64, bool) {
	n, err := strconv.ParseUint(string(raw), 10, 64)
	return n, err == nil && strconv.FormatUint(n, 10) == string(raw)
}
func aiKind(raw []byte) (pb.MessagingKind, bool) {
	n, err := strconv.ParseInt(string(raw), 10, 32)
	return pb.MessagingKind(n), err == nil && strconv.FormatInt(n, 10) == string(raw)
}

func (r *AILocalResolver) Resolve(ctx context.Context, bridge, legacySource *DecodedAICommand) (*AILocalQualification, error) {
	if r == nil || r.pool == nil || !aiValidBinding(r.binding) {
		return nil, ErrAILocalBinding
	}
	if !aiDecodedIdentityValid(bridge) || bridge.Source.Object != AIBridgeCommandSource {
		return nil, ErrAILocalBinding
	}
	if legacySource != nil && ValidateAISourcePair(bridge, legacySource) != nil {
		return nil, ErrAISourceHandoff
	}
	snapshot, gaps, err := r.observe(ctx, bridge, legacySource)
	if err != nil {
		return nil, err
	}
	return &AILocalQualification{binding: aiCloneBinding(r.binding), bridge: aiCloneCommand(bridge), legacy: aiCloneCommand(legacySource), baseline: snapshot.hash(), nodes: snapshot.nodes, gaps: gaps, qualified: true}, nil
}

// Recheck uses a real caller-supplied transaction and re-observes every baseline
// fact, including metadata and reverse points, with current locking reads. Any
// read failure or change invalidates this qualification; it cannot be resumed.
func (q *AILocalQualification) Recheck(ctx context.Context, tx *gorm.DB) error {
	if q == nil {
		return ErrAILocalBinding
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.qualified || q.invalid {
		return ErrAILocalChanged
	}
	r, err := NewAILocalResolver(ctx, tx, q.binding)
	if err == nil {
		var current *aiLocalSnapshot
		current, _, err = r.observe(ctx, q.bridge, q.legacy)
		if err == nil && current.hash() != q.baseline {
			err = ErrAILocalChanged
		}
	}
	if err != nil {
		q.invalid = true
	}
	return err
}

func (r *AILocalResolver) observe(ctx context.Context, source, legacySource *DecodedAICommand) (*aiLocalSnapshot, []string, error) {
	if err := r.verifyBinding(ctx); err != nil {
		return nil, nil, err
	}
	s := &aiLocalSnapshot{}
	read := func(name, q string, args ...any) ([][][]byte, error) {
		rows, err := r.read(ctx, q, args...)
		if err == nil {
			err = s.add(name, rows)
		}
		return rows, err
	}
	admission, err := read("admission", "SELECT CAST(closed AS BINARY),CAST(revision AS BINARY),CAST(updated_at AS BINARY) FROM ai_messaging_admission WHERE singleton=1 LOCK IN SHARE MODE")
	if err != nil {
		return nil, nil, err
	}
	if len(admission) != 1 || len(admission[0]) != 3 || aiText(admission[0], 0) != "1" || aiText(admission[0], 1) != strconv.FormatUint(r.binding.AdmissionRevision, 10) {
		return nil, nil, ErrAILocalBinding
	}
	requests, err := read("requests", aiRequestQuery, source.RequestID, source.ResourceID)
	if err != nil {
		return nil, nil, err
	}
	if len(requests) != 1 || len(requests[0]) != 12 || aiText(requests[0], 0) != source.RequestID {
		return nil, nil, ErrAILocalRelation
	}
	request, projection, err := aiVerifyRequest(requests[0], source)
	if err != nil {
		return nil, nil, err
	}
	events, err := read("events", "SELECT CAST(event_id AS BINARY),CAST(request_id AS BINARY),CAST(version AS BINARY),CAST(payload_hash AS BINARY) FROM ai_bridge_events WHERE event_id=? OR request_id=? ORDER BY event_id LIMIT 1025 FOR UPDATE", projection.EventID, source.RequestID)
	if err != nil {
		return nil, nil, err
	}
	canonical, err := json.Marshal(projection)
	if err != nil {
		return nil, nil, ErrAILocalTerminal
	}
	currentEvent := false
	for _, row := range events {
		if len(row) != 4 || !aiOriginalUUID(aiText(row, 0)) || aiText(row, 1) != source.RequestID || !evidenceHash(aiText(row, 3)) {
			return nil, nil, ErrAILocalRelation
		}
		version, valid := aiUint(row[2])
		if !valid || version == 0 || version > uint64(projection.Version) {
			return nil, nil, ErrAILocalRelation
		}
		if aiText(row, 0) == projection.EventID {
			if currentEvent || version != uint64(projection.Version) || aiText(row, 3) != sourceSHA(canonical) {
				return nil, nil, ErrAILocalTerminal
			}
			currentEvent = true
		}
	}
	if !currentEvent {
		return nil, nil, ErrAILocalTerminal
	}
	assessments, err := read("assessments", "SELECT CAST(request_id AS BINARY),CAST(assessment_id AS BINARY) FROM ai_bridge_request_assessments WHERE request_id=? ORDER BY assessment_id LIMIT 1025 FOR UPDATE", source.RequestID)
	if err != nil {
		return nil, nil, err
	}
	if len(assessments) != len(request.AssessmentIDs) {
		return nil, nil, ErrAILocalRelation
	}
	expectedAssessments := map[string]bool{}
	for _, id := range request.AssessmentIDs {
		expectedAssessments[id] = true
	}
	for _, row := range assessments {
		if len(row) != 2 || aiText(row, 0) != source.RequestID || !expectedAssessments[aiText(row, 1)] {
			return nil, nil, ErrAILocalRelation
		}
		delete(expectedAssessments, aiText(row, 1))
	}
	bridges, err := read("bridge", aiBridgeQuery, source.CommandID, source.RequestID)
	if err != nil {
		return nil, nil, err
	}
	legacyRows, err := read("legacy", aiLegacyQuery, source.CommandID, source.RequestID)
	if err != nil {
		return nil, nil, err
	}
	currentSources, expectedBodyHashes, err := r.verifySourceRows(bridges, legacyRows, source, legacySource, request, requests[0][10])
	if err != nil {
		return nil, nil, err
	}
	ops, err := read("operations", aiOperationQuery, source.CommandID, source.RequestID, projection.SessionID)
	if err != nil {
		return nil, nil, err
	}
	aggregates, err := read("aggregate_order", "SELECT CAST(aggregate_key AS BINARY),CAST(next_sequence AS BINARY) FROM ai_messaging_aggregates WHERE aggregate_key=? FOR UPDATE", source.RequestID)
	if err != nil {
		return nil, nil, err
	}
	if err = aiVerifyAggregate(aggregates, ops, source.RequestID); err != nil {
		return nil, nil, err
	}
	boxes, err := read("outbox", aiOutboxQuery, source.RequestID, source.CommandID, projection.EventID)
	if err != nil {
		return nil, nil, err
	}
	ids := []string{source.CommandID, source.RequestID, projection.EventID}
	for _, op := range ops {
		if len(op) != 17 {
			return nil, nil, ErrAILocalRelation
		}
		if op[10] != nil {
			ids = append(ids, aiText(op, 10))
		}
	}
	query, args := aiIDsClause(aiInboxBase, ids, " ORDER BY producer,message_id LIMIT 1025 FOR UPDATE")
	args = append([]any{source.RequestID}, args...)
	inbox, err := read("inbox", query, args...)
	if err != nil {
		return nil, nil, err
	}
	for _, entry := range inbox {
		if len(entry) != 10 {
			return nil, nil, ErrAILocalRelation
		}
		ids = append(ids, aiText(entry, 1))
	}
	query, args = aiIDsClause(aiFailureBase, ids, " ORDER BY producer,message_id LIMIT 1025 FOR UPDATE")
	args = append([]any{source.RequestID}, args...)
	failures, err := read("failures", query, args...)
	if err != nil {
		return nil, nil, err
	}
	// Quarantine has no trusted org/command ownership. Do not hide a possibly
	// relevant row behind an org filter or an untrusted claimed inner identity.
	quarantine, err := read("quarantine", "SELECT CAST(wire_sha256 AS BINARY),CAST(code AS BINARY),CAST(attempts AS BINARY) FROM ai_messaging_quarantine ORDER BY wire_sha256 LIMIT 1 FOR UPDATE")
	if err != nil {
		return nil, nil, err
	}
	if len(quarantine) != 0 {
		return nil, nil, ErrAILocalUnknown
	}
	if err := aiVerifyMQGraph(ops, boxes, inbox, failures, events, request, projection, currentSources, expectedBodyHashes); err != nil {
		return nil, nil, err
	}
	gaps := []string{"qs_ai_all_historical_runs_orphans_and_mq_closure_unknown", "global_reverse_message_body_association_not_audited", "outgoing_protected_header_not_reauthenticated", "inbox_original_authenticated_wire_not_retained"}
	if len(ops) == 0 {
		gaps = append(gaps, "historical_original_command_receipt_not_retained")
	}
	return s, gaps, nil
}

func aiVerifyRequest(row [][]byte, source *DecodedAICommand) (*app.Start, *app.Event, error) {
	if aiPayloadShape(row[2], "start") != nil {
		return nil, nil, ErrAILocalRelation
	}
	var request app.Start
	if json.Unmarshal(row[2], &request) != nil || request.RequestID != source.RequestID || request.Actor.OrgID != source.OrganizationID || request.Actor.SubjectID != source.SubjectID {
		return nil, nil, ErrAILocalRelation
	}
	raw, err := json.Marshal(request)
	if err != nil || sourceSHA(raw) != aiText(row, 1) {
		return nil, nil, ErrAILocalRelation
	}
	if _, err = aiDecodeBusiness("start", request.RequestID, request.RequestID, row[2], aiText(row, 1)); err != nil {
		return nil, nil, ErrAILocalRelation
	}
	for _, pair := range []struct {
		index int
		value string
	}{{7, request.Actor.OrgID}, {8, request.Actor.SubjectID}, {9, request.TesteeID}} {
		if row[pair.index] != nil && aiText(row, pair.index) != pair.value {
			return nil, nil, ErrAILocalRelation
		}
	}
	var projection app.Event
	var projectionFields map[string]json.RawMessage
	if strictJSON(row[6]) != nil || json.Unmarshal(row[6], &projectionFields) != nil || !aiExactKeys(projectionFields, []string{"event_id", "request_id", "session_id", "actor", "testee_id", "version", "status", "question_id", "question", "can_skip", "failure_code"}, []string{"artifact_json"}) || json.Unmarshal(row[6], &projection) != nil || app.ValidateEvent(projection) != nil || projection.RequestID != request.RequestID || projection.Actor != request.Actor || projection.TesteeID != request.TesteeID || projection.SessionID != aiText(row, 3) || !aiOriginalUUID(projection.SessionID) || strconv.FormatInt(projection.Version, 10) != aiText(row, 4) || projection.Status != aiText(row, 5) || (projection.Status != "completed" && projection.Status != "cancelled") {
		return nil, nil, ErrAILocalTerminal
	}
	if err := aiVerifyArtifactRequest(projection, &request); err != nil {
		return nil, nil, err
	}
	if source.SourceKind == "start" {
		if source.WriterPayloadDigest.SHA256 != aiText(row, 1) {
			return nil, nil, ErrAILocalRelation
		}
	} else if source.ResourceID != projection.SessionID {
		return nil, nil, ErrAILocalRelation
	}
	return &request, &projection, nil
}

func aiCurrentSource(row [][]byte, table string, columns SQLColumns, boundary SourceBoundary) (*DecodedAICommand, error) {
	if boundary.Name != table || len(row) != len(columns) {
		return nil, ErrAILocalChanged
	}
	var file bytes.Buffer
	if json.NewEncoder(&file).Encode(sqlSourceHeader{Protocol: SQLSourceProtocol, Columns: columns, Boundary: boundary}) != nil {
		return nil, ErrAILocalRead
	}
	encoded := make([]*string, len(row))
	h := sha256.New()
	sourceFrame(h, []byte(aiJSONHash(columns)), false)
	var size uint64
	for i, cell := range row {
		sourceFrame(h, cell, cell == nil)
		size += uint64(len(cell))
		if cell != nil {
			encoded[i] = ptrString(base64.StdEncoding.EncodeToString(cell))
		}
	}
	if json.NewEncoder(&file).Encode(encoded) != nil {
		return nil, ErrAILocalRead
	}
	expect := SourceCopyExpectation{Boundary: boundary, Records: 1, Bytes: size, DataHash: hex.EncodeToString(h.Sum(nil))}
	r, err := NewAISQLSourceReader(&file, expect)
	if err != nil {
		return nil, ErrAILocalChanged
	}
	value, err := r.Next()
	if err != nil {
		return nil, ErrAILocalChanged
	}
	if _, err = r.Next(); err != io.EOF || !r.Receipt().Complete {
		return nil, ErrAILocalChanged
	}
	return value, nil
}

func aiVerifyArtifactRequest(event app.Event, request *app.Start) error {
	artifact, err := app.ValidateArtifact(event)
	if err != nil {
		return ErrAILocalTerminal
	}
	if artifact == nil {
		return nil
	}
	if len(request.Evidence) != 1 || len(request.AssessmentIDs) != 1 {
		return ErrAILocalRelation
	}
	original := request.Evidence[0]
	if artifact.AssessmentID != request.AssessmentIDs[0] || artifact.AssessmentID != original.AssessmentID || artifact.ReportID != original.ReportID || artifact.SourceVersion != original.SourceVersion {
		return ErrAILocalRelation
	}
	return nil
}

func (r *AILocalResolver) verifySourceRows(bridges, legacyRows [][][]byte, source, legacySource *DecodedAICommand, request *app.Start, createdAt []byte) (map[string]*DecodedAICommand, map[string]string, error) {
	byID := map[string]*DecodedAICommand{}
	mapped := map[string]*DecodedAICommand{}
	expectedHashes := map[string]string{}
	for _, row := range bridges {
		if len(row) != 8 {
			return nil, nil, ErrAILocalChanged
		}
		value, err := aiCurrentSource(row, AIBridgeCommandSource, r.binding.BridgeColumns, r.binding.BridgeBoundary)
		if err != nil {
			return nil, nil, err
		}
		if value.RequestID != source.RequestID || value.OrganizationID != request.Actor.OrgID || value.SubjectID != request.Actor.SubjectID || byID[value.CommandID] != nil {
			return nil, nil, ErrAILocalRelation
		}
		byID[value.CommandID] = value
		hash, err := aiExpectedCommandHash(row[3], value.SourceKind)
		if err != nil {
			return nil, nil, err
		}
		expectedHashes[value.CommandID] = hash
	}
	actual := byID[source.CommandID]
	if actual == nil || !reflect.DeepEqual(actual, source) {
		return nil, nil, ErrAILocalChanged
	}
	for _, row := range legacyRows {
		if len(row) != 10 {
			return nil, nil, ErrAILocalChanged
		}
		value, err := aiCurrentSource(row, AILegacyCommandSource, r.binding.LegacyColumns, r.binding.LegacyBoundary)
		if err != nil {
			return nil, nil, err
		}
		if ValidateAISourcePair(byID[value.CommandID], value) != nil || mapped[value.CommandID] != nil || value.Transport.MessagingBodySHA256 != expectedHashes[value.CommandID] {
			return nil, nil, ErrAILocalRelation
		}
		mapped[value.CommandID] = value
		if value.SourceKind == "start" {
			stamp := ""
			if createdAt != nil {
				at, err := time.Parse("2006-01-02 15:04:05.000000", string(createdAt))
				if err != nil {
					return nil, nil, ErrAILocalRelation
				}
				stamp = at.In(time.FixedZone("UTC+8", 28800)).Format(time.RFC3339Nano)
			}
			if value.Transport.OriginalOccurredAt != stamp {
				return nil, nil, ErrAILocalRelation
			}
		}
	}
	if (legacySource == nil) != (mapped[source.CommandID] == nil) || (legacySource != nil && !reflect.DeepEqual(mapped[source.CommandID], legacySource)) {
		return nil, nil, ErrAILocalChanged
	}
	for id, value := range byID {
		if !*value.Transport.Delivered && mapped[id] == nil {
			return nil, nil, ErrAILocalResponsibility
		}
	}
	for id, value := range mapped {
		byID[id] = value
	}
	return byID, expectedHashes, nil
}

// This is the exact retained writer mapping (messaging_commands.go), applied
// only to strictly decoded original source bytes. It never prepares a wire,
// creates an ID or stages a command; the transient payload is discarded.
func aiExpectedCommandHash(raw []byte, kind string) (string, error) {
	var body *pb.MessagingBody
	switch kind {
	case "start":
		var value app.Start
		if json.Unmarshal(raw, &value) != nil {
			return "", ErrAILocalRelation
		}
		start := &pb.StartCommand{RequestId: value.RequestID, Actor: &pb.Actor{OrgId: value.Actor.OrgID, SubjectId: value.Actor.SubjectID}, TesteeId: value.TesteeID, AssessmentIds: value.AssessmentIDs, Goal: value.Goal}
		for _, entry := range value.Evidence {
			item := &pb.EvidenceItem{AssessmentId: entry.AssessmentID, TesteeId: entry.TesteeID, ReportId: entry.ReportID, SourceVersion: entry.SourceVersion}
			for _, fact := range entry.Facts {
				item.Facts = append(item.Facts, &pb.Fact{Ref: fact.Ref, Value: fact.Value})
			}
			start.Evidence = append(start.Evidence, item)
		}
		body = &pb.MessagingBody{Value: &pb.MessagingBody_Start{Start: start}}
	case "answer", "cancel":
		var value app.Change
		if json.Unmarshal(raw, &value) != nil {
			return "", ErrAILocalRelation
		}
		body = &pb.MessagingBody{Value: &pb.MessagingBody_Change{Change: &pb.ChangeCommand{CommandId: value.CommandID, SessionId: value.SessionID, Actor: &pb.Actor{OrgId: value.Actor.OrgID, SubjectId: value.Actor.SubjectID}, Action: value.Action, ExpectedVersion: value.ExpectedVersion, QuestionId: value.QuestionID, Answer: value.Answer, Skip: value.Skip}}}
	default:
		return "", ErrAILocalRelation
	}
	encoded, err := proto.MarshalOptions{Deterministic: true}.Marshal(body)
	if err != nil {
		return "", ErrAILocalRelation
	}
	return sourceSHA(encoded), nil
}

func aiVerifyAggregate(rows, ops [][][]byte, requestID string) error {
	if len(rows) == 0 {
		if len(ops) == 0 {
			return nil
		}
		return ErrAILocalRelation
	}
	if len(rows) != 1 || len(rows[0]) != 2 || aiText(rows[0], 0) != requestID {
		return ErrAILocalRelation
	}
	next, valid := aiUint(rows[0][1])
	if !valid || next != uint64(len(ops))+1 {
		return ErrAILocalRelation
	}
	seen := map[uint64]bool{}
	for _, row := range ops {
		if len(row) != 17 {
			return ErrAILocalRelation
		}
		sequence, valid := aiUint(row[7])
		if !valid || sequence == 0 || sequence >= next || seen[sequence] {
			return ErrAILocalRelation
		}
		seen[sequence] = true
	}
	return nil
}

// Outbound wire is recipient encrypted. We verify its original outer identity
// and persisted byte hash; this is explicitly not a new JOSE authentication.
func aiCheckStoredWire(wire []byte, id, wireHash string) bool {
	if len(wire) == 0 || len(wire) > protected.NSQMaxBytes || sourceSHA(wire) != wireHash {
		return false
	}
	var header struct {
		Revision int `json:"schema_revision"`
	}
	if strictJSON(wire) != nil || json.Unmarshal(wire, &header) != nil || header.Revision != int(legacy.Revision2) {
		return false
	}
	outer, recognized, err := legacy.Decode(wire)
	if err != nil || !recognized || outer.UUID != id || !reflect.DeepEqual(outer.Metadata, map[string]string{"secure_profile": protected.Profile}) {
		return false
	}
	parts := strings.Split(string(outer.Payload), ".")
	if len(parts) != 5 {
		return false
	}
	protectedHeader, err := base64.RawURLEncoding.Strict().DecodeString(parts[0])
	if err != nil || strictJSON(protectedHeader) != nil {
		return false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(protectedHeader, &fields) != nil || !aiExactKeys(fields, []string{"alg", "enc", "kid", "typ", "cty", "epk"}, nil) {
		return false
	}
	for key, want := range map[string]string{"alg": string(jose.ECDH_ES_A256KW), "enc": string(jose.A256GCM), "typ": protected.Profile, "cty": "JWS"} {
		var value string
		if json.Unmarshal(fields[key], &value) != nil || value != want {
			return false
		}
	}
	var kid string
	if json.Unmarshal(fields["kid"], &kid) != nil || kid == "" {
		return false
	}
	// Structural parsing only. The recipient key and original protected
	// signature are unavailable here; no Decrypt/Open is called or claimed.
	_, err = jose.ParseEncryptedCompact(string(outer.Payload), []jose.KeyAlgorithm{jose.ECDH_ES_A256KW}, []jose.ContentEncryption{jose.A256GCM})
	return err == nil
}

func aiStoredBody(kind pb.MessagingKind, id, aggregate, correlation, hash string, raw []byte) (*pb.MessagingBody, error) {
	producer, destination, _, err := app.MessagingRoute(kind)
	if err != nil {
		return nil, ErrAILocalRelation
	}
	envelope := &pb.MessagingEnvelope{SchemaVersion: app.MessagingSchema, Producer: producer, Destination: destination, MessageId: id, Kind: kind, AggregateKey: aggregate, CorrelationCommandId: correlation, BodySha256: hash, BodyLength: uint64(len(raw)), Body: &pb.MessagingEnvelope_InlineBody{InlineBody: raw}}
	body, err := app.ParseMessagingBody(envelope, raw)
	if err != nil {
		return nil, ErrAILocalRelation
	}
	return body, nil
}

func aiVerifyMQGraph(ops, boxes, inbox, failures, events [][][]byte, request *app.Start, projection *app.Event, sources map[string]*DecodedAICommand, expectedBodyHashes map[string]string) error {
	operations := map[string][][]byte{}
	messages := map[string][][]byte{}
	received := map[string][][]byte{}
	for _, row := range ops {
		if len(row) != 17 || !aiOriginalUUID(aiText(row, 0)) || operations[aiText(row, 0)] != nil || aiText(row, 3) != request.Actor.OrgID || aiText(row, 6) != request.RequestID {
			return ErrAILocalRelation
		}
		kind, valid := aiKind(row[1])
		if !valid || (kind != pb.MessagingKind_START && kind != pb.MessagingKind_CHANGE && kind != pb.MessagingKind_PARTICIPANT_RETRY) {
			return ErrAILocalRelation
		}
		if (kind == pb.MessagingKind_START && (aiText(row, 5) != request.RequestID || aiText(row, 4) != request.Actor.SubjectID)) || (kind != pb.MessagingKind_START && aiText(row, 5) != projection.SessionID) || (kind == pb.MessagingKind_CHANGE && aiText(row, 4) != request.Actor.SubjectID) {
			return ErrAILocalRelation
		}
		// Existing retirement flags are not imported proof. Their raw metadata
		// participates in the baseline, but this point resolver refuses to
		// reinterpret a prior historical conclusion as a live receipt.
		if aiText(row, 14) != "0" {
			return ErrAILocalUnknown
		}
		if !evidenceHash(aiText(row, 2)) || aiText(row, 8) != "accepted" && aiText(row, 8) != "rejected" || row[10] == nil || row[11] == nil || row[12] == nil || row[13] == nil || row[16] != nil {
			return ErrAILocalResponsibility
		}
		operations[aiText(row, 0)] = row
	}
	for _, row := range boxes {
		if len(row) != 21 || aiText(row, 0) != "qs-server" || aiText(row, 1) != "qs-ai" || aiText(row, 8) != request.Actor.OrgID || aiText(row, 10) != request.RequestID || messages[aiText(row, 2)] != nil {
			return ErrAILocalRelation
		}
		kind, valid := aiKind(row[7])
		producer, destination, topic, routeErr := app.MessagingRoute(kind)
		if !valid || routeErr != nil || producer != aiText(row, 0) || destination != aiText(row, 1) || topic != aiText(row, 9) || !aiCheckStoredWire(row[5], aiText(row, 2), aiText(row, 6)) || sourceSHA(row[4]) != aiText(row, 3) {
			return ErrAILocalRelation
		}
		if aiText(row, 14) != "confirmed" || row[19] == nil {
			return ErrAILocalResponsibility
		}
		messages[aiText(row, 2)] = row
		if kind == pb.MessagingKind_EVENT_ACKNOWLEDGEMENT {
			if aiText(row, 12) != "0" || aiText(row, 13) != "0" || aiText(row, 11) != "1" {
				return ErrAILocalRelation
			}
		} else {
			op := operations[aiText(row, 2)]
			if op == nil || aiText(row, 12) != "1" || aiText(row, 13) != "1" || aiText(row, 3) != aiText(op, 2) || aiText(row, 7) != aiText(op, 1) || aiText(row, 11) != aiText(op, 7) {
				return ErrAILocalRelation
			}
			body, err := aiStoredBody(kind, aiText(row, 2), request.RequestID, "", aiText(row, 3), row[4])
			if err != nil {
				return err
			}
			if err := aiCheckCommandBody(body, kind, op, request, projection); err != nil {
				return err
			}
			if original := sources[aiText(row, 2)]; original != nil && original.Source.Object == AILegacyCommandSource {
				floor := uint64(original.Transport.HandoffBudgetFloor)
				count, valid := aiUint(row[15])
				if !valid || count < floor || count > 8 || aiText(row, 3) != original.Transport.MessagingBodySHA256 || aiText(row, 3) != expectedBodyHashes[original.CommandID] {
					return ErrAILocalRelation
				}
			}
		}
	}
	for id, op := range operations {
		if messages[id] == nil {
			return ErrAILocalRelation
		}
		if !aiOriginalUUID(aiText(op, 10)) {
			return ErrAILocalRelation
		}
	}
	for id, original := range sources {
		if original.Source.Object == AILegacyCommandSource && operations[id] == nil {
			return ErrAILocalRelation
		}
		if original.Source.Object == AIBridgeCommandSource && operations[id] != nil {
			return ErrAILocalRelation
		}
	}
	for _, row := range inbox {
		if len(row) != 10 || aiText(row, 0) != "qs-ai" || aiText(row, 6) != request.RequestID || !aiOriginalUUID(aiText(row, 1)) || !aiOriginalUUID(aiText(row, 7)) || !evidenceHash(aiText(row, 4)) || row[8] == nil || received[aiText(row, 1)] != nil {
			return ErrAILocalRelation
		}
		if aiText(row, 9) != "stored" {
			return ErrAILocalResponsibility
		}
		kind, valid := aiKind(row[5])
		if !valid || (kind != pb.MessagingKind_COMMAND_RECEIPT && kind != pb.MessagingKind_INTERPRETATION_STATE) {
			return ErrAILocalRelation
		}
		var rawBody pb.MessagingBody
		if proto.Unmarshal(row[3], &rawBody) != nil {
			return ErrAILocalRelation
		}
		correlation := ""
		if kind == pb.MessagingKind_COMMAND_RECEIPT {
			if rawBody.GetCommandReceipt() == nil {
				return ErrAILocalRelation
			}
			correlation = rawBody.GetCommandReceipt().CommandId
		}
		body, err := aiStoredBody(kind, aiText(row, 1), request.RequestID, correlation, aiText(row, 2), row[3])
		if err != nil {
			return err
		}
		if kind == pb.MessagingKind_COMMAND_RECEIPT {
			receipt := body.GetCommandReceipt()
			op := operations[receipt.CommandId]
			if op == nil || receipt.CommandBodySha256 != aiText(op, 2) || aiText(op, 10) != aiText(row, 1) || !bytes.Equal(row[3], op[11]) {
				return ErrAILocalRelation
			}
			if receipt.Decision == pb.MessagingDecision_HELD {
				return ErrAILocalResponsibility
			}
			decision := "accepted"
			if receipt.Decision == pb.MessagingDecision_REJECTED {
				decision = "rejected"
			}
			if decision != aiText(op, 8) || receipt.Code != aiText(op, 9) {
				return ErrAILocalRelation
			}
			if receipt.Decision == pb.MessagingDecision_ACCEPTED {
				workflow := receipt.GetWorkflowReceipt()
				if workflow == nil || workflow.SessionId != projection.SessionID || !aiOriginalUUID(workflow.RunId) || workflow.Version < 1 || workflow.Status == "" {
					return ErrAILocalRelation
				}
			}
		} else {
			value := body.GetInterpretationState()
			event := app.Event{EventID: value.EventId, RequestID: value.RequestId, SessionID: value.SessionId, Actor: app.Actor{OrgID: value.GetActor().GetOrgId(), SubjectID: value.GetActor().GetSubjectId()}, TesteeID: value.TesteeId, Version: value.Version, Status: value.Status, QuestionID: value.QuestionId, Question: value.Question, CanSkip: value.CanSkip, FailureCode: value.FailureCode, ArtifactJSON: value.ArtifactJson}
			if app.ValidateEvent(event) != nil || event.RequestID != request.RequestID || event.SessionID != projection.SessionID || event.Actor != request.Actor || event.TesteeID != request.TesteeID || event.Version > projection.Version {
				return ErrAILocalRelation
			}
			if err := aiVerifyArtifactRequest(event, request); err != nil {
				return err
			}
			encoded, err := json.Marshal(event)
			if err != nil {
				return ErrAILocalRelation
			}
			found := false
			for _, fact := range events {
				if len(fact) == 4 && aiText(fact, 0) == event.EventID && aiText(fact, 1) == event.RequestID && aiText(fact, 2) == strconv.FormatInt(event.Version, 10) && aiText(fact, 3) == sourceSHA(encoded) {
					found = true
				}
			}
			if !found {
				return ErrAILocalRelation
			}
		}
		ack := messages[aiText(row, 7)]
		if ack == nil {
			return ErrAILocalRelation
		}
		ackBody, err := aiStoredBody(pb.MessagingKind_EVENT_ACKNOWLEDGEMENT, aiText(row, 7), request.RequestID, "", aiText(ack, 3), ack[4])
		if err != nil {
			return err
		}
		proof := ackBody.GetEventAcknowledgement()
		if proof.EventId != aiText(row, 1) || proof.EventBodySha256 != aiText(row, 2) || proof.EventKind != kind || proof.Outcome != pb.MessagingEventAcknowledgement_STORED {
			return ErrAILocalRelation
		}
		received[aiText(row, 1)] = row
	}
	for _, op := range operations {
		if received[aiText(op, 10)] == nil {
			return ErrAILocalRelation
		}
	}
	for _, box := range messages {
		kind, _ := aiKind(box[7])
		if kind == pb.MessagingKind_EVENT_ACKNOWLEDGEMENT {
			body, err := aiStoredBody(kind, aiText(box, 2), request.RequestID, "", aiText(box, 3), box[4])
			if err != nil {
				return err
			}
			entry := received[body.GetEventAcknowledgement().EventId]
			if entry == nil || aiText(entry, 7) != aiText(box, 2) {
				return ErrAILocalRelation
			}
		}
	}
	for _, row := range failures {
		if len(row) != 9 || aiText(row, 0) != "qs-ai" || aiText(row, 4) != request.RequestID {
			return ErrAILocalUnknown
		}
		entry := received[aiText(row, 1)]
		count, valid := aiUint(row[6])
		if entry == nil {
			return ErrAILocalResponsibility
		}
		if !valid || count < 1 || count > 8 || aiText(row, 2) != aiText(entry, 2) || aiText(row, 3) != aiText(entry, 5) || !aiCheckStoredWire(row[5], aiText(entry, 1), aiText(entry, 4)) {
			return ErrAILocalRelation
		}
	}
	return nil
}

func aiCheckCommandBody(body *pb.MessagingBody, kind pb.MessagingKind, op [][]byte, request *app.Start, projection *app.Event) error {
	switch kind {
	case pb.MessagingKind_START:
		value := body.GetStart()
		if value.RequestId != request.RequestID || value.GetActor().GetOrgId() != request.Actor.OrgID || value.GetActor().GetSubjectId() != request.Actor.SubjectID || value.TesteeId != request.TesteeID || !reflect.DeepEqual(value.AssessmentIds, request.AssessmentIDs) || value.Goal != request.Goal {
			return ErrAILocalRelation
		}
	case pb.MessagingKind_CHANGE:
		value := body.GetChange()
		if value.CommandId != aiText(op, 0) || value.SessionId != projection.SessionID || value.GetActor().GetOrgId() != request.Actor.OrgID || value.GetActor().GetSubjectId() != request.Actor.SubjectID || (value.Action != "answer" && value.Action != "cancel") || value.ExpectedVersion < 1 {
			return ErrAILocalRelation
		}
	case pb.MessagingKind_PARTICIPANT_RETRY:
		value := body.GetParticipantRetry()
		if value.CommandId != aiText(op, 0) || value.SessionId != projection.SessionID || strconv.FormatInt(value.GetScope().GetOrganizationId(), 10) != request.Actor.OrgID || strconv.FormatInt(value.GetScope().GetOperatorUserId(), 10) != aiText(op, 4) || value.ExpectedProviderInvocations != 1 || !aiOriginalUUID(value.ExpectedRunId) {
			return ErrAILocalRelation
		}
	default:
		return ErrAILocalRelation
	}
	return nil
}
