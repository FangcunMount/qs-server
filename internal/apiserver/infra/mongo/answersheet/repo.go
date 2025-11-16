package answersheet

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/answersheet"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/answersheet/port"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	v1 "github.com/FangcunMount/qs-server/pkg/meta/v1"
)

// Repository 答卷MongoDB存储库
type Repository struct {
	mongoBase.BaseRepository
	mapper *AnswerSheetMapper
}

// NewRepository 创建答卷MongoDB存储库
func NewRepository(db *mongo.Database) port.AnswerSheetRepositoryMongo {
	po := &AnswerSheetPO{}
	return &Repository{
		BaseRepository: mongoBase.NewBaseRepository(db, po.CollectionName()),
		mapper:         NewAnswerSheetMapper(),
	}
}

// Create 创建答卷
func (r *Repository) Create(ctx context.Context, aDomain *answersheet.AnswerSheet) error {
	po := r.mapper.ToPO(aDomain)
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
	aDomain.SetID(v1.NewID(po.DomainID))

	return nil
}

// FindByID 根据ID查找答卷
func (r *Repository) FindByID(ctx context.Context, id uint64) (*answersheet.AnswerSheet, error) {
	filter := bson.M{
		"domain_id": id,
	}

	var po AnswerSheetPO
	err := r.Collection().FindOne(ctx, filter).Decode(&po)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // 或者返回自定义的NotFound错误
		}
		return nil, err
	}

	return r.mapper.ToBO(&po), nil
}

// FindListByWriter 根据答卷者ID查找答卷列表
func (r *Repository) FindListByWriter(ctx context.Context, writerID uint64, page, pageSize int) ([]*answersheet.AnswerSheet, error) {
	filter := bson.M{
		"writer.id": writerID,
	}

	// 设置分页选项
	skip := int64((page - 1) * pageSize)
	limit := int64(pageSize)
	opts := options.Find().
		SetSkip(skip).
		SetLimit(limit).
		SetSort(bson.M{"created_at": -1}) // 按创建时间倒序

	cursor, err := r.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var answerSheets []*answersheet.AnswerSheet
	for cursor.Next(ctx) {
		var po AnswerSheetPO
		if err := cursor.Decode(&po); err != nil {
			return nil, err
		}
		answerSheets = append(answerSheets, r.mapper.ToBO(&po))
	}

	if err := cursor.Err(); err != nil {
		return nil, err
	}

	return answerSheets, nil
}

// FindListByTestee 根据被试者ID查找答卷列表
func (r *Repository) FindListByTestee(ctx context.Context, testeeID uint64, page, pageSize int) ([]*answersheet.AnswerSheet, error) {
	filter := bson.M{
		"testee.id": testeeID,
	}

	// 设置分页选项
	skip := int64((page - 1) * pageSize)
	limit := int64(pageSize)
	opts := options.Find().
		SetSkip(skip).
		SetLimit(limit).
		SetSort(bson.M{"created_at": -1}) // 按创建时间倒序

	cursor, err := r.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var answerSheets []*answersheet.AnswerSheet
	for cursor.Next(ctx) {
		var po AnswerSheetPO
		if err := cursor.Decode(&po); err != nil {
			return nil, err
		}
		answerSheets = append(answerSheets, r.mapper.ToBO(&po))
	}

	if err := cursor.Err(); err != nil {
		return nil, err
	}

	return answerSheets, nil
}

// CountWithConditions 根据条件统计答卷数量
func (r *Repository) CountWithConditions(ctx context.Context, conditions map[string]interface{}) (int64, error) {
	filter := bson.M(conditions)

	// 添加软删除过滤条件
	filter["deleted_at"] = bson.M{
		"$exists": false,
	}

	return r.CountDocuments(ctx, filter)
}

// FindByQuestionnaireCode 根据问卷代码查找答卷列表
func (r *Repository) FindByQuestionnaireCode(ctx context.Context, questionnaireCode string, page, pageSize int) ([]*answersheet.AnswerSheet, error) {
	filter := bson.M{
		"questionnaire_code": questionnaireCode,
	}

	// 设置分页选项
	skip := int64((page - 1) * pageSize)
	limit := int64(pageSize)
	opts := options.Find().
		SetSkip(skip).
		SetLimit(limit).
		SetSort(bson.M{"created_at": -1}) // 按创建时间倒序

	cursor, err := r.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var answerSheets []*answersheet.AnswerSheet
	for cursor.Next(ctx) {
		var po AnswerSheetPO
		if err := cursor.Decode(&po); err != nil {
			return nil, err
		}
		answerSheets = append(answerSheets, r.mapper.ToBO(&po))
	}

	if err := cursor.Err(); err != nil {
		return nil, err
	}

	return answerSheets, nil
}

// FindByQuestionnaireCodeAndVersion 根据问卷代码和版本查找答卷列表
func (r *Repository) FindByQuestionnaireCodeAndVersion(ctx context.Context, questionnaireCode, version string, page, pageSize int) ([]*answersheet.AnswerSheet, error) {
	filter := bson.M{
		"questionnaire_code":    questionnaireCode,
		"questionnaire_version": version,
	}

	// 设置分页选项
	skip := int64((page - 1) * pageSize)
	limit := int64(pageSize)
	opts := options.Find().
		SetSkip(skip).
		SetLimit(limit).
		SetSort(bson.M{"created_at": -1}) // 按创建时间倒序

	cursor, err := r.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var answerSheets []*answersheet.AnswerSheet
	for cursor.Next(ctx) {
		var po AnswerSheetPO
		if err := cursor.Decode(&po); err != nil {
			return nil, err
		}
		answerSheets = append(answerSheets, r.mapper.ToBO(&po))
	}

	if err := cursor.Err(); err != nil {
		return nil, err
	}

	return answerSheets, nil
}

// Update 更新答卷
func (r *Repository) Update(ctx context.Context, aDomain *answersheet.AnswerSheet) error {
	po := r.mapper.ToPO(aDomain)
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

	// 使用 $set 操作符包装更新数据，避免覆盖其他字段
	update := bson.M{"$set": updateData}

	filter := bson.M{
		"domain_id": aDomain.GetID().Value(),
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

// Remove 删除答卷（软删除）
func (r *Repository) Remove(ctx context.Context, id uint64) error {
	now := time.Now()
	update := bson.M{
		"$set": bson.M{
			"deleted_at": now,
			"deleted_by": 0, // 这里应该从上下文中获取当前用户ID
			"updated_at": now,
		},
	}

	filter := bson.M{
		"domain_id": id,
	}

	result, err := r.Collection().UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return mongo.ErrNoDocuments // 或者返回自定义的NotFound错误
	}

	return nil
}

// HardDelete 物理删除答卷
func (r *Repository) HardDelete(ctx context.Context, id uint64) error {
	filter := bson.M{
		"domain_id": id,
	}

	result, err := r.Collection().DeleteOne(ctx, filter)
	if err != nil {
		return err
	}

	if result.DeletedCount == 0 {
		return mongo.ErrNoDocuments // 或者返回自定义的NotFound错误
	}

	return nil
}

// ExistsByID 检查ID是否存在
func (r *Repository) ExistsByID(ctx context.Context, id uint64) (bool, error) {
	filter := bson.M{
		"domain_id": id,
	}

	return r.ExistsByFilter(ctx, filter)
}
