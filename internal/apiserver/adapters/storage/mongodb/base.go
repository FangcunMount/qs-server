package mongodb

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// BaseRepository MongoDB 基础仓储
// 提供通用的 MongoDB 操作方法，供具体业务仓储继承使用
type BaseRepository struct {
	client   *mongo.Client
	database string
}

// NewBaseRepository 创建基础仓储
func NewBaseRepository(client *mongo.Client, database string) *BaseRepository {
	return &BaseRepository{
		client:   client,
		database: database,
	}
}

// GetClient 获取 MongoDB 客户端
func (r *BaseRepository) GetClient() *mongo.Client {
	return r.client
}

// GetDatabase 获取数据库名称
func (r *BaseRepository) GetDatabase() string {
	return r.database
}

// GetCollection 获取集合
func (r *BaseRepository) GetCollection(name string) *mongo.Collection {
	return r.client.Database(r.database).Collection(name)
}

// InsertOne 插入单个文档
func (r *BaseRepository) InsertOne(ctx context.Context, collectionName string, document interface{}) (*mongo.InsertOneResult, error) {
	collection := r.GetCollection(collectionName)
	return collection.InsertOne(ctx, document)
}

// FindOne 查找单个文档
func (r *BaseRepository) FindOne(ctx context.Context, collectionName string, filter interface{}, result interface{}) error {
	collection := r.GetCollection(collectionName)
	return collection.FindOne(ctx, filter).Decode(result)
}

// UpdateOne 更新单个文档
func (r *BaseRepository) UpdateOne(ctx context.Context, collectionName string, filter interface{}, update interface{}, opts ...*options.UpdateOptions) error {
	collection := r.GetCollection(collectionName)
	_, err := collection.UpdateOne(ctx, filter, update, opts...)
	return err
}

// DeleteOne 删除单个文档
func (r *BaseRepository) DeleteOne(ctx context.Context, collectionName string, filter interface{}) error {
	collection := r.GetCollection(collectionName)
	_, err := collection.DeleteOne(ctx, filter)
	return err
}

// Find 查找多个文档
func (r *BaseRepository) Find(ctx context.Context, collectionName string, filter interface{}, opts ...*options.FindOptions) (*mongo.Cursor, error) {
	collection := r.GetCollection(collectionName)
	return collection.Find(ctx, filter, opts...)
}

// Count 统计文档数量
func (r *BaseRepository) Count(ctx context.Context, collectionName string, filter interface{}) (int64, error) {
	collection := r.GetCollection(collectionName)
	return collection.CountDocuments(ctx, filter)
}

// Aggregate 聚合查询
func (r *BaseRepository) Aggregate(ctx context.Context, collectionName string, pipeline interface{}, opts ...*options.AggregateOptions) (*mongo.Cursor, error) {
	collection := r.GetCollection(collectionName)
	return collection.Aggregate(ctx, pipeline, opts...)
}

// BulkWrite 批量写操作
func (r *BaseRepository) BulkWrite(ctx context.Context, collectionName string, models []mongo.WriteModel, opts ...*options.BulkWriteOptions) (*mongo.BulkWriteResult, error) {
	collection := r.GetCollection(collectionName)
	return collection.BulkWrite(ctx, models, opts...)
}

// CreateIndex 创建索引
func (r *BaseRepository) CreateIndex(ctx context.Context, collectionName string, model mongo.IndexModel) (string, error) {
	collection := r.GetCollection(collectionName)
	return collection.Indexes().CreateOne(ctx, model)
}

// CreateIndexes 创建多个索引
func (r *BaseRepository) CreateIndexes(ctx context.Context, collectionName string, models []mongo.IndexModel) ([]string, error) {
	collection := r.GetCollection(collectionName)
	return collection.Indexes().CreateMany(ctx, models)
}

// DropIndex 删除索引
func (r *BaseRepository) DropIndex(ctx context.Context, collectionName string, name string) error {
	collection := r.GetCollection(collectionName)
	_, err := collection.Indexes().DropOne(ctx, name)
	return err
}

// Transaction 执行事务
func (r *BaseRepository) Transaction(ctx context.Context, fn func(mongo.SessionContext) error) error {
	session, err := r.client.StartSession()
	if err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}
	defer session.EndSession(ctx)

	// 使用事务
	return mongo.WithSession(ctx, session, func(sc mongo.SessionContext) error {
		_, err := session.WithTransaction(sc, func(sessionContext mongo.SessionContext) (interface{}, error) {
			return nil, fn(sessionContext)
		})
		return err
	})
}

// PaginationQuery 分页查询构建器
type PaginationQuery struct {
	filter interface{}
	opts   *options.FindOptions
}

// NewPaginationQuery 创建分页查询构建器
func (r *BaseRepository) NewPaginationQuery() *PaginationQuery {
	return &PaginationQuery{
		filter: bson.M{},
		opts:   options.Find(),
	}
}

// WithFilter 设置查询过滤器
func (pq *PaginationQuery) WithFilter(filter interface{}) *PaginationQuery {
	pq.filter = filter
	return pq
}

// WithSort 设置排序
func (pq *PaginationQuery) WithSort(sort interface{}) *PaginationQuery {
	pq.opts.SetSort(sort)
	return pq
}

// WithLimit 设置限制数量
func (pq *PaginationQuery) WithLimit(limit int64) *PaginationQuery {
	pq.opts.SetLimit(limit)
	return pq
}

// WithSkip 设置跳过数量
func (pq *PaginationQuery) WithSkip(skip int64) *PaginationQuery {
	pq.opts.SetSkip(skip)
	return pq
}

// WithProjection 设置投影
func (pq *PaginationQuery) WithProjection(projection interface{}) *PaginationQuery {
	pq.opts.SetProjection(projection)
	return pq
}

// Execute 执行分页查询
func (pq *PaginationQuery) Execute(ctx context.Context, repo *BaseRepository, collectionName string) (*mongo.Cursor, error) {
	return repo.Find(ctx, collectionName, pq.filter, pq.opts)
}

// PaginatedResult 分页结果
type PaginatedResult struct {
	Items      []interface{} `json:"items"`
	TotalCount int64         `json:"total_count"`
	Page       int           `json:"page"`
	PageSize   int           `json:"page_size"`
	HasMore    bool          `json:"has_more"`
}

// ExecutePaginated 执行分页查询并返回结果
func (pq *PaginationQuery) ExecutePaginated(ctx context.Context, repo *BaseRepository, collectionName string, page, pageSize int, decoder func(*mongo.Cursor) ([]interface{}, error)) (*PaginatedResult, error) {
	// 计算跳过的文档数量
	skip := int64((page - 1) * pageSize)
	pq.WithSkip(skip).WithLimit(int64(pageSize))

	// 执行查询
	cursor, err := pq.Execute(ctx, repo, collectionName)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer cursor.Close(ctx)

	// 解码结果
	items, err := decoder(cursor)
	if err != nil {
		return nil, fmt.Errorf("failed to decode results: %w", err)
	}

	// 统计总数
	totalCount, err := repo.Count(ctx, collectionName, pq.filter)
	if err != nil {
		return nil, fmt.Errorf("failed to count documents: %w", err)
	}

	// 计算是否还有更多数据
	hasMore := skip+int64(len(items)) < totalCount

	return &PaginatedResult{
		Items:      items,
		TotalCount: totalCount,
		Page:       page,
		PageSize:   pageSize,
		HasMore:    hasMore,
	}, nil
}

// EnsureIndexes 确保索引存在的帮助方法
func (r *BaseRepository) EnsureIndexes(ctx context.Context, collectionName string, indexes []mongo.IndexModel) error {
	if len(indexes) == 0 {
		return nil
	}

	collection := r.GetCollection(collectionName)

	// 获取现有索引
	existingIndexes, err := collection.Indexes().List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list existing indexes: %w", err)
	}
	defer existingIndexes.Close(ctx)

	// 构建现有索引名称集合
	existingNames := make(map[string]bool)
	for existingIndexes.Next(ctx) {
		var index bson.M
		if err := existingIndexes.Decode(&index); err != nil {
			continue
		}
		if name, ok := index["name"].(string); ok {
			existingNames[name] = true
		}
	}

	// 过滤出需要创建的索引
	var newIndexes []mongo.IndexModel
	for _, index := range indexes {
		if index.Options != nil && index.Options.Name != nil {
			if !existingNames[*index.Options.Name] {
				newIndexes = append(newIndexes, index)
			}
		}
	}

	// 创建新索引
	if len(newIndexes) > 0 {
		_, err := r.CreateIndexes(ctx, collectionName, newIndexes)
		if err != nil {
			return fmt.Errorf("failed to create indexes: %w", err)
		}
	}

	return nil
}

// BuildTextSearchFilter 构建文本搜索过滤器
func (r *BaseRepository) BuildTextSearchFilter(keyword string, fields []string) bson.M {
	if keyword == "" || len(fields) == 0 {
		return bson.M{}
	}

	orConditions := make([]bson.M, len(fields))
	for i, field := range fields {
		orConditions[i] = bson.M{
			field: bson.M{
				"$regex":   keyword,
				"$options": "i", // 不区分大小写
			},
		}
	}

	return bson.M{"$or": orConditions}
}

// BuildTimeRangeFilter 构建时间范围过滤器
func (r *BaseRepository) BuildTimeRangeFilter(field string, start, end *time.Time) bson.M {
	filter := bson.M{}

	if start != nil || end != nil {
		timeFilter := bson.M{}
		if start != nil {
			timeFilter["$gte"] = *start
		}
		if end != nil {
			timeFilter["$lte"] = *end
		}
		filter[field] = timeFilter
	}

	return filter
}

// MergeFilters 合并多个过滤器
func (r *BaseRepository) MergeFilters(filters ...bson.M) bson.M {
	result := bson.M{}

	for _, filter := range filters {
		for key, value := range filter {
			if existingValue, exists := result[key]; exists {
				// 如果键已存在，需要合并条件
				if key == "$and" || key == "$or" {
					// 对于逻辑操作符，合并数组
					if existingArray, ok := existingValue.([]interface{}); ok {
						if newArray, ok := value.([]interface{}); ok {
							result[key] = append(existingArray, newArray...)
						}
					}
				} else {
					// 对于其他键，使用 $and 组合
					result = bson.M{
						"$and": []bson.M{
							{key: existingValue},
							{key: value},
						},
					}
				}
			} else {
				result[key] = value
			}
		}
	}

	return result
}
