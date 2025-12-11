package questionnaire

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
)

// Repository 问卷MongoDB存储库
type Repository struct {
	mongoBase.BaseRepository
	mapper *QuestionnaireMapper
}

// NewRepository 创建问卷MongoDB存储库
func NewRepository(db *mongo.Database) questionnaire.Repository {
	po := &QuestionnairePO{}
	return &Repository{
		BaseRepository: mongoBase.NewBaseRepository(db, po.CollectionName()),
		mapper:         NewQuestionnaireMapper(),
	}
}

// Create 创建问卷
func (r *Repository) Create(ctx context.Context, qDomain *questionnaire.Questionnaire) error {
	po := r.mapper.ToPO(qDomain)
	po.BeforeInsert()

	insertData, err := po.ToBsonM()
	if err != nil {
		return err
	}

	_, err = r.InsertOne(ctx, insertData)
	if err != nil {
		return err
	}

	return nil
}

// FindByCode 根据编码查询问卷
func (r *Repository) FindByCode(ctx context.Context, code string) (*questionnaire.Questionnaire, error) {
	filter := bson.M{
		"code": code,
	}

	var po QuestionnairePO
	err := r.FindOne(ctx, filter, &po)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // 或者返回自定义的NotFound错误
		}
		return nil, err
	}

	return r.mapper.ToBO(&po), nil
}

// FindByCodeVersion 根据编码和版本查询问卷
func (r *Repository) FindByCodeVersion(ctx context.Context, code, version string) (*questionnaire.Questionnaire, error) {
	filter := bson.M{
		"code":    code,
		"version": version,
	}

	var po QuestionnairePO
	err := r.FindOne(ctx, filter, &po)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // 或者返回自定义的NotFound错误
		}
		return nil, err
	}

	return r.mapper.ToBO(&po), nil
}

// Update 更新问卷
func (r *Repository) Update(ctx context.Context, qDomain *questionnaire.Questionnaire) error {
	po := r.mapper.ToPO(qDomain)
	po.BeforeUpdate()

	// 根据领域ID查找文档
	filter := bson.M{"code": qDomain.GetCode().Value()}

	// 将领域模型转换为BSON M
	updateData, err := po.ToBsonM()
	if err != nil {
		return err
	}

	// 使用 $set 操作符包装更新数据，避免覆盖其他字段
	update := bson.M{"$set": updateData}

	_, err = r.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	return nil
}

// Remove 删除问卷（软删除）
func (r *Repository) Remove(ctx context.Context, code string) error {
	filter := bson.M{"code": code}

	now := time.Now()
	update := bson.M{
		"$set": bson.M{
			"deleted_at": now,
			"deleted_by": 0, // 这里应该从上下文中获取当前用户ID
			"updated_at": now,
		},
	}

	result, err := r.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return mongo.ErrNoDocuments // 或者返回自定义的NotFound错误
	}

	return nil
}

// HardDelete 物理删除问卷
func (r *Repository) HardDelete(ctx context.Context, code string) error {
	filter := bson.M{"code": code}

	result, err := r.DeleteOne(ctx, filter)
	if err != nil {
		return err
	}

	if result.DeletedCount == 0 {
		return mongo.ErrNoDocuments // 或者返回自定义的NotFound错误
	}

	return nil
}

// ExistsByCode 检查编码是否存在
func (r *Repository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	filter := bson.M{
		"code":       code,
		"deleted_at": nil,
	}

	return r.ExistsByFilter(ctx, filter)
}

// CountWithConditions 根据条件统计问卷数量
func (r *Repository) CountWithConditions(ctx context.Context, conditions map[string]interface{}) (int64, error) {
	filter := bson.M{
		"deleted_at": nil,
	}

	// 添加条件过滤
	if code, ok := conditions["code"].(string); ok && code != "" {
		filter["code"] = code
	}
	if title, ok := conditions["title"].(string); ok && title != "" {
		filter["title"] = bson.M{"$regex": title, "$options": "i"}
	}
	if status, ok := conditions["status"]; ok && status != nil {
		filter["status"] = status // 直接使用，支持 uint8 或其他类型
	}

	return r.CountDocuments(ctx, filter)
}

// FindSummaryList 查询问卷摘要列表（轻量级，使用 Projection 排除 questions 字段）
func (r *Repository) FindSummaryList(ctx context.Context, page, pageSize int, conditions map[string]interface{}) ([]*questionnaire.QuestionnaireSummary, error) {
	filter := bson.M{
		"deleted_at": nil,
	}

	// 添加条件过滤
	if code, ok := conditions["code"].(string); ok && code != "" {
		filter["code"] = code
	}
	if title, ok := conditions["title"].(string); ok && title != "" {
		filter["title"] = bson.M{"$regex": title, "$options": "i"} // 模糊查询，不区分大小写
	}
	if status, ok := conditions["status"]; ok && status != nil {
		filter["status"] = status // 直接使用，支持 uint8 或其他类型
	}

	// 计算分页
	skip := int64((page - 1) * pageSize)
	limit := int64(pageSize)

	// 创建查询选项，使用 Projection 排除 questions 字段
	opts := options.Find().
		SetSkip(skip).
		SetLimit(limit).
		SetProjection(bson.M{
			"_id":         1,
			"code":        1,
			"title":       1,
			"description": 1,
			"img_url":     1,
			"version":     1,
			"status":      1,
			// 排除 questions 字段，大幅减少内存占用
		})

	cursor, err := r.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var summaries []*questionnaire.QuestionnaireSummary
	for cursor.Next(ctx) {
		var po QuestionnaireSummaryPO
		if err := cursor.Decode(&po); err != nil {
			return nil, err
		}
		summaries = append(summaries, &questionnaire.QuestionnaireSummary{
			Code:        po.Code,
			Title:       po.Title,
			Description: po.Description,
			ImgUrl:      po.ImgUrl,
			Version:     po.Version,
			Status:      questionnaire.Status(po.Status),
		})
	}

	if err := cursor.Err(); err != nil {
		return nil, err
	}

	return summaries, nil
}
