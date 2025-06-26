package questionnaire

import (
	"context"
	"time"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/questionnaire"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/ports/shared/interfaces"
	"github.com/yshujie/questionnaire-scale/internal/apiserver/ports/shared/patterns"
)

// CommandRepository 问卷命令仓储端口（写操作）
// 专门处理写入操作，支持事务和事件发布
type CommandRepository interface {
	patterns.AggregateRepository[*questionnaire.Questionnaire, questionnaire.QuestionnaireID]

	// SaveQuestionnaire 保存问卷
	SaveQuestionnaire(ctx context.Context, q *questionnaire.Questionnaire) error
	// UpdateQuestionnaire 更新问卷
	UpdateQuestionnaire(ctx context.Context, q *questionnaire.Questionnaire) error
	// RemoveQuestionnaire 删除问卷
	RemoveQuestionnaire(ctx context.Context, id questionnaire.QuestionnaireID) error

	// 业务特定操作
	// PublishQuestionnaire 发布问卷（状态变更）
	PublishQuestionnaire(ctx context.Context, id questionnaire.QuestionnaireID) error
	// ArchiveQuestionnaire 归档问卷
	ArchiveQuestionnaire(ctx context.Context, id questionnaire.QuestionnaireID) error

	// 批量操作
	// BulkSaveQuestionnaires 批量保存问卷
	BulkSaveQuestionnaires(ctx context.Context, questionnaires []*questionnaire.Questionnaire) error
	// BulkUpdateStatus 批量更新状态
	BulkUpdateStatus(ctx context.Context, ids []questionnaire.QuestionnaireID, status questionnaire.Status) error
}

// QueryRepository 问卷查询仓储端口（读操作）
// 专门处理查询操作，支持复杂查询和分页
type QueryRepository interface {
	patterns.ReadOnlyRepository[*questionnaire.Questionnaire, *QueryOptions]

	// FindByID 根据ID查找问卷
	FindByID(ctx context.Context, id questionnaire.QuestionnaireID) (*questionnaire.Questionnaire, error)
	// FindByCode 根据代码查找问卷
	FindByCode(ctx context.Context, code string) (*questionnaire.Questionnaire, error)

	// 业务查询
	// FindPublishedQuestionnaires 查找已发布的问卷
	FindPublishedQuestionnaires(ctx context.Context, options *QueryOptions) (*QueryResult, error)
	// FindQuestionnairesByCreator 查找指定创建者的问卷
	FindQuestionnairesByCreator(ctx context.Context, creatorID string, options *QueryOptions) (*QueryResult, error)
	// FindQuestionnairesByStatus 查找指定状态的问卷
	FindQuestionnairesByStatus(ctx context.Context, status questionnaire.Status, options *QueryOptions) (*QueryResult, error)

	// 复杂查询
	// FindQuestionnaires 通用查询方法
	FindQuestionnaires(ctx context.Context, options *QueryOptions) (*QueryResult, error)
	// SearchQuestionnaires 搜索问卷
	SearchQuestionnaires(ctx context.Context, criteria *SearchCriteria) (*QueryResult, error)

	// 统计查询
	// CountQuestionnaires 统计问卷数量
	CountQuestionnaires(ctx context.Context, options *QueryOptions) (int64, error)
	// GetQuestionnaireStatistics 获取问卷统计信息
	GetQuestionnaireStatistics(ctx context.Context, criteria *StatisticsCriteria) (*StatisticsResult, error)

	// 存在性检查
	// ExistsByCode 检查代码是否存在
	ExistsByCode(ctx context.Context, code string) (bool, error)
	// ExistsByID 检查ID是否存在
	ExistsByID(ctx context.Context, id questionnaire.QuestionnaireID) (bool, error)
}

// Repository 问卷完整仓储接口（组合命令和查询）
type Repository interface {
	CommandRepository
	QueryRepository

	// 事务支持
	interfaces.TransactionManager
}

// DocumentRepository 问卷文档仓储端口
// 专门处理问卷的文档结构存储（问题列表、设置等复杂数据）
type DocumentRepository interface {
	interfaces.Port

	// 基本文档操作
	SaveDocument(ctx context.Context, q *questionnaire.Questionnaire) error
	GetDocument(ctx context.Context, id questionnaire.QuestionnaireID) (*DocumentResult, error)
	UpdateDocument(ctx context.Context, q *questionnaire.Questionnaire) error
	RemoveDocument(ctx context.Context, id questionnaire.QuestionnaireID) error

	// 批量操作
	FindDocumentsByIDs(ctx context.Context, ids []questionnaire.QuestionnaireID) (map[string]*DocumentResult, error)
	BulkSaveDocuments(ctx context.Context, questionnaires []*questionnaire.Questionnaire) error

	// 搜索功能
	SearchDocuments(ctx context.Context, query *DocumentSearchQuery) ([]*DocumentResult, error)

	// 版本管理
	GetDocumentVersion(ctx context.Context, id questionnaire.QuestionnaireID) (int, error)
	GetDocumentHistory(ctx context.Context, id questionnaire.QuestionnaireID, limit int) ([]*DocumentResult, error)
}

// CacheRepository 问卷缓存仓储端口
type CacheRepository interface {
	patterns.CacheableRepository[*questionnaire.Questionnaire, questionnaire.QuestionnaireID]

	// 缓存预热
	WarmUpPopularQuestionnaires(ctx context.Context) error
	// 缓存失效
	InvalidateQuestionnaireCache(ctx context.Context, id questionnaire.QuestionnaireID) error
	InvalidateCreatorCache(ctx context.Context, creatorID string) error

	// 缓存统计
	GetCacheStatistics(ctx context.Context) (*CacheStatistics, error)
}

// QueryOptions 查询选项
type QueryOptions struct {
	// 分页
	Offset int
	Limit  int

	// 排序
	SortBy    string
	SortOrder string // "asc" | "desc"

	// 过滤条件
	CreatorID *string
	Status    *questionnaire.Status
	Keyword   *string

	// 时间范围
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	UpdatedAfter  *time.Time
	UpdatedBefore *time.Time

	// 包含关系
	IncludeQuestions bool
	IncludeSettings  bool
	IncludeMetadata  bool
}

// SetDefaults 设置默认值
func (opts *QueryOptions) SetDefaults() {
	if opts.Limit <= 0 {
		opts.Limit = 20
	}
	if opts.Limit > 100 {
		opts.Limit = 100
	}
	if opts.SortBy == "" {
		opts.SortBy = "updated_at"
	}
	if opts.SortOrder == "" {
		opts.SortOrder = "desc"
	}
}

// QueryResult 查询结果
type QueryResult struct {
	Items      []*questionnaire.Questionnaire
	TotalCount int64
	HasMore    bool
	Page       int
	PageSize   int

	// 元数据
	QueryTime   time.Duration
	FromCache   bool
	CacheExpiry *time.Time
}

// SearchCriteria 搜索条件
type SearchCriteria struct {
	QueryOptions

	// 高级搜索
	Title       *string
	Description *string
	Tags        []string

	// 复杂条件
	MinQuestions   *int
	MaxQuestions   *int
	HasTimeLimit   *bool
	AllowAnonymous *bool

	// 全文搜索
	FullTextQuery *string
	SearchFields  []string // 指定搜索哪些字段
}

// StatisticsCriteria 统计条件
type StatisticsCriteria struct {
	// 时间范围
	StartDate *time.Time
	EndDate   *time.Time

	// 分组维度
	GroupBy []string // "status", "creator", "date", "tag"

	// 聚合类型
	Aggregations []string // "count", "avg_questions", "completion_rate"
}

// StatisticsResult 统计结果
type StatisticsResult struct {
	TotalCount     int64
	ActiveCount    int64
	PublishedCount int64
	ArchivedCount  int64

	// 按状态分组
	StatusCounts map[questionnaire.Status]int64

	// 按创建者分组
	CreatorCounts map[string]int64

	// 时间序列数据
	DailyStats   []DailyStat
	WeeklyStats  []WeeklyStat
	MonthlyStats []MonthlyStat

	// 平均指标
	AvgQuestionsPerQuestionnaire float64
	AvgResponseTime              time.Duration

	// 热门数据
	PopularTags     []TagStat
	TopCreators     []CreatorStat
	MostViewedItems []ViewStat
}

// DailyStat 日统计
type DailyStat struct {
	Date  time.Time
	Count int64
}

// WeeklyStat 周统计
type WeeklyStat struct {
	Week  string // "2024-W01"
	Count int64
}

// MonthlyStat 月统计
type MonthlyStat struct {
	Month string // "2024-01"
	Count int64
}

// TagStat 标签统计
type TagStat struct {
	Tag   string
	Count int64
}

// CreatorStat 创建者统计
type CreatorStat struct {
	CreatorID  string
	Count      int64
	LastActive time.Time
}

// ViewStat 浏览统计
type ViewStat struct {
	QuestionnaireID string
	Title           string
	ViewCount       int64
	LastViewed      time.Time
}

// DocumentResult 文档查询结果
type DocumentResult struct {
	ID        string
	Questions []QuestionResult
	Settings  SettingsResult
	Version   int
	CreatedAt time.Time
	UpdatedAt time.Time

	// 扩展信息
	Metadata map[string]interface{}
}

// QuestionResult 问题查询结果
type QuestionResult struct {
	ID       string
	Type     string
	Title    string
	Required bool
	Options  []OptionResult
	Settings map[string]interface{}
	Order    int
}

// OptionResult 选项查询结果
type OptionResult struct {
	ID    string
	Text  string
	Value string
	Order int
}

// SettingsResult 设置查询结果
type SettingsResult struct {
	AllowAnonymous bool
	ShowProgress   bool
	RandomOrder    bool
	TimeLimit      *time.Duration
}

// DocumentSearchQuery 文档搜索查询
type DocumentSearchQuery struct {
	Keyword string
	Limit   int
	Skip    int

	// 高级过滤
	QuestionTypes []string
	MinQuestions  *int
	MaxQuestions  *int

	// 版本过滤
	MinVersion *int
	MaxVersion *int

	// 时间过滤
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	UpdatedAfter  *time.Time
	UpdatedBefore *time.Time
}

// CacheStatistics 缓存统计
type CacheStatistics struct {
	HitRate    float64
	MissRate   float64
	HitCount   int64
	MissCount  int64
	KeyCount   int64
	MemoryUsed int64
	LastReset  time.Time
}

// RepositorySpec 问卷仓储规约接口
type RepositorySpec interface {
	patterns.RepositorySpec[*questionnaire.Questionnaire]

	// 业务特定规约
	IsSatisfiedBy(q *questionnaire.Questionnaire) bool

	// 查询条件转换
	ToQueryOptions() *QueryOptions
	ToSearchCriteria() *SearchCriteria
}

// PublishedQuestionnaireSpec 已发布问卷规约
type PublishedQuestionnaireSpec struct{}

// IsSatisfiedBy 实现规约接口
func (spec *PublishedQuestionnaireSpec) IsSatisfiedBy(q *questionnaire.Questionnaire) bool {
	return q.Status() == questionnaire.StatusPublished
}

// ToSQL 实现规约接口
func (spec *PublishedQuestionnaireSpec) ToSQL() (string, []interface{}, error) {
	return "status = ?", []interface{}{questionnaire.StatusPublished}, nil
}

// ToMongo 实现规约接口
func (spec *PublishedQuestionnaireSpec) ToMongo() (interface{}, error) {
	return map[string]interface{}{
		"status": questionnaire.StatusPublished,
	}, nil
}

// ToQueryOptions 转换为查询选项
func (spec *PublishedQuestionnaireSpec) ToQueryOptions() *QueryOptions {
	status := questionnaire.StatusPublished
	return &QueryOptions{
		Status: &status,
	}
}

// ToSearchCriteria 转换为搜索条件
func (spec *PublishedQuestionnaireSpec) ToSearchCriteria() *SearchCriteria {
	status := questionnaire.StatusPublished
	return &SearchCriteria{
		QueryOptions: QueryOptions{
			Status: &status,
		},
	}
}
