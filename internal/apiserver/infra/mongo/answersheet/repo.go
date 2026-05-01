package answersheet

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	mongoEventOutbox "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/eventoutbox"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/safeconv"
)

// Repository 答卷MongoDB存储库
type Repository struct {
	mongoBase.BaseRepository
	mapper          *AnswerSheetMapper
	idempotencyColl *mongo.Collection
	outboxStore     *mongoEventOutbox.Store
}

// NewRepository 创建答卷MongoDB存储库
func NewRepository(db *mongo.Database, opts ...mongoBase.BaseRepositoryOptions) (*Repository, error) {
	return NewRepositoryWithTopicResolver(db, nil, opts...)
}

func NewRepositoryWithTopicResolver(db *mongo.Database, resolver eventcatalog.TopicResolver, opts ...mongoBase.BaseRepositoryOptions) (*Repository, error) {
	po := &AnswerSheetPO{}
	repo := &Repository{
		BaseRepository:  mongoBase.NewBaseRepository(db, po.CollectionName(), opts...),
		mapper:          NewAnswerSheetMapper(),
		idempotencyColl: db.Collection((&AnswerSheetSubmitIdempotencyPO{}).CollectionName()),
	}
	outboxStore, err := mongoEventOutbox.NewStoreWithTopicResolver(db, resolver)
	if err != nil {
		return nil, err
	}
	repo.outboxStore = outboxStore
	if err := repo.ensureIndexes(context.Background()); err != nil {
		return nil, err
	}
	return repo, nil
}

// Create 创建答卷
func (r *Repository) Create(ctx context.Context, sheet *answersheet.AnswerSheet) error {
	po := r.mapper.ToPO(sheet)
	if po == nil {
		return nil
	}

	mongoBase.ApplyAuditCreate(ctx, po)
	po.BeforeInsert()

	insertData, err := po.ToBsonM()
	if err != nil {
		return err
	}

	_, err = r.InsertOne(ctx, insertData)
	if err != nil {
		return err
	}

	// 将生成的 ID 设置回领域对象
	sheet.AssignID(meta.ID(po.DomainID))

	return nil
}

// Update 更新答卷
func (r *Repository) Update(ctx context.Context, sheet *answersheet.AnswerSheet) error {
	po := r.mapper.ToPO(sheet)
	if po == nil {
		return nil
	}

	mongoBase.ApplyAuditUpdate(ctx, po)
	po.BeforeUpdate()

	updateData, err := po.ToBsonM()
	if err != nil {
		return err
	}

	// 移除 _id 字段，避免更新主键
	delete(updateData, "_id")

	// 使用 $set 操作符包装更新数据
	update := bson.M{"$set": updateData}
	domainID, err := safeconv.MetaIDToUint64(sheet.ID())
	if err != nil {
		return err
	}

	filter := bson.M{
		"domain_id": domainID,
	}

	result, err := r.Collection().UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return mongo.ErrNoDocuments
	}

	return nil
}

// FindByID 根据 ID 查询答卷
func (r *Repository) FindByID(ctx context.Context, id meta.ID) (*answersheet.AnswerSheet, error) {
	domainID, err := safeconv.MetaIDToUint64(id)
	if err != nil {
		return nil, err
	}
	filter := bson.M{
		"domain_id":  domainID,
		"deleted_at": nil,
	}

	var po AnswerSheetPO
	err = r.Collection().FindOne(ctx, filter).Decode(&po)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}

	return r.mapper.ToBO(&po), nil
}

// Delete 删除答卷（软删除）
func (r *Repository) Delete(ctx context.Context, id meta.ID) error {
	now := time.Now()
	update := bson.M{
		"$set": bson.M{
			"deleted_at": now,
			"updated_at": now,
		},
	}
	domainID, err := safeconv.MetaIDToUint64(id)
	if err != nil {
		return err
	}

	filter := bson.M{
		"domain_id": domainID,
	}

	result, err := r.Collection().UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return mongo.ErrNoDocuments
	}

	return nil
}
