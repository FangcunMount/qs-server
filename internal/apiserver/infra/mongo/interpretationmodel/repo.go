package interpretationmodel

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretationmodel"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationmodel"
)

type Repository struct {
	mongoBase.BaseRepository
	mapper *Mapper
}

var (
	_ port.PublishedModelReader = (*Repository)(nil)
	_ port.PublishedModelWriter = (*Repository)(nil)
)

func NewRepository(db *mongo.Database, opts ...mongoBase.BaseRepositoryOptions) *Repository {
	po := &InterpretationModelPO{}
	return &Repository{
		BaseRepository: mongoBase.NewBaseRepository(db, po.CollectionName(), opts...),
		mapper:         NewMapper(),
	}
}

func (r *Repository) UpsertPublished(ctx context.Context, snapshot *domain.RuleSetSnapshot) error {
	if snapshot == nil {
		return mongo.ErrNilDocument
	}
	po := r.mapper.ToPO(snapshot)
	now := time.Now()
	po.Status = statusPublished
	po.PublishedAt = &now

	filter := bson.M{
		"model_kind":    po.ModelKind,
		"model_code":    po.ModelCode,
		"model_version": po.ModelVersion,
		"deleted_at":    nil,
	}

	var existing InterpretationModelPO
	findErr := r.FindOne(ctx, filter, &existing)
	if findErr == mongo.ErrNoDocuments {
		mongoBase.ApplyAuditCreate(ctx, po)
		po.BeforeInsert()
		po.Status = statusPublished
		po.PublishedAt = &now
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
	po.Status = statusPublished
	po.PublishedAt = &now
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

func (r *Repository) GetPublishedByRef(ctx context.Context, ref port.ModelRef) (*domain.RuleSetSnapshot, error) {
	filter := publishedFilter(bson.M{
		"model_kind": ref.Kind.String(),
		"model_code": ref.Code,
	})
	if ref.Version != "" {
		filter["model_version"] = ref.Version
	}
	return r.findOne(ctx, filter)
}

func (r *Repository) FindPublishedByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*domain.RuleSetSnapshot, error) {
	filter := publishedFilter(bson.M{
		"questionnaire_code": questionnaireCode,
	})
	if questionnaireVersion != "" {
		filter["questionnaire_version"] = questionnaireVersion
	}
	return r.findOne(ctx, filter)
}

func (r *Repository) findOne(ctx context.Context, filter bson.M) (*domain.RuleSetSnapshot, error) {
	var po InterpretationModelPO
	err := r.FindOne(ctx, filter, &po)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.mapper.ToDomain(&po), nil
}
