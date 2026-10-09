package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strconv"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"gorm.io/gorm"
)

var ErrSQLHistoricalBatchCAS = errors.New("sql_historical_batch_cas_rejected")

// Attachments are write inputs, not source authentication or business closure.
// The host must independently qualify the four complete approved sources.
type SQLHistoricalBatchAttachment struct {
	AssessmentID, OutcomeID uint64
	Entry                   evidence.HistoricalReferenceEntryV1
	ContentDigest           evidence.Digest
}

type sqlHistoricalCASImage struct {
	rows    map[string][]historicalSQLRow
	schema  map[string]string
	columns map[string][]string
}
type sqlHistoricalCASGroup struct {
	table, column string
	id            uint64
	entries       []evidence.HistoricalReferenceEntryV1
	set           *evidence.HistoricalReferenceSetV1
}

// A plan retains the opaque page's actual full-column baseline. It cannot be
// serialized or reconstructed from an editable Snapshot or terminal boolean.
type SQLHistoricalBatchCASPlan struct {
	oldTransaction             sqlResponsibilityTransaction
	identity, server, database string
	request                    SQLHistoricalOwnerBatchRequest
	limits                     SQLHistoricalOwnerBatchLimits
	before                     sqlHistoricalCASImage
	groups                     []sqlHistoricalCASGroup
	attachments                []SQLHistoricalBatchAttachment
	missingOriginalRunIDs      []string
}

// Statement facts deliberately do not certify host Commit. The host must
// independently read back the expected full raw baseline in a different actual
// RR-RO transaction; unknown Commit outcomes never become DROP authorization.
type SQLHistoricalBatchCASStatement struct {
	plan        *SQLHistoricalBatchCASPlan
	transaction sqlResponsibilityTransaction
	expected    sqlHistoricalCASImage
	written     map[string]bool
}
type SQLHistoricalBatchCASLocation struct {
	Table, Column                                     string
	ID                                                uint64
	EventID, EventType                                string
	Source                                            evidence.HistoricalSourceReferenceV1
	ContentDigest                                     evidence.Digest
	BusinessBindingSHA256                             string
	StatementWrote, AlreadyMatched, ReferenceAppended bool
}
type SQLHistoricalBatchCASReport struct {
	Version, DatabaseIdentitySHA256, ExpectedBusinessRowsSHA256                 string
	Locations                                                                   []SQLHistoricalBatchCASLocation
	StatementApplied, HostCommitRequired, IndependentReadbackRequired           bool
	HostCommitVerified, SourceAuthenticated, BusinessClosureVerified, DropReady bool
}

func (*SQLHistoricalBatchCASPlan) MarshalJSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalBatchCASPlan) MarshalBSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalBatchCASPlan) String() string {
	return "private SQL historical CAS plan; not closure authorization"
}
func (p *SQLHistoricalBatchCASPlan) GoString() string { return p.String() }
func (*SQLHistoricalBatchCASStatement) MarshalJSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalBatchCASStatement) MarshalBSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalBatchCASStatement) String() string {
	return "private uncommitted SQL historical CAS statement facts"
}
func (s *SQLHistoricalBatchCASStatement) GoString() string { return s.String() }

func casCloneImage(v sqlHistoricalCASImage) sqlHistoricalCASImage {
	out := sqlHistoricalCASImage{rows: map[string][]historicalSQLRow{}, schema: map[string]string{}, columns: map[string][]string{}}
	for k, rows := range v.rows {
		if rows == nil {
			out.rows[k] = nil
			continue
		}
		out.rows[k] = make([]historicalSQLRow, len(rows))
		for i, row := range rows {
			out.rows[k][i] = historicalSQLRow{}
			for c, val := range row {
				if val == nil {
					out.rows[k][i][c] = nil
				} else {
					copy := *val
					out.rows[k][i][c] = &copy
				}
			}
		}
	}
	for k, v := range v.schema {
		out.schema[k] = v
	}
	for k, v := range v.columns {
		out.columns[k] = slices.Clone(v)
	}
	return out
}
func casImageHash(v sqlHistoricalCASImage) string {
	return (&SQLHistoricalOwnerBatch{rows: v.rows, schema: v.schema}).rowsHash()
}
func casRow(v sqlHistoricalCASImage, table string, id uint64) (historicalSQLRow, error) {
	var found historicalSQLRow
	for _, row := range v.rows[table] {
		n, e := sqlHistoricalUint(row, "id")
		if e != nil {
			return nil, ErrSQLHistoricalBatchCAS
		}
		if n == id {
			if found != nil {
				return nil, ErrSQLHistoricalBatchCAS
			}
			found = row
		}
	}
	if found == nil {
		return nil, ErrSQLHistoricalBatchCAS
	}
	return found, nil
}
func casTarget(v sqlHistoricalCASImage, a SQLHistoricalBatchAttachment) (string, string, uint64, historicalSQLRow, historicalSQLRow, error) {
	return casTargetForBinding(v, a, false)
}

// bindingOnly is private to the digest reader. It never permits Prepare/Apply
// to bypass the exact Unverifiable conclusion or any full-row CAS invariant.
func casTargetForBinding(v sqlHistoricalCASImage, a SQLHistoricalBatchAttachment, bindingOnly bool) (string, string, uint64, historicalSQLRow, historicalSQLRow, error) {
	table, column, id := "assessment", "historical_lifecycle_evidence", a.AssessmentID
	if a.OutcomeID != 0 {
		table, column, id = "evaluation_outcome", "historical_committed_evidence", a.OutcomeID
	}
	if a.AssessmentID == 0 || !historicalAllowed(table, a.Entry.EventType) {
		return "", "", 0, nil, nil, ErrSQLHistoricalBatchCAS
	}
	owner, e := casRow(v, "assessment", a.AssessmentID)
	if e != nil {
		return "", "", 0, nil, nil, e
	}
	row, e := casRow(v, table, id)
	if e != nil {
		return "", "", 0, nil, nil, e
	}
	if table == "evaluation_outcome" && (valueOrEmpty(row["assessment_id"]) != strconv.FormatUint(a.AssessmentID, 10) || valueOrEmpty(row["org_id"]) != valueOrEmpty(owner["org_id"]) || valueOrEmpty(row["testee_id"]) != valueOrEmpty(owner["testee_id"])) {
		return "", "", 0, nil, nil, ErrSQLHistoricalBatchCAS
	}
	var run historicalSQLRow
	if a.Entry.Run != nil {
		for _, r := range v.rows["runtime_checkpoint"] {
			if valueOrEmpty(r["resource_id"]) == a.Entry.Run.RunID {
				if run != nil || valueOrEmpty(r["scope"]) != "evaluation_run" || valueOrEmpty(r["assessment_id"]) != strconv.FormatUint(a.AssessmentID, 10) || valueOrEmpty(r["attempt_no"]) != strconv.FormatUint(uint64(a.Entry.Run.Attempt), 10) || r["deleted_at"] != nil {
					return "", "", 0, nil, nil, ErrSQLHistoricalBatchCAS
				}
				run = r
			}
		}
		if run == nil {
			return "", "", 0, nil, nil, ErrSQLHistoricalBatchCAS
		}
	}
	if table == "evaluation_outcome" {
		originalID := valueOrEmpty(row["evaluation_run_id"])
		if a.Entry.Run != nil {
			if run == nil || originalID != a.Entry.Run.RunID {
				return "", "", 0, nil, nil, ErrSQLHistoricalBatchCAS
			}
		} else {
			if originalID == "" || valueOrEmpty(owner["status"]) != "evaluated" || (!bindingOnly && !historicalOutcomeRunAbsent(a.Entry)) {
				return "", "", 0, nil, nil, ErrSQLHistoricalBatchCAS
			}
			// No org/scope/deleted filter may hide a retained contradictory Run.
			for _, r := range v.rows["runtime_checkpoint"] {
				if valueOrEmpty(r["resource_id"]) == originalID {
					return "", "", 0, nil, nil, ErrSQLHistoricalBatchCAS
				}
			}
		}
	}
	return table, column, id, row, run, nil
}

// This reconstructs an actual immutable owner/declared original Run anchor. It
// is not evidence that the caller's event bytes, terminal flags or source are true.
func SQLHistoricalBatchBinding(ctx context.Context, b *SQLHistoricalOwnerBatch, assessmentID, outcomeID uint64, eventType string, run *evidence.HistoricalRunReferenceV1) (string, error) {
	if b == nil || !b.report.Complete {
		return "", ErrSQLHistoricalBatchCAS
	}
	if e := b.ValidateBorrowedSnapshot(ctx); e != nil {
		return "", e
	}
	a := SQLHistoricalBatchAttachment{AssessmentID: assessmentID, OutcomeID: outcomeID, Entry: evidence.HistoricalReferenceEntryV1{EventType: eventType, Run: run}}
	table, _, _, row, r, e := casTargetForBinding(sqlHistoricalCASImage{rows: b.rows}, a, true)
	if e != nil {
		return "", e
	}
	tx, e := historicalTx(ctx)
	if e != nil {
		return "", e
	}
	server, database, e := historicalDatabase(tx)
	if e != nil {
		return "", e
	}
	if table == "evaluation_outcome" && run == nil {
		if e := casMissingOriginalRuns(tx, []string{valueOrEmpty(row["evaluation_run_id"])}, false); e != nil {
			return "", e
		}
	}
	return historicalStableBinding(server, database, table, eventType, row, r)
}

func PrepareSQLHistoricalBatchCAS(ctx context.Context, b *SQLHistoricalOwnerBatch, attachments []SQLHistoricalBatchAttachment) (*SQLHistoricalBatchCASPlan, error) {
	if b == nil || !b.report.Complete || len(attachments) == 0 || len(attachments) > 512 {
		return nil, ErrSQLHistoricalBatchCAS
	}
	if e := b.ValidateBorrowedSnapshot(ctx); e != nil {
		return nil, e
	}
	tx, e := historicalTx(ctx)
	if e != nil {
		return nil, e
	}
	server, database, e := historicalDatabase(tx)
	if e != nil {
		return nil, e
	}
	p := &SQLHistoricalBatchCASPlan{oldTransaction: b.cycle.transaction, identity: b.report.DatabaseIdentitySHA256, server: server, database: database, request: SQLHistoricalOwnerBatchRequest{AssessmentIDs: slices.Clone(b.request.AssessmentIDs), AnswerSheetIDs: slices.Clone(b.request.AnswerSheetIDs)}, limits: b.limits, before: casCloneImage(sqlHistoricalCASImage{rows: b.rows, schema: b.schema, columns: b.columns})}
	head, _, _, e := cycleQuery(tx, "SELECT version,dirty FROM schema_migrations ORDER BY version", 2)
	if e != nil || len(head) != 1 || valueOrEmpty(head[0]["dirty"]) != "0" {
		return nil, ErrSQLHistoricalBatchCAS
	}
	p.before.rows["cas_migration_head"] = head
	type referenceIdentity struct {
		table string
		id    uint64
		entry evidence.HistoricalReferenceEntryV1
	}
	byEvent, bySource := map[string]referenceIdentity{}, map[string]referenceIdentity{}
	missingRuns := map[string]bool{}
	register := func(table string, id uint64, entry evidence.HistoricalReferenceEntryV1) error {
		value := referenceIdentity{table: table, id: id, entry: entry}
		source := entry.Source.Database + ":" + entry.Source.Object + ":" + entry.Source.PrimaryKeySHA256
		if prior, ok := byEvent[entry.EventID]; ok && !reflect.DeepEqual(prior, value) {
			return evidence.ErrHistoricalReferenceConflict
		}
		if prior, ok := bySource[source]; ok && !reflect.DeepEqual(prior, value) {
			return evidence.ErrHistoricalReferenceConflict
		}
		byEvent[entry.EventID], bySource[source] = value, value
		if table == "evaluation_outcome" && entry.Run == nil {
			row, err := casRow(p.before, table, id)
			if err != nil || !historicalOutcomeRunAbsent(entry) {
				return ErrSQLHistoricalBatchCAS
			}
			ownerID, err := sqlHistoricalUint(row, "assessment_id")
			facts := b.owners[ownerID]
			if err != nil || facts == nil {
				return ErrSQLHistoricalBatchCAS
			}
			// The new missing-Run exception cannot settle an actual current
			// message or lease. These facts come from this opaque SQL8 cycle;
			// source/global/joint authority remains independently required.
			for _, responsibility := range facts.snapshot.Responsibilities {
				if responsibility.Unfinished || responsibility.LeasePresent {
					return ErrSQLHistoricalBatchCAS
				}
			}
			missingRuns[valueOrEmpty(row["evaluation_run_id"])] = true
		}
		return nil
	}
	for _, table := range []string{"assessment", "evaluation_outcome"} {
		column := "historical_lifecycle_evidence"
		if table == "evaluation_outcome" {
			column = "historical_committed_evidence"
		}
		for _, row := range p.before.rows[table] {
			stored, err := historicalDecode(row[column])
			if err != nil {
				return nil, err
			}
			if stored == nil {
				continue
			}
			id, err := sqlHistoricalUint(row, "id")
			if err != nil {
				return nil, err
			}
			owner, outcome := id, uint64(0)
			if table == "evaluation_outcome" {
				owner, err = sqlHistoricalUint(row, "assessment_id")
				if err != nil {
					return nil, err
				}
				outcome = id
			}
			for _, entry := range stored.Entries {
				a := SQLHistoricalBatchAttachment{AssessmentID: owner, OutcomeID: outcome, Entry: entry}
				priorTable, _, _, priorRow, priorRun, err := casTarget(p.before, a)
				if err != nil {
					return nil, err
				}
				binding, err := historicalStableBinding(server, database, priorTable, entry.EventType, priorRow, priorRun)
				if err != nil || binding != entry.Proof.BusinessBindingSHA256 {
					return nil, ErrSQLHistoricalBatchCAS
				}
				if err = register(table, id, entry); err != nil {
					return nil, err
				}
			}
		}
	}
	byGroup := map[string]int{}
	seen := map[string]SQLHistoricalBatchAttachment{}
	for _, input := range attachments {
		a := input
		a.Entry = input.Entry.Clone()
		if e = a.Entry.Validate(); e != nil {
			return nil, e
		}
		if a.Entry.Source.Digest.Kind != "mysql-cast-binary-row-v2" || a.ContentDigest.Kind != "legacy-domain-json-bytes-v1" || !evidence.ValidSHA256(a.ContentDigest.SHA256) {
			return nil, ErrSQLHistoricalBatchCAS
		}
		table, column, id, row, run, err := casTarget(p.before, a)
		if err != nil {
			return nil, err
		}
		binding, err := historicalStableBinding(server, database, table, a.Entry.EventType, row, run)
		if err != nil || binding != a.Entry.Proof.BusinessBindingSHA256 {
			return nil, ErrSQLHistoricalBatchCAS
		}
		if prior, ok := seen[a.Entry.EventID]; ok {
			if !reflect.DeepEqual(prior, a) {
				return nil, evidence.ErrHistoricalReferenceConflict
			}
			continue
		}
		seen[a.Entry.EventID] = a
		if err = register(table, id, a.Entry); err != nil {
			return nil, err
		}
		key := table + ":" + strconv.FormatUint(id, 10)
		position, ok := byGroup[key]
		if !ok {
			stored, err := historicalDecode(row[column])
			if err != nil {
				return nil, err
			}
			p.groups = append(p.groups, sqlHistoricalCASGroup{table: table, column: column, id: id, set: stored})
			position = len(p.groups) - 1
			byGroup[key] = position
		}
		group := &p.groups[position]
		group.entries = append(group.entries, a.Entry)
		next, err := group.set.Append(a.Entry)
		if err != nil {
			return nil, err
		}
		group.set = next
		p.attachments = append(p.attachments, a)
	}
	for id := range missingRuns {
		p.missingOriginalRunIDs = append(p.missingOriginalRunIDs, id)
	}
	slices.Sort(p.missingOriginalRunIDs)
	if len(p.missingOriginalRunIDs) != 0 {
		if e = casMissingOriginalRuns(tx, p.missingOriginalRunIDs, false); e != nil {
			return nil, e
		}
	}
	if e = b.ValidateBorrowedSnapshot(ctx); e != nil {
		return nil, e
	}
	return p, nil
}

// An owner batch is not a global absence proof. Recheck the declared IDs across
// every scope/owner/deleted state; any retained row is a conflict, not a gap.
func casMissingOriginalRuns(tx *gorm.DB, ids []string, lock bool) error {
	if len(ids) == 0 || len(ids) > 512 {
		return ErrSQLHistoricalBatchCAS
	}
	for _, id := range ids {
		if id == "" {
			return ErrSQLHistoricalBatchCAS
		}
	}
	q := "SELECT * FROM runtime_checkpoint WHERE CAST(resource_id AS BINARY) IN ? ORDER BY id LIMIT ?"
	if lock {
		q += " FOR UPDATE"
	}
	rows, _, _, err := cycleQuery(tx, q, 1, ids, 2)
	if err != nil {
		return err
	}
	if len(rows) != 0 {
		return ErrSQLHistoricalBatchCAS
	}
	return nil
}

func casActualRW(tx *gorm.DB) (sqlResponsibilityTransaction, error) {
	rows, _, _, e := cycleQuery(tx, "SELECT t.PROCESSLIST_ID AS connection_id,e.THREAD_ID AS thread_id,e.EVENT_ID AS event_id,e.STATE AS state,e.END_EVENT_ID AS end_event_id,e.ACCESS_MODE AS access_mode,e.ISOLATION_LEVEL AS isolation_level,e.AUTOCOMMIT AS autocommit FROM performance_schema.events_transactions_current e JOIN performance_schema.threads t ON t.THREAD_ID=e.THREAD_ID WHERE t.PROCESSLIST_ID=CONNECTION_ID()", 2)
	if e != nil || len(rows) != 1 || valueOrEmpty(rows[0]["state"]) != "ACTIVE" || rows[0]["end_event_id"] != nil {
		return sqlResponsibilityTransaction{}, ErrSQLResponsibilityVisibility
	}
	r := rows[0]
	if valueOrEmpty(r["access_mode"]) != "READ WRITE" || valueOrEmpty(r["isolation_level"]) != "REPEATABLE READ" || valueOrEmpty(r["autocommit"]) != "NO" {
		return sqlResponsibilityTransaction{}, ErrSQLResponsibilityTransaction
	}
	a, e1 := sqlHistoricalUint(r, "connection_id")
	b, e2 := sqlHistoricalUint(r, "thread_id")
	c, e3 := sqlHistoricalUint(r, "event_id")
	if e1 != nil || e2 != nil || e3 != nil {
		return sqlResponsibilityTransaction{}, ErrSQLResponsibilityVisibility
	}
	return sqlResponsibilityTransaction{a, b, c}, nil
}

// Full globally selected row sets include additions/deletions, future columns,
// physical NULL and actual server JSON bytes; no org/soft-delete predicate hides
// a conflicting owner. All selected rows are locked before any evidence write.
func (p *SQLHistoricalBatchCASPlan) capture(tx *gorm.DB, lock bool) (sqlHistoricalCASImage, error) {
	v := sqlHistoricalCASImage{rows: map[string][]historicalSQLRow{}, schema: map[string]string{}, columns: map[string][]string{}}
	for _, table := range batchBusinessTables {
		cols, hash, _, e := cycleSchema(tx, sqlResponsibilityTable{name: table, keys: []string{"id"}})
		if e != nil {
			return v, e
		}
		v.schema[table], v.columns[table] = hash, cols
	}
	var count int
	var size uint64
	read := func(table, index, predicate string, args ...any) ([]historicalSQLRow, error) {
		q := "SELECT * FROM `" + table + "` FORCE INDEX (`" + index + "`) WHERE " + predicate + " ORDER BY id LIMIT ?"
		if lock {
			q += " FOR UPDATE"
		}
		remaining := p.limits.MaxRows - count
		if remaining <= 0 {
			return nil, ErrSQLHistoricalBatchBounds
		}
		rows, names, bytes, e := cycleQuery(tx, q, remaining, append(args, remaining+1)...)
		if e != nil {
			return nil, e
		}
		if !reflect.DeepEqual(names, v.columns[table]) || bytes > p.limits.MaxBytes-size {
			return nil, ErrSQLHistoricalBatchConflict
		}
		count += len(rows)
		size += bytes
		return rows, nil
	}
	selected := map[uint64]historicalSQLRow{}
	for _, selector := range []struct {
		ids           []uint64
		index, column string
	}{{p.request.AssessmentIDs, "PRIMARY", "id"}, {p.request.AnswerSheetIDs, "uk_answer_sheet_id", "answer_sheet_id"}} {
		if len(selector.ids) == 0 {
			continue
		}
		rows, e := read("assessment", selector.index, selector.column+" IN ?", selector.ids)
		if e != nil {
			return v, e
		}
		for _, row := range rows {
			id, e := sqlHistoricalUint(row, "id")
			if e != nil {
				return v, e
			}
			if prior := selected[id]; prior != nil && !reflect.DeepEqual(prior, row) {
				return v, ErrSQLHistoricalBatchConflict
			}
			selected[id] = row
		}
	}
	ids := make([]uint64, 0, len(selected))
	for id := range selected {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		v.rows["assessment"] = append(v.rows["assessment"], selected[id])
	}
	if len(ids) == 0 {
		v.rows["assessment"] = []historicalSQLRow{}
	}
	for _, pair := range [][2]string{{"runtime_checkpoint", "idx_runtime_checkpoint_assessment_id"}, {"evaluation_outcome", "uk_evaluation_outcome_assessment_id"}} {
		v.rows[pair[0]] = []historicalSQLRow{}
		if len(ids) > 0 {
			rows, e := read(pair[0], pair[1], "assessment_id IN ?", ids)
			if e != nil {
				return v, e
			}
			v.rows[pair[0]] = rows
		}
	}
	if len(p.missingOriginalRunIDs) != 0 {
		if e := casMissingOriginalRuns(tx, p.missingOriginalRunIDs, lock); e != nil {
			return v, e
		}
	}
	for key := range p.before.rows {
		if key == "business_clock_columns" {
			rows, _, _, e := cycleQuery(tx, "SELECT TABLE_NAME AS table_name,COLUMN_NAME AS column_name,DATA_TYPE AS data_type,DATETIME_PRECISION AS datetime_precision,COLUMN_TYPE AS column_type,IS_NULLABLE AS is_nullable FROM information_schema.columns WHERE table_schema=DATABASE() AND ((table_name='assessment' AND column_name IN ('submitted_at','evaluated_at','failed_at')) OR (table_name='runtime_checkpoint' AND column_name IN ('finished_at','lease_expires_at','next_attempt_at')) OR (table_name='evaluation_outcome' AND column_name='evaluated_at')) ORDER BY TABLE_NAME,COLUMN_NAME", 7)
			if e != nil {
				return v, e
			}
			v.rows[key] = rows
		} else if key == "cas_migration_head" {
			q := "SELECT version,dirty FROM schema_migrations ORDER BY version"
			if lock {
				q += " FOR UPDATE"
			}
			rows, _, _, e := cycleQuery(tx, q, 2)
			if e != nil {
				return v, e
			}
			v.rows[key] = rows
		} else if len(key) > 15 && key[:15] == "verified_index:" {
			parts := splitCASIndex(key)
			if len(parts) != 2 {
				return v, ErrSQLHistoricalBatchCAS
			}
			rows, _, _, e := cycleQuery(tx, "SELECT INDEX_NAME AS index_name,NON_UNIQUE AS non_unique,SEQ_IN_INDEX AS seq_in_index,COLUMN_NAME AS column_name,SUB_PART AS sub_part,COLLATION AS collation,INDEX_TYPE AS index_type,IS_VISIBLE AS is_visible,EXPRESSION AS expression FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name=? AND index_name=? ORDER BY SEQ_IN_INDEX", 2, parts[0], parts[1])
			if e != nil {
				return v, e
			}
			v.rows[key] = rows
		}
	}
	for _, table := range batchBusinessTables {
		_, hash, _, e := cycleSchema(tx, sqlResponsibilityTable{name: table, keys: []string{"id"}})
		if e != nil || hash != v.schema[table] {
			return v, ErrSQLHistoricalBatchConflict
		}
	}
	return v, nil
}
func splitCASIndex(key string) []string {
	for i := 15; i < len(key); i++ {
		if key[i] == ':' {
			return []string{key[15:i], key[i+1:]}
		}
	}
	return nil
}

func (p *SQLHistoricalBatchCASPlan) expectedImage(tx *gorm.DB) (sqlHistoricalCASImage, error) {
	v := casCloneImage(p.before)
	for _, g := range p.groups {
		raw, e := json.Marshal(g.set)
		if e != nil {
			return v, e
		}
		rows, _, _, e := cycleQuery(tx, "SELECT CAST(? AS JSON) AS evidence", 1, string(raw))
		if e != nil || len(rows) != 1 || rows[0]["evidence"] == nil {
			return v, ErrSQLHistoricalBatchCAS
		}
		row, e := casRow(v, g.table, g.id)
		if e != nil {
			return v, e
		}
		row[g.column] = rows[0]["evidence"]
	}
	return v, nil
}

func (p *SQLHistoricalBatchCASPlan) Apply(ctx context.Context) (*SQLHistoricalBatchCASStatement, error) {
	if p == nil || len(p.groups) == 0 {
		return nil, ErrSQLHistoricalBatchCAS
	}
	tx, e := historicalTx(ctx)
	if e != nil {
		return nil, e
	}
	server, database, e := historicalDatabase(tx)
	if e != nil || server != p.server || database != p.database {
		return nil, ErrSQLHistoricalFactsIdentity
	}
	// Establish a real transaction before inspecting P_S. The full locking read
	// is still only a read: no source/closure gate is inferred from this Tx.
	current, e := p.capture(tx, true)
	if e != nil {
		return nil, e
	}
	transaction, e := casActualRW(tx)
	if e != nil {
		return nil, e
	}
	if transaction == p.oldTransaction {
		return nil, ErrSQLHistoricalBatchCAS
	}
	expected, e := p.expectedImage(tx)
	if e != nil {
		return nil, e
	}
	sameOld, sameNew := reflect.DeepEqual(current, p.before), reflect.DeepEqual(current, expected)
	if !sameOld && !sameNew {
		return nil, ErrSQLHistoricalBatchConflict
	}
	written := map[string]bool{}
	if sameOld && !sameNew {
		for _, g := range p.groups {
			row, _ := casRow(current, g.table, g.id)
			next, _ := casRow(expected, g.table, g.id)
			if reflect.DeepEqual(row[g.column], next[g.column]) {
				continue
			}
			q := "UPDATE `" + g.table + "` SET `" + g.column + "`=?"
			if _, exists := row["updated_at"]; exists {
				q += ",updated_at=updated_at"
			}
			q += " WHERE id=? AND "
			args := []any{*next[g.column], g.id}
			if row[g.column] == nil {
				q += "`" + g.column + "` IS NULL"
			} else {
				q += "CAST(`" + g.column + "` AS BINARY)=CAST(? AS BINARY)"
				args = append(args, *row[g.column])
			}
			result := tx.Exec(q, args...)
			if result.Error != nil {
				return nil, result.Error
			}
			if result.RowsAffected != 1 {
				return nil, ErrSQLHistoricalBatchConflict
			}
			written[g.table+":"+strconv.FormatUint(g.id, 10)] = true
		}
	}
	after, e := p.capture(tx, true)
	if e != nil {
		return nil, e
	}
	if !reflect.DeepEqual(after, expected) {
		return nil, ErrSQLHistoricalBatchConflict
	}
	actual, e := casActualRW(tx)
	if e != nil || actual != transaction {
		return nil, ErrSQLHistoricalBatchCAS
	}
	return &SQLHistoricalBatchCASStatement{plan: p, transaction: transaction, expected: after, written: written}, nil
}

func (s *SQLHistoricalBatchCASStatement) Report() SQLHistoricalBatchCASReport {
	r := SQLHistoricalBatchCASReport{Version: "sql-historical-batch-cas-statement/v1", HostCommitRequired: true, IndependentReadbackRequired: true}
	if s == nil || s.plan == nil {
		return r
	}
	r.DatabaseIdentitySHA256 = s.plan.identity
	r.ExpectedBusinessRowsSHA256 = casImageHash(s.expected)
	r.StatementApplied = true
	for _, a := range s.plan.attachments {
		table, column, id, _, _, e := casTarget(s.plan.before, a)
		if e != nil {
			return SQLHistoricalBatchCASReport{HostCommitRequired: true, IndependentReadbackRequired: true}
		}
		wrote := s.written[table+":"+strconv.FormatUint(id, 10)]
		before, _ := casRow(s.plan.before, table, id)
		stored, _ := historicalDecode(before[column])
		existed := false
		if stored != nil {
			for _, entry := range stored.Entries {
				if reflect.DeepEqual(entry, a.Entry) {
					existed = true
					break
				}
			}
		}
		r.Locations = append(r.Locations, SQLHistoricalBatchCASLocation{Table: table, Column: column, ID: id, EventID: a.Entry.EventID, EventType: a.Entry.EventType, Source: a.Entry.Source, ContentDigest: a.ContentDigest, BusinessBindingSHA256: a.Entry.Proof.BusinessBindingSHA256, StatementWrote: wrote, AlreadyMatched: existed || !wrote, ReferenceAppended: wrote && !existed})
	}
	return r
}

// VerifyPersisted observes the exact expected raw state, after a host Commit,
// in an independently prepared RR-RO transaction. It never certifies a Commit
// response or original source/closure/fence and cannot authorize DROP.
func (s *SQLHistoricalBatchCASStatement) VerifyPersisted(ctx context.Context, fresh *SQLHistoricalOwnerBatch) error {
	if s == nil || s.plan == nil || fresh == nil || !fresh.report.Complete || fresh.cycle == nil || fresh.cycle.transaction == s.transaction || fresh.cycle.transaction == s.plan.oldTransaction || fresh.report.DatabaseIdentitySHA256 != s.plan.identity || !reflect.DeepEqual(fresh.request, s.plan.request) {
		return ErrSQLHistoricalBatchCAS
	}
	if e := fresh.ValidateBorrowedSnapshot(ctx); e != nil {
		return e
	}
	tx, e := historicalTx(ctx)
	if e != nil {
		return e
	}
	head, _, _, e := cycleQuery(tx, "SELECT version,dirty FROM schema_migrations ORDER BY version", 2)
	if e != nil {
		return e
	}
	if len(s.plan.missingOriginalRunIDs) != 0 {
		if e := casMissingOriginalRuns(tx, s.plan.missingOriginalRunIDs, false); e != nil {
			return e
		}
	}
	observed := casCloneImage(sqlHistoricalCASImage{rows: fresh.rows, schema: fresh.schema, columns: fresh.columns})
	observed.rows["cas_migration_head"] = head
	if !reflect.DeepEqual(observed, s.expected) {
		return ErrSQLHistoricalBatchConflict
	}
	return fresh.ValidateBorrowedSnapshot(ctx)
}
