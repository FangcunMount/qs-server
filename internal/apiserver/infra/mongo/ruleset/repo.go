package ruleset

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
	_ port.PublishedRuleSetReader = (*Repository)(nil)
	_ port.PublishedRuleSetWriter = (*Repository)(nil)
)

func NewRepository(db *mongo.Database, opts ...mongoBase.BaseRepositoryOptions) *Repository {
	po := &EvaluationRuleSetPO{}
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
		"ruleset_kind":    po.RuleSetKind,
		"ruleset_code":    po.RuleSetCode,
		"ruleset_version": po.RuleSetVersion,
		"deleted_at":      nil,
	}

	var existing EvaluationRuleSetPO
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

func (r *Repository) GetPublishedByRef(ctx context.Context, ref port.RuleSetRef) (*domain.RuleSetSnapshot, error) {
	if ref.Version == "" {
		return nil, domain.ErrVersionRequired
	}
	filter := publishedFilter(bson.M{
		"ruleset_kind":    ref.Kind.String(),
		"ruleset_code":    ref.Code,
		"ruleset_version": ref.Version,
	})
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
	var po EvaluationRuleSetPO
	err = r.FindOne(ctx, filter, &po)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.mapper.ToDomain(&po), nil
}

func (r *Repository) ListPublished(ctx context.Context) ([]*domain.Snapshot, error) {
	cursor, err := r.Collection().Find(ctx, publishedFilter(bson.M{}))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	snapshots := make([]*domain.Snapshot, 0)
	for cursor.Next(ctx) {
		var po EvaluationRuleSetPO
		if err := cursor.Decode(&po); err != nil {
			return nil, err
		}
		snapshots = append(snapshots, r.mapper.ToDomain(&po))
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return snapshots, nil
}
