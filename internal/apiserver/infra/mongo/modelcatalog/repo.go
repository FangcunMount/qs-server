package modelcatalog

import (
	"context"
	"fmt"
	"sort"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

type Repository struct {
	mongoBase.BaseRepository
	mapper *Mapper
}

var (
	_ port.PublishedWriter          = (*Repository)(nil)
	_ port.PublishedAlgorithmLister = (*Repository)(nil)
)

func NewRepository(db *mongo.Database, opts ...mongoBase.BaseRepositoryOptions) *Repository {
	po := &PublishedAssessmentModelPO{}
	return &Repository{
		BaseRepository: mongoBase.NewBaseRepository(db, po.CollectionName(), opts...),
		mapper:         NewMapper(),
	}
}

func (r *Repository) UpsertPublishedModel(ctx context.Context, model *port.PublishedModel) error {
	if model == nil {
		return mongo.ErrNilDocument
	}
	return r.upsertPublishedModel(ctx, model)
}

// BackfillPublishedDefinitionV2 updates only DefinitionV2 on the exact
// historical published row. It intentionally does not normalize identity or
// rewrite payload fields, which makes it safe for one-off migrations.
func (r *Repository) BackfillPublishedDefinitionV2(ctx context.Context, model *port.PublishedModel, definitionV2 *modeldefinition.Definition) error {
	if model == nil || definitionV2 == nil || model.Code == "" || model.Version == "" {
		return fmt.Errorf("%w: published model and definition_v2 are required", domain.ErrInvalidArgument)
	}
	filter := publishedFilter(bson.M{
		"model_code":    model.Code,
		"model_version": model.Version,
		"payload":       model.Payload,
	})
	result, err := r.Collection().UpdateOne(ctx, filter, bson.M{"$set": bson.M{
		"definition_schema_version": definitionSchemaVersion(definitionV2),
		"definition_v2":             definitionToPO(definitionV2),
		"updated_at":                time.Now(),
	}})
	if err != nil {
		return err
	}
	if result.MatchedCount != 1 {
		return fmt.Errorf("%w: published model %s@%s", domain.ErrNotFound, model.Code, model.Version)
	}
	return nil
}

func (r *Repository) upsertPublishedModel(ctx context.Context, model *port.PublishedModel) error {
	po := r.mapper.ToPO(model)
	now := time.Now()
	po.Status = statusPublished
	po.PublishedAt = &now

	filter := publishedModelUpsertFilter(po)

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

func (r *Repository) GetPublishedModelByRef(ctx context.Context, ref port.Ref) (*port.PublishedModel, error) {
	if ref.Version == "" {
		return nil, domain.ErrVersionRequired
	}
	filter := publishedFilter(r.refFilter(ref))
	return r.findOnePublished(ctx, filter)
}

func (r *Repository) FindPublishedModelByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*port.PublishedModel, error) {
	filter := publishedFilter(bson.M{
		"questionnaire_code": questionnaireCode,
	})
	if questionnaireVersion != "" {
		filter["questionnaire_version"] = questionnaireVersion
	}
	return r.findOnePublished(ctx, filter)
}

func (r *Repository) FindLatestPublishedModelByModelCode(ctx context.Context, kind domain.Kind, code string) (*port.PublishedModel, error) {
	if code == "" {
		return nil, domain.ErrNotFound
	}
	filter := publishedFilter(bson.M{
		"model_kind": kindBSONFilter(kind),
		"model_code": code,
	})
	return r.findLatestPublished(ctx, filter)
}

func (r *Repository) ListPublishedModels(ctx context.Context, filter port.ListPublishedFilter) ([]*port.PublishedModel, int64, error) {
	page := filter.Page
	if page <= 0 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize <= 0 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}

	extra := bson.M{}
	if filter.Kind != "" {
		extra["model_kind"] = kindBSONFilter(filter.Kind)
	}
	if filter.Algorithm != "" {
		extra["model_algorithm"] = string(filter.Algorithm)
	}
	mongoFilter := publishedFilter(extra)

	total, err := r.CountDocuments(ctx, mongoFilter)
	if err != nil {
		return nil, 0, err
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "model_code", Value: 1}}).
		SetSkip(int64((page - 1) * pageSize)).
		SetLimit(int64(pageSize))

	cursor, err := r.Find(ctx, mongoFilter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	models := make([]*port.PublishedModel, 0)
	for cursor.Next(ctx) {
		var po PublishedAssessmentModelPO
		if err := cursor.Decode(&po); err != nil {
			return nil, 0, err
		}
		models = append(models, r.mapper.ToPublished(&po))
	}
	if err := cursor.Err(); err != nil {
		return nil, 0, err
	}
	return models, total, nil
}

func (r *Repository) ListPublishedAlgorithms(ctx context.Context) ([]domain.Algorithm, error) {
	if r == nil {
		return nil, domain.ErrNotFound
	}
	mongoFilter := publishedFilter(bson.M{
		"model_kind":     kindBSONFilter(domain.KindTypology),
		"model_sub_kind": string(domain.SubKindTypology),
	})
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: mongoFilter}},
		bson.D{{Key: "$group", Value: bson.M{"_id": "$model_algorithm"}}},
	}
	cursor, err := r.Collection().Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	seen := make(map[domain.Algorithm]struct{})
	for cursor.Next(ctx) {
		var grouped struct {
			ID string `bson:"_id"`
		}
		if err := cursor.Decode(&grouped); err != nil {
			return nil, err
		}
		if grouped.ID == "" {
			continue
		}
		seen[domain.Algorithm(grouped.ID)] = struct{}{}
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	out := make([]domain.Algorithm, 0, len(seen))
	for algorithm := range seen {
		out = append(out, algorithm)
	}
	sortAlgorithms(out)
	return out, nil
}

func sortAlgorithms(algorithms []domain.Algorithm) {
	order := map[domain.Algorithm]int{
		domain.AlgorithmMBTI:    0,
		domain.AlgorithmSBTI:    1,
		domain.AlgorithmBigFive: 2,
	}
	sort.Slice(algorithms, func(i, j int) bool {
		left, okLeft := order[algorithms[i]]
		right, okRight := order[algorithms[j]]
		switch {
		case okLeft && okRight:
			return left < right
		case okLeft:
			return true
		case okRight:
			return false
		default:
			return algorithms[i] < algorithms[j]
		}
	})
}

func (r *Repository) refFilter(ref port.Ref) bson.M {
	filter := bson.M{
		"model_kind":    kindBSONFilter(ref.Kind),
		"model_code":    ref.Code,
		"model_version": ref.Version,
	}
	if ref.SubKind != "" {
		filter["model_sub_kind"] = string(ref.SubKind)
	}
	if ref.Algorithm != "" {
		filter["model_algorithm"] = string(ref.Algorithm)
	}
	return filter
}

func (r *Repository) findOnePublished(ctx context.Context, filter bson.M) (*port.PublishedModel, error) {
	var po PublishedAssessmentModelPO
	err := r.FindOne(ctx, filter, &po)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.mapper.ToPublished(&po), nil
}

func (r *Repository) findLatestPublished(ctx context.Context, filter bson.M) (*port.PublishedModel, error) {
	var po PublishedAssessmentModelPO
	opts := options.FindOne().SetSort(bson.D{
		{Key: "published_at", Value: -1},
		{Key: "updated_at", Value: -1},
		{Key: "model_version", Value: -1},
	})
	err := r.Collection().FindOne(ctx, filter, opts).Decode(&po)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.mapper.ToPublished(&po), nil
}
