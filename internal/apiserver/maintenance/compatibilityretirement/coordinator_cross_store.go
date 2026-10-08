package retirement

import (
	"context"
	"encoding/hex"
	"slices"
	"strconv"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
)

// This journal is minted only by consuming the coordinator's own source page.
// It preserves original observation keys and opaque business qualification;
// it is not EventEvidence or permission to write, replay, or DROP anything.
type HistoricalCoordinatorCrossStorePage struct {
	owner          *HistoricalCoordinator
	page           *HistoricalSourcePage
	cross          *SQLMongoCrossStoreResponsibilityPage
	candidateStart int
	candidateCount int
	candidateSHA   string
	bindings       []HistoricalCrossStoreObservationBinding
	bindingSHA     string
}

type HistoricalCrossStoreObservationBinding struct {
	Source                                                                  evidence.HistoricalSourceReferenceV1
	OriginalID, EventType, OrganizationID, SourceFactsSHA256                string
	LegacyContentSHA256, BusinessBindingSHA256, MongoBusinessBaselineSHA256 string
	SQLBusinessBaselineSHA256, SQLFullRowBaselineSHA256, SQLCycleID         string
	MongoSnapshotSHA256, ActualMongoSessionTransactionSHA256                string
	OwnerBoundObservationKeys, ProvisionalOwnerResolvedObservationKeys      []string
	OriginalObservationRowSHA256                                            map[string]string
	CurrentStandardReferences                                               []evidence.StandardReference
	ActualOriginalRun                                                       *evidence.HistoricalRunReferenceV1
}

func (*HistoricalCoordinatorCrossStorePage) MarshalJSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}
func (*HistoricalCoordinatorCrossStorePage) MarshalBSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}
func (*HistoricalCoordinatorCrossStorePage) String() string {
	return "private consumed cross-store page; joint closure unproven"
}
func (p *HistoricalCoordinatorCrossStorePage) GoString() string { return p.String() }
func (HistoricalCrossStoreObservationBinding) MarshalJSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}
func (HistoricalCrossStoreObservationBinding) MarshalBSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}
func (HistoricalCrossStoreObservationBinding) String() string {
	return "private original source and cross-store observation binding"
}
func (v HistoricalCrossStoreObservationBinding) GoString() string { return v.String() }

type HistoricalCrossStorePageSummary struct {
	WholeFourSourceCoverageBound                                                                                                    bool
	SourcePageSequence, MongoCandidates, OwnerBoundObservations, ProvisionalOwnersResolved                                          uint64
	CandidateSHA256, ObservationBindingSHA256                                                                                       string
	ExternalOriginRequired, AIInboxRequired, GlobalUnboundRequired, FinalFreshRequired, WriterFenceRequired, CASRequired, DropReady bool
}

// QualifyCrossStorePage uses the actual coordinator page, complete-copy facts,
// original SQL/Mongo owner batches, and the private cross-store qualification.
// It never accepts a caller's terminal, closure, or exported observation DTO.
func (c *HistoricalCoordinator) QualifyCrossStorePage(ctx context.Context, page *HistoricalSourcePage, sql *SQLBusinessOwnerBatch, mongo *MongoHistoricalOwnerBatch, cross *SQLMongoCrossStoreResponsibilityPage) (*HistoricalCoordinatorCrossStorePage, error) {
	if c == nil {
		return nil, ErrCoordinatorInvalid
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.alive(ctx); err != nil {
		return nil, err
	}
	if err := c.pageValid(page); err != nil {
		return nil, err
	}
	if err := c.crossStorePageBinding(page, sql, mongo, cross); err != nil {
		return nil, err
	}
	if err := cross.ValidateBorrowedSnapshot(ctx); err != nil {
		return nil, ErrCoordinatorInvalid
	}
	journal := &HistoricalCoordinatorCrossStorePage{owner: c, page: page, cross: cross, candidateStart: len(c.candidates), candidateCount: len(page.rows)}
	rows := make([]HistoricalCandidate, 0, len(page.rows))
	for _, row := range page.rows {
		if err := c.alive(ctx); err != nil {
			return nil, err
		}
		facts, err := row.event.Facts()
		if err != nil {
			return nil, err
		}
		candidate := coordinatorEventCandidate(facts)
		if facts.Source.Database == "mysql" {
			coordinatorCrossStoreSQLCandidate(ctx, &candidate, facts, row.event, sql, mongo)
		} else {
			key := row.keys[0]
			qualified := cross.qualification[key]
			if qualified == nil || qualified.batch != mongo {
				return nil, ErrCoordinatorPage
			}
			local := qualified.Local()
			view, err := cross.ResolveSource(ctx, row.event)
			if err != nil {
				return nil, err
			}
			binding, err := coordinatorCrossStoreObservationBinding(facts, local, view, cross, mongo)
			if err != nil {
				return nil, err
			}
			journal.bindings = append(journal.bindings, binding)
			candidate.LocalQualified = local.OwnerLocalTerminal && len(local.BlockingReasons) == 0 && len(view.BlockingReasons) == 0
			candidate.HistoricalGaps = append(candidate.HistoricalGaps, local.Gaps...)
			candidate.HistoricalGaps = append(candidate.HistoricalGaps, view.Gaps...)
			candidate.BlockingReasons = append(candidate.BlockingReasons, local.BlockingReasons...)
			candidate.BlockingReasons = append(candidate.BlockingReasons, view.BlockingReasons...)
			candidate.BusinessBindingSHA256 = local.BusinessBindingSHA256
			candidate.BusinessBaselineSHA256 = mongo.Report().BusinessRowsSHA256
			candidate.ActualOriginalRun = local.OriginalRun
			candidate.RequiredAdapters = append(candidate.RequiredAdapters, "mongo_original_types_remaining_ai_and_inbox_closure", "complete_sql_mongo_global_unbound_observation_reconciliation")
		}
		// The exact provisional keys are retained above; aggregate global
		// unknown/Invalid/blocking counts are never decremented or hidden.
		if sql.responsibility.Report().Unknown > 0 || sql.responsibility.Report().Blocking > 0 {
			candidate.BlockingReasons = append(candidate.BlockingReasons, "global_sql_orphan_unknown_or_conflicting_responsibility")
		}
		candidate.BlockingReasons = append(candidate.BlockingReasons, mongo.global.Report().BlockingReasons...)
		candidate.RequiredAdapters = append(candidate.RequiredAdapters, mongo.global.Report().CoverageGaps...)
		coordinatorClassify(&candidate)
		rows = append(rows, candidate)
	}
	if err := cross.ValidateBorrowedSnapshot(ctx); err != nil {
		return nil, ErrCoordinatorInvalid
	}
	if err := c.alive(ctx); err != nil {
		return nil, err
	}
	if err := c.crossStorePageBinding(page, sql, mongo, cross); err != nil {
		return nil, err
	}
	journal.candidateSHA = coordinatorCandidateHash(rows)
	if !evidence.ValidSHA256(journal.candidateSHA) {
		return nil, ErrCoordinatorPage
	}
	hash, err := privateFactsSHA(journal.bindings)
	if err != nil {
		return nil, err
	}
	journal.bindingSHA = hex.EncodeToString(hash[:])
	receipt := HistoricalCoordinatorPageReceipt{SQLBusinessBaselineSHA256: sql.Report().BusinessRowsSHA256, MongoBusinessBaselineSHA256: mongo.Report().BusinessRowsSHA256}
	if err := c.consume(page, rows, receipt); err != nil {
		return nil, err
	}
	return journal, nil
}

func (c *HistoricalCoordinator) crossStorePageBinding(page *HistoricalSourcePage, sql *SQLBusinessOwnerBatch, mongo *MongoHistoricalOwnerBatch, cross *SQLMongoCrossStoreResponsibilityPage) error {
	if err := c.pageValid(page); err != nil {
		return err
	}
	if sql == nil || sql.facts == nil || sql.responsibility == nil || mongo == nil || mongo.sql != sql.facts || cross == nil || cross.page == nil || cross.sql != sql || cross.mongo != mongo || cross.catalog == nil || cross.catalog.current != sql.responsibility || cross.catalog.index == nil || !cross.catalog.Report().Complete || cross.Report().CycleID != sql.responsibility.Report().CycleID || !cross.Report().Complete {
		return ErrCoordinatorInvalid
	}
	var sources []*VerifiedSourceEvent
	for _, row := range page.rows {
		if row.event == nil || row.bridge != nil || row.legacy != nil || len(row.keys) != 1 || !c.remaining[row.keys[0]] {
			return ErrCoordinatorPage
		}
		facts, err := row.event.Facts()
		if err != nil {
			return err
		}
		key, err := sourceAuthKey(facts.Source.Database, facts.Source.Object, facts.Source.PrimaryKeySHA256)
		if err != nil || key != row.keys[0] {
			return ErrCoordinatorPage
		}
		hash, err := privateFactsSHA(facts)
		approved, found := c.authenticated.rows[key]
		if err != nil || !found || hash != approved.facts {
			return ErrSourceAuthentication
		}
		if facts.Source.Database == "mongodb" {
			_, boundKey, err := mongo.source(row.event)
			if err != nil || boundKey != key || mongo.sourceFactsSHA[key] != hash {
				return ErrCoordinatorPage
			}
			sources = append(sources, row.event)
		}
	}
	if len(sources) == 0 || len(sources) != len(cross.sources) {
		return ErrCoordinatorPage
	}
	for i, source := range sources {
		if source != cross.sources[i] {
			return ErrCoordinatorPage
		}
	}
	return nil
}

// This is the existing SQL branch's composition, through its actual opaque
// resolver and reverse Mongo batch. No business decision is reimplemented.
func coordinatorCrossStoreSQLCandidate(ctx context.Context, candidate *HistoricalCandidate, facts *DecodedSourceEvent, source *VerifiedSourceEvent, sql *SQLBusinessOwnerBatch, mongo *MongoHistoricalOwnerBatch) {
	qualified, err := sql.ResolveUntrustedSQLSource(ctx, facts)
	if err != nil {
		candidate.BlockingReasons = append(candidate.BlockingReasons, "sql_original_business_or_responsibility_rejected")
	} else {
		local := qualified.Local()
		candidate.LocalQualified = local.OwnerLocalTerminal && len(local.BlockingReasons) == 0
		candidate.HistoricalGaps = append(candidate.HistoricalGaps, local.Gaps...)
		candidate.BlockingReasons = append(candidate.BlockingReasons, local.BlockingReasons...)
		candidate.BusinessBaselineSHA256 = sql.Report().BusinessRowsSHA256
		candidate.ActualOriginalRun = local.OriginalRun
		candidate.AuthorizationRun = local.AuthorizationRun
		candidate.ExecutionRun = local.ExecutionRun
	}
	view, err := mongo.ResponsibilitiesForSource(ctx, source)
	if err != nil {
		candidate.BlockingReasons = append(candidate.BlockingReasons, "mongo_original_downstream_or_responsibility_rejected")
	} else {
		candidate.BlockingReasons = append(candidate.BlockingReasons, view.BlockingReasons...)
	}
	candidate.RequiredAdapters = append(candidate.RequiredAdapters, "sql_stable_original_business_binding_writer_adapter")
}

func coordinatorCrossStoreObservationBinding(facts *DecodedSourceEvent, local MongoLocalResolution, view SQLMongoCrossStoreResponsibilityView, cross *SQLMongoCrossStoreResponsibilityPage, mongo *MongoHistoricalOwnerBatch) (HistoricalCrossStoreObservationBinding, error) {
	b := HistoricalCrossStoreObservationBinding{}
	if facts.Source.Database != "mongodb" || facts.EventID != local.EventID || facts.EventType != local.EventType || facts.OrgID != local.OrgID || facts.OrgID != view.OrganizationID || facts.EventID != view.EventID || facts.EventType != view.EventType || facts.Source.Digest.SHA256 != view.SourceRowSHA256 || facts.ContentDigest.SHA256 != view.LegacyContentSHA256 || local.BusinessBindingSHA256 != view.BusinessBindingSHA256 || local.AssessmentID != view.AssessmentID || !view.SourceCopyFactsBound || !view.CurrentSQLCoverageObserved || view.DropReady {
		return b, ErrCoordinatorPage
	}
	factHash, err := privateFactsSHA(facts)
	if err != nil {
		return b, err
	}
	b = HistoricalCrossStoreObservationBinding{Source: facts.Source, OriginalID: facts.EventID, EventType: facts.EventType, OrganizationID: strconv.FormatUint(facts.OrgID, 10), SourceFactsSHA256: hex.EncodeToString(factHash[:]), LegacyContentSHA256: facts.ContentDigest.SHA256, BusinessBindingSHA256: local.BusinessBindingSHA256, MongoBusinessBaselineSHA256: mongo.Report().BusinessRowsSHA256, SQLBusinessBaselineSHA256: cross.sql.Report().BusinessRowsSHA256, SQLFullRowBaselineSHA256: cross.Report().RowsSHA256, SQLCycleID: cross.Report().CycleID, MongoSnapshotSHA256: mongo.global.Report().SnapshotSHA256, ActualMongoSessionTransactionSHA256: mongoOwnerHashParts("actual-mongo-session-txn/v1", string(mongo.global.txn.session), strconv.FormatInt(mongo.global.txn.number, 10)), OwnerBoundObservationKeys: append([]string(nil), view.OwnerBoundObservationKeys...), ProvisionalOwnerResolvedObservationKeys: append([]string(nil), view.ProvisionalOwnerResolvedObservationKeys...), OriginalObservationRowSHA256: map[string]string{}, ActualOriginalRun: local.OriginalRun}
	for _, key := range b.OwnerBoundObservationKeys {
		if b.OriginalObservationRowSHA256[key] != "" {
			return HistoricalCrossStoreObservationBinding{}, ErrCoordinatorPage
		}
		found := false
		for _, row := range view.Observations {
			o := row.Observation
			if o.Store+":"+o.PrimaryKeySHA256 == key {
				if found || o.Invalid || o.OrgID != facts.OrgID || !evidence.ValidSHA256(o.RowSHA256) {
					return HistoricalCrossStoreObservationBinding{}, ErrCoordinatorPage
				}
				found = true
				b.OriginalObservationRowSHA256[key] = o.RowSHA256
				if actual, exists := mongo.global.graph.messages[o.EventID]; exists {
					b.CurrentStandardReferences = append(b.CurrentStandardReferences, actual.reference)
				}
			}
		}
		if !found {
			return HistoricalCrossStoreObservationBinding{}, ErrCoordinatorPage
		}
	}
	for _, key := range b.ProvisionalOwnerResolvedObservationKeys {
		if !slices.Contains(b.OwnerBoundObservationKeys, key) {
			return HistoricalCrossStoreObservationBinding{}, ErrCoordinatorPage
		}
	}
	return b, nil
}

func (p *HistoricalCoordinatorCrossStorePage) wholeCoverageBoundLocked() error {
	if p == nil || p.owner == nil || p.page == nil || p.cross == nil {
		return ErrCoordinatorPage
	}
	c := p.owner
	if !c.coverage || c.failed {
		return ErrCoordinatorIncomplete
	}
	if c.now().Sub(c.started) > c.limits.MaxDuration {
		return ErrCoordinatorExpired
	}
	if p.page.owner != c || !p.page.consumed || p.page.sequence == 0 || p.page.sequence > uint64(len(c.pages)) || p.candidateStart < 0 || p.candidateCount < 1 || p.candidateStart+p.candidateCount > len(c.candidates) || c.pages[p.page.sequence-1].CandidateSHA256 != p.candidateSHA || coordinatorCandidateHash(c.candidates[p.candidateStart:p.candidateStart+p.candidateCount]) != p.candidateSHA {
		return ErrCoordinatorPage
	}
	hash, err := privateFactsSHA(p.bindings)
	if err != nil || hex.EncodeToString(hash[:]) != p.bindingSHA {
		return ErrCoordinatorPage
	}
	for _, b := range p.bindings {
		key, err := sourceAuthKey(b.Source.Database, b.Source.Object, b.Source.PrimaryKeySHA256)
		approved, found := c.authenticated.rows[key]
		if err != nil || !found || hex.EncodeToString(approved.facts[:]) != b.SourceFactsSHA256 || c.remaining[key] {
			return ErrCoordinatorPage
		}
	}
	return nil
}

func (p *HistoricalCoordinatorCrossStorePage) Summary() HistoricalCrossStorePageSummary {
	r := HistoricalCrossStorePageSummary{ExternalOriginRequired: true, AIInboxRequired: true, GlobalUnboundRequired: true, FinalFreshRequired: true, WriterFenceRequired: true, CASRequired: true}
	if p == nil || p.owner == nil {
		return r
	}
	p.owner.mu.Lock()
	defer p.owner.mu.Unlock()
	if p.wholeCoverageBoundLocked() != nil {
		return r
	}
	r.WholeFourSourceCoverageBound = true
	r.SourcePageSequence = p.page.sequence
	r.MongoCandidates = uint64(len(p.bindings))
	r.CandidateSHA256, r.ObservationBindingSHA256 = p.candidateSHA, p.bindingSHA
	for _, b := range p.bindings {
		r.OwnerBoundObservations += uint64(len(b.OwnerBoundObservationKeys))
		r.ProvisionalOwnersResolved += uint64(len(b.ProvisionalOwnerResolvedObservationKeys))
	}
	return r
}

// Only after genuine four-copy second EOF can bounded, body-free observations
// be inspected. They remain defensive copies, never write authority.
func (p *HistoricalCoordinatorCrossStorePage) ObservationBindingRange(offset, limit int) ([]HistoricalCrossStoreObservationBinding, error) {
	if p == nil || p.owner == nil {
		return nil, ErrCoordinatorPage
	}
	p.owner.mu.Lock()
	defer p.owner.mu.Unlock()
	if err := p.wholeCoverageBoundLocked(); err != nil {
		return nil, err
	}
	if offset < 0 || offset > len(p.bindings) || limit < 1 || limit > 512 {
		return nil, ErrCoordinatorBounds
	}
	end := min(len(p.bindings), offset+limit)
	out := make([]HistoricalCrossStoreObservationBinding, end-offset)
	for i := range out {
		v := p.bindings[offset+i]
		v.OwnerBoundObservationKeys = append([]string(nil), v.OwnerBoundObservationKeys...)
		v.ProvisionalOwnerResolvedObservationKeys = append([]string(nil), v.ProvisionalOwnerResolvedObservationKeys...)
		v.CurrentStandardReferences = append([]evidence.StandardReference(nil), v.CurrentStandardReferences...)
		v.OriginalObservationRowSHA256 = map[string]string{}
		for key, digest := range p.bindings[offset+i].OriginalObservationRowSHA256 {
			v.OriginalObservationRowSHA256[key] = digest
		}
		if v.ActualOriginalRun != nil {
			clone := *v.ActualOriginalRun
			v.ActualOriginalRun = &clone
		}
		out[i] = v
	}
	return out, nil
}
