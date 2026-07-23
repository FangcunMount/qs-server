package modelcatalog

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
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
	_ port.PublishedModelReader          = (*Repository)(nil)
	_ port.ActivePublishedModelReader    = (*Repository)(nil)
	_ port.PublishedModelLister          = (*Repository)(nil)
	_ port.PublishedReleaseHistoryReader = (*Repository)(nil)
	_ port.PublishedWriter               = (*Repository)(nil)
	_ port.PublishedAlgorithmLister      = (*Repository)(nil)
	_ port.PublishedSnapshotRepository   = (*Repository)(nil)
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

func (r *Repository) Save(ctx context.Context, model *port.PublishedModel) error {
	return r.UpsertPublishedModel(ctx, model)
}

func (r *Repository) FindPublishedByModelCode(ctx context.Context, kind domain.Kind, code string) (*port.PublishedModel, error) {
	return r.FindActivePublishedModelByModelCode(ctx, kind, code)
}

func (r *Repository) FindLatestPublishedByModelCode(ctx context.Context, kind domain.Kind, code string) (*port.PublishedModel, error) {
	return r.FindLatestPublishedModelByModelCode(ctx, kind, code)
}

func (r *Repository) FindPublishedByModelCodeVersion(ctx context.Context, kind domain.Kind, code, version string) (*port.PublishedModel, error) {
	return r.GetPublishedModelByRef(ctx, port.Ref{Kind: kind, Code: code, Version: version})
}

func (r *Repository) ListPublished(ctx context.Context, filter port.ListPublishedFilter) ([]*port.PublishedModel, int64, error) {
	return r.ListPublishedModels(ctx, filter)
}

// DeletePublished only deactivates the externally visible snapshot. Retained
// releases remain available to exact-version readers and draft recovery.
func (r *Repository) DeletePublished(ctx context.Context, kind domain.Kind, code string) error {
	if code == "" {
		return domain.ErrNotFound
	}
	_, err := r.Collection().UpdateMany(ctx, activePublishedFilter(bson.M{
		"kind": kindBSONFilter(kind),
		"code": code,
	}), bson.M{"$set": bson.M{
		"release_status":      string(domain.ReleaseStatusArchived),
		"release_archived_at": time.Now(),
		"updated_at":          time.Now(),
	}})
	return err
}

func (r *Repository) upsertPublishedModel(ctx context.Context, model *port.PublishedModel) error {
	po := r.mapper.ToPO(model)
	now := time.Now()
	po.Status = statusPublished
	po.ReleaseStatus = string(domain.ReleaseStatusActive)
	po.PublishedAt = &now
	po.ReleaseArchivedAt = nil

	filter := publishedModelUpsertFilter(po)
	var existing PublishedAssessmentModelPO
	findErr := r.FindOne(ctx, filter, &existing)
	if findErr == nil {
		incoming := r.mapper.ToPublished(po)
		persisted := r.mapper.ToPublished(&existing)
		if !sameImmutablePublishedContent(persisted, incoming) {
			return fmt.Errorf("%w: %s@%s", domain.ErrReleaseVersionConflict, model.Code, model.Version)
		}
		if domain.NormalizeReleaseStatus(domain.ReleaseStatus(existing.ReleaseStatus)) != domain.ReleaseStatusActive {
			return fmt.Errorf("%w: archived release %s@%s cannot be reactivated", domain.ErrInvalidState, model.Code, model.Version)
		}
		return nil
	}
	if findErr != mongo.ErrNoDocuments {
		return findErr
	}

	// Earlier active releases remain immutable and are archived in the same
	// outer Mongo transaction before the new active row is inserted.
	_, err := r.Collection().UpdateMany(ctx, activePublishedFilter(bson.M{
		"kind": po.Kind,
		"code": po.Code,
	}), bson.M{"$set": bson.M{
		"release_status":      string(domain.ReleaseStatusArchived),
		"release_archived_at": now,
		"updated_at":          now,
	}})
	if err != nil {
		return err
	}
	mongoBase.ApplyAuditCreate(ctx, po)
	po.BeforeInsert()
	insertData, err := po.ToBsonM()
	if err != nil {
		return err
	}
	_, err = r.InsertOne(ctx, insertData)
	return err
}

func sameImmutablePublishedContent(a, b *port.PublishedModel) bool {
	if a == nil || b == nil {
		return a == b
	}
	aCopy, bCopy := *a, *b
	aCopy.ReleaseStatus, bCopy.ReleaseStatus = "", ""
	aCopy.PublishedAt, bCopy.PublishedAt = nil, nil
	aCopy.ReleaseArchivedAt, bCopy.ReleaseArchivedAt = nil, nil
	return reflect.DeepEqual(aCopy, bCopy)
}

func (r *Repository) GetPublishedModelByRef(ctx context.Context, ref port.Ref) (*port.PublishedModel, error) {
	if ref.Version == "" {
		return nil, domain.ErrVersionRequired
	}
	filter := publishedFilter(r.refFilter(ref))
	return r.findOnePublished(ctx, filter)
}

// GetActivePublishedModelByRef resolves an exact version only when it is the
// currently active release. Assessment intake must use this method; workers
// intentionally continue to use GetPublishedModelByRef for retained history.
func (r *Repository) GetActivePublishedModelByRef(ctx context.Context, ref port.Ref) (*port.PublishedModel, error) {
	if ref.Version == "" {
		return nil, domain.ErrVersionRequired
	}
	return r.findOnePublished(ctx, activePublishedFilter(r.refFilter(ref)))
}

func (r *Repository) FindPublishedModelByQuestionnaire(ctx context.Context, questionnaireCode, questionnaireVersion string) (*port.PublishedModel, error) {
	filter := activePublishedFilter(bson.M{
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
		"kind": kindBSONFilter(kind),
		"code": code,
	})
	return r.findLatestPublished(ctx, filter)
}

// FindActivePublishedModelByModelCode resolves the one snapshot visible to
// runtime callers. Historical snapshots deliberately stay readable through
// FindLatestPublishedModelByModelCode and GetPublishedModelByRef.
func (r *Repository) FindActivePublishedModelByModelCode(ctx context.Context, kind domain.Kind, code string) (*port.PublishedModel, error) {
	if code == "" {
		return nil, domain.ErrNotFound
	}
	return r.findLatestPublished(ctx, activePublishedFilter(bson.M{
		"kind": kindBSONFilter(kind),
		"code": code,
	}))
}

func (r *Repository) FindPublishedModelByCode(ctx context.Context, kind domain.Kind, code string) (*port.PublishedModel, error) {
	return r.FindActivePublishedModelByModelCode(ctx, kind, code)
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
	if codeValue := strings.TrimSpace(filter.Code); codeValue != "" {
		extra["code"] = codeValue
	}
	if len(filter.Kinds) == 1 {
		extra["kind"] = kindBSONFilter(filter.Kinds[0])
	} else if len(filter.Kinds) > 1 {
		values := make([]any, 0, len(filter.Kinds))
		for _, kind := range filter.Kinds {
			values = append(values, kindBSONFilter(kind))
		}
		extra["kind"] = bson.M{"$in": values}
	} else if filter.Kind != "" {
		extra["kind"] = kindBSONFilter(filter.Kind)
	}
	if filter.Algorithm != "" {
		extra["algorithm"] = string(filter.Algorithm)
	}
	if filter.Category != "" {
		extra["category"] = filter.Category
	}
	if filter.QuestionnaireCode != "" {
		extra["questionnaire_code"] = filter.QuestionnaireCode
	}
	if filter.QuestionnaireVersion != "" {
		extra["questionnaire_version"] = filter.QuestionnaireVersion
	}
	mongoFilter := activePublishedFilter(extra)
	if filter.Keyword != "" {
		mongoFilter["title"] = bson.M{"$regex": strings.TrimSpace(filter.Keyword), "$options": "i"}
	}

	total, err := r.CountDocuments(ctx, mongoFilter)
	if err != nil {
		return nil, 0, err
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "code", Value: 1}}).
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

func (r *Repository) ListPublishedReleaseHistory(ctx context.Context, codeValue string) ([]*port.PublishedModel, error) {
	if strings.TrimSpace(codeValue) == "" {
		return nil, domain.ErrNotFound
	}
	cursor, err := r.Find(ctx, publishedFilter(bson.M{"code": strings.TrimSpace(codeValue)}), options.Find().SetSort(bson.D{
		{Key: "published_at", Value: -1},
		{Key: "release_version", Value: -1},
	}))
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	items := make([]*port.PublishedModel, 0)
	for cursor.Next(ctx) {
		var po PublishedAssessmentModelPO
		if err := cursor.Decode(&po); err != nil {
			return nil, err
		}
		items = append(items, r.mapper.ToPublished(&po))
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) ListPublishedAlgorithms(ctx context.Context) ([]domain.Algorithm, error) {
	if r == nil {
		return nil, domain.ErrNotFound
	}
	mongoFilter := activePublishedFilter(bson.M{
		"kind": kindBSONFilter(domain.KindTypology),
	})
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: mongoFilter}},
		bson.D{{Key: "$group", Value: bson.M{"_id": "$algorithm"}}},
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
	sort.Slice(algorithms, func(i, j int) bool {
		return algorithms[i] < algorithms[j]
	})
}

func (r *Repository) refFilter(ref port.Ref) bson.M {
	filter := bson.M{
		"kind":            kindBSONFilter(ref.Kind),
		"code":            ref.Code,
		"release_version": ref.Version,
	}
	if ref.SubKind != "" {
		filter["sub_kind"] = string(ref.SubKind)
	}
	if ref.Algorithm != "" {
		filter["algorithm"] = string(ref.Algorithm)
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
		{Key: "release_version", Value: -1},
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
