package questionnaire

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/storage/mongodb"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/ports/storage"
)

// Repository MongoDB 问卷文档仓储实现
type Repository struct {
	*mongodb.BaseRepository
	converter  *Converter
	collection string
}

// NewRepository 创建问卷文档仓储
func NewRepository(client *mongo.Client, database string) storage.QuestionnaireDocumentRepository {
	return &Repository{
		BaseRepository: mongodb.NewBaseRepository(client, database),
		converter:      NewConverter(),
		collection:     "questionnaire_docs",
	}
}

// SaveDocument 保存问卷文档
func (r *Repository) SaveDocument(ctx context.Context, q *questionnaire.Questionnaire) error {
	// 转换为文档模型
	doc := r.converter.DomainToDocument(q)
	if doc == nil {
		return fmt.Errorf("failed to convert questionnaire to document")
	}

	// 验证并准备文档
	preparedDoc, err := r.converter.ValidateAndPrepare(doc, false)
	if err != nil {
		return fmt.Errorf("failed to prepare document for save: %w", err)
	}

	// 保存文档
	_, err = r.InsertOne(ctx, r.collection, preparedDoc)
	if err != nil {
		return fmt.Errorf("failed to save questionnaire document: %w", err)
	}

	return nil
}

// GetDocument 获取问卷文档
func (r *Repository) GetDocument(ctx context.Context, id questionnaire.QuestionnaireID) (*storage.QuestionnaireDocumentResult, error) {
	filter := bson.M{"_id": id.Value()}

	var doc Document
	err := r.FindOne(ctx, r.collection, filter, &doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, questionnaire.ErrQuestionnaireNotFound
		}
		return nil, fmt.Errorf("failed to get questionnaire document: %w", err)
	}

	return r.converter.DocumentToResult(&doc), nil
}

// UpdateDocument 更新问卷文档
func (r *Repository) UpdateDocument(ctx context.Context, q *questionnaire.Questionnaire) error {
	// 转换为文档模型
	doc := r.converter.DomainToDocument(q)
	if doc == nil {
		return fmt.Errorf("failed to convert questionnaire to document")
	}

	// 验证并准备文档
	preparedDoc, err := r.converter.ValidateAndPrepare(doc, true)
	if err != nil {
		return fmt.Errorf("failed to prepare document for update: %w", err)
	}

	filter := bson.M{"_id": q.ID().Value()}
	update := bson.M{"$set": preparedDoc}

	err = r.UpdateOne(ctx, r.collection, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update questionnaire document: %w", err)
	}

	return nil
}

// RemoveDocument 删除问卷文档
func (r *Repository) RemoveDocument(ctx context.Context, id questionnaire.QuestionnaireID) error {
	filter := bson.M{"_id": id.Value()}

	err := r.DeleteOne(ctx, r.collection, filter)
	if err != nil {
		return fmt.Errorf("failed to remove questionnaire document: %w", err)
	}

	return nil
}

// FindDocumentsByQuestionnaireIDs 批量获取问卷文档
func (r *Repository) FindDocumentsByQuestionnaireIDs(ctx context.Context, ids []questionnaire.QuestionnaireID) (map[string]*storage.QuestionnaireDocumentResult, error) {
	// 转换ID为字符串切片
	idStrings := make([]string, len(ids))
	for i, id := range ids {
		idStrings[i] = id.Value()
	}

	// 构建查询过滤器
	filter := r.converter.BuildDocumentFilter(idStrings)

	// 执行查询
	cursor, err := r.Find(ctx, r.collection, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to find questionnaire documents: %w", err)
	}
	defer cursor.Close(ctx)

	// 构建结果映射
	result := make(map[string]*storage.QuestionnaireDocumentResult)
	for cursor.Next(ctx) {
		var doc Document
		if err := cursor.Decode(&doc); err != nil {
			return nil, fmt.Errorf("failed to decode questionnaire document: %w", err)
		}
		result[doc.ID] = r.converter.DocumentToResult(&doc)
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("cursor error: %w", err)
	}

	return result, nil
}

// SearchDocuments 搜索问卷文档
func (r *Repository) SearchDocuments(ctx context.Context, query storage.DocumentSearchQuery) ([]*storage.QuestionnaireDocumentResult, error) {
	// 构建搜索过滤器
	filter := r.converter.BuildSearchFilter(query)

	// 构建查询选项
	findOptions := options.Find()
	if query.Limit > 0 {
		findOptions.SetLimit(int64(query.Limit))
	}
	if query.Skip > 0 {
		findOptions.SetSkip(int64(query.Skip))
	}

	// 执行查询
	cursor, err := r.Find(ctx, r.collection, filter, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to search questionnaire documents: %w", err)
	}
	defer cursor.Close(ctx)

	// 解码结果
	var docs []*Document
	for cursor.Next(ctx) {
		var doc Document
		if err := cursor.Decode(&doc); err != nil {
			return nil, fmt.Errorf("failed to decode questionnaire document: %w", err)
		}
		docs = append(docs, &doc)
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("cursor error: %w", err)
	}

	// 转换为结果
	return r.converter.DocumentsToResults(docs), nil
}

// GetCollectionName 获取集合名称
func (r *Repository) GetCollectionName() string {
	return r.collection
}

// EnsureIndexes 确保索引存在
func (r *Repository) EnsureIndexes(ctx context.Context) error {
	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "questions.title", Value: "text"},
			},
			Options: options.Index().SetName("text_search_questions_title"),
		},
		{
			Keys: bson.D{
				{Key: "created_at", Value: 1},
			},
			Options: options.Index().SetName("idx_created_at"),
		},
		{
			Keys: bson.D{
				{Key: "updated_at", Value: 1},
			},
			Options: options.Index().SetName("idx_updated_at"),
		},
		{
			Keys: bson.D{
				{Key: "version", Value: 1},
			},
			Options: options.Index().SetName("idx_version"),
		},
	}

	return r.BaseRepository.EnsureIndexes(ctx, r.collection, indexes)
}

// SearchWithPagination 带分页的搜索功能
func (r *Repository) SearchWithPagination(ctx context.Context, query storage.DocumentSearchQuery, page, pageSize int) (*mongodb.PaginatedResult, error) {
	// 构建分页查询
	paginationQuery := r.NewPaginationQuery().
		WithFilter(r.converter.BuildSearchFilter(query)).
		WithSort(bson.D{{Key: "updated_at", Value: -1}}) // 按更新时间倒序

	// 执行分页查询
	decoder := func(cursor *mongo.Cursor) ([]interface{}, error) {
		var items []interface{}
		for cursor.Next(ctx) {
			var doc Document
			if err := cursor.Decode(&doc); err != nil {
				return nil, err
			}
			items = append(items, r.converter.DocumentToResult(&doc))
		}
		return items, nil
	}

	return paginationQuery.ExecutePaginated(ctx, r.BaseRepository, r.collection, page, pageSize, decoder)
}

// GetDocumentStatistics 获取文档统计信息
func (r *Repository) GetDocumentStatistics(ctx context.Context) (map[string]interface{}, error) {
	// 统计总数
	totalCount, err := r.Count(ctx, r.collection, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("failed to count total documents: %w", err)
	}

	// 统计各类型问题数量（聚合查询）
	pipeline := []bson.M{
		{
			"$unwind": "$questions",
		},
		{
			"$group": bson.M{
				"_id":   "$questions.type",
				"count": bson.M{"$sum": 1},
			},
		},
	}

	cursor, err := r.Aggregate(ctx, r.collection, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate question types: %w", err)
	}
	defer cursor.Close(ctx)

	questionTypes := make(map[string]int)
	for cursor.Next(ctx) {
		var result struct {
			ID    string `bson:"_id"`
			Count int    `bson:"count"`
		}
		if err := cursor.Decode(&result); err != nil {
			return nil, fmt.Errorf("failed to decode aggregation result: %w", err)
		}
		questionTypes[result.ID] = result.Count
	}

	return map[string]interface{}{
		"total_documents": totalCount,
		"question_types":  questionTypes,
		"collection_name": r.collection,
		"database_name":   r.GetDatabase(),
	}, nil
}

// BulkSaveDocuments 批量保存文档
func (r *Repository) BulkSaveDocuments(ctx context.Context, questionnaires []*questionnaire.Questionnaire) error {
	if len(questionnaires) == 0 {
		return nil
	}

	// 构建批量写操作
	var models []mongo.WriteModel
	for _, q := range questionnaires {
		doc := r.converter.DomainToDocument(q)
		if doc == nil {
			return fmt.Errorf("failed to convert questionnaire %s to document", q.ID().Value())
		}

		preparedDoc, err := r.converter.ValidateAndPrepare(doc, false)
		if err != nil {
			return fmt.Errorf("failed to prepare document %s for save: %w", q.ID().Value(), err)
		}

		model := mongo.NewInsertOneModel().SetDocument(preparedDoc)
		models = append(models, model)
	}

	// 执行批量写入
	_, err := r.BulkWrite(ctx, r.collection, models)
	if err != nil {
		return fmt.Errorf("failed to bulk save questionnaire documents: %w", err)
	}

	return nil
}

// BulkUpdateDocuments 批量更新文档
func (r *Repository) BulkUpdateDocuments(ctx context.Context, questionnaires []*questionnaire.Questionnaire) error {
	if len(questionnaires) == 0 {
		return nil
	}

	// 构建批量写操作
	var models []mongo.WriteModel
	for _, q := range questionnaires {
		doc := r.converter.DomainToDocument(q)
		if doc == nil {
			return fmt.Errorf("failed to convert questionnaire %s to document", q.ID().Value())
		}

		preparedDoc, err := r.converter.ValidateAndPrepare(doc, true)
		if err != nil {
			return fmt.Errorf("failed to prepare document %s for update: %w", q.ID().Value(), err)
		}

		filter := bson.M{"_id": q.ID().Value()}
		update := bson.M{"$set": preparedDoc}

		model := mongo.NewUpdateOneModel().
			SetFilter(filter).
			SetUpdate(update).
			SetUpsert(false)
		models = append(models, model)
	}

	// 执行批量更新
	_, err := r.BulkWrite(ctx, r.collection, models)
	if err != nil {
		return fmt.Errorf("failed to bulk update questionnaire documents: %w", err)
	}

	return nil
}
