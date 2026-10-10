package retirement

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strconv"
	"time"

	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	ErrMongoBatchInvalid  SourceError = "mongo_historical_owner_batch_invalid"
	ErrMongoBatchBounds   SourceError = "mongo_historical_owner_batch_budget_exceeded"
	ErrMongoBatchConflict SourceError = "mongo_historical_owner_batch_conflict"
)

// Only opaque, actually prepared SQL capabilities are admitted by public
// entrypoints. This private interface shares business verification; it is not
// an operator-provided facts DTO, and deliberately has no legacy Recheck.
type mongoSQLBusinessFacts interface {
	Snapshot() sqlevaluation.SQLHistoricalFactsSnapshot
	OutcomeRecord(uint64) (*evaluationfact.Record, error)
	HasVerifiedAnswerSheetAssociation(uint64) bool
}

type mongoBusinessReader interface {
	rows(context.Context, string, bson.D) ([]bson.Raw, error)
	artifactIndexes(context.Context) ([]bson.Raw, error)
}

type MongoHistoricalOwnerBatchLimits struct {
	MaxSources, MaxRows int
	MaxBytes            uint64
	MaxDuration         time.Duration
}

func DefaultMongoHistoricalOwnerBatchLimits() MongoHistoricalOwnerBatchLimits {
	return MongoHistoricalOwnerBatchLimits{MaxSources: 128, MaxRows: 32768, MaxBytes: 64 << 20, MaxDuration: time.Minute}
}

func (l MongoHistoricalOwnerBatchLimits) valid() bool {
	return l.MaxSources > 0 && l.MaxSources <= 512 && l.MaxRows > 0 && l.MaxRows <= 131072 && l.MaxBytes > 0 && l.MaxBytes <= 256<<20 && l.MaxDuration > 0 && l.MaxDuration <= 2*time.Minute
}

type MongoHistoricalOwnerBatchReport struct {
	Protocol, DatabaseIdentitySHA256, GlobalSnapshotSHA256, SQLBusinessRowsSHA256, BusinessRowsSHA256 string
	Sources, UniqueBusinessRows, ReadRows, ReadBytes, Queries                                         uint64
	SourceCopyFactsBound, ExternalOriginAuthenticationRequired, SQLAIInboxCoverageRequired            bool
	OriginalBusinessFactsRequired, WriterFenceRequired, CASRequired, DropReady                        bool
}

type mongoBatchSelection struct {
	sheets, outcomes, generations, artifacts, runs map[uint64]bool
}

// A bounded business page borrows both actual host snapshots. The global
// responsibility graph is scanned once elsewhere; this page never reads RM,
// replay, Inbox, held/dead-letter or AI responsibility stores.
type MongoHistoricalOwnerBatch struct {
	global         *MongoResponsibilitySnapshot
	sql            *sqlevaluation.SQLHistoricalOwnerBatch
	limits         MongoHistoricalOwnerBatchLimits
	sources        map[verifiedSourceKey]*DecodedSourceEvent
	sourceFactsSHA map[verifiedSourceKey][32]byte
	sourceHandles  []*VerifiedSourceEvent
	sqlOwners      map[verifiedSourceKey]*sqlevaluation.SQLHistoricalBatchOwnerFacts
	sqlAbsent      map[verifiedSourceKey]bool
	selection      mongoBatchSelection
	data           map[string][]bson.Raw
	indexes        map[string]map[string]map[uint64][]bson.Raw
	seen           map[string]map[string]bson.Raw
	report         MongoHistoricalOwnerBatchReport
	started        time.Time
	sqlConnection  any
}

type MongoHistoricalBatchOwnerQualification struct {
	batch            *MongoHistoricalOwnerBatch
	local            MongoLocalResolution
	responsibilities MongoSourceResponsibilityView
}

func (*MongoHistoricalOwnerBatch) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*MongoHistoricalOwnerBatch) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*MongoHistoricalOwnerBatch) String() string {
	return "private Mongo business owner batch; not retirement approval"
}
func (b *MongoHistoricalOwnerBatch) GoString() string { return b.String() }
func (*MongoHistoricalBatchOwnerQualification) MarshalJSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}
func (*MongoHistoricalBatchOwnerQualification) MarshalBSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}
func (*MongoHistoricalBatchOwnerQualification) String() string {
	return "private Mongo batch owner qualification; no legacy recheck"
}
func (q *MongoHistoricalBatchOwnerQualification) GoString() string { return q.String() }

// These are selectors only. The whole approved source-copy index and its
// external provenance remain independently owned by the coordinator.
func MongoHistoricalSQLBatchSelectors(sources []*VerifiedSourceEvent) (sqlevaluation.SQLHistoricalOwnerBatchRequest, error) {
	assessments, sheets := map[uint64]bool{}, map[uint64]bool{}
	if len(sources) == 0 || len(sources) > 512 {
		return sqlevaluation.SQLHistoricalOwnerBatchRequest{}, ErrMongoBatchBounds
	}
	for _, handle := range sources {
		facts, err := handle.Facts()
		if err != nil {
			return sqlevaluation.SQLHistoricalOwnerBatchRequest{}, err
		}
		switch facts.EventType {
		case "answersheet.submitted":
			if facts.Submitted == nil {
				return sqlevaluation.SQLHistoricalOwnerBatchRequest{}, ErrMongoBatchInvalid
			}
			id, e := mongoCycleStringID(facts.Submitted.AnswerSheetID)
			if e != nil {
				return sqlevaluation.SQLHistoricalOwnerBatchRequest{}, e
			}
			sheets[id] = true
		case "interpretation.report.generated":
			if facts.Generated == nil {
				return sqlevaluation.SQLHistoricalOwnerBatchRequest{}, ErrMongoBatchInvalid
			}
			id, e := mongoCycleStringID(facts.Generated.AssessmentID)
			if e != nil {
				return sqlevaluation.SQLHistoricalOwnerBatchRequest{}, e
			}
			assessments[id] = true
		case "evaluation.requested", "evaluation.retry.requested", "evaluation.outcome.committed", "evaluation.failed":
			id, e := sqlSourceAssessment(facts)
			if e != nil {
				return sqlevaluation.SQLHistoricalOwnerBatchRequest{}, ErrMongoBatchInvalid
			}
			assessments[id] = true
		default:
			return sqlevaluation.SQLHistoricalOwnerBatchRequest{}, ErrSourceEventType
		}
	}
	return sqlevaluation.SQLHistoricalOwnerBatchRequest{AssessmentIDs: mongoBatchIDs(assessments), AnswerSheetIDs: mongoBatchIDs(sheets)}, nil
}

func PrepareMongoHistoricalOwnerBatch(ctx context.Context, global *MongoResponsibilitySnapshot, sql *sqlevaluation.SQLHistoricalOwnerBatch, sources []*VerifiedSourceEvent, limits MongoHistoricalOwnerBatchLimits) (*MongoHistoricalOwnerBatch, error) {
	if global == nil || sql == nil || !global.report.Complete || !sql.Report().Complete || !limits.valid() || len(sources) == 0 || len(sources) > limits.MaxSources {
		return nil, ErrMongoBatchInvalid
	}
	if err := global.ValidateBorrowedSnapshot(ctx); err != nil {
		return nil, err
	}
	tx, err := hostmysql.RequireTx(ctx)
	if err != nil || tx.Statement == nil || tx.Statement.ConnPool == nil || reflect.TypeOf(tx.Statement.ConnPool).Kind() != reflect.Pointer || reflect.ValueOf(tx.Statement.ConnPool).IsNil() {
		return nil, ErrMongoBatchInvalid
	}
	if err := sql.ValidateBorrowedSnapshot(ctx); err != nil {
		return nil, err
	}
	b := &MongoHistoricalOwnerBatch{global: global, sql: sql, limits: limits, data: map[string][]bson.Raw{}, indexes: map[string]map[string]map[uint64][]bson.Raw{}, seen: map[string]map[string]bson.Raw{}, started: time.Now(), sqlConnection: tx.Statement.ConnPool}
	b.sourceHandles = append([]*VerifiedSourceEvent(nil), sources...)
	b.report = MongoHistoricalOwnerBatchReport{Protocol: "mongo-business-owner-batch/v1", DatabaseIdentitySHA256: global.report.IdentitySHA256, GlobalSnapshotSHA256: global.report.SnapshotSHA256, SQLBusinessRowsSHA256: sql.Report().BusinessRowsSHA256, SourceCopyFactsBound: true, ExternalOriginAuthenticationRequired: true, SQLAIInboxCoverageRequired: true, OriginalBusinessFactsRequired: true, WriterFenceRequired: true, CASRequired: true}
	selection, err := selectMongoOwnerSources(sql, sources)
	if err != nil {
		return nil, err
	}
	b.sources, b.sourceFactsSHA, b.sqlOwners, b.sqlAbsent, b.selection = selection.sources, selection.sourceFactsSHA, selection.sqlOwners, selection.sqlAbsent, selection.selection
	b.report.Sources = uint64(len(selection.sources))
	if err := b.capture(ctx); err != nil {
		return nil, err
	}
	return b, nil
}

func (b *MongoHistoricalOwnerBatch) ValidateBorrowedSnapshot(ctx context.Context) error {
	if err := b.validateReaderContext(ctx); err != nil {
		return err
	}
	return b.sql.ValidateBorrowedSnapshot(ctx)
}

// Pure lookups retain an already authenticated host connection, not a new SQL
// liveness claim. The host must call the real ValidateBorrowedSnapshot at page
// boundaries and the independent fresh gates before using qualifications.
func (b *MongoHistoricalOwnerBatch) validateReaderContext(ctx context.Context) error {
	if b == nil || b.global == nil || b.sql == nil || b.sqlConnection == nil {
		return ErrMongoBatchInvalid
	}
	if time.Since(b.started) > b.limits.MaxDuration {
		return ErrMongoBatchBounds
	}
	if err := b.global.ValidateBorrowedSnapshot(ctx); err != nil {
		return err
	}
	tx, err := hostmysql.RequireTx(ctx)
	if err != nil || tx.Statement == nil || tx.Statement.ConnPool == nil || reflect.TypeOf(tx.Statement.ConnPool).Kind() != reflect.Pointer || reflect.ValueOf(tx.Statement.ConnPool).IsNil() || tx.Statement.ConnPool != b.sqlConnection {
		return ErrMongoBatchInvalid
	}
	return ctx.Err()
}

func (b *MongoHistoricalOwnerBatch) Report() MongoHistoricalOwnerBatchReport {
	if b == nil {
		return MongoHistoricalOwnerBatchReport{ExternalOriginAuthenticationRequired: true, SQLAIInboxCoverageRequired: true, OriginalBusinessFactsRequired: true, WriterFenceRequired: true, CASRequired: true}
	}
	return b.report
}

func (b *MongoHistoricalOwnerBatch) source(handle *VerifiedSourceEvent) (*DecodedSourceEvent, verifiedSourceKey, error) {
	var empty verifiedSourceKey
	if b == nil || handle == nil {
		return nil, empty, ErrMongoBatchInvalid
	}
	facts, err := handle.Facts()
	if err != nil {
		return nil, empty, err
	}
	key, err := sourceAuthKey(facts.Source.Database, facts.Source.Object, facts.Source.PrimaryKeySHA256)
	if err != nil {
		return nil, empty, err
	}
	baseline := b.sources[key]
	if baseline == nil {
		return nil, empty, ErrMongoBatchInvalid
	}
	digest, err := privateFactsSHA(facts)
	if err != nil || digest != b.sourceFactsSHA[key] {
		return nil, empty, ErrMongoBatchConflict
	}
	// copyMongoSource normalizes only private clone-only fields; binding uses
	// the actual opaque source facts, never a caller's mutable source DTO.
	if baseline.Source != facts.Source || baseline.EventID != facts.EventID || baseline.ContentDigest != facts.ContentDigest {
		return nil, empty, ErrMongoBatchConflict
	}
	return baseline, key, nil
}

func (b *MongoHistoricalOwnerBatch) ResolveSource(ctx context.Context, handle *VerifiedSourceEvent) (*MongoHistoricalBatchOwnerQualification, error) {
	if err := b.validateReaderContext(ctx); err != nil {
		return nil, err
	}
	source, key, err := b.source(handle)
	if err != nil {
		return nil, err
	}
	if source.EventType != "answersheet.submitted" && source.EventType != "interpretation.report.generated" {
		return nil, ErrSourceEventType
	}
	metadata := mongoOwnerMetadata{identity: b.global.metadata.identity, collections: map[string]string{}}
	for _, name := range []string{"schema_migrations", "answersheets", "report_generations", "interpretation_runs", "interpret_report_artifacts", "rm_outbox", "qs_rm_replay_requests"} {
		if d, ok := b.global.metadata.definitions[name]; ok {
			metadata.collections[name] = d.uuid
		} else {
			metadata.collections[name] = "absent"
		}
	}
	r := &MongoOwnerResolution{db: b.global.db, config: b.global.config, source: source, metadata: metadata, businessReader: b}
	if actual := b.sqlOwners[key]; actual != nil {
		r.sqlFacts = actual
	}
	r.local = MongoLocalResolution{EventID: source.EventID, EventType: source.EventType, OrgID: source.OrgID, SourceAuthenticationRequired: true, SQLCrossClosureRequired: true, SQLResponsibilityRequired: true, GlobalUnboundResponsibilityCoverageRequired: true}
	if source.EventType == "answersheet.submitted" {
		err = r.readSubmission(ctx)
	} else {
		err = r.readGenerated(ctx)
	}
	if err != nil {
		return nil, err
	}
	view, err := b.ResponsibilitiesForSource(ctx, handle)
	if err != nil {
		return nil, err
	}
	for _, reason := range view.BlockingReasons {
		r.block(reason)
	}
	for _, observation := range view.Observations {
		if observation.Collection == "rm_outbox" {
			r.local.CurrentResponsibilityCount++
		}
	}
	for _, expected := range r.expectedStandard {
		actual, ok := b.global.graph.messages[expected.EventID]
		if !ok {
			r.block("mongo_live_standard_reference_absent")
		} else if actual.reference != expected {
			return nil, ErrMongoOwnerConflict
		}
	}
	if source.EventType == "answersheet.submitted" && r.local.FrozenAdmissionPurpose == "independent_questionnaire" {
		if !b.sqlAbsent[key] {
			return nil, ErrMongoOwnerConflict
		}
		var gaps []string
		for _, gap := range r.local.Gaps {
			if gap != "independent_admission_sql_absence_and_global_responsibility_not_checked" {
				gaps = append(gaps, gap)
			}
		}
		r.local.Gaps = append(gaps, "independent_admission_unique_sql_absence_observed_external_coverage_required")
	}
	return &MongoHistoricalBatchOwnerQualification{batch: b, local: r.Local(), responsibilities: view}, nil
}

func (q *MongoHistoricalBatchOwnerQualification) Local() MongoLocalResolution {
	if q == nil {
		var empty *MongoOwnerResolution
		return empty.Local()
	}
	r := &MongoOwnerResolution{local: q.local}
	return r.Local()
}

// Link completion is based on actual SQL Outcome ownership and the globally
// scanned Mongo reverse graph. Missing source Run IDs remain missing; no
// latest winner, nearest timestamp or new ID is inserted into source facts.
func (b *MongoHistoricalOwnerBatch) ResponsibilitiesForSource(ctx context.Context, handle *VerifiedSourceEvent) (MongoSourceResponsibilityView, error) {
	if err := b.validateReaderContext(ctx); err != nil {
		return MongoSourceResponsibilityView{}, err
	}
	source, key, err := b.source(handle)
	if err != nil {
		return MongoSourceResponsibilityView{}, err
	}
	queries := []*DecodedSourceEvent{source}
	owner := b.sqlOwners[key]
	if owner != nil {
		actual := owner.Snapshot()
		outcomes := map[uint64]bool{}
		for _, outcome := range actual.Outcomes {
			outcomes[outcome.ID] = true
		}
		for outcome := range outcomes {
			c := *source
			c.BusinessIDs = map[string]string{"assessment_id": strconv.FormatUint(actual.Owner.AssessmentID, 10), "outcome_id": strconv.FormatUint(outcome, 10)}
			if actual.Owner.AnswerSheetID != 0 {
				c.BusinessIDs["answer_sheet_id"] = strconv.FormatUint(actual.Owner.AnswerSheetID, 10)
			}
			queries = append(queries, &c)
			for _, raw := range b.indexes["report_generations"]["outcome_id"][outcome] {
				id, ok := mongoExactInteger(raw.Lookup("domain_id"))
				if !ok || id <= 0 {
					return viewForMongoBatchError(), ErrMongoCycleSchema
				}
				gid := uint64(id)
				c2 := c
				c2.BusinessIDs = map[string]string{"generation_id": strconv.FormatUint(gid, 10), "outcome_id": strconv.FormatUint(outcome, 10), "assessment_id": strconv.FormatUint(actual.Owner.AssessmentID, 10)}
				queries = append(queries, &c2)
			}
		}
		// A failed assessment can legitimately have no Outcome. Its actual
		// AnswerSheet still links responsibilities; no empty result closes
		// the global unknown/orphan/AI/Inbox coverage obligations.
		c := *source
		c.BusinessIDs = map[string]string{"assessment_id": strconv.FormatUint(actual.Owner.AssessmentID, 10)}
		if actual.Owner.AnswerSheetID != 0 {
			c.BusinessIDs["answer_sheet_id"] = strconv.FormatUint(actual.Owner.AnswerSheetID, 10)
		}
		queries = append(queries, &c)
	}
	view := MongoSourceResponsibilityView{SourceAuthenticationRequired: true, OriginalBusinessFactsRequired: true, SQLAIInboxCoverageRequired: true, WriterFenceRequired: true, CASRequired: true}
	seen := map[string]bool{}
	for _, query := range queries {
		v, e := b.global.ForUntrustedSource(ctx, query)
		if e != nil {
			return view, e
		}
		view.BlockingReasons = append(view.BlockingReasons, v.BlockingReasons...)
		for _, o := range v.Observations {
			key := o.Collection + ":" + o.PrimaryKeySHA256
			if !seen[key] {
				seen[key] = true
				view.Observations = append(view.Observations, o)
			}
		}
	}
	view.BlockingReasons = uniqueMongoCycleStrings(view.BlockingReasons)
	return view, nil
}

// The host must provide genuinely fresh paired snapshots. This does not call
// the point reader's FOR UPDATE-capable SQL Recheck and is not DROP approval.
func (b *MongoHistoricalOwnerBatch) RecheckBusiness(ctx context.Context, freshGlobal *MongoResponsibilitySnapshot, freshSQL *sqlevaluation.SQLHistoricalOwnerBatch) error {
	if b == nil || freshGlobal == nil || freshSQL == nil || b.global == freshGlobal || b.sql == freshSQL || b.global.report.IdentitySHA256 != freshGlobal.report.IdentitySHA256 || b.sql.Report().DatabaseIdentitySHA256 != freshSQL.Report().DatabaseIdentitySHA256 || b.sql.Report().BusinessRowsSHA256 != freshSQL.Report().BusinessRowsSHA256 || b.sql.Report().ResponsibilityCycleID == freshSQL.Report().ResponsibilityCycleID || b.global.txn.number == freshGlobal.txn.number && string(b.global.txn.session) == string(freshGlobal.txn.session) {
		return ErrMongoBatchInvalid
	}
	if err := freshGlobal.ValidateBorrowedSnapshot(ctx); err != nil {
		return err
	}
	if err := freshSQL.ValidateBorrowedSnapshot(ctx); err != nil {
		return err
	}
	tx, err := hostmysql.RequireTx(ctx)
	if err != nil || tx.Statement == nil || tx.Statement.ConnPool == b.sqlConnection {
		return ErrMongoBatchInvalid
	}
	current, err := PrepareMongoHistoricalOwnerBatch(ctx, freshGlobal, freshSQL, b.sourceHandles, b.limits)
	if err != nil {
		return err
	}
	if b.global.metadata.hash != freshGlobal.metadata.hash || !reflect.DeepEqual(b.data, current.data) || !reflect.DeepEqual(b.sqlAbsent, current.sqlAbsent) {
		return ErrMongoBatchConflict
	}
	return nil
}

func mongoBatchIDs(set map[uint64]bool) []uint64 {
	ids := make([]uint64, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
func viewForMongoBatchError() MongoSourceResponsibilityView {
	return MongoSourceResponsibilityView{SourceAuthenticationRequired: true, OriginalBusinessFactsRequired: true, SQLAIInboxCoverageRequired: true, WriterFenceRequired: true, CASRequired: true}
}

type mongoOwnerSourceSelection struct {
	sources        map[verifiedSourceKey]*DecodedSourceEvent
	sourceFactsSHA map[verifiedSourceKey][32]byte
	sqlOwners      map[verifiedSourceKey]*sqlevaluation.SQLHistoricalBatchOwnerFacts
	sqlAbsent      map[verifiedSourceKey]bool
	selection      mongoBatchSelection
}

// Source selectors and real SQL ownership are identical for transaction and
// snapshot-input readers. This helper grants no Mongo capability.
func selectMongoOwnerSources(sql *sqlevaluation.SQLHistoricalOwnerBatch, sources []*VerifiedSourceEvent) (*mongoOwnerSourceSelection, error) {
	selection := &mongoOwnerSourceSelection{sources: map[verifiedSourceKey]*DecodedSourceEvent{}, sourceFactsSHA: map[verifiedSourceKey][32]byte{}, sqlOwners: map[verifiedSourceKey]*sqlevaluation.SQLHistoricalBatchOwnerFacts{}, sqlAbsent: map[verifiedSourceKey]bool{}, selection: mongoBatchSelection{map[uint64]bool{}, map[uint64]bool{}, map[uint64]bool{}, map[uint64]bool{}, map[uint64]bool{}}}
	for _, handle := range sources {
		facts, err := handle.Facts()
		if err != nil {
			return nil, err
		}
		key, err := sourceAuthKey(facts.Source.Database, facts.Source.Object, facts.Source.PrimaryKeySHA256)
		if err != nil {
			return nil, err
		}
		if selection.sources[key] != nil {
			return nil, ErrMongoBatchInvalid
		}
		digest, err := privateFactsSHA(facts)
		if err != nil {
			return nil, err
		}
		selection.sourceFactsSHA[key] = digest
		var owner *sqlevaluation.SQLHistoricalBatchOwnerFacts
		switch facts.EventType {
		case "answersheet.submitted":
			checked, e := copyMongoSource(facts)
			if e != nil {
				return nil, e
			}
			facts = checked
			id, _ := mongoCycleStringID(facts.Submitted.AnswerSheetID)
			selection.selection.sheets[id] = true
			owner, err = sql.OwnerByAnswerSheet(id)
			if errors.Is(err, sqlevaluation.ErrSQLHistoricalOwnerAbsent) {
				selection.sqlAbsent[key] = true
				err = nil
			}
		case "interpretation.report.generated":
			checked, e := copyMongoSource(facts)
			if e != nil {
				return nil, e
			}
			facts = checked
			id, _ := mongoCycleStringID(facts.Generated.AssessmentID)
			owner, err = sql.OwnerByAssessment(id)
			for _, v := range []struct {
				text   string
				target map[uint64]bool
			}{{facts.Generated.GenerationID, selection.selection.generations}, {facts.Generated.OutcomeID, selection.selection.outcomes}, {facts.Generated.ReportID, selection.selection.artifacts}, {facts.Generated.RunID, selection.selection.runs}} {
				n, e := mongoCycleStringID(v.text)
				if e != nil {
					return nil, e
				}
				v.target[n] = true
			}
		case "evaluation.requested", "evaluation.retry.requested", "evaluation.outcome.committed", "evaluation.failed":
			id, e := sqlSourceAssessment(facts)
			if e != nil {
				return nil, e
			}
			owner, err = sql.OwnerByAssessment(id)
		default:
			return nil, ErrSourceEventType
		}
		if err != nil {
			return nil, err
		}
		if owner != nil {
			actual := owner.Snapshot()
			if actual.Owner.OrgID != facts.OrgID {
				return nil, ErrSourceOrganization
			}
			if actual.Owner.AnswerSheetID != 0 {
				selection.selection.sheets[actual.Owner.AnswerSheetID] = true
			}
			for _, outcome := range actual.Outcomes {
				if outcome.AssessmentID != actual.Owner.AssessmentID || outcome.OrgID != facts.OrgID || outcome.ID == 0 {
					return nil, ErrMongoBatchConflict
				}
				selection.selection.outcomes[outcome.ID] = true
			}
			selection.sqlOwners[key] = owner
		}
		selection.sources[key] = facts
	}
	return selection, nil
}
