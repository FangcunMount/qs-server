package questionnaire

import (
	"context"
	"fmt"
	"time"

	"github.com/vinllen/mgo"
	"gorm.io/gorm"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/adapters/storage/mysql"
	questionnaireDomain "github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/ports/storage"
)

// Repository 问卷仓储适配器
// 使用 MySQL 存储基础信息，MongoDB 存储文档结构
type Repository struct {
	*mysql.BaseRepository
	mongo     *mgo.Session
	database  string
	converter *Converter
}

// NewRepository 创建问卷仓储适配器
func NewRepository(mysqlDB *gorm.DB, mongo *mgo.Session, mongoDatabase string) storage.QuestionnaireRepository {
	return &Repository{
		BaseRepository: mysql.NewBaseRepository(mysqlDB),
		mongo:          mongo,
		database:       mongoDatabase,
		converter:      NewConverter(),
	}
}

// questionnaireDocument MongoDB 文档模型
type questionnaireDocument struct {
	ID        string                 `bson:"_id"`
	Questions []questionDocument     `bson:"questions"`
	Settings  map[string]interface{} `bson:"settings"`
	Version   int                    `bson:"version"`
	UpdatedAt time.Time              `bson:"updated_at"`
}

// questionDocument 问题文档
type questionDocument struct {
	ID       string                 `bson:"id"`
	Type     string                 `bson:"type"`
	Title    string                 `bson:"title"`
	Required bool                   `bson:"required"`
	Options  []optionDocument       `bson:"options"`
	Settings map[string]interface{} `bson:"settings"`
}

// optionDocument 选项文档
type optionDocument struct {
	ID    string `bson:"id"`
	Text  string `bson:"text"`
	Value string `bson:"value"`
}

// Save 保存问卷
func (r *Repository) Save(ctx context.Context, q *questionnaireDomain.Questionnaire) error {
	// 1. 保存基础信息到 MySQL
	model := r.converter.DomainToModel(q)

	if err := r.Create(ctx, model); err != nil {
		return fmt.Errorf("failed to save questionnaire to MySQL: %w", err)
	}

	// 2. 保存文档结构到 MongoDB（如果可用）
	if r.mongo != nil {
		doc := map[string]interface{}{
			"_id":        q.ID().Value(),
			"questions":  r.questionsToMongo(q.Questions()),
			"settings":   r.settingsToMongo(q.Settings()),
			"version":    q.Version(),
			"updated_at": q.UpdatedAt(),
		}

		session := r.mongo.Copy()
		defer session.Close()

		collection := session.DB(r.database).C("questionnaire_docs")
		if err := collection.Insert(doc); err != nil {
			// 如果 MongoDB 失败，回滚 MySQL
			_ = r.Delete(ctx, model)
			return fmt.Errorf("failed to save questionnaire to MongoDB: %w", err)
		}
	}

	return nil
}

// FindByID 根据ID查找问卷
func (r *Repository) FindByID(ctx context.Context, id questionnaireDomain.QuestionnaireID) (*questionnaireDomain.Questionnaire, error) {
	// 1. 从 MySQL 获取基础信息
	var model Model
	if err := r.BaseRepository.FindByID(ctx, &model, id.Value()); err != nil {
		return nil, fmt.Errorf("failed to find questionnaire in MySQL: %w", err)
	}

	// 如果记录不存在，model 的字段会为零值
	if model.ID == "" {
		return nil, questionnaireDomain.ErrQuestionnaireNotFound
	}

	// 2. 暂时返回一个基础的问卷对象
	// TODO: 从 MongoDB 加载完整信息
	return r.converter.ModelToDomain(&model), nil
}

// FindByCode 根据代码查找问卷
func (r *Repository) FindByCode(ctx context.Context, code string) (*questionnaireDomain.Questionnaire, error) {
	var model Model
	if err := r.FindByField(ctx, &model, "code", code); err != nil {
		return nil, fmt.Errorf("failed to find questionnaire by code: %w", err)
	}

	// 如果记录不存在，model 的字段会为零值
	if model.ID == "" {
		return nil, questionnaireDomain.ErrQuestionnaireNotFound
	}

	// TODO: 从 MongoDB 加载完整信息
	return r.converter.ModelToDomain(&model), nil
}

// Update 更新问卷
func (r *Repository) Update(ctx context.Context, q *questionnaireDomain.Questionnaire) error {
	model := r.converter.DomainToModel(q)
	return r.BaseRepository.Update(ctx, model)
}

// Remove 删除问卷
func (r *Repository) Remove(ctx context.Context, id questionnaireDomain.QuestionnaireID) error {
	// 1. 删除 MongoDB 文档（如果可用）
	if r.mongo != nil {
		session := r.mongo.Copy()
		defer session.Close()

		collection := session.DB(r.database).C("questionnaire_docs")
		_ = collection.RemoveId(id.Value())
	}

	// 2. 删除 MySQL 记录
	return r.DeleteByID(ctx, &Model{}, id.Value())
}

// FindPublishedQuestionnaires 查找已发布的问卷
func (r *Repository) FindPublishedQuestionnaires(ctx context.Context) ([]*questionnaireDomain.Questionnaire, error) {
	return r.FindQuestionnairesByStatus(ctx, questionnaireDomain.StatusPublished)
}

// FindQuestionnairesByCreator 根据创建者查找问卷
func (r *Repository) FindQuestionnairesByCreator(ctx context.Context, creatorID string) ([]*questionnaireDomain.Questionnaire, error) {
	var models []Model
	conditions := map[string]interface{}{
		"created_by": creatorID,
	}

	if err := r.FindWithConditions(ctx, &models, conditions); err != nil {
		return nil, err
	}

	result := make([]*questionnaireDomain.Questionnaire, len(models))
	for i, model := range models {
		result[i] = r.converter.ModelToDomain(&model)
	}
	return result, nil
}

// FindQuestionnairesByStatus 根据状态查找问卷
func (r *Repository) FindQuestionnairesByStatus(ctx context.Context, status questionnaireDomain.Status) ([]*questionnaireDomain.Questionnaire, error) {
	var models []Model
	conditions := map[string]interface{}{
		"status": int(status),
	}

	if err := r.FindWithConditions(ctx, &models, conditions); err != nil {
		return nil, err
	}

	result := make([]*questionnaireDomain.Questionnaire, len(models))
	for i, model := range models {
		result[i] = r.converter.ModelToDomain(&model)
	}
	return result, nil
}

// FindQuestionnaires 分页查询问卷
func (r *Repository) FindQuestionnaires(ctx context.Context, query storage.QueryOptions) (*storage.QuestionnaireQueryResult, error) {
	paginatedQuery := r.NewPaginatedQuery(ctx, &Model{}).
		Offset(query.Offset).
		Limit(query.Limit)

	// 应用过滤条件
	if query.CreatorID != nil {
		paginatedQuery = paginatedQuery.Where("created_by = ?", *query.CreatorID)
	}
	if query.Status != nil {
		paginatedQuery = paginatedQuery.Where("status = ?", int(*query.Status))
	}
	if query.Keyword != nil {
		paginatedQuery = paginatedQuery.Search(*query.Keyword, "title", "description")
	}

	// 应用排序
	if query.SortBy != "" {
		order := query.SortBy
		if query.SortOrder == "desc" {
			order += " DESC"
		}
		paginatedQuery = paginatedQuery.OrderBy(order)
	}

	var models []Model
	result, err := paginatedQuery.Execute(&models)
	if err != nil {
		return nil, err
	}

	// 转换为领域对象
	questionnaires := make([]*questionnaireDomain.Questionnaire, len(models))
	for i, model := range models {
		questionnaires[i] = r.converter.ModelToDomain(&model)
	}

	return &storage.QuestionnaireQueryResult{
		Items:      questionnaires,
		TotalCount: result.TotalCount,
		HasMore:    result.HasMore,
	}, nil
}

// ExistsByCode 检查代码是否存在
func (r *Repository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	return r.ExistsByField(ctx, &Model{}, "code", code)
}

// ExistsByID 检查ID是否存在
func (r *Repository) ExistsByID(ctx context.Context, id questionnaireDomain.QuestionnaireID) (bool, error) {
	return r.ExistsByField(ctx, &Model{}, "id", id.Value())
}

// 辅助方法 - MongoDB 转换
func (r *Repository) questionsToMongo(questions []questionnaireDomain.Question) []map[string]interface{} {
	result := make([]map[string]interface{}, len(questions))
	for i, q := range questions {
		result[i] = map[string]interface{}{
			"id":       q.ID(),
			"type":     string(q.Type()),
			"title":    q.Title(),
			"required": q.Required(),
			"options":  r.optionsToMongo(q.Options()),
			"settings": q.Settings(),
		}
	}
	return result
}

func (r *Repository) optionsToMongo(options []questionnaireDomain.Option) []map[string]interface{} {
	result := make([]map[string]interface{}, len(options))
	for i, opt := range options {
		result[i] = map[string]interface{}{
			"id":    opt.ID(),
			"text":  opt.Text(),
			"value": opt.Value(),
		}
	}
	return result
}

func (r *Repository) settingsToMongo(settings questionnaireDomain.Settings) map[string]interface{} {
	result := map[string]interface{}{
		"allowAnonymous": settings.AllowAnonymous(),
		"showProgress":   settings.ShowProgress(),
		"randomOrder":    settings.RandomOrder(),
	}
	if timeLimit := settings.TimeLimit(); timeLimit != nil {
		result["timeLimit"] = timeLimit.Seconds()
	}
	return result
}
