package assessmentmodel

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/assessmentmodel"
)

type Repository struct {
	mongoBase.BaseRepository
	mapper *Mapper
}

var (
	_ port.PublishedReader = (*Repository)(nil)
	_ port.PublishedWriter = (*Repository)(nil)
)

func NewRepository(db *mongo.Database, opts ...mongoBase.BaseRepositoryOptions) *Repository {
	po := &PublishedAssessmentModelPO{}
	return &Repository{
		BaseRepository: mongoBase.NewBaseRepository(db, po.CollectionName(), opts...),
		mapper:         NewMapper(),
	}
}

func (r *Repository) UpsertPublished(ctx context.Context, snapshot *domain.Snapshot) error {
	if snapshot == nil {
		return mongo.ErrNilDocument
	}
	published := domain.PublishedFromLegacy(snapshot)
	if published == nil {
		return mongo.ErrNilDocument
	}
	return r.upsertPublishedModel(ctx, published)
}

func (r *Repository) UpsertPublishedModel(ctx context.Context, snapshot *domain.PublishedModelSnapshot) error {
	if snapshot == nil {
		return mongo.ErrNilDocument
	}
	return r.upsertPublishedModel(ctx, snapshot)
}

func (r *Repository) upsertPublishedModel(ctx context.Context, snapshot *domain.PublishedModelSnapshot) error {
	po := r.mapper.ToPO(snapshot)
	now := time.Now()
	po.Status = statusPublished
	po.PublishedAt = &now

	filter := bson.M{
		"model_kind":      po.ModelKind,
		"model_sub_kind":  po.ModelSubKind,
		"model_algorithm": po.ModelAlgorithm,
		"model_code":      po.ModelCode,
		"model_version":   po.ModelVersion,
		"deleted_at":      nil,
	}

	var existing PublishedAssessmentModelPO
	findErr := r.FindOne(ctx, filter, &existing)
	if findErr == mongo.ErrNoDocuments {
		mongoBase.ApplyAuditCreate(ctx, po)
		po.BeforeInsert()
		insertData, err := po.ToBsonM()
		if err != nil {
			return err
		}
		_, err = r.InsertOne(ctx, insertData)
		return err
	}
	if findErr != nil {
		return findErr
	}

	mongoBase.ApplyAuditUpdate(ctx, po)
	po.BeforeUpdate()
	updateData, err := po.ToBsonM()
	if err != nil {
		return err
	}
	delete(updateData, "_id")
	delete(updateData, "created_at")
	delete(updateData, "created_by")
	_, err = r.Collection().UpdateOne(ctx, filter, bson.M{"$set": updateData})
	return err
}

func (r *Repository) GetPublishedByRef(ctx context.Context, ref port.Ref) (*domain.Snapshot, error) {
	if ref.Version == "" {
		return nil, domain.ErrVersionRequired
	}
	filter := publishedFilter(r.refFilter(ref))
	return r.findOne(ctx, filter)
}

func (r *Repository) FindPublishedByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*domain.Snapshot, error) {
	filter := publishedFilter(bson.M{
		"questionnaire_code": questionnaireCode,
	})
	if questionnaireVersion != "" {
		filter["questionnaire_version"] = questionnaireVersion
	}
	return r.findOne(ctx, filter)
}

func (r *Repository) refFilter(ref port.Ref) bson.M {
	kind, subKind, algorithm, mapped := domain.LegacyKindMapping(ref.Kind)
	if !mapped {
		kind = ref.Kind
	}
	filter := bson.M{
		"model_kind":    string(kind),
		"model_code":    ref.Code,
		"model_version": ref.Version,
	}
	if subKind != "" {
		filter["model_sub_kind"] = string(subKind)
	}
	if algorithm != "" {
		filter["model_algorithm"] = string(algorithm)
	}
	return filter
}

func (r *Repository) findOne(ctx context.Context, filter bson.M) (*domain.Snapshot, error) {
	count, err := r.Collection().CountDocuments(ctx, filter)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, domain.ErrNotFound
	}
	if count > 1 {
		return nil, domain.ErrAmbiguousVersion
	}
	var po PublishedAssessmentModelPO
	err = r.FindOne(ctx, filter, &po)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.mapper.ToLegacySnapshot(&po), nil
}
