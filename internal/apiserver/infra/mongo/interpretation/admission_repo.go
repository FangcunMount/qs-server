package interpretation

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/admission"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

const admissionFailureCollection = "interpretation_admission_failures"

// AdmissionFailurePO is the Mongo document for lifecycle-front admission evidence.
type AdmissionFailurePO struct {
	DomainID       uint64    `bson:"domain_id"`
	OutcomeID      uint64    `bson:"outcome_id"`
	OrgID          int64     `bson:"org_id,omitempty"`
	AssessmentID   uint64    `bson:"assessment_id,omitempty"`
	TesteeID       uint64    `bson:"testee_id,omitempty"`
	EventID        string    `bson:"event_id,omitempty"`
	TraceID        string    `bson:"trace_id,omitempty"`
	Kind           string    `bson:"kind"`
	Code           string    `bson:"code"`
	SafeMessage    string    `bson:"safe_message"`
	Retryable      bool      `bson:"retryable"`
	Fingerprint    string    `bson:"fingerprint"`
	GenerationID   uint64    `bson:"generation_id,omitempty"`
	OutcomeVersion string    `bson:"outcome_version,omitempty"`
	Attempt        uint      `bson:"attempt"`
	Decision       string    `bson:"decision"`
	FirstFailedAt  time.Time `bson:"first_failed_at"`
	LastFailedAt   time.Time `bson:"last_failed_at"`
	OccurredAt     time.Time `bson:"occurred_at"`
}

func (AdmissionFailurePO) CollectionName() string { return admissionFailureCollection }

// AdmissionFailureRepository persists AdmissionFailure evidence.
type AdmissionFailureRepository struct {
	base.BaseRepository
}

func NewAdmissionFailureRepository(db *mongo.Database, opts ...base.BaseRepositoryOptions) (*AdmissionFailureRepository, error) {
	repo := &AdmissionFailureRepository{BaseRepository: base.NewBaseRepository(db, admissionFailureCollection, opts...)}
	if _, err := repo.Collection().Indexes().CreateMany(context.Background(), admissionFailureIndexModels()); err != nil {
		return nil, fmt.Errorf("create interpretation admission failure indexes: %w", err)
	}
	return repo, nil
}

func admissionFailureIndexModels() []mongo.IndexModel {
	return []mongo.IndexModel{
		{Keys: bson.D{{Key: "domain_id", Value: 1}}, Options: options.Index().SetName("uk_admission_failure_domain_id").SetUnique(true)},
		{Keys: bson.D{{Key: "fingerprint", Value: 1}}, Options: options.Index().SetName("uk_admission_failure_fingerprint").SetUnique(true)},
		{Keys: bson.D{{Key: "outcome_id", Value: 1}, {Key: "occurred_at", Value: -1}}, Options: options.Index().SetName("idx_admission_failure_outcome_occurred")},
		{Keys: bson.D{{Key: "org_id", Value: 1}, {Key: "kind", Value: 1}, {Key: "decision", Value: 1}, {Key: "occurred_at", Value: -1}, {Key: "domain_id", Value: -1}}, Options: options.Index().SetName("idx_admission_failure_operations")},
	}
}

var _ admission.QueryRepository = (*AdmissionFailureRepository)(nil)

var _ admission.Repository = (*AdmissionFailureRepository)(nil)

func (r *AdmissionFailureRepository) UpsertByFingerprint(ctx context.Context, failure *admission.Failure) (bool, error) {
	if failure == nil {
		return false, fmt.Errorf("admission failure is required")
	}
	po := admissionFailureToPO(failure)
	insert := *po
	insert.Attempt = 0
	insert.LastFailedAt = time.Time{}
	res, err := r.Collection().UpdateOne(ctx,
		bson.M{"fingerprint": po.Fingerprint},
		bson.M{
			"$setOnInsert": insert,
			"$inc":         bson.M{"attempt": 1},
			"$set": bson.M{
				"last_failed_at": po.LastFailedAt,
				"trace_id":       po.TraceID,
			},
		},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return false, nil
		}
		return false, fmt.Errorf("upsert admission failure: %w", err)
	}
	return res.UpsertedCount > 0, nil
}

func (r *AdmissionFailureRepository) FindByFingerprint(ctx context.Context, fingerprint string) (*admission.Failure, error) {
	var po AdmissionFailurePO
	if err := r.FindOne(ctx, bson.M{"fingerprint": fingerprint}, &po); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, admission.ErrNotFound
		}
		return nil, fmt.Errorf("find admission failure by fingerprint: %w", err)
	}
	return admissionFailureToDomain(&po)
}

func (r *AdmissionFailureRepository) FindByOutcomeID(ctx context.Context, outcomeID meta.ID, limit int) ([]*admission.Failure, error) {
	if limit <= 0 {
		limit = 20
	}
	cur, err := r.Collection().Find(ctx, bson.M{"outcome_id": outcomeID.Uint64()}, options.Find().SetSort(bson.D{{Key: "occurred_at", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, fmt.Errorf("list admission failures: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }()
	items := make([]*admission.Failure, 0)
	for cur.Next(ctx) {
		var po AdmissionFailurePO
		if err := cur.Decode(&po); err != nil {
			return nil, err
		}
		item, err := admissionFailureToDomain(&po)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, cur.Err()
}

func (r *AdmissionFailureRepository) ListFailures(
	ctx context.Context,
	filter admission.QueryFilter,
	cursor string,
	limit int,
) (admission.QueryPage, error) {
	if filter.OrgID == 0 {
		return admission.QueryPage{}, fmt.Errorf("admission failure query requires organization")
	}
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	query := bson.M{"org_id": filter.OrgID}
	if filter.Kind != nil {
		query["kind"] = string(*filter.Kind)
	}
	if filter.Decision != "" {
		query["decision"] = filter.Decision
	}
	if filter.AssessmentID != nil {
		query["assessment_id"] = filter.AssessmentID.Uint64()
	}
	if filter.OutcomeID != nil {
		query["outcome_id"] = filter.OutcomeID.Uint64()
	}
	timeRange := bson.M{}
	if filter.OccurredFrom != nil {
		timeRange["$gte"] = filter.OccurredFrom.UTC()
	}
	if filter.OccurredTo != nil {
		timeRange["$lte"] = filter.OccurredTo.UTC()
	}
	if len(timeRange) > 0 {
		query["occurred_at"] = timeRange
	}
	cursorAt, cursorID, err := decodeAdmissionCursor(cursor)
	if err != nil {
		return admission.QueryPage{}, err
	}
	if !cursorAt.IsZero() {
		query["$or"] = []bson.M{
			{"occurred_at": bson.M{"$lt": cursorAt}},
			{"occurred_at": cursorAt, "domain_id": bson.M{"$lt": cursorID}},
		}
	}
	cur, err := r.Collection().Find(ctx, query, options.Find().
		SetSort(bson.D{{Key: "occurred_at", Value: -1}, {Key: "domain_id", Value: -1}}).
		SetLimit(int64(limit+1)))
	if err != nil {
		return admission.QueryPage{}, fmt.Errorf("query admission failures: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }()
	items := make([]*admission.Failure, 0, limit+1)
	for cur.Next(ctx) {
		var po AdmissionFailurePO
		if err := cur.Decode(&po); err != nil {
			return admission.QueryPage{}, err
		}
		item, err := admissionFailureToDomain(&po)
		if err != nil {
			return admission.QueryPage{}, err
		}
		items = append(items, item)
	}
	if err := cur.Err(); err != nil {
		return admission.QueryPage{}, err
	}
	page := admission.QueryPage{Items: items}
	if len(items) > limit {
		last := items[limit-1]
		page.Items = items[:limit]
		page.NextCursor = encodeAdmissionCursor(last.OccurredAt(), last.ID().Uint64())
	}
	return page, nil
}

func encodeAdmissionCursor(at time.Time, id uint64) string {
	raw := strconv.FormatInt(at.UTC().UnixNano(), 10) + ":" + strconv.FormatUint(id, 10)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeAdmissionCursor(value string) (time.Time, uint64, error) {
	if value == "" {
		return time.Time{}, 0, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("invalid admission failure cursor")
	}
	parts := strings.Split(string(raw), ":")
	if len(parts) != 2 {
		return time.Time{}, 0, fmt.Errorf("invalid admission failure cursor")
	}
	nanos, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("invalid admission failure cursor")
	}
	id, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil || id == 0 {
		return time.Time{}, 0, fmt.Errorf("invalid admission failure cursor")
	}
	return time.Unix(0, nanos).UTC(), id, nil
}

func admissionFailureToPO(failure *admission.Failure) *AdmissionFailurePO {
	return &AdmissionFailurePO{
		DomainID: failure.ID().Uint64(), OutcomeID: failure.OutcomeID().Uint64(), OrgID: failure.OrgID(),
		AssessmentID: failure.AssessmentID().Uint64(), TesteeID: failure.TesteeID(), EventID: failure.EventID(),
		TraceID: failure.TraceID(), Kind: string(failure.Kind()), Code: failure.Code(), SafeMessage: failure.SafeMessage(),
		Retryable: failure.Retryable(), Fingerprint: failure.Fingerprint(),
		GenerationID: failure.GenerationID().Uint64(), OutcomeVersion: failure.OutcomeVersion(),
		Attempt: failure.Attempt(), Decision: failure.Decision(),
		FirstFailedAt: failure.FirstFailedAt(), LastFailedAt: failure.LastFailedAt(),
		OccurredAt: failure.OccurredAt(),
	}
}

func admissionFailureToDomain(po *AdmissionFailurePO) (*admission.Failure, error) {
	return admission.NewFailure(admission.Input{
		ID: meta.FromUint64(po.DomainID), OutcomeID: meta.FromUint64(po.OutcomeID), OrgID: po.OrgID,
		AssessmentID: meta.FromUint64(po.AssessmentID), TesteeID: po.TesteeID, EventID: po.EventID, TraceID: po.TraceID,
		Kind: admission.Kind(po.Kind), Code: po.Code, SafeMessage: po.SafeMessage, Retryable: po.Retryable, OccurredAt: po.OccurredAt,
		GenerationID: meta.FromUint64(po.GenerationID), OutcomeVersion: po.OutcomeVersion,
		Attempt: po.Attempt, Decision: po.Decision, FirstFailedAt: po.FirstFailedAt, LastFailedAt: po.LastFailedAt,
	})
}
