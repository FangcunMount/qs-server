package retirement

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/retirementevidence"
	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	eventoutcome "github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const (
	ErrMongoOwnerResolution SourceError = "mongo_historical_owner_resolution_rejected"
	ErrMongoOwnerRead       SourceError = "mongo_historical_owner_read_failed"
	ErrMongoOwnerIdentity   SourceError = "mongo_historical_database_identity_conflict"
	ErrMongoOwnerConflict   SourceError = "mongo_historical_owner_snapshot_conflict"
	ErrMongoOwnerBounds     SourceError = "mongo_historical_owner_bound_exceeded"
	MongoOwnerRowLimit                  = 256
	MongoOwnerByteLimit                 = 32 << 20
)

// MongoOwnerConfig is a local read constraint, not an approval or a source seal.
// ExpectedIdentityHash must come from the independently approved inventory.
type MongoOwnerConfig struct {
	ExpectedIdentityHash     string
	ExpectedMigrationVersion int64
}

// MongoLocalResolution never certifies complete source authentication, SQL
// held/dead-letter/Inbox stores, global orphan coverage, or permission to DROP.
type MongoLocalResolution struct {
	EventID, EventType, BusinessBindingSHA256                                                                                     string
	OrgID, TesteeID, AnswerSheetID, AssessmentID, GenerationID, ReportID, OutcomeID                                               uint64
	OriginalRun                                                                                                                   *evidence.HistoricalRunReferenceV1
	SQLSubmissionClock                                                                                                            *MongoSQLSubmissionClockFact
	FrozenAdmissionPurpose                                                                                                        string
	OwnerLocalTerminal                                                                                                            bool
	CurrentResponsibilityCount                                                                                                    int
	BlockingReasons, Gaps                                                                                                         []string
	SourceAuthenticationRequired, SQLCrossClosureRequired, SQLResponsibilityRequired, GlobalUnboundResponsibilityCoverageRequired bool
}

// Storage precision is a separate local fact, never complete original-time
// verification or an authorization to change the stored SQL clock.
type MongoSQLSubmissionClockFact struct {
	DataType                         string
	Precision                        int
	ActualSQLTime, OriginalMongoTime time.Time
	ComparisonRule                   string
	ExactBusinessMilliseconds        bool
}

func (MongoLocalResolution) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (MongoLocalResolution) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (MongoLocalResolution) String() string {
	return "private Mongo-local historical resolution; not retirement approval"
}
func (v MongoLocalResolution) GoString() string { return v.String() }

type mongoOwnerRead struct {
	collection string
	filter     bson.D
	rows       []bson.Raw
}

// MongoOwnerResolution owns no connection/session/transaction. Its private raw
// BSON and source copy are only local baselines; callers cannot edit them.
type MongoOwnerResolution struct {
	db               *mongo.Database
	config           MongoOwnerConfig
	source           *DecodedSourceEvent
	sqlFacts         mongoSQLBusinessFacts
	legacySQLFacts   *sqlevaluation.SQLHistoricalOwnerFacts
	businessReader   mongoBusinessReader
	metadata         mongoOwnerMetadata
	reads            []mongoOwnerRead
	bytes            int
	local            MongoLocalResolution
	expectedStandard []evidence.StandardReference
}

func (*MongoOwnerResolution) MarshalJSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*MongoOwnerResolution) MarshalBSON() ([]byte, error) { return nil, ErrSourceSerialization }
func (*MongoOwnerResolution) String() string {
	return "private opaque Mongo-local qualification; not retirement approval"
}
func (r *MongoOwnerResolution) GoString() string { return r.String() }

func ResolveMongoOwner(ctx context.Context, db *mongo.Database, config MongoOwnerConfig, source *DecodedSourceEvent, sqlFacts *sqlevaluation.SQLHistoricalOwnerFacts) (*MongoOwnerResolution, error) {
	if db == nil || config.ExpectedMigrationVersion <= 0 || !evidence.ValidSHA256(config.ExpectedIdentityHash) {
		return nil, ErrMongoOwnerIdentity
	}
	if err := retirementevidence.RequireTransaction(ctx, db.Collection("rm_outbox")); err != nil {
		return nil, err
	}
	copy, err := copyMongoSource(source)
	if err != nil {
		return nil, err
	}
	metadata, err := observeMongoOwnerMetadata(ctx, db, config)
	if err != nil {
		return nil, err
	}
	r := &MongoOwnerResolution{db: db, config: config, source: copy, legacySQLFacts: sqlFacts, metadata: metadata}
	if sqlFacts != nil {
		r.sqlFacts = sqlFacts
	}
	r.local = MongoLocalResolution{EventID: copy.EventID, EventType: copy.EventType, OrgID: copy.OrgID, SourceAuthenticationRequired: true, SQLCrossClosureRequired: true, SQLResponsibilityRequired: true, GlobalUnboundResponsibilityCoverageRequired: true}
	switch copy.EventType {
	case "answersheet.submitted":
		err = r.readSubmission(ctx)
	case "interpretation.report.generated":
		err = r.readGenerated(ctx)
	default:
		err = ErrSourceEventType
	}
	if err != nil {
		return nil, err
	}
	if err = r.readMongoResponsibilities(ctx); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *MongoOwnerResolution) Local() MongoLocalResolution {
	if r == nil {
		return MongoLocalResolution{BlockingReasons: []string{"resolution_absent"}, SourceAuthenticationRequired: true, SQLCrossClosureRequired: true, SQLResponsibilityRequired: true, GlobalUnboundResponsibilityCoverageRequired: true}
	}
	v := r.local
	v.BlockingReasons = append([]string(nil), v.BlockingReasons...)
	v.Gaps = append([]string(nil), v.Gaps...)
	if v.OriginalRun != nil {
		clone := *v.OriginalRun
		v.OriginalRun = &clone
	}
	if v.SQLSubmissionClock != nil {
		clone := *v.SQLSubmissionClock
		v.SQLSubmissionClock = &clone
	}
	return v
}

// RecheckMongo requires another actual host transaction. A host must establish
// a fresh snapshot after its maintenance fence; repeating a transaction's
// snapshot cannot prove concurrent writers stopped. SQL recheck is separate.
func (r *MongoOwnerResolution) RecheckMongo(ctx context.Context) error {
	if r == nil {
		return ErrMongoOwnerResolution
	}
	current, err := ResolveMongoOwner(ctx, r.db, r.config, r.source, r.legacySQLFacts)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(r.metadata, current.metadata) || !reflect.DeepEqual(r.reads, current.reads) || !reflect.DeepEqual(r.local, current.local) {
		return ErrMongoOwnerConflict
	}
	return nil
}

func (r *MongoOwnerResolution) RecheckSQL(ctx context.Context) error {
	if r == nil || r.legacySQLFacts == nil {
		return ErrMongoOwnerResolution
	}
	return r.legacySQLFacts.Recheck(ctx)
}

func copyMongoSource(v *DecodedSourceEvent) (*DecodedSourceEvent, error) {
	if v == nil || v.Source.Database != "mongodb" || v.Source.Object != "domain_event_outbox" || (v.Source.PrimaryKeyKind != "mongodb_objectid" && v.Source.PrimaryKeyKind != "mongodb_raw_bson_v1") || v.Source.Digest.Kind != MongoRowDigestKind || v.ContentDigest.Kind != ContentDigestKind || v.SupportedSchema != "legacy-domain-json-v1" || !evidence.ValidSHA256(v.Source.PrimaryKeySHA256) || !evidence.ValidSHA256(v.Source.Digest.SHA256) || !evidence.ValidSHA256(v.ContentDigest.SHA256) || v.OrgID == 0 || v.OrgID > 1<<63-1 || !identifier(v.EventID, 64) || v.BusinessAt.IsZero() {
		return nil, ErrMongoOwnerResolution
	}
	c := *v
	c.BusinessIDs = nil
	c.ResolverGaps = nil
	c.OriginalRun.Missing = nil
	if v.OuterOrgID != nil {
		n := *v.OuterOrgID
		if n <= 0 || uint64(n) != v.OrgID {
			return nil, ErrMongoOwnerResolution
		}
		c.OuterOrgID = &n
	}
	clone := func(in, out any) error {
		raw, err := json.Marshal(in)
		if err != nil {
			return ErrMongoOwnerResolution
		}
		if err = json.Unmarshal(raw, out); err != nil {
			return ErrMongoOwnerResolution
		}
		return nil
	}
	switch v.EventType {
	case "answersheet.submitted":
		if v.Submitted == nil || v.Generated != nil || v.Requested != nil || v.Failed != nil || v.OutcomeCommitted != nil || v.AggregateType != "AnswerSheet" || v.AggregateID != v.Submitted.AnswerSheetID || v.Submitted.OrgID != v.OrgID || !BusinessTimeEqual(v.BusinessAt, v.Submitted.SubmittedAt) {
			return nil, ErrMongoOwnerResolution
		}
		id, e := strconv.ParseUint(v.Submitted.AnswerSheetID, 10, 64)
		if e != nil || id == 0 || strconv.FormatUint(id, 10) != v.Submitted.AnswerSheetID {
			return nil, ErrMongoOwnerResolution
		}
		c.Submitted = new(eventpayload.AnswerSheetSubmittedData)
		if err := clone(v.Submitted, c.Submitted); err != nil {
			return nil, err
		}
	case "interpretation.report.generated":
		if v.Generated == nil || v.Submitted != nil || v.Requested != nil || v.Failed != nil || v.OutcomeCommitted != nil || v.AggregateType != "ReportGeneration" || v.AggregateID != v.Generated.GenerationID || v.Generated.OrgID <= 0 || uint64(v.Generated.OrgID) != v.OrgID || !BusinessTimeEqual(v.BusinessAt, v.Generated.GeneratedAt) {
			return nil, ErrMongoOwnerResolution
		}
		for _, text := range []string{v.Generated.GenerationID, v.Generated.RunID, v.Generated.ReportID, v.Generated.OutcomeID, v.Generated.AssessmentID} {
			n, e := strconv.ParseUint(text, 10, 64)
			if e != nil || n == 0 || strconv.FormatUint(n, 10) != text {
				return nil, ErrMongoOwnerResolution
			}
		}
		c.Generated = new(eventoutcome.ReportGeneratedPayload)
		if err := clone(v.Generated, c.Generated); err != nil {
			return nil, err
		}
	default:
		return nil, ErrSourceEventType
	}
	return &c, nil
}
