package medicalscale

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	medicalScale "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/medical-scale"
	medicalscale "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/medical-scale"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/medical-scale/port"
	mongoBase "github.com/yshujie/questionnaire-scale/internal/apiserver/infrastructure/mongo"
	v1 "github.com/yshujie/questionnaire-scale/pkg/meta/v1"
)

// Repository 医学量表MongoDB存储库
type Repository struct {
	mongoBase.BaseRepository
	mapper *MedicalScaleMapper
}

// NewRepository 创建医学量表MongoDB存储库
func NewRepository(db *mongo.Database) port.MedicalScaleRepositoryMongo {
	po := &MedicalScalePO{}
	return &Repository{
		BaseRepository: mongoBase.NewBaseRepository(db, po.CollectionName()),
		mapper:         NewMedicalScaleMapper(),
	}
}

// Create 创建医学量表
func (r *Repository) Create(ctx context.Context, scale *medicalscale.MedicalScale) error {
	po := r.mapper.ToPO(scale)
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
	scale.SetID(v1.NewID(po.DomainID))

	return nil
}

// FindByID 根据ID查找医学量表
func (r *Repository) FindByID(ctx context.Context, id v1.ID) (*medicalScale.MedicalScale, error) {
	objectID, err := mongoBase.Uint64ToObjectID(id.Value())
	if err != nil {
		return nil, err
	}

	filter := bson.M{
		"_id":        objectID,
		"deleted_at": bson.M{"$exists": false},
	}

	var po MedicalScalePO
	err = r.FindOne(ctx, filter, &po)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}

	return r.mapper.ToBO(&po), nil
}

// FindByCode 根据代码查找医学量表
func (r *Repository) FindByCode(ctx context.Context, code string) (*medicalScale.MedicalScale, error) {
	filter := bson.M{
		"code":       code,
		"deleted_at": bson.M{"$exists": false},
	}

	var po MedicalScalePO
	err := r.FindOne(ctx, filter, &po)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}

	return r.mapper.ToBO(&po), nil
}

// FindByQuestionnaireCode 根据问卷代码查找医学量表列表
func (r *Repository) FindByQuestionnaireCode(ctx context.Context, questionnaireCode string) (*medicalscale.MedicalScale, error) {
	filter := bson.M{
		"questionnaire_code": questionnaireCode,
		"deleted_at":         bson.M{"$exists": false},
	}

	cursor, err := r.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var scales []*medicalScale.MedicalScale
	for cursor.Next(ctx) {
		var po MedicalScalePO
		if err := cursor.Decode(&po); err != nil {
			return nil, err
		}
		scales = append(scales, r.mapper.ToBO(&po))
	}

	if err := cursor.Err(); err != nil {
		return nil, err
	}

	return scales[0], nil
}

// Update 更新医学量表
func (r *Repository) Update(ctx context.Context, scale *medicalscale.MedicalScale) error {
	po := r.mapper.ToPO(scale)
	po.BeforeUpdate()

	// 根据代码查找文档
	filter := bson.M{
		"code":       scale.GetCode(),
		"deleted_at": bson.M{"$exists": false},
	}

	// 将领域模型转换为BSON M
	updateData, err := po.ToBsonM()
	if err != nil {
		return err
	}

	// 移除不需要更新的字段
	delete(updateData, "_id")
	delete(updateData, "created_at")
	delete(updateData, "created_by")

	// 使用 $set 操作符包装更新数据
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

// Delete 删除医学量表（软删除）
func (r *Repository) Delete(ctx context.Context, id v1.ID) error {
	objectID, err := mongoBase.Uint64ToObjectID(id.Value())
	if err != nil {
		return err
	}

	filter := bson.M{
		"_id":        objectID,
		"deleted_at": bson.M{"$exists": false},
	}

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
		return mongo.ErrNoDocuments
	}

	return nil
}

// ExistsByCode 检查代码是否已存在
func (r *Repository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	filter := bson.M{
		"code":       code,
		"deleted_at": bson.M{"$exists": false},
	}

	count, err := r.CountDocuments(ctx, filter)
	return count > 0, err
}

// HardDelete 硬删除医学量表
func (r *Repository) HardDelete(ctx context.Context, id v1.ID) error {
	objectID, err := mongoBase.Uint64ToObjectID(id.Value())
	if err != nil {
		return err
	}

	filter := bson.M{"_id": objectID}

	result, err := r.DeleteOne(ctx, filter)
	if err != nil {
		return err
	}

	if result.DeletedCount == 0 {
		return mongo.ErrNoDocuments
	}

	return nil
}

// FindList 根据条件查找医学量表列表
func (r *Repository) FindList(ctx context.Context, page, pageSize int, conditions map[string]string) ([]*medicalScale.MedicalScale, error) {
	// 构建查询条件
	filter := bson.M{
		"deleted_at": bson.M{"$exists": false},
	}

	// 添加条件过滤
	for key, value := range conditions {
		if value != "" {
			switch key {
			case "title":
				filter["title"] = bson.M{"$regex": value, "$options": "i"}
			case "questionnaire_code":
				filter["questionnaire_code"] = value
			case "code":
				filter["code"] = value
			}
		}
	}

	// 计算跳过的文档数
	skip := (page - 1) * pageSize

	// 设置分页选项
	opts := options.Find().
		SetSkip(int64(skip)).
		SetLimit(int64(pageSize)).
		SetSort(bson.M{"created_at": -1}) // 按创建时间倒序

	// 执行查询
	cursor, err := r.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var scales []*medicalScale.MedicalScale
	for cursor.Next(ctx) {
		var po MedicalScalePO
		if err := cursor.Decode(&po); err != nil {
			return nil, err
		}
		scales = append(scales, r.mapper.ToBO(&po))
	}

	if err := cursor.Err(); err != nil {
		return nil, err
	}

	return scales, nil
}

// CountWithConditions 根据条件计算医学量表数量
func (r *Repository) CountWithConditions(ctx context.Context, conditions map[string]string) (int64, error) {
	// 构建查询条件
	filter := bson.M{
		"deleted_at": bson.M{"$exists": false},
	}

	// 添加条件过滤
	for key, value := range conditions {
		if value != "" {
			switch key {
			case "title":
				filter["title"] = bson.M{"$regex": value, "$options": "i"}
			case "questionnaire_code":
				filter["questionnaire_code"] = value
			case "code":
				filter["code"] = value
			}
		}
	}

	return r.CountDocuments(ctx, filter)
}
