package retirement

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	"go.mongodb.org/mongo-driver/mongo"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"gorm.io/gorm"
)

const (
	ErrAIReverseBinding SourceError = "ai_reverse_binding_rejected"
	ErrAIReverseRead    SourceError = "ai_reverse_read_failed"
	ErrAIReverseSchema  SourceError = "ai_reverse_schema_unsupported"
	ErrAIReverseBounds  SourceError = "ai_reverse_budget_exceeded"
	ErrAIReverseChanged SourceError = "ai_reverse_fresh_snapshot_changed"
	ErrAIReverseFresh   SourceError = "ai_reverse_new_actual_snapshot_required"
)

// These limits abort rather than truncate. RetainedBytes is a conservative
// logical allocation reservation, not an RSS or production-runtime guarantee.
type AIReverseLimits struct {
	PageRows                            int
	MaxRows, MaxBytes, MaxRetainedBytes uint64
	MaxDuration                         time.Duration
}

func DefaultAIReverseLimits() AIReverseLimits {
	return AIReverseLimits{256, 2_000_000, 8 << 30, 1 << 30, 900 * time.Second}
}
func (l AIReverseLimits) valid() bool {
	return l.PageRows > 0 && l.PageRows <= 4096 && l.MaxRows > 0 && l.MaxRows <= 10_000_000 && l.MaxBytes > 0 && l.MaxBytes <= 32<<30 && l.MaxRetainedBytes > 0 && l.MaxRetainedBytes <= 4<<30 && l.MaxDuration > 0 && l.MaxDuration <= 1500*time.Second
}

type AIReverseLedgerSummary struct {
	Store                                                   string
	Rows, Bytes, Pages                                      uint64
	SchemaSHA256, PrimaryKeySHA256, UpperSHA256, RowsSHA256 string
}
type AIReverseSummary struct {
	Version, EpochID, ResponsibilityCycleID, DatabaseIdentitySHA256                                                                          string
	MigrationVersion                                                                                                                         uint64
	StartedAt, CompletedAt                                                                                                                   time.Time
	Ledgers                                                                                                                                  []AIReverseLedgerSummary
	Rows, Bytes, RetainedBudgetBytes, SourceScopeRetainedBudgetBytes, Related, OutsideRetirement, Unknown, Blocking, OutsideActive           uint64
	DataSHA256, BusinessAnchorsSHA256, SourceScopeSHA256                                                                                     string
	BlockingReasons                                                                                                                          []string
	SourceCopies                                                                                                                             [4]SourceCopyReceipt
	ActualReadOnlyRR, WholeLedgerEOF                                                                                                         bool
	SourceAuthenticationRequired, ExternalOriginRequired, ExternalQSAIClosureRequired, StoredWireAuthenticationRequired, WriterFenceRequired bool
	UnboundOrphanNegativeClosureRequired, NewOwnerOrganizationNegativeClosureRequired                                                        bool
	GlobalReverseQualified, CASAuthority, DropReady                                                                                          bool
}
type AIReverseObservation struct {
	Store, PrimaryKeySHA256, RowSHA256, Scope string
	Invalid, Unfinished, Held                 bool
	Reasons                                   []string
}

func (AIReverseObservation) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (AIReverseObservation) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (AIReverseObservation) String() string               { return "private body-free AI reverse observation" }
func (v AIReverseObservation) GoString() string           { return v.String() }

type aiReverseSpec struct {
	table   string
	keys    []string
	columns []string
	numeric map[string]bool
}

func aiReverseColumns(s string) []string { return strings.Fields(s) }

// The complete ordinal physical layouts are migrations 72,83,91-95,97, at
// clean head99. Evaluation governance runs belong to qs-ai, not to the host's
// assessment runtime_checkpoint. No JOIN or organization predicate hides rows.
var aiReverseSpecs = []aiReverseSpec{
	{"ai_bridge_requests", []string{"request_id"}, aiReverseColumns("request_id request_hash payload session_id version status projection organization_id subject_id testee_id created_at updated_at"), nil},
	{"ai_bridge_commands", []string{"command_id"}, aiReverseColumns("command_id request_id kind payload payload_hash delivered attempts available_at"), nil},
	{"ai_bridge_events", []string{"event_id"}, aiReverseColumns("event_id request_id version payload_hash"), nil},
	{"ai_bridge_request_assessments", []string{"request_id", "assessment_id"}, aiReverseColumns("request_id assessment_id"), map[string]bool{"assessment_id": true}},
	{"ai_messaging_legacy_commands", []string{"command_id"}, aiReverseColumns("command_id request_id source_kind source_payload source_payload_hash source_attempts source_available_at source_original_time messaging_body_sha256 transferred_at"), nil},
	{"ai_messaging_operations", []string{"command_id"}, aiReverseColumns("command_id kind body_sha256 organization_id subject_id resource_id aggregate_key aggregate_sequence decision code receipt_id receipt created_at decided_at retired retirement_evidence retired_at"), nil},
	{"ai_messaging_outbox", []string{"producer", "destination", "message_id"}, aiReverseColumns("producer destination message_id body_sha256 body wire wire_sha256 kind organization_id topic aggregate_key aggregate_sequence ordered requires_receipt stage attempts available_at created_at published_at confirmed_at error_code"), nil},
	{"ai_messaging_inbox", []string{"producer", "message_id"}, aiReverseColumns("producer message_id body_sha256 body wire_sha256 kind aggregate_key ack_id received_at outcome"), nil},
	{"ai_messaging_failures", []string{"producer", "message_id"}, aiReverseColumns("producer message_id body_sha256 kind aggregate_key wire attempts first_seen_at last_seen_at"), nil},
	{"ai_messaging_aggregates", []string{"aggregate_key"}, aiReverseColumns("aggregate_key next_sequence"), nil},
	{"ai_messaging_evaluation_states", []string{"run_id"}, aiReverseColumns("run_id organization_id event_sequence version state updated_at"), nil},
	{"ai_messaging_quarantine", []string{"wire_sha256"}, aiReverseColumns("wire_sha256 wire code attempts first_seen_at last_seen_at"), nil},
	{"ai_messaging_admission", []string{"singleton"}, aiReverseColumns("singleton closed revision updated_at"), map[string]bool{"singleton": true}},
	{"ai_messaging_observations", []string{"kind"}, aiReverseColumns("kind recorded_count recording_since last_observed_at"), nil},
}
var aiReverseObservationKinds = map[string]bool{"duplicate_event": true, "payload_fetch_unavailable": true, "payload_fetch_reference_mismatch": true, "payload_fetch_workload_denied": true, "payload_serve_reference_mismatch": true, "payload_serve_workload_denied": true, "payload_serve_storage_unavailable": true}

type aiReverseRow map[string][]byte

func (r aiReverseRow) text(n string) string { return string(r[n]) }
func aiReverseHash(parts ...string) string  { return aiFramedParts(parts...) }
func aiReverseRowSHA(columns []string, row aiReverseRow) string {
	h := sha256.New()
	sourceFrame(h, []byte("ai-reverse-native-row/v1"), false)
	for _, col := range columns {
		sourceFrame(h, []byte(col), false)
		sourceFrame(h, row[col], row[col] == nil)
	}
	return hex.EncodeToString(h.Sum(nil))
}
func aiReverseKeySHA(key []string) string {
	return aiReverseHash(append([]string{"ai-reverse-native-pk/v1"}, key...)...)
}
func aiReverseUint(r aiReverseRow, n string, positive bool) (uint64, error) {
	v, ok := aiUint(r[n])
	if !ok || positive && v == 0 {
		return 0, ErrAIReverseBinding
	}
	return v, nil
}
func aiReverseBool(r aiReverseRow, n string) (bool, error) {
	if r.text(n) != "0" && r.text(n) != "1" {
		return false, ErrAIReverseBinding
	}
	return r.text(n) == "1", nil
}
func aiReverseClock(raw []byte, optional bool) bool {
	return optional && raw == nil || aiSQLMicrosecondClock(string(raw))
}
func aiReverseASCII(v string) bool {
	if v == "" || len(v) > 192 || strings.TrimSpace(v) != v {
		return false
	}
	for _, c := range []byte(v) {
		if c < 33 || c > 126 {
			return false
		}
	}
	return true
}
func aiReverseKey(spec aiReverseSpec, r aiReverseRow) ([]string, error) {
	out := make([]string, len(spec.keys))
	for i, n := range spec.keys {
		if r[n] == nil {
			return nil, ErrAIReverseSchema
		}
		out[i] = r.text(n)
		if spec.numeric[n] {
			if _, e := aiReverseUint(r, n, true); e != nil {
				return nil, e
			}
		} else if !aiReverseASCII(out[i]) {
			return nil, ErrAIReverseBinding
		}
	}
	return out, nil
}
func aiReverseCompare(spec aiReverseSpec, a, b []string) int {
	for i, n := range spec.keys {
		if spec.numeric[n] {
			x, _ := strconv.ParseUint(a[i], 10, 64)
			y, _ := strconv.ParseUint(b[i], 10, 64)
			if x < y {
				return -1
			}
			if x > y {
				return 1
			}
		} else if v := strings.Compare(a[i], b[i]); v != 0 {
			return v
		}
	}
	return 0
}

// Expanded lexicographic PK predicates retain native PRIMARY range access.
// PK schema permits only exact binary collations; row keys are printable ASCII
// without padding. Unknown Unicode/padded keys abort, never claim coverage.
func aiReversePredicate(spec aiReverseSpec, key []string, upper bool) (string, []any) {
	var terms []string
	var args []any
	for i, n := range spec.keys {
		var term []string
		for j := 0; j < i; j++ {
			term = append(term, "`"+spec.keys[j]+"`=?")
			args = append(args, aiReverseArg(spec, spec.keys[j], key[j]))
		}
		op := ">"
		if upper {
			op = "<"
			if i == len(spec.keys)-1 {
				op = "<="
			}
		}
		term = append(term, "`"+n+"`"+op+"?")
		args = append(args, aiReverseArg(spec, n, key[i]))
		terms = append(terms, "("+strings.Join(term, " AND ")+")")
	}
	return "(" + strings.Join(terms, " OR ") + ")", args
}
func aiReverseArg(spec aiReverseSpec, n, v string) any {
	if spec.numeric[n] {
		x, _ := strconv.ParseUint(v, 10, 64)
		return x
	}
	return v
}

type aiReverseMetadata struct {
	columns       []string
	sourceColumns SQLColumns
	schema, pk    string
}
type aiReverseNode struct {
	inputPage                                                                                                            int
	observation                                                                                                          AIReverseObservation
	id, aggregate, request, resource, org, subject, testee, hash, bodyHash, wireHash                                     string
	command, receipt, receiptResource, receiptRun, ack, event, linkedHash, linkedWireHash, projectionHash, sourceRowHash string
	sequence, version, attempts                                                                                          uint64
	kind, eventKind                                                                                                      pb.MessagingKind
	decision                                                                                                             pb.MessagingDecision
	ackOutcome                                                                                                           pb.MessagingEventAcknowledgement_Outcome
	state                                                                                                                string
	retired, delivered, ordered, requiresReceipt, projectionAbsent                                                       bool
	assessments                                                                                                          []string
	payloadSHA, expectedBodyHash, sourceKind, sourceClock, originalClock, createdClock, receiptFamily                    string
}
type aiReverseAnchor struct{ id, org, testee, sheet, rawSHA string }
type AIReverseSnapshot struct {
	self                                               *AIReverseSnapshot
	snapshot                                           *SQLResponsibilitySnapshot
	pool                                               gorm.ConnPool
	head                                               uint64
	limits                                             AIReverseLimits
	started                                            time.Time
	metadata                                           []aiReverseMetadata
	upper                                              [][]string
	nodes                                              []*aiReverseNode
	byTable                                            map[string]map[string]*aiReverseNode
	anchors                                            map[string]aiReverseAnchor
	anchorMetadataSHA                                  string
	structuralReasons                                  []string
	report                                             AIReverseSummary
	scope                                              *aiReverseScope
	inputSink                                          aiHistoricalInputSink
	inputPage                                          int
	component                                          *HistoricalCASComponent
	componentSource                                    *HistoricalComponentSourceObservation
	componentPair                                      *AIHistoricalInputPair
	componentSeal                                      string
	componentComplete                                  bool
	componentAssessments, componentResources           map[string]bool
	historicalSources                                  *HistoricalSourceInputPair
	historicalPair                                     *AIHistoricalInputPair
	historicalNative, historicalSeal, historicalAccess string
}
type aiReverseScope struct {
	owner                       *HistoricalCoordinator
	auth                        *VerifiedSourceCopies
	receipts                    [4]SourceCopyReceipt
	relatedRequests, relatedIDs map[string]bool
	identityConflicts           map[string]bool
	missingCurrent              bool
	reservation                 uint64
	sha, typedFactsSHA          string
	verifiedEntries             uint64
}

func (*AIReverseSnapshot) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*AIReverseSnapshot) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*AIReverseSnapshot) String() string {
	return "private whole AI reverse snapshot; not closure or execution authority"
}
func (s *AIReverseSnapshot) GoString() string { return s.String() }
func (s *AIReverseSnapshot) Summary() AIReverseSummary {
	if s == nil || s.self != s {
		return AIReverseSummary{SourceAuthenticationRequired: true, ExternalOriginRequired: true, ExternalQSAIClosureRequired: true, StoredWireAuthenticationRequired: true, WriterFenceRequired: true}
	}
	r := s.report
	r.Ledgers = append([]AIReverseLedgerSummary(nil), r.Ledgers...)
	r.BlockingReasons = append([]string(nil), r.BlockingReasons...)
	r.ExternalOriginRequired, r.ExternalQSAIClosureRequired, r.StoredWireAuthenticationRequired, r.WriterFenceRequired = true, true, true, true
	r.GlobalReverseQualified, r.CASAuthority, r.DropReady = false, false, false
	return r
}
func (s *AIReverseSnapshot) ObservationsPage(offset uint64, limit int) ([]AIReverseObservation, uint64, error) {
	if s == nil || s.self != s || !s.report.WholeLedgerEOF || limit <= 0 || limit > 4096 || offset > uint64(len(s.nodes)) {
		return nil, 0, ErrAIReverseBinding
	}
	end := offset + uint64(limit)
	if end > uint64(len(s.nodes)) {
		end = uint64(len(s.nodes))
	}
	out := make([]AIReverseObservation, 0, end-offset)
	for i := offset; i < end; i++ {
		v := s.nodes[i].observation
		v.Reasons = append([]string(nil), v.Reasons...)
		out = append(out, v)
	}
	return out, end, nil
}
func (s *AIReverseSnapshot) alive(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil || time.Since(s.started) > s.limits.MaxDuration {
		return ErrAIReverseBounds
	}
	return nil
}
func (s *AIReverseSnapshot) read(ctx context.Context, q string, max int, args ...any) (out []aiReverseRow, names []string, size uint64, result error) {
	if e := s.alive(ctx); e != nil {
		return nil, nil, 0, e
	}
	qctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	rows, e := s.pool.QueryContext(qctx, q, args...)
	if e != nil {
		return nil, nil, 0, ErrAIReverseRead
	}
	defer func() {
		if rows.Close() != nil && result == nil {
			result = ErrAIReverseRead
		}
	}()
	names, e = rows.Columns()
	if e != nil || len(names) == 0 || len(names) > 128 {
		return nil, nil, 0, ErrAIReverseSchema
	}
	seen := map[string]bool{}
	for _, n := range names {
		if seen[n] {
			return nil, nil, 0, ErrAIReverseSchema
		}
		seen[n] = true
	}
	for rows.Next() {
		if len(out) >= max {
			return nil, nil, 0, ErrAIReverseBounds
		}
		raw := make([]sql.RawBytes, len(names))
		dest := make([]any, len(names))
		for i := range raw {
			dest[i] = &raw[i]
		}
		if rows.Scan(dest...) != nil {
			return nil, nil, 0, ErrAIReverseRead
		}
		r := aiReverseRow{}
		for i, n := range names {
			if uint64(len(raw[i])) > 64<<20-size {
				return nil, nil, 0, ErrAIReverseBounds
			}
			size += uint64(len(raw[i]))
			if raw[i] != nil {
				r[n] = append([]byte{}, raw[i]...)
			} else {
				r[n] = nil
			}
		}
		out = append(out, r)
		if e := s.alive(ctx); e != nil {
			return nil, nil, 0, e
		}
	}
	if rows.Err() != nil {
		return nil, nil, 0, ErrAIReverseRead
	}
	return out, names, size, nil
}
func (s *AIReverseSnapshot) identity(ctx context.Context) error {
	rows, _, _, e := s.read(ctx, "SELECT @@server_uuid AS server,DATABASE() AS db,VERSION() AS version", 1)
	if e != nil || len(rows) != 1 || !strings.HasPrefix(rows[0].text("version"), "8.") || aiReverseHash("mysql_database_identity_v1", rows[0].text("server"), rows[0].text("db")) != s.snapshot.Report().DatabaseIdentitySHA256 {
		return ErrAIReverseBinding
	}
	head, _, _, e := s.read(ctx, "SELECT CAST(version AS BINARY) AS version,CAST(dirty AS BINARY) AS dirty FROM schema_migrations", 2)
	if e != nil || len(head) != 1 || head[0].text("version") != strconv.FormatUint(s.head, 10) || head[0].text("dirty") != "0" {
		return ErrAIReverseBinding
	}
	return nil
}
func (s *AIReverseSnapshot) ValidateBorrowedSnapshot(ctx context.Context) error {
	if s == nil || s.self != s || s.snapshot == nil || s.pool == nil || !s.report.WholeLedgerEOF {
		return ErrAIReverseBinding
	}
	if e := s.alive(ctx); e != nil {
		return e
	}
	if s.snapshot.ValidateBorrowedSnapshot(ctx) != nil {
		return ErrAIReverseFresh
	}
	tx, e := hostmysql.RequireTx(ctx)
	if e != nil || tx == nil || tx.Statement == nil || tx.Statement.ConnPool != s.pool {
		return ErrAIReverseFresh
	}
	if _, e = sdkmysql.BindGORM(tx); e != nil {
		return ErrAIReverseFresh
	}
	return s.identity(ctx)
}

func PrepareAIReverseSnapshot(ctx context.Context, snapshot *SQLResponsibilitySnapshot, expectedMigration uint64, limits AIReverseLimits) (*AIReverseSnapshot, error) {
	return prepareAIReverseSnapshotWithInput(ctx, snapshot, expectedMigration, limits, nil)
}

func prepareAIReverseSnapshotWithInput(ctx context.Context, snapshot *SQLResponsibilitySnapshot, expectedMigration uint64, limits AIReverseLimits, sink aiHistoricalInputSink) (*AIReverseSnapshot, error) {
	if ctx == nil || snapshot == nil || snapshot.cycle == nil || expectedMigration != 99 || !limits.valid() {
		return nil, ErrAIReverseBinding
	}
	if snapshot.ValidateBorrowedSnapshot(ctx) != nil {
		return nil, ErrAIReverseFresh
	}
	tx, e := hostmysql.RequireTx(ctx)
	if e != nil || tx == nil || tx.Statement == nil || tx.Statement.ConnPool == nil {
		return nil, ErrAIReverseFresh
	}
	if _, e = sdkmysql.BindGORM(tx); e != nil {
		return nil, ErrAIReverseFresh
	}
	r := snapshot.Report()
	if r.CompletedAt.IsZero() || !r.ActualTransactionReadOnlyRR || len(r.Ledgers) != 8 {
		return nil, ErrAIReverseFresh
	}
	s := &AIReverseSnapshot{inputSink: sink, snapshot: snapshot, pool: tx.Statement.ConnPool, head: expectedMigration, limits: limits, started: time.Now(), byTable: map[string]map[string]*aiReverseNode{}, anchors: map[string]aiReverseAnchor{}}
	s.self = s
	var nonce [16]byte
	if _, e = rand.Read(nonce[:]); e != nil {
		return nil, ErrAIReverseRead
	}
	s.report = AIReverseSummary{Version: "ai-reverse-snapshot/v1", EpochID: hex.EncodeToString(nonce[:]), ResponsibilityCycleID: r.CycleID, DatabaseIdentitySHA256: r.DatabaseIdentitySHA256, MigrationVersion: expectedMigration, StartedAt: s.started.UTC(), SourceAuthenticationRequired: true, ExternalOriginRequired: true, ExternalQSAIClosureRequired: true, StoredWireAuthenticationRequired: true, WriterFenceRequired: true}
	if e = s.identity(ctx); e != nil {
		return nil, e
	}
	for _, spec := range aiReverseSpecs {
		if e = s.scan(ctx, spec); e != nil {
			return nil, e
		}
	}
	if e = s.readAssessmentAnchors(ctx); e != nil {
		return nil, e
	}
	s.reverse()
	s.classify(nil)
	for i, spec := range aiReverseSpecs {
		meta, err := s.schema(ctx, spec)
		if err != nil || !reflect.DeepEqual(meta, s.metadata[i]) {
			return nil, ErrAIReverseSchema
		}
	}
	if _, hash, err := s.assessmentMetadata(ctx); err != nil || hash != s.anchorMetadataSHA {
		return nil, ErrAIReverseSchema
	}
	if e = s.identity(ctx); e != nil {
		return nil, e
	}
	if snapshot.ValidateBorrowedSnapshot(ctx) != nil {
		return nil, ErrAIReverseFresh
	}
	s.report.ActualReadOnlyRR, s.report.WholeLedgerEOF = true, true
	s.report.CompletedAt = time.Now().UTC()
	s.report.DataSHA256 = s.dataDigest()
	return s, nil
}

// Actual metadata is part of the digest; unsupported columns/PK semantics fail
// before scanning. Defaults, nullable clocks, generation and every index are
// frozen. PK comparisons are never cast to expressions that disable PRIMARY.
func (s *AIReverseSnapshot) schema(ctx context.Context, spec aiReverseSpec) (aiReverseMetadata, error) {
	tables, _, _, e := s.read(ctx, "SELECT TABLE_TYPE AS kind,ENGINE AS engine,TABLE_COLLATION AS collation FROM information_schema.tables WHERE table_schema=DATABASE() AND BINARY table_name=BINARY ?", 2, spec.table)
	if e != nil || len(tables) != 1 || tables[0].text("kind") != "BASE TABLE" || tables[0].text("engine") != "InnoDB" {
		return aiReverseMetadata{}, ErrAIReverseSchema
	}
	cols, names, _, e := s.read(ctx, "SELECT COLUMN_NAME AS name,ORDINAL_POSITION AS ordinal,COLUMN_TYPE AS type,DATA_TYPE AS data_type,IS_NULLABLE AS nullable,COLUMN_DEFAULT AS `default`,EXTRA AS extra,CHARACTER_SET_NAME AS charset,COLLATION_NAME AS collation,GENERATION_EXPRESSION AS generation FROM information_schema.columns WHERE table_schema=DATABASE() AND BINARY table_name=BINARY ? ORDER BY ORDINAL_POSITION", 128, spec.table)
	if e != nil || len(cols) != len(spec.columns) {
		return aiReverseMetadata{}, ErrAIReverseSchema
	}
	h := sha256.New()
	sourceFrame(h, []byte("ai-reverse-schema/v1"), false)
	sourceFrame(h, []byte(aiReverseRowSHA([]string{"kind", "engine", "collation"}, tables[0])), false)
	meta := aiReverseMetadata{columns: append([]string(nil), spec.columns...)}
	byName := map[string]aiReverseRow{}
	for i, col := range cols {
		if col.text("name") != spec.columns[i] || col.text("ordinal") != strconv.Itoa(i+1) || col.text("generation") != "" || !aiReverseSupportedColumn(spec.table, col) {
			return meta, ErrAIReverseSchema
		}
		byName[col.text("name")] = col
		sourceFrame(h, []byte(aiReverseRowSHA(names, col)), false)
		meta.sourceColumns = append(meta.sourceColumns, []*string{ptrString(col.text("name")), ptrString(col.text("type")), ptrString(col.text("nullable")), aiReverseNullableString(col["default"]), ptrString(col.text("extra")), aiReverseNullableString(col["collation"])})
	}
	ddl, ddlNames, _, e := s.read(ctx, "SHOW CREATE TABLE `"+spec.table+"`", 1)
	if e != nil || len(ddl) != 1 || len(ddlNames) != 2 || ddl[0].text(ddlNames[0]) != spec.table {
		return meta, ErrAIReverseSchema
	}
	sourceFrame(h, []byte(aiReverseRowSHA(ddlNames, ddl[0])), false)
	indexes, indexNames, _, e := s.read(ctx, "SELECT INDEX_NAME AS name,NON_UNIQUE AS non_unique,SEQ_IN_INDEX AS ordinal,COLUMN_NAME AS col,SUB_PART AS sub_part,COLLATION AS direction,INDEX_TYPE AS type,IS_VISIBLE AS visible,EXPRESSION AS expression FROM information_schema.statistics WHERE table_schema=DATABASE() AND BINARY table_name=BINARY ? ORDER BY INDEX_NAME,SEQ_IN_INDEX", 256, spec.table)
	if e != nil {
		return meta, e
	}
	pk := sha256.New()
	found := 0
	for _, ix := range indexes {
		sourceFrame(h, []byte(aiReverseRowSHA(indexNames, ix)), false)
		if ix.text("name") != "PRIMARY" {
			continue
		}
		if found >= len(spec.keys) || ix.text("col") != spec.keys[found] || ix.text("ordinal") != strconv.Itoa(found+1) || ix.text("non_unique") != "0" || ix["sub_part"] != nil || ix.text("direction") != "A" || ix.text("type") != "BTREE" || ix.text("visible") != "YES" || ix["expression"] != nil {
			return meta, ErrAIReverseSchema
		}
		col := byName[spec.keys[found]]
		if col.text("nullable") != "NO" {
			return meta, ErrAIReverseSchema
		}
		if spec.numeric[spec.keys[found]] {
			if col.text("data_type") != "bigint" && col.text("data_type") != "tinyint" {
				return meta, ErrAIReverseSchema
			}
		} else if col.text("collation") != "ascii_bin" && col.text("collation") != "utf8mb4_bin" {
			return meta, ErrAIReverseSchema
		}
		sourceFrame(pk, []byte(aiReverseRowSHA(indexNames, ix)), false)
		found++
	}
	if found != len(spec.keys) {
		return meta, ErrAIReverseSchema
	}
	meta.schema = hex.EncodeToString(h.Sum(nil))
	meta.pk = hex.EncodeToString(pk.Sum(nil))
	return meta, nil
}
func aiReverseNullableString(raw []byte) *string {
	if raw == nil {
		return nil
	}
	return ptrString(string(raw))
}
func aiReverseSupportedColumn(table string, col aiReverseRow) bool {
	n, t := col.text("name"), col.text("type")
	nullable := col.text("nullable")
	if nullable != "YES" && nullable != "NO" {
		return false
	}
	optional := map[string]bool{}
	switch table {
	case "ai_bridge_requests":
		for _, key := range strings.Fields("session_id projection organization_id subject_id testee_id created_at updated_at") {
			optional[key] = true
		}
	case "ai_messaging_operations":
		for _, key := range strings.Fields("body_sha256 aggregate_sequence receipt_id receipt created_at decided_at retirement_evidence retired_at") {
			optional[key] = true
		}
	case "ai_messaging_outbox":
		optional["published_at"], optional["confirmed_at"] = true, true
	case "ai_messaging_observations":
		optional["last_observed_at"] = true
	}
	if (nullable == "YES") != optional[n] {
		return false
	}
	// Exact family/width declarations from the real additive AI migrations.
	want := map[string]string{"producer": "varchar(64)", "destination": "varchar(64)", "request_id": "char(36)", "command_id": "char(36)", "resource_id": "char(36)", "event_id": "char(36)", "run_id": "char(36)", "session_id": "char(36)", "ack_id": "char(36)", "receipt_id": "char(36)", "request_hash": "char(64)", "payload_hash": "char(64)", "body_sha256": "char(64)", "wire_sha256": "char(64)", "source_payload_hash": "char(64)", "messaging_body_sha256": "char(64)", "subject_id": "varchar(128)", "aggregate_key": "varchar(192)", "topic": "varchar(64)", "code": "varchar(128)", "error_code": "varchar(128)", "decision": "varchar(32)", "stage": "varchar(32)", "outcome": "varchar(16)", "source_original_time": "varchar(64)", "source_kind": "varchar(16)", "organization_id": "bigint unsigned", "testee_id": "bigint unsigned", "assessment_id": "bigint unsigned", "aggregate_sequence": "bigint unsigned", "next_sequence": "bigint unsigned", "event_sequence": "bigint unsigned", "revision": "bigint unsigned", "recorded_count": "bigint unsigned", "payload": "json", "projection": "json", "retirement_evidence": "json", "source_payload": "mediumblob", "wire": "mediumblob", "body": "mediumblob", "receipt": "mediumblob", "state": "mediumblob", "singleton": "tinyint unsigned"}
	if n == "message_id" {
		if table == "ai_messaging_outbox" {
			return t == "varchar(128)"
		}
		return t == "char(36)"
	}
	if n == "kind" {
		switch table {
		case "ai_bridge_commands":
			return t == "varchar(16)"
		case "ai_messaging_observations":
			return t == "varchar(64)"
		default:
			return t == "int"
		}
	}
	if n == "version" {
		return t == "bigint"
	}
	if n == "status" {
		return t == "varchar(32)"
	}
	if n == "attempts" {
		if table == "ai_bridge_commands" {
			return t == "int"
		}
		return t == "bigint unsigned"
	}
	if n == "source_attempts" {
		return t == "int"
	}
	if n == "retired" || n == "closed" || n == "ordered" || n == "requires_receipt" || n == "delivered" {
		return t == "tinyint(1)"
	}
	if strings.HasSuffix(n, "_at") || n == "recording_since" {
		return t == "datetime(6)"
	}
	return want[n] != "" && t == want[n]
}
func (s *AIReverseSnapshot) scan(ctx context.Context, spec aiReverseSpec) error {
	meta, e := s.schema(ctx, spec)
	if e != nil {
		return e
	}
	s.metadata = append(s.metadata, meta)
	var keyProjection, orderDesc, order, projection []string
	for _, n := range spec.keys {
		keyProjection = append(keyProjection, "CAST(`"+n+"` AS BINARY) AS `"+n+"`")
		order = append(order, "`"+n+"`")
		orderDesc = append(orderDesc, "`"+n+"` DESC")
	}
	for _, n := range meta.columns {
		projection = append(projection, "CAST(`"+n+"` AS BINARY) AS `"+n+"`")
	}
	upperRows, _, _, e := s.read(ctx, "SELECT "+strings.Join(keyProjection, ",")+" FROM `"+spec.table+"` FORCE INDEX(PRIMARY) ORDER BY "+strings.Join(orderDesc, ",")+" LIMIT 1", 1)
	if e != nil {
		return e
	}
	countRows, _, _, e := s.read(ctx, "SELECT CAST(COUNT(*) AS BINARY) AS n FROM `"+spec.table+"`", 1)
	if e != nil || len(countRows) != 1 {
		return ErrAIReverseRead
	}
	expected, e := aiReverseUint(countRows[0], "n", false)
	if e != nil {
		return e
	}
	if expected > s.limits.MaxRows-s.report.Rows {
		return ErrAIReverseBounds
	}
	var upper, after []string
	if len(upperRows) == 1 {
		upper, e = aiReverseKey(spec, upperRows[0])
		if e != nil {
			return e
		}
	} else if expected != 0 {
		return ErrAIReverseChanged
	}
	s.upper = append(s.upper, upper)
	ledger := AIReverseLedgerSummary{Store: spec.table, SchemaSHA256: meta.schema, PrimaryKeySHA256: meta.pk, UpperSHA256: aiReverseKeySHA(upper)}
	h := sha256.New()
	sourceFrame(h, []byte("ai-reverse-ledger/v1"), false)
	sourceFrame(h, []byte(meta.schema), false)
	s.byTable[spec.table] = map[string]*aiReverseNode{}
	for upper != nil {
		predicate, args := aiReversePredicate(spec, upper, true)
		if after != nil {
			p, vals := aiReversePredicate(spec, after, false)
			predicate += " AND " + p
			args = append(args, vals...)
		}
		args = append(args, s.limits.PageRows)
		rows, names, size, e := s.read(ctx, "SELECT "+strings.Join(projection, ",")+" FROM `"+spec.table+"` FORCE INDEX(PRIMARY) WHERE "+predicate+" ORDER BY "+strings.Join(order, ",")+" LIMIT ?", s.limits.PageRows, args...)
		if e != nil {
			return e
		}
		if !reflect.DeepEqual(names, meta.columns) {
			return ErrAIReverseSchema
		}
		if len(rows) == 0 {
			break
		}
		if size > s.limits.MaxBytes-s.report.Bytes || uint64(len(rows)) > s.limits.MaxRows-s.report.Rows {
			return ErrAIReverseBounds
		}
		s.report.Bytes += size
		s.report.Rows += uint64(len(rows))
		ledger.Bytes += size
		ledger.Rows += uint64(len(rows))
		ledger.Pages++
		for _, row := range rows {
			key, e := aiReverseKey(spec, row)
			if e != nil {
				return e
			}
			if after != nil && aiReverseCompare(spec, key, after) <= 0 || aiReverseCompare(spec, key, upper) > 0 {
				return ErrAIReverseChanged
			}
			after = key
			n := s.decode(spec, row, meta)
			n.inputPage = s.inputPage
			n.observation.Store = spec.table
			n.observation.PrimaryKeySHA256 = aiReverseKeySHA(key)
			n.observation.RowSHA256 = aiReverseRowSHA(meta.columns, row)
			sourceFrame(h, []byte(n.observation.RowSHA256), false)
			cost := uint64(2048)
			for _, v := range []string{n.id, n.aggregate, n.request, n.resource, n.org, n.subject, n.testee, n.hash, n.bodyHash, n.wireHash, n.command, n.receipt, n.receiptResource, n.receiptRun, n.receiptFamily, n.ack, n.event, n.linkedHash, n.linkedWireHash, n.projectionHash, n.sourceRowHash, n.state, n.payloadSHA, n.expectedBodyHash, n.sourceKind, n.sourceClock, n.originalClock, n.createdClock} {
				cost += uint64(len(v))
			}
			for _, v := range n.assessments {
				cost += uint64(len(v))
			}
			for _, v := range n.observation.Reasons {
				cost += uint64(len(v))
			}
			if cost > s.limits.MaxRetainedBytes-s.report.RetainedBudgetBytes {
				return ErrAIReverseBounds
			}
			s.report.RetainedBudgetBytes += cost
			if _, exists := s.byTable[spec.table][n.id]; exists {
				return ErrAIReverseChanged
			}
			s.byTable[spec.table][n.id] = n
			s.nodes = append(s.nodes, n)
		}
		if s.inputSink != nil {
			if e = s.inputSink(ctx, ledger, rows, false); e != nil {
				return e
			}
			s.inputPage++
		}
	}
	if ledger.Rows != expected || expected > 0 && aiReverseCompare(spec, after, upper) != 0 {
		return ErrAIReverseChanged
	}
	ledger.RowsSHA256 = hex.EncodeToString(h.Sum(nil))
	s.report.Ledgers = append(s.report.Ledgers, ledger)
	afterMeta, e := s.schema(ctx, spec)
	if e != nil || !reflect.DeepEqual(meta, afterMeta) {
		return ErrAIReverseSchema
	}
	if s.inputSink != nil {
		if e = s.inputSink(ctx, ledger, nil, true); e != nil {
			return e
		}
		s.inputPage++
	}
	return nil
}

func aiReverseReason(n *aiReverseNode, reason string) {
	for _, v := range n.observation.Reasons {
		if v == reason {
			return
		}
	}
	n.observation.Reasons = append(n.observation.Reasons, reason)
	n.observation.Invalid = true
}
func aiReverseKindsCommand(k pb.MessagingKind) bool {
	return k == pb.MessagingKind_START || k == pb.MessagingKind_CHANGE || k == pb.MessagingKind_PARTICIPANT_RETRY || k == pb.MessagingKind_EVALUATION_START || k == pb.MessagingKind_EVALUATION_CANCEL
}
func aiReverseRemote(k pb.MessagingKind) bool {
	return k == pb.MessagingKind_EVALUATION_START || k == pb.MessagingKind_EVALUATION_CANCEL || k == pb.MessagingKind_EVALUATION_STATE
}
func aiReverseSpecByTable(table string) aiReverseSpec {
	for _, v := range aiReverseSpecs {
		if v.table == table {
			return v
		}
	}
	return aiReverseSpec{}
}
func aiReverseSourceSHA(cols SQLColumns, row aiReverseRow, order []string) string {
	h := sha256.New()
	sourceFrame(h, []byte(aiJSONHash(cols)), false)
	for _, col := range order {
		sourceFrame(h, row[col], row[col] == nil)
	}
	return hex.EncodeToString(h.Sum(nil))
}
func (s *AIReverseSnapshot) decode(spec aiReverseSpec, r aiReverseRow, meta aiReverseMetadata) *aiReverseNode {
	n := &aiReverseNode{}
	bad := func() { aiReverseReason(n, "invalid_native_row_or_protocol") }
	for _, idcol := range []string{"command_id", "request_id", "message_id", "event_id", "run_id", "aggregate_key", "kind", "wire_sha256", "singleton"} {
		if r[idcol] != nil {
			n.id = r.text(idcol)
			break
		}
	}
	n.aggregate = r.text("aggregate_key")
	n.request = r.text("request_id")
	n.org = r.text("organization_id")
	n.subject = r.text("subject_id")
	n.resource = r.text("resource_id")
	n.testee = r.text("testee_id")
	n.bodyHash = r.text("body_sha256")
	n.wireHash = r.text("wire_sha256")
	n.state = r.text("status")
	switch spec.table {
	case "ai_bridge_requests":
		n.request = n.id
		n.hash = r.text("request_hash")
		n.resource = r.text("session_id")
		v, e := aiDecodeBusiness("start", n.id, n.id, r["payload"], n.hash)
		if e != nil {
			bad()
			return n
		}
		n.org, n.subject, n.testee = v.OrganizationID, v.SubjectID, v.Business.TesteeID
		n.assessments = append([]string(nil), v.Business.AssessmentIDs...)
		n.expectedBodyHash, e = aiExpectedCommandHash(r["payload"], "start")
		if e != nil {
			bad()
		}
		for _, pair := range [][2]string{{"organization_id", n.org}, {"subject_id", n.subject}, {"testee_id", n.testee}} {
			if r[pair[0]] != nil && r.text(pair[0]) != pair[1] {
				bad()
			}
		}
		if !aiReverseClock(r["created_at"], true) || !aiReverseClock(r["updated_at"], true) {
			bad()
		}
		n.createdClock = r.text("created_at")
		version, ok := aiUint(r["version"])
		if !ok {
			bad()
		}
		n.version = version
		// An accepted START receipt binds the session before its first state
		// projection. The reverse graph must prove that exact original receipt;
		// a non-NULL session alone is never accepted as evidence of completion.
		if r["projection"] == nil {
			n.projectionAbsent = true
			if version != 0 || n.state != "pending" || r["session_id"] != nil && !aiOriginalUUID(n.resource) {
				bad()
			}
			n.observation.Unfinished = true
			break
		}
		var fields map[string]json.RawMessage
		var p app.Event
		if strictJSON(r["projection"]) != nil || json.Unmarshal(r["projection"], &fields) != nil || !aiExactKeys(fields, []string{"event_id", "request_id", "session_id", "actor", "testee_id", "version", "status", "question_id", "question", "can_skip", "failure_code"}, []string{"artifact_json"}) || json.Unmarshal(r["projection"], &p) != nil || app.ValidateEvent(p) != nil || p.RequestID != n.request || p.Actor.OrgID != n.org || p.Actor.SubjectID != n.subject || p.TesteeID != n.testee || p.SessionID != n.resource || !aiOriginalUUID(p.SessionID) || p.Version < 1 || uint64(p.Version) != n.version || p.Status != n.state {
			bad()
			break
		}
		n.event = p.EventID
		raw, e := json.Marshal(p)
		if e != nil {
			bad()
		} else {
			n.projectionHash = sourceSHA(raw)
		}
		var original app.Start
		if json.Unmarshal(r["payload"], &original) != nil || aiVerifyArtifactRequest(p, &original) != nil {
			bad()
		}
		n.observation.Unfinished = n.state != "completed" && n.state != "cancelled"
	case "ai_bridge_commands", "ai_messaging_legacy_commands":
		payload, writer, kind, attempt, clock := "payload", "payload_hash", "kind", "attempts", "available_at"
		if spec.table == AILegacyCommandSource {
			payload, writer, kind, attempt, clock = "source_payload", "source_payload_hash", "source_kind", "source_attempts", "source_available_at"
		}
		v, e := aiDecodeBusiness(r.text(kind), n.id, n.request, r[payload], r.text(writer))
		if e != nil {
			bad()
			return n
		}
		n.org, n.subject, n.resource, n.testee = v.OrganizationID, v.SubjectID, v.ResourceID, v.Business.TesteeID
		n.hash = v.WriterPayloadDigest.SHA256
		n.payloadSHA = sourceSHA(r[payload])
		n.sourceKind = r.text(kind)
		n.sourceClock = r.text(clock)
		n.expectedBodyHash, e = aiExpectedCommandHash(r[payload], n.sourceKind)
		if e != nil {
			bad()
		}
		n.sourceRowHash = aiReverseSourceSHA(meta.sourceColumns, r, spec.columns)
		attempts, e := aiReverseUint(r, attempt, false)
		if e != nil || attempts > 2147483647 || !aiSQLMicrosecondClock(n.sourceClock) {
			bad()
		}
		n.attempts = attempts
		if spec.table == AIBridgeCommandSource {
			n.delivered, e = aiReverseBool(r, "delivered")
			if e != nil {
				bad()
			}
			n.observation.Unfinished = !n.delivered
		} else {
			n.bodyHash = r.text("messaging_body_sha256")
			if !evidenceHash(n.bodyHash) || n.bodyHash != n.expectedBodyHash || !aiReverseClock(r["transferred_at"], false) {
				bad()
			}
			if r.text("source_original_time") != "" {
				n.originalClock = r.text("source_original_time")
				at, e := time.Parse(time.RFC3339Nano, r.text("source_original_time"))
				_, offset := at.Zone()
				if e != nil || offset != 28800 || n.sourceKind != "start" || at.Nanosecond()%1000 != 0 || at.Format(time.RFC3339Nano) != r.text("source_original_time") {
					bad()
				}
			}
		}
	case "ai_bridge_events":
		// The original event PK is distinct from its parent request FK.
		n.id = r.text("event_id")
		n.hash = r.text("payload_hash")
		n.version, _ = aiUint(r["version"])
		if !aiOriginalUUID(n.id) || !aiOriginalUUID(n.request) || !evidenceHash(n.hash) || n.version == 0 {
			bad()
		}
	case "ai_bridge_request_assessments":
		n.id = n.request + ":" + r.text("assessment_id")
		n.resource = r.text("assessment_id")
		if !aiOriginalUUID(n.request) || !aiPositiveNumber(n.resource) {
			bad()
		}
	case "ai_messaging_operations":
		k, ok := aiKind(r["kind"])
		n.kind = k
		n.hash = n.bodyHash
		n.receipt = r.text("receipt_id")
		n.state = r.text("decision")
		n.retired, _ = aiReverseBool(r, "retired")
		n.sequence, _ = aiUint(r["aggregate_sequence"])
		if !ok || !aiReverseKindsCommand(k) || !aiOriginalUUID(n.id) || !aiOriginalUUID(n.aggregate) || !aiOriginalUUID(n.resource) || !aiPositiveNumber(n.org) || n.subject == "" || len(n.subject) > 128 {
			bad()
		}
		if n.retired {
			if r["body_sha256"] != nil || r["aggregate_sequence"] != nil || r["created_at"] != nil || r["receipt_id"] != nil || r["receipt"] != nil || r["decided_at"] != nil || r.text("decision") != "" || r.text("code") != "" || !aiReverseClock(r["retired_at"], false) || !aiReverseRetirementMetadata(r, n, false) {
				bad()
			}
			break
		}
		if r.text("retired") != "0" || r["retired_at"] != nil || !evidenceHash(n.hash) || n.sequence == 0 || !aiReverseClock(r["created_at"], false) || !aiReverseClock(r["decided_at"], true) {
			bad()
		}
		if r["retirement_evidence"] != nil && !aiReverseRetirementMetadata(r, n, true) {
			bad()
		}
		switch n.state {
		case "":
			n.observation.Unfinished = true
			if r["receipt_id"] != nil || r["receipt"] != nil || r["decided_at"] != nil {
				bad()
			}
		case "held":
			n.observation.Held = true
			n.observation.Unfinished = true
			fallthrough
		case "accepted", "rejected":
			if !aiOriginalUUID(n.receipt) || r["receipt"] == nil || !aiReverseClock(r["decided_at"], false) {
				bad()
				break
			}
			var body pb.MessagingBody
			if proto.Unmarshal(r["receipt"], &body) != nil || aiReverseProtoUnknown(body.ProtoReflect()) {
				bad()
				break
			}
			receipt := body.GetCommandReceipt()
			if receipt == nil {
				bad()
				break
			}
			parsed, e := aiStoredBody(pb.MessagingKind_COMMAND_RECEIPT, n.receipt, n.aggregate, n.id, sourceSHA(r["receipt"]), r["receipt"])
			if e != nil {
				bad()
				break
			}
			v := parsed.GetCommandReceipt()
			n.linkedHash = sourceSHA(r["receipt"])
			n.decision = v.Decision
			if v.CommandId != n.id || v.CommandBodySha256 != n.hash || v.Code != r.text("code") || v.Decision != aiReverseDecision(n.state) {
				bad()
			}
			aiReverseReceiptFacts(n, v, bad)
		default:
			bad()
		}
	case "ai_messaging_outbox":
		k, ok := aiKind(r["kind"])
		n.kind = k
		n.sequence, _ = aiUint(r["aggregate_sequence"])
		var boolErr error
		n.ordered, boolErr = aiReverseBool(r, "ordered")
		if boolErr != nil {
			bad()
		}
		n.requiresReceipt, boolErr = aiReverseBool(r, "requires_receipt")
		if boolErr != nil {
			bad()
		}
		n.attempts, _ = aiUint(r["attempts"])
		n.state = r.text("stage")
		producer, destination, topic, e := app.MessagingRoute(k)
		if !ok || e != nil || producer != "qs-server" || destination != "qs-ai" || r.text("producer") != producer || r.text("destination") != destination || r.text("topic") != topic || !aiPositiveNumber(n.org) || !aiOriginalUUID(n.aggregate) || !aiCheckStoredWire(r["wire"], n.id, n.wireHash) || !evidenceHash(n.bodyHash) || sourceSHA(r["body"]) != n.bodyHash {
			bad()
			break
		}
		if !aiReverseClock(r["available_at"], false) || !aiReverseClock(r["created_at"], false) || !aiReverseClock(r["published_at"], true) || !aiReverseClock(r["confirmed_at"], true) {
			bad()
		}
		switch n.state {
		case "staged", "awaiting_receipt":
			n.observation.Unfinished = true
		case "confirmed":
			if r["confirmed_at"] == nil {
				bad()
			}
		case "held":
			n.observation.Held = true
			n.observation.Unfinished = true
		default:
			bad()
		}
		correlation := ""
		var transient pb.MessagingBody
		if proto.Unmarshal(r["body"], &transient) != nil {
			bad()
			break
		}
		if k == pb.MessagingKind_COMMAND_RECEIPT && transient.GetCommandReceipt() != nil {
			correlation = transient.GetCommandReceipt().CommandId
		}
		body, e := aiStoredBody(k, n.id, n.aggregate, correlation, n.bodyHash, r["body"])
		if e != nil {
			bad()
			break
		}
		storedOrg := n.org
		s.bodyFacts(n, body, bad)
		if n.org != storedOrg {
			bad()
		}
		if k == pb.MessagingKind_EVENT_ACKNOWLEDGEMENT {
			if n.ordered || n.requiresReceipt || n.sequence != 1 {
				bad()
			}
		} else if !aiReverseKindsCommand(k) || !n.ordered || !n.requiresReceipt || n.sequence == 0 {
			bad()
		}
	case "ai_messaging_inbox":
		k, ok := aiKind(r["kind"])
		n.kind = k
		n.ack = r.text("ack_id")
		n.state = r.text("outcome")
		if !ok || (k != pb.MessagingKind_COMMAND_RECEIPT && k != pb.MessagingKind_INTERPRETATION_STATE && k != pb.MessagingKind_EVALUATION_STATE) || r.text("producer") != "qs-ai" || !aiOriginalUUID(n.id) || !aiOriginalUUID(n.aggregate) || !aiOriginalUUID(n.ack) || !evidenceHash(n.wireHash) || !aiReverseClock(r["received_at"], false) {
			bad()
		}
		switch n.state {
		case "stored":
		case "held":
			n.observation.Held = true
			n.observation.Unfinished = true
		default:
			bad()
		}
		var transient pb.MessagingBody
		if proto.Unmarshal(r["body"], &transient) != nil {
			bad()
			break
		}
		correlation := ""
		if k == pb.MessagingKind_COMMAND_RECEIPT && transient.GetCommandReceipt() != nil {
			correlation = transient.GetCommandReceipt().CommandId
		}
		body, e := aiStoredBody(k, n.id, n.aggregate, correlation, n.bodyHash, r["body"])
		if e != nil {
			bad()
			break
		}
		s.bodyFacts(n, body, bad)
	case "ai_messaging_failures":
		k, ok := aiKind(r["kind"])
		n.kind = k
		n.attempts, _ = aiUint(r["attempts"])
		n.wireHash = sourceSHA(r["wire"])
		if !ok || r.text("producer") != "qs-ai" || !aiOriginalUUID(n.id) || !aiOriginalUUID(n.aggregate) || !evidenceHash(n.bodyHash) || n.attempts == 0 || !aiReverseClock(r["first_seen_at"], false) || !aiReverseClock(r["last_seen_at"], false) || !aiCheckStoredWire(r["wire"], n.id, n.wireHash) {
			bad()
		}
	case "ai_messaging_aggregates":
		n.aggregate = n.id
		n.sequence, _ = aiUint(r["next_sequence"])
		if !aiOriginalUUID(n.id) || n.sequence == 0 {
			bad()
		}
	case "ai_messaging_evaluation_states":
		n.aggregate = n.id
		n.kind = pb.MessagingKind_EVALUATION_STATE
		n.sequence, _ = aiUint(r["event_sequence"])
		n.version, _ = aiUint(r["version"])
		n.bodyHash = sourceSHA(r["state"])
		var body pb.MessagingBody
		if proto.Unmarshal(r["state"], &body) != nil || aiReverseProtoUnknown(body.ProtoReflect()) || body.GetEvaluationState() == nil {
			bad()
			break
		}
		v := body.GetEvaluationState()
		if !aiOriginalUUID(n.id) || !aiPositiveNumber(n.org) || v.RunId != n.id || v.OrganizationId != n.org || v.EventSequence != n.sequence || v.Version < 1 || uint64(v.Version) != n.version || n.sequence == 0 || !aiReverseClock(r["updated_at"], false) {
			bad()
		}
		n.state = v.Status
	case "ai_messaging_quarantine":
		n.id = r.text("wire_sha256")
		n.wireHash = n.id
		n.attempts, _ = aiUint(r["attempts"])
		if !evidenceHash(n.id) || sourceSHA(r["wire"]) != n.id || n.attempts == 0 || !aiReverseClock(r["first_seen_at"], false) || !aiReverseClock(r["last_seen_at"], false) {
			bad()
		}
		aiReverseReason(n, "quarantine_owner_and_authentication_unknown")
	case "ai_messaging_admission":
		n.id = r.text("singleton")
		closed, e := aiReverseBool(r, "closed")
		revision, ok := aiUint(r["revision"])
		n.sequence = revision
		n.state = "open"
		if closed {
			n.state = "closed"
		}
		if n.id != "1" || e != nil || !ok || !aiReverseClock(r["updated_at"], false) {
			bad()
		}
	case "ai_messaging_observations":
		n.id = r.text("kind")
		n.sequence, _ = aiUint(r["recorded_count"])
		if !aiReverseObservationKinds[n.id] || !aiReverseClock(r["recording_since"], false) || !aiReverseClock(r["last_observed_at"], true) {
			bad()
		}
	}
	return n
}
func aiReverseDecision(state string) pb.MessagingDecision {
	switch state {
	case "accepted":
		return pb.MessagingDecision_ACCEPTED
	case "rejected":
		return pb.MessagingDecision_REJECTED
	case "held":
		return pb.MessagingDecision_HELD
	}
	return pb.MessagingDecision_MESSAGING_DECISION_UNSPECIFIED
}
func aiReverseReceiptFacts(n *aiReverseNode, v *pb.MessagingCommandReceipt, bad func()) {
	n.command = v.CommandId
	n.hash = v.CommandBodySha256
	n.decision = v.Decision
	if v.Decision == pb.MessagingDecision_ACCEPTED {
		if w := v.GetWorkflowReceipt(); w != nil {
			if !aiOriginalUUID(w.SessionId) || !aiOriginalUUID(w.RunId) || w.Version < 1 || w.Status == "" {
				bad()
			}
			n.receiptFamily = "workflow"
			n.receiptResource = w.SessionId
			n.receiptRun = w.RunId
		} else if e := v.GetEvaluationReceipt(); e != nil {
			if !aiOriginalUUID(e.RunId) || e.Version < 1 {
				bad()
			}
			n.receiptFamily = "evaluation"
			n.receiptResource = e.RunId
			n.receiptRun = e.RunId
		} else {
			bad()
		}
	}
	if v.Decision == pb.MessagingDecision_HELD {
		n.observation.Held = true
		n.observation.Unfinished = true
	}
}
func (s *AIReverseSnapshot) bodyFacts(n *aiReverseNode, b *pb.MessagingBody, bad func()) {
	if b == nil || aiReverseProtoUnknown(b.ProtoReflect()) {
		bad()
		return
	}
	switch n.kind {
	case pb.MessagingKind_START:
		v := b.GetStart()
		if v == nil {
			return
		}
		n.request = v.RequestId
		n.org = v.GetActor().GetOrgId()
		n.subject = v.GetActor().GetSubjectId()
		n.testee = v.TesteeId
		n.resource = v.RequestId
		n.assessments = append([]string(nil), v.AssessmentIds...)
		if n.request != n.aggregate || !aiPositiveNumber(n.org) || !aiPositiveNumber(n.testee) || len(n.assessments) < 1 || len(n.assessments) > 10 {
			bad()
		}
	case pb.MessagingKind_CHANGE:
		v := b.GetChange()
		if v == nil {
			return
		}
		n.request = n.aggregate
		n.resource = v.SessionId
		n.org = v.GetActor().GetOrgId()
		n.subject = v.GetActor().GetSubjectId()
		if !aiOriginalUUID(n.resource) || v.ExpectedVersion < 1 || v.Action != "answer" && v.Action != "cancel" {
			bad()
		}
	case pb.MessagingKind_PARTICIPANT_RETRY:
		v := b.GetParticipantRetry()
		if v == nil {
			return
		}
		n.request = n.aggregate
		n.resource = v.SessionId
		n.org = strconv.FormatInt(v.GetScope().GetOrganizationId(), 10)
		n.subject = strconv.FormatInt(v.GetScope().GetOperatorUserId(), 10)
		if v.ExpectedProviderInvocations != 1 || !aiOriginalUUID(v.ExpectedRunId) || !aiOriginalUUID(n.resource) || v.ExpectedVersion < 1 {
			bad()
		}
	case pb.MessagingKind_EVALUATION_START:
		v := b.GetEvaluationStart()
		if v == nil {
			return
		}
		n.resource = v.GetScope().GetRunId()
		n.org = strconv.FormatInt(v.GetScope().GetOrganizationId(), 10)
		n.subject = strconv.FormatInt(v.GetScope().GetOperatorUserId(), 10)
		if n.resource != n.aggregate || !aiOriginalUUID(n.resource) || !aiPositiveNumber(n.org) {
			bad()
		}
	case pb.MessagingKind_EVALUATION_CANCEL:
		v := b.GetEvaluationCancel()
		if v == nil {
			return
		}
		n.resource = v.GetScope().GetRunId()
		n.org = strconv.FormatInt(v.GetScope().GetOrganizationId(), 10)
		n.subject = strconv.FormatInt(v.GetScope().GetOperatorUserId(), 10)
		if n.resource != n.aggregate || !aiOriginalUUID(n.resource) || !aiPositiveNumber(n.org) {
			bad()
		}
	case pb.MessagingKind_COMMAND_RECEIPT:
		v := b.GetCommandReceipt()
		if v != nil {
			aiReverseReceiptFacts(n, v, bad)
		}
	case pb.MessagingKind_INTERPRETATION_STATE:
		v := b.GetInterpretationState()
		if v == nil {
			return
		}
		p := app.Event{EventID: v.EventId, RequestID: v.RequestId, SessionID: v.SessionId, Actor: app.Actor{OrgID: v.GetActor().GetOrgId(), SubjectID: v.GetActor().GetSubjectId()}, TesteeID: v.TesteeId, Version: v.Version, Status: v.Status, QuestionID: v.QuestionId, Question: v.Question, CanSkip: v.CanSkip, FailureCode: v.FailureCode, ArtifactJSON: v.ArtifactJson}
		if app.ValidateEvent(p) != nil || p.RequestID != n.aggregate || !aiOriginalUUID(p.SessionID) {
			bad()
		}
		n.request, n.resource, n.org, n.subject, n.testee, n.event = p.RequestID, p.SessionID, p.Actor.OrgID, p.Actor.SubjectID, p.TesteeID, p.EventID
		n.version = uint64(p.Version)
		raw, e := json.Marshal(p)
		if e != nil {
			bad()
		} else {
			n.projectionHash = sourceSHA(raw)
		}
	case pb.MessagingKind_EVALUATION_STATE:
		v := b.GetEvaluationState()
		if v == nil {
			return
		}
		n.org = v.OrganizationId
		n.resource = v.RunId
		n.version = uint64(v.Version)
		n.sequence = v.EventSequence
		if v.RunId != n.aggregate || !aiOriginalUUID(v.RunId) || !aiPositiveNumber(v.OrganizationId) || v.Version < 1 || v.EventSequence < 1 {
			bad()
		}
	case pb.MessagingKind_EVENT_ACKNOWLEDGEMENT:
		v := b.GetEventAcknowledgement()
		if v != nil {
			n.event = v.EventId
			n.hash = v.EventBodySha256
			n.eventKind = v.EventKind
			n.ackOutcome = v.Outcome
			// The event outcome does not settle this ACK's own transport. Keep
			// staged, awaiting-receipt and held Outbox responsibility intact.
			outcomeHeld := v.Outcome == pb.MessagingEventAcknowledgement_TECHNICALLY_HELD
			n.observation.Held = n.observation.Held || outcomeHeld
			n.observation.Unfinished = n.observation.Unfinished || outcomeHeld
		}
	default:
		bad()
	}
}

// Persisted retirement metadata remains historical, never a live accepted
// receipt. Unknown fields or partial identities cannot retire a reverse edge.
func aiReverseRetirementMetadata(r aiReverseRow, n *aiReverseNode, transferred bool) bool {
	if strictJSON(r["retirement_evidence"]) != nil {
		return false
	}
	var e struct {
		Version            int       `json:"version"`
		OperationID        string    `json:"operation_id"`
		VerifierVersion    string    `json:"verifier_version"`
		VerificationMethod string    `json:"verification_method"`
		VerifiedAt         time.Time `json:"verified_at"`
		AdmissionRevision  uint64    `json:"admission_revision"`
		CommandID          string    `json:"command_id"`
		RequestID          string    `json:"request_id"`
		SourceKind         string    `json:"source_kind"`
		OrganizationID     string    `json:"organization_id"`
		SubjectID          string    `json:"subject_id"`
		ResourceID         string    `json:"resource_id"`
		LiveBodySHA256     string    `json:"live_body_sha256,omitempty"`
		Sources            []struct {
			Table               string `json:"table"`
			CommandID           string `json:"command_id"`
			BytesKind           string `json:"bytes_kind"`
			BytesSHA256         string `json:"bytes_sha256"`
			BusinessPayloadHash string `json:"business_payload_hash"`
		} `json:"sources"`
		References []struct {
			Kind string `json:"kind"`
			ID   string `json:"id"`
		} `json:"references"`
		Conclusion           string `json:"conclusion"`
		Reason               string `json:"reason"`
		OwnershipVerified    bool   `json:"ownership_verified"`
		ResponsibilityClosed bool   `json:"responsibility_closed"`
		BusinessTerminal     bool   `json:"business_terminal"`
	}
	d := json.NewDecoder(bytes.NewReader(r["retirement_evidence"]))
	d.DisallowUnknownFields()
	if d.Decode(&e) != nil || e.Version != 1 || !aiLocalOperationID(e.OperationID) || e.CommandID != n.id || e.RequestID != n.aggregate || e.OrganizationID != n.org || e.SubjectID != n.subject || e.ResourceID != n.resource || !e.OwnershipVerified || e.VerifiedAt.IsZero() || e.VerifierVersion == "" || len(e.VerifierVersion) > 128 || len(e.Sources) < 1 || len(e.Sources) > 2 || len(e.References) < 1 || len(e.References) > 16 {
		return false
	}
	_, offset := e.VerifiedAt.Zone()
	if offset != 0 {
		return false
	}
	if e.SourceKind != "start" && e.SourceKind != "answer" && e.SourceKind != "cancel" {
		return false
	}
	if e.SourceKind == "start" && (e.CommandID != e.RequestID || e.ResourceID != e.RequestID) {
		return false
	}
	for _, ref := range e.References {
		switch ref.Kind {
		case "business_record", "operation", "event", "migration_manifest", "readonly_run":
		default:
			return false
		}
		if !aiReverseASCII(ref.ID) {
			return false
		}
	}
	if transferred {
		if e.Conclusion != "transferred_verified" || e.Reason != "handoff_verified" || e.VerificationMethod != "source_identity_hash_and_live_ledger" || e.LiveBodySHA256 != n.bodyHash {
			return false
		}
	} else if !e.BusinessTerminal || !e.ResponsibilityClosed || e.LiveBodySHA256 != "" || e.VerificationMethod != "source_identity_hash_and_business_closure" || e.Conclusion != "verified" && e.Conclusion != "unverifiable" || e.Conclusion == "verified" && e.Reason != "history_terminal_verified" || e.Conclusion == "unverifiable" && e.Reason != "history_terminal_evidence_gap" {
		return false
	}
	seen := map[string]bool{}
	for _, v := range e.Sources {
		kind := AIBridgePayloadBytesKind
		if v.Table == AILegacyCommandSource {
			kind = AILegacyPayloadBytesKind
		} else if v.Table != AIBridgeCommandSource {
			return false
		}
		if seen[v.Table] || v.CommandID != n.id || v.BytesKind != kind || !evidenceHash(v.BytesSHA256) || !evidenceHash(v.BusinessPayloadHash) {
			return false
		}
		seen[v.Table] = true
	}
	return seen[AIBridgeCommandSource] && (!transferred || seen[AILegacyCommandSource])
}

func aiReverseProtoUnknown(m protoreflect.Message) bool {
	if !m.IsValid() || len(m.GetUnknown()) != 0 {
		return true
	}
	unknown := false
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		if fd.IsMap() {
			if fd.MapValue().Kind() == protoreflect.MessageKind {
				v.Map().Range(func(_ protoreflect.MapKey, item protoreflect.Value) bool {
					unknown = aiReverseProtoUnknown(item.Message())
					return !unknown
				})
			}
		} else if fd.IsList() {
			if fd.Kind() == protoreflect.MessageKind {
				for i := 0; i < v.List().Len(); i++ {
					if aiReverseProtoUnknown(v.List().Get(i).Message()) {
						unknown = true
						break
					}
				}
			}
		} else if fd.Kind() == protoreflect.MessageKind {
			unknown = aiReverseProtoUnknown(v.Message())
		}
		return !unknown
	})
	return unknown
}
func aiReverseSafeColumn(v string) bool {
	if v == "" || len(v) > 64 {
		return false
	}
	for _, c := range v {
		if c != '_' && (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// All actual Assessment columns, including SQL99's read-only evidence column,
// participate in the raw baseline. The actual PRIMARY/unique sheet constraint
// prevents a cross-organization duplicate from being hidden by an org WHERE.
func (s *AIReverseSnapshot) assessmentMetadata(ctx context.Context) ([]string, string, error) {
	cols, names, _, e := s.read(ctx, "SELECT COLUMN_NAME AS name,ORDINAL_POSITION AS ordinal,COLUMN_TYPE AS type,IS_NULLABLE AS nullable,COLUMN_DEFAULT AS `default`,EXTRA AS extra,COLLATION_NAME AS collation,GENERATION_EXPRESSION AS generation FROM information_schema.columns WHERE table_schema=DATABASE() AND BINARY table_name=BINARY 'assessment' ORDER BY ORDINAL_POSITION", 128)
	if e != nil || len(cols) == 0 {
		return nil, "", ErrAIReverseSchema
	}
	h := sha256.New()
	sourceFrame(h, []byte("ai-reverse-assessment-schema/v1"), false)
	columns := make([]string, 0, len(cols))
	by := map[string]aiReverseRow{}
	for i, col := range cols {
		n := col.text("name")
		if !aiReverseSafeColumn(n) || col.text("ordinal") != strconv.Itoa(i+1) || col.text("generation") != "" || by[n] != nil {
			return nil, "", ErrAIReverseSchema
		}
		columns = append(columns, n)
		by[n] = col
		sourceFrame(h, []byte(aiReverseRowSHA(names, col)), false)
	}
	for _, n := range []string{"id", "org_id", "testee_id", "answer_sheet_id", "deleted_at", "historical_lifecycle_evidence"} {
		if by[n] == nil {
			return nil, "", ErrAIReverseSchema
		}
	}
	if by["id"].text("type") != "bigint unsigned" || by["org_id"].text("type") != "bigint" || by["testee_id"].text("type") != "bigint unsigned" || by["answer_sheet_id"].text("type") != "bigint unsigned" || by["historical_lifecycle_evidence"].text("type") != "json" {
		return nil, "", ErrAIReverseSchema
	}
	ddl, dn, _, e := s.read(ctx, "SHOW CREATE TABLE assessment", 1)
	if e != nil || len(ddl) != 1 || len(dn) != 2 {
		return nil, "", ErrAIReverseSchema
	}
	sourceFrame(h, []byte(aiReverseRowSHA(dn, ddl[0])), false)
	ix, in, _, e := s.read(ctx, "SELECT INDEX_NAME AS name,NON_UNIQUE AS non_unique,SEQ_IN_INDEX AS ordinal,COLUMN_NAME AS col,SUB_PART AS sub_part,INDEX_TYPE AS type,IS_VISIBLE AS visible,EXPRESSION AS expression FROM information_schema.statistics WHERE table_schema=DATABASE() AND BINARY table_name=BINARY 'assessment' ORDER BY INDEX_NAME,SEQ_IN_INDEX", 256)
	if e != nil {
		return nil, "", e
	}
	counts := map[string]int{}
	for _, v := range ix {
		sourceFrame(h, []byte(aiReverseRowSHA(in, v)), false)
		if v.text("name") != "PRIMARY" && v.text("name") != "uk_answer_sheet_id" {
			continue
		}
		want := "id"
		if v.text("name") == "uk_answer_sheet_id" {
			want = "answer_sheet_id"
		}
		counts[v.text("name")]++
		if v.text("non_unique") != "0" || v.text("ordinal") != "1" || v.text("col") != want || v["sub_part"] != nil || v.text("type") != "BTREE" || v.text("visible") != "YES" || v["expression"] != nil {
			return nil, "", ErrAIReverseSchema
		}
	}
	if counts["PRIMARY"] != 1 || counts["uk_answer_sheet_id"] != 1 {
		return nil, "", ErrAIReverseSchema
	}
	return columns, hex.EncodeToString(h.Sum(nil)), nil
}
func (s *AIReverseSnapshot) readAssessmentAnchors(ctx context.Context) error {
	columns, schema, e := s.assessmentMetadata(ctx)
	if e != nil {
		return e
	}
	s.anchorMetadataSHA = schema
	wanted := map[string]bool{}
	for _, n := range s.nodes {
		if n.observation.Store == "ai_bridge_request_assessments" {
			wanted[n.resource] = true
		}
		for _, id := range n.assessments {
			if !aiPositiveNumber(id) {
				return ErrAIReverseBinding
			}
			wanted[id] = true
		}
	}
	ids := make([]string, 0, len(wanted))
	for id := range wanted {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		x, _ := strconv.ParseUint(ids[i], 10, 64)
		y, _ := strconv.ParseUint(ids[j], 10, 64)
		return x < y
	})
	projection := make([]string, 0, len(columns))
	for _, n := range columns {
		projection = append(projection, "CAST(`"+n+"` AS BINARY) AS `"+n+"`")
	}
	for offset := 0; offset < len(ids); offset += s.limits.PageRows {
		end := offset + s.limits.PageRows
		if end > len(ids) {
			end = len(ids)
		}
		args := make([]any, 0, end-offset)
		place := make([]string, 0, end-offset)
		for _, id := range ids[offset:end] {
			v, _ := strconv.ParseUint(id, 10, 64)
			args = append(args, v)
			place = append(place, "?")
		}
		rows, names, size, e := s.read(ctx, "SELECT "+strings.Join(projection, ",")+" FROM assessment FORCE INDEX(PRIMARY) WHERE id IN ("+strings.Join(place, ",")+") ORDER BY id", end-offset, args...)
		if e != nil || !reflect.DeepEqual(names, columns) {
			return ErrAIReverseRead
		}
		if size > s.limits.MaxBytes-s.report.Bytes {
			return ErrAIReverseBounds
		}
		s.report.Bytes += size
		for _, r := range rows {
			id := r.text("id")
			if !wanted[id] || s.anchors[id].id != "" || !aiPositiveNumber(r.text("org_id")) || !aiPositiveNumber(r.text("testee_id")) || !aiPositiveNumber(r.text("answer_sheet_id")) || r["deleted_at"] != nil {
				return ErrAIReverseBinding
			}
			a := aiReverseAnchor{id, r.text("org_id"), r.text("testee_id"), r.text("answer_sheet_id"), aiReverseRowSHA(columns, r)}
			s.anchors[id] = a
			cost := uint64(1024 + len(a.id) + len(a.org) + len(a.testee) + len(a.sheet))
			if cost > s.limits.MaxRetainedBytes-s.report.RetainedBudgetBytes {
				return ErrAIReverseBounds
			}
			s.report.RetainedBudgetBytes += cost
		}
	}
	_, after, e := s.assessmentMetadata(ctx)
	if e != nil || schema != after {
		return ErrAIReverseSchema
	}
	h := sha256.New()
	sourceFrame(h, []byte(schema), false)
	for _, id := range ids {
		a := s.anchors[id]
		sourceFrame(h, []byte(id), false)
		sourceFrame(h, []byte(a.rawSHA), false)
	}
	s.report.BusinessAnchorsSHA256 = hex.EncodeToString(h.Sum(nil))
	return nil
}
func aiReverseSameOwner(a, b *aiReverseNode) bool {
	return a != nil && b != nil && a.org == b.org && a.subject == b.subject && (a.testee == "" || b.testee == "" || a.testee == b.testee)
}
func aiReversePair(a, b *aiReverseNode, reason string) {
	if a != nil {
		aiReverseReason(a, reason)
	}
	if b != nil {
		aiReverseReason(b, reason)
	}
}
func aiReverseLegacyTimeMatches(legacy, request *aiReverseNode) bool {
	if legacy == nil || request == nil {
		return false
	}
	if legacy.sourceKind != "start" {
		return legacy.originalClock == ""
	}
	if request.createdClock == "" {
		return legacy.originalClock == ""
	}
	at, e := time.Parse("2006-01-02 15:04:05.000000", request.createdClock)
	return e == nil && legacy.originalClock == at.In(time.FixedZone("UTC+8", 28800)).Format(time.RFC3339Nano)
}
func aiReverseAcceptedReceiptMatches(op, request *aiReverseNode) bool {
	if op == nil || op.state != "accepted" {
		return true
	}
	if aiReverseRemote(op.kind) {
		return op.receiptFamily == "evaluation" && op.receiptResource == op.resource && op.receiptRun == op.resource
	}
	return request != nil && op.receiptFamily == "workflow" && op.receiptResource == request.resource && aiOriginalUUID(op.receiptRun)
}
func (s *AIReverseSnapshot) reverse() {
	reqs, ops, boxes, inboxes := s.byTable["ai_bridge_requests"], s.byTable["ai_messaging_operations"], s.byTable["ai_messaging_outbox"], s.byTable["ai_messaging_inbox"]
	associated := map[string]map[string]bool{}
	sessions := map[string]*aiReverseNode{}
	for _, n := range s.byTable["ai_bridge_request_assessments"] {
		r := reqs[n.request]
		a := s.anchors[n.resource]
		if r == nil || a.id == "" || a.org != r.org || a.testee != r.testee {
			aiReversePair(n, r, "assessment_owner_or_association_conflict")
			continue
		}
		n.org, n.subject, n.testee = r.org, r.subject, r.testee
		if associated[n.request] == nil {
			associated[n.request] = map[string]bool{}
		}
		associated[n.request][n.resource] = true
	}
	for _, r := range reqs {
		if len(associated[r.id]) != len(r.assessments) {
			aiReverseReason(r, "request_assessment_set_incomplete")
		}
		for _, id := range r.assessments {
			a := s.anchors[id]
			if !associated[r.id][id] || a.org != r.org || a.testee != r.testee {
				aiReverseReason(r, "request_assessment_set_incomplete")
			}
		}
		if r.resource != "" {
			if prior := sessions[r.resource]; prior != nil {
				aiReversePair(r, prior, "session_owner_conflict")
			}
			sessions[r.resource] = r
		}
	}
	seenVersion := map[string]*aiReverseNode{}
	for _, n := range s.byTable["ai_bridge_events"] {
		r := reqs[n.request]
		if r == nil {
			aiReverseReason(n, "event_request_orphan")
			continue
		}
		n.org, n.subject, n.testee = r.org, r.subject, r.testee
		n.resource = r.resource
		if n.version > r.version {
			aiReversePair(n, r, "event_version_conflict")
		}
		key := n.request + ":" + strconv.FormatUint(n.version, 10)
		if prior := seenVersion[key]; prior != nil {
			aiReversePair(n, prior, "event_version_duplicate")
		}
		seenVersion[key] = n
		if n.id == r.event && (n.version != r.version || n.hash != r.projectionHash) {
			aiReversePair(n, r, "current_projection_event_conflict")
		}
		if n.version == r.version && n.id != r.event {
			aiReversePair(n, r, "current_projection_event_conflict")
		}
	}
	for _, r := range reqs {
		if r.version > 0 && s.byTable["ai_bridge_events"][r.event] == nil {
			aiReverseReason(r, "projection_event_orphan")
		}
	}
	for _, table := range []string{AIBridgeCommandSource, AILegacyCommandSource} {
		for _, n := range s.byTable[table] {
			r := reqs[n.request]
			if r == nil || !aiReverseSameOwner(n, r) || n.sourceKind == "start" && (n.id != r.id || n.hash != r.hash) || n.sourceKind != "start" && n.resource != r.resource {
				aiReversePair(n, r, "legacy_command_request_conflict")
				continue
			}
			if n.sourceKind == "start" && n.expectedBodyHash != r.expectedBodyHash {
				aiReversePair(n, r, "legacy_command_body_conflict")
			}
			if table == AILegacyCommandSource {
				op := ops[n.id]
				if op == nil || op.retired || op.hash != n.bodyHash || op.aggregate != n.request || !aiReverseSameOwner(n, op) {
					aiReversePair(n, op, "legacy_mapping_live_operation_conflict")
				}
				if !aiReverseLegacyTimeMatches(n, r) {
					aiReversePair(n, r, "legacy_mapping_original_time_conflict")
				}
			}
		}
	}
	for id, legacy := range s.byTable[AILegacyCommandSource] {
		old := s.byTable[AIBridgeCommandSource][id]
		if old == nil || old.request != legacy.request || old.sourceKind != legacy.sourceKind || old.hash != legacy.hash || old.payloadSHA != legacy.payloadSHA || old.expectedBodyHash != legacy.expectedBodyHash || old.attempts != legacy.attempts || old.sourceClock != legacy.sourceClock {
			aiReversePair(old, legacy, "legacy_mapping_source_conflict")
		}
		box := boxes[id]
		floor := legacy.attempts
		if floor > 8 {
			floor = 8
		}
		if box == nil || box.attempts < floor || box.attempts > 8 || box.bodyHash != legacy.bodyHash {
			aiReversePair(legacy, box, "legacy_mapping_inherited_budget_conflict")
		}
	}
	seqs := map[string]map[uint64]*aiReverseNode{}
	for _, op := range ops {
		if aiReverseRemote(op.kind) {
			if op.aggregate != op.resource {
				aiReverseReason(op, "remote_evaluation_owner_conflict")
			}
		} else {
			r := reqs[op.aggregate]
			if r == nil || !aiReverseSameOwner(op, r) {
				aiReversePair(op, r, "operation_request_owner_conflict")
			} else {
				op.request = r.id
				op.testee = r.testee
				if op.kind == pb.MessagingKind_START && op.resource != r.id || op.kind != pb.MessagingKind_START && op.resource != r.resource {
					aiReversePair(op, r, "operation_resource_conflict")
				}
				if op.receiptResource != "" && op.receiptResource != r.resource {
					aiReversePair(op, r, "operation_receipt_resource_conflict")
				}
			}
		}
		if !aiReverseAcceptedReceiptMatches(op, reqs[op.aggregate]) {
			aiReverseReason(op, "operation_original_receipt_kind_or_run_conflict")
		}
		box := boxes[op.id]
		if op.retired {
			if box != nil || op.receipt != "" {
				aiReversePair(op, box, "retired_id_has_live_message")
			}
			continue
		}
		if box == nil || box.kind != op.kind || box.bodyHash != op.bodyHash || box.aggregate != op.aggregate || box.sequence != op.sequence || box.org != op.org || box.subject != op.subject || box.resource != op.resource {
			aiReversePair(op, box, "operation_outbox_orphan_or_conflict")
		}
		if box != nil && ((op.state == "accepted" || op.state == "rejected") && box.state != "confirmed" || op.state != "accepted" && op.state != "rejected" && box.state == "confirmed" || op.state == "held" && box.state != "held") {
			aiReversePair(op, box, "operation_transport_decision_conflict")
		}
		if seqs[op.aggregate] == nil {
			seqs[op.aggregate] = map[uint64]*aiReverseNode{}
		}
		if prior := seqs[op.aggregate][op.sequence]; prior != nil {
			aiReversePair(op, prior, "aggregate_sequence_duplicate")
		}
		seqs[op.aggregate][op.sequence] = op
		if op.receipt != "" {
			in := inboxes[op.receipt]
			if in == nil || in.kind != pb.MessagingKind_COMMAND_RECEIPT || in.command != op.id || in.bodyHash != op.linkedHash || in.hash != op.bodyHash || in.aggregate != op.aggregate || in.decision != op.decision || in.receiptFamily != op.receiptFamily || in.receiptResource != op.receiptResource || in.receiptRun != op.receiptRun {
				aiReversePair(op, in, "operation_receipt_inbox_orphan_or_conflict")
			}
		}
	}
	for _, a := range s.byTable["ai_messaging_aggregates"] {
		group := seqs[a.id]
		if len(group) == 0 || a.sequence != uint64(len(group))+1 {
			aiReverseReason(a, "aggregate_sequence_coverage_conflict")
		} else {
			for i := uint64(1); i < a.sequence; i++ {
				if group[i] == nil {
					aiReverseReason(a, "aggregate_sequence_coverage_conflict")
					break
				}
			}
		}
		if r := reqs[a.id]; r != nil {
			a.request, a.org, a.subject, a.testee = r.id, r.org, r.subject, r.testee
		} else {
			for _, op := range group {
				if a.org != "" && a.org != op.org {
					aiReversePair(a, op, "aggregate_organization_conflict")
				}
				a.org = op.org
			}
		}
	}
	for aggregate, group := range seqs {
		if s.byTable["ai_messaging_aggregates"][aggregate] == nil {
			for _, op := range group {
				aiReverseReason(op, "aggregate_counter_orphan")
			}
		}
	}
	acks := map[string]*aiReverseNode{}
	for _, n := range boxes {
		if n.kind != pb.MessagingKind_EVENT_ACKNOWLEDGEMENT {
			if ops[n.id] == nil {
				aiReverseReason(n, "outbox_operation_orphan")
			}
			if !aiReverseRemote(n.kind) {
				r := reqs[n.aggregate]
				if r == nil || !aiReverseSameOwner(n, r) {
					aiReversePair(n, r, "outbox_request_owner_conflict")
				}
				if n.kind == pb.MessagingKind_START && r != nil && (n.bodyHash != r.expectedBodyHash || !reflect.DeepEqual(n.assessments, r.assessments)) {
					aiReversePair(n, r, "start_body_request_conflict")
				}
			}
			continue
		}
		in := inboxes[n.event]
		if prior := acks[n.event]; prior != nil {
			aiReversePair(n, prior, "acknowledgement_event_duplicate")
		}
		acks[n.event] = n
		if in == nil || in.ack != n.id || in.bodyHash != n.hash || in.kind != n.eventKind || in.aggregate != n.aggregate || in.org != "" && in.org != n.org {
			aiReversePair(n, in, "acknowledgement_inbox_orphan_or_conflict")
			continue
		}
		if in.state == "stored" && n.ackOutcome != pb.MessagingEventAcknowledgement_STORED || in.state == "held" && n.ackOutcome != pb.MessagingEventAcknowledgement_TECHNICALLY_HELD {
			aiReversePair(n, in, "acknowledgement_outcome_conflict")
		}
	}
	latestEval := map[string]*aiReverseNode{}
	for _, in := range inboxes {
		switch in.kind {
		case pb.MessagingKind_COMMAND_RECEIPT:
			op := ops[in.command]
			if op == nil || op.retired || in.id != op.receipt || in.aggregate != op.aggregate || in.hash != op.bodyHash {
				aiReversePair(in, op, "inbox_operation_orphan_or_conflict")
			} else {
				in.org, in.subject, in.request, in.testee = op.org, op.subject, op.request, op.testee
			}
		case pb.MessagingKind_INTERPRETATION_STATE:
			r := reqs[in.request]
			ev := s.byTable["ai_bridge_events"][in.id]
			if r == nil || !aiReverseSameOwner(in, r) || in.resource != r.resource || ev == nil || ev.request != in.request || ev.version != in.version || ev.hash != in.projectionHash {
				aiReversePair(in, ev, "inbox_interpretation_event_conflict")
			}
			if r != nil && in.version > r.version {
				aiReversePair(in, r, "inbox_interpretation_version_conflict")
			}
		case pb.MessagingKind_EVALUATION_STATE:
			state := s.byTable["ai_messaging_evaluation_states"][in.resource]
			if state == nil || state.org != in.org || in.version > state.version || in.sequence > state.sequence {
				aiReversePair(in, state, "inbox_evaluation_state_conflict")
			}
			if prior := latestEval[in.resource]; prior == nil || in.sequence > prior.sequence {
				latestEval[in.resource] = in
			} else if in.sequence == prior.sequence {
				aiReversePair(in, prior, "evaluation_event_sequence_duplicate")
			}
		default:
			aiReverseReason(in, "unsupported_inbox_kind")
		}
		if in.state != "held" && acks[in.id] == nil {
			aiReverseReason(in, "inbox_acknowledgement_orphan")
		}
	}
	// Receipt ownership is derived from the original live operation, so check
	// ACK organization a second time after those inboxes acquire their owner.
	for id, ack := range acks {
		in := inboxes[id]
		if in != nil {
			if ack.org != in.org {
				aiReversePair(ack, in, "acknowledgement_organization_conflict")
			}
			ack.request, ack.subject, ack.testee = in.request, in.subject, in.testee
		}
	}
	for _, state := range s.byTable["ai_messaging_evaluation_states"] {
		in := latestEval[state.id]
		if in == nil || in.sequence != state.sequence || in.version != state.version || in.bodyHash != state.bodyHash || in.org != state.org {
			aiReversePair(state, in, "evaluation_state_latest_reference_conflict")
		}
	}
	for _, f := range s.byTable["ai_messaging_failures"] {
		in := inboxes[f.id]
		if in == nil || in.kind != f.kind || in.bodyHash != f.bodyHash || in.wireHash != f.wireHash || in.aggregate != f.aggregate || f.attempts > 8 {
			aiReversePair(f, in, "failure_inbox_orphan_or_conflict")
		} else {
			f.org, f.subject, f.request, f.testee = in.org, in.subject, in.request, in.testee
			f.observation.Held = in.observation.Held
			f.observation.Unfinished = in.observation.Unfinished
		}
	}
	// Identity-only receipt binding does not imply an applied projection or
	// terminal business result. Require the actual original START graph while
	// leaving this still-pending request visibly unfinished.
	for _, r := range reqs {
		if !r.projectionAbsent || r.resource == "" {
			continue
		}
		op := ops[r.id]
		var box, in *aiReverseNode
		if op != nil {
			box = boxes[op.id]
			in = inboxes[op.receipt]
		}
		if op == nil || op.retired || op.kind != pb.MessagingKind_START || op.state != "accepted" || op.aggregate != r.id || !aiReverseAcceptedReceiptMatches(op, r) || op.observation.Invalid || box == nil || box.observation.Invalid || in == nil || in.observation.Invalid {
			aiReverseReason(r, "pending_session_original_start_receipt_unproven")
		}
	}
	// Handoff deliberately retains delivered=false in the old source. Only a
	// complete, exact mapping moves its transport responsibility to the actual
	// current operation/outbox. Their own held/pending state remains unchanged.
	for id, legacy := range s.byTable[AILegacyCommandSource] {
		old, op, box := s.byTable[AIBridgeCommandSource][id], ops[id], boxes[id]
		if old != nil && op != nil && box != nil && !op.retired && !old.observation.Invalid && !legacy.observation.Invalid && !op.observation.Invalid && !box.observation.Invalid {
			old.observation.Unfinished = false
		}
	}
	if len(s.byTable["ai_messaging_admission"]) != 1 {
		for _, n := range s.byTable["ai_messaging_admission"] {
			aiReverseReason(n, "admission_singleton_coverage_conflict")
		}
		s.structuralReasons = append(s.structuralReasons, "admission_singleton_coverage_conflict")
	}
	if len(s.byTable["ai_messaging_observations"]) != len(aiReverseObservationKinds) {
		s.structuralReasons = append(s.structuralReasons, "observation_kind_coverage_conflict")
	}
}

func aiReverseSortedKeys[V any](v map[string]V) []string {
	keys := make([]string, 0, len(v))
	for key := range v {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
func (s *AIReverseSnapshot) classify(scope *aiReverseScope) {
	base := append([]string(nil), s.structuralReasons...)
	s.report.Related, s.report.OutsideRetirement, s.report.Unknown, s.report.Blocking, s.report.OutsideActive = 0, 0, 0, uint64(len(s.structuralReasons)), 0
	s.report.SourceAuthenticationRequired = scope == nil
	unique := map[string]bool{}
	for _, r := range base {
		unique[r] = true
	}
	if scope != nil && scope.missingCurrent {
		unique["authenticated_old_ai_source_not_in_current_snapshot"] = true
		s.report.Blocking++
	}
	for _, n := range s.nodes {
		n.observation.Scope = "unknown"
		control := n.observation.Store == "ai_messaging_admission" || n.observation.Store == "ai_messaging_observations"
		if control {
			n.observation.Scope = "outside_retirement"
		} else if scope != nil && (scope.relatedRequests[n.request] || scope.relatedRequests[n.aggregate] || scope.relatedIDs[n.id] || scope.relatedIDs[n.command] || scope.relatedIDs[n.event]) {
			n.observation.Scope = "retirement_related"
		} else if scope != nil && !n.observation.Invalid && n.org != "" && n.observation.Store != "ai_messaging_quarantine" {
			n.observation.Scope = "outside_retirement"
		}
		if scope != nil && scope.identityConflicts[n.observation.Store+":"+n.id] {
			aiReverseReason(n, "source_identity_reused_or_current_source_changed")
		}
		switch n.observation.Scope {
		case "retirement_related":
			s.report.Related++
		case "outside_retirement":
			s.report.OutsideRetirement++
			if n.observation.Unfinished {
				s.report.OutsideActive++
			}
		default:
			s.report.Unknown++
		}
		blocking := n.observation.Invalid || n.observation.Scope == "unknown" || n.observation.Scope == "retirement_related" && (n.observation.Unfinished || n.observation.Held || len(n.observation.Reasons) > 0)
		if blocking {
			s.report.Blocking++
			for _, r := range n.observation.Reasons {
				unique[r] = true
			}
			if n.observation.Scope == "unknown" {
				unique["source_scope_or_business_owner_unknown"] = true
			}
			if n.observation.Scope == "retirement_related" && (n.observation.Unfinished || n.observation.Held) {
				unique["target_related_ai_responsibility_unfinished"] = true
			}
		}
		sort.Strings(n.observation.Reasons)
	}
	if scope != nil {
		s.scope = scope
		s.report.SourceScopeRetainedBudgetBytes = scope.reservation
		s.report.SourceCopies = scope.receipts
		s.report.SourceScopeSHA256 = scope.sha
	}
	s.report.BlockingReasons = aiReverseSortedKeys(unique)
	// A complete local reverse classification is a real observation, but not
	// origin approval, qs-ai remote settlement or a cross-service writer fence.
	s.report.GlobalReverseQualified, s.report.CASAuthority, s.report.DropReady = false, false, false
}
func (s *AIReverseSnapshot) dataDigest() string {
	h := sha256.New()
	sourceFrame(h, []byte("ai-reverse-full-data/v1"), false)
	sourceFrame(h, []byte(s.report.DatabaseIdentitySHA256), false)
	sourceFrame(h, []byte(strconv.FormatUint(s.head, 10)), false)
	sourceFrame(h, []byte(s.report.BusinessAnchorsSHA256), false)
	for _, l := range s.report.Ledgers {
		for _, v := range []string{l.Store, l.SchemaSHA256, l.PrimaryKeySHA256, l.UpperSHA256, l.RowsSHA256, strconv.FormatUint(l.Rows, 10), strconv.FormatUint(l.Bytes, 10)} {
			sourceFrame(h, []byte(v), false)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// BindAIReverseSourceScope borrows readers at their current cursors. The host
// must rewind/reopen them; this method never seeks, closes, or repairs a header.
// It reads all four streams to authenticated EOF before publishing scope. The
// same independent expected bounds and all original typed facts must match the
// real coordinator's private first pass. Relative-copy authentication remains
// distinct from production-origin or independent-approval authority.
func (c *HistoricalCoordinator) BindAIReverseSourceScope(ctx context.Context, s *AIReverseSnapshot, copies []SourceCopyInput) error {
	if c == nil || s == nil || s.self != s || s.scope != nil || len(copies) != 4 {
		return ErrAIReverseBinding
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.now == nil || c.alive(ctx) != nil || c.authenticated == nil || !c.authenticated.complete || c.authenticated.rows == nil || uint64(len(c.authenticated.rows)) != c.authenticated.entries || s.ValidateBorrowedSnapshot(ctx) != nil {
		return ErrAIReverseBinding
	}
	var expected [4]SourceCopyExpectation
	for i := range expected {
		expected[i] = c.copies[i].Expected
	}
	return bindAIReverseSourceFacts(ctx, s, copies, c, c.authenticated, expected, [2]string{c.binding.SourceSHA, c.binding.OperationID}, c.alive)
}

// This private adapter uses only a live, fully captured input epoch and its
// authenticated copies. An absent coordinator is intentional: the resulting
// scope cannot enter the existing coordinator-backed replay/write APIs.
func bindAIReverseInputSourceScope(ctx context.Context, s *AIReverseSnapshot, source *HistoricalSourceInputEpoch, copies []SourceCopyInput) error {
	if source == nil || !source.complete || source.alive(ctx) != nil || source.verifyFrozen(ctx) != nil {
		return ErrAIReverseBinding
	}
	authenticated := source.recipe.binding.copies
	if authenticated == nil || !authenticated.complete || authenticated.rows == nil || uint64(len(authenticated.rows)) != authenticated.entries {
		return ErrAIReverseBinding
	}
	return bindAIReverseSourceFacts(ctx, s, copies, nil, authenticated, source.recipe.binding.expected, [2]string{"historical-source-input/v1", source.recipe.hash}, source.alive)
}

func bindAIReverseSourceFacts(ctx context.Context, s *AIReverseSnapshot, copies []SourceCopyInput, owner *HistoricalCoordinator, authenticated *VerifiedSourceCopies, expected [4]SourceCopyExpectation, binding [2]string, check func(context.Context) error) error {
	if s == nil || s.self != s || s.scope != nil || len(copies) != 4 || check == nil || check(ctx) != nil || s.ValidateBorrowedSnapshot(ctx) != nil {
		return ErrAIReverseBinding
	}
	for i, input := range copies {
		if sourceReaderAbsent(input.Input) || !reflect.DeepEqual(input.Expected, expected[i]) {
			return ErrAIReverseBinding
		}
	}
	scope := &aiReverseScope{owner: owner, auth: authenticated, relatedRequests: map[string]bool{}, relatedIDs: map[string]bool{}, identityConflicts: map[string]bool{}}
	factsHash := sha256.New()
	sourceFrame(factsHash, []byte("ai-reverse-authenticated-source-facts/v1"), false)
	sourceFrame(factsHash, []byte(binding[0]), false)
	sourceFrame(factsHash, []byte(binding[1]), false)
	byAssessment, bySheet := map[string][]*aiReverseNode{}, map[string][]*aiReverseNode{}
	for _, r := range s.byTable["ai_bridge_requests"] {
		for _, id := range r.assessments {
			byAssessment[id] = append(byAssessment[id], r)
			if a := s.anchors[id]; a.id != "" {
				bySheet[a.sheet] = append(bySheet[a.sheet], r)
			}
		}
	}
	attachEvent := func(v *DecodedSourceEvent) error {
		if v == nil {
			return ErrAIReverseBinding
		}
		if _, e := authenticated.BindEvent(v); e != nil {
			return e
		}
		if e := aiReverseSourceFactFrame(factsHash, authenticated, v.Source.Database, v.Source.Object, v.Source.PrimaryKeySHA256, v.EventType); e != nil {
			return e
		}
		scope.verifiedEntries++
		owners := append([]*aiReverseNode(nil), byAssessment[v.BusinessIDs["assessment_id"]]...)
		owners = append(owners, bySheet[v.BusinessIDs["answersheet_id"]]...)
		owners = append(owners, bySheet[v.BusinessIDs["answer_sheet_id"]]...)
		for _, r := range owners {
			if e := s.scopeAdd(scope, scope.relatedRequests, r.id); e != nil {
				return e
			}
			if r.org != strconv.FormatUint(v.OrgID, 10) {
				scope.identityConflicts["ai_bridge_requests:"+r.id] = true
			}
		}
		for _, table := range []string{"ai_messaging_operations", "ai_messaging_outbox", "ai_messaging_inbox", "ai_bridge_events"} {
			if n := s.byTable[table][v.EventID]; n != nil {
				if e := s.scopeAdd(scope, scope.relatedIDs, v.EventID); e != nil {
					return e
				}
				scope.identityConflicts[table+":"+v.EventID] = true
			}
		}
		return nil
	}
	for i, input := range copies {
		if s.alive(ctx) != nil || check(ctx) != nil {
			return ErrAIReverseBounds
		}
		raw, e := json.Marshal(input.Expected)
		if e != nil {
			return ErrAIReverseBinding
		}
		sourceFrame(factsHash, []byte(strconv.Itoa(i)), false)
		sourceFrame(factsHash, raw, false)
		switch i {
		case 0:
			r, e := NewSQLSourceReader(input.Input, input.Expected)
			if e != nil {
				return e
			}
			for {
				v, e := r.Next()
				if e == io.EOF {
					break
				}
				if e != nil {
					return e
				}
				if e = attachEvent(v); e != nil {
					return e
				}
				if s.alive(ctx) != nil {
					return ErrAIReverseBounds
				}
			}
			scope.receipts[i] = r.Receipt()
		case 3:
			r, e := NewMongoSourceReader(input.Input, input.Expected)
			if e != nil {
				return e
			}
			for {
				v, e := r.Next()
				if e == io.EOF {
					break
				}
				if e != nil {
					return e
				}
				if e = attachEvent(v); e != nil {
					return e
				}
				if s.alive(ctx) != nil {
					return ErrAIReverseBounds
				}
			}
			scope.receipts[i] = r.Receipt()
		default:
			r, e := NewAISQLSourceReader(input.Input, input.Expected)
			if e != nil {
				return e
			}
			for {
				v, e := r.Next()
				if e == io.EOF {
					break
				}
				if e != nil {
					return e
				}
				if _, e = authenticated.BindAICommand(v); e != nil {
					return e
				}
				if e = aiReverseSourceFactFrame(factsHash, authenticated, v.Source.Database, v.Source.Object, v.Source.PrimaryKeySHA256, "ai-command/"+v.SourceKind); e != nil {
					return e
				}
				scope.verifiedEntries++
				if e := s.scopeAdd(scope, scope.relatedRequests, v.RequestID); e != nil {
					return e
				}
				if e := s.scopeAdd(scope, scope.relatedIDs, v.CommandID); e != nil {
					return e
				}
				n := s.byTable[input.Expected.Boundary.Name][v.CommandID]
				if n == nil {
					scope.missingCurrent = true
				} else if n.sourceRowHash != v.Source.Digest.SHA256 || n.request != v.RequestID || n.org != v.OrganizationID || n.subject != v.SubjectID || n.resource != v.ResourceID {
					scope.identityConflicts[input.Expected.Boundary.Name+":"+v.CommandID] = true
				}
				if s.alive(ctx) != nil {
					return ErrAIReverseBounds
				}
			}
			scope.receipts[i] = r.Receipt()
		}
		if !scope.receipts[i].Complete || scope.receipts[i].BusinessClosureVerified || scope.receipts[i].DropReady || !reflect.DeepEqual(scope.receipts[i], authenticated.receipts[i]) {
			return ErrAIReverseBinding
		}
		receiptRaw, e := json.Marshal(scope.receipts[i])
		if e != nil {
			return ErrAIReverseBinding
		}
		sourceFrame(factsHash, receiptRaw, false)
	}
	if scope.verifiedEntries != authenticated.entries {
		return ErrAIReverseBinding
	}
	scope.typedFactsSHA = hex.EncodeToString(factsHash.Sum(nil))
	if check(ctx) != nil || s.ValidateBorrowedSnapshot(ctx) != nil {
		return ErrAIReverseFresh
	}
	h := sha256.New()
	sourceFrame(h, []byte("ai-reverse-complete-source-scope/v1"), false)
	for i, r := range scope.receipts {
		raw, e := json.Marshal(copies[i].Expected)
		if e != nil {
			return ErrAIReverseBinding
		}
		sourceFrame(h, raw, false)
		sourceFrame(h, []byte(r.DataHash), false)
	}
	for _, key := range aiReverseSortedKeys(scope.relatedRequests) {
		sourceFrame(h, []byte("request"), false)
		sourceFrame(h, []byte(key), false)
	}
	for _, key := range aiReverseSortedKeys(scope.relatedIDs) {
		sourceFrame(h, []byte("id"), false)
		sourceFrame(h, []byte(key), false)
	}
	for _, key := range aiReverseSortedKeys(scope.identityConflicts) {
		sourceFrame(h, []byte("conflict"), false)
		sourceFrame(h, []byte(key), false)
	}
	scope.sha = hex.EncodeToString(h.Sum(nil))
	s.classify(scope)
	return nil
}

// AIReverseFreshProof is deliberately not an execution capability. Its actual
// old transaction must have ended and the complete new snapshot must match.
type AIReverseFreshProof struct {
	self                                           *AIReverseFreshProof
	old, fresh                                     *AIReverseSnapshot
	verifiedAt                                     time.Time
	anchor                                         *AIReverseRecheckAnchor
	current                                        *SQLResponsibilitySnapshot
	coordinator                                    *HistoricalCoordinator
	currentPool                                    gorm.ConnPool
	currentCycle, typedFactsSHA, classificationSHA string
}

func (*AIReverseFreshProof) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*AIReverseFreshProof) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*AIReverseFreshProof) String() string {
	return "private independently re-read AI reverse proof; no execution authority"
}
func (p *AIReverseFreshProof) GoString() string { return p.String() }
func (s *AIReverseSnapshot) oldTransactionEnded(ctx context.Context) error {
	if ctx == nil || s == nil || s.self != s || s.pool == nil {
		return ErrAIReverseFresh
	}
	qctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	rows, e := s.pool.QueryContext(qctx, "SELECT 1")
	if rows != nil {
		if rows.Close() != nil {
			return ErrAIReverseRead
		}
	}
	if !errors.Is(e, sql.ErrTxDone) {
		return ErrAIReverseFresh
	}
	return nil
}

// RecheckFresh requires a host-ended old Tx, a different actual current RRRO
// pool/cycle, and complete re-reading of every schema, row, upper and anchor.
// It cannot turn an active same transaction or only above-upper query into
// freshness. All connections, transactions and source readers remain borrowed.
func (s *AIReverseSnapshot) RecheckFresh(ctx context.Context, freshSnapshot *SQLResponsibilitySnapshot, expectedMigration uint64, coordinator *HistoricalCoordinator, copies []SourceCopyInput) (*AIReverseFreshProof, error) {
	if s == nil || s.self != s || time.Since(s.started) > s.limits.MaxDuration || !s.report.WholeLedgerEOF || freshSnapshot == nil || freshSnapshot == s.snapshot || freshSnapshot.cycle == nil {
		return nil, ErrAIReverseFresh
	}
	// The new epoch must have independently authenticated all four copies.
	// Neither a Summary nor reuse of the original coordinator authenticates
	// this current epoch's exact bounds, original keys and typed facts.
	if expectedMigration != 99 || expectedMigration != s.head || s.scope == nil || s.scope.owner == nil || s.scope.auth == nil || s.scope.auth != s.scope.owner.authenticated || coordinator == nil || coordinator == s.scope.owner || coordinator.authenticated == nil || coordinator.authenticated == s.scope.auth || !coordinator.authenticated.complete || coordinator.alive(ctx) != nil || len(copies) != 4 || coordinator.binding != s.scope.owner.binding {
		return nil, ErrAIReverseBinding
	}
	a, b := s.scope.auth, coordinator.authenticated
	if !a.complete || a.entries != b.entries || uint64(len(a.rows)) != a.entries || uint64(len(b.rows)) != b.entries || !reflect.DeepEqual(a.receipts, b.receipts) || !reflect.DeepEqual(a.rows, b.rows) || !reflect.DeepEqual(a.pairs, b.pairs) || !reflect.DeepEqual(a.eventIDs, b.eventIDs) {
		return nil, ErrAIReverseBinding
	}
	for i, input := range copies {
		if sourceReaderAbsent(input.Input) || !reflect.DeepEqual(input.Expected, s.scope.owner.copies[i].Expected) || !reflect.DeepEqual(input.Expected, coordinator.copies[i].Expected) {
			return nil, ErrAIReverseBinding
		}
	}
	if s.oldTransactionEnded(ctx) != nil {
		return nil, ErrAIReverseFresh
	}
	if freshSnapshot.ValidateBorrowedSnapshot(ctx) != nil || freshSnapshot.Report().CycleID == s.report.ResponsibilityCycleID {
		return nil, ErrAIReverseFresh
	}
	tx, e := hostmysql.RequireTx(ctx)
	if e != nil || tx.Statement == nil || tx.Statement.ConnPool == s.pool {
		return nil, ErrAIReverseFresh
	}
	if _, e = s.snapshot.RecheckFresh(ctx); e != nil {
		return nil, ErrAIReverseChanged
	}
	fresh, e := PrepareAIReverseSnapshot(ctx, freshSnapshot, s.head, s.limits)
	if e != nil {
		return nil, e
	}
	if e = coordinator.BindAIReverseSourceScope(ctx, fresh, copies); e != nil {
		return nil, e
	}
	if fresh.report.DatabaseIdentitySHA256 != s.report.DatabaseIdentitySHA256 || fresh.report.DataSHA256 != s.report.DataSHA256 || fresh.report.BusinessAnchorsSHA256 != s.report.BusinessAnchorsSHA256 || fresh.report.SourceScopeSHA256 != s.report.SourceScopeSHA256 || !reflect.DeepEqual(fresh.report.Ledgers, s.report.Ledgers) || !reflect.DeepEqual(fresh.report.SourceCopies, s.report.SourceCopies) || fresh.report.Related != s.report.Related || fresh.report.OutsideRetirement != s.report.OutsideRetirement || fresh.report.Unknown != s.report.Unknown || fresh.report.OutsideActive != s.report.OutsideActive || fresh.report.Blocking != s.report.Blocking || !reflect.DeepEqual(fresh.report.BlockingReasons, s.report.BlockingReasons) {
		return nil, ErrAIReverseChanged
	}
	if len(fresh.nodes) != len(s.nodes) {
		return nil, ErrAIReverseChanged
	}
	for i := range s.nodes {
		a, b := s.nodes[i].observation, fresh.nodes[i].observation
		if !reflect.DeepEqual(a, b) {
			return nil, ErrAIReverseChanged
		}
	}
	p := &AIReverseFreshProof{old: s, fresh: fresh, verifiedAt: time.Now().UTC()}
	p.self = p
	return p, nil
}

// Source identity sets also participate in the logical retained budget.
func (s *AIReverseSnapshot) scopeAdd(scope *aiReverseScope, target map[string]bool, id string) error {
	if target[id] {
		return nil
	}
	cost := uint64(512 + len(id))
	if scope.reservation > s.limits.MaxRetainedBytes-s.report.RetainedBudgetBytes || cost > s.limits.MaxRetainedBytes-s.report.RetainedBudgetBytes-scope.reservation {
		return ErrAIReverseBounds
	}
	scope.reservation += cost
	target[id] = true
	return nil
}

// Frame the actual private PK/facts entry only after the same streamed row has
// been authenticated. No sorted million-row copy of the original map is made.
func aiReverseSourceFactFrame(h hash.Hash, auth *VerifiedSourceCopies, database, object, pk, kind string) error {
	if h == nil || auth == nil || !auth.complete || auth.rows == nil || kind == "" {
		return ErrAIReverseBinding
	}
	key, err := sourceAuthKey(database, object, pk)
	if err != nil {
		return err
	}
	row, ok := auth.rows[key]
	if !ok {
		return ErrSourceAuthentication
	}
	sourceFrame(h, []byte{key.object}, false)
	sourceFrame(h, key.pk[:], false)
	sourceFrame(h, []byte(kind), false)
	sourceFrame(h, row.facts[:], false)
	return nil
}

// This ordered digest represents the actual private reverse classifications,
// independently of physical source bytes, typed facts and owner bindings.
func aiReverseClassificationSHA(s *AIReverseSnapshot) (string, error) {
	if s == nil || s.self != s || !s.report.WholeLedgerEOF || s.scope == nil || uint64(len(s.nodes)) != s.report.Rows {
		return "", ErrAIReverseBinding
	}
	h := sha256.New()
	sourceFrame(h, []byte("ai-reverse-complete-classification/v1"), false)
	sourceFrame(h, []byte(strconv.FormatUint(s.report.Rows, 10)), false)
	for _, n := range s.nodes {
		if n == nil {
			return "", ErrAIReverseBinding
		}
		v := n.observation
		if !evidenceHash(v.PrimaryKeySHA256) || !evidenceHash(v.RowSHA256) || (v.Scope != "retirement_related" && v.Scope != "outside_retirement" && v.Scope != "unknown") {
			return "", ErrAIReverseBinding
		}
		for _, text := range []string{v.Store, v.PrimaryKeySHA256, v.RowSHA256, v.Scope, strconv.FormatBool(v.Invalid), strconv.FormatBool(v.Unfinished), strconv.FormatBool(v.Held), strconv.Itoa(len(v.Reasons))} {
			sourceFrame(h, []byte(text), false)
		}
		for _, reason := range v.Reasons {
			sourceFrame(h, []byte(reason), false)
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type aiReverseSQLSeal struct {
	Version, Identity, BusinessAnchors, SchemaCoverage string
	Ledgers                                            [8]AIReverseLedgerSummary
	Observed, Related, Outside, Unknown, Blocking      uint64
}

func aiReverseSQLSnapshotSeal(s *SQLResponsibilitySnapshot) (aiReverseSQLSeal, error) {
	if s == nil || s.cycle == nil {
		return aiReverseSQLSeal{}, ErrAIReverseBinding
	}
	r := s.cycle.Report()
	if !r.ActualTransactionReadOnlyRR || r.CompletedAt.IsZero() || r.CycleID == "" || len(r.Ledgers) != 8 || !evidenceHash(r.DatabaseIdentitySHA256) || !evidenceHash(r.BusinessAnchorsSHA256) || r.DropReady {
		return aiReverseSQLSeal{}, ErrAIReverseBinding
	}
	value := aiReverseSQLSeal{Version: r.Version, Identity: r.DatabaseIdentitySHA256, BusinessAnchors: r.BusinessAnchorsSHA256, SchemaCoverage: r.SchemaCoverage, Observed: r.Observed, Related: r.RetirementRelated, Outside: r.OutsideRetirement, Unknown: r.Unknown, Blocking: r.Blocking}
	names := [8]string{"rm_outbox", "retry_event_hold", "event_delivery_dead_letter", "qs_rm_evaluation_request_ref", "qs_rm_gap_recovery_request", "qs_rm_replay_requests", "qs_rm_replay_items", "system_governance_action_runs"}
	for i, l := range r.Ledgers {
		if l.Store != names[i] || !evidenceHash(l.SchemaSHA256) || !evidenceHash(l.PrimaryKeySHA256) || !evidenceHash(l.UpperBoundSHA256) || !evidenceHash(l.RowsSHA256) {
			return aiReverseSQLSeal{}, ErrAIReverseBinding
		}
		value.Ledgers[i] = AIReverseLedgerSummary{l.Store, l.Rows, l.Bytes, l.Pages, l.SchemaSHA256, l.PrimaryKeySHA256, l.UpperBoundSHA256, l.RowsSHA256}
	}
	return value, nil
}

// All fields below are bounded metadata copied from an actual origin seal.
// There is deliberately no OriginCopyBinding/VerifiedSourceCopies pointer.
type aiReverseOriginSeal struct {
	Expected                                                     [4]SourceCopyExpectation
	Files                                                        [4]string
	FileBytes                                                    [4]uint64
	Limits                                                       SourceOriginLimits
	Started                                                      time.Time
	BindingSHA, OriginalEpochSHA, OriginalSeal                   string
	SQLCycle, SQLIdentity, SQLHead, MongoIdentity, MongoMetadata string
	Transaction                                                  mongoCycleTxn
	Receipts                                                     [4]SourceCopyReceipt
	Boundaries                                                   [4]SourceBoundary
}

// AIReverseRecheckAnchor is a private, graphless, same-process capability. It
// retains only the original borrowed Tx, fixed metadata and sealed digests;
// never old source/coordinator/SQL8/AI14 graphs or per-row scope maps. Keeping
// this Tx does not end it, close its borrowed pool, stop writers or grant CAS.
type AIReverseRecheckAnchor struct {
	self                                  *AIReverseRecheckAnchor
	pool                                  gorm.ConnPool
	poolToken                             string
	started, sealedAt, coordinatorStarted time.Time
	coordinatorDuration                   time.Duration
	limits                                AIReverseLimits
	head                                  uint64
	binding                               HistoricalCoordinatorBinding
	expected                              [4]SourceCopyExpectation
	receipts                              [4]SourceCopyReceipt
	entries                               uint64
	typedFactsSHA, classificationSHA      string
	sqlCycle                              string
	sql                                   aiReverseSQLSeal
	ai                                    AIReverseSummary
	origin                                aiReverseOriginSeal
	seal                                  string
}

func (*AIReverseRecheckAnchor) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*AIReverseRecheckAnchor) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*AIReverseRecheckAnchor) UnmarshalJSON([]byte) error   { return ErrSourceSerialization }
func (*AIReverseRecheckAnchor) UnmarshalBSON([]byte) error   { return ErrSourceSerialization }
func (*AIReverseRecheckAnchor) String() string {
	return "private graphless AI/origin anchor; no closure or execution authority"
}
func (a *AIReverseRecheckAnchor) GoString() string { return a.String() }
func aiReversePoolToken(pool gorm.ConnPool) (string, error) {
	if pool == nil {
		return "", ErrAIReverseFresh
	}
	v := reflect.ValueOf(pool)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return "", ErrAIReverseFresh
	}
	return aiReverseHash("actual-borrowed-pool/v1", v.Type().String(), strconv.FormatUint(uint64(v.Pointer()), 10)), nil
}
func (a *AIReverseRecheckAnchor) factsDigest() (string, error) {
	if a == nil {
		return "", ErrAIReverseBinding
	}
	return sourceOriginDigest(struct {
		Pool                                              string
		Started, Sealed, Coordinator                      time.Time
		CoordinatorDuration                               time.Duration
		Limits                                            AIReverseLimits
		Head                                              uint64
		Binding                                           HistoricalCoordinatorBinding
		Expected                                          [4]SourceCopyExpectation
		Receipts                                          [4]SourceCopyReceipt
		Entries                                           uint64
		Typed, Classification, SQLCycle, MongoTransaction string
		SQL                                               aiReverseSQLSeal
		AI                                                AIReverseSummary
		Origin                                            aiReverseOriginSeal
	}{a.poolToken, a.started, a.sealedAt, a.coordinatorStarted, a.coordinatorDuration, a.limits, a.head, a.binding, a.expected, a.receipts, a.entries, a.typedFactsSHA, a.classificationSHA, a.sqlCycle, mongoOwnerHashParts("actual-mongo-session-txn/v1", string(a.origin.Transaction.session), originMongoNumber(a.origin.Transaction.number)), a.sql, a.ai, a.origin})
}
func (a *AIReverseRecheckAnchor) intact() bool {
	if a == nil || a.self != a || a.pool == nil || a.head != 99 || a.seal == "" || a.sqlCycle == "" || a.started.IsZero() || a.sealedAt.Before(a.started) || a.coordinatorStarted.IsZero() || a.coordinatorStarted.After(a.sealedAt) || a.coordinatorDuration <= 0 || !a.limits.valid() || !a.origin.Limits.valid() || a.origin.Started.IsZero() || len(a.origin.Transaction.session) == 0 || len(a.origin.Transaction.session) > 4096 || len(a.ai.Ledgers) != 14 || !a.ai.WholeLedgerEOF || a.ai.SourceAuthenticationRequired || a.ai.GlobalReverseQualified || a.ai.CASAuthority || a.ai.DropReady || !evidenceHash(a.typedFactsSHA) || !evidenceHash(a.classificationSHA) {
		return false
	}
	token, err := aiReversePoolToken(a.pool)
	if err != nil || token != a.poolToken {
		return false
	}
	digest, err := a.factsDigest()
	return err == nil && digest == a.seal
}
func (a *AIReverseRecheckAnchor) alive(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil || !a.intact() || time.Since(a.started) > a.limits.MaxDuration || time.Since(a.origin.Started) > a.origin.Limits.MaxDuration || time.Since(a.coordinatorStarted) > a.coordinatorDuration {
		return ErrAIReverseFresh
	}
	return nil
}

// Every native/read loop receives the earliest original absolute deadline.
// Rebuilding a second graph cannot obtain a fresh now+duration budget.
func (a *AIReverseRecheckAnchor) recheckContext(ctx context.Context) (context.Context, context.CancelFunc, error) {
	if a.alive(ctx) != nil {
		return nil, nil, ErrAIReverseFresh
	}
	deadline := a.started.Add(a.limits.MaxDuration)
	for _, candidate := range []time.Time{a.origin.Started.Add(a.origin.Limits.MaxDuration), a.coordinatorStarted.Add(a.coordinatorDuration)} {
		if candidate.Before(deadline) {
			deadline = candidate
		}
	}
	scope, cancel := context.WithDeadline(ctx, deadline)
	return scope, cancel, nil
}

// Freeze happens inside the original active paired epoch and consumes only
// actual opaque handles. No report/clock/bool supplied by a caller can mint it.
func (s *AIReverseSnapshot) FreezeFreshAnchor(ctx context.Context, origin *SourceOriginEpoch) (*AIReverseRecheckAnchor, error) {
	if s == nil || s.self != s || origin == nil || s.scope == nil || s.scope.owner == nil || s.scope.auth == nil || s.scope.auth != s.scope.owner.authenticated || s.scope.verifiedEntries != s.scope.auth.entries || !evidenceHash(s.scope.typedFactsSHA) || s.ValidateBorrowedSnapshot(ctx) != nil || origin.binding == nil || origin.binding.copies != s.scope.auth || origin.sql != s.snapshot.cycle || origin.sqlConnection != s.pool {
		return nil, ErrAIReverseBinding
	}
	c := s.scope.owner
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.alive(ctx) != nil || !c.coverage || c.failed || !c.authenticated.complete || c.authenticated.entries != uint64(len(c.authenticated.rows)) || c.receipts != c.authenticated.receipts || c.receipts != s.scope.receipts {
		return nil, ErrAIReverseBinding
	}
	originAnchor, err := origin.FreezeFreshRecheckAnchor(ctx)
	if err != nil {
		return nil, err
	}
	sqlFacts, err := aiReverseSQLSnapshotSeal(s.snapshot)
	if err != nil {
		return nil, err
	}
	classification, err := aiReverseClassificationSHA(s)
	if err != nil {
		return nil, err
	}
	poolToken, err := aiReversePoolToken(s.pool)
	if err != nil {
		return nil, err
	}
	if len(s.report.BlockingReasons) > 256 {
		return nil, ErrAIReverseBounds
	}
	reasonBytes := 0
	for _, reason := range s.report.BlockingReasons {
		reasonBytes += len(reason)
	}
	if reasonBytes > 64<<10 {
		return nil, ErrAIReverseBounds
	}
	a := &AIReverseRecheckAnchor{pool: s.pool, poolToken: poolToken, started: s.started, sealedAt: time.Now(), coordinatorStarted: c.started, coordinatorDuration: c.limits.MaxDuration, limits: s.limits, head: s.head, binding: c.binding, receipts: s.scope.receipts, entries: s.scope.verifiedEntries, typedFactsSHA: s.scope.typedFactsSHA, classificationSHA: classification, sqlCycle: s.report.ResponsibilityCycleID, sql: sqlFacts, ai: s.Summary()}
	a.self = a
	for i, input := range c.copies {
		a.expected[i] = input.Expected
		if input.Expected != origin.binding.expected[i] {
			return nil, ErrAIReverseBinding
		}
	}
	b := originAnchor.binding
	a.origin = aiReverseOriginSeal{Expected: b.expected, Files: b.fileHashes, FileBytes: b.fileBytes, Limits: b.limits, Started: b.started, BindingSHA: b.hash, OriginalEpochSHA: originAnchor.originalEpochHash, OriginalSeal: originAnchor.seal, SQLCycle: originAnchor.sqlCycleID, SQLIdentity: originAnchor.sqlIdentity, SQLHead: originAnchor.sqlHead, MongoIdentity: originAnchor.mongoIdentity, MongoMetadata: originAnchor.mongoMetadata, Transaction: mongoCycleTxn{session: bytes.Clone(originAnchor.transaction.session), number: originAnchor.transaction.number}, Receipts: originAnchor.receipts, Boundaries: originAnchor.boundaries}
	if a.sqlCycle != a.origin.SQLCycle || a.sql.Identity != a.origin.SQLIdentity || a.receipts != a.origin.Receipts {
		return nil, ErrAIReverseBinding
	}
	a.seal, err = a.factsDigest()
	if err != nil || a.alive(ctx) != nil {
		return nil, ErrAIReverseBinding
	}
	return a, nil
}
func (a *AIReverseRecheckAnchor) oldTransactionEnded(ctx context.Context) error {
	if a.alive(ctx) != nil {
		return ErrAIReverseFresh
	}
	qctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	rows, err := a.pool.QueryContext(qctx, "SELECT 1")
	if rows != nil && rows.Close() != nil {
		return ErrAIReverseRead
	}
	if !errors.Is(err, sql.ErrTxDone) {
		return ErrAIReverseFresh
	}
	return nil
}

// c.started is set by the real coordinator constructor, never a public clock.
// Requiring construction after the frozen first epoch rejects reuse without
// retaining an old owner/auth pointer. All original deadlines stay unchanged.
// c.failed and all consumed/qualified coordinator state follow c.mu's existing
// protocol. Keep that lock only for metadata inspection: native SQL validation
// and the later independently locking BindAIReverseSourceScope run unlocked.
func (a *AIReverseRecheckAnchor) currentCoordinatorState(ctx context.Context, c *HistoricalCoordinator) error {
	if a == nil || c == nil {
		return ErrAIReverseBinding
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.now == nil || !c.started.After(a.sealedAt) || c.alive(ctx) != nil || c.authenticated == nil || !c.authenticated.complete || c.authenticated.entries != uint64(len(c.authenticated.rows)) || c.authenticated.entries != a.entries || c.binding != a.binding || c.authenticated.receipts != a.receipts {
		return ErrAIReverseBinding
	}
	for i, input := range c.copies {
		if input.Expected != a.expected[i] {
			return ErrAIReverseBinding
		}
	}
	return nil
}
func (a *AIReverseRecheckAnchor) validateCurrent(ctx context.Context, s *SQLResponsibilitySnapshot, c *HistoricalCoordinator) error {
	if a.alive(ctx) != nil || s == nil || s.cycle == nil || a.currentCoordinatorState(ctx, c) != nil {
		return ErrAIReverseBinding
	}
	if s.ValidateBorrowedSnapshot(ctx) != nil || s.Report().CycleID == a.sqlCycle {
		return ErrAIReverseFresh
	}
	tx, err := hostmysql.RequireTx(ctx)
	if err != nil || tx.Statement == nil || tx.Statement.ConnPool == a.pool {
		return ErrAIReverseFresh
	}
	if _, err = sdkmysql.BindGORM(tx); err != nil {
		return ErrAIReverseFresh
	}
	if a.alive(ctx) != nil {
		return ErrAIReverseFresh
	}
	return a.currentCoordinatorState(ctx, c)
}

// The supplied second opaque SQL cycle already performed a genuine complete
// 8-ledger scan in this new Tx. Compare its sealed raw/schema/upper/owner facts,
// then independently rescan all AI14 and four authenticated source streams.
func (a *AIReverseRecheckAnchor) RecheckFresh(ctx context.Context, current *SQLResponsibilitySnapshot, expectedMigration uint64, c *HistoricalCoordinator, copies []SourceCopyInput) (*AIReverseFreshProof, error) {
	if expectedMigration != 99 || a.alive(ctx) != nil || expectedMigration != a.head || len(copies) != 4 {
		return nil, ErrAIReverseFresh
	}
	scope, cancel, err := a.recheckContext(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	ctx = scope
	if a.oldTransactionEnded(ctx) != nil || a.validateCurrent(ctx, current, c) != nil {
		return nil, ErrAIReverseFresh
	}
	sqlFacts, err := aiReverseSQLSnapshotSeal(current)
	if err != nil || sqlFacts != a.sql {
		return nil, ErrAIReverseChanged
	}
	fresh, err := PrepareAIReverseSnapshot(ctx, current, expectedMigration, a.limits)
	if err != nil {
		return nil, err
	}
	if err = c.BindAIReverseSourceScope(ctx, fresh, copies); err != nil {
		return nil, err
	}
	classification, err := aiReverseClassificationSHA(fresh)
	if err != nil {
		return nil, err
	}
	// Canonical summaries preserve equal empty reason sets across both epochs.
	old, new := a.ai, fresh.Summary()
	if new.DatabaseIdentitySHA256 != old.DatabaseIdentitySHA256 || new.DataSHA256 != old.DataSHA256 || new.BusinessAnchorsSHA256 != old.BusinessAnchorsSHA256 || new.SourceScopeSHA256 != old.SourceScopeSHA256 || !reflect.DeepEqual(new.Ledgers, old.Ledgers) || new.SourceCopies != old.SourceCopies || new.Related != old.Related || new.OutsideRetirement != old.OutsideRetirement || new.Unknown != old.Unknown || new.OutsideActive != old.OutsideActive || new.Blocking != old.Blocking || !reflect.DeepEqual(new.BlockingReasons, old.BlockingReasons) || fresh.scope.typedFactsSHA != a.typedFactsSHA || fresh.scope.verifiedEntries != a.entries || classification != a.classificationSHA {
		return nil, ErrAIReverseChanged
	}
	if fresh.ValidateBorrowedSnapshot(ctx) != nil || a.validateCurrent(ctx, current, c) != nil || a.alive(ctx) != nil {
		return nil, ErrAIReverseFresh
	}
	proof := &AIReverseFreshProof{anchor: a, current: current, coordinator: c, currentPool: fresh.pool, currentCycle: current.Report().CycleID, typedFactsSHA: fresh.scope.typedFactsSHA, classificationSHA: classification, verifiedAt: time.Now().UTC()}
	proof.self = proof
	return proof, nil
}

// Only a genuine freshAI proof from this same graphless anchor can temporarily
// bind the immutable original origin metadata to this second actual auth. The
// old binding/start/limits/file hashes are not rewritten or newly approved.
// The original full SQL3/Mongo1 origin verifier remains the executor; no old
// SourceOrigin/OriginCopyBinding/VerifiedSourceCopies reference is retained.
func (a *AIReverseRecheckAnchor) RecheckOrigin(ctx context.Context, p *AIReverseFreshProof, current *SQLResponsibilitySnapshot, mgo *MongoResponsibilitySnapshot, c *HistoricalCoordinator, readers []io.Reader) (*SourceOriginRecheckProof, error) {
	if a == nil || a.alive(ctx) != nil || p == nil || p.self != p || p.anchor != a || p.current != current || p.coordinator != c || p.old != nil || p.fresh != nil || p.typedFactsSHA != a.typedFactsSHA || p.classificationSHA != a.classificationSHA || p.currentCycle != current.Report().CycleID || a.oldTransactionEnded(ctx) != nil || a.validateCurrent(ctx, current, c) != nil || mgo == nil || len(readers) != 4 {
		return nil, ErrAIReverseFresh
	}
	borrowed, ok := ctx.(mongo.SessionContext)
	if !ok {
		return nil, ErrAIReverseFresh
	}
	session := mongo.SessionFromContext(borrowed)
	if session == nil {
		return nil, ErrAIReverseFresh
	}
	scope, cancel, err := a.recheckContext(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	// Preserve the same borrowed session on the original absolute deadline.
	ctx = mongo.NewSessionContext(scope, session)
	tx, err := hostmysql.RequireTx(ctx)
	if err != nil || tx.Statement == nil || tx.Statement.ConnPool != p.currentPool {
		return nil, ErrAIReverseFresh
	}
	o := a.origin
	binding := &OriginCopyBinding{copies: c.authenticated, expected: o.Expected, fileHashes: o.Files, fileBytes: o.FileBytes, limits: o.Limits, started: o.Started, hash: o.BindingSHA}
	original := &FreshRecheckAnchor{binding: binding, sqlConnection: a.pool, sqlCycleID: o.SQLCycle, sqlIdentity: o.SQLIdentity, sqlHead: o.SQLHead, mongoIdentity: o.MongoIdentity, mongoMetadata: o.MongoMetadata, transaction: mongoCycleTxn{session: bytes.Clone(o.Transaction.session), number: o.Transaction.number}, receipts: o.Receipts, boundaries: o.Boundaries, originalEpochHash: o.OriginalEpochSHA, seal: o.OriginalSeal}
	if !original.intact() || binding.alive(ctx) != nil || a.alive(ctx) != nil {
		return nil, ErrAIReverseBinding
	}
	proof, err := original.RecheckSnapshots(ctx, current, mgo, readers)
	if err != nil {
		return nil, err
	}
	if a.alive(ctx) != nil || a.validateCurrent(ctx, current, c) != nil {
		return nil, ErrAIReverseFresh
	}
	return proof, nil
}

// PrepareAIHistoricalComponentSnapshot reads current local AI responsibilities
// in the host's same actual SQL/Mongo component scopes. The two full inputs are
// immutable selection data, never a renewed global reverse/write proof. The
// host must ValidateFrozen on both complete pairs before its first write.
func PrepareAIHistoricalComponentSnapshot(parent context.Context, component *HistoricalCASComponent, pair *AIHistoricalInputPair, source *HistoricalComponentSourceObservation, limits AIReverseLimits) (*AIReverseSnapshot, error) {
	if parent == nil || !limits.valid() || limits.MaxDuration > 20*time.Second || limits.MaxRetainedBytes > aiHistoricalInputMaxRetained || source == nil || source.component != component || source.ValidateBorrowedObservation(parent) != nil || aiComponentPairIntact(parent, pair, source.pair) != nil {
		return nil, ErrAIReverseBinding
	}
	tx, err := hostmysql.RequireTx(parent)
	if err != nil || tx.Statement == nil || tx.Statement.ConnPool == nil || tx.Statement.ConnPool == pair.first.pool || tx.Statement.ConnPool == pair.second.pool {
		return nil, ErrAIReverseFresh
	}
	if _, err = sdkmysql.BindGORM(tx); err != nil {
		return nil, ErrAIReverseFresh
	}
	s := &AIReverseSnapshot{pool: tx.Statement.ConnPool, head: 99, limits: limits, started: time.Now(), byTable: map[string]map[string]*aiReverseNode{}, anchors: map[string]aiReverseAnchor{}, component: component, componentSource: source, componentPair: pair, componentSeal: mongoHistoricalComponentInputSeal(component)}
	s.self = s
	s.report = AIReverseSummary{Version: "ai-reverse-component/v1", MigrationVersion: 99, DatabaseIdentitySHA256: pair.second.report.DatabaseIdentitySHA256, StartedAt: s.started.UTC(), SourceAuthenticationRequired: true, ExternalOriginRequired: true, ExternalQSAIClosureRequired: true, StoredWireAuthenticationRequired: true, WriterFenceRequired: true, UnboundOrphanNegativeClosureRequired: true, NewOwnerOrganizationNegativeClosureRequired: true}
	bounded, cancel := context.WithDeadline(parent, s.started.Add(limits.MaxDuration))
	defer cancel()
	ctx := mongo.NewSessionContext(bounded, mongo.SessionFromContext(parent))
	if err = s.validateComponentScope(ctx); err != nil {
		return nil, err
	}
	scope := &aiReverseScope{relatedRequests: map[string]bool{}, relatedIDs: map[string]bool{}, identityConflicts: map[string]bool{}, receipts: pair.second.report.SourceCopies, sha: aiReverseHash("ai-component-scope/v1", s.componentSeal, pair.second.componentIndexSHA, source.rowsSHA)}
	assessments, resources, owners := map[string]bool{}, map[string]bool{}, map[string]string{}
	pages := map[int]bool{}
	originalRequests := map[string]*aiReverseNode{}
	for _, frame := range component.inputs {
		for _, owner := range frame.owners {
			id := strconv.FormatUint(owner.id, 10)
			if owner.kind == "assessment" {
				assessments[id] = true
				owners[id] = strconv.FormatUint(owner.org, 10)
			}
			for _, request := range pair.second.componentRequests[owner.kind+":"+id] {
				scope.relatedRequests[request] = true
			}
		}
		ids, err := frame.sqlRecipe.SourceEventIDs()
		if err != nil {
			return nil, ErrAIReverseBinding
		}
		for _, id := range ids {
			scope.relatedIDs[id] = true
		}
	}
	for id := range scope.relatedRequests {
		for _, page := range pair.second.componentPages["request:"+id] {
			pages[page] = true
		}
	}
	for id := range scope.relatedIDs {
		for _, page := range pair.second.componentPages["id:"+id] {
			pages[page] = true
		}
	}
	// Only selected protected pages are reread here. Full EOF/page verification
	// belongs to the one aggregate pre-write boundary, not every component.
	for _, page := range aiComponentPageOrder(pages) {
		rows, spec, meta, err := pair.second.componentPage(ctx, page)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			n := s.decode(spec, row, meta)
			if scope.relatedRequests[n.request] || scope.relatedRequests[n.aggregate] || scope.relatedIDs[n.id] || scope.relatedIDs[n.command] || scope.relatedIDs[n.event] {
				if spec.table == "ai_bridge_requests" {
					originalRequests[n.id] = n
				}
				aiComponentExpand(scope, assessments, resources, n)
			}
		}
	}
	// Resolve actual current SQL owners for every sheet, including input-only
	// SQL absence sheets. These are native rows, not terminal/owner assertions.
	sheets := map[string]bool{}
	for _, frame := range component.inputs {
		for _, owner := range frame.owners {
			if owner.kind == "sheet" {
				id := strconv.FormatUint(owner.id, 10)
				sheets[id] = true
				org := strconv.FormatUint(owner.org, 10)
				if prior := owners["sheet:"+id]; prior != "" && prior != org {
					return nil, ErrAIReverseBinding
				}
				owners["sheet:"+id] = org
			}
		}
	}
	if len(sheets) > 0 {
		pred, args, err := aiComponentIn("answer_sheet_id", sheets, true)
		if err != nil {
			return nil, err
		}
		rows, _, size, err := s.read(ctx, "SELECT CAST(id AS BINARY) AS id,CAST(org_id AS BINARY) AS org_id,CAST(answer_sheet_id AS BINARY) AS answer_sheet_id FROM assessment WHERE "+pred+" ORDER BY id LIMIT 4097", 4096, args...)
		if err != nil {
			return nil, err
		}
		if uint64(len(rows)) > limits.MaxRows || size > limits.MaxBytes-s.report.Bytes {
			return nil, ErrAIReverseBounds
		}
		s.report.Bytes += size
		for _, row := range rows {
			if !aiPositiveNumber(row.text("id")) || !aiPositiveNumber(row.text("org_id")) || !sheets[row.text("answer_sheet_id")] {
				return nil, ErrAIReverseBinding
			}
			if owners["sheet:"+row.text("answer_sheet_id")] != row.text("org_id") || owners[row.text("id")] != "" && owners[row.text("id")] != row.text("org_id") {
				return nil, ErrAIReverseBinding
			}
			assessments[row.text("id")] = true
			owners[row.text("id")] = row.text("org_id")
		}
	}
	for i, spec := range aiReverseSpecs {
		meta, err := s.schema(ctx, spec)
		if err != nil || !reflect.DeepEqual(meta, pair.second.metadata[i]) {
			return nil, ErrAIReverseSchema
		}
		s.metadata = append(s.metadata, meta)
		s.byTable[spec.table] = map[string]*aiReverseNode{}
	}
	queries := map[string]string{}
	stable := false
	for pass := 0; pass < 16; pass++ {
		changed := false
		for i, spec := range aiReverseSpecs {
			predicate, args, err := aiComponentPredicate(spec, scope, assessments, resources)
			if err != nil {
				return nil, err
			}
			signature := aiReverseHash(predicate, fmtAIComponentArgs(args))
			if queries[spec.table] == signature {
				continue
			}
			queries[spec.table] = signature
			changed = true
			if predicate == "" {
				continue
			}
			meta := s.metadata[i]
			projection, order := []string{}, []string{}
			for _, name := range meta.columns {
				projection = append(projection, "CAST(`"+name+"` AS BINARY) AS `"+name+"`")
			}
			for _, name := range spec.keys {
				order = append(order, "`"+name+"`")
			}
			max := int(s.limits.MaxRows)
			if max > 100000 {
				max = 100000
			}
			args = append(args, max+1)
			rows, names, _, err := s.read(ctx, "SELECT "+strings.Join(projection, ",")+" FROM `"+spec.table+"` WHERE "+predicate+" ORDER BY "+strings.Join(order, ",")+" LIMIT ?", max, args...)
			if err != nil || !reflect.DeepEqual(names, meta.columns) {
				if err != nil {
					return nil, err
				}
				return nil, ErrAIReverseSchema
			}
			for _, row := range rows {
				key, err := aiReverseKey(spec, row)
				if err != nil {
					return nil, err
				}
				n := s.decode(spec, row, meta)
				n.observation.Store = spec.table
				n.observation.PrimaryKeySHA256 = aiReverseKeySHA(key)
				n.observation.RowSHA256 = aiReverseRowSHA(meta.columns, row)
				if old := s.byTable[spec.table][n.id]; old != nil {
					if old.observation.PrimaryKeySHA256 != n.observation.PrimaryKeySHA256 || old.observation.RowSHA256 != n.observation.RowSHA256 {
						return nil, ErrAIReverseChanged
					}
					continue
				}
				rawSize := uint64(0)
				for _, v := range row {
					rawSize += uint64(len(v))
				}
				cost := uint64(2048) + rawSize
				if s.report.Rows >= s.limits.MaxRows || rawSize > s.limits.MaxBytes-s.report.Bytes || cost > s.limits.MaxRetainedBytes-s.report.RetainedBudgetBytes {
					return nil, ErrAIReverseBounds
				}
				s.report.Rows++
				s.report.Bytes += rawSize
				s.report.RetainedBudgetBytes += cost
				s.byTable[spec.table][n.id] = n
				s.nodes = append(s.nodes, n)
				if spec.table != "ai_messaging_admission" && spec.table != "ai_messaging_observations" && spec.table != "ai_messaging_quarantine" {
					aiComponentExpand(scope, assessments, resources, n)
				}
			}
		}
		if !changed {
			stable = true
			break
		}
	}
	if !stable {
		return nil, ErrAIReverseBounds
	}
	for id, original := range originalRequests {
		current := s.byTable["ai_bridge_requests"][id]
		if current == nil || current.hash != original.hash || current.expectedBodyHash != original.expectedBodyHash || current.org != original.org || current.subject != original.subject || current.testee != original.testee || !reflect.DeepEqual(current.assessments, original.assessments) || original.resource != "" && current.resource != original.resource {
			return nil, ErrAIReverseChanged
		}
	}
	if err = s.readAssessmentAnchors(ctx); err != nil {
		return nil, err
	}
	for id, org := range owners {
		if strings.HasPrefix(id, "sheet:") {
			continue
		}
		if anchor, ok := s.anchors[id]; ok && anchor.org != org {
			return nil, ErrAIReverseBinding
		}
	}
	s.reverse()
	s.classify(scope)
	// Preserve unknown/held/orphan facts; a scoped absence never means the
	// whole organization, external qs-ai or unbound native rows are clear.
	s.report.SourceAuthenticationRequired = true
	for i, spec := range aiReverseSpecs {
		meta, err := s.schema(ctx, spec)
		if err != nil || !reflect.DeepEqual(meta, s.metadata[i]) {
			return nil, ErrAIReverseSchema
		}
	}
	if err = s.validateComponentScope(ctx); err != nil {
		return nil, err
	}
	s.scope = scope
	s.componentAssessments, s.componentResources = assessments, resources
	s.report.DataSHA256 = aiComponentDataDigest(s)
	s.report.CompletedAt = time.Now().UTC()
	s.componentComplete = true
	return s, nil
}

func aiComponentPairIntact(ctx context.Context, p *AIHistoricalInputPair, sources *HistoricalSourceInputPair) error {
	if p == nil || p.self != p || sources == nil || p.sources != sources || p.first == nil || p.second == nil || p.first == p.second || !p.first.complete || !p.second.complete || p.first.validFile(ctx) != nil || p.second.validFile(ctx) != nil || p.first.source != sources.first || p.second.source != sources.second || p.first.pool == p.second.pool || p.first.cycle == p.second.cycle || p.first.componentIndexSHA == "" || p.second.componentIndexSHA == "" || len(p.second.metadata) != len(aiReverseSpecs) || p.first.report.DataSHA256 != p.second.report.DataSHA256 || p.first.report.DatabaseIdentitySHA256 != p.second.report.DatabaseIdentitySHA256 || sourceComponentPairIntact(ctx, sources) != nil {
		return ErrAIReverseBinding
	}
	return nil
}
func (s *AIReverseSnapshot) validateComponentScope(ctx context.Context) error {
	if s == nil || s.self != s || s.snapshot != nil || s.componentSource == nil || s.componentSource.component != s.component || s.componentSource.ValidateBorrowedObservation(ctx) != nil || s.componentSeal == "" || s.componentSeal != mongoHistoricalComponentInputSeal(s.component) || s.alive(ctx) != nil || aiComponentPairIntact(ctx, s.componentPair, s.componentSource.pair) != nil {
		return ErrAIReverseFresh
	}
	tx, err := hostmysql.RequireTx(ctx)
	if err != nil || tx.Statement == nil || tx.Statement.ConnPool != s.pool {
		return ErrAIReverseFresh
	}
	rows, _, _, err := s.read(ctx, "SELECT @@server_uuid AS server,DATABASE() AS db,VERSION() AS version", 1)
	if err != nil || len(rows) != 1 || !strings.HasPrefix(rows[0].text("version"), "8.") || aiReverseHash("mysql_database_identity_v1", rows[0].text("server"), rows[0].text("db")) != s.componentPair.second.report.DatabaseIdentitySHA256 {
		return ErrAIReverseBinding
	}
	rows, _, _, err = s.read(ctx, "SELECT CAST(version AS BINARY) AS version,CAST(dirty AS BINARY) AS dirty FROM schema_migrations", 2)
	if err != nil || len(rows) != 1 || rows[0].text("version") != "99" || rows[0].text("dirty") != "0" {
		return ErrAIReverseBinding
	}
	return nil
}

// ValidateComponentObservation verifies only this same native scoped read. It
// deliberately cannot satisfy ValidateBorrowedSnapshot/RecheckFresh or Apply.
func (s *AIReverseSnapshot) ValidateComponentObservation(ctx context.Context) error {
	if s == nil || !s.componentComplete || s.report.WholeLedgerEOF || s.report.DataSHA256 == "" || s.report.DataSHA256 != aiComponentDataDigest(s) {
		return ErrAIReverseBinding
	}
	return s.validateComponentScope(ctx)
}
func (e *AIHistoricalInputEpoch) componentFrame(ctx context.Context, page int) (*aiHistoricalInputFrame, error) {
	if e.validFile(ctx) != nil || !e.complete || e.source == nil || e.componentIndexSHA == "" || page < 0 || page >= len(e.pages) {
		return nil, ErrAIReverseBinding
	}
	ref := e.pages[page]
	if ref.Offset < 0 || ref.Length <= 0 || ref.Length > 2*sourceOriginInputPageBytes+(1<<20) || ref.Offset > e.end-ref.Length {
		return nil, ErrAIReverseRead
	}
	raw := make([]byte, int(ref.Length))
	n, err := e.file.ReadAt(raw, ref.Offset)
	if err != nil || n != len(raw) || historicalSpoolSHA(raw) != ref.SHA256 {
		return nil, ErrAIReverseRead
	}
	var frame aiHistoricalInputFrame
	if historicalSpoolDecode(raw, &frame) != nil || frame.Version != 1 || frame.Epoch != e.epoch || frame.SourceInput != e.source.resultHash {
		return nil, ErrAIReverseBinding
	}
	return &frame, nil
}
func (e *AIHistoricalInputEpoch) componentPage(ctx context.Context, page int) ([]aiReverseRow, aiReverseSpec, aiReverseMetadata, error) {
	frame, err := e.componentFrame(ctx, page)
	if err != nil {
		return nil, aiReverseSpec{}, aiReverseMetadata{}, err
	}
	if frame.EOF {
		return nil, aiReverseSpec{}, aiReverseMetadata{}, ErrAIReverseBinding
	}
	var rows []aiReverseRow
	if json.Unmarshal(frame.RowsJSON, &rows) != nil {
		return nil, aiReverseSpec{}, aiReverseMetadata{}, ErrAIReverseRead
	}
	for i, spec := range aiReverseSpecs {
		if spec.table == frame.Ledger.Store {
			return rows, spec, e.metadata[i], nil
		}
	}
	return nil, aiReverseSpec{}, aiReverseMetadata{}, ErrAIReverseSchema
}
func aiComponentPageOrder(pages map[int]bool) []int {
	result := make([]int, 0, len(pages))
	for page := range pages {
		result = append(result, page)
	}
	sort.Ints(result)
	return result
}
func aiComponentExpand(scope *aiReverseScope, assessments, resources map[string]bool, n *aiReverseNode) {
	for _, id := range []string{n.id, n.command, n.receipt, n.event, n.ack} {
		if id != "" {
			scope.relatedIDs[id] = true
		}
	}
	for _, id := range []string{n.request, n.aggregate} {
		if id != "" {
			scope.relatedRequests[id] = true
		}
	}
	for _, id := range []string{n.resource, n.receiptResource, n.receiptRun} {
		if id != "" {
			resources[id] = true
		}
	}
	for _, id := range n.assessments {
		assessments[id] = true
	}
	if n.observation.Store == "ai_bridge_request_assessments" && n.resource != "" {
		assessments[n.resource] = true
	}
}
func aiComponentIn(column string, values map[string]bool, numeric bool) (string, []any, error) {
	if len(values) == 0 {
		return "", nil, nil
	}
	if len(values) > 4096 {
		return "", nil, ErrAIReverseBounds
	}
	placeholders, args := []string{}, []any{}
	for _, value := range aiReverseSortedKeys(values) {
		if !aiReverseASCII(value) {
			return "", nil, ErrAIReverseBinding
		}
		placeholders = append(placeholders, "?")
		if numeric {
			id, err := strconv.ParseUint(value, 10, 64)
			if err != nil || id == 0 {
				return "", nil, ErrAIReverseBinding
			}
			args = append(args, id)
		} else {
			args = append(args, value)
		}
	}
	return "`" + column + "` IN (" + strings.Join(placeholders, ",") + ")", args, nil
}
func aiComponentPredicate(spec aiReverseSpec, scope *aiReverseScope, assessments, resources map[string]bool) (string, []any, error) {
	// Whole current quarantine/control sets cannot be assigned an owner by
	// absence of an indexed request. Unknown wire responsibility stays visible.
	if spec.table == "ai_messaging_quarantine" || spec.table == "ai_messaging_admission" || spec.table == "ai_messaging_observations" {
		return "1=1", nil, nil
	}
	terms, args := []string{}, []any{}
	add := func(column string, values map[string]bool, numeric bool) error {
		predicate, params, err := aiComponentIn(column, values, numeric)
		if err != nil {
			return err
		}
		if predicate != "" {
			terms = append(terms, predicate)
			args = append(args, params...)
		}
		return nil
	}
	var err error
	switch spec.table {
	case "ai_bridge_requests":
		err = add("request_id", scope.relatedRequests, false)
		if err == nil {
			err = add("session_id", resources, false)
		}
	case "ai_bridge_request_assessments":
		err = add("request_id", scope.relatedRequests, false)
		if err == nil {
			err = add("assessment_id", assessments, true)
		}
	case "ai_bridge_commands", "ai_messaging_legacy_commands":
		err = add("request_id", scope.relatedRequests, false)
		if err == nil {
			err = add("command_id", scope.relatedIDs, false)
		}
	case "ai_bridge_events":
		err = add("request_id", scope.relatedRequests, false)
		if err == nil {
			err = add("event_id", scope.relatedIDs, false)
		}
	case "ai_messaging_operations":
		err = add("aggregate_key", scope.relatedRequests, false)
		if err == nil {
			err = add("command_id", scope.relatedIDs, false)
		}
		if err == nil {
			err = add("resource_id", resources, false)
		}
		if err == nil {
			err = add("receipt_id", scope.relatedIDs, false)
		}
	case "ai_messaging_outbox", "ai_messaging_inbox", "ai_messaging_failures":
		err = add("aggregate_key", scope.relatedRequests, false)
		if err == nil {
			err = add("message_id", scope.relatedIDs, false)
		}
	case "ai_messaging_aggregates":
		err = add("aggregate_key", scope.relatedRequests, false)
	case "ai_messaging_evaluation_states":
		err = add("run_id", resources, false)
		if err == nil {
			err = add("run_id", scope.relatedRequests, false)
		}
	default:
		return "", nil, ErrAIReverseSchema
	}
	if err != nil {
		return "", nil, err
	}
	return strings.Join(terms, " OR "), args, nil
}
func fmtAIComponentArgs(args []any) string { raw, _ := json.Marshal(args); return string(raw) }
func aiComponentDataDigest(s *AIReverseSnapshot) string {
	parts := []string{"ai-reverse-component-current/v1", s.componentSeal, s.report.DatabaseIdentitySHA256, s.report.SourceScopeSHA256, s.report.BusinessAnchorsSHA256}
	if s.scope != nil {
		parts = append(parts, aiJSONHash(s.scope.relatedRequests), aiJSONHash(s.scope.relatedIDs), aiJSONHash(s.componentAssessments), aiJSONHash(s.componentResources))
	}
	for _, spec := range aiReverseSpecs {
		for _, id := range aiReverseSortedKeys(s.byTable[spec.table]) {
			n := s.byTable[spec.table][id]
			parts = append(parts, spec.table, n.observation.PrimaryKeySHA256, n.observation.RowSHA256, n.observation.Scope, strconv.FormatBool(n.observation.Invalid), strconv.FormatBool(n.observation.Unfinished), strconv.FormatBool(n.observation.Held))
			parts = append(parts, n.observation.Reasons...)
		}
	}
	return aiReverseHash(parts...)
}

// This live full14 observer borrows a genuine fresh native RRRO transaction.
// Frozen pairs select original inputs; they never supply a completed SQL8 cycle.
func prepareHistoricalAIFullSnapshot(ctx context.Context, sources *HistoricalSourceInputPair, pair *AIHistoricalInputPair, limits AIReverseLimits, access string) (*AIReverseSnapshot, error) {
	if ctx == nil || access != "READ ONLY" && access != "READ WRITE" || !limits.valid() || limits.MaxRetainedBytes > aiHistoricalInputMaxRetained || aiComponentPairIntact(ctx, pair, sources) != nil || !sources.first.captureStopped || !sources.second.captureStopped {
		return nil, ErrAIReverseBinding
	}
	tx, err := hostmysql.RequireTx(ctx)
	if err != nil || tx.Statement == nil || tx.Statement.ConnPool == nil || tx.Statement.ConnPool == pair.first.pool || tx.Statement.ConnPool == pair.second.pool {
		return nil, ErrAIReverseFresh
	}
	if _, err = sdkmysql.BindGORM(tx); err != nil {
		return nil, ErrAIReverseFresh
	}
	s := &AIReverseSnapshot{pool: tx.Statement.ConnPool, head: 99, limits: limits, started: time.Now(), byTable: map[string]map[string]*aiReverseNode{}, anchors: map[string]aiReverseAnchor{}, historicalSources: sources, historicalPair: pair, historicalAccess: access}
	s.self = s
	s.report = AIReverseSummary{Version: "ai-reverse-historical-live/v1", DatabaseIdentitySHA256: pair.second.report.DatabaseIdentitySHA256, MigrationVersion: 99, StartedAt: s.started.UTC(), ExternalOriginRequired: true, ExternalQSAIClosureRequired: true, StoredWireAuthenticationRequired: true, WriterFenceRequired: true}
	work, cancel := context.WithDeadline(ctx, s.started.Add(limits.MaxDuration))
	defer cancel()
	if s.historicalNative, err = s.historicalTransaction(work, access); err != nil {
		return nil, err
	}
	for _, spec := range aiReverseSpecs {
		if err = s.scan(work, spec); err != nil {
			return nil, err
		}
	}
	if err = s.readAssessmentAnchors(work); err != nil {
		return nil, err
	}
	s.reverse()
	commands, err := historicalAIOriginalCommands(work, sources, pair)
	if err != nil {
		return nil, err
	}
	scope := &aiReverseScope{relatedRequests: map[string]bool{}, relatedIDs: map[string]bool{}, identityConflicts: map[string]bool{}, receipts: sources.second.receipts, sha: aiReverseHash("historical-full14-source/v1", sources.second.resultHash, pair.second.componentIndexSHA)}
	for i, table := range []string{AIBridgeCommandSource, AILegacyCommandSource} {
		if len(commands[i]) != len(s.byTable[table]) {
			return nil, ErrAIReverseChanged
		}
		for id, v := range commands[i] {
			n := s.byTable[table][id]
			if n == nil || n.sourceRowHash != v.Source.Digest.SHA256 || n.request != v.RequestID || n.org != v.OrganizationID || n.subject != v.SubjectID || n.resource != v.ResourceID {
				return nil, ErrSourceAuthentication
			}
			scope.relatedRequests[v.RequestID], scope.relatedIDs[id] = true, true
		}
	}
	s.classify(scope)
	for i, spec := range aiReverseSpecs {
		meta, e := s.schema(work, spec)
		if e != nil || !reflect.DeepEqual(meta, pair.second.metadata[i]) || !reflect.DeepEqual(meta, s.metadata[i]) {
			return nil, ErrAIReverseSchema
		}
	}
	if _, hash, e := s.assessmentMetadata(work); e != nil || hash != s.anchorMetadataSHA {
		return nil, ErrAIReverseSchema
	}
	if token, e := s.historicalTransaction(work, access); e != nil || token != s.historicalNative {
		return nil, ErrAIReverseFresh
	}
	s.report.ActualReadOnlyRR, s.report.WholeLedgerEOF = access == "READ ONLY", true
	s.report.CompletedAt, s.report.DataSHA256 = time.Now().UTC(), s.dataDigest()
	s.historicalSeal = s.historicalDigest()
	if s.validateHistoricalSnapshot(work) != nil {
		return nil, ErrAIReverseFresh
	}
	return s, nil
}

// Instrumentation binds actual transaction identity and mode, not session defaults.
func (s *AIReverseSnapshot) historicalTransaction(ctx context.Context, access string) (string, error) {
	if s == nil || s.pool == nil || s.alive(ctx) != nil {
		return "", ErrAIReverseFresh
	}
	rows, _, _, err := s.read(ctx, "SELECT t.PROCESSLIST_ID AS connection_id,e.THREAD_ID AS thread_id,e.EVENT_ID AS event_id,e.STATE AS state,e.END_EVENT_ID AS end_event_id,e.ACCESS_MODE AS access_mode,e.ISOLATION_LEVEL AS isolation_level,e.AUTOCOMMIT AS autocommit FROM performance_schema.events_transactions_current e JOIN performance_schema.threads t ON t.THREAD_ID=e.THREAD_ID WHERE t.PROCESSLIST_ID=CONNECTION_ID()", 2)
	if err != nil || len(rows) != 1 || rows[0].text("state") != "ACTIVE" || rows[0]["end_event_id"] != nil || rows[0].text("access_mode") != access || rows[0].text("isolation_level") != "REPEATABLE READ" || rows[0].text("autocommit") != "NO" {
		return "", ErrAIReverseFresh
	}
	for _, name := range []string{"connection_id", "thread_id", "event_id"} {
		if !aiPositiveNumber(rows[0].text(name)) {
			return "", ErrAIReverseFresh
		}
	}
	identity, _, _, e := s.read(ctx, "SELECT @@server_uuid AS server,DATABASE() AS db,VERSION() AS version", 1)
	if e != nil || len(identity) != 1 || !strings.HasPrefix(identity[0].text("version"), "8.") || aiReverseHash("mysql_database_identity_v1", identity[0].text("server"), identity[0].text("db")) != s.report.DatabaseIdentitySHA256 {
		return "", ErrAIReverseBinding
	}
	head, _, _, e := s.read(ctx, "SELECT CAST(version AS BINARY) AS version,CAST(dirty AS BINARY) AS dirty FROM schema_migrations", 2)
	if e != nil || len(head) != 1 || head[0].text("version") != "99" || head[0].text("dirty") != "0" {
		return "", ErrAIReverseBinding
	}
	return aiReverseHash("historical-ai-native/v1", rows[0].text("connection_id"), rows[0].text("thread_id"), rows[0].text("event_id"), access), nil
}
func (s *AIReverseSnapshot) historicalDigest() string {
	if s == nil || s.historicalSources == nil || s.historicalPair == nil {
		return ""
	}
	return aiJSONHash(struct {
		Source, Input, Native, Access, Data, Class string
		Started                                    time.Time
		Limits                                     AIReverseLimits
	}{s.historicalSources.second.resultHash, s.historicalPair.second.componentIndexSHA, s.historicalNative, s.historicalAccess, s.report.DataSHA256, aiJSONHash(s.report), s.started, s.limits})
}
func (s *AIReverseSnapshot) validateHistoricalSnapshot(ctx context.Context) error {
	if s == nil || s.self != s || s.snapshot != nil || s.component != nil || !s.report.WholeLedgerEOF || s.historicalAccess != "READ ONLY" && s.historicalAccess != "READ WRITE" || s.historicalSeal == "" || s.historicalSeal != s.historicalDigest() || aiComponentPairIntact(ctx, s.historicalPair, s.historicalSources) != nil {
		return ErrAIReverseBinding
	}
	tx, err := hostmysql.RequireTx(ctx)
	if err != nil || tx.Statement == nil || tx.Statement.ConnPool != s.pool {
		return ErrAIReverseFresh
	}
	if _, err = sdkmysql.BindGORM(tx); err != nil {
		return ErrAIReverseFresh
	}
	token, err := s.historicalTransaction(ctx, s.historicalAccess)
	if err != nil || token != s.historicalNative {
		return ErrAIReverseFresh
	}
	return nil
}
