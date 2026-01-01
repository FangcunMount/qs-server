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

	return nil
}

// FindByCode 根据编码查询问卷
func (r *Repository) FindByCode(ctx context.Context, code string) (*questionnaire.Questionnaire, error) {
	q, err := r.FindBaseByCode(ctx, code)
	if err != nil || q == nil {
		return q, err
	}
	if err := r.LoadQuestions(ctx, q); err != nil {
		return nil, err
	}
	return q, nil
}

// FindByCodeVersion 根据编码和版本查询问卷
func (r *Repository) FindByCodeVersion(ctx context.Context, code, version string) (*questionnaire.Questionnaire, error) {
	q, err := r.FindBaseByCodeVersion(ctx, code, version)
	if err != nil || q == nil {
		return q, err
	}
	if err := r.LoadQuestions(ctx, q); err != nil {
		return nil, err
	}
	return q, nil
}

// FindBaseByCode 根据编码查询问卷基础信息（不含问题详情）
func (r *Repository) FindBaseByCode(ctx context.Context, code string) (*questionnaire.Questionnaire, error) {
	filter := bson.M{
		"code": code,
	}

	po, err := r.aggregateBase(ctx, filter)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}

	return r.mapper.ToBO(po), nil
}

// FindBaseByCodeVersion 根据编码和版本查询问卷基础信息（不含问题详情）
func (r *Repository) FindBaseByCodeVersion(ctx context.Context, code, version string) (*questionnaire.Questionnaire, error) {
	filter := bson.M{
		"code":    code,
		"version": version,
	}

	po, err := r.aggregateBase(ctx, filter)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}

	return r.mapper.ToBO(po), nil
}

// LoadQuestions 加载问卷问题详情
func (r *Repository) LoadQuestions(ctx context.Context, qDomain *questionnaire.Questionnaire) error {
	filter := bson.M{
		"code": qDomain.GetCode().Value(),
	}
	if qDomain.GetVersion().String() != "" {
		filter["version"] = qDomain.GetVersion().String()
	}

	projection := bson.M{"questions": 1}
	var po QuestionnairePO
	if err := r.Collection().FindOne(ctx, filter, options.FindOne().SetProjection(projection)).Decode(&po); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil
		}
		return err
	}

	qDomain.SetQuestions(r.mapper.mapQuestions(po.Questions))
	return nil
}

// Update 更新问卷
func (r *Repository) Update(ctx context.Context, qDomain *questionnaire.Questionnaire) error {
	po := r.mapper.ToPO(qDomain)
	mongoBase.ApplyAuditUpdate(ctx, po)
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
	userID := mongoBase.AuditUserID(ctx)
	update := bson.M{
		"$set": bson.M{
			"deleted_at": now,
			"deleted_by": userID,
			"updated_at": now,
			"updated_by": userID,
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
	if typ, ok := conditions["type"].(string); ok && typ != "" {
		filter["type"] = typ
	}

	return r.CountDocuments(ctx, filter)
}

// FindBaseList 查询问卷基础列表（轻量级，使用聚合管道计算 question_count）
func (r *Repository) FindBaseList(ctx context.Context, page, pageSize int, conditions map[string]interface{}) ([]*questionnaire.Questionnaire, error) {
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
	if typ, ok := conditions["type"].(string); ok && typ != "" {
		filter["type"] = typ
	}

	// 计算分页
	skip := int64((page - 1) * pageSize)
	limit := int64(pageSize)

	pipeline := buildBasePipeline(filter, &skip, &limit)

	cursor, err := r.Collection().Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var questionnaires []*questionnaire.Questionnaire
	for cursor.Next(ctx) {
		var po QuestionnairePO
		if err := cursor.Decode(&po); err != nil {
			return nil, err
		}
		questionnaires = append(questionnaires, r.mapper.ToBO(&po))
	}

	if err := cursor.Err(); err != nil {
		return nil, err
	}

	return questionnaires, nil
}

func (r *Repository) aggregateBase(ctx context.Context, filter bson.M) (*QuestionnairePO, error) {
	limit := int64(1)
	pipeline := buildBasePipeline(filter, nil, &limit)
	cursor, err := r.Collection().Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	if !cursor.Next(ctx) {
		if err := cursor.Err(); err != nil {
			return nil, err
		}
		return nil, mongo.ErrNoDocuments
	}

	var po QuestionnairePO
	if err := cursor.Decode(&po); err != nil {
		return nil, err
	}
	return &po, nil
}

func buildBasePipeline(filter bson.M, skip, limit *int64) []bson.M {
	pipeline := []bson.M{
		{"$match": filter},
	}
	if skip != nil {
		pipeline = append(pipeline, bson.M{"$skip": *skip})
	}
	if limit != nil {
		pipeline = append(pipeline, bson.M{"$limit": *limit})
	}
	pipeline = append(pipeline, bson.M{"$project": bson.M{
		"code":           1,
		"title":          1,
		"description":    1,
		"img_url":        1,
		"version":        1,
		"status":         1,
		"type":           1,
		"question_count": bson.M{"$size": bson.M{"$ifNull": []interface{}{"$questions", []interface{}{}}}},
		"created_by":     1,
		"created_at":     1,
		"updated_by":     1,
		"updated_at":     1,
	}})
	return pipeline
}
