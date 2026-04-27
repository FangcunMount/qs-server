package mongo_indexes

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// QuestionnairesIndexes 问卷集合索引定义
type QuestionnairesIndexes struct {
	collection *mongo.Collection
}

// NewQuestionnairesIndexes 创建问卷索引管理器
func NewQuestionnairesIndexes(collection *mongo.Collection) *QuestionnairesIndexes {
	return &QuestionnairesIndexes{collection: collection}
}

// EnsureIndexes 确保所有推荐索引已创建
func (q *QuestionnairesIndexes) EnsureIndexes(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	indexModels := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "code", Value: 1},
				{Key: "record_role", Value: 1},
				{Key: "deleted_at", Value: 1},
			},
			Options: options.Index().SetName("idx_code_record_role_deleted"),
		},
		{
			Keys: bson.D{
				{Key: "code", Value: 1},
				{Key: "version", Value: 1},
				{Key: "record_role", Value: 1},
				{Key: "deleted_at", Value: 1},
			},
			Options: options.Index().SetName("idx_code_version_record_role_deleted"),
		},
		{
			Keys: bson.D{
				{Key: "code", Value: 1},
				{Key: "is_active_published", Value: 1},
				{Key: "deleted_at", Value: 1},
			},
			Options: options.Index().SetName("idx_code_active_deleted"),
		},
		{
			Keys: bson.D{
				{Key: "status", Value: 1},
				{Key: "deleted_at", Value: 1},
				{Key: "updated_at", Value: -1},
			},
			Options: options.Index().SetName("idx_status_deleted_updated"),
		},
	}

	if _, err := q.collection.Indexes().CreateMany(ctx, indexModels); err != nil {
		return fmt.Errorf("create questionnaires indexes: %w", err)
	}

	return nil
}

// AnswerSheetsIndexes 答卷集合索引定义
type AnswerSheetsIndexes struct {
	collection *mongo.Collection
}

// NewAnswerSheetsIndexes 创建答卷索引管理器
func NewAnswerSheetsIndexes(collection *mongo.Collection) *AnswerSheetsIndexes {
	return &AnswerSheetsIndexes{collection: collection}
}

// EnsureIndexes 确保所有推荐索引已创建
func (a *AnswerSheetsIndexes) EnsureIndexes(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	indexModels := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "filler_id", Value: 1},
				{Key: "deleted_at", Value: 1},
				{Key: "filled_at", Value: -1},
			},
			Options: options.Index().SetName("idx_filler_deleted_filled"),
		},
		{
			Keys: bson.D{
				{Key: "questionnaire_code", Value: 1},
				{Key: "deleted_at", Value: 1},
				{Key: "filled_at", Value: -1},
			},
			Options: options.Index().SetName("idx_question_deleted_filled"),
		},
		{
			Keys: bson.D{
				{Key: "domain_id", Value: 1},
				{Key: "deleted_at", Value: 1},
			},
			Options: options.Index().SetName("idx_domain_deleted"),
		},
	}

	if _, err := a.collection.Indexes().CreateMany(ctx, indexModels); err != nil {
		return fmt.Errorf("create answersheets indexes: %w", err)
	}

	return nil
}

// ScalesIndexes 量表集合索引定义
type ScalesIndexes struct {
	collection *mongo.Collection
}

// NewScalesIndexes 创建量表索引管理器
func NewScalesIndexes(collection *mongo.Collection) *ScalesIndexes {
	return &ScalesIndexes{collection: collection}
}

// EnsureIndexes 确保所有推荐索引已创建
func (s *ScalesIndexes) EnsureIndexes(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	indexModels := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "code", Value: 1},
				{Key: "deleted_at", Value: 1},
			},
			Options: options.Index().SetName("idx_code_deleted"),
		},
		{
			Keys: bson.D{
				{Key: "questionnaire_code", Value: 1},
				{Key: "deleted_at", Value: 1},
			},
			Options: options.Index().SetName("idx_question_deleted"),
		},
		{
			Keys: bson.D{
				{Key: "category", Value: 1},
				{Key: "status", Value: 1},
				{Key: "deleted_at", Value: 1},
			},
			Options: options.Index().SetName("idx_category_status_deleted"),
		},
	}

	if _, err := s.collection.Indexes().CreateMany(ctx, indexModels); err != nil {
		return fmt.Errorf("create scales indexes: %w", err)
	}

	return nil
}

// InterpretReportsIndexes 解读报告集合索引定义
type InterpretReportsIndexes struct {
	collection *mongo.Collection
}

// NewInterpretReportsIndexes 创建解读报告索引管理器
func NewInterpretReportsIndexes(collection *mongo.Collection) *InterpretReportsIndexes {
	return &InterpretReportsIndexes{collection: collection}
}

// EnsureIndexes 确保所有推荐索引已创建
func (i *InterpretReportsIndexes) EnsureIndexes(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	indexModels := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "testee_id", Value: 1},
				{Key: "deleted_at", Value: 1},
				{Key: "created_at", Value: -1},
			},
			Options: options.Index().SetName("idx_testee_deleted_created"),
		},
		{
			Keys: bson.D{
				{Key: "domain_id", Value: 1},
				{Key: "deleted_at", Value: 1},
			},
			Options: options.Index().SetName("idx_domain_deleted"),
		},
	}

	if _, err := i.collection.Indexes().CreateMany(ctx, indexModels); err != nil {
		return fmt.Errorf("create interpret_reports indexes: %w", err)
	}

	return nil
}

// IndexManager 集中管理所有集合的索引创建
type IndexManager struct {
	db *mongo.Database
}

// NewIndexManager 创建索引管理器
func NewIndexManager(db *mongo.Database) *IndexManager {
	return &IndexManager{db: db}
}

// EnsureAllIndexes 确保所有集合的索引都已创建
func (m *IndexManager) EnsureAllIndexes(ctx context.Context) error {
	// 问卷集合
	if err := NewQuestionnairesIndexes(m.db.Collection("questionnaires")).EnsureIndexes(ctx); err != nil {
		return err
	}

	// 答卷集合
	if err := NewAnswerSheetsIndexes(m.db.Collection("answersheets")).EnsureIndexes(ctx); err != nil {
		return err
	}

	// 量表集合
	if err := NewScalesIndexes(m.db.Collection("scales")).EnsureIndexes(ctx); err != nil {
		return err
	}

	// 解读报告集合
	if err := NewInterpretReportsIndexes(m.db.Collection("interpret_reports")).EnsureIndexes(ctx); err != nil {
		return err
	}

	return nil
}

// MongoDB Shell 脚本版本 (用于手动创建)
const MongoDBIndexScript = `
// ========== Questionnaires 集合 ==========
db.questionnaires.createIndex(
    { code: 1, record_role: 1, deleted_at: 1 },
    { name: "idx_code_record_role_deleted" }
);

db.questionnaires.createIndex(
    { code: 1, version: 1, record_role: 1, deleted_at: 1 },
    { name: "idx_code_version_record_role_deleted" }
);

db.questionnaires.createIndex(
    { code: 1, is_active_published: 1, deleted_at: 1 },
    { name: "idx_code_active_deleted" }
);

db.questionnaires.createIndex(
    { status: 1, deleted_at: 1, updated_at: -1 },
    { name: "idx_status_deleted_updated" }
);

// ========== AnswerSheets 集合 ==========
db.answersheets.createIndex(
    { filler_id: 1, deleted_at: 1, filled_at: -1 },
    { name: "idx_filler_deleted_filled" }
);

db.answersheets.createIndex(
    { questionnaire_code: 1, deleted_at: 1, filled_at: -1 },
    { name: "idx_question_deleted_filled" }
);

db.answersheets.createIndex(
    { domain_id: 1, deleted_at: 1 },
    { name: "idx_domain_deleted" }
);

// ========== Scales 集合 ==========
db.scales.createIndex(
    { code: 1, deleted_at: 1 },
    { name: "idx_code_deleted" }
);

db.scales.createIndex(
    { questionnaire_code: 1, deleted_at: 1 },
    { name: "idx_question_deleted" }
);

db.scales.createIndex(
    { category: 1, status: 1, deleted_at: 1 },
    { name: "idx_category_status_deleted" }
);

// ========== InterpretReports 集合 ==========
db.interpret_reports.createIndex(
    { testee_id: 1, deleted_at: 1, created_at: -1 },
    { name: "idx_testee_deleted_created" }
);

db.interpret_reports.createIndex(
    { domain_id: 1, deleted_at: 1 },
    { name: "idx_domain_deleted" }
);

// ========== 验证索引 ==========
// 查看所有索引
db.questionnaires.getIndexes();
db.answersheets.getIndexes();
db.scales.getIndexes();
db.interpret_reports.getIndexes();

// 删除索引（如需要）
// db.questionnaires.dropIndex("idx_code_record_role_deleted");
`
