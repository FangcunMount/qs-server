package retirement

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strconv"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	eventoutcome "github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
)

const (
	ErrSQLMongoCrossStore         SourceError = "sql_mongo_cross_store_responsibility_invalid"
	ErrSQLMongoCrossStoreConflict SourceError = "sql_mongo_cross_store_responsibility_conflict"
)

// The catalog authenticates only actual current SQL keys and its complete
// cycle. Original source authentication and real Mongo business qualification
// are separate required capabilities of every page below.
type SQLCrossStoreResponsibilityCatalog struct {
	current *SQLResponsibilitySnapshot
	index   *sqlevaluation.SQLHistoricalCrossStoreCatalog
}

func (*SQLCrossStoreResponsibilityCatalog) MarshalJSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}
func (*SQLCrossStoreResponsibilityCatalog) MarshalBSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}
func (*SQLCrossStoreResponsibilityCatalog) String() string {
	return "private current SQL cross-store catalog; not source proof"
}
func (c *SQLCrossStoreResponsibilityCatalog) GoString() string { return c.String() }

func PrepareSQLCrossStoreResponsibilityCatalog(ctx context.Context, current *SQLResponsibilitySnapshot, limits sqlevaluation.SQLCrossStoreLimits) (*SQLCrossStoreResponsibilityCatalog, error) {
	if current == nil || current.cycle == nil {
		return nil, ErrSQLMongoCrossStore
	}
	index, err := sqlevaluation.PrepareSQLHistoricalCrossStoreCatalog(ctx, current.cycle, limits)
	if err != nil {
		return nil, err
	}
	return &SQLCrossStoreResponsibilityCatalog{current: current, index: index}, nil
}
func (c *SQLCrossStoreResponsibilityCatalog) Report() sqlevaluation.SQLCrossStoreCatalogReport {
	if c == nil {
		return (*sqlevaluation.SQLHistoricalCrossStoreCatalog)(nil).Report()
	}
	return c.index.Report()
}

type SQLMongoCrossStoreResponsibilityPage struct {
	catalog       *SQLCrossStoreResponsibilityCatalog
	sql           *SQLBusinessOwnerBatch
	mongo         *MongoHistoricalOwnerBatch
	page          *sqlevaluation.SQLHistoricalCrossStorePage
	sources       []*VerifiedSourceEvent
	qualification map[verifiedSourceKey]*MongoHistoricalBatchOwnerQualification
	views         map[verifiedSourceKey]SQLMongoCrossStoreResponsibilityView
	resolvedOwner map[verifiedSourceKey][]string
}
type SQLMongoCrossStoreResponsibilityView struct {
	EventID, EventType, SourceRowSHA256, LegacyContentSHA256, BusinessBindingSHA256                                                                                                   string
	OrganizationID, AssessmentID                                                                                                                                                      uint64
	Observations                                                                                                                                                                      []sqlevaluation.SQLCrossStoreRow
	OwnerBoundObservationKeys, BlockingReasons, Gaps                                                                                                                                  []string
	ProvisionalOwnerResolvedObservationKeys                                                                                                                                           []string
	CurrentSQLCoverageObserved, SourceCopyFactsBound, ExternalOriginAuthenticationRequired, GlobalFreshRequired, AIInboxCoverageRequired, WriterFenceRequired, CASRequired, DropReady bool
	GlobalUnboundSQLCoverageRequired                                                                                                                                                  bool
}

func (*SQLMongoCrossStoreResponsibilityPage) MarshalJSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}
func (*SQLMongoCrossStoreResponsibilityPage) MarshalBSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}
func (*SQLMongoCrossStoreResponsibilityPage) String() string {
	return "private Mongo to SQL responsibility page; not DROP proof"
}
func (p *SQLMongoCrossStoreResponsibilityPage) GoString() string { return p.String() }
func (SQLMongoCrossStoreResponsibilityView) MarshalJSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}
func (SQLMongoCrossStoreResponsibilityView) MarshalBSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}
func (SQLMongoCrossStoreResponsibilityView) String() string {
	return "private cross-store qualification; payload is not public"
}
func (v SQLMongoCrossStoreResponsibilityView) GoString() string { return v.String() }

func PrepareSQLMongoCrossStoreResponsibilityPage(ctx context.Context, catalog *SQLCrossStoreResponsibilityCatalog, sql *SQLBusinessOwnerBatch, mongo *MongoHistoricalOwnerBatch, sources []*VerifiedSourceEvent) (*SQLMongoCrossStoreResponsibilityPage, error) {
	if ctx == nil || catalog == nil || catalog.index == nil || sql == nil || sql.facts == nil || sql.responsibility != catalog.current || mongo == nil || mongo.sql != sql.facts || len(sources) == 0 || len(sources) > 512 {
		return nil, ErrSQLMongoCrossStore
	}
	if err := sql.ValidateBorrowedSnapshot(ctx); err != nil {
		return nil, err
	}
	if err := mongo.ValidateBorrowedSnapshot(ctx); err != nil {
		return nil, err
	}
	p := &SQLMongoCrossStoreResponsibilityPage{catalog: catalog, sql: sql, mongo: mongo, sources: append([]*VerifiedSourceEvent(nil), sources...), qualification: map[verifiedSourceKey]*MongoHistoricalBatchOwnerQualification{}, views: map[verifiedSourceKey]SQLMongoCrossStoreResponsibilityView{}, resolvedOwner: map[verifiedSourceKey][]string{}}
	selectors := sqlevaluation.SQLCrossStoreSelectors{}
	assessments, orgs := map[uint64]bool{}, map[uint64]bool{}
	owners := map[sqlevaluation.SQLCrossStoreOwnerReference]bool{}
	for _, handle := range sources {
		source, key, err := mongo.source(handle)
		if err != nil {
			return nil, err
		}
		if p.qualification[key] != nil {
			return nil, ErrSQLMongoCrossStore
		}
		if source.EventType != "answersheet.submitted" && source.EventType != "interpretation.report.generated" {
			return nil, ErrSourceEventType
		}
		q, err := mongo.ResolveSource(ctx, handle)
		if err != nil {
			return nil, err
		}
		p.qualification[key] = q
		local := q.Local()
		if local.EventID != source.EventID || local.EventType != source.EventType || local.OrgID != source.OrgID || local.BusinessBindingSHA256 == "" {
			return nil, ErrSQLMongoCrossStoreConflict
		}
		if source.Submitted != nil {
			id, err := mongoCycleStringID(source.Submitted.AnswerSheetID)
			if err != nil {
				return nil, err
			}
			actual, e := sql.OwnerByAnswerSheet(id)
			if local.FrozenAdmissionPurpose == "independent_questionnaire" {
				if !errors.Is(e, sqlevaluation.ErrSQLHistoricalOwnerAbsent) || !mongo.sqlAbsent[key] {
					return nil, ErrSQLMongoCrossStoreConflict
				}
			} else if e != nil || !actual.HasVerifiedAnswerSheetAssociation(id) || actual.Snapshot().Owner.AssessmentID != local.AssessmentID {
				return nil, ErrSQLMongoCrossStoreConflict
			}
		}
		selectors.EventIDs = append(selectors.EventIDs, source.EventID)
		orgs[source.OrgID] = true
		if local.AssessmentID != 0 {
			assessments[local.AssessmentID] = true
		}
		if local.AnswerSheetID != 0 {
			owners[sqlevaluation.SQLCrossStoreOwnerReference{Kind: "AnswerSheet", ID: strconv.FormatUint(local.AnswerSheetID, 10)}] = true
		}
		if local.GenerationID != 0 {
			owners[sqlevaluation.SQLCrossStoreOwnerReference{Kind: "ReportGeneration", ID: strconv.FormatUint(local.GenerationID, 10)}] = true
		}
		if owner := mongo.sqlOwners[key]; owner != nil {
			for _, outcome := range owner.Snapshot().Outcomes {
				for _, raw := range mongo.indexes["report_generations"]["outcome_id"][outcome.ID] {
					id, ok := mongoExactInteger(raw.Lookup("domain_id"))
					if !ok || id <= 0 {
						return nil, ErrSQLMongoCrossStoreConflict
					}
					owners[sqlevaluation.SQLCrossStoreOwnerReference{Kind: "ReportGeneration", ID: strconv.FormatInt(id, 10)}] = true
				}
			}
		}
	}
	selectors.AssessmentIDs = mongoBatchIDs(assessments)
	selectors.OrganizationIDs = mongoBatchIDs(orgs)
	for owner := range owners {
		selectors.MongoOwners = append(selectors.MongoOwners, owner)
	}
	page, err := sqlevaluation.PrepareSQLHistoricalCrossStorePage(ctx, catalog.index, sql.facts, selectors)
	if err != nil {
		return nil, err
	}
	p.page = page
	for _, handle := range sources {
		source, key, err := mongo.source(handle)
		if err != nil {
			return nil, err
		}
		if err = p.qualifyProvisionalOwner(ctx, source, key); err != nil {
			return nil, err
		}
		view, err := p.resolve(source, key)
		if err != nil {
			return nil, err
		}
		p.views[key] = view
	}
	if err := p.ValidateBorrowedSnapshot(ctx); err != nil {
		return nil, err
	}
	return p, nil
}

// This reader is constructed only after the same private full-row page and
// original Mongo graph agree. It resolves the one known Mongo2 provisional
// owner cause; it cannot erase a real Invalid flag, an execution obligation,
// an unknown namespace, or an unrelated row. No public DTO can create it.
type sqlMongoResolvedOwnerReader struct {
	actual   *sqlevaluation.SQLHistoricalBatchOwnerFacts
	resolved map[string]sqlevaluation.SQLResponsibilityObservation
}

func (r *sqlMongoResolvedOwnerReader) Snapshot() sqlevaluation.SQLHistoricalFactsSnapshot {
	return sqlMongoResolvedSnapshot(r.actual.Snapshot(), r.resolved)
}

func sqlMongoResolvedSnapshot(v sqlevaluation.SQLHistoricalFactsSnapshot, resolved map[string]sqlevaluation.SQLResponsibilityObservation) sqlevaluation.SQLHistoricalFactsSnapshot {
	for i := range v.Responsibilities {
		current := &v.Responsibilities[i]
		o, ok := resolved[current.Store+":"+current.ID]
		if ok && sqlMongoProvisionalMatches(*current, o) {
			current.Invalid = false
		}
	}
	return v
}
func sqlMongoProvisionalMatches(current sqlevaluation.SQLHistoricalResponsibility, o sqlevaluation.SQLResponsibilityObservation) bool {
	return current.Invalid && !o.Invalid && o.OwnerUnproven && len(o.Reasons) == 0 && current.Store == o.Store && current.ID == o.PrimaryKeySHA256 && current.EventID == o.EventID && current.EventType == o.EventType && current.OrgID == o.OrgID && current.AssessmentID == o.AssessmentID && current.TesteeID == o.TesteeID && current.State == o.State && current.Unfinished == o.Unfinished && current.LeasePresent == o.LeasePresent
}
func (r *sqlMongoResolvedOwnerReader) OutcomeRecord(id uint64) (*evaluationfact.Record, error) {
	return r.actual.OutcomeRecord(id)
}
func (r *sqlMongoResolvedOwnerReader) HasVerifiedAnswerSheetAssociation(id uint64) bool {
	return r.actual.HasVerifiedAnswerSheetAssociation(id)
}

func (p *SQLMongoCrossStoreResponsibilityPage) qualifyProvisionalOwner(ctx context.Context, source *DecodedSourceEvent, key verifiedSourceKey) error {
	actual := p.mongo.sqlOwners[key]
	if actual == nil {
		return nil
	}
	local := p.qualification[key].Local()
	rows, err := p.page.Lookup(source.EventID, local.AssessmentID, source.OrgID)
	if err != nil {
		return err
	}
	resolved := map[string]sqlevaluation.SQLResponsibilityObservation{}
	facts := actual.Snapshot()
	for _, row := range rows {
		o := row.Observation
		// These exact causes are assigned by the frozen SQL decoder only
		// for valid Mongo2 held/dead-letter payloads requiring cross-store
		// owner association. Transport pending/lease is never removed.
		if (o.Store != "retry_event_hold" && o.Store != "event_delivery_dead_letter") || o.EventID != source.EventID || o.EventType != source.EventType || !o.OwnerUnproven || o.Invalid || o.Unfinished || o.LeasePresent || o.ScopeClass != "retirement_related" || len(o.Reasons) != 0 {
			continue
		}
		var observed SQLMongoCrossStoreResponsibilityView
		if err := p.verifyMongoMessage(&observed, source, local, row); err != nil {
			return err
		}
		if len(observed.BlockingReasons) != 0 || len(observed.OwnerBoundObservationKeys) != 1 {
			return ErrSQLMongoCrossStoreConflict
		}
		for _, current := range facts.Responsibilities {
			if sqlMongoProvisionalMatches(current, o) {
				resolved[o.Store+":"+o.PrimaryKeySHA256] = o
			}
		}
	}
	if len(resolved) == 0 {
		return nil
	}
	// Re-run the actual original business graph through its shared verifier,
	// rather than editing the previous public terminal result. The immutable
	// SQL facts/Outcome capability and raw BSON business cache remain owners.
	r := &MongoOwnerResolution{db: p.mongo.global.db, config: p.mongo.global.config, source: source, businessReader: p.mongo, sqlFacts: &sqlMongoResolvedOwnerReader{actual: actual, resolved: resolved}}
	r.local = MongoLocalResolution{EventID: source.EventID, EventType: source.EventType, OrgID: source.OrgID, SourceAuthenticationRequired: true, SQLCrossClosureRequired: true, SQLResponsibilityRequired: true, GlobalUnboundResponsibilityCoverageRequired: true}
	if source.EventType == "answersheet.submitted" {
		err = r.readSubmission(ctx)
	} else {
		err = r.readGenerated(ctx)
	}
	if err != nil {
		return err
	}
	q := p.qualification[key]
	for _, reason := range q.responsibilities.BlockingReasons {
		r.block(reason)
	}
	for _, o := range q.responsibilities.Observations {
		if o.Collection == "rm_outbox" {
			r.local.CurrentResponsibilityCount++
		}
	}
	for _, expected := range r.expectedStandard {
		current, exists := p.mongo.global.graph.messages[expected.EventID]
		if !exists {
			r.block("mongo_live_standard_reference_absent")
		} else if current.reference != expected {
			return ErrMongoOwnerConflict
		}
	}
	if r.local.BusinessBindingSHA256 != local.BusinessBindingSHA256 || !reflect.DeepEqual(r.local.OriginalRun, local.OriginalRun) {
		return ErrSQLMongoCrossStoreConflict
	}
	p.qualification[key] = &MongoHistoricalBatchOwnerQualification{batch: p.mongo, local: r.Local(), responsibilities: q.responsibilities}
	for observationKey := range resolved {
		p.resolvedOwner[key] = append(p.resolvedOwner[key], observationKey)
	}
	sort.Strings(p.resolvedOwner[key])
	return nil
}

func (p *SQLMongoCrossStoreResponsibilityPage) resolve(source *DecodedSourceEvent, key verifiedSourceKey) (SQLMongoCrossStoreResponsibilityView, error) {
	local := p.qualification[key].Local()
	v := SQLMongoCrossStoreResponsibilityView{EventID: source.EventID, EventType: source.EventType, SourceRowSHA256: source.Source.Digest.SHA256, LegacyContentSHA256: source.ContentDigest.SHA256, BusinessBindingSHA256: local.BusinessBindingSHA256, OrganizationID: source.OrgID, AssessmentID: local.AssessmentID, SourceCopyFactsBound: true, ExternalOriginAuthenticationRequired: true, GlobalFreshRequired: true, AIInboxCoverageRequired: true, WriterFenceRequired: true, CASRequired: true}
	v.BlockingReasons = append(v.BlockingReasons, local.BlockingReasons...)
	v.Gaps = append(v.Gaps, local.Gaps...)
	v.ProvisionalOwnerResolvedObservationKeys = append([]string(nil), p.resolvedOwner[key]...)
	v.GlobalUnboundSQLCoverageRequired = true
	if !local.OwnerLocalTerminal {
		v.BlockingReasons = append(v.BlockingReasons, "cross_store_original_business_not_terminal")
	}
	rows, err := p.page.Lookup(source.EventID, local.AssessmentID, source.OrgID)
	if err != nil {
		return v, err
	}
	for _, row := range rows {
		o := row.Observation
		if o.OrgID != source.OrgID || o.AssessmentID != 0 && local.AssessmentID != o.AssessmentID {
			return v, ErrSQLMongoCrossStoreConflict
		}
		if o.Invalid {
			return v, ErrSQLMongoCrossStoreConflict
		}
		switch o.Store {
		case "rm_outbox", "retry_event_hold", "event_delivery_dead_letter":
			if o.EventType == "answersheet.submitted" || o.EventType == "interpretation.report.generated" {
				if err := p.verifyMongoMessage(&v, source, local, row); err != nil {
					return v, err
				}
			} else {
				if o.EventID == source.EventID {
					return v, ErrSQLMongoCrossStoreConflict
				}
				if o.AssessmentID == 0 || o.OwnerUnproven || o.ScopeClass == "coordination_required" {
					v.BlockingReasons = append(v.BlockingReasons, "cross_store_related_sql_owner_unproven")
				}
			}
			// Publication/replayed/delivered is only transport evidence. The
			// separately qualified original business remains required above.
			if o.Unfinished || o.LeasePresent {
				v.BlockingReasons = append(v.BlockingReasons, "cross_store_sql_message_responsibility_unclosed")
			}
		case "qs_rm_replay_items":
			if err := p.verifyReplay(&v, source, local, row); err != nil {
				return v, err
			}
		case "system_governance_action_runs":
			if o.ScopeClass != "scope_outside_retirement" && (o.Unfinished || o.OwnerUnproven || o.ScopeClass == "coordination_required") {
				v.BlockingReasons = append(v.BlockingReasons, "cross_store_governance_responsibility_unclosed_or_unknown")
			}
		case "qs_rm_evaluation_request_ref", "qs_rm_gap_recovery_request":
			if o.EventID == source.EventID {
				return v, ErrSQLMongoCrossStoreConflict
			}
			if o.Invalid || o.Unfinished || o.LeasePresent || o.OwnerUnproven {
				v.BlockingReasons = append(v.BlockingReasons, "cross_store_sql_owner_reference_unclosed_or_unknown")
			}
		case "qs_rm_replay_requests":
			v.BlockingReasons = append(v.BlockingReasons, "cross_store_replay_header_without_original_item")
		default:
			return v, ErrSQLMongoCrossStoreConflict
		}
		v.Observations = append(v.Observations, row)
	}
	v.CurrentSQLCoverageObserved = true
	v.BlockingReasons = uniqueMongoCycleStrings(v.BlockingReasons)
	v.Gaps = uniqueMongoCycleStrings(v.Gaps)
	sort.Strings(v.OwnerBoundObservationKeys)
	return v, nil
}

func (p *SQLMongoCrossStoreResponsibilityPage) verifyMongoMessage(v *SQLMongoCrossStoreResponsibilityView, source *DecodedSourceEvent, local MongoLocalResolution, row sqlevaluation.SQLCrossStoreRow) error {
	o := row.Observation
	if row.Inner == nil || o.EventType != row.Inner.EventType || o.EventID != row.Inner.ID || o.OwnerKind != row.Inner.AggregateType || o.OwnerID != row.Inner.AggregateID || row.InnerDataSHA256 == "" || row.LegacyContentSHA256 == "" {
		return ErrSQLMongoCrossStoreConflict
	}
	if o.Store == "rm_outbox" {
		v.BlockingReasons = append(v.BlockingReasons, "cross_store_mongo_event_in_sql_rm_namespace")
	}
	if o.EventID == source.EventID {
		return verifyOriginalMongoSQLWire(v, source, local, row)
	} else {
		// A different live event must have a real globally authenticated
		// Mongo SDK row and original reverse business graph. No synthetic
		// evaluation source or current winner replaces the old event.
		current, ok := p.mongo.global.graph.messages[o.EventID]
		if !ok {
			v.BlockingReasons = append(v.BlockingReasons, "cross_store_related_mongo_event_original_owner_unknown")
			return nil
		}
		actual := p.mongo.global.observations[current.observation]
		if actual.Invalid || actual.OrgID != source.OrgID || actual.ContentSHA256 != row.LegacyContentSHA256 || actual.OwnerKind != o.OwnerKind || actual.OwnerID != o.OwnerID {
			return ErrSQLMongoCrossStoreConflict
		}
		if o.EventType == "answersheet.submitted" {
			var body eventpayload.AnswerSheetSubmittedData
			if strictTyped(row.Inner.Data, &body) != nil || body.TesteeID != local.TesteeID || body.AnswerSheetID != strconv.FormatUint(local.AnswerSheetID, 10) {
				return ErrSQLMongoCrossStoreConflict
			}
		} else {
			var body eventoutcome.ReportGeneratedPayload
			if strictTyped(row.Inner.Data, &body) != nil || body.TesteeID != local.TesteeID || body.AssessmentID != strconv.FormatUint(local.AssessmentID, 10) {
				return ErrSQLMongoCrossStoreConflict
			}
			owner := p.mongo.sqlOwners[mustSQLMongoSourceKey(source)]
			if owner == nil {
				return ErrSQLMongoCrossStoreConflict
			}
			id, err := mongoCycleStringID(body.OutcomeID)
			if err != nil {
				return err
			}
			if _, err = owner.OutcomeRecord(id); err != nil {
				return err
			}
		}
	}
	if o.TesteeID != local.TesteeID || o.OrgID != local.OrgID {
		return ErrSQLMongoCrossStoreConflict
	}
	v.OwnerBoundObservationKeys = append(v.OwnerBoundObservationKeys, o.Store+":"+o.PrimaryKeySHA256)
	return nil
}

// Exact original Mongo source/wire identity. Related sources must each pass
// their own real business graph; this helper does not replace a current SDK row.
func verifyOriginalMongoSQLWire(v *SQLMongoCrossStoreResponsibilityView, source *DecodedSourceEvent, local MongoLocalResolution, row sqlevaluation.SQLCrossStoreRow) error {
	o := row.Observation
	if row.Inner == nil || o.EventID != source.EventID || o.EventType != row.Inner.EventType || o.EventID != row.Inner.ID || o.OwnerKind != row.Inner.AggregateType || o.OwnerID != row.Inner.AggregateID || row.InnerDataSHA256 == "" || row.LegacyContentSHA256 == "" {
		return ErrSQLMongoCrossStoreConflict
	}
	if o.EventType != source.EventType || o.OwnerKind != source.AggregateType || o.OwnerID != source.AggregateID || !row.Inner.OccurredAt.Equal(source.OccurredAt) || row.LegacyContentSHA256 != source.ContentDigest.SHA256 {
		return ErrSQLMongoCrossStoreConflict
	}
	if source.Submitted != nil {
		var body eventpayload.AnswerSheetSubmittedData
		if strictTyped(row.Inner.Data, &body) != nil || !reflect.DeepEqual(&body, source.Submitted) {
			return ErrSQLMongoCrossStoreConflict
		}
	} else {
		var body eventoutcome.ReportGeneratedPayload
		if strictTyped(row.Inner.Data, &body) != nil || !reflect.DeepEqual(&body, source.Generated) {
			return ErrSQLMongoCrossStoreConflict
		}
	}
	if o.TesteeID != local.TesteeID || o.OrgID != local.OrgID {
		return ErrSQLMongoCrossStoreConflict
	}
	v.OwnerBoundObservationKeys = append(v.OwnerBoundObservationKeys, o.Store+":"+o.PrimaryKeySHA256)
	return nil
}

func mustSQLMongoSourceKey(source *DecodedSourceEvent) verifiedSourceKey {
	key, _ := sourceAuthKey(source.Source.Database, source.Source.Object, source.Source.PrimaryKeySHA256)
	return key
}

func (p *SQLMongoCrossStoreResponsibilityPage) verifyReplay(v *SQLMongoCrossStoreResponsibilityView, source *DecodedSourceEvent, local MongoLocalResolution, row sqlevaluation.SQLCrossStoreRow) error {
	if row.Replay == nil || !row.Replay.FingerprintVerified || row.Replay.OrganizationID != source.OrgID {
		return ErrSQLMongoCrossStoreConflict
	}
	var actualRow *mongoOwnerStandardRow
	authorized := false
	if row.Replay != nil {
		for _, item := range row.Replay.Items {
			authorized = authorized || item.EventID == row.Observation.EventID && item.Authorized
		}
	}
	if row.Replay != nil && row.Replay.Store == "mongo-domain-events" && authorized {
		if current, ok := p.mongo.global.graph.messages[row.Observation.EventID]; ok {
			actual := p.mongo.global.observations[current.observation]
			if actual.Invalid || actual.OrgID != source.OrgID {
				return ErrSQLMongoCrossStoreConflict
			}
			if actual.Unfinished || actual.LeasePresent {
				v.BlockingReasons = append(v.BlockingReasons, "cross_store_authorized_replay_business_or_current_responsibility_unclosed")
			}
			actualRow = &current.row
		}
	}
	return verifySQLMongoReplay(v, source, local, row, actualRow)
}

// Current is supplied only by an actual current Mongo row checker; no imported
// summary or historical source stands in for the SDK row's replay claim.
func verifySQLMongoReplay(v *SQLMongoCrossStoreResponsibilityView, source *DecodedSourceEvent, local MongoLocalResolution, row sqlevaluation.SQLCrossStoreRow, current *mongoOwnerStandardRow) error {
	r := row.Replay
	o := row.Observation
	if r == nil || !r.FingerprintVerified || r.OrganizationID != source.OrgID {
		return ErrSQLMongoCrossStoreConflict
	}
	if r.Store == "assessment-mysql-outbox" && o.EventID != source.EventID && o.AssessmentID == local.AssessmentID && o.AssessmentID != 0 {
		if o.OwnerUnproven || o.Invalid || o.OwnerKind != "Evaluation" {
			return ErrSQLMongoCrossStoreConflict
		}
		if o.Unfinished || o.LeasePresent {
			v.BlockingReasons = append(v.BlockingReasons, "cross_store_related_sql_replay_unclosed")
		}
		v.OwnerBoundObservationKeys = append(v.OwnerBoundObservationKeys, o.Store+":"+o.PrimaryKeySHA256)
		return nil
	}
	if r.Store != "mongo-domain-events" {
		return ErrSQLMongoCrossStoreConflict
	}
	var item *sqlevaluation.SQLCrossStoreReplayItem
	for i := range r.Items {
		if r.Items[i].EventID == o.EventID {
			if item != nil {
				return ErrSQLMongoCrossStoreConflict
			}
			item = &r.Items[i]
		}
	}
	if item == nil {
		return ErrSQLMongoCrossStoreConflict
	}
	if item.Authorized {
		if current == nil {
			v.BlockingReasons = append(v.BlockingReasons, "cross_store_authorized_replay_current_mongo_event_absent")
			return nil
		}
		if current.ManualReplayRequestID != r.RequestID || current.ManualReplayVersion == 0 || current.Version < current.ManualReplayVersion || current.FailureCount < item.ExpectedFailureCount {
			v.BlockingReasons = append(v.BlockingReasons, "cross_store_replay_current_claim_binding_unknown")
		}
		if current.State != "published" || !local.OwnerLocalTerminal {
			v.BlockingReasons = append(v.BlockingReasons, "cross_store_authorized_replay_business_or_current_responsibility_unclosed")
		}
	} else if item.Reason == "organization_mismatch" || item.Reason == "ambiguous_identity" {
		v.BlockingReasons = append(v.BlockingReasons, "cross_store_replay_original_identity_ambiguous")
	}
	v.OwnerBoundObservationKeys = append(v.OwnerBoundObservationKeys, o.Store+":"+o.PrimaryKeySHA256)
	return nil
}

func (p *SQLMongoCrossStoreResponsibilityPage) ResolveSource(ctx context.Context, handle *VerifiedSourceEvent) (SQLMongoCrossStoreResponsibilityView, error) {
	if p == nil || p.page == nil {
		return SQLMongoCrossStoreResponsibilityView{}, ErrSQLMongoCrossStore
	}
	if err := p.mongo.validateReaderContext(ctx); err != nil {
		return SQLMongoCrossStoreResponsibilityView{}, err
	}
	_, key, err := p.mongo.source(handle)
	if err != nil {
		return SQLMongoCrossStoreResponsibilityView{}, err
	}
	v, ok := p.views[key]
	if !ok {
		return SQLMongoCrossStoreResponsibilityView{}, ErrSQLMongoCrossStore
	}
	v.BlockingReasons = append([]string(nil), v.BlockingReasons...)
	v.Gaps = append([]string(nil), v.Gaps...)
	v.OwnerBoundObservationKeys = append([]string(nil), v.OwnerBoundObservationKeys...)
	v.ProvisionalOwnerResolvedObservationKeys = append([]string(nil), v.ProvisionalOwnerResolvedObservationKeys...)
	rows, err := p.page.Lookup(v.EventID, v.AssessmentID, v.OrganizationID)
	if err != nil {
		return SQLMongoCrossStoreResponsibilityView{}, err
	}
	v.Observations = rows
	return v, nil
}
func (p *SQLMongoCrossStoreResponsibilityPage) Report() sqlevaluation.SQLCrossStorePageReport {
	if p == nil {
		return (*sqlevaluation.SQLHistoricalCrossStorePage)(nil).Report()
	}
	return p.page.Report()
}
func (p *SQLMongoCrossStoreResponsibilityPage) ValidateBorrowedSnapshot(ctx context.Context) error {
	if p == nil || p.page == nil {
		return ErrSQLMongoCrossStore
	}
	if err := p.mongo.ValidateBorrowedSnapshot(ctx); err != nil {
		return err
	}
	return p.page.ValidateBorrowedSnapshot(ctx)
}

// Complete global SQL/Mongo fresh rechecks are separate mandatory coordinator
// gates. This rechecks only this exact bounded page in genuinely new snapshots.
func (p *SQLMongoCrossStoreResponsibilityPage) RecheckBusiness(ctx context.Context, freshCatalog *SQLCrossStoreResponsibilityCatalog, freshSQL *SQLBusinessOwnerBatch, freshMongo *MongoHistoricalOwnerBatch) error {
	if p == nil || freshCatalog == nil || freshSQL == nil || freshSQL.responsibility == nil || freshMongo == nil || p.catalog == freshCatalog || p.catalog.Report().CycleID == freshCatalog.Report().CycleID || p.catalog.Report().DatabaseIdentitySHA256 != freshCatalog.Report().DatabaseIdentitySHA256 {
		return ErrSQLMongoCrossStore
	}
	if err := p.sql.RecheckBusiness(ctx, freshSQL.responsibility); err != nil {
		return err
	}
	if err := p.mongo.RecheckBusiness(ctx, freshMongo.global, freshSQL.facts); err != nil {
		return err
	}
	current, err := PrepareSQLMongoCrossStoreResponsibilityPage(ctx, freshCatalog, freshSQL, freshMongo, p.sources)
	if err != nil {
		return err
	}
	if p.page.Report().RowsSHA256 != current.page.Report().RowsSHA256 || !reflect.DeepEqual(p.views, current.views) {
		return ErrSQLMongoCrossStoreConflict
	}
	return nil
}
