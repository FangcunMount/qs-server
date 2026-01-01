package scale

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
)

// Repository Scale MongoDB 存储库
type Repository struct {
	mongoBase.BaseRepository
	mapper *ScaleMapper
}

// NewRepository 创建 Scale MongoDB 存储库
func NewRepository(db *mongo.Database) scale.Repository {
	po := &ScalePO{}
	return &Repository{
		BaseRepository: mongoBase.NewBaseRepository(db, po.CollectionName()),
		mapper:         NewScaleMapper(),
	}
}

// Create 创建量表
func (r *Repository) Create(ctx context.Context, domain *scale.MedicalScale) error {
	po := r.mapper.ToPO(domain)
	mongoBase.ApplyAuditCreate(ctx, po)
	po.BeforeInsert()

	insertData, err := po.ToBsonM()
	if err != nil {
		return err
	}

	_, err = r.InsertOne(ctx, insertData)
	return err
}

// FindByCode 根据编码查询量表
func (r *Repository) FindByCode(ctx context.Context, code string) (*scale.MedicalScale, error) {
	filter := bson.M{
		"code":       code,
		"deleted_at": nil, // 排除已软删除的记录
	}

	var po ScalePO
	err := r.FindOne(ctx, filter, &po)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, err
		}
		return nil, err
	}

	return r.mapper.ToDomain(ctx, &po), nil
}

// FindByQuestionnaireCode 根据问卷编码查询量表
func (r *Repository) FindByQuestionnaireCode(ctx context.Context, questionnaireCode string) (*scale.MedicalScale, error) {
	filter := bson.M{
		"questionnaire_code": questionnaireCode,
		"deleted_at":         nil,
	}

	var po ScalePO
	err := r.FindOne(ctx, filter, &po)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, err
		}
		return nil, err
	}

	return r.mapper.ToDomain(ctx, &po), nil
}

// FindSummaryList 分页查询量表摘要列表（不包含 factors）
func (r *Repository) FindSummaryList(ctx context.Context, page, pageSize int, conditions map[string]string) ([]*scale.ScaleSummary, error) {
	filter := r.buildFilter(conditions)

	// 设置分页选项和投影（排除 factors 字段）
	skip := int64((page - 1) * pageSize)
	limit := int64(pageSize)
	opts := options.Find().
		SetSkip(skip).
		SetLimit(limit).
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetProjection(bson.M{
			"code":               1,
			"title":              1,
			"description":        1,
			"category":           1,
			"stages":             1,
			"applicable_ages":    1,
			"reporters":          1,
			"tags":               1,
			"questionnaire_code": 1,
			"status":             1,
		})

	cursor, err := r.Collection().Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var poList []ScaleSummaryPO
	if err := cursor.All(ctx, &poList); err != nil {
		return nil, err
	}

	// 转换为领域摘要对象
	result := make([]*scale.ScaleSummary, 0, len(poList))
	for _, po := range poList {
		// 转换标签列表
		tags := make([]scale.Tag, 0, len(po.Tags))
		for _, tagStr := range po.Tags {
			tags = append(tags, scale.NewTag(tagStr))
		}

		// 转换填报人列表
		reporters := make([]scale.Reporter, 0, len(po.Reporters))
		for _, reporterStr := range po.Reporters {
			reporters = append(reporters, scale.NewReporter(reporterStr))
		}

		// 转换阶段列表
		stages := make([]scale.Stage, 0, len(po.Stages))
		for _, stageStr := range po.Stages {
			stages = append(stages, scale.NewStage(stageStr))
		}

		// 转换使用年龄列表
		applicableAges := make([]scale.ApplicableAge, 0, len(po.ApplicableAges))
		for _, ageStr := range po.ApplicableAges {
			applicableAges = append(applicableAges, scale.NewApplicableAge(ageStr))
		}

		result = append(result, &scale.ScaleSummary{
			Code:              po.Code,
			Title:             po.Title,
			Description:       po.Description,
			Category:          scale.NewCategory(po.Category),
			Stages:            stages,
			ApplicableAges:    applicableAges,
			Reporters:         reporters,
			Tags:              tags,
			QuestionnaireCode: po.QuestionnaireCode,
			Status:            scale.Status(po.Status),
		})
	}

	return result, nil
}

// CountWithConditions 根据条件统计量表数量
func (r *Repository) CountWithConditions(ctx context.Context, conditions map[string]string) (int64, error) {
	filter := r.buildFilter(conditions)
	return r.Collection().CountDocuments(ctx, filter)
}

// Update 更新量表
func (r *Repository) Update(ctx context.Context, domain *scale.MedicalScale) error {
	po := r.mapper.ToPO(domain)
	mongoBase.ApplyAuditUpdate(ctx, po)
	po.BeforeUpdate()

	filter := bson.M{
		"code":       domain.GetCode().String(),
		"deleted_at": nil,
	}

	updateData, err := po.ToBsonM()
	if err != nil {
		return err
	}

	update := bson.M{"$set": updateData}

	result, err := r.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return mongo.ErrNoDocuments
	}

	return nil
}

// Remove 删除量表（软删除）
func (r *Repository) Remove(ctx context.Context, code string) error {
	filter := bson.M{
		"code":       code,
		"deleted_at": nil,
	}

	now := time.Now()
	userID := mongoBase.AuditUserID(ctx)
	update := bson.M{
		"$set": bson.M{
			"deleted_at": now,
			"updated_at": now,
			"updated_by": userID,
			"deleted_by": userID,
		},
	}

	result, err := r.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return mongo.ErrNoDocuments
	}

	return nil
}

// ExistsByCode 检查编码是否存在
func (r *Repository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	filter := bson.M{
		"code":       code,
		"deleted_at": nil,
	}

	count, err := r.Collection().CountDocuments(ctx, filter)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// buildFilter 构建查询过滤条件
func (r *Repository) buildFilter(conditions map[string]string) bson.M {
	filter := bson.M{
		"deleted_at": nil, // 排除已软删除的记录
	}

	if conditions == nil {
		return filter
	}

	// 状态过滤
	if status, ok := conditions["status"]; ok && status != "" {
		// 将状态字符串转换为对应的数值
		switch status {
		case "草稿", "draft":
			filter["status"] = uint8(0)
		case "已发布", "published":
			filter["status"] = uint8(1)
		case "已归档", "archived":
			filter["status"] = uint8(2)
		}
	}

	// 标题模糊搜索
	if title, ok := conditions["title"]; ok && title != "" {
		filter["title"] = bson.M{"$regex": title, "$options": "i"}
	}

	// 问卷编码过滤
	if qCode, ok := conditions["questionnaire_code"]; ok && qCode != "" {
		filter["questionnaire_code"] = qCode
	}

	// 主类过滤
	if category, ok := conditions["category"]; ok && category != "" {
		filter["category"] = category
	}

	return filter
}
