package interpretreport

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	interpretreport "github.com/fangcun-mount/qs-server/internal/apiserver/domain/interpret-report"
	interpretport "github.com/fangcun-mount/qs-server/internal/apiserver/domain/interpret-report/port"
	base "github.com/fangcun-mount/qs-server/internal/apiserver/infrastructure/mongo"
	"github.com/fangcun-mount/qs-server/pkg/log"
	v1 "github.com/fangcun-mount/qs-server/pkg/meta/v1"
)

// Repository 解读报告MongoDB仓储
type Repository struct {
	base.BaseRepository
	mapper *Mapper
}

// NewRepository 创建解读报告仓储
func NewRepository(db *mongo.Database) *Repository {
	return &Repository{
		BaseRepository: base.NewBaseRepository(db, (&InterpretReportPO{}).CollectionName()),
		mapper:         NewMapper(),
	}
}

// 确保实现了接口
var _ interpretport.InterpretReportRepositoryMongo = (*Repository)(nil)

// Create 创建解读报告
func (r *Repository) Create(ctx context.Context, report *interpretreport.InterpretReport) error {
	log.Infof("开始创建解读报告，领域对象ID: %d", report.GetID().Value())

	// 转换为持久化对象
	po, err := r.mapper.ToPO(report)
	if err != nil {
		log.Errorf("转换领域对象为持久化对象失败: %v", err)
		return fmt.Errorf("转换领域对象为持久化对象失败: %v", err)
	}

	log.Infof("持久化对象转换成功，DomainID: %d", po.DomainID)

	// 设置创建时间等字段
	po.BeforeInsert()

	log.Infof("BeforeInsert完成，DomainID: %d", po.DomainID)

	// 插入数据库
	result, err := r.InsertOne(ctx, po)
	if err != nil {
		log.Errorf("插入解读报告到MongoDB失败: %v", err)
		return fmt.Errorf("插入解读报告失败: %v", err)
	}

	log.Infof("MongoDB插入成功，ObjectID: %v", result.InsertedID)

	// 更新领域对象的ID
	report.SetID(v1.NewID(po.DomainID))

	log.Infof("领域对象ID更新完成，新ID: %d", report.GetID().Value())

	return nil
}

// FindByAnswerSheetId 根据答卷ID查找解读报告
func (r *Repository) FindByAnswerSheetId(ctx context.Context, answerSheetId uint64) (*interpretreport.InterpretReport, error) {
	filter := bson.M{
		"answer_sheet_id": answerSheetId,
		"deleted_at":      bson.M{"$exists": false},
	}

	var po InterpretReportPO
	err := r.FindOne(ctx, filter, &po)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("解读报告不存在")
		}
		return nil, fmt.Errorf("查找解读报告失败: %v", err)
	}

	// 转换为领域对象
	entity, err := r.mapper.ToEntity(&po)
	if err != nil {
		return nil, fmt.Errorf("转换持久化对象为领域对象失败: %v", err)
	}

	return entity, nil
}

// FindList 根据条件查找解读报告列表
func (r *Repository) FindList(ctx context.Context, page, pageSize int, conditions map[string]string) ([]*interpretreport.InterpretReport, error) {
	// 构建查询条件
	filter := bson.M{
		"deleted_at": bson.M{"$exists": false},
	}

	// 添加条件过滤
	for key, value := range conditions {
		if value != "" {
			switch key {
			case "medical_scale_code":
				filter["medical_scale_code"] = value
			case "title":
				filter["title"] = bson.M{"$regex": value, "$options": "i"}
			case "answer_sheet_id":
				if id, err := strconv.ParseUint(value, 10, 64); err == nil {
					filter["answer_sheet_id"] = id
				}
			case "created_after":
				if t, err := time.Parse("2006-01-02", value); err == nil {
					filter["created_at"] = bson.M{"$gte": t}
				}
			case "created_before":
				if t, err := time.Parse("2006-01-02", value); err == nil {
					if existing, ok := filter["created_at"].(bson.M); ok {
						existing["$lte"] = t.Add(24 * time.Hour)
					} else {
						filter["created_at"] = bson.M{"$lte": t.Add(24 * time.Hour)}
					}
				}
			}
		}
	}

	// 设置分页选项
	findOptions := options.Find()
	findOptions.SetSkip(int64((page - 1) * pageSize))
	findOptions.SetLimit(int64(pageSize))
	findOptions.SetSort(bson.M{"created_at": -1}) // 按创建时间倒序

	// 查询数据
	cursor, err := r.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, fmt.Errorf("查询解读报告列表失败: %v", err)
	}
	defer cursor.Close(ctx)

	// 解析结果
	var pos []*InterpretReportPO
	for cursor.Next(ctx) {
		var po InterpretReportPO
		if err := cursor.Decode(&po); err != nil {
			return nil, fmt.Errorf("解析解读报告数据失败: %v", err)
		}
		pos = append(pos, &po)
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("遍历解读报告数据失败: %v", err)
	}

	// 转换为领域对象
	entities, err := r.mapper.ToEntityList(pos)
	if err != nil {
		return nil, fmt.Errorf("转换持久化对象列表为领域对象列表失败: %v", err)
	}

	return entities, nil
}

// CountWithConditions 根据条件计算解读报告数量
func (r *Repository) CountWithConditions(ctx context.Context, conditions map[string]string) (int64, error) {
	// 构建查询条件
	filter := bson.M{
		"deleted_at": bson.M{"$exists": false},
	}

	// 添加条件过滤
	for key, value := range conditions {
		if value != "" {
			switch key {
			case "medical_scale_code":
				filter["medical_scale_code"] = value
			case "title":
				filter["title"] = bson.M{"$regex": value, "$options": "i"}
			case "answer_sheet_id":
				if id, err := strconv.ParseUint(value, 10, 64); err == nil {
					filter["answer_sheet_id"] = id
				}
			case "created_after":
				if t, err := time.Parse("2006-01-02", value); err == nil {
					filter["created_at"] = bson.M{"$gte": t}
				}
			case "created_before":
				if t, err := time.Parse("2006-01-02", value); err == nil {
					if existing, ok := filter["created_at"].(bson.M); ok {
						existing["$lte"] = t.Add(24 * time.Hour)
					} else {
						filter["created_at"] = bson.M{"$lte": t.Add(24 * time.Hour)}
					}
				}
			}
		}
	}

	// 统计数量
	count, err := r.CountDocuments(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("统计解读报告数量失败: %v", err)
	}

	return count, nil
}

// Update 更新解读报告
func (r *Repository) Update(ctx context.Context, report *interpretreport.InterpretReport) error {
	// 转换为持久化对象
	po, err := r.mapper.ToPO(report)
	if err != nil {
		return fmt.Errorf("转换领域对象为持久化对象失败: %v", err)
	}

	// 设置更新时间等字段
	po.BeforeUpdate()

	// 构建更新条件
	filter := bson.M{
		"domain_id":  report.GetID().Value(),
		"deleted_at": bson.M{"$exists": false},
	}

	// 更新数据库
	result, err := r.UpdateOne(ctx, filter, bson.M{"$set": po})
	if err != nil {
		return fmt.Errorf("更新解读报告失败: %v", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("解读报告不存在")
	}

	return nil
}

// ExistsByAnswerSheetId 检查指定答卷ID的解读报告是否存在
func (r *Repository) ExistsByAnswerSheetId(ctx context.Context, answerSheetId uint64) (bool, error) {
	filter := bson.M{
		"answer_sheet_id": answerSheetId,
		"deleted_at":      bson.M{"$exists": false},
	}

	count, err := r.CountDocuments(ctx, filter)
	if err != nil {
		return false, fmt.Errorf("检查解读报告是否存在失败: %v", err)
	}

	return count > 0, nil
}
