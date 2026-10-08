package evaluation

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"gorm.io/gorm"
)

var (
	ErrSQLResponsibilityInvalid     = errors.New("sql_responsibility_cycle_invalid")
	ErrSQLResponsibilityBounds      = errors.New("sql_responsibility_cycle_bound_exceeded")
	ErrSQLResponsibilityRead        = errors.New("sql_responsibility_cycle_read_failed")
	ErrSQLResponsibilitySchema      = errors.New("sql_responsibility_cycle_schema_conflict")
	ErrSQLResponsibilityVisibility  = errors.New("sql_responsibility_actual_transaction_visibility_unproven")
	ErrSQLResponsibilityTransaction = errors.New("sql_responsibility_actual_transaction_mode_conflict")
	ErrSQLResponsibilityFresh       = errors.New("sql_responsibility_new_snapshot_required")
	ErrSQLResponsibilityChanged     = errors.New("sql_responsibility_fresh_snapshot_changed")
)

// Limits are scan budgets, not permission to truncate a ledger. Every budget
// violation aborts without returning a complete cycle. No message body is kept.
type SQLResponsibilityLimits struct {
	PageRows         int
	MaxRows          uint64
	MaxBytes         uint64
	MaxRetainedBytes uint64
}

func DefaultSQLResponsibilityLimits() SQLResponsibilityLimits {
	return SQLResponsibilityLimits{PageRows: 512, MaxRows: 2_000_000, MaxBytes: 8 << 30, MaxRetainedBytes: 1 << 30}
}

func (l SQLResponsibilityLimits) valid() bool {
	return l.PageRows > 0 && l.PageRows <= 4096 && l.MaxRows > 0 && l.MaxRows <= 10_000_000 && l.MaxBytes > 0 && l.MaxBytes <= 32<<30 && l.MaxRetainedBytes > 0 && l.MaxRetainedBytes <= 4<<30
}

type SQLResponsibilityLedgerReport struct {
	Store                                                        string `json:"store"`
	Rows, Bytes, Pages                                           uint64
	SchemaSHA256, PrimaryKeySHA256, UpperBoundSHA256, RowsSHA256 string
}

// Only aggregate evidence is public. Source authentication, Mongo/AI/Inbox
// coverage and a writer fence are independently owned by the coordinator.
type SQLResponsibilityCycleReport struct {
	OriginalBusinessFactsRequired                                                             bool
	SchemaCoverage                                                                            string
	BusinessAnchorsSHA256                                                                     string
	RetainedBudgetBytes                                                                       uint64
	Version, CycleID, DatabaseIdentitySHA256                                                  string
	StartedAt, CompletedAt                                                                    time.Time
	Limits                                                                                    SQLResponsibilityLimits
	Ledgers                                                                                   []SQLResponsibilityLedgerReport
	Observed, RetirementRelated, OutsideRetirement, Unknown, Blocking                         uint64
	ActualTransactionReadOnlyRR                                                               bool
	SourceAuthenticationRequired, ExternalResponsibilityCoverageRequired, WriterFenceRequired bool
	DropReady                                                                                 bool
}

type SQLResponsibilityObservation struct {
	Store, PrimaryKeySHA256, RowSHA256, EventID, EventType, State string
	SDKFingerprintSHA256                                          string
	OrgID, AssessmentID, TesteeID                                 uint64
	OwnerKind, OwnerID, ScopeClass                                string
	Invalid, Unfinished, LeasePresent, OwnerUnproven              bool
	Reasons                                                       []string
	link                                                          sqlResponsibilityLink
	payloadSHA256                                                 string
	outcomeID                                                     uint64
	runID                                                         string
}

func (SQLResponsibilityObservation) MarshalJSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (SQLResponsibilityObservation) MarshalBSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (SQLResponsibilityObservation) String() string { return "private SQL responsibility observation" }
func (SQLResponsibilityObservation) GoString() string {
	return "private SQL responsibility observation"
}

type sqlResponsibilityTable struct {
	name       string
	keys       []string
	binaryKeys map[string]bool
}

var sqlResponsibilityTables = []sqlResponsibilityTable{
	{"rm_outbox", []string{"id"}, nil},
	{"retry_event_hold", []string{"id"}, nil},
	{"event_delivery_dead_letter", []string{"id"}, nil},
	{"qs_rm_evaluation_request_ref", []string{"event_id"}, map[string]bool{"event_id": true}},
	{"qs_rm_gap_recovery_request", []string{"org_id", "request_id"}, map[string]bool{"request_id": true}},
	{"qs_rm_replay_requests", []string{"org_id", "request_id"}, map[string]bool{"request_id": true}},
	{"qs_rm_replay_items", []string{"org_id", "request_id", "ordinal"}, map[string]bool{"request_id": true}},
	{"system_governance_action_runs", []string{"id"}, nil},
}

type sqlResponsibilityTransaction struct{ connection, thread, event uint64 }
type sqlResponsibilityLedger struct {
	spec    sqlResponsibilityTable
	columns []string
	upper   []string
	report  SQLResponsibilityLedgerReport
}
type sqlResponsibilityOwner struct {
	org, testee     uint64
	absent, invalid bool
}

// SQLHistoricalResponsibilityCycle has no write or transaction lifecycle API.
// Its private indexes originate only from a complete, bounded actual snapshot.
type SQLHistoricalResponsibilityCycle struct {
	report        SQLResponsibilityCycleReport
	transaction   sqlResponsibilityTransaction
	ledgers       []sqlResponsibilityLedger
	observations  []SQLResponsibilityObservation
	owners        map[uint64]sqlResponsibilityOwner
	byOwner       map[uint64][]int
	byEvent       map[string][]int
	byOrgActions  map[uint64][]int
	anchorDigests map[string]string
	rows, bytes   uint64
}

func (*SQLHistoricalResponsibilityCycle) MarshalJSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalResponsibilityCycle) MarshalBSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalResponsibilityCycle) String() string   { return "private SQL responsibility cycle" }
func (*SQLHistoricalResponsibilityCycle) GoString() string { return "private SQL responsibility cycle" }

func (c *SQLHistoricalResponsibilityCycle) Report() SQLResponsibilityCycleReport {
	if c == nil {
		return SQLResponsibilityCycleReport{}
	}
	r := c.report
	r.Ledgers = append([]SQLResponsibilityLedgerReport(nil), r.Ledgers...)
	return r
}

func (c *SQLHistoricalResponsibilityCycle) observationsAt(index []int) []SQLResponsibilityObservation {
	var out []SQLResponsibilityObservation
	for _, i := range index {
		v := c.observations[i]
		v.Reasons = append([]string(nil), v.Reasons...)
		out = append(out, v)
	}
	return out
}

func (c *SQLHistoricalResponsibilityCycle) ForAssessment(id uint64) []SQLResponsibilityObservation {
	if c == nil {
		return nil
	}
	out := c.observationsAt(c.byOwner[id])
	if owner, ok := c.owners[id]; ok {
		out = append(out, c.observationsAt(c.byOrgActions[owner.org])...)
	}
	return out
}
func (c *SQLHistoricalResponsibilityCycle) ForEvent(id string) []SQLResponsibilityObservation {
	if c == nil {
		return nil
	}
	return c.observationsAt(c.byEvent[id])
}

func (c *SQLHistoricalResponsibilityCycle) ForOrganizationActions(orgID uint64) []SQLResponsibilityObservation {
	if c == nil {
		return nil
	}
	return c.observationsAt(c.byOrgActions[orgID])
}

func cycleFrame(h hash.Hash, raw []byte, present bool) {
	var frame [9]byte
	if present {
		frame[0] = 1
	}
	binary.BigEndian.PutUint64(frame[1:], uint64(len(raw)))
	_, _ = h.Write(frame[:])
	_, _ = h.Write(raw)
}
func cycleRowDigest(columns []string, row historicalSQLRow) string {
	h := sha256.New()
	cycleFrame(h, []byte("sql-responsibility-row/v1"), true)
	for _, col := range columns {
		cycleFrame(h, []byte(col), true)
		v := row[col]
		if v == nil {
			cycleFrame(h, nil, false)
		} else {
			cycleFrame(h, []byte(*v), true)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}
func cycleKeyDigest(values []string) string {
	h := sha256.New()
	for _, v := range values {
		cycleFrame(h, []byte(v), true)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func cycleQuery(tx *gorm.DB, query string, limit int, args ...any) (out []historicalSQLRow, names []string, size uint64, err error) {
	rows, e := tx.Raw(query, args...).Rows()
	if e != nil {
		return nil, nil, 0, ErrSQLResponsibilityRead
	}
	defer func() {
		if e := rows.Close(); err == nil && e != nil {
			err = ErrSQLResponsibilityRead
		}
	}()
	names, e = rows.Columns()
	if e != nil || len(names) == 0 || len(names) > 128 {
		return nil, nil, 0, ErrSQLResponsibilitySchema
	}
	seen := map[string]bool{}
	for _, n := range names {
		if seen[n] {
			return nil, nil, 0, ErrSQLResponsibilitySchema
		}
		seen[n] = true
	}
	for rows.Next() {
		if len(out) >= limit {
			return nil, nil, 0, ErrSQLResponsibilityBounds
		}
		values := make([]sql.RawBytes, len(names))
		dest := make([]any, len(names))
		for i := range values {
			dest[i] = &values[i]
		}
		if rows.Scan(dest...) != nil {
			return nil, nil, 0, ErrSQLResponsibilityRead
		}
		row := historicalSQLRow{}
		for i, n := range names {
			size += uint64(len(values[i]))
			if size > 64<<20 {
				return nil, nil, 0, ErrSQLResponsibilityBounds
			}
			if values[i] == nil {
				row[n] = nil
			} else {
				v := string(values[i])
				row[n] = &v
			}
		}
		out = append(out, row)
	}
	if rows.Err() != nil {
		return nil, nil, 0, ErrSQLResponsibilityRead
	}
	return out, names, size, nil
}

// Check actual current transaction instrumentation, not session defaults.
// Missing instrumentation/consumer/privilege is an evidence gap, not a claim
// that the host's BeginTx was wrong. The observer never changes instrumentation.
func cycleActualTransaction(tx *gorm.DB) (sqlResponsibilityTransaction, error) {
	rows, _, _, err := cycleQuery(tx, "SELECT t.PROCESSLIST_ID AS connection_id,e.THREAD_ID AS thread_id,e.EVENT_ID AS event_id,e.STATE AS state,e.END_EVENT_ID AS end_event_id,e.ACCESS_MODE AS access_mode,e.ISOLATION_LEVEL AS isolation_level,e.AUTOCOMMIT AS autocommit FROM performance_schema.events_transactions_current e JOIN performance_schema.threads t ON t.THREAD_ID=e.THREAD_ID WHERE t.PROCESSLIST_ID=CONNECTION_ID()", 2)
	if err != nil || len(rows) != 1 {
		return sqlResponsibilityTransaction{}, ErrSQLResponsibilityVisibility
	}
	r := rows[0]
	if valueOrEmpty(r["state"]) != "ACTIVE" || r["end_event_id"] != nil {
		return sqlResponsibilityTransaction{}, ErrSQLResponsibilityVisibility
	}
	if valueOrEmpty(r["access_mode"]) != "READ ONLY" || valueOrEmpty(r["isolation_level"]) != "REPEATABLE READ" || valueOrEmpty(r["autocommit"]) != "NO" {
		return sqlResponsibilityTransaction{}, ErrSQLResponsibilityTransaction
	}
	connection, e1 := sqlHistoricalUint(r, "connection_id")
	thread, e2 := sqlHistoricalUint(r, "thread_id")
	event, e3 := sqlHistoricalUint(r, "event_id")
	if e1 != nil || e2 != nil || e3 != nil {
		return sqlResponsibilityTransaction{}, ErrSQLResponsibilityVisibility
	}
	return sqlResponsibilityTransaction{connection, thread, event}, nil
}

func cycleSchema(tx *gorm.DB, spec sqlResponsibilityTable) (columns []string, schema, pk string, err error) {
	tables, _, _, err := cycleQuery(tx, "SELECT TABLE_TYPE AS table_type,ENGINE AS engine,TABLE_COLLATION AS table_collation FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?", 2, spec.name)
	if err != nil || len(tables) != 1 || valueOrEmpty(tables[0]["table_type"]) != "BASE TABLE" || valueOrEmpty(tables[0]["engine"]) != "InnoDB" {
		return nil, "", "", ErrSQLResponsibilitySchema
	}
	cols, names, _, err := cycleQuery(tx, "SELECT COLUMN_NAME AS column_name,ORDINAL_POSITION AS ordinal_position,COLUMN_TYPE AS column_type,DATA_TYPE AS data_type,IS_NULLABLE AS is_nullable,COLUMN_DEFAULT AS column_default,CHARACTER_SET_NAME AS character_set_name,COLLATION_NAME AS collation_name,EXTRA AS extra,GENERATION_EXPRESSION AS generation_expression FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? ORDER BY ORDINAL_POSITION", 128, spec.name)
	if err != nil || len(cols) == 0 {
		return nil, "", "", ErrSQLResponsibilitySchema
	}
	h := sha256.New()
	cycleFrame(h, []byte(cycleRowDigest([]string{"table_type", "engine", "table_collation"}, tables[0])), true)
	ddlRows, _, _, err := cycleQuery(tx, "SHOW CREATE TABLE `"+spec.name+"`", 1)
	if err != nil || len(ddlRows) != 1 || valueOrEmpty(ddlRows[0]["Table"]) != spec.name {
		return nil, "", "", ErrSQLResponsibilitySchema
	}
	ddl, err := cycleNormalizeDDL(valueOrEmpty(ddlRows[0]["Create Table"]))
	if err != nil {
		return nil, "", "", err
	}
	cycleFrame(h, []byte(ddl), true)
	byName := map[string]historicalSQLRow{}
	for _, col := range cols {
		n := valueOrEmpty(col["column_name"])
		if n == "" || byName[n] != nil {
			return nil, "", "", ErrSQLResponsibilitySchema
		}
		byName[n] = col
		columns = append(columns, n)
		cycleFrame(h, []byte(cycleRowDigest(names, col)), true)
	}
	indexes, indexNames, _, err := cycleQuery(tx, "SELECT INDEX_NAME AS index_name,NON_UNIQUE AS non_unique,SEQ_IN_INDEX AS seq_in_index,COLUMN_NAME AS column_name,SUB_PART AS sub_part,COLLATION AS collation,INDEX_TYPE AS index_type,IS_VISIBLE AS is_visible,EXPRESSION AS expression FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name=? ORDER BY INDEX_NAME,SEQ_IN_INDEX", 128, spec.name)
	if err != nil {
		return nil, "", "", ErrSQLResponsibilitySchema
	}
	p := sha256.New()
	found := 0
	for _, index := range indexes {
		digest := cycleRowDigest(indexNames, index)
		cycleFrame(h, []byte(digest), true)
		if valueOrEmpty(index["index_name"]) != "PRIMARY" {
			continue
		}
		if found >= len(spec.keys) || valueOrEmpty(index["column_name"]) != spec.keys[found] || valueOrEmpty(index["seq_in_index"]) != strconv.Itoa(found+1) || valueOrEmpty(index["non_unique"]) != "0" || index["sub_part"] != nil || valueOrEmpty(index["collation"]) != "A" || index["expression"] != nil || valueOrEmpty(index["is_visible"]) != "YES" {
			return nil, "", "", ErrSQLResponsibilitySchema
		}
		column := byName[spec.keys[found]]
		dataType := valueOrEmpty(column["data_type"])
		if valueOrEmpty(column["is_nullable"]) != "NO" || spec.binaryKeys[spec.keys[found]] && dataType != "varbinary" || !spec.binaryKeys[spec.keys[found]] && dataType != "bigint" && dataType != "smallint" {
			return nil, "", "", ErrSQLResponsibilitySchema
		}
		cycleFrame(p, []byte(digest), true)
		found++
	}
	if found != len(spec.keys) {
		return nil, "", "", ErrSQLResponsibilitySchema
	}
	return columns, hex.EncodeToString(h.Sum(nil)), hex.EncodeToString(p.Sum(nil)), nil
}

// SHOW CREATE includes CHECK/FK/generated expressions and table options that
// column/index metadata alone misses. Only its leading InnoDB AUTO_INCREMENT
// allocation counter is excluded: it is outside the RR row snapshot.
func cycleNormalizeDDL(ddl string) (string, error) {
	marker := "\n) ENGINE=InnoDB"
	at := strings.LastIndex(ddl, marker)
	if at < 0 {
		return "", ErrSQLResponsibilitySchema
	}
	end := at + len(marker)
	tail := ddl[end:]
	prefix := " AUTO_INCREMENT="
	if strings.HasPrefix(tail, prefix) {
		remainder := tail[len(prefix):]
		tokenEnd := strings.IndexByte(remainder, ' ')
		if tokenEnd < 0 {
			tokenEnd = len(remainder)
		}
		number := remainder[:tokenEnd]
		n, e := strconv.ParseUint(number, 10, 64)
		if e != nil || n == 0 || strconv.FormatUint(n, 10) != number {
			return "", ErrSQLResponsibilitySchema
		}
		tail = remainder[tokenEnd:]
	}
	return ddl[:end] + tail, nil
}

func cycleKey(spec sqlResponsibilityTable, row historicalSQLRow) ([]string, error) {
	key := make([]string, len(spec.keys))
	for i, n := range spec.keys {
		v := row[n]
		if v == nil {
			return nil, ErrSQLResponsibilitySchema
		}
		key[i] = *v
		if spec.binaryKeys[n] {
			if len(*v) == 0 || len(*v) > 128 {
				return nil, ErrSQLResponsibilityInvalid
			}
		} else {
			number, e := strconv.ParseUint(*v, 10, 64)
			if e != nil || strconv.FormatUint(number, 10) != *v || number == 0 && n != "ordinal" {
				return nil, ErrSQLResponsibilityInvalid
			}
		}
	}
	return key, nil
}
func cycleCompare(spec sqlResponsibilityTable, a, b []string) int {
	for i, n := range spec.keys {
		if spec.binaryKeys[n] {
			if c := bytes.Compare([]byte(a[i]), []byte(b[i])); c != 0 {
				return c
			}
		} else {
			x, _ := strconv.ParseUint(a[i], 10, 64)
			y, _ := strconv.ParseUint(b[i], 10, 64)
			if x < y {
				return -1
			}
			if x > y {
				return 1
			}
		}
	}
	return 0
}

// Expand complete lexicographic comparisons so MySQL can use every primary-key
// column as an index range. Combining row-constructor inequalities can instead
// select a full index scan, rereading the same prefix on each bounded page.
func cycleKeyPredicate(spec sqlResponsibilityTable, values []string, upper bool) (string, []any) {
	bound := cycleArgs(spec, values)
	terms := make([]string, 0, len(spec.keys))
	var args []any
	for i, key := range spec.keys {
		var clause []string
		for prefix := 0; prefix < i; prefix++ {
			clause = append(clause, "`"+spec.keys[prefix]+"` = ?")
			args = append(args, bound[prefix])
		}
		op := ">"
		if upper {
			op = "<"
			if i == len(spec.keys)-1 {
				op = "<="
			}
		}
		clause = append(clause, "`"+key+"` "+op+" ?")
		args = append(args, bound[i])
		terms = append(terms, "("+strings.Join(clause, " AND ")+")")
	}
	return "(" + strings.Join(terms, " OR ") + ")", args
}

func cycleArgs(spec sqlResponsibilityTable, values []string) []any {
	out := make([]any, len(values))
	for i, v := range values {
		if spec.binaryKeys[spec.keys[i]] {
			// GORM Raw expands a slice in a parenthesized placeholder into
			// IN-list operands. Bind binary PK bytes as one driver value.
			out[i] = cycleBinaryValue([]byte(v))
		} else {
			out[i], _ = strconv.ParseUint(v, 10, 64)
		}
	}
	return out
}

type cycleBinaryValue []byte

func (value cycleBinaryValue) Value() (driver.Value, error) { return []byte(value), nil }

// A bounded sequential reverse reader makes unbound/global rows inspectable
// without materializing another full cycle or publishing raw identifiers.
func (c *SQLHistoricalResponsibilityCycle) ObservationsPage(offset uint64, limit int) ([]SQLResponsibilityObservation, uint64, error) {
	if c == nil || limit <= 0 || limit > 4096 || offset > uint64(len(c.observations)) {
		return nil, 0, ErrSQLResponsibilityInvalid
	}
	end := offset + uint64(limit)
	if end > uint64(len(c.observations)) {
		end = uint64(len(c.observations))
	}
	indexes := make([]int, 0, end-offset)
	for i := offset; i < end; i++ {
		indexes = append(indexes, int(i))
	}
	return c.observationsAt(indexes), end, nil
}

func PrepareSQLHistoricalResponsibilityCycle(ctx context.Context, identity string, limits SQLResponsibilityLimits) (*SQLHistoricalResponsibilityCycle, error) {
	if !evidence.ValidSHA256(identity) || !limits.valid() {
		return nil, ErrSQLResponsibilityInvalid
	}
	tx, err := historicalTx(ctx)
	if err != nil {
		return nil, err
	}
	server, database, err := historicalDatabase(tx)
	if err != nil {
		return nil, ErrSQLResponsibilityRead
	}
	if sqlHistoricalIdentity(server, database) != identity {
		return nil, ErrSQLHistoricalFactsIdentity
	}
	c := &SQLHistoricalResponsibilityCycle{owners: map[uint64]sqlResponsibilityOwner{}, byOwner: map[uint64][]int{}, byEvent: map[string][]int{}, byOrgActions: map[uint64][]int{}, anchorDigests: map[string]string{}}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, ErrSQLResponsibilityRead
	}
	c.report = SQLResponsibilityCycleReport{Version: "sql-responsibility-cycle/v1", CycleID: hex.EncodeToString(nonce[:]), DatabaseIdentitySHA256: identity, StartedAt: time.Now().UTC(), Limits: limits, SourceAuthenticationRequired: true, ExternalResponsibilityCoverageRequired: true, WriterFenceRequired: true, OriginalBusinessFactsRequired: true, SchemaCoverage: "show_create_table_without_auto_increment+columns+indexes/v1"}
	for _, spec := range sqlResponsibilityTables {
		columns, schema, pk, e := cycleSchema(tx, spec)
		if e != nil {
			return nil, e
		}
		order := strings.Join(spec.keys, ",")
		upperRows, _, _, e := cycleQuery(tx, "SELECT "+order+" FROM `"+spec.name+"` ORDER BY "+strings.Join(spec.keys, " DESC,")+" DESC LIMIT 1", 1)
		if e != nil {
			return nil, e
		}
		// The first actual InnoDB read establishes this RR consistent snapshot.
		actual, e := cycleActualTransaction(tx)
		if e != nil {
			return nil, e
		}
		if c.transaction.event == 0 {
			c.transaction = actual
		} else if actual != c.transaction {
			return nil, ErrSQLResponsibilityTransaction
		}
		ledger := sqlResponsibilityLedger{spec: spec, columns: columns, report: SQLResponsibilityLedgerReport{Store: spec.name, SchemaSHA256: schema, PrimaryKeySHA256: pk}}
		var expected uint64
		if e := tx.Raw("SELECT COUNT(*) FROM `" + spec.name + "`").Scan(&expected).Error; e != nil {
			return nil, ErrSQLResponsibilityRead
		}
		if expected > limits.MaxRows-c.rows {
			return nil, ErrSQLResponsibilityBounds
		}
		if len(upperRows) > 0 {
			ledger.upper, e = cycleKey(spec, upperRows[0])
			if e != nil {
				return nil, e
			}
		} else if expected != 0 {
			return nil, ErrSQLResponsibilityChanged
		}
		ledger.report.UpperBoundSHA256 = cycleKeyDigest(ledger.upper)
		h := sha256.New()
		cycleFrame(h, []byte("sql-responsibility-ledger/v1"), true)
		cycleFrame(h, []byte(schema), true)
		var after []string
		for ledger.upper != nil {
			predicate, args := cycleKeyPredicate(spec, ledger.upper, true)
			if after != nil {
				lower, lowerArgs := cycleKeyPredicate(spec, after, false)
				predicate += " AND " + lower
				args = append(args, lowerArgs...)
			}
			args = append(args, limits.PageRows)
			rows, names, size, e := cycleQuery(tx, "SELECT * FROM `"+spec.name+"` WHERE "+predicate+" ORDER BY "+order+" LIMIT ?", limits.PageRows, args...)
			if e != nil {
				return nil, e
			}
			if !reflect.DeepEqual(names, columns) {
				return nil, ErrSQLResponsibilitySchema
			}
			if len(rows) == 0 {
				break
			}
			if size > limits.MaxBytes-c.bytes || uint64(len(rows)) > limits.MaxRows-c.rows {
				return nil, ErrSQLResponsibilityBounds
			}
			c.bytes += size
			c.rows += uint64(len(rows))
			ledger.report.Bytes += size
			ledger.report.Rows += uint64(len(rows))
			ledger.report.Pages++
			for _, row := range rows {
				key, e := cycleKey(spec, row)
				if e != nil {
					return nil, e
				}
				if after != nil && cycleCompare(spec, key, after) <= 0 || cycleCompare(spec, key, ledger.upper) > 0 {
					return nil, ErrSQLResponsibilityChanged
				}
				after = key
				digest := cycleRowDigest(columns, row)
				cycleFrame(h, []byte(digest), true)
				observation := cycleDecode(spec.name, row)
				observation.PrimaryKeySHA256 = cycleKeyDigest(key)
				observation.RowSHA256 = digest
				cost := uint64(1024 + len(observation.EventID) + len(observation.EventType) + len(observation.State) + len(observation.OwnerKind) + len(observation.OwnerID) + len(observation.link.requestID) + len(observation.link.action) + len(observation.runID))
				for _, reason := range observation.Reasons {
					cost += uint64(len(reason))
				}
				if cost > limits.MaxRetainedBytes-c.report.RetainedBudgetBytes {
					return nil, ErrSQLResponsibilityBounds
				}
				c.report.RetainedBudgetBytes += cost
				c.observations = append(c.observations, observation)
			}
			if e := c.checkPageOwners(tx, len(c.observations)-len(rows)); e != nil {
				return nil, e
			}
		}
		if ledger.report.Rows != expected || expected != 0 && cycleCompare(spec, after, ledger.upper) != 0 {
			return nil, ErrSQLResponsibilityChanged
		}
		ledger.report.RowsSHA256 = hex.EncodeToString(h.Sum(nil))
		c.ledgers = append(c.ledgers, ledger)
		c.report.Ledgers = append(c.report.Ledgers, ledger.report)
		_, schemaAfter, pkAfter, e := cycleSchema(tx, spec)
		if e != nil || schemaAfter != schema || pkAfter != pk {
			return nil, ErrSQLResponsibilitySchema
		}
	}
	actual, err := cycleActualTransaction(tx)
	if err != nil {
		return nil, err
	}
	if actual != c.transaction {
		return nil, ErrSQLResponsibilityTransaction
	}
	c.checkReverse()
	keys := make([]string, 0, len(c.anchorDigests))
	for key := range c.anchorDigests {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	anchors := sha256.New()
	for _, key := range keys {
		cycleFrame(anchors, []byte(key), true)
		cycleFrame(anchors, []byte(c.anchorDigests[key]), true)
	}
	c.report.BusinessAnchorsSHA256 = hex.EncodeToString(anchors.Sum(nil))
	c.report.ActualTransactionReadOnlyRR = true
	c.report.CompletedAt = time.Now().UTC()
	return c, nil
}

type SQLResponsibilityFreshReport struct {
	OriginalCycleID, FreshCycleID string
	Identical                     bool
	ChangedStores                 []string
	AboveUpperRows                uint64
	WriterFenceRequired           bool
	DropReady                     bool
}

// RecheckFresh requires a different actual transaction and rescans complete
// ledgers and schemas. A >upper-only query cannot detect edits/deletes below it.
// Neither old nor fresh snapshot proves that a subsequent writer is stopped.
func (c *SQLHistoricalResponsibilityCycle) RecheckFresh(ctx context.Context) (SQLResponsibilityFreshReport, error) {
	if c == nil {
		return SQLResponsibilityFreshReport{}, ErrSQLResponsibilityInvalid
	}
	tx, err := historicalTx(ctx)
	if err != nil {
		return SQLResponsibilityFreshReport{}, err
	}
	// Force a real consistent read before inspecting the active transaction.
	var first uint64
	if tx.Raw("SELECT COUNT(*) FROM rm_outbox").Scan(&first).Error != nil {
		return SQLResponsibilityFreshReport{}, ErrSQLResponsibilityRead
	}
	actual, err := cycleActualTransaction(tx)
	if err != nil {
		return SQLResponsibilityFreshReport{}, err
	}
	if actual == c.transaction {
		return SQLResponsibilityFreshReport{}, ErrSQLResponsibilityFresh
	}
	fresh, err := PrepareSQLHistoricalResponsibilityCycle(ctx, c.report.DatabaseIdentitySHA256, c.report.Limits)
	if err != nil {
		return SQLResponsibilityFreshReport{}, err
	}
	r := SQLResponsibilityFreshReport{OriginalCycleID: c.report.CycleID, FreshCycleID: fresh.report.CycleID, Identical: true, WriterFenceRequired: true}
	if c.report.BusinessAnchorsSHA256 != fresh.report.BusinessAnchorsSHA256 {
		r.Identical = false
		r.ChangedStores = append(r.ChangedStores, "business_owner_anchors")
	}
	for i, old := range c.ledgers {
		current := fresh.ledgers[i]
		if old.report.SchemaSHA256 != current.report.SchemaSHA256 || old.report.PrimaryKeySHA256 != current.report.PrimaryKeySHA256 || old.report.RowsSHA256 != current.report.RowsSHA256 || old.report.Rows != current.report.Rows || !reflect.DeepEqual(old.upper, current.upper) {
			r.Identical = false
			r.ChangedStores = append(r.ChangedStores, old.spec.name)
		}
		if current.upper == nil {
			continue
		}
		var count uint64
		predicate := "1=1"
		var args []any
		if old.upper != nil {
			predicate, args = cycleKeyPredicate(old.spec, old.upper, false)
		}
		if tx.Raw("SELECT COUNT(*) FROM `"+old.spec.name+"` WHERE "+predicate, args...).Scan(&count).Error != nil {
			return r, ErrSQLResponsibilityRead
		}
		r.AboveUpperRows += count
	}
	if !r.Identical {
		return r, ErrSQLResponsibilityChanged
	}
	return r, nil
}

func (c *SQLHistoricalResponsibilityCycle) ValidateBorrowedSnapshot(ctx context.Context) error {
	if c == nil {
		return ErrSQLResponsibilityInvalid
	}
	tx, err := historicalTx(ctx)
	if err != nil {
		return err
	}
	actual, err := cycleActualTransaction(tx)
	if err != nil {
		return err
	}
	if actual != c.transaction {
		return ErrSQLResponsibilityFresh
	}
	server, database, err := historicalDatabase(tx)
	if err != nil {
		return ErrSQLResponsibilityRead
	}
	if sqlHistoricalIdentity(server, database) != c.report.DatabaseIdentitySHA256 {
		return ErrSQLHistoricalFactsIdentity
	}
	return nil
}

func cycleReason(v *SQLResponsibilityObservation, reason string) {
	for _, r := range v.Reasons {
		if r == reason {
			return
		}
	}
	v.Reasons = append(v.Reasons, reason)
	v.Invalid = true
}
func cyclePositive(row historicalSQLRow, key string) uint64 {
	n, e := sqlHistoricalUint(row, key)
	if e != nil {
		return 0
	}
	return n
}
func cycleSafeUint(row historicalSQLRow, key string) (uint64, bool) {
	v := row[key]
	if v == nil {
		return 0, false
	}
	n, e := strconv.ParseUint(*v, 10, 64)
	return n, e == nil && strconv.FormatUint(n, 10) == *v
}
func cyclePair(org uint64, id string) string {
	return fmt.Sprint(org) + ":" + hex.EncodeToString([]byte(id))
}
