package mongo_indexes

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

// EnsureIndexes 确保所有推荐索引已创建（含 unified head/snapshot partial unique）。
func (q *QuestionnairesIndexes) EnsureIndexes(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
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
	indexModels = append(indexModels, unifiedQuestionnaireIndexModels()...)

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
		{
			Keys: bson.D{
				{Key: "questionnaire_code", Value: 1},
				{Key: "record_role", Value: 1},
				{Key: "is_active_published", Value: 1},
				{Key: "status", Value: 1},
			},
			Options: options.Index().
				SetName("idx_scales_published_questionnaire_active").
				SetPartialFilterExpression(bson.M{"deleted_at": nil}),
		},
	}

	if _, err := s.collection.Indexes().CreateMany(ctx, indexModels); err != nil {
		return fmt.Errorf("create scales indexes: %w", err)
	}

	return nil
}

// AssessmentModelsIndexes manages unified assessment_models indexes.
type AssessmentModelsIndexes struct {
	collection *mongo.Collection
}

// NewAssessmentModelsIndexes creates an assessment_models index manager.
func NewAssessmentModelsIndexes(collection *mongo.Collection) *AssessmentModelsIndexes {
	return &AssessmentModelsIndexes{collection: collection}
}

// EnsureIndexes creates the canonical role-based partial unique indexes.
func (a *AssessmentModelsIndexes) EnsureIndexes(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if _, err := a.collection.Indexes().CreateMany(ctx, unifiedAssessmentModelIndexModels()); err != nil {
		return fmt.Errorf("create assessment_models indexes: %w", err)
	}
	return nil
}

// AssessmentNormsIndexes manages assessment_norms indexes.
type AssessmentNormsIndexes struct {
	collection *mongo.Collection
}

// NewAssessmentNormsIndexes creates an assessment_norms index manager.
func NewAssessmentNormsIndexes(collection *mongo.Collection) *AssessmentNormsIndexes {
	return &AssessmentNormsIndexes{collection: collection}
}

// EnsureIndexes creates the canonical Norm table_version unique index.
func (a *AssessmentNormsIndexes) EnsureIndexes(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if _, err := a.collection.Indexes().CreateMany(ctx, unifiedAssessmentNormIndexModels()); err != nil {
		return fmt.Errorf("create assessment_norms indexes: %w", err)
	}
	return nil
}

func unifiedAssessmentModelIndexModels() []mongo.IndexModel {
	return []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "code", Value: 1}, {Key: "record_role", Value: 1}},
			Options: options.Index().SetName("idx_assessment_models_head_code").SetUnique(true).
				SetPartialFilterExpression(bson.M{"record_role": "head", "deleted_at": nil}),
		},
		{
			Keys: bson.D{
				{Key: "kind", Value: 1}, {Key: "algorithm", Value: 1},
				{Key: "code", Value: 1}, {Key: "release_version", Value: 1}, {Key: "record_role", Value: 1},
			},
			Options: options.Index().SetName("idx_assessment_models_snapshot_identity_version_v2").SetUnique(true).
				SetPartialFilterExpression(bson.M{"record_role": "published_snapshot", "deleted_at": nil}),
		},
		{
			Keys: bson.D{{Key: "code", Value: 1}, {Key: "record_role", Value: 1}, {Key: "release_status", Value: 1}},
			Options: options.Index().SetName("idx_assessment_models_active_code").SetUnique(true).
				SetPartialFilterExpression(bson.M{"record_role": "published_snapshot", "release_status": "active", "deleted_at": nil}),
		},
		{
			Keys: bson.D{
				{Key: "questionnaire_code", Value: 1}, {Key: "questionnaire_version", Value: 1},
				{Key: "record_role", Value: 1}, {Key: "release_status", Value: 1},
			},
			Options: options.Index().SetName("idx_assessment_models_active_questionnaire").SetUnique(true).
				SetPartialFilterExpression(bson.M{"record_role": "published_snapshot", "release_status": "active", "deleted_at": nil}),
		},
		{
			Keys: bson.D{
				{Key: "record_role", Value: 1}, {Key: "release_status", Value: 1}, {Key: "status", Value: 1},
				{Key: "kind", Value: 1}, {Key: "category", Value: 1}, {Key: "algorithm", Value: 1}, {Key: "code", Value: 1},
			},
			Options: options.Index().SetName("idx_assessment_models_active_catalog"),
		},
		{
			Keys:    bson.D{{Key: "code", Value: 1}, {Key: "record_role", Value: 1}, {Key: "published_at", Value: -1}},
			Options: options.Index().SetName("idx_assessment_models_release_history"),
		},
	}
}

func unifiedQuestionnaireIndexModels() []mongo.IndexModel {
	return []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "code", Value: 1}, {Key: "record_role", Value: 1}},
			Options: options.Index().SetName("idx_questionnaires_head_code").SetUnique(true).
				SetPartialFilterExpression(bson.M{"record_role": "head", "deleted_at": nil}),
		},
		{
			Keys: bson.D{{Key: "code", Value: 1}, {Key: "version", Value: 1}, {Key: "record_role", Value: 1}},
			Options: options.Index().SetName("idx_questionnaires_snapshot_version").SetUnique(true).
				SetPartialFilterExpression(bson.M{"record_role": "published_snapshot", "deleted_at": nil}),
		},
		{
			Keys: bson.D{{Key: "code", Value: 1}, {Key: "record_role", Value: 1}, {Key: "release_status", Value: 1}},
			Options: options.Index().SetName("idx_questionnaires_active_code").SetUnique(true).
				SetPartialFilterExpression(bson.M{"record_role": "published_snapshot", "release_status": "active", "deleted_at": nil}),
		},
		{
			Keys:    bson.D{{Key: "code", Value: 1}, {Key: "record_role", Value: 1}, {Key: "published_at", Value: -1}},
			Options: options.Index().SetName("idx_questionnaires_release_history"),
		},
	}
}

func unifiedAssessmentNormIndexModels() []mongo.IndexModel {
	return []mongo.IndexModel{{
		Keys: bson.D{{Key: "table_version", Value: 1}},
		Options: options.Index().SetName("idx_assessment_norms_table_version").SetUnique(true).
			SetPartialFilterExpression(bson.M{"deleted_at": nil}),
	}}
}

// RequiredUnifiedIndexNames lists canonical indexes that must exist after migration 000013.
func RequiredUnifiedIndexNames() map[string][]string {
	return map[string][]string{
		"assessment_models": {
			"idx_assessment_models_head_code",
			"idx_assessment_models_snapshot_identity_version_v2",
			"idx_assessment_models_active_code",
			"idx_assessment_models_active_questionnaire",
			"idx_assessment_models_active_catalog",
			"idx_assessment_models_release_history",
		},
		"questionnaires": {
			"idx_questionnaires_head_code",
			"idx_questionnaires_snapshot_version",
			"idx_questionnaires_active_code",
			"idx_questionnaires_release_history",
		},
		"assessment_norms": {
			"idx_assessment_norms_table_version",
		},
	}
}

// ForbiddenLegacyIndexNames lists indexes that conflict with unified head/snapshot coexistence.
func ForbiddenLegacyIndexNames() map[string][]string {
	return map[string][]string{
		"assessment_models": {"idx_assessment_models_code"},
		"questionnaires":    {"idx_code_version"},
	}
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
	// Reconcile covers assessment_models / assessment_norms / questionnaires (incl. unified).
	if err := m.ReconcileUnifiedModelCatalogIndexes(ctx); err != nil {
		return err
	}
	if err := NewAnswerSheetsIndexes(m.db.Collection("answersheets")).EnsureIndexes(ctx); err != nil {
		return err
	}
	if err := NewScalesIndexes(m.db.Collection("scales")).EnsureIndexes(ctx); err != nil {
		return err
	}
	return nil
}

// ReconcileUnifiedModelCatalogIndexes drops conflicting legacy unique indexes
// (ignoring IndexNotFound) then ensures the canonical unified indexes exist.
// Legacy dropIndexes inside JSON migrations are intentionally avoided: already-
// cutover environments fail with IndexNotFound and leave golang-migrate dirty.
func (m *IndexManager) ReconcileUnifiedModelCatalogIndexes(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if err := m.DropForbiddenLegacyIndexes(ctx); err != nil {
		return err
	}
	if err := NewAssessmentModelsIndexes(m.db.Collection("assessment_models")).EnsureIndexes(ctx); err != nil {
		return err
	}
	if err := NewAssessmentNormsIndexes(m.db.Collection("assessment_norms")).EnsureIndexes(ctx); err != nil {
		return err
	}
	// Unified questionnaire indexes are included in QuestionnairesIndexes.EnsureIndexes.
	if err := NewQuestionnairesIndexes(m.db.Collection("questionnaires")).EnsureIndexes(ctx); err != nil {
		return err
	}
	return nil
}

// DropForbiddenLegacyIndexes removes indexes that conflict with head/snapshot coexistence.
// Missing indexes are ignored so cutover environments remain idempotent.
func (m *IndexManager) DropForbiddenLegacyIndexes(ctx context.Context) error {
	for collection, names := range ForbiddenLegacyIndexNames() {
		coll := m.db.Collection(collection)
		for _, name := range names {
			_, err := coll.Indexes().DropOne(ctx, name)
			if err == nil {
				continue
			}
			if isIndexNotFound(err) {
				continue
			}
			return fmt.Errorf("drop legacy index %s.%s: %w", collection, name, err)
		}
	}
	return nil
}

func isIndexNotFound(err error) bool {
	if err == nil {
		return false
	}
	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) {
		// MongoDB IndexNotFound = 27
		return cmdErr.Code == 27 || cmdErr.Name == "IndexNotFound"
	}
	msg := err.Error()
	return strings.Contains(msg, "IndexNotFound") || strings.Contains(msg, "index not found")
}

// VerifyUnifiedModelCatalogIndexes checks that required unified indexes exist and
// conflicting legacy unique indexes are absent. Missing required indexes fail closed.
func (m *IndexManager) VerifyUnifiedModelCatalogIndexes(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	for collection, names := range RequiredUnifiedIndexNames() {
		present, err := listIndexNames(ctx, m.db.Collection(collection))
		if err != nil {
			return fmt.Errorf("list indexes for %s: %w", collection, err)
		}
		for _, name := range names {
			if !present[name] {
				return fmt.Errorf("required unified index %s.%s is missing; run Mongo migration 000013 / ReconcileUnifiedModelCatalogIndexes", collection, name)
			}
		}
	}
	for collection, names := range ForbiddenLegacyIndexNames() {
		present, err := listIndexNames(ctx, m.db.Collection(collection))
		if err != nil {
			return fmt.Errorf("list indexes for %s: %w", collection, err)
		}
		for _, name := range names {
			if present[name] {
				return fmt.Errorf("legacy conflicting index %s.%s still exists; drop it before serving write traffic", collection, name)
			}
		}
	}
	return nil
}

func listIndexNames(ctx context.Context, collection *mongo.Collection) (map[string]bool, error) {
	cursor, err := collection.Indexes().List(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()
	out := make(map[string]bool)
	for cursor.Next(ctx) {
		var item struct {
			Name string `bson:"name"`
		}
		if err := cursor.Decode(&item); err != nil {
			return nil, err
		}
		out[item.Name] = true
	}
	return out, cursor.Err()
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

db.questionnaires.createIndex(
    { code: 1, record_role: 1 },
    { name: "idx_questionnaires_head_code", unique: true, partialFilterExpression: { record_role: "head", deleted_at: null } }
);

db.questionnaires.createIndex(
    { code: 1, version: 1, record_role: 1 },
    { name: "idx_questionnaires_snapshot_version", unique: true, partialFilterExpression: { record_role: "published_snapshot", deleted_at: null } }
);

db.questionnaires.createIndex(
    { code: 1, record_role: 1, release_status: 1 },
    { name: "idx_questionnaires_active_code", unique: true, partialFilterExpression: { record_role: "published_snapshot", release_status: "active", deleted_at: null } }
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

db.scales.createIndex(
    { questionnaire_code: 1, record_role: 1, is_active_published: 1, status: 1 },
    {
        name: "idx_scales_published_questionnaire_active",
        partialFilterExpression: { deleted_at: null }
    }
);

// ========== Assessment Models / Norms（unified schema，与 migration 000013 同源）==========
db.assessment_models.createIndex(
    { code: 1, record_role: 1 },
    { name: "idx_assessment_models_head_code", unique: true, partialFilterExpression: { record_role: "head", deleted_at: null } }
);
db.assessment_norms.createIndex(
    { table_version: 1 },
    { name: "idx_assessment_norms_table_version", unique: true, partialFilterExpression: { deleted_at: null } }
);

// ========== 验证索引 ==========
db.questionnaires.getIndexes();
db.answersheets.getIndexes();
db.scales.getIndexes();
db.assessment_models.getIndexes();
db.assessment_norms.getIndexes();
`
