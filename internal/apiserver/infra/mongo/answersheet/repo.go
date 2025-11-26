package answersheet

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// Repository 答卷MongoDB存储库
type Repository struct {
	mongoBase.BaseRepository
	mapper *AnswerSheetMapper
}

// NewRepository 创建答卷MongoDB存储库
func NewRepository(db *mongo.Database) answersheet.Repository {
	po := &AnswerSheetPO{}
	return &Repository{
		BaseRepository: mongoBase.NewBaseRepository(db, po.CollectionName()),
		mapper:         NewAnswerSheetMapper(),
	}
}

// Create 创建答卷
func (r *Repository) Create(ctx context.Context, sheet *answersheet.AnswerSheet) error {
	po := r.mapper.ToPO(sheet)
	if po == nil {
		return nil
	}

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

	po.BeforeUpdate()

	updateData, err := po.ToBsonM()
	if err != nil {
		return err
	}

	// 移除 _id 字段，避免更新主键
	delete(updateData, "_id")

	// 使用 $set 操作符包装更新数据
	update := bson.M{"$set": updateData}

	filter := bson.M{
		"domain_id": uint64(sheet.ID()),
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
	filter := bson.M{
		"domain_id":  uint64(id),
		"deleted_at": bson.M{"$exists": false},
	}

	var po AnswerSheetPO
	err := r.Collection().FindOne(ctx, filter).Decode(&po)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}

	return r.mapper.ToBO(&po), nil
}

// FindListByFiller 查询填写者的答卷列表
func (r *Repository) FindListByFiller(ctx context.Context, fillerID uint64, page, pageSize int) ([]*answersheet.AnswerSheet, error) {
	filter := bson.M{
		"filler_id":  int64(fillerID),
		"deleted_at": bson.M{"$exists": false},
	}

	// 设置分页选项
	skip := int64((page - 1) * pageSize)
	limit := int64(pageSize)
	opts := options.Find().
		SetSkip(skip).
		SetLimit(limit).
		SetSort(bson.M{"filled_at": -1}) // 按填写时间倒序

	cursor, err := r.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var sheets []*answersheet.AnswerSheet
	for cursor.Next(ctx) {
		var po AnswerSheetPO
		if err := cursor.Decode(&po); err != nil {
			return nil, err
		}
		sheets = append(sheets, r.mapper.ToBO(&po))
	}

	if err := cursor.Err(); err != nil {
		return nil, err
	}

	return sheets, nil
}

// FindListByQuestionnaire 查询问卷的答卷列表
func (r *Repository) FindListByQuestionnaire(ctx context.Context, questionnaireCode string, page, pageSize int) ([]*answersheet.AnswerSheet, error) {
	filter := bson.M{
		"questionnaire_code": questionnaireCode,
		"deleted_at":         bson.M{"$exists": false},
	}

	// 设置分页选项
	skip := int64((page - 1) * pageSize)
	limit := int64(pageSize)
	opts := options.Find().
		SetSkip(skip).
		SetLimit(limit).
		SetSort(bson.M{"filled_at": -1}) // 按填写时间倒序

	cursor, err := r.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var sheets []*answersheet.AnswerSheet
	for cursor.Next(ctx) {
		var po AnswerSheetPO
		if err := cursor.Decode(&po); err != nil {
			return nil, err
		}
		sheets = append(sheets, r.mapper.ToBO(&po))
	}

	if err := cursor.Err(); err != nil {
		return nil, err
	}

	return sheets, nil
}

// CountWithConditions 根据条件统计数量
func (r *Repository) CountWithConditions(ctx context.Context, conditions map[string]interface{}) (int64, error) {
	filter := bson.M(conditions)

	// 添加软删除过滤条件
	filter["deleted_at"] = bson.M{"$exists": false}

	return r.CountDocuments(ctx, filter)
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

	filter := bson.M{
		"domain_id": uint64(id),
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
