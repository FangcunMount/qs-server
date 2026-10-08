package retirement

import (
	"bytes"
	"context"
	"sort"
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/retirementevidence"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const (
	ErrMongoCycleRead        SourceError = "mongo_responsibility_cycle_read_failed"
	ErrMongoCycleSchema      SourceError = "mongo_responsibility_cycle_schema_unknown"
	ErrMongoCycleBounds      SourceError = "mongo_responsibility_cycle_budget_exceeded"
	ErrMongoCycleTransaction SourceError = "mongo_responsibility_cycle_snapshot_required"
	ErrMongoCycleConflict    SourceError = "mongo_responsibility_cycle_fresh_conflict"
)

// Budgets are explicit host constraints, never approval or an estimated row
// count. Exceeding any budget fails the cycle; a partial cycle is not returned.
type MongoResponsibilityLimits struct {
	PageRows                                                    int
	MaxRows, MaxBytes, MaxPages, MaxGraphEntries, MaxGraphBytes uint64
	MaxDuration                                                 time.Duration
}

func (l MongoResponsibilityLimits) valid() bool {
	return l.PageRows > 0 && l.PageRows <= 4096 && l.MaxRows > 0 && l.MaxRows <= 2_000_000 && l.MaxBytes > 0 && l.MaxBytes <= 8<<30 && l.MaxPages > 0 && l.MaxPages <= 2_000_000 && l.MaxGraphEntries > 0 && l.MaxGraphEntries <= 4_000_000 && l.MaxGraphBytes > 0 && l.MaxGraphBytes <= 1<<30 && l.MaxDuration > 0 && l.MaxDuration <= 5*time.Minute
}

type MongoResponsibilityCollectionReport struct {
	Collection, UUID, IDType, LowerTokenSHA256, UpperTokenSHA256, RowsSHA256 string
	Present                                                                  bool
	Rows, Bytes, Pages                                                       uint64
}

// This safe report contains hashes/counts/categories only. It does not contain
// BSON tokens, event IDs, organizations, payloads, or original source approval.
type MongoResponsibilityCycleReport struct {
	Protocol, IdentitySHA256, MetadataSHA256, SnapshotSHA256                                                                             string
	MigrationVersion                                                                                                                     int64
	Collections                                                                                                                          []MongoResponsibilityCollectionReport
	Rows, Bytes, Pages, ClassifiedRows, GraphEntries, GraphBytes                                                                         uint64
	ClassCounts                                                                                                                          map[string]uint64
	BlockingReasons, CoverageGaps                                                                                                        []string
	Complete, LocalGraphChecked                                                                                                          bool
	SourceAuthenticationRequired, OriginalBusinessFactsRequired, SQLAIInboxCoverageRequired, WriterFenceRequired, CASRequired, DropReady bool
}

type MongoResponsibilityObservation struct {
	Collection, PrimaryKeySHA256, RowSHA256, Class, EventID, EventType, OwnerKind, OwnerID, BindingSHA256, ContentSHA256 string
	OrgID, AnswerSheetID, AssessmentID, GenerationID, OutcomeID, RunID                                                   uint64
	Unfinished, LeasePresent, Invalid, OwnerUnproven                                                                     bool
	Reasons                                                                                                              []string
}

func (MongoResponsibilityObservation) MarshalJSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}
func (MongoResponsibilityObservation) MarshalBSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}
func (MongoResponsibilityObservation) String() string {
	return "private Mongo responsibility observation"
}
func (v MongoResponsibilityObservation) GoString() string { return v.String() }

type mongoCycleTxn struct {
	session bson.Raw
	number  int64
}
type mongoCycleBoundary struct {
	report       MongoResponsibilityCollectionReport
	lower, upper bson.RawValue
}

// This object borrows a live host snapshot. It never opens, commits, aborts or
// closes a session/client. Private graph indexes are produced by one global
// scan, not by repeatedly querying RM for each historical AnswerSheet.
type MongoResponsibilitySnapshot struct {
	db                                                      *mongo.Database
	config                                                  MongoOwnerConfig
	limits                                                  MongoResponsibilityLimits
	txn                                                     mongoCycleTxn
	metadata                                                mongoCycleMetadata
	boundaries                                              map[string]mongoCycleBoundary
	observations                                            []MongoResponsibilityObservation
	byEvent, bySheet, byAssessment, byGeneration, byOutcome map[string][]int
	graph                                                   mongoCycleGraph
	report                                                  MongoResponsibilityCycleReport
	started                                                 time.Time
}

func (*MongoResponsibilitySnapshot) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*MongoResponsibilitySnapshot) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*MongoResponsibilitySnapshot) String() string {
	return "private global Mongo responsibility snapshot; not retirement approval"
}
func (s *MongoResponsibilitySnapshot) GoString() string { return s.String() }

func mongoCycleTransaction(ctx context.Context, db *mongo.Database) (mongoCycleTxn, error) {
	if db == nil || retirementevidence.RequireTransaction(ctx, db.Collection("rm_outbox")) != nil {
		return mongoCycleTxn{}, ErrMongoCycleTransaction
	}
	s := mongo.SessionFromContext(ctx)
	// Public Session exposes neither current read concern nor transaction
	// number. The pinned SDK/driver XSession contract proves both, rather
	// than trusting an editable "fresh" boolean or only a session ID.
	x, ok := s.(mongo.XSession) //nolint:staticcheck // pinned SDK already uses this actual transaction accessor
	if !ok || x.ClientSession() == nil || !x.ClientSession().TransactionRunning() || x.ClientSession().CurrentRc == nil || x.ClientSession().CurrentRc.Level != "snapshot" {
		return mongoCycleTxn{}, ErrMongoCycleTransaction
	}
	return mongoCycleTxn{session: append(bson.Raw(nil), s.ID()...), number: x.ClientSession().TxnNumber}, nil
}

func PrepareMongoResponsibilitySnapshot(ctx context.Context, db *mongo.Database, config MongoOwnerConfig, limits MongoResponsibilityLimits) (*MongoResponsibilitySnapshot, error) {
	if !limits.valid() || !evidence.ValidSHA256(config.ExpectedIdentityHash) || config.ExpectedMigrationVersion <= 0 {
		return nil, ErrMongoCycleBounds
	}
	txn, err := mongoCycleTransaction(ctx, db)
	if err != nil {
		return nil, err
	}
	s := &MongoResponsibilitySnapshot{db: db, config: config, limits: limits, txn: txn, started: time.Now(), boundaries: map[string]mongoCycleBoundary{}, byEvent: map[string][]int{}, bySheet: map[string][]int{}, byAssessment: map[string][]int{}, byGeneration: map[string][]int{}, byOutcome: map[string][]int{}}
	s.graph.initialize()
	s.report = MongoResponsibilityCycleReport{Protocol: "mongo-global-responsibility-cycle/v1", MigrationVersion: config.ExpectedMigrationVersion, ClassCounts: map[string]uint64{}, SourceAuthenticationRequired: true, OriginalBusinessFactsRequired: true, SQLAIInboxCoverageRequired: true, WriterFenceRequired: true, CASRequired: true, CoverageGaps: []string{"sql_ai_generation_artifact_runtime_inbox_held_deadletter_require_separate_actual_cycle", "approved_complete_legacy_source_authentication_not_bound", "original_cross_database_business_closure_not_bound", "production_writer_fence_and_final_cas_not_bound"}}
	s.metadata, err = observeMongoCycleMetadata(ctx, db, config)
	if err != nil {
		return nil, err
	}
	s.report.IdentitySHA256, s.report.MetadataSHA256 = s.metadata.identity, s.metadata.hash
	for _, name := range s.metadata.unknown {
		s.gap("unknown_catalog_namespace_coverage_required")
		_ = name
	}
	for _, name := range mongoCycleCollections {
		b, e := s.scanCollection(ctx, name, nil, true)
		if e != nil {
			return nil, e
		}
		s.boundaries[name] = b
		s.report.Collections = append(s.report.Collections, b.report)
	}
	if err = s.ValidateBorrowedSnapshot(ctx); err != nil {
		return nil, err
	}
	// Metadata is outside Mongo transactions; bracket row scans with actual
	// same-client metadata reads and exact immutable schema/UUID comparison.
	end, err := observeMongoCycleMetadata(ctx, db, config)
	if err != nil {
		return nil, err
	}
	if end.hash != s.metadata.hash {
		return nil, ErrMongoCycleConflict
	}
	if err = s.classifyGraph(); err != nil {
		return nil, err
	}
	s.report.Complete, s.report.LocalGraphChecked = true, true
	s.report.SnapshotSHA256 = s.snapshotHash()
	sort.Strings(s.report.BlockingReasons)
	sort.Strings(s.report.CoverageGaps)
	return s, nil
}

func (s *MongoResponsibilitySnapshot) ValidateBorrowedSnapshot(ctx context.Context) error {
	if s == nil {
		return ErrMongoCycleTransaction
	}
	t, err := mongoCycleTransaction(ctx, s.db)
	if err != nil || t.number != s.txn.number || !bytes.Equal(t.session, s.txn.session) {
		return ErrMongoCycleTransaction
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if time.Since(s.started) > s.limits.MaxDuration {
		return ErrMongoCycleBounds
	}
	return nil
}

func (s *MongoResponsibilitySnapshot) Report() MongoResponsibilityCycleReport {
	if s == nil {
		return MongoResponsibilityCycleReport{SourceAuthenticationRequired: true, OriginalBusinessFactsRequired: true, SQLAIInboxCoverageRequired: true, WriterFenceRequired: true, CASRequired: true}
	}
	r := s.report
	r.Collections = append([]MongoResponsibilityCollectionReport(nil), r.Collections...)
	r.BlockingReasons = append([]string(nil), r.BlockingReasons...)
	r.CoverageGaps = append([]string(nil), r.CoverageGaps...)
	r.ClassCounts = map[string]uint64{}
	for k, v := range s.report.ClassCounts {
		r.ClassCounts[k] = v
	}
	return r
}

type MongoSourceResponsibilityView struct {
	Observations                                                                                                              []MongoResponsibilityObservation
	BlockingReasons                                                                                                           []string
	SourceAuthenticationRequired, OriginalBusinessFactsRequired, SQLAIInboxCoverageRequired, WriterFenceRequired, CASRequired bool
}

func (MongoSourceResponsibilityView) MarshalJSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}
func (MongoSourceResponsibilityView) MarshalBSON() ([]byte, error) {
	return nil, ErrSourceSerialization
}
func (MongoSourceResponsibilityView) String() string {
	return "private Mongo source responsibility lookup; source is untrusted"
}
func (v MongoSourceResponsibilityView) GoString() string { return v.String() }

// No I/O occurs here. The source DTO remains untrusted; authenticated complete
// source copies and original business facts are separate coordinator inputs.
func (s *MongoResponsibilitySnapshot) ForUntrustedSource(ctx context.Context, source *DecodedSourceEvent) (MongoSourceResponsibilityView, error) {
	v := MongoSourceResponsibilityView{SourceAuthenticationRequired: true, OriginalBusinessFactsRequired: true, SQLAIInboxCoverageRequired: true, WriterFenceRequired: true, CASRequired: true}
	if s == nil || !s.report.Complete || source == nil || source.OrgID == 0 || source.EventID == "" {
		return v, ErrMongoCycleSchema
	}
	switch source.EventType {
	case "answersheet.submitted", "interpretation.report.generated", "evaluation.requested", "evaluation.retry.requested", "evaluation.outcome.committed", "evaluation.failed":
	default:
		return v, ErrSourceEventType
	}
	if err := ctx.Err(); err != nil {
		return v, err
	}
	seen := map[int]bool{}
	add := func(ids []int) {
		for _, i := range ids {
			if seen[i] {
				continue
			}
			seen[i] = true
			o := s.observations[i]
			o.Reasons = append([]string(nil), o.Reasons...)
			v.Observations = append(v.Observations, o)
			if o.OrgID != 0 && o.OrgID != source.OrgID || o.Invalid || o.Unfinished || o.LeasePresent {
				v.BlockingReasons = append(v.BlockingReasons, "source_linked_mongo_responsibility_unclosed_or_conflicting")
			}
			if o.OwnerUnproven {
				v.BlockingReasons = append(v.BlockingReasons, "source_linked_mongo_owner_or_cross_database_coverage_unknown")
			}
			if o.EventID == source.EventID && o.EventType != "" && (o.EventType != source.EventType || o.OwnerKind != source.AggregateType || o.OwnerID != source.AggregateID) {
				v.BlockingReasons = append(v.BlockingReasons, "original_event_identity_collision")
			}
			if o.EventID == source.EventID && o.ContentSHA256 != "" && o.ContentSHA256 != source.ContentDigest.SHA256 {
				v.BlockingReasons = append(v.BlockingReasons, "original_event_immutable_content_collision")
			}
		}
	}
	add(s.byEvent[source.EventID])
	for k, id := range source.BusinessIDs {
		switch k {
		case "answer_sheet_id", "answersheet_id":
			add(s.bySheet[id])
		case "assessment_id":
			add(s.byAssessment[id])
		case "generation_id":
			add(s.byGeneration[id])
		case "outcome_id":
			add(s.byOutcome[id])
		}
	}
	switch source.AggregateType {
	case "AnswerSheet":
		add(s.bySheet[source.AggregateID])
	case "Assessment":
		add(s.byAssessment[source.AggregateID])
	case "ReportGeneration":
		add(s.byGeneration[source.AggregateID])
	}
	v.BlockingReasons = uniqueMongoCycleStrings(v.BlockingReasons)
	return v, nil
}

type MongoResponsibilityFreshReport struct {
	FreshActualTransaction, MetadataUnchanged, OldBoundRowsUnchanged, NoNewRows, EntireSnapshotUnchanged, DropReady bool
	SnapshotSHA256                                                                                                  string
}

func (s *MongoResponsibilitySnapshot) RecheckFresh(ctx context.Context) (MongoResponsibilityFreshReport, error) {
	r := MongoResponsibilityFreshReport{}
	if s == nil || !s.report.Complete {
		return r, ErrMongoCycleConflict
	}
	t, err := mongoCycleTransaction(ctx, s.db)
	if err != nil {
		return r, err
	}
	if t.number == s.txn.number && bytes.Equal(t.session, s.txn.session) {
		return r, ErrMongoCycleTransaction
	}
	r.FreshActualTransaction = true
	fresh, err := PrepareMongoResponsibilitySnapshot(ctx, s.db, s.config, s.limits)
	if err != nil {
		return r, err
	}
	r.MetadataUnchanged = fresh.metadata.hash == s.metadata.hash
	r.EntireSnapshotUnchanged = fresh.report.SnapshotSHA256 == s.report.SnapshotSHA256
	r.SnapshotSHA256 = fresh.report.SnapshotSHA256
	r.OldBoundRowsUnchanged, r.NoNewRows = true, true
	for _, name := range mongoCycleCollections {
		old := s.boundaries[name]
		now := fresh.boundaries[name]
		if old.report.Present != now.report.Present {
			r.OldBoundRowsUnchanged = false
			continue
		}
		b, e := fresh.scanCollection(ctx, name, &old, false)
		if e != nil {
			return r, e
		}
		if b.report.Rows != old.report.Rows || b.report.RowsSHA256 != old.report.RowsSHA256 {
			r.OldBoundRowsUnchanged = false
		}
		outside, e := fresh.hasOutsideRows(ctx, name, old)
		if e != nil {
			return r, e
		}
		if outside {
			r.NoNewRows = false
		}
	}
	end, err := observeMongoCycleMetadata(ctx, s.db, s.config)
	if err != nil {
		return r, err
	}
	if end.hash != s.metadata.hash {
		r.MetadataUnchanged = false
	}
	if !r.MetadataUnchanged || !r.OldBoundRowsUnchanged || !r.NoNewRows || !r.EntireSnapshotUnchanged {
		return r, ErrMongoCycleConflict
	}
	return r, nil
}

func (s *MongoResponsibilitySnapshot) block(reason string) {
	s.report.BlockingReasons = uniqueMongoCycleStrings(append(s.report.BlockingReasons, reason))
}
func (s *MongoResponsibilitySnapshot) gap(reason string) {
	s.report.CoverageGaps = uniqueMongoCycleStrings(append(s.report.CoverageGaps, reason))
}
func uniqueMongoCycleStrings(values []string) []string {
	result := []string{}
	seen := map[string]bool{}
	for _, v := range values {
		if !seen[v] {
			result = append(result, v)
			seen[v] = true
		}
	}
	sort.Strings(result)
	return result
}
func mongoCycleKey(id uint64) string { return strconv.FormatUint(id, 10) }
