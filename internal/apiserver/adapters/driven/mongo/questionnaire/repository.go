package questionnaire

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	mongoBase "github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/driven/mongo"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire/port"
)

// Repository 问卷MongoDB存储库
type Repository struct {
	mongoBase.BaseRepository
	mapper *QuestionnaireMapper
}

// NewRepository 创建问卷MongoDB存储库
func NewRepository(db *mongo.Database) port.QuestionnaireRepository {
	doc := &QuestionnaireDocument{}
	return &Repository{
		BaseRepository: mongoBase.NewBaseRepository(db, doc.CollectionName()),
		mapper:         NewQuestionnaireMapper(),
	}
}

// Save 保存问卷
func (r *Repository) Save(ctx context.Context, qDomain *questionnaire.Questionnaire) error {
	doc := r.mapper.ToDocument(qDomain)
	doc.BeforeInsert()

	result, err := r.InsertOne(ctx, doc)
	if err != nil {
		return err
	}

	// 同步生成的ID和时间戳回领域对象
	if oid, ok := result.InsertedID.(primitive.ObjectID); ok {
		domainID := r.mapper.Uint64FromObjectID(oid)
		qDomain.SetID(questionnaire.NewQuestionnaireID(domainID))
	}

	qDomain.SetCreatedAt(doc.CreatedAt)
	qDomain.SetUpdatedAt(doc.UpdatedAt)
	qDomain.SetCreatedBy(doc.CreatedBy)
	qDomain.SetUpdatedBy(doc.UpdatedBy)

	return nil
}

// FindByID 根据ID查询问卷
func (r *Repository) FindByID(ctx context.Context, id uint64) (*questionnaire.Questionnaire, error) {
	// 方法1: 根据自定义字段查询（如果你在文档中存储了uint64 ID）
	filter := bson.M{"domain_id": id} // 假设你在文档中添加了domain_id字段

	var doc QuestionnaireDocument
	err := r.FindOne(ctx, filter, &doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // 或者返回自定义的NotFound错误
		}
		return nil, err
	}

	return r.mapper.ToDomain(&doc), nil
}

// FindByCode 根据编码查询问卷
func (r *Repository) FindByCode(ctx context.Context, code string) (*questionnaire.Questionnaire, error) {
	filter := bson.M{
		"code":       code,
		"deleted_at": bson.M{"$exists": false}, // 排除已删除的文档
	}

	var doc QuestionnaireDocument
	err := r.FindOne(ctx, filter, &doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // 或者返回自定义的NotFound错误
		}
		return nil, err
	}

	return r.mapper.ToDomain(&doc), nil
}

// Update 更新问卷
func (r *Repository) Update(ctx context.Context, qDomain *questionnaire.Questionnaire) error {
	doc := r.mapper.ToDocument(qDomain)
	doc.BeforeUpdate()

	// 根据领域ID查找文档
	filter := bson.M{"domain_id": qDomain.ID.Value()}

	update := bson.M{
		"$set": bson.M{
			"code":       doc.Code,
			"title":      doc.Title,
			"img_url":    doc.ImgUrl,
			"version":    doc.Version,
			"status":     doc.Status,
			"updated_at": doc.UpdatedAt,
			"updated_by": doc.UpdatedBy,
		},
	}

	result, err := r.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return mongo.ErrNoDocuments // 或者返回自定义的NotFound错误
	}

	// 同步更新时间回领域对象
	qDomain.SetUpdatedAt(doc.UpdatedAt)
	qDomain.SetUpdatedBy(doc.UpdatedBy)

	return nil
}

// Remove 删除问卷（软删除）
func (r *Repository) Remove(ctx context.Context, id uint64) error {
	filter := bson.M{"domain_id": id}

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
func (r *Repository) HardDelete(ctx context.Context, id uint64) error {
	filter := bson.M{"domain_id": id}

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
		var doc QuestionnaireDocument
		if err := cursor.Decode(&doc); err != nil {
			return nil, err
		}
		questionnaires = append(questionnaires, r.mapper.ToDomain(&doc))
	}

	if err := cursor.Err(); err != nil {
		return nil, err
	}

	return questionnaires, nil
}
