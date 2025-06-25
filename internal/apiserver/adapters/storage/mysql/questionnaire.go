package mysql

import (
	"context"
	"fmt"
	"time"

	"github.com/vinllen/mgo"
	"gorm.io/gorm"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/ports/storage"
)

// questionnaireRepository 问卷仓储适配器
// 使用 MySQL 存储基础信息，MongoDB 存储文档结构
type questionnaireRepository struct {
	*BaseRepository
	mongo    *mgo.Session
	database string
}

// NewQuestionnaireRepository 创建问卷仓储适配器
func NewQuestionnaireRepository(mysql *gorm.DB, mongo *mgo.Session, mongoDatabase string) storage.QuestionnaireRepository {
	return &questionnaireRepository{
		BaseRepository: NewBaseRepository(mysql),
		mongo:          mongo,
		database:       mongoDatabase,
	}
}

// questionnaireModel MySQL 表模型
type questionnaireModel struct {
	ID          string    `gorm:"primaryKey" json:"id"`
	Code        string    `gorm:"uniqueIndex" json:"code"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      int       `json:"status"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Version     int       `json:"version"`
}

// TableName 表名
func (questionnaireModel) TableName() string {
	return "questionnaires"
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
func (r *questionnaireRepository) Save(ctx context.Context, q *questionnaire.Questionnaire) error {
	// 1. 保存基础信息到 MySQL
	model := &questionnaireModel{
		ID:          q.ID().Value(),
		Code:        q.Code(),
		Title:       q.Title(),
		Description: q.Description(),
		Status:      int(q.Status()),
		CreatedBy:   q.CreatedBy(),
		CreatedAt:   q.CreatedAt(),
		UpdatedAt:   q.UpdatedAt(),
		Version:     q.Version(),
	}

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
func (r *questionnaireRepository) FindByID(ctx context.Context, id questionnaire.QuestionnaireID) (*questionnaire.Questionnaire, error) {
	// 1. 从 MySQL 获取基础信息
	var model questionnaireModel
	if err := r.BaseRepository.FindByID(ctx, &model, id.Value()); err != nil {
		return nil, fmt.Errorf("failed to find questionnaire in MySQL: %w", err)
	}

	// 如果记录不存在，model 的字段会为零值
	if model.ID == "" {
		return nil, questionnaire.ErrQuestionnaireNotFound
	}

	// 2. 暂时返回一个基础的问卷对象
	// TODO: 从 MongoDB 加载完整信息
	q := questionnaire.NewQuestionnaire(model.Code, model.Title, model.Description, model.CreatedBy)
	return q, nil
}

// FindByCode 根据代码查找问卷
func (r *questionnaireRepository) FindByCode(ctx context.Context, code string) (*questionnaire.Questionnaire, error) {
	var model questionnaireModel
	if err := r.FindByField(ctx, &model, "code", code); err != nil {
		return nil, fmt.Errorf("failed to find questionnaire by code: %w", err)
	}

	// 如果记录不存在，model 的字段会为零值
	if model.ID == "" {
		return nil, questionnaire.ErrQuestionnaireNotFound
	}

	// TODO: 从 MongoDB 加载完整信息
	q := questionnaire.NewQuestionnaire(model.Code, model.Title, model.Description, model.CreatedBy)
	return q, nil
}

// Update 更新问卷
func (r *questionnaireRepository) Update(ctx context.Context, q *questionnaire.Questionnaire) error {
	model := &questionnaireModel{
		ID:          q.ID().Value(),
		Code:        q.Code(),
		Title:       q.Title(),
		Description: q.Description(),
		Status:      int(q.Status()),
		CreatedBy:   q.CreatedBy(),
		CreatedAt:   q.CreatedAt(),
		UpdatedAt:   q.UpdatedAt(),
		Version:     q.Version(),
	}

	return r.BaseRepository.Update(ctx, model)
}

// Remove 删除问卷
func (r *questionnaireRepository) Remove(ctx context.Context, id questionnaire.QuestionnaireID) error {
	// 1. 删除 MongoDB 文档（如果可用）
	if r.mongo != nil {
		session := r.mongo.Copy()
		defer session.Close()

		collection := session.DB(r.database).C("questionnaire_docs")
		_ = collection.RemoveId(id.Value())
	}

	// 2. 删除 MySQL 记录
	return r.DeleteByID(ctx, &questionnaireModel{}, id.Value())
}

// FindPublishedQuestionnaires 查找已发布的问卷
func (r *questionnaireRepository) FindPublishedQuestionnaires(ctx context.Context) ([]*questionnaire.Questionnaire, error) {
	return r.FindQuestionnairesByStatus(ctx, questionnaire.StatusPublished)
}

// FindQuestionnairesByCreator 根据创建者查找问卷
func (r *questionnaireRepository) FindQuestionnairesByCreator(ctx context.Context, creatorID string) ([]*questionnaire.Questionnaire, error) {
	var models []questionnaireModel
	conditions := map[string]interface{}{
		"created_by": creatorID,
	}

	if err := r.FindWithConditions(ctx, &models, conditions); err != nil {
		return nil, err
	}

	result := make([]*questionnaire.Questionnaire, len(models))
	for i, model := range models {
		result[i] = questionnaire.NewQuestionnaire(model.Code, model.Title, model.Description, model.CreatedBy)
	}
	return result, nil
}

// FindQuestionnairesByStatus 根据状态查找问卷
func (r *questionnaireRepository) FindQuestionnairesByStatus(ctx context.Context, status questionnaire.Status) ([]*questionnaire.Questionnaire, error) {
	var models []questionnaireModel
	conditions := map[string]interface{}{
		"status": int(status),
	}

	if err := r.FindWithConditions(ctx, &models, conditions); err != nil {
		return nil, err
	}

	result := make([]*questionnaire.Questionnaire, len(models))
	for i, model := range models {
		result[i] = questionnaire.NewQuestionnaire(model.Code, model.Title, model.Description, model.CreatedBy)
	}
	return result, nil
}

// FindQuestionnaires 分页查询问卷
func (r *questionnaireRepository) FindQuestionnaires(ctx context.Context, query storage.QueryOptions) (*storage.QuestionnaireQueryResult, error) {
	paginatedQuery := r.NewPaginatedQuery(ctx, &questionnaireModel{}).
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

	var models []questionnaireModel
	result, err := paginatedQuery.Execute(&models)
	if err != nil {
		return nil, err
	}

	// 转换为领域对象
	questionnaires := make([]*questionnaire.Questionnaire, len(models))
	for i, model := range models {
		questionnaires[i] = questionnaire.NewQuestionnaire(model.Code, model.Title, model.Description, model.CreatedBy)
	}

	return &storage.QuestionnaireQueryResult{
		Items:      questionnaires,
		TotalCount: result.TotalCount,
		HasMore:    result.HasMore,
	}, nil
}

// ExistsByCode 检查代码是否存在
func (r *questionnaireRepository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	return r.ExistsByField(ctx, &questionnaireModel{}, "code", code)
}

// ExistsByID 检查ID是否存在
func (r *questionnaireRepository) ExistsByID(ctx context.Context, id questionnaire.QuestionnaireID) (bool, error) {
	return r.ExistsByField(ctx, &questionnaireModel{}, "id", id.Value())
}

// 辅助方法
func (r *questionnaireRepository) questionsToMongo(questions []questionnaire.Question) []map[string]interface{} {
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

func (r *questionnaireRepository) optionsToMongo(options []questionnaire.Option) []map[string]interface{} {
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

func (r *questionnaireRepository) settingsToMongo(settings questionnaire.Settings) map[string]interface{} {
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
