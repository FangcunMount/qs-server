package scale

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
)

// Repository Scale MongoDB 存储库
type Repository struct {
	mongoBase.BaseRepository
	mapper *ScaleMapper
}

// NewRepository 创建 Scale MongoDB 存储库
func NewRepository(db *mongo.Database, opts ...mongoBase.BaseRepositoryOptions) *Repository {
	po := &ScalePO{}
	return &Repository{
		BaseRepository: mongoBase.NewBaseRepository(db, po.CollectionName(), opts...),
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
			return nil, scale.ErrNotFound
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
			return nil, scale.ErrNotFound
		}
		return nil, err
	}

	return r.mapper.ToDomain(ctx, &po), nil
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
		return scale.ErrNotFound
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
		return scale.ErrNotFound
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
