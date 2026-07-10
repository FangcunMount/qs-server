package modelcatalog

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

type DraftRepository struct {
	mongoBase.BaseRepository
	mapper *DraftMapper
}

var _ port.ModelRepository = (*DraftRepository)(nil)

func NewDraftRepository(db *mongo.Database, opts ...mongoBase.BaseRepositoryOptions) *DraftRepository {
	po := &AssessmentModelPO{}
	return &DraftRepository{
		BaseRepository: mongoBase.NewBaseRepository(db, po.CollectionName(), opts...),
		mapper:         NewDraftMapper(),
	}
}

func (r *DraftRepository) Create(ctx context.Context, model *domain.AssessmentModel) error {
	if model == nil {
		return fmt.Errorf("assessment model is nil")
	}
	po := r.mapper.ToPO(model)
	mongoBase.ApplyAuditCreate(ctx, po)
	po.BeforeInsert()
	data, err := po.ToBsonM()
	if err != nil {
		return err
	}
	_, err = r.InsertOne(ctx, data)
	if mongo.IsDuplicateKeyError(err) {
		return fmt.Errorf("%w: code %s already exists", domain.ErrInvalidArgument, model.Code)
	}
	return err
}

func (r *DraftRepository) Update(ctx context.Context, model *domain.AssessmentModel) error {
	if model == nil || model.Code == "" {
		return fmt.Errorf("assessment model is invalid")
	}
	po := r.mapper.ToPO(model)
	mongoBase.ApplyAuditUpdate(ctx, po)
	po.BeforeUpdate()
	updateData, err := po.ToBsonM()
	if err != nil {
		return err
	}
	delete(updateData, "_id")
	delete(updateData, "created_at")
	delete(updateData, "created_by")

	filter := draftFilter(bson.M{
		"code":    model.Code,
		"version": model.Version - 1,
	})
	result, err := r.UpdateOne(ctx, filter, bson.M{"$set": updateData})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *DraftRepository) FindByCode(ctx context.Context, code string) (*domain.AssessmentModel, error) {
	if code == "" {
		return nil, domain.ErrNotFound
	}
	var po AssessmentModelPO
	if err := r.FindOne(ctx, draftFilter(bson.M{"code": code}), &po); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.mapper.ToDomain(&po), nil
}

func (r *DraftRepository) FindByQuestionnaireCode(ctx context.Context, kind domain.Kind, questionnaireCode string) (*domain.AssessmentModel, error) {
	if questionnaireCode == "" {
		return nil, domain.ErrNotFound
	}
	filter := draftFilter(bson.M{
		"questionnaire_code": questionnaireCode,
	})
	if kind != "" {
		filter["kind"] = string(kind)
	}
	var po AssessmentModelPO
	if err := r.FindOne(ctx, filter, &po); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.mapper.ToDomain(&po), nil
}

func (r *DraftRepository) List(ctx context.Context, filter port.ListFilter) ([]*domain.AssessmentModel, int64, error) {
	page := filter.Page
	if page <= 0 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	extra := bson.M{}
	if filter.Kind != "" {
		extra["kind"] = string(filter.Kind)
	}
	if filter.SubKind != "" {
		extra["sub_kind"] = string(filter.SubKind)
	}
	if filter.Status != "" {
		extra["status"] = string(filter.Status)
	}
	if filter.Category != "" {
		extra["category"] = filter.Category
	}
	if filter.Algorithm != "" {
		extra["algorithm"] = string(filter.Algorithm)
	}
	if filter.QuestionnaireCode != "" {
		extra["questionnaire_code"] = filter.QuestionnaireCode
	}
	if filter.QuestionnaireVersion != "" {
		extra["questionnaire_version"] = filter.QuestionnaireVersion
	}
	mongoFilter := draftFilter(extra)
	if filter.Keyword != "" {
		mongoFilter["title"] = bson.M{"$regex": strings.TrimSpace(filter.Keyword), "$options": "i"}
	}

	total, err := r.Collection().CountDocuments(ctx, mongoFilter)
	if err != nil {
		return nil, 0, err
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "updated_at", Value: -1}}).
		SetSkip(int64((page - 1) * pageSize)).
		SetLimit(int64(pageSize))

	cursor, err := r.Collection().Find(ctx, mongoFilter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	models := make([]*domain.AssessmentModel, 0)
	for cursor.Next(ctx) {
		var po AssessmentModelPO
		if err := cursor.Decode(&po); err != nil {
			return nil, 0, err
		}
		models = append(models, r.mapper.ToDomain(&po))
	}
	return models, total, cursor.Err()
}

func (r *DraftRepository) Delete(ctx context.Context, code string) error {
	if code == "" {
		return domain.ErrNotFound
	}
	now := time.Now()
	result, err := r.UpdateOne(ctx, draftFilter(bson.M{"code": code}), bson.M{"$set": bson.M{
		"deleted_at": now,
		"updated_at": now,
	}})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return domain.ErrNotFound
	}
	return nil
}
