package answersheet

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

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
		"deleted_at": nil,
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

// FindSummaryListByFiller 查询填写者的答卷摘要列表（使用聚合管道计算 answer_count）
func (r *Repository) FindSummaryListByFiller(ctx context.Context, fillerID uint64, page, pageSize int) ([]*answersheet.AnswerSheetSummary, error) {
	// 如果 pageSize <= 0，直接返回空列表（MongoDB limit 必须为正数）
	if pageSize <= 0 {
		return []*answersheet.AnswerSheetSummary{}, nil
	}

	filter := bson.M{
		"filler_id":  int64(fillerID),
		"deleted_at": nil,
	}

	// 计算分页
	skip := int64((page - 1) * pageSize)
	limit := int64(pageSize)

	// 使用聚合管道：计算 answer_count 并排除 answers 数组
	pipeline := []bson.M{
		{"$match": filter},
		{"$sort": bson.M{"filled_at": -1}},
		{"$skip": skip},
		{"$limit": limit},
		{"$project": bson.M{
			"domain_id":           1,
			"questionnaire_code":  1,
			"questionnaire_title": 1,
			"filler_id":           1,
			"filler_type":         1,
			"total_score":         1,
			"filled_at":           1,
			"answer_count":        bson.M{"$size": bson.M{"$ifNull": []interface{}{"$answers", []interface{}{}}}},
		}},
	}

	cursor, err := r.Collection().Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var summaries []*answersheet.AnswerSheetSummary
	for cursor.Next(ctx) {
		var po AnswerSheetSummaryPO
		if err := cursor.Decode(&po); err != nil {
			return nil, err
		}
		summary := &answersheet.AnswerSheetSummary{
			ID:                 meta.ID(po.DomainID),
			QuestionnaireCode:  po.QuestionnaireCode,
			QuestionnaireTitle: po.QuestionnaireTitle,
			FillerID:           uint64(po.FillerID),
			FillerType:         po.FillerType,
			TotalScore:         po.TotalScore,
			AnswerCount:        po.AnswerCount,
		}
		if po.FilledAt != nil {
			summary.FilledAt = *po.FilledAt
		}
		summaries = append(summaries, summary)
	}

	if err := cursor.Err(); err != nil {
		return nil, err
	}

	return summaries, nil
}

// FindSummaryListByQuestionnaire 查询问卷的答卷摘要列表（使用聚合管道计算 answer_count）
func (r *Repository) FindSummaryListByQuestionnaire(ctx context.Context, questionnaireCode string, page, pageSize int) ([]*answersheet.AnswerSheetSummary, error) {
	// 如果 pageSize <= 0，直接返回空列表（MongoDB limit 必须为正数）
	if pageSize <= 0 {
		return []*answersheet.AnswerSheetSummary{}, nil
	}

	filter := bson.M{
		"questionnaire_code": questionnaireCode,
		"deleted_at":         nil,
	}

	// 计算分页
	skip := int64((page - 1) * pageSize)
	limit := int64(pageSize)

	// 使用聚合管道：计算 answer_count 并排除 answers 数组
	pipeline := []bson.M{
		{"$match": filter},
		{"$sort": bson.M{"filled_at": -1}},
		{"$skip": skip},
		{"$limit": limit},
		{"$project": bson.M{
			"domain_id":           1,
			"questionnaire_code":  1,
			"questionnaire_title": 1,
			"filler_id":           1,
			"filler_type":         1,
			"total_score":         1,
			"filled_at":           1,
			"answer_count":        bson.M{"$size": bson.M{"$ifNull": []interface{}{"$answers", []interface{}{}}}},
		}},
	}

	cursor, err := r.Collection().Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var summaries []*answersheet.AnswerSheetSummary
	for cursor.Next(ctx) {
		var po AnswerSheetSummaryPO
		if err := cursor.Decode(&po); err != nil {
			return nil, err
		}
		summary := &answersheet.AnswerSheetSummary{
			ID:                 meta.ID(po.DomainID),
			QuestionnaireCode:  po.QuestionnaireCode,
			QuestionnaireTitle: po.QuestionnaireTitle,
			FillerID:           uint64(po.FillerID),
			FillerType:         po.FillerType,
			TotalScore:         po.TotalScore,
			AnswerCount:        po.AnswerCount,
		}
		if po.FilledAt != nil {
			summary.FilledAt = *po.FilledAt
		}
		summaries = append(summaries, summary)
	}

	if err := cursor.Err(); err != nil {
		return nil, err
	}

	return summaries, nil
}

// CountWithConditions 根据条件统计数量
func (r *Repository) CountWithConditions(ctx context.Context, conditions map[string]interface{}) (int64, error) {
	filter := bson.M(conditions)

	// 添加软删除过滤条件
	filter["deleted_at"] = nil

	return r.CountDocuments(ctx, filter)
}

// CountByFiller 统计填写者的答卷数量
func (r *Repository) CountByFiller(ctx context.Context, fillerID uint64) (int64, error) {
	filter := bson.M{
		"filler_id":  int64(fillerID),
		"deleted_at": nil,
	}
	return r.CountDocuments(ctx, filter)
}

// CountByQuestionnaire 统计问卷的答卷数量
func (r *Repository) CountByQuestionnaire(ctx context.Context, questionnaireCode string) (int64, error) {
	filter := bson.M{
		"questionnaire_code": questionnaireCode,
		"deleted_at":         nil,
	}
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
