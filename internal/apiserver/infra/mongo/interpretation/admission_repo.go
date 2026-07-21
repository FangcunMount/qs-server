package interpretation

import (
	"context"
	"errors"
	"fmt"
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
	DomainID     uint64    `bson:"domain_id"`
	OutcomeID    uint64    `bson:"outcome_id"`
	OrgID        int64     `bson:"org_id,omitempty"`
	AssessmentID uint64    `bson:"assessment_id,omitempty"`
	TesteeID     uint64    `bson:"testee_id,omitempty"`
	EventID      string    `bson:"event_id,omitempty"`
	TraceID      string    `bson:"trace_id,omitempty"`
	Kind         string    `bson:"kind"`
	Code         string    `bson:"code"`
	SafeMessage  string    `bson:"safe_message"`
	Retryable    bool      `bson:"retryable"`
	Fingerprint  string    `bson:"fingerprint"`
	OccurredAt   time.Time `bson:"occurred_at"`
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
	}
}

var _ admission.Repository = (*AdmissionFailureRepository)(nil)

func (r *AdmissionFailureRepository) UpsertByFingerprint(ctx context.Context, failure *admission.Failure) (bool, error) {
	if failure == nil {
		return false, fmt.Errorf("admission failure is required")
	}
	po := admissionFailureToPO(failure)
	res, err := r.Collection().UpdateOne(ctx,
		bson.M{"fingerprint": po.Fingerprint},
		bson.M{"$setOnInsert": po},
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
	defer cur.Close(ctx)
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

func admissionFailureToPO(failure *admission.Failure) *AdmissionFailurePO {
	return &AdmissionFailurePO{
		DomainID: failure.ID().Uint64(), OutcomeID: failure.OutcomeID().Uint64(), OrgID: failure.OrgID(),
		AssessmentID: failure.AssessmentID().Uint64(), TesteeID: failure.TesteeID(), EventID: failure.EventID(),
		TraceID: failure.TraceID(), Kind: string(failure.Kind()), Code: failure.Code(), SafeMessage: failure.SafeMessage(),
		Retryable: failure.Retryable(), Fingerprint: failure.Fingerprint(), OccurredAt: failure.OccurredAt(),
	}
}

func admissionFailureToDomain(po *AdmissionFailurePO) (*admission.Failure, error) {
	return admission.NewFailure(admission.Input{
		ID: meta.FromUint64(po.DomainID), OutcomeID: meta.FromUint64(po.OutcomeID), OrgID: po.OrgID,
		AssessmentID: meta.FromUint64(po.AssessmentID), TesteeID: po.TesteeID, EventID: po.EventID, TraceID: po.TraceID,
		Kind: admission.Kind(po.Kind), Code: po.Code, SafeMessage: po.SafeMessage, Retryable: po.Retryable, OccurredAt: po.OccurredAt,
	})
}
