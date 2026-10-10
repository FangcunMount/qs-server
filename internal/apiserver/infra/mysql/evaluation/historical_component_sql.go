package evaluation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	domainwire "github.com/FangcunMount/reliable-messaging/wire/domain"
	"gorm.io/gorm"
)

var ErrSQLHistoricalComponent = errors.New("sql_historical_component_rejected")

// A recipe is a copy of actual original reads, never a renewed qualification.
// It can only be produced while the original native owner/page is still alive.
type SQLHistoricalComponentRecipe struct {
	self             *SQLHistoricalComponentRecipe
	plan             *SQLHistoricalBatchCASPlan
	selectors        SQLCrossStoreSelectors
	responsibility   sqlHistoricalCASImage
	anchors          map[string]string
	limits           SQLCrossStoreLimits
	oldPool          gorm.ConnPool
	seal             string
	input            *SQLHistoricalCASFrozenInput
	spool            *SQLHistoricalCASSpool
	frame            sqlSpoolRef
	inlineSeal       string
	ownerPartitioned bool
}

// The host owns every transaction and its commit. This observer grants only a
// short-lived SQL component read, not source, Mongo, AI or retirement authority.
type SQLHistoricalComponentObservation struct {
	applyMu        sync.Mutex
	self           *SQLHistoricalComponentObservation
	recipe         *SQLHistoricalComponentRecipe
	pool           gorm.ConnPool
	transaction    sqlResponsibilityTransaction
	expires        time.Time
	writable       bool
	used           bool
	seal           string
	business       sqlHistoricalCASImage
	responsibility sqlHistoricalCASImage
	observations   []SQLResponsibilityObservation
	semantic       *SQLHistoricalComponentSemanticView
}

type SQLHistoricalComponentStatement struct {
	self         *SQLHistoricalComponentStatement
	observation  *SQLHistoricalComponentObservation
	observations []*SQLHistoricalComponentObservation
	plan         *SQLHistoricalBatchCASPlan
	statement    *SQLHistoricalBatchCASStatement
	seal         string
}

type SQLHistoricalComponentReadReport struct {
	BusinessMatched, ResponsibilitiesMatched, IndependentPersistedReadMatched              bool
	FullSourcesRequired, MongoQualificationRequired, AIClosureRequired, HostCommitRequired bool
	HostCommitVerified, WholeRetirementComplete, DropReady                                 bool
}

func (*SQLHistoricalComponentRecipe) MarshalJSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalComponentRecipe) MarshalBSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalComponentRecipe) UnmarshalJSON([]byte) error {
	return ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalComponentRecipe) UnmarshalBSON([]byte) error {
	return ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalComponentObservation) MarshalJSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalComponentObservation) MarshalBSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalComponentStatement) MarshalJSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalComponentStatement) MarshalBSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}

func componentCopySelectors(v SQLCrossStoreSelectors) SQLCrossStoreSelectors {
	return SQLCrossStoreSelectors{EventIDs: slices.Clone(v.EventIDs), AssessmentIDs: slices.Clone(v.AssessmentIDs), OrganizationIDs: slices.Clone(v.OrganizationIDs), MongoOwners: slices.Clone(v.MongoOwners)}
}

func (r *SQLHistoricalComponentRecipe) digest() string {
	if r != nil && r.spool != nil {
		if r.plan != nil || r.input != nil || r.inlineSeal == "" || r.frame.SHA256 == "" || r.spool.valid(context.Background()) != nil {
			return ""
		}
		return cycleKeyDigest([]string{"sql-component-spooled-input/v1", r.inlineSeal, r.frame.SHA256, strconv.FormatInt(r.frame.Offset, 10), strconv.FormatInt(r.frame.Length, 10), sqlHistoricalProvenancePoolToken(r.oldPool)})
	}
	if r == nil || r.plan == nil || r.input == nil || r.input.self != r.input || r.input.seal == "" || r.input.seal != r.input.digest() {
		return ""
	}
	parts := []string{strconv.FormatBool(r.ownerPartitioned), "sql-historical-component-input/v1", r.input.seal, r.plan.identity, casImageHash(r.plan.before), casImageHash(r.responsibility), sqlHistoricalProvenancePoolToken(r.oldPool)}
	parts = append(parts, r.selectors.EventIDs...)
	for _, id := range r.selectors.AssessmentIDs {
		parts = append(parts, "assessment:"+strconv.FormatUint(id, 10))
	}
	for _, id := range r.selectors.OrganizationIDs {
		parts = append(parts, "organization:"+strconv.FormatUint(id, 10))
	}
	for _, owner := range r.selectors.MongoOwners {
		parts = append(parts, owner.Kind+":"+owner.ID)
	}
	keys := make([]string, 0, len(r.anchors))
	for key := range r.anchors {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		parts = append(parts, key, r.anchors[key])
	}
	// This private codec also freezes attachments, groups, negative selectors,
	// original native transaction and limits. No exported import is provided.
	record := componentPlanRecord(r.plan)
	raw, err := json.Marshal(record)
	if err != nil {
		return ""
	}
	parts = append(parts, sqlSpoolSHA(raw), strconv.Itoa(r.limits.MaxPageRows), strconv.FormatUint(r.limits.MaxPageBytes, 10), r.limits.MaxDuration.String())
	return cycleKeyDigest(parts)
}
func (r *SQLHistoricalComponentRecipe) intact() bool {
	return r != nil && r.self == r && r.seal != "" && r.seal == r.digest()
}

// Input hashes/dependencies describe frozen reads and writes, never permission.
func (r *SQLHistoricalComponentRecipe) InputSHA256() (string, error) {
	if !r.intact() {
		return "", ErrSQLHistoricalComponent
	}
	if _, err := r.load(context.Background()); err != nil {
		return "", err
	}
	return r.seal, nil
}
func (r *SQLHistoricalComponentRecipe) RowDependencies(visit func(string, uint64, string, uint64, bool) error) error {
	if !r.intact() {
		return ErrSQLHistoricalComponent
	}
	loaded, err := r.load(context.Background())
	if err != nil {
		return err
	}
	return loaded.input.RowDependencies(visit)
}

func componentPlanRecord(p *SQLHistoricalBatchCASPlan) sqlSpoolPlan {
	r := sqlSpoolPlan{Version: 1, Identity: p.identity, Server: p.server, Database: p.database, Old: [3]uint64{p.oldTransaction.connection, p.oldTransaction.thread, p.oldTransaction.event}, Request: p.request, Limits: p.limits, Before: sqlSpoolImageOut(p.before), Attachments: p.attachments, Missing: p.missingOriginalRunIDs}
	for _, g := range p.groups {
		r.Groups = append(r.Groups, sqlSpoolGroup{g.table, g.column, g.id, g.entries, g.set})
	}
	return r
}

func FreezeSQLHistoricalComponentRecipe(ctx context.Context, original *SQLHistoricalOwnerBatch, cross *SQLHistoricalCrossStorePage, provenance *SQLHistoricalCASProvenance, unchanged *SQLHistoricalCASReadBaseline) (*SQLHistoricalComponentRecipe, error) {
	if ctx == nil || ctx.Err() != nil || original == nil || cross == nil || cross.batch != original || !cross.report.Complete || cross.catalog == nil || cross.catalog.cycle != original.cycle || len(cross.selectors.EventIDs) == 0 || (provenance == nil) == (unchanged == nil) || original.ValidateBorrowedSnapshot(ctx) != nil {
		return nil, ErrSQLHistoricalComponent
	}
	input, err := FreezeSQLHistoricalCASInput(ctx, original, provenance, unchanged)
	if err != nil {
		return nil, err
	}
	tx, err := historicalTx(ctx)
	if err != nil {
		return nil, err
	}
	server, database, err := historicalDatabase(tx)
	if err != nil {
		return nil, err
	}
	var plan *SQLHistoricalBatchCASPlan
	if provenance != nil {
		plan = provenance.plan
	} else {
		plan = &SQLHistoricalBatchCASPlan{identity: unchanged.identity, server: server, database: database, oldTransaction: original.cycle.transaction, request: original.request, limits: original.limits, before: unchanged.before}
		for _, id := range original.originalOutcomeRunAbsence {
			plan.missingOriginalRunIDs = append(plan.missingOriginalRunIDs, id)
		}
		sort.Strings(plan.missingOriginalRunIDs)
		plan.missingOriginalRunIDs = slices.Compact(plan.missingOriginalRunIDs)
	}
	// Deep-copy through the existing private NULL/binary-preserving codec.
	raw, err := sqlSpoolEncode(componentPlanRecord(plan))
	if err != nil {
		return nil, err
	}
	var record sqlSpoolPlan
	if err = sqlSpoolDecode(raw, &record); err != nil {
		return nil, err
	}
	plan, err = sqlSpoolReconstruct(record, sqlSpoolImageIn(record.Before))
	if err != nil {
		return nil, err
	}
	r := &SQLHistoricalComponentRecipe{plan: plan, selectors: componentCopySelectors(cross.selectors), limits: cross.catalog.limits, oldPool: tx.Statement.ConnPool, input: input}
	r.responsibility, r.anchors, err = r.captureOriginalResponsibility(ctx, cross.catalog)
	if err != nil {
		return nil, err
	}
	// captureOriginalResponsibility uses the genuine catalog's key index and
	// page.read verifies every raw row against that original observation. Do
	// not rebuild a whole-ledger key/hash map for each component.
	if original.ValidateBorrowedSnapshot(ctx) != nil {
		return nil, ErrSQLHistoricalComponent
	}
	r.self = r
	r.seal = r.digest()
	if !r.intact() {
		return nil, ErrSQLHistoricalComponent
	}
	return r, nil
}

// This reuses the host's existing private FD spool. The recipe retains only a
// bounded record reference; loading one component never renews its old write
// qualification or imports a caller-supplied image. The host owns the file.
func FreezeSQLHistoricalComponentRecipeToSpool(ctx context.Context, original *SQLHistoricalOwnerBatch, cross *SQLHistoricalCrossStorePage, provenance *SQLHistoricalCASProvenance, unchanged *SQLHistoricalCASReadBaseline, spool *SQLHistoricalCASSpool) (*SQLHistoricalComponentRecipe, error) {
	if spool == nil {
		return nil, ErrSQLHistoricalComponent
	}
	r, err := FreezeSQLHistoricalComponentRecipe(ctx, original, cross, provenance, unchanged)
	if err != nil {
		return nil, err
	}
	return spoolSQLHistoricalComponentRecipe(ctx, original, r, spool)
}

func spoolSQLHistoricalComponentRecipe(ctx context.Context, original *SQLHistoricalOwnerBatch, r *SQLHistoricalComponentRecipe, spool *SQLHistoricalCASSpool) (*SQLHistoricalComponentRecipe, error) {
	spool.mu.Lock()
	defer spool.mu.Unlock()
	if spool.valid(ctx) != nil || spool.sealed || spool.oldPool != nil || spool.writePool != nil || len(spool.frames) != 0 || spool.applied != 0 {
		return nil, ErrSQLHistoricalComponent
	}
	record := sqlComponentSpoolRecord{Plan: componentPlanRecord(r.plan), Input: sqlSpoolImageOut(r.input.before), Writes: r.input.writes, Responsibility: sqlSpoolImageOut(r.responsibility), Anchors: r.anchors, Selectors: r.selectors, Limits: r.limits, OwnerPartitioned: r.ownerPartitioned}
	ref, err := spool.put(ctx, record)
	if err != nil || original.ValidateBorrowedSnapshot(ctx) != nil {
		return nil, ErrSQLHistoricalComponent
	}
	r.inlineSeal, r.frame, r.spool = r.seal, ref, spool
	r.plan, r.input, r.anchors = nil, nil, nil
	r.responsibility = sqlHistoricalCASImage{}
	r.seal = r.digest()
	if !r.intact() {
		return nil, ErrSQLHistoricalComponent
	}
	return r, nil
}

type sqlComponentSpoolRecord struct {
	Plan             sqlSpoolPlan
	Input            sqlSpoolImage
	Writes           map[string]bool
	Responsibility   sqlSpoolImage
	Anchors          map[string]string
	Selectors        SQLCrossStoreSelectors
	Limits           SQLCrossStoreLimits
	OwnerPartitioned bool
}

func (r *SQLHistoricalComponentRecipe) load(ctx context.Context) (*SQLHistoricalComponentRecipe, error) {
	if ctx == nil || ctx.Err() != nil || !r.intact() {
		return nil, ErrSQLHistoricalComponent
	}
	if r.spool == nil {
		return r, nil
	}
	var record sqlComponentSpoolRecord
	r.spool.mu.Lock()
	err := r.spool.get(ctx, r.frame, &record)
	r.spool.mu.Unlock()
	if err != nil {
		return nil, ErrSQLHistoricalComponent
	}
	p, err := sqlSpoolReconstruct(record.Plan, sqlSpoolImageIn(record.Plan.Before))
	if err != nil {
		return nil, ErrSQLHistoricalComponent
	}
	f := &SQLHistoricalCASFrozenInput{before: sqlSpoolImageIn(record.Input), writes: record.Writes}
	f.self, f.seal = f, f.digest()
	loaded := &SQLHistoricalComponentRecipe{plan: p, input: f, responsibility: sqlSpoolImageIn(record.Responsibility), anchors: record.Anchors, selectors: record.Selectors, limits: record.Limits, oldPool: r.oldPool, ownerPartitioned: record.OwnerPartitioned}
	loaded.self, loaded.seal = loaded, r.inlineSeal
	if !loaded.intact() || !r.intact() || ctx.Err() != nil {
		return nil, ErrSQLHistoricalComponent
	}
	return loaded, nil
}

// These identities describe actual original owner rows only. They grant no
// source authentication, fresh transaction or write authority.
type SQLHistoricalComponentOwnerIdentity struct {
	AssessmentID, OrganizationID, AnswerSheetID uint64
}

func (r *SQLHistoricalComponentRecipe) OwnerIdentities() ([]SQLHistoricalComponentOwnerIdentity, error) {
	loaded, err := r.load(context.Background())
	if err != nil {
		return nil, err
	}
	var out []SQLHistoricalComponentOwnerIdentity
	for _, row := range loaded.plan.before.rows["assessment"] {
		id, e := sqlHistoricalUint(row, "id")
		org, oe := sqlHistoricalUint(row, "org_id")
		sheet, se := sqlHistoricalUint(row, "answer_sheet_id")
		if e != nil || oe != nil || se != nil {
			return nil, ErrSQLHistoricalComponent
		}
		out = append(out, SQLHistoricalComponentOwnerIdentity{id, org, sheet})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AssessmentID < out[j].AssessmentID })
	return out, nil
}

func (r *SQLHistoricalComponentRecipe) SourceEventIDs() ([]string, error) {
	loaded, err := r.load(context.Background())
	if err != nil {
		return nil, err
	}
	return slices.Clone(loaded.selectors.EventIDs), nil
}

type SQLHistoricalSourceOwnerBinding struct {
	EventID, MongoOwnerKind, MongoOwnerID string
	AssessmentID                          uint64
	SQLOwnerPresent                       bool
}

// An empty owner is an explicit unresolved pure input, never proof of absence
// of a Mongo business owner. Only exact captured attachment/message identities
// populate these values; SourceEventIDs still includes every original source.
func (r *SQLHistoricalComponentRecipe) SourceOwnerBindings() ([]SQLHistoricalSourceOwnerBinding, error) {
	loaded, err := r.load(context.Background())
	if err != nil {
		return nil, err
	}
	owners := map[uint64]bool{}
	sheets := map[string]uint64{}
	for _, row := range loaded.plan.before.rows["assessment"] {
		id, e := sqlHistoricalUint(row, "id")
		if e != nil {
			return nil, e
		}
		owners[id] = true
		sheets[valueOrEmpty(row["answer_sheet_id"])] = id
	}
	var out []SQLHistoricalSourceOwnerBinding
	for _, event := range loaded.selectors.EventIDs {
		b := SQLHistoricalSourceOwnerBinding{EventID: event}
		for _, a := range loaded.plan.attachments {
			if a.Entry.EventID == event {
				if b.AssessmentID != 0 && b.AssessmentID != a.AssessmentID {
					return nil, ErrSQLHistoricalComponent
				}
				b.AssessmentID = a.AssessmentID
			}
		}
		for table, rows := range loaded.responsibility.rows {
			for _, row := range rows {
				v := cycleDecode(table, row)
				if v.EventID != event {
					continue
				}
				if v.AssessmentID != 0 {
					if b.AssessmentID != 0 && b.AssessmentID != v.AssessmentID {
						return nil, ErrSQLHistoricalComponent
					}
					b.AssessmentID = v.AssessmentID
				}
				if v.OwnerKind == "AnswerSheet" || v.OwnerKind == "ReportGeneration" {
					if b.MongoOwnerKind != "" && (b.MongoOwnerKind != v.OwnerKind || b.MongoOwnerID != v.OwnerID) {
						return nil, ErrSQLHistoricalComponent
					}
					b.MongoOwnerKind, b.MongoOwnerID = v.OwnerKind, v.OwnerID
					if v.OwnerKind == "AnswerSheet" && sheets[v.OwnerID] != 0 {
						id := sheets[v.OwnerID]
						if b.AssessmentID != 0 && b.AssessmentID != id {
							return nil, ErrSQLHistoricalComponent
						}
						b.AssessmentID = id
					}
				}
			}
		}
		b.SQLOwnerPresent = owners[b.AssessmentID]
		out = append(out, b)
	}
	return out, nil
}

// OriginalSelectors returns the exact frozen negative ranges as pure metadata.
// A Mongo-only source can retain its AnswerSheet/Generation range even when
// these SQL reads contain no assessment row. It is never a qualification.
func (r *SQLHistoricalComponentRecipe) OriginalSelectors() (SQLCrossStoreSelectors, error) {
	loaded, err := r.load(context.Background())
	if err != nil {
		return SQLCrossStoreSelectors{}, err
	}
	return componentCopySelectors(loaded.selectors), nil
}

// True describes only a complete captured positive or SQL-negative partition.
// False retains the whole unresolved original input, never a guessed owner or
// silently dropped range. Neither value authenticates a source/Mongo owner.
func (r *SQLHistoricalComponentRecipe) OwnerPartitionResolved() bool {
	loaded, err := r.load(context.Background())
	return err == nil && loaded.ownerPartitioned
}

// Freeze planning input in the SAME actual RRRO scope as the original owner
// batch and catalog. Every baseline/negative range is read from that scope;
// no coordinator, global capability, imported evidence or CAS plan is made.
// The recipes have no groups or write edges. Actual owner/replay dependencies
// still join components; a later fresh qualified aggregate must prepare writes.
func FreezeSQLHistoricalOwnerPlanningRecipes(ctx context.Context, original *SQLHistoricalOwnerBatch, catalog *SQLHistoricalCrossStoreCatalog, selectors SQLCrossStoreSelectors, spool *SQLHistoricalCASSpool, sourceSelectors ...map[string]uint64) ([]*SQLHistoricalComponentRecipe, error) {
	page, err := PrepareSQLHistoricalCrossStorePage(ctx, catalog, original, selectors)
	if err != nil {
		return nil, err
	}
	baseline, err := SealSQLHistoricalCASReadBaseline(ctx, original)
	if err != nil {
		return nil, err
	}
	return FreezeSQLHistoricalOwnerComponentRecipes(ctx, original, page, nil, baseline, spool, sourceSelectors...)
}

// The source page is a transport boundary, not an atomic owner boundary.
// Partition only private, live original reads. Replay membership joins real
// owners; shared schema/model/head reads do not join unrelated assessments.
// The first optional map selects actual assessment owners; the second selects
// actual SQL-absent answer sheets. Both are pure joint-reader partition input.
// Every event and owner must already belong to these actual original reads. It
// authenticates no source and cannot grant observation or statement authority.
func FreezeSQLHistoricalOwnerComponentRecipes(ctx context.Context, original *SQLHistoricalOwnerBatch, cross *SQLHistoricalCrossStorePage, provenance *SQLHistoricalCASProvenance, unchanged *SQLHistoricalCASReadBaseline, spool *SQLHistoricalCASSpool, sourceSelectors ...map[string]uint64) ([]*SQLHistoricalComponentRecipe, error) {
	parent, err := FreezeSQLHistoricalComponentRecipe(ctx, original, cross, provenance, unchanged)
	if err != nil {
		return nil, fmt.Errorf("%w: owner_parent: %w", ErrSQLHistoricalComponent, err)
	}
	if len(sourceSelectors) > 2 {
		return nil, ErrSQLHistoricalComponent
	}
	present, absent := map[string]uint64{}, map[string]uint64{}
	if len(sourceSelectors) > 0 {
		for event, id := range sourceSelectors[0] {
			if !slices.Contains(cross.selectors.EventIDs, event) {
				return nil, ErrSQLHistoricalComponent
			}
			if _, err = original.OwnerByAssessment(id); err != nil {
				return nil, ErrSQLHistoricalComponent
			}
			present[event] = id
		}
	}
	if len(sourceSelectors) > 1 {
		for event, sheet := range sourceSelectors[1] {
			if present[event] != 0 || !slices.Contains(cross.selectors.EventIDs, event) || !slices.Contains(original.request.AnswerSheetIDs, sheet) || !slices.Contains(cross.selectors.MongoOwners, SQLCrossStoreOwnerReference{Kind: "AnswerSheet", ID: strconv.FormatUint(sheet, 10)}) {
				return nil, ErrSQLHistoricalComponent
			}
			if _, ownerErr := original.OwnerByAnswerSheet(sheet); !errors.Is(ownerErr, ErrSQLHistoricalOwnerAbsent) {
				return nil, ErrSQLHistoricalComponent
			}
			for _, attachment := range parent.plan.attachments {
				if attachment.Entry.EventID == event {
					return nil, ErrSQLHistoricalComponent
				}
			}
			for _, i := range cross.catalog.cycle.byEvent[event] {
				v := cross.catalog.cycle.observations[i]
				if v.AssessmentID != 0 || v.OwnerKind != "" && (v.OwnerKind != "AnswerSheet" || v.OwnerID != strconv.FormatUint(sheet, 10)) {
					return nil, ErrSQLHistoricalComponent
				}
			}
			absent[event] = sheet
		}
	}
	partitionParent := parent
	var ownerArgs []map[string]uint64
	if len(sourceSelectors) > 0 {
		ownerArgs = []map[string]uint64{present}
	}
	mixedClosed := true
	if len(absent) > 0 {
		partitionParent, mixedClosed = componentOriginalMixedParent(parent, cross.catalog, absent)
	}
	var parts []sqlOriginalOwnerPart
	if mixedClosed {
		parts, err = componentOriginalOwnerPartitions(partitionParent, cross.catalog, ownerArgs...)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: owner_partition", ErrSQLHistoricalComponent)
	}
	if !mixedClosed || parts == nil && len(partitionParent.selectors.EventIDs) != 0 {
		if spool != nil {
			parent, err = spoolSQLHistoricalComponentRecipe(ctx, original, parent, spool)
		}
		if err != nil {
			return nil, err
		}
		return []*SQLHistoricalComponentRecipe{parent}, nil
	}
	out := make([]*SQLHistoricalComponentRecipe, 0, len(parts))
	for _, part := range parts {
		child, err := componentOriginalOwnerChild(partitionParent, part)
		if err != nil {
			return nil, fmt.Errorf("%w: owner_child: %w", ErrSQLHistoricalComponent, err)
		}
		child.responsibility, child.anchors, err = child.captureOriginalResponsibility(ctx, cross.catalog)
		if err != nil || original.ValidateBorrowedSnapshot(ctx) != nil {
			return nil, fmt.Errorf("%w: owner_responsibility: %v", ErrSQLHistoricalComponent, err)
		}
		child.self, child.seal = child, child.digest()
		if !child.intact() {
			return nil, fmt.Errorf("%w: owner_seal", ErrSQLHistoricalComponent)
		}
		out = append(out, child)
	}
	absentGroups := map[uint64][]string{}
	for event, sheet := range absent {
		absentGroups[sheet] = append(absentGroups[sheet], event)
	}
	sheets := make([]uint64, 0, len(absentGroups))
	for sheet := range absentGroups {
		sheets = append(sheets, sheet)
	}
	slices.Sort(sheets)
	for _, sheet := range sheets {
		events := absentGroups[sheet]
		slices.Sort(events)
		child, e := FreezeSQLHistoricalAbsentOwnerSelectorRecipe(ctx, original, cross, events, []SQLCrossStoreOwnerReference{{Kind: "AnswerSheet", ID: strconv.FormatUint(sheet, 10)}}, []uint64{sheet}, nil)
		if e != nil {
			return nil, e
		}
		out = append(out, child)
	}
	// All owner closures and resource bounds are checked before any child is
	// returned. Persisting these pure inputs never runs a statement or commit.
	if spool != nil {
		for i, child := range out {
			out[i], err = spoolSQLHistoricalComponentRecipe(ctx, original, child, spool)
			if err != nil {
				return nil, fmt.Errorf("%w: owner_spool", ErrSQLHistoricalComponent)
			}
		}
	}
	return out, nil
}

// Preserve the full parent when an actual replay crosses empty/present owners,
// different empty sheets or an original source that was not captured here.
// Otherwise only remove those exact empty selectors from the present child
// basis. The original immutable Before/Groups remain genuine captured values.
func componentOriginalMixedParent(parent *SQLHistoricalComponentRecipe, catalog *SQLHistoricalCrossStoreCatalog, absent map[string]uint64) (*SQLHistoricalComponentRecipe, bool) {
	actualOwners := map[uint64]bool{}
	for _, row := range parent.plan.before.rows["assessment"] {
		id, err := sqlHistoricalUint(row, "id")
		if err != nil {
			return parent, false
		}
		actualOwners[id] = true
	}
	for _, id := range parent.plan.request.AssessmentIDs {
		if !actualOwners[id] {
			return parent, false
		}
	}
	for _, row := range parent.responsibility.rows["qs_rm_replay_items"] {
		v := cycleDecode("qs_rm_replay_items", row)
		var group uint64
		var set bool
		for _, i := range catalog.byRequest[cyclePair(v.OrgID, v.link.requestID)] {
			member := catalog.cycle.observations[i]
			if !slices.Contains(parent.selectors.EventIDs, member.EventID) || set && absent[member.EventID] != group {
				return parent, false
			}
			group, set = absent[member.EventID], true
		}
	}
	copy := *parent
	plan := *parent.plan
	copy.plan = &plan
	copy.selectors = componentCopySelectors(parent.selectors)
	copy.selectors.EventIDs = slices.DeleteFunc(copy.selectors.EventIDs, func(event string) bool { return absent[event] != 0 })
	sheets := map[uint64]bool{}
	for _, sheet := range absent {
		sheets[sheet] = true
	}
	copy.selectors.MongoOwners = slices.DeleteFunc(copy.selectors.MongoOwners, func(owner SQLCrossStoreOwnerReference) bool {
		return owner.Kind == "AnswerSheet" && sheets[cyclePayloadID(owner.ID)]
	})
	plan.request.AnswerSheetIDs = slices.DeleteFunc(slices.Clone(parent.plan.request.AnswerSheetIDs), func(sheet uint64) bool { return sheets[sheet] })
	if len(copy.selectors.EventIDs) == 0 {
		if len(plan.before.rows["assessment"]) != 0 || len(plan.request.AssessmentIDs)+len(plan.request.AnswerSheetIDs)+len(copy.selectors.MongoOwners) != 0 {
			return parent, false
		}
	}
	copy.self, copy.seal = &copy, copy.digest()
	return &copy, copy.intact()
}

// FreezeSQLHistoricalAbsentOwnerSelectorRecipe captures only an actual original
// SQL-empty sheet range. All selectors must be contained in the live original
// page. The new indexed reads stay in its original RR snapshot; they are pure
// input and do not authenticate the Mongo owner, source or a future write.
func FreezeSQLHistoricalAbsentOwnerSelectorRecipe(ctx context.Context, original *SQLHistoricalOwnerBatch, cross *SQLHistoricalCrossStorePage, eventIDs []string, mongoOwners []SQLCrossStoreOwnerReference, answerSheetIDs []uint64, spool *SQLHistoricalCASSpool) (*SQLHistoricalComponentRecipe, error) {
	if ctx == nil || ctx.Err() != nil || original == nil || cross == nil || cross.batch != original || !cross.report.Complete || cross.catalog == nil || cross.catalog.cycle != original.cycle || len(eventIDs) == 0 || len(answerSheetIDs) == 0 || original.ValidateBorrowedSnapshot(ctx) != nil {
		return nil, ErrSQLHistoricalComponent
	}
	events := map[string]bool{}
	sheets := map[uint64]bool{}
	owners := map[string]bool{}
	for _, event := range eventIDs {
		if events[event] || !slices.Contains(cross.selectors.EventIDs, event) {
			return nil, ErrSQLHistoricalComponent
		}
		events[event] = true
	}
	for _, sheet := range answerSheetIDs {
		if sheets[sheet] || !slices.Contains(original.request.AnswerSheetIDs, sheet) {
			return nil, ErrSQLHistoricalComponent
		}
		if _, err := original.OwnerByAnswerSheet(sheet); !errors.Is(err, ErrSQLHistoricalOwnerAbsent) {
			return nil, ErrSQLHistoricalComponent
		}
		sheets[sheet] = true
	}
	for _, owner := range mongoOwners {
		key := owner.Kind + ":" + owner.ID
		if owners[key] || !slices.Contains(cross.selectors.MongoOwners, owner) || owner.Kind != "AnswerSheet" || !sheets[cyclePayloadID(owner.ID)] {
			return nil, ErrSQLHistoricalComponent
		}
		owners[key] = true
	}
	if len(owners) != len(sheets) {
		return nil, ErrSQLHistoricalComponent
	}
	selected := map[int]bool{}
	for event := range events {
		for _, i := range cross.catalog.cycle.byEvent[event] {
			selected[i] = true
		}
	}
	for owner := range owners {
		for _, i := range cross.catalog.byMongoOwner[owner] {
			selected[i] = true
		}
	}
	pairs := map[string]bool{}
	for i := range selected {
		v := cross.catalog.cycle.observations[i]
		if v.link.requestID != "" {
			pairs[cyclePair(v.OrgID, v.link.requestID)] = true
		}
	}
	// Replay is an atomic responsibility: inspect every actual catalog member,
	// not only those matching this subset. A wider pair requires regrouping.
	for pair := range pairs {
		for _, i := range cross.catalog.byRequest[pair] {
			selected[i] = true
		}
	}
	for i := range selected {
		v := cross.catalog.cycle.observations[i]
		if v.AssessmentID != 0 || v.EventID != "" && !events[v.EventID] || v.OwnerKind != "" && (v.OwnerKind != "AnswerSheet" || !owners[v.OwnerKind+":"+v.OwnerID]) {
			return nil, ErrSQLHistoricalComponent
		}
	}
	batch, err := PrepareSQLHistoricalOwnerBatch(ctx, original.cycle, SQLHistoricalOwnerBatchRequest{AnswerSheetIDs: slices.Clone(answerSheetIDs)}, original.limits)
	if err != nil || len(batch.owners) != 0 {
		return nil, ErrSQLHistoricalComponent
	}
	for _, sheet := range answerSheetIDs {
		if _, err = batch.OwnerByAnswerSheet(sheet); !errors.Is(err, ErrSQLHistoricalOwnerAbsent) {
			return nil, ErrSQLHistoricalComponent
		}
	}
	subset, err := PrepareSQLHistoricalCrossStorePage(ctx, cross.catalog, batch, SQLCrossStoreSelectors{EventIDs: slices.Clone(eventIDs), OrganizationIDs: slices.Clone(cross.selectors.OrganizationIDs), MongoOwners: slices.Clone(mongoOwners)})
	if err != nil {
		return nil, err
	}
	baseline, err := SealSQLHistoricalCASReadBaseline(ctx, batch)
	if err != nil {
		return nil, err
	}
	r, err := FreezeSQLHistoricalComponentRecipe(ctx, batch, subset, nil, baseline)
	if err != nil {
		return nil, err
	}
	// OwnerBatch represents its empty deduplicated assessment result as nil;
	// the existing native CAS capture uses an explicit empty result. Freeze
	// that actual original capture after checking this sole representation
	// difference. Fresh reads still use the unchanged exact image comparator.
	tx, err := historicalTx(ctx)
	if err != nil {
		return nil, err
	}
	actual, err := r.plan.capture(tx, false)
	expected := casCloneImage(r.plan.before)
	if expected.rows["assessment"] == nil {
		expected.rows["assessment"] = []historicalSQLRow{}
	}
	if err != nil || !reflect.DeepEqual(actual, expected) {
		return nil, ErrSQLHistoricalComponent
	}
	r.plan.before = actual
	r.input.before = casCloneImage(actual)
	r.input.seal = r.input.digest()
	// Resolved refers only to this complete actual SQL-negative closure. The
	// recipe still has no SQL owner, source/Mongo authentication or write grant.
	r.ownerPartitioned = true
	r.seal = r.digest()
	if !r.intact() {
		return nil, ErrSQLHistoricalComponent
	}
	// captureOriginalResponsibility can expand negative governance ranges. No
	// expanded member may quietly import another source or actual SQL owner.
	for table, rows := range r.responsibility.rows {
		for _, row := range rows {
			v := cycleDecode(table, row)
			if v.AssessmentID != 0 || v.EventID != "" && !events[v.EventID] || v.OwnerKind != "" && (v.OwnerKind != "AnswerSheet" || !owners[v.OwnerKind+":"+v.OwnerID]) {
				return nil, ErrSQLHistoricalComponent
			}
		}
	}
	if original.ValidateBorrowedSnapshot(ctx) != nil {
		return nil, ErrSQLHistoricalComponent
	}
	if spool != nil {
		return spoolSQLHistoricalComponentRecipe(ctx, batch, r, spool)
	}
	return r, nil
}

type sqlOriginalOwnerPart struct {
	ids    []uint64
	events []string
	sheets []uint64
	mongo  []SQLCrossStoreOwnerReference
}

func componentOriginalOwnerPartitions(r *SQLHistoricalComponentRecipe, c *SQLHistoricalCrossStoreCatalog, sourceOwners ...map[string]uint64) ([]sqlOriginalOwnerPart, error) {
	if !r.intact() || c == nil || c.cycle == nil || !c.report.Complete || len(sourceOwners) > 1 {
		return nil, ErrSQLHistoricalComponent
	}
	parents := map[uint64]uint64{}
	ownerSheets := map[uint64]uint64{}
	ownerOrgs := map[uint64]uint64{}
	for _, id := range r.plan.request.AssessmentIDs {
		parents[id] = id
	}
	for _, row := range r.plan.before.rows["assessment"] {
		id, err := sqlHistoricalUint(row, "id")
		sheet, se := sqlHistoricalUint(row, "answer_sheet_id")
		org, oe := sqlHistoricalUint(row, "org_id")
		if err != nil || se != nil || oe != nil || id == 0 || sheet == 0 || org == 0 {
			return nil, ErrSQLHistoricalComponent
		}
		parents[id] = id
		if prior := ownerSheets[sheet]; prior != 0 && prior != id {
			return nil, ErrSQLHistoricalComponent
		}
		ownerSheets[sheet] = id
		ownerOrgs[id] = org
	}
	if len(parents) == 0 {
		return nil, nil
	}
	var find func(uint64) uint64
	find = func(id uint64) uint64 {
		if parents[id] != id {
			parents[id] = find(parents[id])
		}
		return parents[id]
	}
	join := func(a, b uint64) {
		a, b = find(a), find(b)
		if a > b {
			a, b = b, a
		}
		parents[b] = a
	}
	eventOwners := map[string]uint64{}
	bind := func(event string, id uint64) error {
		if event == "" || id == 0 {
			return nil
		}
		if _, ok := parents[id]; !ok {
			return ErrSQLHistoricalComponent
		}
		if prior := eventOwners[event]; prior != 0 && prior != id {
			return ErrSQLHistoricalComponent
		}
		eventOwners[event] = id
		return nil
	}
	for _, a := range r.plan.attachments {
		if err := bind(a.Entry.EventID, a.AssessmentID); err != nil {
			return nil, err
		}
	}
	for _, event := range r.selectors.EventIDs {
		for _, i := range c.cycle.byEvent[event] {
			v := c.cycle.observations[i]
			id := v.AssessmentID
			if id == 0 && v.OwnerKind == "AnswerSheet" {
				id = ownerSheets[cyclePayloadID(v.OwnerID)]
			}
			if err := bind(event, id); err != nil {
				return nil, err
			}
		}
	}
	for _, selected := range sourceOwners {
		for event, id := range selected {
			if !slices.Contains(r.selectors.EventIDs, event) || ownerOrgs[id] == 0 || bind(event, id) != nil {
				return nil, ErrSQLHistoricalComponent
			}
		}
	}
	// A selector cannot contradict any current readable identity, organization
	// or sheet binding. Its presence is not evidence of that event's origin.
	for _, event := range r.selectors.EventIDs {
		id := eventOwners[event]
		for _, i := range c.cycle.byEvent[event] {
			v := c.cycle.observations[i]
			if id != 0 && v.OrgID != 0 && ownerOrgs[id] != v.OrgID {
				return nil, ErrSQLHistoricalComponent
			}
			if v.OwnerKind == "AnswerSheet" && id != 0 && ownerSheets[cyclePayloadID(v.OwnerID)] != id {
				return nil, ErrSQLHistoricalComponent
			}
		}
	}
	// Every actual member of each selected replay request is included. A
	// member targeting an owner outside the captured business page cannot be
	// omitted: the caller must capture that owner before splitting this page.
	pairs := map[string]bool{}
	for _, row := range r.responsibility.rows["qs_rm_replay_items"] {
		v := cycleDecode("qs_rm_replay_items", row)
		pairs[cyclePair(v.OrgID, v.link.requestID)] = true
	}
	for pair := range pairs {
		var first uint64
		for _, i := range c.byRequest[pair] {
			v := c.cycle.observations[i]
			id := eventOwners[v.EventID]
			if id == 0 {
				id = v.AssessmentID
			}
			if id == 0 && v.OwnerKind == "AnswerSheet" {
				id = ownerSheets[cyclePayloadID(v.OwnerID)]
			}
			if id == 0 {
				continue
			}
			if _, ok := parents[id]; !ok {
				return nil, ErrSQLHistoricalComponent
			}
			if err := bind(v.EventID, id); err != nil {
				return nil, err
			}
			if first == 0 {
				first = id
			} else {
				join(first, id)
			}
		}
	}
	// Unbound source and missing SQL sheet ranges remain explicit unresolved
	// input. A generation's SQL-negative range can be shared read-only across
	// children; it is never guessed to be an assessment identity.
	for _, event := range r.selectors.EventIDs {
		if eventOwners[event] == 0 {
			return nil, nil
		}
	}
	for _, sheet := range r.plan.request.AnswerSheetIDs {
		if ownerSheets[sheet] == 0 {
			return nil, nil
		}
	}
	for _, owner := range r.selectors.MongoOwners {
		if owner.Kind == "ReportGeneration" && len(sourceOwners) == 1 {
			continue
		}
		if owner.Kind != "AnswerSheet" || ownerSheets[cyclePayloadID(owner.ID)] == 0 {
			return nil, nil
		}
	}
	byRoot := map[uint64]*sqlOriginalOwnerPart{}
	for id := range parents {
		root := find(id)
		if byRoot[root] == nil {
			byRoot[root] = &sqlOriginalOwnerPart{}
		}
		byRoot[root].ids = append(byRoot[root].ids, id)
	}
	for _, event := range r.selectors.EventIDs {
		root := find(eventOwners[event])
		byRoot[root].events = append(byRoot[root].events, event)
	}
	for _, sheet := range r.plan.request.AnswerSheetIDs {
		root := find(ownerSheets[sheet])
		byRoot[root].sheets = append(byRoot[root].sheets, sheet)
	}
	for _, owner := range r.selectors.MongoOwners {
		if owner.Kind == "ReportGeneration" {
			for _, part := range byRoot {
				part.mongo = append(part.mongo, owner)
			}
			continue
		}
		root := find(ownerSheets[cyclePayloadID(owner.ID)])
		byRoot[root].mongo = append(byRoot[root].mongo, owner)
	}
	var out []sqlOriginalOwnerPart
	for _, part := range byRoot {
		sort.Slice(part.ids, func(i, j int) bool { return part.ids[i] < part.ids[j] })
		sort.Strings(part.events)
		out = append(out, *part)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ids[0] < out[j].ids[0] })
	return out, nil
}

func componentOriginalOwnerChild(parent *SQLHistoricalComponentRecipe, part sqlOriginalOwnerPart) (*SQLHistoricalComponentRecipe, error) {
	selected := map[uint64]bool{}
	for _, id := range part.ids {
		selected[id] = true
	}
	p := *parent.plan
	p.before = sqlHistoricalCASImage{rows: map[string][]historicalSQLRow{}, schema: map[string]string{}, columns: map[string][]string{}}
	for table, hash := range parent.plan.before.schema {
		p.before.schema[table] = hash
		p.before.columns[table] = slices.Clone(parent.plan.before.columns[table])
	}
	for table, rows := range parent.plan.before.rows {
		p.before.rows[table] = nil
		if table == "assessment" || !slices.Contains(batchBusinessTables, table) && rows != nil {
			p.before.rows[table] = []historicalSQLRow{}
		}
		for _, row := range rows {
			if !slices.Contains(batchBusinessTables, table) {
				// Actual schema/index/clock/head reads are readonly shared input.
				p.before.rows[table] = append(p.before.rows[table], row)
				continue
			}
			column := "assessment_id"
			if table == "assessment" {
				column = "id"
			}
			id, err := sqlHistoricalUint(row, column)
			if err != nil {
				return nil, ErrSQLHistoricalComponent
			}
			if selected[id] {
				p.before.rows[table] = append(p.before.rows[table], row)
			}
		}
	}
	// The existing native capture preserves nil query results when an owner
	// exists, and explicit empty descendant ranges when no owner exists.
	if len(p.before.rows["assessment"]) == 0 {
		p.before.rows["runtime_checkpoint"] = []historicalSQLRow{}
		p.before.rows["evaluation_outcome"] = []historicalSQLRow{}
	}
	p.request = SQLHistoricalOwnerBatchRequest{AssessmentIDs: slices.Clone(part.ids), AnswerSheetIDs: slices.Clone(part.sheets)}
	p.groups = nil
	p.attachments = nil
	p.missingOriginalRunIDs = nil
	for _, a := range parent.plan.attachments {
		if selected[a.AssessmentID] {
			p.attachments = append(p.attachments, a)
		}
	}
	for _, g := range parent.plan.groups {
		row, err := casRow(parent.plan.before, g.table, g.id)
		if err != nil {
			return nil, err
		}
		column := "assessment_id"
		if g.table == "assessment" {
			column = "id"
		}
		id, err := sqlHistoricalUint(row, column)
		if err != nil {
			return nil, err
		}
		if selected[id] {
			p.groups = append(p.groups, g)
		}
	}
	for _, missing := range parent.plan.missingOriginalRunIDs {
		for _, row := range p.before.rows["evaluation_outcome"] {
			if valueOrEmpty(row["evaluation_run_id"]) == missing {
				p.missingOriginalRunIDs = append(p.missingOriginalRunIDs, missing)
				break
			}
		}
	}
	raw, err := sqlSpoolEncode(componentPlanRecord(&p))
	if err != nil {
		return nil, err
	}
	var record sqlSpoolPlan
	if err = sqlSpoolDecode(raw, &record); err != nil {
		return nil, err
	}
	plan, err := sqlSpoolReconstruct(record, sqlSpoolImageIn(record.Before))
	if err != nil {
		return nil, err
	}
	input := &SQLHistoricalCASFrozenInput{before: casCloneImage(plan.before), writes: map[string]bool{}}
	for _, g := range plan.groups {
		input.writes[sqlSpoolKey(g.table, g.id)] = true
	}
	input.self, input.seal = input, input.digest()
	if err = input.RowDependencies(func(string, uint64, string, uint64, bool) error { return nil }); err != nil {
		return nil, err
	}
	return &SQLHistoricalComponentRecipe{plan: plan, input: input, oldPool: parent.oldPool, limits: parent.limits, ownerPartitioned: true, selectors: SQLCrossStoreSelectors{EventIDs: slices.Clone(part.events), AssessmentIDs: slices.Clone(part.ids), OrganizationIDs: slices.Clone(parent.selectors.OrganizationIDs), MongoOwners: slices.Clone(part.mongo)}}, nil
}

// The complete original cycle already classified every old ledger row and
// authenticated its actual ordered primary key. Expand the component using
// those private indexes and re-read only the exact PRIMARY keys. This is an
// ORIGINAL input read, never a substitute for fresh negative responsibility.
// Global unknown/blocking counts remain in the original cycle and must be
// rejected by whole-source qualification; freezing pure input does not renew
// that qualification or turn unrelated unknown rows into successful evidence.
func (r *SQLHistoricalComponentRecipe) captureOriginalResponsibility(ctx context.Context, catalog *SQLHistoricalCrossStoreCatalog) (sqlHistoricalCASImage, map[string]string, error) {
	image := sqlHistoricalCASImage{rows: map[string][]historicalSQLRow{}, schema: map[string]string{}, columns: map[string][]string{}}
	if catalog == nil || catalog.cycle == nil || !catalog.report.Complete || catalog.cycle.ValidateBorrowedSnapshot(ctx) != nil {
		return image, nil, ErrSQLHistoricalComponent
	}
	selected := map[int]bool{}
	add := func(ids []int) {
		for _, i := range ids {
			selected[i] = true
		}
	}
	for _, id := range r.selectors.EventIDs {
		add(catalog.cycle.byEvent[id])
	}
	for _, id := range r.selectors.AssessmentIDs {
		add(catalog.cycle.byOwner[id])
	}
	for _, owner := range r.selectors.MongoOwners {
		add(catalog.byMongoOwner[owner.Kind+":"+owner.ID])
	}
	for _, org := range r.selectors.OrganizationIDs {
		add(catalog.byGovernanceOrg[org])
		for _, i := range catalog.requestOrganizations[org] {
			v := catalog.cycle.observations[i]
			if len(catalog.byRequest[cyclePair(v.OrgID, v.link.requestID)]) == 0 {
				selected[i] = true
			}
		}
	}
	for round := 0; ; round++ {
		before := len(selected)
		if before > r.limits.MaxPageRows || round > 512 {
			return image, nil, ErrSQLHistoricalComponent
		}
		for i := range selected {
			v := catalog.cycle.observations[i]
			if v.EventID != "" {
				add(catalog.cycle.byEvent[v.EventID])
			}
			if v.Store == "qs_rm_replay_items" || v.Store == "qs_rm_replay_requests" {
				pair := cyclePair(v.OrgID, v.link.requestID)
				add(catalog.requestParents[pair])
				add(catalog.byRequest[pair])
			}
		}
		if len(selected) == before {
			break
		}
	}
	tx, err := historicalTx(ctx)
	if err != nil {
		return image, nil, err
	}
	page := &SQLHistoricalCrossStorePage{catalog: catalog, rows: map[int]historicalSQLRow{}, started: time.Now()}
	if err = page.read(ctx, selected); err != nil {
		return image, nil, err
	}
	for _, ledger := range catalog.cycle.ledgers {
		cols, hash, _, err := cycleSchema(tx, ledger.spec)
		if err != nil || hash != ledger.report.SchemaSHA256 || !reflect.DeepEqual(cols, ledger.columns) || !sqlCrossStoreSupportedColumns(ledger.spec.name, cols) {
			return image, nil, ErrSQLHistoricalComponent
		}
		image.columns[ledger.spec.name], image.schema[ledger.spec.name] = cols, hash
		image.rows[ledger.spec.name] = []historicalSQLRow{}
	}
	for i, row := range page.rows {
		name := catalog.cycle.observations[i].Store
		image.rows[name] = append(image.rows[name], row)
	}
	anchors, err := componentValidateResponsibility(tx, image)
	return image, anchors, err
}

// These expressions are selectors only. Exact original bytes are subsequently
// decoded with the real domain/legacy readers; invalid JSON is never discarded.
func componentMessageDocument(column string) (string, string) {
	// Native message bodies are binary columns. JSON_EXTRACT refuses MySQL's
	// binary character set; conversion is used only for selectors, while every
	// retained body and its fingerprint still use the exact original bytes.
	text := "CONVERT(`" + column + "` USING utf8mb4)"
	outer := "(CASE WHEN JSON_VALID(" + text + ") THEN " + text + " ELSE '{}' END)"
	inner := "(CASE WHEN JSON_UNQUOTE(JSON_EXTRACT(" + outer + ",'$.type'))='component-base.messaging.message.v1' THEN CONVERT(FROM_BASE64(JSON_UNQUOTE(JSON_EXTRACT(" + outer + ",'$.payload'))) USING utf8mb4) ELSE " + text + " END)"
	safe := "(CASE WHEN JSON_VALID(" + inner + ") THEN " + inner + " ELSE '{}' END)"
	return inner, safe
}

func componentPredicate(spec sqlResponsibilityTable, s SQLCrossStoreSelectors, events []string) (string, []any, error) {
	terms := []string{}
	args := []any{}
	add := func(q string, v any) { terms = append(terms, q); args = append(args, v) }
	switch spec.name {
	case "rm_outbox", "retry_event_hold", "event_delivery_dead_letter":
		column, identity := "payload_json", "event_id"
		if spec.name == "rm_outbox" {
			column, identity = "payload", "message_id"
		}
		inner, doc := componentMessageDocument(column)
		terms = append(terms, "COALESCE(JSON_VALID("+inner+"),0)=0")
		add("CAST(`"+identity+"` AS BINARY) IN ?", events)
		add("CAST(JSON_UNQUOTE(JSON_EXTRACT("+doc+",'$.id')) AS BINARY) IN ?", events)
		if len(s.AssessmentIDs) > 0 {
			ids := make([]string, len(s.AssessmentIDs))
			for i, id := range s.AssessmentIDs {
				ids[i] = strconv.FormatUint(id, 10)
			}
			add("CAST(JSON_UNQUOTE(JSON_EXTRACT("+doc+",'$.data.assessment_id')) AS BINARY) IN ?", ids)
			add("(JSON_UNQUOTE(JSON_EXTRACT("+doc+",'$.aggregateType'))='Evaluation' AND CAST(JSON_UNQUOTE(JSON_EXTRACT("+doc+",'$.aggregateID')) AS BINARY) IN ?)", ids)
		}
		for _, o := range s.MongoOwners {
			terms = append(terms, "(CAST(JSON_UNQUOTE(JSON_EXTRACT("+doc+",'$.aggregateType')) AS BINARY)=CAST(? AS BINARY) AND CAST(JSON_UNQUOTE(JSON_EXTRACT("+doc+",'$.aggregateID')) AS BINARY)=CAST(? AS BINARY))")
			args = append(args, o.Kind, o.ID)
		}
	case "qs_rm_evaluation_request_ref", "qs_rm_gap_recovery_request":
		add("CAST(event_id AS BINARY) IN ?", events)
		if len(s.AssessmentIDs) > 0 {
			add("assessment_id IN ?", s.AssessmentIDs)
		}
	case "qs_rm_replay_items":
		add("CAST(event_id AS BINARY) IN ?", events)
	case "qs_rm_replay_requests":
		// An orphan organization request cannot be hidden merely because no
		// event-bearing item yet exists. Complete known pairs are added below.
		if len(s.OrganizationIDs) == 0 {
			return "0=1", nil, nil
		}
		return "(org_id IN ? AND NOT EXISTS (SELECT 1 FROM qs_rm_replay_items component_item WHERE component_item.org_id=qs_rm_replay_requests.org_id AND component_item.request_id=qs_rm_replay_requests.request_id))", []any{s.OrganizationIDs}, nil
	case "system_governance_action_runs":
		if len(s.OrganizationIDs) > 0 {
			add("org_id IN ?", s.OrganizationIDs)
		} else {
			return "0=1", nil, nil
		}
	default:
		return "", nil, ErrSQLHistoricalComponent
	}
	return "(" + strings.Join(terms, " OR ") + ")", args, nil
}

// Bounded fixed-point closure includes every member of a replay pair and then
// every event referenced by those members. It reads new matching rows, rather
// than re-reading only the old primary keys. A growing/oversized graph fails.
func (r *SQLHistoricalComponentRecipe) captureResponsibility(tx *gorm.DB) (sqlHistoricalCASImage, map[string]string, []SQLResponsibilityObservation, error) {
	image := sqlHistoricalCASImage{rows: map[string][]historicalSQLRow{}, schema: map[string]string{}, columns: map[string][]string{}}
	started := time.Now()
	events := map[string]bool{}
	for _, id := range r.selectors.EventIDs {
		events[id] = true
	}
	pairs := map[string]SQLResponsibilityObservation{}
	retained := map[string]map[string]historicalSQLRow{}
	var total int
	var bytes uint64
	for _, spec := range sqlResponsibilityTables {
		cols, hash, _, err := cycleSchema(tx, spec)
		if err != nil || !sqlCrossStoreSupportedColumns(spec.name, cols) {
			return image, nil, nil, ErrSQLHistoricalComponent
		}
		image.schema[spec.name], image.columns[spec.name] = hash, cols
		retained[spec.name] = map[string]historicalSQLRow{}
	}
	for round := 0; round <= 512; round++ {
		if time.Since(started) > r.limits.MaxDuration {
			return image, nil, nil, ErrSQLHistoricalComponent
		}
		beforeEvents, beforePairs := len(events), len(pairs)
		ids := make([]string, 0, len(events))
		for id := range events {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		if len(ids) > 512 || len(pairs) > 512 || len(ids) == 0 && len(r.selectors.AssessmentIDs)+len(r.selectors.MongoOwners)+len(r.selectors.OrganizationIDs) == 0 {
			return image, nil, nil, ErrSQLHistoricalComponent
		}
		for _, spec := range sqlResponsibilityTables {
			predicate, args, err := componentPredicate(spec, r.selectors, ids)
			if err != nil {
				return image, nil, nil, err
			}
			if spec.name == "qs_rm_replay_items" || spec.name == "qs_rm_replay_requests" {
				keys := make([]string, 0, len(pairs))
				for key := range pairs {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				for _, key := range keys {
					v := pairs[key]
					predicate += " OR (org_id=? AND CAST(request_id AS BINARY)=CAST(? AS BINARY))"
					args = append(args, v.OrgID, v.link.requestID)
				}
			}
			rows, cols, size, err := cycleQuery(tx, "SELECT * FROM `"+spec.name+"` WHERE "+predicate+" ORDER BY "+strings.Join(spec.keys, ",")+" LIMIT ?", r.limits.MaxPageRows, append(args, r.limits.MaxPageRows+1)...)
			if err != nil {
				return image, nil, nil, err
			}
			if !reflect.DeepEqual(cols, image.columns[spec.name]) || size > r.limits.MaxPageBytes {
				return image, nil, nil, ErrSQLHistoricalComponent
			}
			for _, row := range rows {
				key, err := cycleKey(spec, row)
				if err != nil {
					return image, nil, nil, err
				}
				digest := cycleKeyDigest(key)
				if previous := retained[spec.name][digest]; previous != nil {
					if !reflect.DeepEqual(previous, row) {
						return image, nil, nil, ErrSQLHistoricalComponent
					}
					continue
				}
				total++
				bytes += uint64(len(cycleRowDigest(cols, row)))
				for _, cell := range row {
					if cell != nil {
						bytes += uint64(len(*cell))
					}
				}
				if total > r.limits.MaxPageRows || bytes > r.limits.MaxPageBytes {
					return image, nil, nil, ErrSQLHistoricalComponent
				}
				retained[spec.name][digest] = row
				v := cycleDecode(spec.name, row)
				if v.EventID != "" {
					events[v.EventID] = true
				}
				if spec.name == "qs_rm_replay_items" || spec.name == "qs_rm_replay_requests" {
					if v.OrgID == 0 || v.link.requestID == "" {
						return image, nil, nil, ErrSQLHistoricalComponent
					}
					pairs[cyclePair(v.OrgID, v.link.requestID)] = v
				}
			}
		}
		if len(events) == beforeEvents && len(pairs) == beforePairs {
			break
		}
		if round == 512 {
			return image, nil, nil, ErrSQLHistoricalComponent
		}
	}
	for _, spec := range sqlResponsibilityTables {
		for _, row := range retained[spec.name] {
			image.rows[spec.name] = append(image.rows[spec.name], row)
		}
	}
	anchors, observations, err := componentResponsibilityFacts(tx, image)
	return image, anchors, observations, err
}

func componentValidateResponsibility(tx *gorm.DB, image sqlHistoricalCASImage) (map[string]string, error) {
	anchors, _, err := componentResponsibilityFacts(tx, image)
	return anchors, err
}

// This decoder classifies actual scoped rows; it never marks a global cycle complete.
func componentResponsibilityFacts(tx *gorm.DB, image sqlHistoricalCASImage) (map[string]string, []SQLResponsibilityObservation, error) {
	c := &SQLHistoricalResponsibilityCycle{owners: map[uint64]sqlResponsibilityOwner{}, anchorDigests: map[string]string{}, byOwner: map[uint64][]int{}, byEvent: map[string][]int{}, byOrgActions: map[uint64][]int{}}
	for _, spec := range sqlResponsibilityTables {

		if image.rows[spec.name] == nil {
			image.rows[spec.name] = []historicalSQLRow{}
		}
		sort.Slice(image.rows[spec.name], func(i, j int) bool {
			a, _ := cycleKey(spec, image.rows[spec.name][i])
			b, _ := cycleKey(spec, image.rows[spec.name][j])
			return cycleCompare(spec, a, b) < 0
		})
		for _, row := range image.rows[spec.name] {
			c.observations = append(c.observations, cycleDecode(spec.name, row))
		}
		_, hash, _, err := cycleSchema(tx, spec)
		if err != nil || hash != image.schema[spec.name] {
			return nil, nil, ErrSQLHistoricalComponent
		}
	}
	if err := c.checkPageOwners(tx, 0); err != nil {
		return nil, nil, err
	}
	c.checkReverse()
	for _, v := range c.observations {
		if v.Invalid || v.ScopeClass == "retirement_related" && (v.Unfinished || v.LeasePresent) {
			return nil, nil, ErrSQLHistoricalComponent
		}
	}
	return c.anchorDigests, c.observations, nil
}

func (o *SQLHistoricalComponentObservation) digest() string {
	if o == nil || !o.recipe.intact() {
		return ""
	}
	return cycleKeyDigest([]string{o.recipe.seal, sqlHistoricalProvenancePoolToken(o.pool), strconv.FormatUint(o.transaction.connection, 10), strconv.FormatUint(o.transaction.thread, 10), strconv.FormatUint(o.transaction.event, 10), o.expires.UTC().Format(time.RFC3339Nano), strconv.FormatBool(o.writable), casImageHash(o.business), casImageHash(o.responsibility)})
}
func (o *SQLHistoricalComponentObservation) live(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil || o == nil || o.self != o || o.seal == "" || o.seal != o.digest() || time.Now().After(o.expires) {
		return ErrSQLHistoricalComponent
	}
	bounded, cancel := context.WithDeadline(ctx, o.expires)
	defer cancel()
	tx, err := historicalTx(bounded)
	if err != nil || tx.Statement.ConnPool != o.pool {
		return ErrSQLHistoricalComponent
	}
	server, database, err := historicalDatabase(tx)
	if err != nil || server != o.recipe.plan.server || database != o.recipe.plan.database || sqlHistoricalIdentity(server, database) != o.recipe.plan.identity {
		return ErrSQLHistoricalComponent
	}
	var actual sqlResponsibilityTransaction
	if o.writable {
		actual, err = casActualRW(tx)
	} else {
		actual, err = cycleActualTransaction(tx)
	}
	if err != nil || actual != o.transaction || bounded.Err() != nil || !time.Now().Before(o.expires) {
		return ErrSQLHistoricalComponent
	}
	return nil
}

// writable selects native RW instrumentation; it is not a permission boolean.
// Both modes perform the same genuine fresh full component reads before return.
func PrepareSQLHistoricalComponentObservation(ctx context.Context, r *SQLHistoricalComponentRecipe, budget time.Duration, writable bool) (*SQLHistoricalComponentObservation, error) {
	if ctx == nil || ctx.Err() != nil || !r.intact() || budget <= 0 || budget > 20*time.Second {
		return nil, ErrSQLHistoricalComponent
	}
	started := time.Now()
	bounded, cancel := context.WithDeadline(ctx, started.Add(budget))
	defer cancel()
	r, err := r.load(bounded)
	if err != nil {
		return nil, err
	}
	tx, err := historicalTx(bounded)
	if err != nil || tx.Statement.ConnPool == r.oldPool {
		return nil, ErrSQLHistoricalComponent
	}
	server, database, err := historicalDatabase(tx)
	if err != nil || server != r.plan.server || database != r.plan.database {
		return nil, ErrSQLHistoricalComponent
	}
	image, err := r.plan.capture(tx, writable)
	if err != nil || !reflect.DeepEqual(image, r.plan.before) {
		return nil, fmt.Errorf("%w: business_read_or_baseline", ErrSQLHistoricalComponent)
	}
	var actual sqlResponsibilityTransaction
	if writable {
		actual, err = casActualRW(tx)
	} else {
		actual, err = cycleActualTransaction(tx)
	}
	if err != nil || actual == r.plan.oldTransaction {
		return nil, ErrSQLHistoricalComponent
	}
	responsibility, anchors, observations, err := r.captureResponsibility(tx)
	if err != nil {
		return nil, fmt.Errorf("%w: responsibility_read", ErrSQLHistoricalComponent)
	}
	if !reflect.DeepEqual(responsibility, r.responsibility) {
		return nil, fmt.Errorf("%w: responsibility_baseline", ErrSQLHistoricalComponent)
	}
	if !reflect.DeepEqual(anchors, r.anchors) {
		return nil, fmt.Errorf("%w: responsibility_anchors", ErrSQLHistoricalComponent)
	}
	o := &SQLHistoricalComponentObservation{recipe: r, pool: tx.Statement.ConnPool, transaction: actual, expires: started.Add(budget), writable: writable, business: image, responsibility: responsibility, observations: observations}
	o.self = o
	o.seal = o.digest()
	if o.live(bounded) != nil {
		return nil, ErrSQLHistoricalComponent
	}
	return o, nil
}

func (o *SQLHistoricalComponentObservation) Report() SQLHistoricalComponentReadReport {
	return SQLHistoricalComponentReadReport{BusinessMatched: o != nil && o.self == o && o.seal != "" && o.seal == o.digest(), ResponsibilitiesMatched: o != nil && o.self == o && o.seal != "" && o.seal == o.digest(), FullSourcesRequired: true, MongoQualificationRequired: true, AIClosureRequired: true, HostCommitRequired: true}
}

// ValidateBorrowedObservation checks the actual host pool, database identity,
// native transaction and original deadline. It grants no source, business-terminal or write permit.
func (o *SQLHistoricalComponentObservation) ValidateBorrowedObservation(ctx context.Context) error {
	if o.live(ctx) != nil || o.used {
		return ErrSQLHistoricalComponent
	}
	return nil
}

// This view retains only facts decoded from this observer's actual fresh reads.
// It cannot construct a completed global cycle or enter the physical CAS API.
type SQLHistoricalComponentSemanticView struct {
	self        *SQLHistoricalComponentSemanticView
	observation *SQLHistoricalComponentObservation
	owners      map[uint64]*SQLHistoricalOwnerFacts
	sheets      map[uint64]uint64
	seal        string
}

func (*SQLHistoricalComponentSemanticView) MarshalJSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalComponentSemanticView) MarshalBSON() ([]byte, error) {
	return nil, ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalComponentSemanticView) UnmarshalJSON([]byte) error {
	return ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalComponentSemanticView) UnmarshalBSON([]byte) error {
	return ErrSQLHistoricalFactsSerialization
}
func (*SQLHistoricalComponentSemanticView) String() string {
	return "private fresh scoped SQL facts; source/Mongo/AI qualification required"
}
func (v *SQLHistoricalComponentSemanticView) GoString() string { return v.String() }

func (v *SQLHistoricalComponentSemanticView) ValidateBorrowedSnapshot(ctx context.Context) error {
	if v == nil || v.self != v || v.observation == nil || v.seal == "" || v.seal != v.observation.seal {
		return ErrSQLHistoricalComponent
	}
	return v.observation.ValidateBorrowedObservation(ctx)
}

// MatchesInput binds these live facts to the exact original recipe, including
// a revalidated owned spool. Matching a hash alone never creates this view.
func (v *SQLHistoricalComponentSemanticView) MatchesInput(ctx context.Context, input *SQLHistoricalComponentRecipe) error {
	if v.ValidateBorrowedSnapshot(ctx) != nil {
		return ErrSQLHistoricalComponent
	}
	r, err := input.load(ctx)
	if err != nil || r.seal != v.observation.recipe.seal {
		return ErrSQLHistoricalComponent
	}
	return v.ValidateBorrowedSnapshot(ctx)
}

func (o *SQLHistoricalComponentObservation) SemanticView(ctx context.Context) (*SQLHistoricalComponentSemanticView, error) {
	if o.ValidateBorrowedObservation(ctx) != nil {
		return nil, ErrSQLHistoricalComponent
	}
	if o.semantic != nil {
		if err := o.semantic.ValidateBorrowedSnapshot(ctx); err != nil {
			return nil, err
		}
		return o.semantic, nil
	}
	bounded, cancel := context.WithDeadline(ctx, o.expires)
	defer cancel()
	tx, err := historicalTx(bounded)
	if err != nil {
		return nil, err
	}
	v := &SQLHistoricalComponentSemanticView{observation: o, owners: map[uint64]*SQLHistoricalOwnerFacts{}, sheets: map[uint64]uint64{}}
	clocks := o.business.rows["business_clock_columns"]
	if len(clocks) != 7 {
		return nil, ErrSQLHistoricalComponent
	}
	ids := make([]uint64, 0, len(o.business.rows["assessment"]))
	for _, row := range o.business.rows["assessment"] {
		id, e := sqlHistoricalUint(row, "id")
		sheet, se := sqlHistoricalUint(row, "answer_sheet_id")
		if e != nil || se != nil || id == 0 || sheet == 0 || row["deleted_at"] != nil || v.owners[id] != nil || v.sheets[sheet] != 0 {
			return nil, ErrSQLHistoricalComponent
		}
		f := &SQLHistoricalOwnerFacts{identity: o.recipe.plan.identity, assessmentID: id, outcomeRecords: map[uint64]*evaluationfact.Record{}}
		if err = batchDecodeOwner(f, row, clocks); err != nil {
			return nil, err
		}
		v.owners[id], v.sheets[sheet] = f, id
		ids = append(ids, id)
	}
	for _, row := range o.business.rows["runtime_checkpoint"] {
		id, e := sqlHistoricalUint(row, "assessment_id")
		f := v.owners[id]
		if e != nil || f == nil || len(f.snapshot.Runs) >= SQLHistoricalOwnerRowLimit {
			return nil, ErrSQLHistoricalComponent
		}
		run, e := batchDecodeRun(row)
		if e != nil {
			return nil, e
		}
		f.snapshot.Runs = append(f.snapshot.Runs, run)
	}
	// Use the existing domain outcome mapper, with every typed row checked
	// against the already captured raw bytes in the SAME native transaction.
	var pos []EvaluationOutcomePO
	if len(ids) != 0 {
		if err = tx.Raw("SELECT * FROM evaluation_outcome FORCE INDEX (uk_evaluation_outcome_assessment_id) WHERE assessment_id IN ? ORDER BY id LIMIT ?", ids, o.recipe.plan.limits.MaxRows+1).Scan(&pos).Error; err != nil {
			return nil, ErrSQLHistoricalFactsRead
		}
	}
	if len(pos) != len(o.business.rows["evaluation_outcome"]) {
		return nil, ErrSQLHistoricalComponent
	}
	for i, row := range o.business.rows["evaluation_outcome"] {
		id, e := sqlHistoricalUint(row, "assessment_id")
		rowID, re := sqlHistoricalUint(row, "id")
		org, oe := sqlHistoricalUint(row, "org_id")
		testee, te := sqlHistoricalUint(row, "testee_id")
		at, ae := sqlHistoricalTime(row, "evaluated_at")
		f, po := v.owners[id], &pos[i]
		if e != nil || re != nil || oe != nil || te != nil || ae != nil || at == nil || f == nil || po.ID != rowID || po.AssessmentID != id || po.OrgID <= 0 || uint64(po.OrgID) != org || po.TesteeID != testee || po.EvaluationRunID != valueOrEmpty(row["evaluation_run_id"]) || !po.EvaluatedAt.Equal(*at) || org != f.snapshot.Owner.OrgID || testee != f.snapshot.Owner.TesteeID {
			return nil, ErrSQLHistoricalComponent
		}
		if po.ModelKind != valueOrEmpty(row["model_kind"]) || po.ModelCode != valueOrEmpty(row["model_code"]) || po.ModelVersion != valueOrEmpty(row["model_version"]) || strconv.FormatUint(uint64(po.SchemaVersion), 10) != valueOrEmpty(row["schema_version"]) || po.PayloadJSON != valueOrEmpty(row["payload_json"]) {
			return nil, ErrSQLHistoricalComponent
		}
		for column, actual := range map[string]*string{"model_sub_kind": po.ModelSubKind, "model_algorithm": po.ModelAlgorithm, "model_title": po.ModelTitle, "decision_kind": po.DecisionKind, "input_snapshot_ref": po.InputSnapshotRef, "report_input_json": po.ReportInputJSON} {
			if raw, exists := row[column]; !exists || !reflect.DeepEqual(raw, actual) {
				return nil, ErrSQLHistoricalComponent
			}
		}
		record, e := outcomeFromPO(po)
		if e == nil {
			f.outcomeRecords[rowID] = sqlHistoricalFactRecord(record)
		}
		f.snapshot.Outcomes = append(f.snapshot.Outcomes, SQLHistoricalOutcome{ID: rowID, AssessmentID: id, OrgID: org, TesteeID: testee, RunID: po.EvaluationRunID, EvaluatedAt: *at, Invalid: e != nil})
	}
	for id, f := range v.owners {
		for _, row := range o.observations {
			if row.AssessmentID == id || row.Store == "system_governance_action_runs" && row.OrgID == f.snapshot.Owner.OrgID {
				f.snapshot.Responsibilities = append(f.snapshot.Responsibilities, SQLHistoricalResponsibility{Store: row.Store, ID: row.PrimaryKeySHA256, EventID: row.EventID, EventType: row.EventType, State: row.State, OrgID: row.OrgID, AssessmentID: row.AssessmentID, TesteeID: row.TesteeID, LeasePresent: row.LeasePresent, Unfinished: row.Unfinished, Invalid: row.Invalid || row.OwnerUnproven})
			}
		}
	}
	v.self, v.seal = v, o.seal
	if v.ValidateBorrowedSnapshot(bounded) != nil {
		return nil, ErrSQLHistoricalComponent
	}
	o.semantic = v
	return v, nil
}

func (v *SQLHistoricalComponentSemanticView) OwnerByAssessment(ctx context.Context, id uint64) (SQLHistoricalFactsSnapshot, error) {
	if id == 0 || v.ValidateBorrowedSnapshot(ctx) != nil {
		return SQLHistoricalFactsSnapshot{}, ErrSQLHistoricalComponent
	}
	if f := v.owners[id]; f != nil {
		return f.Snapshot(), nil
	}
	if slices.Contains(v.observation.recipe.plan.request.AssessmentIDs, id) {
		return SQLHistoricalFactsSnapshot{}, ErrSQLHistoricalOwnerAbsent
	}
	return SQLHistoricalFactsSnapshot{}, ErrSQLHistoricalComponent
}

func (v *SQLHistoricalComponentSemanticView) OwnerByAnswerSheet(ctx context.Context, id uint64) (SQLHistoricalFactsSnapshot, error) {
	if id == 0 || v.ValidateBorrowedSnapshot(ctx) != nil {
		return SQLHistoricalFactsSnapshot{}, ErrSQLHistoricalComponent
	}
	if owner := v.sheets[id]; owner != 0 {
		return v.owners[owner].Snapshot(), nil
	}
	if slices.Contains(v.observation.recipe.plan.request.AnswerSheetIDs, id) {
		return SQLHistoricalFactsSnapshot{}, ErrSQLHistoricalOwnerAbsent
	}
	return SQLHistoricalFactsSnapshot{}, ErrSQLHistoricalComponent
}

// Return the whole actual scoped closure, including all replay members and
// negative ranges. The aggregate must not replace it with an event-only slice.
func (v *SQLHistoricalComponentSemanticView) Responsibilities(ctx context.Context) ([]SQLResponsibilityObservation, error) {
	if v.ValidateBorrowedSnapshot(ctx) != nil {
		return nil, ErrSQLHistoricalComponent
	}
	rows := slices.Clone(v.observation.observations)
	for i := range rows {
		rows[i].Reasons = slices.Clone(rows[i].Reasons)
	}
	return rows, nil
}

// CrossStoreRows returns data from the whole actual responsibility closure,
// including every replay member. It grants no completed cycle or CAS permit.
func (v *SQLHistoricalComponentSemanticView) CrossStoreRows(ctx context.Context) ([]SQLCrossStoreRow, error) {
	if v.ValidateBorrowedSnapshot(ctx) != nil {
		return nil, ErrSQLHistoricalComponent
	}
	o := v.observation
	bounded, cancel := context.WithDeadline(ctx, o.expires)
	defer cancel()
	tx, err := historicalTx(bounded)
	if err != nil || tx.Statement.ConnPool != o.pool {
		return nil, ErrSQLHistoricalComponent
	}
	head, _, _, err := cycleQuery(tx, "SELECT version,dirty FROM schema_migrations ORDER BY version", 2)
	if err != nil || len(head) != 1 || valueOrEmpty(head[0]["dirty"]) != "0" || !reflect.DeepEqual(head, o.business.rows["cas_migration_head"]) {
		return nil, ErrSQLHistoricalComponent
	}
	for _, spec := range sqlResponsibilityTables {
		columns, schema, _, e := cycleSchema(tx, spec)
		if e != nil || schema != o.responsibility.schema[spec.name] || !reflect.DeepEqual(columns, o.responsibility.columns[spec.name]) || !sqlCrossStoreSupportedColumns(spec.name, columns) {
			return nil, ErrSQLHistoricalComponent
		}
	}
	rows, err := componentCrossStoreRows(o.responsibility, o.observations)
	if err != nil || v.ValidateBorrowedSnapshot(bounded) != nil {
		return nil, ErrSQLHistoricalComponent
	}
	return rows, nil
}

// Only the live caller validates native scope/schema. This private decoder
// consumes that caller's captured image; no selector can cut its closure.
func componentCrossStoreRows(image sqlHistoricalCASImage, observations []SQLResponsibilityObservation) ([]SQLCrossStoreRow, error) {
	if len(image.rows) != len(sqlResponsibilityTables) || len(image.columns) != len(sqlResponsibilityTables) {
		return nil, ErrSQLHistoricalComponent
	}
	out := make([]SQLCrossStoreRow, 0, len(observations))
	rawRows := make([]historicalSQLRow, 0, len(observations))
	parents, items := map[string][]int{}, map[string][]int{}
	for _, spec := range sqlResponsibilityTables {
		rows, ok := image.rows[spec.name]
		if !ok || len(image.columns[spec.name]) == 0 {
			return nil, ErrSQLHistoricalComponent
		}
		var previous []string
		for _, row := range rows {
			key, err := cycleKey(spec, row)
			if err != nil || previous != nil && cycleCompare(spec, previous, key) >= 0 || len(out) >= len(observations) || len(row) != len(image.columns[spec.name]) {
				return nil, ErrSQLHistoricalComponent
			}
			for _, column := range image.columns[spec.name] {
				if _, ok = row[column]; !ok {
					return nil, ErrSQLHistoricalComponent
				}
			}
			previous = key
			observed, decoded := observations[len(out)], cycleDecode(spec.name, row)
			pk, hash := cycleKeyDigest(key), cycleRowDigest(image.columns[spec.name], row)
			if observed.Invalid || observed.PrimaryKeySHA256 != "" && observed.PrimaryKeySHA256 != pk || observed.RowSHA256 != "" && observed.RowSHA256 != hash {
				return nil, ErrSQLHistoricalComponent
			}
			base := observed
			base.PrimaryKeySHA256, base.RowSHA256 = "", ""
			base.Reasons, base.OwnerUnproven = decoded.Reasons, decoded.OwnerUnproven
			// checkReverse enriches replay items from their actual parent/current
			// message, and a gap decision from its actual current message. Keep
			// those observed conclusions while checking the raw intrinsic fields.
			switch spec.name {
			case "qs_rm_replay_items":
				base.AssessmentID, base.TesteeID, base.EventType = decoded.AssessmentID, decoded.TesteeID, decoded.EventType
				base.OwnerKind, base.OwnerID, base.ScopeClass = decoded.OwnerKind, decoded.OwnerID, decoded.ScopeClass
				base.link.store, base.Unfinished = decoded.link.store, decoded.Unfinished
			case "qs_rm_gap_recovery_request":
				base.Unfinished = decoded.Unfinished
			}
			if !reflect.DeepEqual(base, decoded) {
				return nil, ErrSQLHistoricalComponent
			}
			f := SQLCrossStoreRow{Observation: observed}
			f.Observation.PrimaryKeySHA256, f.Observation.RowSHA256 = pk, hash
			f.Observation.Reasons = slices.Clone(observed.Reasons)
			if spec.name == "rm_outbox" || spec.name == "retry_event_hold" || spec.name == "event_delivery_dead_letter" {
				raw := mustCycleInner(row, spec.name)
				var inner domainwire.Envelope
				if sqlHistoricalStrictJSON(raw, &inner) != nil {
					return nil, ErrSQLHistoricalComponent
				}
				f.Inner = &inner
				sum := sha256.Sum256(raw)
				f.LegacyContentSHA256 = hex.EncodeToString(sum[:])
				sum = sha256.Sum256(inner.Data)
				f.InnerDataSHA256 = hex.EncodeToString(sum[:])
			}
			pair := cyclePair(observed.OrgID, observed.link.requestID)
			switch spec.name {
			case "qs_rm_replay_requests":
				parents[pair] = append(parents[pair], len(out))
			case "qs_rm_replay_items":
				items[pair] = append(items[pair], len(out))
			}
			out, rawRows = append(out, f), append(rawRows, row)
		}
	}
	if len(out) != len(observations) {
		return nil, ErrSQLHistoricalComponent
	}
	for pair, indexes := range items {
		if len(parents[pair]) != 1 {
			return nil, ErrSQLHistoricalComponent
		}
		parent := parents[pair][0]
		header := rawRows[parent]
		org := cyclePositive(header, "org_id")
		if org == 0 || org > 1<<63-1 {
			return nil, ErrSQLHistoricalComponent
		}
		input := standard.ReplayRequest{OrgID: int64(org), RequestID: valueOrEmpty(header["request_id"]), Store: valueOrEmpty(header["store_name"]), Reason: valueOrEmpty(header["reason"])}
		replay := SQLCrossStoreReplay{OrganizationID: org, RequestID: input.RequestID, Store: input.Store, InputSHA256: hex.EncodeToString([]byte(valueOrEmpty(header["input_hash"])))}
		for ordinal, index := range indexes {
			row := rawRows[index]
			n, valid := cycleSafeUint(row, "ordinal")
			failure, validFailure := cycleSafeUint(row, "expected_failure_count")
			if !valid || !validFailure || n != uint64(ordinal) || cyclePositive(row, "org_id") != org || valueOrEmpty(row["request_id"]) != input.RequestID {
				return nil, ErrSQLHistoricalComponent
			}
			replay.Items = append(replay.Items, SQLCrossStoreReplayItem{EventID: valueOrEmpty(row["event_id"]), ExpectedFailureCount: failure, Authorized: valueOrEmpty(row["authorized"]) == "1", Reason: valueOrEmpty(row["reason"])})
			input.Targets = append(input.Targets, standard.ReplayTarget{EventID: valueOrEmpty(row["event_id"]), ExpectedFailureCount: failure})
		}
		fingerprint, err := input.Fingerprint()
		replay.FingerprintVerified = err == nil && bytes.Equal(fingerprint[:], []byte(valueOrEmpty(header["input_hash"])))
		for _, index := range append(indexes, parent) {
			copy := replay
			copy.Items = slices.Clone(replay.Items)
			out[index].Replay = &copy
		}
	}
	return out, nil
}

func (v *SQLHistoricalComponentSemanticView) OutcomeRecord(ctx context.Context, id uint64) (*evaluationfact.Record, error) {
	if id == 0 || v.ValidateBorrowedSnapshot(ctx) != nil {
		return nil, ErrSQLHistoricalComponent
	}
	for _, f := range v.owners {
		if record := f.outcomeRecords[id]; record != nil {
			return record, nil
		}
	}
	return nil, ErrSQLHistoricalComponent
}

func (v *SQLHistoricalComponentSemanticView) OriginalOutcomeRunAbsent(ctx context.Context, outcomeID uint64, runID string) error {
	record, err := v.OutcomeRecord(ctx, outcomeID)
	if err != nil || runID == "" || record.RunID() != runID {
		return ErrSQLHistoricalComponent
	}
	bounded, cancel := context.WithDeadline(ctx, v.observation.expires)
	defer cancel()
	tx, err := historicalTx(bounded)
	if err != nil || casMissingOriginalRuns(tx, []string{runID}, false) != nil {
		return ErrSQLHistoricalComponent
	}
	return v.ValidateBorrowedSnapshot(bounded)
}

func (v *SQLHistoricalComponentSemanticView) BusinessBinding(ctx context.Context, assessmentID, outcomeID uint64, eventType string, originalRun *evidence.HistoricalRunReferenceV1) (string, error) {
	if v.ValidateBorrowedSnapshot(ctx) != nil {
		return "", ErrSQLHistoricalComponent
	}
	a := SQLHistoricalBatchAttachment{AssessmentID: assessmentID, OutcomeID: outcomeID, Entry: evidence.HistoricalReferenceEntryV1{EventType: eventType, Run: originalRun}}
	table, _, _, row, run, err := casTargetForBinding(v.observation.business, a, true)
	if err != nil {
		return "", err
	}
	if table == "evaluation_outcome" && originalRun == nil {
		if err = v.OriginalOutcomeRunAbsent(ctx, outcomeID, valueOrEmpty(row["evaluation_run_id"])); err != nil {
			return "", err
		}
	}
	p := v.observation.recipe.plan
	result, err := historicalStableBinding(p.server, p.database, table, eventType, row, run)
	if err != nil || v.ValidateBorrowedSnapshot(ctx) != nil {
		return "", ErrSQLHistoricalComponent
	}
	return result, nil
}

// The old frozen-provenance regression path remains private. Planning inputs
// use the explicit SQL physical persistence boundary below; the maintenance
// host independently qualifies source/Mongo/AI before any production effect.
func (o *SQLHistoricalComponentObservation) apply(ctx context.Context) (*SQLHistoricalComponentStatement, error) {
	if o == nil {
		return nil, ErrSQLHistoricalComponent
	}
	o.applyMu.Lock()
	defer o.applyMu.Unlock()
	if o.live(ctx) != nil || !o.writable || o.used || len(o.recipe.plan.groups) == 0 {
		return nil, ErrSQLHistoricalComponent
	}
	o.used = true
	statement, err := o.recipe.plan.Apply(ctx)
	if err != nil {
		return nil, err
	}
	if statement.transaction != o.transaction || o.live(ctx) != nil {
		return nil, ErrSQLHistoricalComponent
	}
	s := &SQLHistoricalComponentStatement{observation: o, plan: o.recipe.plan, statement: statement}
	s.self = s
	s.seal = s.digest()
	return s, nil
}

// ApplyHistoricalAttachments is the SQL physical persistence boundary, like
// SQLHistoricalBatchCASPlan.Apply. It grants no source, AI or business closure
// authority. The maintenance host must qualify the actual cross-store graph
// before calling it; a CLI must not call it with imported conclusions.
// Every target and binding uses this observer's fresh locked full-row baseline
// and actual semantic owner view, never a reconstructed cycle or owner batch.
func (o *SQLHistoricalComponentObservation) ApplyHistoricalAttachments(ctx context.Context, attachments []SQLHistoricalBatchAttachment) (*SQLHistoricalComponentStatement, error) {
	return ApplySQLHistoricalComponentAttachments(ctx, []*SQLHistoricalComponentObservation{o}, [][]SQLHistoricalBatchAttachment{attachments})
}

// ApplySQLHistoricalComponentAttachments preserves every fragment's sealed
// source selector and live owner read. Overlapping physical targets are written
// once, with all entries, so the host has one final image to read independently.
// This is still only the SQL physical boundary; source/Mongo/AI qualification is
// required at the maintenance composition before calling it.
func ApplySQLHistoricalComponentAttachments(ctx context.Context, observations []*SQLHistoricalComponentObservation, attachments [][]SQLHistoricalBatchAttachment) (*SQLHistoricalComponentStatement, error) {
	if ctx == nil || ctx.Err() != nil || len(observations) == 0 || len(observations) > 512 || len(observations) != len(attachments) {
		return nil, ErrSQLHistoricalComponent
	}
	unique := map[*SQLHistoricalComponentObservation]bool{}
	order := slices.Clone(observations)
	for _, o := range order {
		if o == nil || unique[o] {
			return nil, ErrSQLHistoricalComponent
		}
		unique[o] = true
	}
	// Consistent lock order also prevents concurrent overlapping submissions
	// from using any original observation twice.
	sort.Slice(order, func(i, j int) bool { return reflect.ValueOf(order[i]).Pointer() < reflect.ValueOf(order[j]).Pointer() })
	for _, o := range order {
		o.applyMu.Lock()
	}
	defer func() {
		for i := len(order) - 1; i >= 0; i-- {
			order[i].applyMu.Unlock()
		}
	}()
	expires := observations[0].expires
	for _, o := range observations {
		if o.live(ctx) != nil || !o.writable || o.used || len(o.recipe.plan.groups) != 0 || len(o.recipe.plan.attachments) != 0 {
			return nil, ErrSQLHistoricalComponent
		}
		if o.expires.Before(expires) {
			expires = o.expires
		}
	}
	bounded, cancel := context.WithDeadline(ctx, expires)
	defer cancel()
	p, err := componentSQLUnionPlan(observations)
	if err != nil {
		return nil, err
	}
	owners := map[uint64]*SQLHistoricalOwnerFacts{}
	var all []SQLHistoricalBatchAttachment
	for i, o := range observations {
		view, err := o.SemanticView(bounded)
		if err != nil {
			return nil, err
		}
		for _, a := range attachments[i] {
			// A different fragment's event or owner cannot expand this scope.
			if !slices.Contains(o.recipe.selectors.EventIDs, a.Entry.EventID) || view.owners[a.AssessmentID] == nil || len(all) == 512 {
				return nil, ErrSQLHistoricalComponent
			}
			all = append(all, a)
		}
		for id, f := range view.owners {
			if previous := owners[id]; previous != nil {
				left, right := previous.Snapshot(), f.Snapshot()
				left.Responsibilities, right.Responsibilities = nil, nil
				if previous.identity != f.identity || !reflect.DeepEqual(left, right) {
					return nil, ErrSQLHistoricalComponent
				}
				for _, responsibility := range f.snapshot.Responsibilities {
					found := false
					for _, prior := range previous.snapshot.Responsibilities {
						if prior.Store == responsibility.Store && prior.ID == responsibility.ID {
							if !reflect.DeepEqual(prior, responsibility) {
								return nil, ErrSQLHistoricalComponent
							}
							found = true
							break
						}
					}
					if !found {
						previous.snapshot.Responsibilities = append(previous.snapshot.Responsibilities, responsibility)
					}
				}
			} else {
				copy := *f
				copy.snapshot = f.Snapshot()
				owners[id] = &copy
			}
		}
	}
	if len(all) == 0 {
		return nil, ErrSQLHistoricalComponent
	}
	tx, err := historicalTx(bounded)
	if err != nil {
		return nil, err
	}
	before := casCloneImage(p.before)
	p, err = prepareSQLHistoricalBatchCASFromImage(bounded, tx, p, owners, all)
	if err != nil || !reflect.DeepEqual(p.before, before) {
		return nil, ErrSQLHistoricalComponent
	}
	for _, o := range observations {
		if o.live(bounded) != nil {
			return nil, ErrSQLHistoricalComponent
		}
	}
	// Poison ALL inputs before the first possible effect, including fragments
	// that provide only a negative dependency. Unknown results cannot be reused.
	for _, o := range observations {
		o.used = true
	}
	statement, err := p.Apply(bounded)
	if err != nil {
		return nil, err
	}
	for _, o := range observations {
		if statement.transaction != o.transaction || o.live(bounded) != nil {
			return nil, ErrSQLHistoricalComponent
		}
	}
	s := &SQLHistoricalComponentStatement{observation: observations[0], observations: slices.Clone(observations), plan: p, statement: statement}
	s.self, s.seal = s, s.digest()
	if s.seal == "" {
		return nil, ErrSQLHistoricalComponent
	}
	return s, nil
}

// Pure merging never grants an observation or changes its sealed selectors.
// All duplicate physical rows must contain exactly the same original bytes.
func componentSQLUnionPlan(observations []*SQLHistoricalComponentObservation) (*SQLHistoricalBatchCASPlan, error) {
	if len(observations) == 0 || observations[0] == nil || observations[0].recipe == nil || observations[0].recipe.plan == nil {
		return nil, ErrSQLHistoricalComponent
	}
	first := observations[0]
	base := first.recipe.plan
	p := &SQLHistoricalBatchCASPlan{oldTransaction: base.oldTransaction, identity: base.identity, server: base.server, database: base.database, limits: base.limits, before: sqlHistoricalCASImage{rows: map[string][]historicalSQLRow{}, schema: map[string]string{}, columns: map[string][]string{}}}
	for _, o := range observations {
		if o == nil || o.recipe == nil || o.recipe.plan == nil || o.pool != first.pool || o.transaction != first.transaction || o.recipe.oldPool != first.recipe.oldPool {
			return nil, ErrSQLHistoricalComponent
		}
		r := o.recipe.plan
		if r.oldTransaction != base.oldTransaction || r.identity != base.identity || r.server != base.server || r.database != base.database || r.limits != base.limits {
			return nil, ErrSQLHistoricalComponent
		}
		p.request.AssessmentIDs = append(p.request.AssessmentIDs, r.request.AssessmentIDs...)
		p.request.AnswerSheetIDs = append(p.request.AnswerSheetIDs, r.request.AnswerSheetIDs...)
		slices.Sort(p.request.AssessmentIDs)
		slices.Sort(p.request.AnswerSheetIDs)
		p.request.AssessmentIDs, p.request.AnswerSheetIDs = slices.Compact(p.request.AssessmentIDs), slices.Compact(p.request.AnswerSheetIDs)
		if len(p.request.AssessmentIDs) > p.limits.MaxOwners || len(p.request.AnswerSheetIDs) > p.limits.MaxOwners {
			return nil, ErrSQLHistoricalComponent
		}
		if err := componentSQLUnionImage(&p.before, o.business); err != nil {
			return nil, err
		}
		var count int
		var size uint64
		for _, table := range batchBusinessTables {
			for _, row := range p.before.rows[table] {
				count++
				for _, value := range row {
					if value != nil {
						size += uint64(len(*value))
					}
				}
			}
		}
		if count > p.limits.MaxRows || size > p.limits.MaxBytes {
			return nil, ErrSQLHistoricalComponent
		}
	}
	// Match the original capture's empty result representation. Descendant
	// queries return nil when selected owners have no rows; without an owner,
	// capture does not query them and records explicit empty negative ranges.
	if len(p.before.rows["assessment"]) == 0 {
		for _, table := range batchBusinessTables {
			p.before.rows[table] = []historicalSQLRow{}
		}
	} else {
		for _, table := range []string{"runtime_checkpoint", "evaluation_outcome"} {
			if len(p.before.rows[table]) == 0 {
				p.before.rows[table] = nil
			}
		}
	}
	return p, nil
}

func componentSQLUnionImage(out *sqlHistoricalCASImage, original sqlHistoricalCASImage) error {
	cloned := casCloneImage(original)
	for key, value := range cloned.schema {
		if prior, ok := out.schema[key]; ok && prior != value {
			return ErrSQLHistoricalComponent
		}
		out.schema[key] = value
	}
	for key, value := range cloned.columns {
		if prior, ok := out.columns[key]; ok && !reflect.DeepEqual(prior, value) {
			return ErrSQLHistoricalComponent
		}
		out.columns[key] = value
	}
	for table, rows := range cloned.rows {
		if !slices.Contains(batchBusinessTables, table) {
			if prior, ok := out.rows[table]; ok && !reflect.DeepEqual(prior, rows) {
				return ErrSQLHistoricalComponent
			}
			out.rows[table] = rows
			continue
		}
		if _, ok := out.rows[table]; !ok {
			out.rows[table] = []historicalSQLRow{}
		}
		byID := map[uint64]historicalSQLRow{}
		for _, row := range out.rows[table] {
			id, err := sqlHistoricalUint(row, "id")
			if err != nil || id == 0 || byID[id] != nil {
				return ErrSQLHistoricalComponent
			}
			byID[id] = row
		}
		seen := map[uint64]bool{}
		for _, row := range rows {
			id, err := sqlHistoricalUint(row, "id")
			if err != nil || id == 0 || seen[id] {
				return ErrSQLHistoricalComponent
			}
			seen[id] = true
			if prior := byID[id]; prior != nil {
				if !reflect.DeepEqual(prior, row) {
					return ErrSQLHistoricalComponent
				}
			} else {
				byID[id] = row
				out.rows[table] = append(out.rows[table], row)
			}
		}
		sort.Slice(out.rows[table], func(i, j int) bool {
			a, _ := sqlHistoricalUint(out.rows[table][i], "id")
			b, _ := sqlHistoricalUint(out.rows[table][j], "id")
			return a < b
		})
	}
	return nil
}

func (s *SQLHistoricalComponentStatement) digest() string {
	if s == nil || s.observation == nil || s.statement == nil || s.plan == nil || s.statement.plan != s.plan || !s.observation.recipe.intact() {
		return ""
	}
	if len(s.observations) != 0 {
		if s.observations[0] != s.observation {
			return ""
		}
		baseline, err := componentSQLUnionPlan(s.observations)
		if err != nil || baseline.identity != s.plan.identity || baseline.server != s.plan.server || baseline.database != s.plan.database || baseline.oldTransaction != s.plan.oldTransaction || baseline.limits != s.plan.limits || !reflect.DeepEqual(baseline.request, s.plan.request) || !reflect.DeepEqual(baseline.before, s.plan.before) {
			return ""
		}
		parts := []string{"sql-component-union-statement/v1", casImageHash(s.statement.expected), strconv.FormatUint(s.statement.transaction.event, 10)}
		for _, o := range s.observations {
			if o.self != o || !o.used || !o.writable || o.seal == "" || o.seal != o.digest() || o.transaction != s.statement.transaction {
				return ""
			}
			parts = append(parts, o.seal)
		}
		raw, err := json.Marshal(componentPlanRecord(s.plan))
		if err != nil {
			return ""
		}
		parts = append(parts, string(raw))
		return cycleKeyDigest(parts)
	}
	base := s.observation.recipe.plan
	if s.plan.identity != base.identity || s.plan.server != base.server || s.plan.database != base.database || s.plan.oldTransaction != base.oldTransaction || !reflect.DeepEqual(s.plan.request, base.request) || s.plan.limits != base.limits || !reflect.DeepEqual(s.plan.before, s.observation.business) || s.statement.transaction != s.observation.transaction {
		return ""
	}
	return cycleKeyDigest([]string{s.observation.seal, casImageHash(s.statement.expected), strconv.FormatUint(s.statement.transaction.event, 10)})
}

// Statement facts are not a host commit response or cross-store permission.
func (s *SQLHistoricalComponentStatement) Report() SQLHistoricalBatchCASReport {
	if s == nil || s.self != s || s.seal == "" || s.seal != s.digest() {
		return SQLHistoricalBatchCASReport{HostCommitRequired: true, IndependentReadbackRequired: true}
	}
	return s.statement.Report()
}

// A different actual RR-RO transaction must read the committed server bytes.
// A rollback/partial/unknown write cannot be converted to expected JSON input.
// SQL readback alone deliberately never certifies the paired host commits.
func (s *SQLHistoricalComponentStatement) VerifyIndependentPersisted(ctx context.Context, budget time.Duration) (SQLHistoricalComponentReadReport, error) {
	r := SQLHistoricalComponentReadReport{FullSourcesRequired: true, MongoQualificationRequired: true, AIClosureRequired: true, HostCommitRequired: true}
	if ctx == nil || ctx.Err() != nil || s == nil || s.self != s || s.seal == "" || s.seal != s.digest() || budget <= 0 || budget > 20*time.Second {
		return r, ErrSQLHistoricalComponent
	}
	bounded, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	tx, err := historicalTx(bounded)
	if err != nil {
		return r, ErrSQLHistoricalComponent
	}
	observations := s.observations
	if len(observations) == 0 {
		observations = []*SQLHistoricalComponentObservation{s.observation}
	}
	for _, o := range observations {
		if !o.recipe.intact() || tx.Statement.ConnPool == o.pool || tx.Statement.ConnPool == o.recipe.oldPool || sqlHistoricalEndedPool(bounded, o.pool) != nil {
			return r, ErrSQLHistoricalComponent
		}
	}
	server, database, err := historicalDatabase(tx)
	p := s.plan
	if err != nil || server != p.server || database != p.database {
		return r, ErrSQLHistoricalComponent
	}
	image, err := p.capture(tx, false)
	if err != nil || !reflect.DeepEqual(image, s.statement.expected) {
		return r, ErrSQLHistoricalComponent
	}
	actual, err := cycleActualTransaction(tx)
	if err != nil || actual == s.statement.transaction || actual == p.oldTransaction {
		return r, ErrSQLHistoricalComponent
	}
	// Every original negative range/replay expansion is re-read. The merged
	// business image does not stand in for any fragment's responsibility scope.
	for _, o := range observations {
		scoped, anchors, _, err := o.recipe.captureResponsibility(tx)
		if err != nil || !reflect.DeepEqual(scoped, o.recipe.responsibility) || !reflect.DeepEqual(anchors, o.recipe.anchors) {
			return r, ErrSQLHistoricalComponent
		}
	}
	after, err := cycleActualTransaction(tx)
	if err != nil || after != actual || bounded.Err() != nil {
		return r, ErrSQLHistoricalComponent
	}
	r.BusinessMatched, r.ResponsibilitiesMatched, r.IndependentPersistedReadMatched = true, true, true
	return r, nil
}
