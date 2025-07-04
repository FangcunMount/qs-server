package questionnaire

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/port"
	mongoBase "github.com/yshujie/questionnaire-scale/internal/apiserver/infrastructure/mongo"
)

// Repository 问卷MongoDB存储库
type Repository struct {
	mongoBase.BaseRepository
	mapper *QuestionnaireMapper
}

// NewRepository 创建问卷MongoDB存储库
func NewRepository(db *mongo.Database) port.QuestionnaireRepositoryMongo {
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

	_, err := r.InsertOne(ctx, po)
	if err != nil {
		return err
	}

	return nil
}

// FindByCode 根据编码查询问卷
func (r *Repository) FindByCode(ctx context.Context, code string) (*questionnaire.Questionnaire, error) {
	filter := bson.M{
		"code":       code,
		"deleted_at": bson.M{"$exists": false}, // 排除已删除的文档
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

	update := bson.M{
		"$set": bson.M{
			"code":       po.Code,
			"title":      po.Title,
			"img_url":    po.ImgUrl,
			"version":    po.Version,
			"status":     po.Status,
			"updated_at": po.UpdatedAt,
			"updated_by": po.UpdatedBy,
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
		"deleted_at": bson.M{"$exists": false},
	}

	return r.ExistsByFilter(ctx, filter)
}

// FindActiveQuestionnaires 查找活跃的问卷
func (r *Repository) FindActiveQuestionnaires(ctx context.Context) ([]*questionnaire.Questionnaire, error) {
	filter := bson.M{
		"status":     1, // StatusActive
		"deleted_at": bson.M{"$exists": false},
	}

	cursor, err := r.Find(ctx, filter)
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
