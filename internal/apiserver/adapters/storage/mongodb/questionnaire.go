package mongodb

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/ports/storage"
)

// questionnaireDocumentRepository MongoDB 问卷文档仓储
// 专门负责存储问卷的文档结构（问题、设置等）
type questionnaireDocumentRepository struct {
	client     *mongo.Client
	database   string
	collection string
}

// NewQuestionnaireDocumentRepository 创建问卷文档仓储
func NewQuestionnaireDocumentRepository(client *mongo.Client, database string) storage.QuestionnaireDocumentRepository {
	return &questionnaireDocumentRepository{
		client:     client,
		database:   database,
		collection: "questionnaire_docs",
	}
}

// questionnaireDocument MongoDB 文档模型
type questionnaireDocument struct {
	ID        string             `bson:"_id"`
	Questions []questionDocument `bson:"questions"`
	Settings  settingsDocument   `bson:"settings"`
	Version   int                `bson:"version"`
	CreatedAt time.Time          `bson:"created_at"`
	UpdatedAt time.Time          `bson:"updated_at"`
}

// questionDocument 问题文档
type questionDocument struct {
	ID       string                 `bson:"id"`
	Type     string                 `bson:"type"`
	Title    string                 `bson:"title"`
	Required bool                   `bson:"required"`
	Options  []optionDocument       `bson:"options"`
	Settings map[string]interface{} `bson:"settings"`
	Order    int                    `bson:"order"`
}

// optionDocument 选项文档
type optionDocument struct {
	ID    string `bson:"id"`
	Text  string `bson:"text"`
	Value string `bson:"value"`
	Order int    `bson:"order"`
}

// settingsDocument 设置文档
type settingsDocument struct {
	AllowAnonymous bool   `bson:"allow_anonymous"`
	ShowProgress   bool   `bson:"show_progress"`
	RandomOrder    bool   `bson:"random_order"`
	TimeLimit      *int64 `bson:"time_limit,omitempty"` // 秒数
}

// SaveDocument 保存问卷文档
func (r *questionnaireDocumentRepository) SaveDocument(ctx context.Context, q *questionnaire.Questionnaire) error {
	collection := r.client.Database(r.database).Collection(r.collection)

	doc := r.domainToDocument(q)

	_, err := collection.InsertOne(ctx, doc)
	if err != nil {
		return fmt.Errorf("failed to save questionnaire document: %w", err)
	}

	return nil
}

// GetDocument 获取问卷文档
func (r *questionnaireDocumentRepository) GetDocument(ctx context.Context, id questionnaire.QuestionnaireID) (*storage.QuestionnaireDocumentResult, error) {
	collection := r.client.Database(r.database).Collection(r.collection)

	var doc questionnaireDocument
	err := collection.FindOne(ctx, bson.M{"_id": id.Value()}).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, questionnaire.ErrQuestionnaireNotFound
		}
		return nil, fmt.Errorf("failed to get questionnaire document: %w", err)
	}

	return r.documentToResult(&doc), nil
}

// UpdateDocument 更新问卷文档
func (r *questionnaireDocumentRepository) UpdateDocument(ctx context.Context, q *questionnaire.Questionnaire) error {
	collection := r.client.Database(r.database).Collection(r.collection)

	doc := r.domainToDocument(q)
	doc.UpdatedAt = time.Now()

	filter := bson.M{"_id": q.ID().Value()}
	update := bson.M{"$set": doc}

	_, err := collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("failed to update questionnaire document: %w", err)
	}

	return nil
}

// RemoveDocument 删除问卷文档
func (r *questionnaireDocumentRepository) RemoveDocument(ctx context.Context, id questionnaire.QuestionnaireID) error {
	collection := r.client.Database(r.database).Collection(r.collection)

	_, err := collection.DeleteOne(ctx, bson.M{"_id": id.Value()})
	if err != nil {
		return fmt.Errorf("failed to remove questionnaire document: %w", err)
	}

	return nil
}

// FindDocumentsByQuestionnaireIDs 批量获取问卷文档
func (r *questionnaireDocumentRepository) FindDocumentsByQuestionnaireIDs(ctx context.Context, ids []questionnaire.QuestionnaireID) (map[string]*storage.QuestionnaireDocumentResult, error) {
	collection := r.client.Database(r.database).Collection(r.collection)

	// 构建查询条件
	idStrings := make([]string, len(ids))
	for i, id := range ids {
		idStrings[i] = id.Value()
	}

	filter := bson.M{"_id": bson.M{"$in": idStrings}}
	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to find questionnaire documents: %w", err)
	}
	defer cursor.Close(ctx)

	// 构建结果映射
	result := make(map[string]*storage.QuestionnaireDocumentResult)
	for cursor.Next(ctx) {
		var doc questionnaireDocument
		if err := cursor.Decode(&doc); err != nil {
			return nil, fmt.Errorf("failed to decode questionnaire document: %w", err)
		}
		result[doc.ID] = r.documentToResult(&doc)
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("cursor error: %w", err)
	}

	return result, nil
}

// SearchDocuments 搜索问卷文档
func (r *questionnaireDocumentRepository) SearchDocuments(ctx context.Context, query storage.DocumentSearchQuery) ([]*storage.QuestionnaireDocumentResult, error) {
	collection := r.client.Database(r.database).Collection(r.collection)

	// 构建搜索过滤器
	filter := bson.M{}

	if query.Keyword != "" {
		filter["$or"] = []bson.M{
			{"questions.title": bson.M{"$regex": query.Keyword, "$options": "i"}},
		}
	}

	// 构建查询选项
	findOptions := options.Find()
	if query.Limit > 0 {
		findOptions.SetLimit(int64(query.Limit))
	}
	if query.Skip > 0 {
		findOptions.SetSkip(int64(query.Skip))
	}

	cursor, err := collection.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to search questionnaire documents: %w", err)
	}
	defer cursor.Close(ctx)

	var results []*storage.QuestionnaireDocumentResult
	for cursor.Next(ctx) {
		var doc questionnaireDocument
		if err := cursor.Decode(&doc); err != nil {
			return nil, fmt.Errorf("failed to decode questionnaire document: %w", err)
		}
		results = append(results, r.documentToResult(&doc))
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("cursor error: %w", err)
	}

	return results, nil
}

// 辅助方法

func (r *questionnaireDocumentRepository) domainToDocument(q *questionnaire.Questionnaire) *questionnaireDocument {
	questions := make([]questionDocument, len(q.Questions()))
	for i, domainQ := range q.Questions() {
		options := make([]optionDocument, len(domainQ.Options()))
		for j, opt := range domainQ.Options() {
			options[j] = optionDocument{
				ID:    opt.ID(),
				Text:  opt.Text(),
				Value: opt.Value(),
				Order: j,
			}
		}

		questions[i] = questionDocument{
			ID:       domainQ.ID(),
			Type:     string(domainQ.Type()),
			Title:    domainQ.Title(),
			Required: domainQ.Required(),
			Options:  options,
			Settings: domainQ.Settings(),
			Order:    i,
		}
	}

	settings := settingsDocument{
		AllowAnonymous: q.Settings().AllowAnonymous(),
		ShowProgress:   q.Settings().ShowProgress(),
		RandomOrder:    q.Settings().RandomOrder(),
	}
	if timeLimit := q.Settings().TimeLimit(); timeLimit != nil {
		seconds := int64(timeLimit.Seconds())
		settings.TimeLimit = &seconds
	}

	return &questionnaireDocument{
		ID:        q.ID().Value(),
		Questions: questions,
		Settings:  settings,
		Version:   q.Version(),
		CreatedAt: q.CreatedAt(),
		UpdatedAt: q.UpdatedAt(),
	}
}

func (r *questionnaireDocumentRepository) documentToResult(doc *questionnaireDocument) *storage.QuestionnaireDocumentResult {
	questions := make([]storage.QuestionResult, len(doc.Questions))
	for i, q := range doc.Questions {
		options := make([]storage.OptionResult, len(q.Options))
		for j, opt := range q.Options {
			options[j] = storage.OptionResult{
				ID:    opt.ID,
				Text:  opt.Text,
				Value: opt.Value,
				Order: opt.Order,
			}
		}

		questions[i] = storage.QuestionResult{
			ID:       q.ID,
			Type:     q.Type,
			Title:    q.Title,
			Required: q.Required,
			Options:  options,
			Settings: q.Settings,
			Order:    q.Order,
		}
	}

	settings := storage.SettingsResult{
		AllowAnonymous: doc.Settings.AllowAnonymous,
		ShowProgress:   doc.Settings.ShowProgress,
		RandomOrder:    doc.Settings.RandomOrder,
	}
	if doc.Settings.TimeLimit != nil {
		duration := time.Duration(*doc.Settings.TimeLimit) * time.Second
		settings.TimeLimit = &duration
	}

	return &storage.QuestionnaireDocumentResult{
		ID:        doc.ID,
		Questions: questions,
		Settings:  settings,
		Version:   doc.Version,
		CreatedAt: doc.CreatedAt,
		UpdatedAt: doc.UpdatedAt,
	}
}
