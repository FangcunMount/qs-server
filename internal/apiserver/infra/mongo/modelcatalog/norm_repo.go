package modelcatalog

import (
	"context"
	"fmt"
	"reflect"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

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
		return err
	}
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(normFromPO(&existing), table) {
		return fmt.Errorf("%w: norm table version %s conflicts with existing content", domain.ErrInvalidArgument, table.TableVersion)
	}
	return nil
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
