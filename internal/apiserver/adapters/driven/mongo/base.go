package mongo

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// BaseRepository MongoDB基础存储库
type BaseRepository struct {
	db         *mongo.Database
	collection *mongo.Collection
}

// NewBaseRepository 创建基础存储库
func NewBaseRepository(db *mongo.Database, collectionName string) BaseRepository {
	return BaseRepository{
		db:         db,
		collection: db.Collection(collectionName),
	}
}

// DB 获取数据库连接
func (r *BaseRepository) DB() *mongo.Database {
	return r.db
}

// Collection 获取集合
func (r *BaseRepository) Collection() *mongo.Collection {
	return r.collection
}

// InsertOne 插入一条文档
func (r *BaseRepository) InsertOne(ctx context.Context, document interface{}) (*mongo.InsertOneResult, error) {
	return r.collection.InsertOne(ctx, document)
}

// FindOne 查找一条文档
func (r *BaseRepository) FindOne(ctx context.Context, filter bson.M, result interface{}) error {
	return r.collection.FindOne(ctx, filter).Decode(result)
}

// FindByID 根据ObjectID查找文档
func (r *BaseRepository) FindByID(ctx context.Context, id primitive.ObjectID, result interface{}) error {
	filter := bson.M{"_id": id}
	return r.collection.FindOne(ctx, filter).Decode(result)
}

// UpdateOne 更新一条文档
func (r *BaseRepository) UpdateOne(ctx context.Context, filter bson.M, update bson.M) (*mongo.UpdateResult, error) {
	return r.collection.UpdateOne(ctx, filter, update)
}

// UpdateByID 根据ObjectID更新文档
func (r *BaseRepository) UpdateByID(ctx context.Context, id primitive.ObjectID, update bson.M) (*mongo.UpdateResult, error) {
	filter := bson.M{"_id": id}
	return r.collection.UpdateOne(ctx, filter, update)
}

// DeleteOne 删除一条文档
func (r *BaseRepository) DeleteOne(ctx context.Context, filter bson.M) (*mongo.DeleteResult, error) {
	return r.collection.DeleteOne(ctx, filter)
}

// DeleteByID 根据ObjectID删除文档
func (r *BaseRepository) DeleteByID(ctx context.Context, id primitive.ObjectID) (*mongo.DeleteResult, error) {
	filter := bson.M{"_id": id}
	return r.collection.DeleteOne(ctx, filter)
}

// Find 查找多条文档
func (r *BaseRepository) Find(ctx context.Context, filter bson.M, opts ...*options.FindOptions) (*mongo.Cursor, error) {
	return r.collection.Find(ctx, filter, opts...)
}

// CountDocuments 统计文档数量
func (r *BaseRepository) CountDocuments(ctx context.Context, filter bson.M) (int64, error) {
	return r.collection.CountDocuments(ctx, filter)
}

// ExistsByFilter 检查是否存在符合条件的文档
func (r *BaseRepository) ExistsByFilter(ctx context.Context, filter bson.M) (bool, error) {
	count, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// BaseDocument MongoDB基础文档结构
type BaseDocument struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time          `bson:"updated_at" json:"updated_at"`
	DeletedAt *time.Time         `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	CreatedBy uint64             `bson:"created_by" json:"created_by"`
	UpdatedBy uint64             `bson:"updated_by" json:"updated_by"`
	DeletedBy uint64             `bson:"deleted_by,omitempty" json:"deleted_by,omitempty"`
}

// SetCreatedAt 设置创建时间
func (d *BaseDocument) SetCreatedAt(t time.Time) {
	d.CreatedAt = t
}

// SetUpdatedAt 设置更新时间
func (d *BaseDocument) SetUpdatedAt(t time.Time) {
	d.UpdatedAt = t
}

// SetDeletedAt 设置删除时间
func (d *BaseDocument) SetDeletedAt(t *time.Time) {
	d.DeletedAt = t
}

// SetCreatedBy 设置创建者
func (d *BaseDocument) SetCreatedBy(userID uint64) {
	d.CreatedBy = userID
}

// SetUpdatedBy 设置更新者
func (d *BaseDocument) SetUpdatedBy(userID uint64) {
	d.UpdatedBy = userID
}

// SetDeletedBy 设置删除者
func (d *BaseDocument) SetDeletedBy(userID uint64) {
	d.DeletedBy = userID
}
