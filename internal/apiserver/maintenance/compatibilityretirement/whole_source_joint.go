package retirement

import (
	"context"
	"encoding/hex"
	"errors"
	"reflect"
	"sort"
	"strconv"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"go.mongodb.org/mongo-driver/mongo"
)

// Only original source capabilities, real owner batches and complete current
// cycles create this reader. The public report never authorizes a write.
type WholeSourceJointPage struct {
	owner                          *HistoricalCoordinator
	page                           *HistoricalSourcePage
	index                          *WholeSourceJointIndex
	catalog                        *SQLCrossStoreResponsibilityCatalog
	sql                            *SQLBusinessOwnerBatch
	mongo                          *MongoHistoricalOwnerBatch
	cross                          *SQLMongoCrossStoreResponsibilityPage
	sources                        []*VerifiedSourceEvent
	current                        []*VerifiedSourceEvent
	resolved                       map[string]sqlevaluation.SQLResponsibilityObservation
	bindings                       []WholeSourceJointObservationBinding
	values                         map[verifiedSourceKey]HistoricalCandidate
	candidateStart, candidateCount int
	candidateSHA, bindingSHA       string
	consumed                       bool
}

type WholeSourceJointObservationBinding struct {
	ObservationKey, OriginalObservationRowSHA256                                          string
	OriginalID, EventType, OrganizationID                                                 string
	SourcePrimaryKeySHA256, SourceRowSHA256, SourceFactsSHA256                            string
	LegacyContentSHA256, InnerWireDataSHA256, SDKFingerprintSHA256, BusinessBindingSHA256 string
	ActualOriginalRun                                                                     *evidence.HistoricalRunReferenceV1
}

type WholeSourceJointPageSummary struct {
	WholeFourSourceCoverageBound                                                                                                                                                 bool
	RelatedOriginalEvents, ConsumedOriginalEvents, ResolvedOriginalObservations                                                                                                  uint64
	SourceIndexSHA256, CandidateSHA256, ObservationBindingSHA256                                                                                                                 string
	SQLCycleID, SQLGlobalLedgersSHA256, SQLFullRowsSHA256, SQLBusinessSHA256, MongoSnapshotSHA256, MongoMetadataSHA256, MongoBusinessSHA256, ActualMongoSessionTransactionSHA256 string
	ExternalOriginRequired, AIInboxRequired, GlobalUnboundRequired, FinalFreshRequired, WriterFenceRequired, CASRequired, DropReady                                              bool
}

func (*WholeSourceJointPage) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*WholeSourceJointPage) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*WholeSourceJointPage) String() string {
	return "private whole-source joint responsibility reader; no write authorization"
}
func (p *WholeSourceJointPage) GoString() string { return p.String() }

// This is the necessary current responsibility closure for one reversible
// evidence component, not a whole-ledger scan or a writer/DROP fence. Both
// callers must still borrow the same genuine SQL/Mongo scopes before effects.
func validateHistoricalComponentResponsibilityClosure(parent context.Context, o *HistoricalComponentObservation) error {
	if o.ValidateBorrowedObservation(parent) != nil {
		return ErrCoordinatorCASQualification
	}
	scope, cancel := context.WithDeadline(parent, o.expires)
	defer cancel()
	ctx := mongo.NewSessionContext(scope, mongo.SessionFromContext(parent))
	current, err := qualifiedHistoricalComponentBusinessRows(ctx, o.source)
	if err != nil || historicalComponentResponsibilityRowsMatch(o.rows, current) != nil {
		return ErrCoordinatorCASQualification
	}
	indexes, err := historicalComponentMongoIndexes(o.source)
	if err != nil || historicalComponentSelectedMongoTerminal(indexes) != nil {
		return ErrCoordinatorCASQualification
	}
	if err = historicalComponentSQLMongoResponsibilities(ctx, o.source, current, indexes); err != nil {
		return err
	}
	return o.ValidateBorrowedObservation(ctx)
}

func historicalComponentResponsibilityRowsMatch(before, current []qualifiedCASRow) error {
	if len(before) == 0 || len(before) != len(current) {
		return ErrCoordinatorCASQualification
	}
	seen := map[string]qualifiedCASRow{}
	for _, row := range before {
		if row.facts == nil || qualifiedCASSourceMatches(row.facts, row.candidate) != nil || !row.candidate.LocalQualified || len(row.candidate.BlockingReasons) != 0 || row.bindingSHA == "" {
			return ErrCoordinatorCASQualification
		}
		if _, exists := seen[row.facts.EventID]; exists {
			return ErrCoordinatorCASQualification
		}
		seen[row.facts.EventID] = row
	}
	for _, row := range current {
		if row.facts == nil || qualifiedCASSourceMatches(row.facts, row.candidate) != nil || !row.candidate.LocalQualified || len(row.candidate.BlockingReasons) != 0 {
			return ErrCoordinatorCASQualification
		}
		prior, exists := seen[row.facts.EventID]
		if !exists || prior.sourceObservation != row.sourceObservation || prior.bindingSHA != row.bindingSHA || !reflect.DeepEqual(prior.facts, row.facts) || !reflect.DeepEqual(prior.candidate.HistoricalGaps, row.candidate.HistoricalGaps) || !reflect.DeepEqual(prior.candidate.ActualOriginalRun, row.candidate.ActualOriginalRun) || !reflect.DeepEqual(prior.candidate.AuthorizationRun, row.candidate.AuthorizationRun) || !reflect.DeepEqual(prior.candidate.ExecutionRun, row.candidate.ExecutionRun) {
			return ErrCoordinatorCASQualification
		}
		delete(seen, row.facts.EventID)
	}
	if len(seen) != 0 {
		return ErrCoordinatorCASQualification
	}
	return nil
}
func (WholeSourceJointObservationBinding) MarshalJSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}
func (WholeSourceJointObservationBinding) MarshalBSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}

// initialSQL supplies the actual current source owners only. Related events
// are expanded from the complete source index and the original owner links,
// then all original graphs are reread in bounded batches in this same epoch.
func (c *HistoricalCoordinator) PrepareWholeSourceJointPage(ctx context.Context, page *HistoricalSourcePage, index *WholeSourceJointIndex, catalog *SQLCrossStoreResponsibilityCatalog, initialSQL *SQLBusinessOwnerBatch, global *MongoResponsibilitySnapshot) (*WholeSourceJointPage, error) {
	if c == nil {
		return nil, ErrWholeSourceJoint
	}
	c.mu.Lock()
	if err := c.alive(ctx); err != nil {
		c.mu.Unlock()
		return nil, err
	}
	if err := c.pageValid(page); err != nil {
		c.mu.Unlock()
		return nil, err
	}
	var current []*VerifiedSourceEvent
	for _, row := range page.rows {
		if row.event == nil || len(row.keys) != 1 || !c.remaining[row.keys[0]] {
			c.mu.Unlock()
			return nil, ErrCoordinatorPage
		}
		current = append(current, row.event)
	}
	c.mu.Unlock()
	p, err := prepareWholeSourceJointPage(ctx, c, page, index, catalog, initialSQL, global, current)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err = c.pageValid(page); err != nil {
		return nil, err
	}
	return p, nil
}

func prepareWholeSourceJointPage(ctx context.Context, c *HistoricalCoordinator, page *HistoricalSourcePage, index *WholeSourceJointIndex, catalog *SQLCrossStoreResponsibilityCatalog, initialSQL *SQLBusinessOwnerBatch, global *MongoResponsibilitySnapshot, current []*VerifiedSourceEvent) (*WholeSourceJointPage, error) {
	if ctx == nil || index == nil || !index.complete || index.owner != c || index.auth != c.authenticated || catalog == nil || !catalog.Report().Complete || initialSQL == nil || initialSQL.responsibility != catalog.current || global == nil || !global.Report().Complete || len(current) == 0 {
		return nil, ErrWholeSourceJoint
	}
	if err := initialSQL.ValidateBorrowedSnapshot(ctx); err != nil {
		return nil, err
	}
	if err := global.ValidateBorrowedSnapshot(ctx); err != nil {
		return nil, err
	}
	ids := map[string]bool{}
	for _, handle := range current {
		f, err := handle.Facts()
		if err != nil {
			return nil, err
		}
		hash, err := privateFactsSHA(f)
		entry, exists := index.entries[f.EventID]
		if err != nil || !exists || hash != entry.FactsSHA256 {
			return nil, ErrSourceAuthentication
		}
		ids[f.EventID] = true
		var actual *sqlevaluation.SQLHistoricalBatchOwnerFacts
		if f.Submitted != nil {
			actual, err = initialSQL.OwnerByAnswerSheet(entry.AnswerSheetID)
		} else {
			actual, err = initialSQL.OwnerByAssessment(entry.AssessmentID)
		}
		if errors.Is(err, sqlevaluation.ErrSQLHistoricalOwnerAbsent) && f.Submitted != nil {
			for _, id := range index.bySheet[entry.AnswerSheetID] {
				ids[id] = true
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		owner := actual.Snapshot().Owner
		if owner.OrgID != f.OrgID {
			return nil, ErrSourceOrganization
		}
		for _, id := range index.byAssessment[owner.AssessmentID] {
			ids[id] = true
		}
		for _, id := range index.bySheet[owner.AnswerSheetID] {
			ids[id] = true
		}
	}
	if len(ids) > index.limits.MaxRelatedSources {
		return nil, ErrWholeSourceJointBounds
	}
	ordered := make([]string, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)
	sources := make([]*VerifiedSourceEvent, 0, len(ordered))
	for _, id := range ordered {
		source, err := index.event(ctx, id)
		if err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	request, err := MongoHistoricalSQLBatchSelectors(sources)
	if err != nil {
		return nil, err
	}
	sql, err := PrepareSQLBusinessOwnerBatch(ctx, catalog.current, request, sqlevaluation.DefaultSQLHistoricalOwnerBatchLimits())
	if err != nil {
		return nil, err
	}
	ml := DefaultMongoHistoricalOwnerBatchLimits()
	ml.MaxSources = index.limits.MaxRelatedSources
	mongo, err := PrepareMongoHistoricalOwnerBatch(ctx, global, sql.facts, sources, ml)
	if err != nil {
		return nil, err
	}
	p := &WholeSourceJointPage{owner: c, page: page, index: index, catalog: catalog, sql: sql, mongo: mongo, sources: sources, current: append([]*VerifiedSourceEvent(nil), current...), resolved: map[string]sqlevaluation.SQLResponsibilityObservation{}, values: map[verifiedSourceKey]HistoricalCandidate{}}
	if err = p.prepareCrossRows(ctx); err != nil {
		return nil, err
	}
	if err = p.proveOriginalMongoOwners(ctx); err != nil {
		return nil, err
	}
	for _, source := range sources {
		_, key, err := mongo.source(source)
		if err != nil {
			return nil, err
		}
		// The owner batch keeps a compact baseline without BusinessIDs and
		// resolver gaps. Candidates require the authenticated source's full
		// defensive facts, as the other coordinator qualification paths do.
		facts, err := source.Facts()
		if err != nil {
			return nil, err
		}
		candidate := coordinatorEventCandidate(facts)
		if facts.Source.Database == "mysql" {
			local, e := p.resolveSQL(ctx, facts)
			if e != nil {
				candidate.BlockingReasons = append(candidate.BlockingReasons, "sql_original_business_or_responsibility_rejected")
			} else {
				candidate.LocalQualified = local.OwnerLocalTerminal && len(local.BlockingReasons) == 0
				candidate.HistoricalGaps = append(candidate.HistoricalGaps, local.Gaps...)
				candidate.BlockingReasons = append(candidate.BlockingReasons, local.BlockingReasons...)
				candidate.ActualOriginalRun = local.OriginalRun
				candidate.AuthorizationRun = local.AuthorizationRun
				candidate.ExecutionRun = local.ExecutionRun
				candidate.BusinessBaselineSHA256 = sql.Report().BusinessRowsSHA256
			}
			view, e := mongo.ResponsibilitiesForSource(ctx, source)
			if e != nil {
				candidate.BlockingReasons = append(candidate.BlockingReasons, "mongo_original_downstream_or_responsibility_rejected")
			} else {
				candidate.BlockingReasons = append(candidate.BlockingReasons, view.BlockingReasons...)
			}
			candidate.RequiredAdapters = append(candidate.RequiredAdapters, "sql_stable_original_business_binding_writer_adapter")
		} else {
			q := p.cross.qualification[key]
			if q == nil {
				return nil, ErrWholeSourceJoint
			}
			local := q.Local()
			view, err := p.jointMongoSQLView(ctx, facts, local)
			if err != nil {
				return nil, err
			}
			candidate.LocalQualified = local.OwnerLocalTerminal && len(local.BlockingReasons) == 0 && len(view.BlockingReasons) == 0
			appendHistoricalComponentMongoGaps(&candidate, local.Gaps)
			candidate.HistoricalGaps = append(candidate.HistoricalGaps, view.Gaps...)
			candidate.BlockingReasons = append(candidate.BlockingReasons, local.BlockingReasons...)
			candidate.BlockingReasons = append(candidate.BlockingReasons, view.BlockingReasons...)
			candidate.BusinessBindingSHA256 = local.BusinessBindingSHA256
			candidate.BusinessBaselineSHA256 = mongo.Report().BusinessRowsSHA256
			candidate.ActualOriginalRun = local.OriginalRun
			candidate.RequiredAdapters = append(candidate.RequiredAdapters, "mongo_original_types_remaining_ai_and_inbox_closure")
		}
		if sql.responsibility.Report().Unknown > 0 || sql.responsibility.Report().Blocking > 0 {
			candidate.BlockingReasons = append(candidate.BlockingReasons, "global_sql_orphan_unknown_or_conflicting_responsibility")
		}
		candidate.BlockingReasons = append(candidate.BlockingReasons, global.Report().BlockingReasons...)
		candidate.RequiredAdapters = append(candidate.RequiredAdapters, global.Report().CoverageGaps...)
		candidate.RequiredAdapters = append(candidate.RequiredAdapters, "complete_global_unbound_observation_reconciliation", "external_original_source_provenance", "actual_joint_fresh_epoch", "writer_fence", "historical_evidence_cas")
		coordinatorClassify(&candidate)
		p.values[key] = candidate
	}
	if err = p.ValidateBorrowedSnapshot(ctx); err != nil {
		return nil, err
	}
	h, err := wholeSourceJointBindingsHash(p.bindings)
	if err != nil {
		return nil, err
	}
	p.bindingSHA = h
	return p, nil
}

func (p *WholeSourceJointPage) prepareCrossRows(ctx context.Context) error {
	selectors := sqlevaluation.SQLCrossStoreSelectors{}
	ass, orgs := map[uint64]bool{}, map[uint64]bool{}
	owners := map[sqlevaluation.SQLCrossStoreOwnerReference]bool{}
	cross := &SQLMongoCrossStoreResponsibilityPage{catalog: p.catalog, sql: p.sql, mongo: p.mongo, qualification: map[verifiedSourceKey]*MongoHistoricalBatchOwnerQualification{}, views: map[verifiedSourceKey]SQLMongoCrossStoreResponsibilityView{}, resolvedOwner: map[verifiedSourceKey][]string{}}
	for _, handle := range p.sources {
		f, key, err := p.mongo.source(handle)
		if err != nil {
			return err
		}
		selectors.EventIDs = append(selectors.EventIDs, f.EventID)
		orgs[f.OrgID] = true
		if actual := p.mongo.sqlOwners[key]; actual != nil {
			o := actual.Snapshot().Owner
			ass[o.AssessmentID] = true
			if o.AnswerSheetID != 0 {
				owners[sqlevaluation.SQLCrossStoreOwnerReference{Kind: "AnswerSheet", ID: strconv.FormatUint(o.AnswerSheetID, 10)}] = true
			}
		}
		if f.Source.Database == "mongodb" {
			q, err := p.mongo.ResolveSource(ctx, handle)
			if err != nil {
				return err
			}
			cross.qualification[key] = q
			cross.sources = append(cross.sources, handle)
			l := q.Local()
			if l.AnswerSheetID != 0 {
				owners[sqlevaluation.SQLCrossStoreOwnerReference{Kind: "AnswerSheet", ID: strconv.FormatUint(l.AnswerSheetID, 10)}] = true
			}
			if l.GenerationID != 0 {
				owners[sqlevaluation.SQLCrossStoreOwnerReference{Kind: "ReportGeneration", ID: strconv.FormatUint(l.GenerationID, 10)}] = true
			}
		}
	}
	selectors.AssessmentIDs = mongoBatchIDs(ass)
	selectors.OrganizationIDs = mongoBatchIDs(orgs)
	for owner := range owners {
		selectors.MongoOwners = append(selectors.MongoOwners, owner)
	}
	page, err := sqlevaluation.PrepareSQLHistoricalCrossStorePage(ctx, p.catalog.index, p.sql.facts, selectors)
	if err != nil {
		return err
	}
	cross.page = page
	p.cross = cross
	return nil
}

func (p *WholeSourceJointPage) ValidateBorrowedSnapshot(ctx context.Context) error {
	if p == nil || p.index == nil || !p.index.complete || p.owner == nil || p.index.owner != p.owner || p.index.auth != p.owner.authenticated || p.sql == nil || p.mongo == nil || p.cross == nil || p.cross.page == nil || p.sql.responsibility != p.catalog.current || p.mongo.sql != p.sql.facts || p.cross.sql != p.sql || p.cross.mongo != p.mongo {
		return ErrWholeSourceJoint
	}
	if err := p.sql.ValidateBorrowedSnapshot(ctx); err != nil {
		return err
	}
	return p.cross.ValidateBorrowedSnapshot(ctx)
}

// Pure bounded cache access: no SQL queries. Actual host snapshot checks remain
// mandatory at page boundaries, and an actual new epoch is required for fresh.
func (p *WholeSourceJointPage) Candidate(ctx context.Context, handle *VerifiedSourceEvent) (HistoricalCandidate, error) {
	if p == nil || p.mongo == nil {
		return HistoricalCandidate{}, ErrWholeSourceJoint
	}
	if err := p.mongo.validateReaderContext(ctx); err != nil {
		return HistoricalCandidate{}, err
	}
	_, key, err := p.mongo.source(handle)
	if err != nil {
		return HistoricalCandidate{}, err
	}
	v, ok := p.values[key]
	if !ok {
		return HistoricalCandidate{}, ErrWholeSourceJoint
	}
	return coordinatorCloneCandidate(v), nil
}

func (c *HistoricalCoordinator) QualifyWholeSourceJointPage(ctx context.Context, page *HistoricalSourcePage, joint *WholeSourceJointPage) error {
	if c == nil {
		return ErrWholeSourceJoint
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.alive(ctx); err != nil {
		return err
	}
	if err := c.pageValid(page); err != nil {
		return err
	}
	if joint == nil || joint.owner != c || joint.page != page || joint.consumed || len(joint.current) != len(page.rows) {
		return ErrWholeSourceJoint
	}
	if err := joint.ValidateBorrowedSnapshot(ctx); err != nil {
		return err
	}
	rows := make([]HistoricalCandidate, 0, len(page.rows))
	for i, row := range page.rows {
		if row.event != joint.current[i] || len(row.keys) != 1 {
			return ErrCoordinatorPage
		}
		f, err := row.event.Facts()
		if err != nil {
			return err
		}
		observed, err := joint.index.event(ctx, f.EventID)
		if err != nil {
			return err
		}
		observedFacts, err := observed.Facts()
		if err != nil {
			return err
		}
		hash, err := privateFactsSHA(observedFacts)
		if err != nil || hash != c.authenticated.rows[row.keys[0]].facts {
			return ErrSourceAuthentication
		}
		candidate, err := joint.Candidate(ctx, row.event)
		if err != nil {
			return err
		}
		rows = append(rows, candidate)
	}
	if err := joint.ValidateBorrowedSnapshot(ctx); err != nil {
		return err
	}
	joint.candidateStart = len(c.candidates)
	joint.candidateCount = len(rows)
	joint.candidateSHA = coordinatorCandidateHash(rows)
	if err := c.consume(page, rows, HistoricalCoordinatorPageReceipt{SQLBusinessBaselineSHA256: joint.sql.Report().BusinessRowsSHA256, MongoBusinessBaselineSHA256: joint.mongo.Report().BusinessRowsSHA256}); err != nil {
		return err
	}
	joint.consumed = true
	return nil
}

func (p *WholeSourceJointPage) Summary() WholeSourceJointPageSummary {
	s := WholeSourceJointPageSummary{ExternalOriginRequired: true, AIInboxRequired: true, GlobalUnboundRequired: true, FinalFreshRequired: true, WriterFenceRequired: true, CASRequired: true}
	if p == nil || p.owner == nil || p.index == nil {
		return s
	}
	c := p.owner
	c.mu.Lock()
	defer c.mu.Unlock()
	s.RelatedOriginalEvents = uint64(len(p.sources))
	s.ResolvedOriginalObservations = uint64(len(p.resolved))
	s.SourceIndexSHA256 = p.index.indexSHA
	s.SQLCycleID = p.sql.responsibility.Report().CycleID
	s.SQLFullRowsSHA256 = p.cross.Report().RowsSHA256
	s.SQLBusinessSHA256 = p.sql.Report().BusinessRowsSHA256
	s.MongoSnapshotSHA256 = p.mongo.global.Report().SnapshotSHA256
	s.MongoBusinessSHA256 = p.mongo.Report().BusinessRowsSHA256
	ledgerHash, err := privateFactsSHA(p.sql.responsibility.Report().Ledgers)
	if err != nil {
		return s
	}
	s.SQLGlobalLedgersSHA256 = hex.EncodeToString(ledgerHash[:])
	s.MongoMetadataSHA256 = p.mongo.global.Report().MetadataSHA256
	s.ActualMongoSessionTransactionSHA256 = mongoOwnerHashParts("actual-mongo-session-txn/v1", string(p.mongo.global.txn.session), strconv.FormatInt(p.mongo.global.txn.number, 10))
	if !c.coverage || !p.consumed || c.failed || c.now().Sub(c.started) > c.limits.MaxDuration {
		return s
	}
	bindingsSHA, err := wholeSourceJointBindingsHash(p.bindings)
	if err != nil || bindingsSHA != p.bindingSHA {
		return s
	}
	if p.candidateStart < 0 || p.candidateCount <= 0 || p.candidateStart+p.candidateCount > len(c.candidates) || !coordinatorStoredCandidateMatches(c.candidates[p.candidateStart:p.candidateStart+p.candidateCount], p.candidateSHA) {
		return s
	}
	s.WholeFourSourceCoverageBound = true
	s.ConsumedOriginalEvents = uint64(p.candidateCount)
	s.CandidateSHA256 = p.candidateSHA
	s.ObservationBindingSHA256 = p.bindingSHA
	return s
}

// Only a body-free journal escapes after the coordinator's real own fourth
// source EOF. Defensive copies cannot mutate the original proof set.
func (p *WholeSourceJointPage) ObservationBindingRange(offset, limit int) ([]WholeSourceJointObservationBinding, error) {
	if p == nil || !p.Summary().WholeFourSourceCoverageBound {
		return nil, ErrCoordinatorIncomplete
	}
	if offset < 0 || limit < 1 || limit > 512 || offset > len(p.bindings) {
		return nil, ErrCoordinatorBounds
	}
	end := offset + limit
	if end > len(p.bindings) {
		end = len(p.bindings)
	}
	out := append([]WholeSourceJointObservationBinding(nil), p.bindings[offset:end]...)
	for i := range out {
		if out[i].ActualOriginalRun != nil {
			run := *out[i].ActualOriginalRun
			out[i].ActualOriginalRun = &run
		}
	}
	return out, nil
}

// This gate does not normalize any old state. Both databases must already be
// in genuinely new host snapshots. It covers full current cycles, original
// graphs and an additional clean four-copy EOF before comparing qualification.
func (p *WholeSourceJointPage) RecheckFresh(ctx context.Context, copies []WholeSourceJointCopy, catalog *SQLCrossStoreResponsibilityCatalog, sql *SQLBusinessOwnerBatch, global *MongoResponsibilitySnapshot) error {
	if p == nil || catalog == nil || sql == nil || global == nil || catalog == p.catalog || catalog.Report().CycleID == p.catalog.Report().CycleID || global == p.mongo.global {
		return ErrWholeSourceJoint
	}
	if err := p.index.RecheckSourceCopies(ctx, copies); err != nil {
		return err
	}
	if _, err := p.sql.responsibility.RecheckFresh(ctx); err != nil {
		return err
	}
	if _, err := p.mongo.global.RecheckFresh(ctx); err != nil {
		return err
	}
	if err := p.sql.RecheckBusiness(ctx, catalog.current); err != nil {
		return err
	}
	fresh, err := prepareWholeSourceJointPage(ctx, p.owner, p.page, p.index, catalog, sql, global, p.current)
	if err != nil {
		return err
	}
	if err := p.mongo.RecheckBusiness(ctx, global, fresh.sql.facts); err != nil {
		return err
	}
	if p.bindingSHA != fresh.bindingSHA || !reflect.DeepEqual(p.values, fresh.values) || p.cross.Report().RowsSHA256 != fresh.cross.Report().RowsSHA256 {
		return ErrSQLMongoCrossStoreConflict
	}
	return nil
}
