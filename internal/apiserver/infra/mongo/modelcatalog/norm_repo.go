package modelcatalog

import (
	"context"
	"fmt"
	"reflect"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

type NormRepository struct{ mongoBase.BaseRepository }

var _ port.NormRepository = (*NormRepository)(nil)

func NewNormRepository(db *mongo.Database, opts ...mongoBase.BaseRepositoryOptions) *NormRepository {
	po := &NormPO{}
	return &NormRepository{BaseRepository: mongoBase.NewBaseRepository(db, po.CollectionName(), opts...)}
}

func (r *NormRepository) UpsertNorm(ctx context.Context, table *norm.Norm) error {
	if table == nil || table.TableVersion == "" {
		return fmt.Errorf("%w: norm table version is required", domain.ErrInvalidArgument)
	}
	filter := bson.M{"table_version": table.TableVersion, "deleted_at": nil}
	var existing NormPO
	err := r.FindOne(ctx, filter, &existing)
	if err == mongo.ErrNoDocuments {
		po := normToPO(table)
		mongoBase.ApplyAuditCreate(ctx, po)
		po.BeforeInsert()
		data, marshalErr := po.ToBsonM()
		if marshalErr != nil {
			return marshalErr
		}
		_, err = r.InsertOne(ctx, data)
		if mongo.IsDuplicateKeyError(err) {
			return r.compareExisting(ctx, table)
		}
		return err
	}
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(normFromPO(&existing), table) {
		return fmt.Errorf("%w: %s", domain.ErrNormVersionConflict, table.TableVersion)
	}
	return nil
}

func (r *NormRepository) compareExisting(ctx context.Context, table *norm.Norm) error {
	var existing NormPO
	if err := r.FindOne(ctx, bson.M{"table_version": table.TableVersion, "deleted_at": nil}, &existing); err != nil {
		return err
	}
	if !reflect.DeepEqual(normFromPO(&existing), table) {
		return fmt.Errorf("%w: %s", domain.ErrNormVersionConflict, table.TableVersion)
	}
	return nil
}

func (r *NormRepository) ListNorms(ctx context.Context, filter port.NormListFilter) ([]*norm.Norm, int64, error) {
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
	mongoFilter := bson.M{"deleted_at": nil}
	if filter.Kind != "" {
		mongoFilter["kind"] = string(filter.Kind)
	}
	if filter.Algorithm != "" {
		mongoFilter["algorithm"] = string(filter.Algorithm)
	}
	if filter.FormVariant != "" {
		mongoFilter["form_variant"] = filter.FormVariant
	}
	total, err := r.CountDocuments(ctx, mongoFilter)
	if err != nil {
		return nil, 0, err
	}
	cursor, err := r.Find(ctx, mongoFilter, options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}, {Key: "table_version", Value: 1}}).
		SetSkip(int64((page-1)*pageSize)).
		SetLimit(int64(pageSize)))
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	var rows []NormPO
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, 0, err
	}
	result := make([]*norm.Norm, 0, len(rows))
	for index := range rows {
		result = append(result, normFromPO(&rows[index]))
	}
	return result, total, nil
}

func (r *NormRepository) FindNorm(ctx context.Context, tableVersion string) (*norm.Norm, error) {
	if tableVersion == "" {
		return nil, domain.ErrNotFound
	}
	var po NormPO
	if err := r.FindOne(ctx, bson.M{"table_version": tableVersion, "deleted_at": nil}, &po); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return normFromPO(&po), nil
}
