package evaluation

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
)

// ReportRepository 报告 MongoDB 仓储
type ReportRepository struct {
	base.BaseRepository
	mapper *ReportMapper
}

// NewReportRepository 创建报告仓储
func NewReportRepository(db *mongo.Database) *ReportRepository {
	return &ReportRepository{
		BaseRepository: base.NewBaseRepository(db, (&InterpretReportPO{}).CollectionName()),
		mapper:         NewReportMapper(),
	}
}

// 确保实现了接口
var _ report.ReportRepository = (*ReportRepository)(nil)

// ==================== 基础操作 ====================

// Save 保存报告
func (r *ReportRepository) Save(ctx context.Context, rpt *report.InterpretReport) error {
	// 检查是否存在
	exists, err := r.ExistsByID(ctx, rpt.ID())
	if err != nil {
		return fmt.Errorf("检查报告是否存在失败: %w", err)
	}

	if exists {
		return r.Update(ctx, rpt)
	}

	// 创建新报告
	// 注意：这里需要 testeeID，但 InterpretReport 不直接包含
	// 需要从外部传入或通过其他方式获取
	// 暂时使用 0，后续需要优化
	po := r.mapper.ToPO(rpt, 0)
	base.ApplyAuditCreate(ctx, po)
	po.BeforeInsert()

	// 确保 DomainID 与报告 ID 一致
	po.DomainID = rpt.ID()

	_, err = r.InsertOne(ctx, po)
	if err != nil {
		return fmt.Errorf("插入报告失败: %w", err)
	}

	return nil
}

// SaveWithTestee 带受试者信息保存报告
func (r *ReportRepository) SaveWithTestee(ctx context.Context, rpt *report.InterpretReport, testeeID testee.ID) error {
	// 检查是否存在
	exists, err := r.ExistsByID(ctx, rpt.ID())
	if err != nil {
		return fmt.Errorf("检查报告是否存在失败: %w", err)
	}

	if exists {
		return r.Update(ctx, rpt)
	}

	// 创建新报告
	po := r.mapper.ToPO(rpt, uint64(testeeID))
	base.ApplyAuditCreate(ctx, po)
	po.BeforeInsert()

	// 确保 DomainID 与报告 ID 一致
	po.DomainID = rpt.ID()

	_, err = r.InsertOne(ctx, po)
	if err != nil {
		return fmt.Errorf("插入报告失败: %w", err)
	}

	return nil
}

// FindByID 根据ID查找报告
func (r *ReportRepository) FindByID(ctx context.Context, id report.ID) (*report.InterpretReport, error) {
	filter := bson.M{
		"domain_id":  id.Uint64(),
		"deleted_at": nil,
	}

	var po InterpretReportPO
	err := r.FindOne(ctx, filter, &po)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, report.ErrReportNotFound
		}
		return nil, fmt.Errorf("查找报告失败: %w", err)
	}

	return r.mapper.ToDomain(&po), nil
}

// FindByAssessmentID 根据测评ID查找报告
func (r *ReportRepository) FindByAssessmentID(ctx context.Context, assessmentID report.AssessmentID) (*report.InterpretReport, error) {
	// 由于 Report.ID == Assessment.ID，直接使用 FindByID
	return r.FindByID(ctx, report.ID(assessmentID))
}

// FindByTesteeID 查询受试者的报告列表
func (r *ReportRepository) FindByTesteeID(ctx context.Context, testeeID testee.ID, pagination report.Pagination) ([]*report.InterpretReport, int64, error) {
	filter := bson.M{
		"testee_id":  uint64(testeeID),
		"deleted_at": nil,
	}

	// 统计总数
	total, err := r.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("统计报告数量失败: %w", err)
	}

	// 分页查询
	findOptions := options.Find()
	findOptions.SetSkip(int64(pagination.Offset()))
	findOptions.SetLimit(int64(pagination.Limit()))
	findOptions.SetSort(bson.M{"created_at": -1})

	cursor, err := r.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, 0, fmt.Errorf("查询报告列表失败: %w", err)
	}
	defer cursor.Close(ctx)

	// 解析结果
	var pos []*InterpretReportPO
	for cursor.Next(ctx) {
		var po InterpretReportPO
		if err := cursor.Decode(&po); err != nil {
			return nil, 0, fmt.Errorf("解析报告数据失败: %w", err)
		}
		pos = append(pos, &po)
	}

	if err := cursor.Err(); err != nil {
		return nil, 0, fmt.Errorf("遍历报告数据失败: %w", err)
	}

	return r.mapper.ToDomainList(pos), total, nil
}

// Update 更新报告
func (r *ReportRepository) Update(ctx context.Context, rpt *report.InterpretReport) error {
	// 获取现有报告以获取 testeeID
	existing, err := r.FindByID(ctx, rpt.ID())
	if err != nil {
		return fmt.Errorf("查找现有报告失败: %w", err)
	}
	_ = existing // 暂时未使用

	po := r.mapper.ToPO(rpt, 0) // testeeID 在更新时不修改
	base.ApplyAuditUpdate(ctx, po)
	po.BeforeUpdate()

	filter := bson.M{
		"domain_id":  rpt.ID().Uint64(),
		"deleted_at": nil,
	}

	// 构建更新内容（不更新 testee_id）
	update := bson.M{
		"$set": bson.M{
			"scale_name":  po.ScaleName,
			"scale_code":  po.ScaleCode,
			"total_score": po.TotalScore,
			"risk_level":  po.RiskLevel,
			"conclusion":  po.Conclusion,
			"dimensions":  po.Dimensions,
			"suggestions": po.Suggestions,
			"updated_at":  po.UpdatedAt,
			"updated_by":  po.UpdatedBy,
		},
	}

	result, err := r.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("更新报告失败: %w", err)
	}

	if result.MatchedCount == 0 {
		return report.ErrReportNotFound
	}

	return nil
}

// Delete 删除报告（软删除）
func (r *ReportRepository) Delete(ctx context.Context, id report.ID) error {
	filter := bson.M{
		"domain_id":  id.Uint64(),
		"deleted_at": nil,
	}

	userID := base.AuditUserID(ctx)
	update := bson.M{
		"$set": bson.M{
			"deleted_at": r.nowTime(),
			"updated_at": r.nowTime(),
			"updated_by": userID,
			"deleted_by": userID,
		},
	}

	result, err := r.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("删除报告失败: %w", err)
	}

	if result.MatchedCount == 0 {
		return report.ErrReportNotFound
	}

	return nil
}

// ExistsByID 检查报告是否存在
func (r *ReportRepository) ExistsByID(ctx context.Context, id report.ID) (bool, error) {
	filter := bson.M{
		"domain_id":  id.Uint64(),
		"deleted_at": nil,
	}

	return r.ExistsByFilter(ctx, filter)
}

// ==================== 辅助方法 ====================

func (r *ReportRepository) nowTime() interface{} {
	return bson.M{"$currentDate": true}
}
