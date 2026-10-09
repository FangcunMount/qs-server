package evaluation

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"time"

	"gorm.io/gorm"
)

var ErrSQLHistoricalCASProvenance = errors.New("sql_historical_cas_provenance_rejected")

// This observes an actual prepared baseline. It does not authorize Apply or
// certify original-message closure, a successful Commit response, or DROP.
type SQLHistoricalCASProvenance struct {
	self     *SQLHistoricalCASProvenance
	plan     *SQLHistoricalBatchCASPlan
	readPool gorm.ConnPool
	seal     string
}

// This binds one actual Apply result to its prepared instance and borrowed RW
// pool. Ending that pool proves only epoch termination, not Commit vs Rollback.
type SQLHistoricalCASStatementBinding struct {
	self      *SQLHistoricalCASStatementBinding
	original  *SQLHistoricalCASProvenance
	statement *SQLHistoricalBatchCASStatement
	writePool gorm.ConnPool
	seal      string
}

// A Mongo-only evidence page still needs the exact unchanged SQL dependency
// selection, including absent AnswerSheet ranges. Retain a bounded raw image,
// not the entire original responsibility cycle/owner graph.
type SQLHistoricalCASReadBaseline struct {
	self     *SQLHistoricalCASReadBaseline
	before   sqlHistoricalCASImage
	request  SQLHistoricalOwnerBatchRequest
	identity string
	readPool gorm.ConnPool
	seal     string
}

func (*SQLHistoricalCASReadBaseline) MarshalJSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalCASReadBaseline) MarshalBSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalCASReadBaseline) UnmarshalJSON([]byte) error {
	return ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalCASReadBaseline) UnmarshalBSON([]byte) error {
	return ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalCASReadBaseline) String() string {
	return "private unchanged SQL dependency baseline; not closure authority"
}
func (b *SQLHistoricalCASReadBaseline) GoString() string { return b.String() }
func (b *SQLHistoricalCASReadBaseline) digest() string {
	if b == nil || historicalNil(b.readPool) {
		return ""
	}
	selectors, err := json.Marshal(b.request)
	if err != nil {
		return ""
	}
	return cycleKeyDigest([]string{"sql-cas-unchanged-read-baseline/v1", b.identity, sqlHistoricalProvenancePoolToken(b.readPool), string(selectors), casImageHash(b.before)})
}
func (b *SQLHistoricalCASReadBaseline) intact() bool {
	return b != nil && b.self == b && b.seal != "" && b.seal == b.digest()
}
func SealSQLHistoricalCASReadBaseline(ctx context.Context, original *SQLHistoricalOwnerBatch) (*SQLHistoricalCASReadBaseline, error) {
	if ctx == nil || ctx.Err() != nil || original == nil || !original.report.Complete || original.ValidateBorrowedSnapshot(ctx) != nil {
		return nil, ErrSQLHistoricalCASProvenance
	}
	tx, err := historicalTx(ctx)
	if err != nil {
		return nil, err
	}
	head, _, _, err := cycleQuery(tx, "SELECT version,dirty FROM schema_migrations ORDER BY version", 2)
	if err != nil || len(head) != 1 || valueOrEmpty(head[0]["dirty"]) != "0" {
		return nil, ErrSQLHistoricalCASProvenance
	}
	image := casCloneImage(sqlHistoricalCASImage{rows: original.rows, schema: original.schema, columns: original.columns})
	image.rows["cas_migration_head"] = head
	b := &SQLHistoricalCASReadBaseline{before: image, request: SQLHistoricalOwnerBatchRequest{AssessmentIDs: append([]uint64(nil), original.request.AssessmentIDs...), AnswerSheetIDs: append([]uint64(nil), original.request.AnswerSheetIDs...)}, identity: original.report.DatabaseIdentitySHA256, readPool: tx.Statement.ConnPool}
	b.self, b.seal = b, b.digest()
	if !b.intact() || original.ValidateBorrowedSnapshot(ctx) != nil {
		return nil, ErrSQLHistoricalCASProvenance
	}
	return b, nil
}
func (b *SQLHistoricalCASReadBaseline) VerifyUnchanged(ctx context.Context, fresh *SQLHistoricalOwnerBatch) error {
	if ctx == nil || ctx.Err() != nil || !b.intact() || fresh == nil || !fresh.report.Complete || !reflect.DeepEqual(fresh.request, b.request) || fresh.report.DatabaseIdentitySHA256 != b.identity || fresh.ValidateBorrowedSnapshot(ctx) != nil {
		return ErrSQLHistoricalCASProvenance
	}
	tx, err := historicalTx(ctx)
	if err != nil {
		return err
	}
	if tx.Statement.ConnPool == b.readPool || sqlHistoricalEndedPool(ctx, b.readPool) != nil {
		return ErrSQLHistoricalCASProvenance
	}
	head, _, _, err := cycleQuery(tx, "SELECT version,dirty FROM schema_migrations ORDER BY version", 2)
	if err != nil {
		return err
	}
	observed := casCloneImage(sqlHistoricalCASImage{rows: fresh.rows, schema: fresh.schema, columns: fresh.columns})
	observed.rows["cas_migration_head"] = head
	if !reflect.DeepEqual(observed, b.before) || !b.intact() {
		return ErrSQLHistoricalCASProvenance
	}
	return fresh.ValidateBorrowedSnapshot(ctx)
}

func (*SQLHistoricalCASProvenance) MarshalJSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalCASProvenance) MarshalBSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalCASProvenance) UnmarshalJSON([]byte) error {
	return ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalCASProvenance) UnmarshalBSON([]byte) error {
	return ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalCASProvenance) String() string {
	return "private SQL CAS baseline provenance; not write authority"
}
func (p *SQLHistoricalCASProvenance) GoString() string { return p.String() }
func (*SQLHistoricalCASStatementBinding) MarshalJSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalCASStatementBinding) MarshalBSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalCASStatementBinding) UnmarshalJSON([]byte) error {
	return ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalCASStatementBinding) UnmarshalBSON([]byte) error {
	return ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalCASStatementBinding) String() string {
	return "private SQL Apply provenance; commit outcome unproven"
}
func (s *SQLHistoricalCASStatementBinding) GoString() string { return s.String() }

func (p *SQLHistoricalCASProvenance) digest() string {
	if p == nil || p.plan == nil || historicalNil(p.readPool) {
		return ""
	}
	groups := make([]string, len(p.plan.groups))
	for i, g := range p.plan.groups {
		value, err := json.Marshal(struct {
			Table, Column string
			ID            uint64
			Set           any
			Entries       any
		}{g.table, g.column, g.id, g.set, g.entries})
		if err != nil {
			return ""
		}
		groups[i] = cycleKeyDigest([]string{string(value)})
	}
	metadata, err := json.Marshal(struct {
		Request        SQLHistoricalOwnerBatchRequest
		Limits         SQLHistoricalOwnerBatchLimits
		OldTransaction [3]uint64
		Attachments    []SQLHistoricalBatchAttachment
		Groups         []string
	}{p.plan.request, p.plan.limits, [3]uint64{p.plan.oldTransaction.connection, p.plan.oldTransaction.thread, p.plan.oldTransaction.event}, p.plan.attachments, groups})
	if err != nil {
		return ""
	}
	return cycleKeyDigest([]string{"sql-cas-provenance/v1", sqlHistoricalProvenancePoolToken(p.readPool), p.plan.identity, p.plan.server, p.plan.database, casImageHash(p.plan.before), string(metadata)})
}
func (p *SQLHistoricalCASProvenance) intact() bool {
	return p != nil && p.self == p && p.seal != "" && p.seal == p.digest()
}
func (s *SQLHistoricalCASStatementBinding) digest() string {
	if s == nil || s.original == nil || !s.original.intact() || s.statement == nil || s.statement.plan != s.original.plan || historicalNil(s.writePool) {
		return ""
	}
	metadata, err := json.Marshal(s.statement.Report())
	if err != nil {
		return ""
	}
	return cycleKeyDigest([]string{"sql-cas-apply-provenance/v1", sqlHistoricalProvenancePoolToken(s.writePool), s.original.seal, strconv.FormatUint(s.statement.transaction.connection, 10), strconv.FormatUint(s.statement.transaction.thread, 10), strconv.FormatUint(s.statement.transaction.event, 10), casImageHash(s.statement.expected), string(metadata)})
}

func sqlHistoricalProvenancePoolToken(pool gorm.ConnPool) string {
	if historicalNil(pool) {
		return ""
	}
	v := reflect.ValueOf(pool)
	if v.Kind() != reflect.Pointer {
		return ""
	}
	return v.Type().String() + ":" + strconv.FormatUint(uint64(v.Pointer()), 16)
}
func (s *SQLHistoricalCASStatementBinding) intact() bool {
	return s != nil && s.self == s && s.seal != "" && s.seal == s.digest()
}

// Seal is called in the ORIGINAL actual RR-RO owner epoch while it is still
// alive. Complete raw columns/NULL/schema/head/request and the real transaction
// are compared. A caller-created report cannot manufacture this capability.
func SealSQLHistoricalCASProvenance(ctx context.Context, plan *SQLHistoricalBatchCASPlan, original *SQLHistoricalOwnerBatch) (*SQLHistoricalCASProvenance, error) {
	if ctx == nil || ctx.Err() != nil || plan == nil || original == nil || !original.report.Complete || original.cycle == nil || len(plan.groups) == 0 || len(plan.attachments) == 0 || len(plan.attachments) > 512 {
		return nil, ErrSQLHistoricalCASProvenance
	}
	if err := original.ValidateBorrowedSnapshot(ctx); err != nil {
		return nil, err
	}
	tx, err := historicalTx(ctx)
	if err != nil {
		return nil, err
	}
	server, database, err := historicalDatabase(tx)
	if err != nil || server != plan.server || database != plan.database || plan.identity != original.report.DatabaseIdentitySHA256 || plan.oldTransaction != original.cycle.transaction || !reflect.DeepEqual(plan.request, original.request) || plan.limits != original.limits {
		return nil, ErrSQLHistoricalCASProvenance
	}
	head, _, _, err := cycleQuery(tx, "SELECT version,dirty FROM schema_migrations ORDER BY version", 2)
	if err != nil || len(head) != 1 || valueOrEmpty(head[0]["dirty"]) != "0" {
		return nil, ErrSQLHistoricalCASProvenance
	}
	observed := casCloneImage(sqlHistoricalCASImage{rows: original.rows, schema: original.schema, columns: original.columns})
	observed.rows["cas_migration_head"] = head
	if !reflect.DeepEqual(observed, plan.before) {
		return nil, ErrSQLHistoricalCASProvenance
	}
	if err = original.ValidateBorrowedSnapshot(ctx); err != nil {
		return nil, err
	}
	p := &SQLHistoricalCASProvenance{plan: plan, readPool: tx.Statement.ConnPool}
	p.self, p.seal = p, p.digest()
	if !p.intact() {
		return nil, ErrSQLHistoricalCASProvenance
	}
	return p, nil
}

// Attachments is a defensive diagnostic view only. It cannot create a plan,
// statement binding or persisted observation; their actual instances remain
// private. The host may compare these values to an authenticated source page.
func (p *SQLHistoricalCASProvenance) Attachments() ([]SQLHistoricalBatchAttachment, error) {
	if !p.intact() {
		return nil, ErrSQLHistoricalCASProvenance
	}
	out := make([]SQLHistoricalBatchAttachment, len(p.plan.attachments))
	for i, v := range p.plan.attachments {
		out[i] = v
		out[i].Entry = v.Entry.Clone()
	}
	return out, nil
}

// Query only the already borrowed, SDK-validated original pool. No new
// connection/transaction or lifecycle method is invoked. ErrTxDone cannot
// distinguish a committed epoch from a rolled-back/aborted epoch.
func sqlHistoricalEndedPool(ctx context.Context, pool gorm.ConnPool) error {
	if ctx == nil || ctx.Err() != nil || historicalNil(pool) {
		return ErrSQLHistoricalCASProvenance
	}
	scope, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	rows, err := pool.QueryContext(scope, "SELECT 1")
	if rows != nil {
		if closeErr := rows.Close(); closeErr != nil {
			return ErrSQLHistoricalCASProvenance
		}
	}
	if scope.Err() != nil || !errors.Is(err, sql.ErrTxDone) {
		return ErrSQLHistoricalCASProvenance
	}
	return nil
}

// Bind during the actual NEW RW epoch, after this exact plan's Apply returns
// and before the host ends that epoch. A later caller cannot reconstruct the
// write pool from a transaction ID or success boolean.
func (p *SQLHistoricalCASProvenance) BindStatement(ctx context.Context, statement *SQLHistoricalBatchCASStatement) (*SQLHistoricalCASStatementBinding, error) {
	if ctx == nil || ctx.Err() != nil || !p.intact() || statement == nil || statement.plan != p.plan || statement.transaction == p.plan.oldTransaction || len(statement.expected.rows) == 0 {
		return nil, ErrSQLHistoricalCASProvenance
	}
	tx, err := historicalTx(ctx)
	if err != nil {
		return nil, err
	}
	if tx.Statement.ConnPool == p.readPool || sqlHistoricalEndedPool(ctx, p.readPool) != nil {
		return nil, ErrSQLHistoricalCASProvenance
	}
	actual, err := casActualRW(tx)
	if err != nil || actual != statement.transaction {
		return nil, ErrSQLHistoricalCASProvenance
	}
	server, database, err := historicalDatabase(tx)
	if err != nil || server != p.plan.server || database != p.plan.database {
		return nil, ErrSQLHistoricalCASProvenance
	}
	expected, err := p.plan.expectedImage(tx)
	if err != nil || !reflect.DeepEqual(expected, statement.expected) {
		return nil, ErrSQLHistoricalCASProvenance
	}
	s := &SQLHistoricalCASStatementBinding{original: p, statement: statement, writePool: tx.Statement.ConnPool}
	s.self, s.seal = s, s.digest()
	if !s.intact() {
		return nil, ErrSQLHistoricalCASProvenance
	}
	return s, nil
}

// This proves the exact expected raw state is visible in an independent actual
// RR-RO epoch, with both earlier SQL epochs ended. It deliberately does not
// certify the host's Commit response: unknown outcome remains a separate gate.
func (s *SQLHistoricalCASStatementBinding) VerifyPersisted(ctx context.Context, fresh *SQLHistoricalOwnerBatch) error {
	if ctx == nil || ctx.Err() != nil || !s.intact() || fresh == nil {
		return ErrSQLHistoricalCASProvenance
	}
	tx, err := historicalTx(ctx)
	if err != nil {
		return err
	}
	if tx.Statement.ConnPool == s.writePool || tx.Statement.ConnPool == s.original.readPool || sqlHistoricalEndedPool(ctx, s.original.readPool) != nil || sqlHistoricalEndedPool(ctx, s.writePool) != nil {
		return ErrSQLHistoricalCASProvenance
	}
	if err = s.statement.VerifyPersisted(ctx, fresh); err != nil {
		return err
	}
	if !s.intact() {
		return ErrSQLHistoricalCASProvenance
	}
	return nil
}

func (s *SQLHistoricalCASStatementBinding) StatementReport() SQLHistoricalBatchCASReport {
	if !s.intact() {
		return (*SQLHistoricalBatchCASStatement)(nil).Report()
	}
	return s.statement.Report()
}
