package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

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
	self        *SQLHistoricalComponentObservation
	recipe      *SQLHistoricalComponentRecipe
	pool        gorm.ConnPool
	transaction sqlResponsibilityTransaction
	expires     time.Time
	writable    bool
	used        bool
	seal        string
}

type SQLHistoricalComponentStatement struct {
	self        *SQLHistoricalComponentStatement
	observation *SQLHistoricalComponentObservation
	statement   *SQLHistoricalBatchCASStatement
	seal        string
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
			if _, err = original.OwnerByAnswerSheet(sheet); !errors.Is(err, ErrSQLHistoricalOwnerAbsent) {
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
func (r *SQLHistoricalComponentRecipe) captureResponsibility(tx *gorm.DB) (sqlHistoricalCASImage, map[string]string, error) {
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
			return image, nil, ErrSQLHistoricalComponent
		}
		image.schema[spec.name], image.columns[spec.name] = hash, cols
		retained[spec.name] = map[string]historicalSQLRow{}
	}
	for round := 0; round <= 512; round++ {
		if time.Since(started) > r.limits.MaxDuration {
			return image, nil, ErrSQLHistoricalComponent
		}
		beforeEvents, beforePairs := len(events), len(pairs)
		ids := make([]string, 0, len(events))
		for id := range events {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		if len(ids) > 512 || len(pairs) > 512 || len(ids) == 0 && len(r.selectors.AssessmentIDs)+len(r.selectors.MongoOwners)+len(r.selectors.OrganizationIDs) == 0 {
			return image, nil, ErrSQLHistoricalComponent
		}
		for _, spec := range sqlResponsibilityTables {
			predicate, args, err := componentPredicate(spec, r.selectors, ids)
			if err != nil {
				return image, nil, err
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
				return image, nil, err
			}
			if !reflect.DeepEqual(cols, image.columns[spec.name]) || size > r.limits.MaxPageBytes {
				return image, nil, ErrSQLHistoricalComponent
			}
			for _, row := range rows {
				key, err := cycleKey(spec, row)
				if err != nil {
					return image, nil, err
				}
				digest := cycleKeyDigest(key)
				if previous := retained[spec.name][digest]; previous != nil {
					if !reflect.DeepEqual(previous, row) {
						return image, nil, ErrSQLHistoricalComponent
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
					return image, nil, ErrSQLHistoricalComponent
				}
				retained[spec.name][digest] = row
				v := cycleDecode(spec.name, row)
				if v.EventID != "" {
					events[v.EventID] = true
				}
				if spec.name == "qs_rm_replay_items" || spec.name == "qs_rm_replay_requests" {
					if v.OrgID == 0 || v.link.requestID == "" {
						return image, nil, ErrSQLHistoricalComponent
					}
					pairs[cyclePair(v.OrgID, v.link.requestID)] = v
				}
			}
		}
		if len(events) == beforeEvents && len(pairs) == beforePairs {
			break
		}
		if round == 512 {
			return image, nil, ErrSQLHistoricalComponent
		}
	}
	for _, spec := range sqlResponsibilityTables {
		for _, row := range retained[spec.name] {
			image.rows[spec.name] = append(image.rows[spec.name], row)
		}
	}
	anchors, err := componentValidateResponsibility(tx, image)
	return image, anchors, err
}

func componentValidateResponsibility(tx *gorm.DB, image sqlHistoricalCASImage) (map[string]string, error) {
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
			return nil, ErrSQLHistoricalComponent
		}
	}
	if err := c.checkPageOwners(tx, 0); err != nil {
		return nil, err
	}
	c.checkReverse()
	for _, v := range c.observations {
		if v.Invalid || v.ScopeClass == "retirement_related" && (v.Unfinished || v.LeasePresent) {
			return nil, ErrSQLHistoricalComponent
		}
	}
	return c.anchorDigests, nil
}

func (o *SQLHistoricalComponentObservation) digest() string {
	if o == nil || !o.recipe.intact() {
		return ""
	}
	return cycleKeyDigest([]string{o.recipe.seal, sqlHistoricalProvenancePoolToken(o.pool), strconv.FormatUint(o.transaction.connection, 10), strconv.FormatUint(o.transaction.thread, 10), strconv.FormatUint(o.transaction.event, 10), o.expires.UTC().Format(time.RFC3339Nano), strconv.FormatBool(o.writable)})
}
func (o *SQLHistoricalComponentObservation) live(ctx context.Context) error {
	if ctx == nil || ctx.Err() != nil || o == nil || o.self != o || o.seal == "" || o.seal != o.digest() || time.Now().After(o.expires) {
		return ErrSQLHistoricalComponent
	}
	tx, err := historicalTx(ctx)
	if err != nil || tx.Statement.ConnPool != o.pool {
		return ErrSQLHistoricalComponent
	}
	var actual sqlResponsibilityTransaction
	if o.writable {
		actual, err = casActualRW(tx)
	} else {
		actual, err = cycleActualTransaction(tx)
	}
	if err != nil || actual != o.transaction {
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
	responsibility, anchors, err := r.captureResponsibility(tx)
	if err != nil {
		return nil, fmt.Errorf("%w: responsibility_read", ErrSQLHistoricalComponent)
	}
	if !reflect.DeepEqual(responsibility, r.responsibility) {
		return nil, fmt.Errorf("%w: responsibility_baseline", ErrSQLHistoricalComponent)
	}
	if !reflect.DeepEqual(anchors, r.anchors) {
		return nil, fmt.Errorf("%w: responsibility_anchors", ErrSQLHistoricalComponent)
	}
	o := &SQLHistoricalComponentObservation{recipe: r, pool: tx.Statement.ConnPool, transaction: actual, expires: started.Add(budget), writable: writable}
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

// The physical native CAS primitive is private. Frozen input and physical
// observations cannot reach a public production write API. A real fresh
// source/Mongo/AI-qualified aggregate must be composed before a public caller
// is added. Native regressions exercise this primitive without granting one.
func (o *SQLHistoricalComponentObservation) apply(ctx context.Context) (*SQLHistoricalComponentStatement, error) {
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
	s := &SQLHistoricalComponentStatement{observation: o, statement: statement}
	s.self = s
	s.seal = s.digest()
	return s, nil
}
func (s *SQLHistoricalComponentStatement) digest() string {
	if s == nil || s.observation == nil || s.statement == nil || s.statement.plan != s.observation.recipe.plan {
		return ""
	}
	return cycleKeyDigest([]string{s.observation.seal, casImageHash(s.statement.expected), strconv.FormatUint(s.statement.transaction.event, 10)})
}

// A different actual RR-RO transaction must read the committed server bytes.
// A rollback/partial/unknown write cannot be converted to expected JSON input.
// SQL readback alone deliberately never certifies the paired host commits.
func (s *SQLHistoricalComponentStatement) VerifyIndependentPersisted(ctx context.Context, budget time.Duration) (SQLHistoricalComponentReadReport, error) {
	r := SQLHistoricalComponentReadReport{FullSourcesRequired: true, MongoQualificationRequired: true, AIClosureRequired: true, HostCommitRequired: true}
	if ctx == nil || ctx.Err() != nil || s == nil || s.self != s || s.seal == "" || s.seal != s.digest() || !s.observation.recipe.intact() || budget <= 0 || budget > 20*time.Second {
		return r, ErrSQLHistoricalComponent
	}
	bounded, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	tx, err := historicalTx(bounded)
	if err != nil || tx.Statement.ConnPool == s.observation.pool || tx.Statement.ConnPool == s.observation.recipe.oldPool || sqlHistoricalEndedPool(bounded, s.observation.pool) != nil {
		return r, ErrSQLHistoricalComponent
	}
	server, database, err := historicalDatabase(tx)
	p := s.observation.recipe.plan
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
	scoped, anchors, err := s.observation.recipe.captureResponsibility(tx)
	if err != nil || !reflect.DeepEqual(scoped, s.observation.recipe.responsibility) || !reflect.DeepEqual(anchors, s.observation.recipe.anchors) {
		return r, ErrSQLHistoricalComponent
	}
	after, err := cycleActualTransaction(tx)
	if err != nil || after != actual || bounded.Err() != nil {
		return r, ErrSQLHistoricalComponent
	}
	r.BusinessMatched, r.ResponsibilitiesMatched, r.IndependentPersistedReadMatched = true, true, true
	return r, nil
}
