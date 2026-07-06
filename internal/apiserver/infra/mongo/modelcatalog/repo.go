package modelcatalog

import (
	"context"
	"sort"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

type Repository struct {
	mongoBase.BaseRepository
	mapper *Mapper
}

var (
	_ port.PublishedReader          = (*Repository)(nil)
	_ port.PublishedLister          = (*Repository)(nil)
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

func (r *Repository) FindPublishedByModelCode(ctx context.Context, kind domain.Kind, code string) (*domain.Snapshot, error) {
	if code == "" {
		return nil, domain.ErrNotFound
	}
	filter := publishedFilter(bson.M{
		"model_kind": string(kind),
		"model_code": code,
	})
	return r.findOne(ctx, filter)
}

func (r *Repository) FindLatestPublishedByModelCode(ctx context.Context, kind domain.Kind, code string) (*domain.Snapshot, error) {
	if code == "" {
		return nil, domain.ErrNotFound
	}
	filter := publishedFilter(bson.M{
		"model_kind": string(kind),
		"model_code": code,
	})
	return r.findLatest(ctx, filter)
}

func (r *Repository) FindPublishedByModelCodeVersion(ctx context.Context, kind domain.Kind, code, version string) (*domain.Snapshot, error) {
	if code == "" || version == "" {
		return nil, domain.ErrNotFound
	}
	filter := publishedFilter(bson.M{
		"model_kind":    string(kind),
		"model_code":    code,
		"model_version": version,
	})
	return r.findOne(ctx, filter)
}

func (r *Repository) ListPublished(ctx context.Context, filter port.ListPublishedFilter) ([]*domain.Snapshot, int64, error) {
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
		extra["model_kind"] = string(filter.Kind)
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

	snapshots := make([]*domain.Snapshot, 0)
	for cursor.Next(ctx) {
		var po PublishedAssessmentModelPO
		if err := cursor.Decode(&po); err != nil {
			return nil, 0, err
		}
		snapshots = append(snapshots, r.mapper.ToLegacySnapshot(&po))
	}
	if err := cursor.Err(); err != nil {
		return nil, 0, err
	}
	return snapshots, total, nil
}

func (r *Repository) ListPublishedAlgorithms(ctx context.Context) ([]domain.Algorithm, error) {
	if r == nil {
		return nil, domain.ErrNotFound
	}
	mongoFilter := publishedFilter(bson.M{
		"model_kind":     string(domain.KindPersonality),
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
	kind := ref.Kind
	subKind := ref.SubKind
	algorithm := ref.Algorithm
	if subKind == "" && algorithm == "" {
		if mappedKind, mappedSubKind, mappedAlgorithm, mapped := domain.LegacyKindMapping(ref.Kind); mapped {
			kind = mappedKind
			subKind = mappedSubKind
			algorithm = mappedAlgorithm
		}
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
	var po PublishedAssessmentModelPO
	err := r.FindOne(ctx, filter, &po)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.mapper.ToLegacySnapshot(&po), nil
}

func (r *Repository) findLatest(ctx context.Context, filter bson.M) (*domain.Snapshot, error) {
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
	return r.mapper.ToLegacySnapshot(&po), nil
}
