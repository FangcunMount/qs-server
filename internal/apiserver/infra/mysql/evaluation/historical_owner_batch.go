package evaluation

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"sort"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
)

var (
	ErrSQLHistoricalBatchInvalid  = errors.New("sql_historical_owner_batch_invalid")
	ErrSQLHistoricalBatchBounds   = errors.New("sql_historical_owner_batch_budget_exceeded")
	ErrSQLHistoricalBatchConflict = errors.New("sql_historical_owner_batch_conflict")
)

// The host supplies a bounded owner page from independently authenticated
// sources. These IDs are selectors, never source or business authorization.
type SQLHistoricalOwnerBatchRequest struct {
	AssessmentIDs, AnswerSheetIDs []uint64
}

type SQLHistoricalOwnerBatchLimits struct {
	MaxOwners, MaxRows int
	MaxBytes           uint64
}

func DefaultSQLHistoricalOwnerBatchLimits() SQLHistoricalOwnerBatchLimits {
	return SQLHistoricalOwnerBatchLimits{MaxOwners: 128, MaxRows: 32768, MaxBytes: 64 << 20}
}

func (l SQLHistoricalOwnerBatchLimits) valid() bool {
	return l.MaxOwners > 0 && l.MaxOwners <= 512 && l.MaxRows > 0 && l.MaxRows <= 131072 && l.MaxBytes > 0 && l.MaxBytes <= 256<<20
}

type SQLHistoricalOwnerBatchReport struct {
	Version, DatabaseIdentitySHA256, ResponsibilityCycleID, BusinessRowsSHA256 string
	OwnerCount, AssessmentAbsentCount, AnswerSheetAbsentCount, Rows            uint64
	Bytes                                                                      uint64
	Complete                                                                   bool
	SourceAuthenticationRequired, ExternalBusinessClosureRequired              bool
	WriterFenceRequired, CASRequired, DropReady                                bool
}

// One actual RR-RO transaction owns both this business page and the complete
// responsibility cycle. No point reader, FOR UPDATE, or repeated RM scan is
// used. Raw complete business rows are private recheck baselines, not proofs.
type SQLHistoricalOwnerBatch struct {
	cycle       *SQLHistoricalResponsibilityCycle
	request     SQLHistoricalOwnerBatchRequest
	limits      SQLHistoricalOwnerBatchLimits
	rows        map[string][]historicalSQLRow
	schema      map[string]string
	columns     map[string][]string
	owners      map[uint64]*SQLHistoricalOwnerFacts
	sheets      map[uint64]uint64
	absent      map[uint64]bool
	absentSheet map[uint64]bool
	// Only actual global reads in this batch's RR-RO epoch populate this map.
	// It is independent of an editable Snapshot and of owner-filtered Run rows.
	originalOutcomeRunAbsence map[uint64]string
	report                    SQLHistoricalOwnerBatchReport
}

// This capability intentionally does not return the legacy concrete facts
// type. Its old AnswerSheet Recheck can execute FOR UPDATE even for an empty
// event ID. Only batch.RecheckBusiness plus cycle.RecheckFresh is supported.
type SQLHistoricalBatchOwnerFacts struct {
	batch *SQLHistoricalOwnerBatch
	id    uint64
	sheet uint64
}

func (*SQLHistoricalOwnerBatch) MarshalJSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalOwnerBatch) MarshalBSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalOwnerBatch) String() string     { return "private bounded SQL business owner page" }
func (b *SQLHistoricalOwnerBatch) GoString() string { return b.String() }
func (*SQLHistoricalBatchOwnerFacts) MarshalJSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalBatchOwnerFacts) MarshalBSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalBatchOwnerFacts) String() string     { return "private SQL batch owner facts" }
func (f *SQLHistoricalBatchOwnerFacts) GoString() string { return f.String() }

func batchNormalizeIDs(ids []uint64) ([]uint64, error) {
	copy := append([]uint64(nil), ids...)
	sort.Slice(copy, func(i, j int) bool { return copy[i] < copy[j] })
	for i, id := range copy {
		if id == 0 || i > 0 && copy[i-1] == id {
			return nil, ErrSQLHistoricalBatchInvalid
		}
	}
	return copy, nil
}

func PrepareSQLHistoricalOwnerBatch(ctx context.Context, cycle *SQLHistoricalResponsibilityCycle, request SQLHistoricalOwnerBatchRequest, limits SQLHistoricalOwnerBatchLimits) (*SQLHistoricalOwnerBatch, error) {
	if cycle == nil || cycle.report.CompletedAt.IsZero() || !limits.valid() || len(request.AssessmentIDs)+len(request.AnswerSheetIDs) == 0 || len(request.AssessmentIDs)+len(request.AnswerSheetIDs) > limits.MaxOwners {
		return nil, ErrSQLHistoricalBatchInvalid
	}
	if err := cycle.ValidateBorrowedSnapshot(ctx); err != nil {
		return nil, err
	}
	assessmentIDs, err := batchNormalizeIDs(request.AssessmentIDs)
	if err != nil {
		return nil, err
	}
	sheetIDs, err := batchNormalizeIDs(request.AnswerSheetIDs)
	if err != nil {
		return nil, err
	}
	b := &SQLHistoricalOwnerBatch{cycle: cycle, request: SQLHistoricalOwnerBatchRequest{AssessmentIDs: assessmentIDs, AnswerSheetIDs: sheetIDs}, limits: limits, rows: map[string][]historicalSQLRow{}, schema: map[string]string{}, columns: map[string][]string{}, owners: map[uint64]*SQLHistoricalOwnerFacts{}, sheets: map[uint64]uint64{}, absent: map[uint64]bool{}, absentSheet: map[uint64]bool{}}
	b.report = SQLHistoricalOwnerBatchReport{Version: "sql-business-owner-batch/v1", DatabaseIdentitySHA256: cycle.report.DatabaseIdentitySHA256, ResponsibilityCycleID: cycle.report.CycleID, SourceAuthenticationRequired: true, ExternalBusinessClosureRequired: true, WriterFenceRequired: true, CASRequired: true}
	if err = b.capture(ctx); err != nil {
		return nil, err
	}
	if err = b.captureOriginalOutcomeRunAbsence(ctx); err != nil {
		return nil, err
	}
	b.report.Complete = true
	return b, nil
}

func (b *SQLHistoricalOwnerBatch) Report() SQLHistoricalOwnerBatchReport {
	if b == nil {
		return SQLHistoricalOwnerBatchReport{SourceAuthenticationRequired: true, ExternalBusinessClosureRequired: true, WriterFenceRequired: true, CASRequired: true}
	}
	r := b.report
	r.SourceAuthenticationRequired, r.ExternalBusinessClosureRequired, r.WriterFenceRequired, r.CASRequired = true, true, true, true
	r.DropReady = false
	return r
}

func (b *SQLHistoricalOwnerBatch) OwnerByAssessment(id uint64) (*SQLHistoricalBatchOwnerFacts, error) {
	if b == nil || !b.report.Complete || id == 0 {
		return nil, ErrSQLHistoricalBatchInvalid
	}
	if b.owners[id] == nil {
		if b.absent[id] {
			return nil, ErrSQLHistoricalOwnerAbsent
		}
		return nil, ErrSQLHistoricalBatchInvalid
	}
	return &SQLHistoricalBatchOwnerFacts{batch: b, id: id}, nil
}

func (b *SQLHistoricalOwnerBatch) OwnerByAnswerSheet(id uint64) (*SQLHistoricalBatchOwnerFacts, error) {
	if b == nil || !b.report.Complete || id == 0 || !slices.Contains(b.request.AnswerSheetIDs, id) {
		return nil, ErrSQLHistoricalBatchInvalid
	}
	owner := b.sheets[id]
	if owner == 0 {
		if b.absentSheet[id] {
			return nil, ErrSQLHistoricalOwnerAbsent
		}
		return nil, ErrSQLHistoricalBatchInvalid
	}
	return &SQLHistoricalBatchOwnerFacts{batch: b, id: owner, sheet: id}, nil
}

func (f *SQLHistoricalBatchOwnerFacts) Snapshot() SQLHistoricalFactsSnapshot {
	if f == nil || f.batch == nil || !f.batch.report.Complete || f.batch.owners[f.id] == nil {
		return SQLHistoricalFactsSnapshot{}
	}
	return f.batch.owners[f.id].Snapshot()
}

func (f *SQLHistoricalBatchOwnerFacts) OutcomeRecord(id uint64) (*evaluationfact.Record, error) {
	if f == nil || f.batch == nil || !f.batch.report.Complete || f.batch.owners[f.id] == nil {
		return nil, ErrSQLHistoricalBatchInvalid
	}
	return f.batch.owners[f.id].OutcomeRecord(id)
}

func (f *SQLHistoricalBatchOwnerFacts) HasVerifiedAnswerSheetAssociation(id uint64) bool {
	return f != nil && f.batch != nil && f.batch.report.Complete && f.sheet != 0 && f.sheet == id && f.batch.sheets[id] == f.id && f.Snapshot().Owner.AnswerSheetID == id
}

func (b *SQLHistoricalOwnerBatch) ValidateBorrowedSnapshot(ctx context.Context) error {
	if b == nil || b.cycle == nil {
		return ErrSQLHistoricalBatchInvalid
	}
	return b.cycle.ValidateBorrowedSnapshot(ctx)
}

// OriginalOutcomeRunAbsent observes only the exact declared original Run of
// this opaque owner's validated Outcome. It proves neither terminal closure
// nor source authentication and never opens or ends a host transaction.
func (f *SQLHistoricalBatchOwnerFacts) OriginalOutcomeRunAbsent(ctx context.Context, outcomeID uint64, originalRunID string) error {
	if ctx == nil || ctx.Err() != nil || f == nil || f.batch == nil || !f.batch.report.Complete || f.batch.originalOutcomeRunAbsence == nil || outcomeID == 0 || originalRunID == "" {
		return ErrSQLHistoricalBatchInvalid
	}
	if err := f.batch.ValidateBorrowedSnapshot(ctx); err != nil {
		return err
	}
	record, err := f.OutcomeRecord(outcomeID)
	if err != nil || record == nil || record.RunID() != originalRunID || f.batch.originalOutcomeRunAbsence[outcomeID] != record.RunID() {
		return ErrSQLHistoricalBatchConflict
	}
	return nil
}

// Selection only; no map produced here is an absence capability. In
// particular, any selected scope/deleted row preserves the Run's presence.
func batchOriginalRunCandidates(rows map[string][]historicalSQLRow, maxOwners int) (map[uint64]string, error) {
	if maxOwners <= 0 || maxOwners > 512 {
		return nil, ErrSQLHistoricalBatchBounds
	}
	present := map[string]bool{}
	for _, row := range rows["runtime_checkpoint"] {
		present[valueOrEmpty(row["resource_id"])] = true
	}
	candidates := map[uint64]string{}
	seen := map[uint64]bool{}
	for _, row := range rows["evaluation_outcome"] {
		id, err := sqlHistoricalUint(row, "id")
		runID := valueOrEmpty(row["evaluation_run_id"])
		if err != nil || id == 0 || seen[id] || runID == "" {
			return nil, ErrSQLHistoricalBatchConflict
		}
		seen[id] = true
		if len(seen) > maxOwners {
			return nil, ErrSQLHistoricalBatchBounds
		}
		if !present[runID] {
			candidates[id] = runID
		}
	}
	return candidates, nil
}

// One bounded global existence read per owner batch, with exact binary IDs.
// No organization/scope/soft-delete predicate can hide a contradictory row.
// DISTINCT is safe here: presence of any physical row forbids the gap; row
// identity/content is still verified by the separate original-business path.
func (b *SQLHistoricalOwnerBatch) captureOriginalOutcomeRunAbsence(ctx context.Context) error {
	candidates, err := batchOriginalRunCandidates(b.rows, b.limits.MaxOwners)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		b.originalOutcomeRunAbsence = candidates
		return nil
	}
	if err = b.ValidateBorrowedSnapshot(ctx); err != nil {
		return err
	}
	tx, err := historicalTx(ctx)
	if err != nil {
		return err
	}
	server, database, err := historicalDatabase(tx)
	if err != nil || sqlHistoricalIdentity(server, database) != b.report.DatabaseIdentitySHA256 {
		return ErrSQLHistoricalFactsIdentity
	}
	columns, schema, _, err := cycleSchema(tx, sqlResponsibilityTable{name: "runtime_checkpoint", keys: []string{"id"}})
	if err != nil || schema != b.schema["runtime_checkpoint"] || !reflect.DeepEqual(columns, b.columns["runtime_checkpoint"]) {
		return ErrSQLHistoricalBatchConflict
	}
	unique := map[string]bool{}
	for _, runID := range candidates {
		unique[runID] = true
	}
	ids := make([]string, 0, len(unique))
	for id := range unique {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	observed, names, size, err := cycleQuery(tx, "SELECT DISTINCT CAST(resource_id AS BINARY) AS resource_id FROM runtime_checkpoint WHERE CAST(resource_id AS BINARY) IN ? ORDER BY resource_id LIMIT ?", len(ids), ids, len(ids)+1)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(names, []string{"resource_id"}) || b.report.Rows > uint64(b.limits.MaxRows) || uint64(len(observed)) > uint64(b.limits.MaxRows)-b.report.Rows || b.report.Bytes > b.limits.MaxBytes || size > b.limits.MaxBytes-b.report.Bytes {
		return ErrSQLHistoricalBatchBounds
	}
	for _, row := range observed {
		id := valueOrEmpty(row["resource_id"])
		if !unique[id] {
			return ErrSQLHistoricalBatchConflict
		}
		for outcome, runID := range candidates {
			if runID == id {
				delete(candidates, outcome)
			}
		}
	}
	_, after, _, err := cycleSchema(tx, sqlResponsibilityTable{name: "runtime_checkpoint", keys: []string{"id"}})
	if err != nil || after != schema {
		return ErrSQLHistoricalBatchConflict
	}
	if err = b.ValidateBorrowedSnapshot(ctx); err != nil {
		return err
	}
	b.originalOutcomeRunAbsence = candidates
	return nil
}

// The coordinator prepares one fresh complete responsibility cycle and reuses
// it for every bounded owner page. This does not rescan any responsibility
// ledger, authenticate original source rows, create a CAS, or fence writers.
func (b *SQLHistoricalOwnerBatch) RecheckBusiness(ctx context.Context, freshCycle *SQLHistoricalResponsibilityCycle) error {
	if b == nil || b.cycle == nil || freshCycle == nil || b.cycle.report.DatabaseIdentitySHA256 != freshCycle.report.DatabaseIdentitySHA256 || b.cycle.transaction == freshCycle.transaction {
		return ErrSQLHistoricalBatchInvalid
	}
	current, err := PrepareSQLHistoricalOwnerBatch(ctx, freshCycle, b.request, b.limits)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(b.rows, current.rows) || !reflect.DeepEqual(b.schema, current.schema) || !reflect.DeepEqual(b.columns, current.columns) || !reflect.DeepEqual(b.sheets, current.sheets) || !reflect.DeepEqual(b.absent, current.absent) || !reflect.DeepEqual(b.absentSheet, current.absentSheet) || !reflect.DeepEqual(b.originalOutcomeRunAbsence, current.originalOutcomeRunAbsence) {
		return ErrSQLHistoricalBatchConflict
	}
	return nil
}
