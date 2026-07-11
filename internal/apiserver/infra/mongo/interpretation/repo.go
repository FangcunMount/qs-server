package interpretation

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	report "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	mongoEventOutbox "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/eventoutbox"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
)

// ReportRepository 报告 MongoDB 仓储
type ReportRepository struct {
	base.BaseRepository
	mapper *ReportMapper
}

// NewReportRepository 创建报告仓储
func NewReportRepository(db *mongo.Database, opts ...base.BaseRepositoryOptions) (*ReportRepository, error) {
	return NewReportRepositoryWithTopicResolver(db, nil, opts...)
}

func NewReportRepositoryWithTopicResolver(db *mongo.Database, _ eventcatalog.TopicResolver, opts ...base.BaseRepositoryOptions) (*ReportRepository, error) {
	repo := &ReportRepository{
		BaseRepository: base.NewBaseRepository(db, (&InterpretReportPO{}).CollectionName(), opts...),
		mapper:         NewReportMapper(),
	}
	if _, err := repo.Collection().Indexes().CreateMany(context.Background(), reportIndexModels()); err != nil {
		return nil, fmt.Errorf("创建报告索引失败: %w", err)
	}
	return repo, nil
}

func reportIndexModels() []mongo.IndexModel {
	return []mongo.IndexModel{
		{Keys: bson.D{{Key: "domain_id", Value: 1}, {Key: "deleted_at", Value: 1}}, Options: options.Index().SetName("uk_report_domain_deleted").SetUnique(true)},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "testee_id", Value: 1}, {Key: "created_at", Value: -1}}, Options: options.Index().SetName("idx_report_status_testee_created")},
	}
}

// 确保实现了接口
var _ report.ReportRepository = (*ReportRepository)(nil)

// ==================== 基础操作 ====================

// Save 保存报告
func (r *ReportRepository) Save(ctx context.Context, rpt *report.InterpretReport) error {
	return r.withTransaction(ctx, func(txCtx mongo.SessionContext) error {
		return r.SaveReportRecord(txCtx, rpt, 0)
	})
}

func (r *ReportRepository) SaveState(ctx context.Context, rpt *report.InterpretReport, testeeID testee.ID) error {
	return r.withTransaction(ctx, func(txCtx mongo.SessionContext) error {
		return r.SaveReportRecord(txCtx, rpt, testeeID)
	})
}

func (r *ReportRepository) SaveReportRecord(ctx context.Context, rpt *report.InterpretReport, testeeID testee.ID) error {
	if rpt == nil {
		return nil
	}
	txCtx, ok := ctx.(mongo.SessionContext)
	if !ok {
		return mongoEventOutbox.ErrActiveSessionTransactionRequired
	}

	exists, err := r.existsByIDTx(txCtx, rpt.ID())
	if err != nil {
		return fmt.Errorf("检查报告是否存在失败: %w", err)
	}

	if exists {
		return r.updateTx(txCtx, ctx, rpt, testeeID)
	}
	return r.insertTx(txCtx, ctx, rpt, testeeID)
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

// Update 更新报告
func (r *ReportRepository) Update(ctx context.Context, rpt *report.InterpretReport) error {
	return r.updateTx(nil, ctx, rpt, 0)
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

func (r *ReportRepository) insertTx(ctx mongo.SessionContext, auditCtx context.Context, rpt *report.InterpretReport, testeeID testee.ID) error {
	po := r.mapper.ToPO(rpt, testeeID.Uint64())
	base.ApplyAuditCreate(auditCtx, po)
	po.BeforeInsert()
	po.DomainID = rpt.ID()

	if _, err := r.Collection().InsertOne(ctx, po); err != nil {
		return fmt.Errorf("插入报告失败: %w", err)
	}
	return nil
}

func (r *ReportRepository) updateTx(txCtx mongo.SessionContext, auditCtx context.Context, rpt *report.InterpretReport, testeeID testee.ID) error {
	po := r.mapper.ToPO(rpt, 0)
	base.ApplyAuditUpdate(auditCtx, po)
	po.BeforeUpdate()

	filter := bson.M{
		"domain_id":  rpt.ID().Uint64(),
		"deleted_at": nil,
	}
	update := bson.M{
		"$set": bson.M{
			"outcome_id":     po.OutcomeID,
			"status":         po.Status,
			"attempt":        po.Attempt,
			"failure_reason": po.FailureReason,
			"generating_at":  po.GeneratingAt,
			"generated_at":   po.GeneratedAt,
			"failed_at":      po.FailedAt,
			"scale_name":     po.ScaleName,
			"scale_code":     po.ScaleCode,
			"model":          po.Model,
			"primary_score":  po.PrimaryScore,
			"level":          po.Level,
			"total_score":    po.TotalScore,
			"risk_level":     po.RiskLevel,
			"conclusion":     po.Conclusion,
			"dimensions":     po.Dimensions,
			"suggestions":    po.Suggestions,
			"model_extra":    po.ModelExtra,
			"updated_at":     po.UpdatedAt,
			"updated_by":     po.UpdatedBy,
		},
	}
	if !testeeID.IsZero() {
		update["$set"].(bson.M)["testee_id"] = testeeID.Uint64()
	}

	if txCtx != nil {
		result, err := r.Collection().UpdateOne(txCtx, filter, update)
		if err != nil {
			return fmt.Errorf("更新报告失败: %w", err)
		}
		if result.MatchedCount == 0 {
			return report.ErrReportNotFound
		}
		return nil
	}

	result, err := r.UpdateOne(auditCtx, filter, update)
	if err != nil {
		return fmt.Errorf("更新报告失败: %w", err)
	}
	if result.MatchedCount == 0 {
		return report.ErrReportNotFound
	}
	return nil
}

func (r *ReportRepository) existsByIDTx(ctx mongo.SessionContext, id report.ID) (bool, error) {
	filter := bson.M{
		"domain_id":  id.Uint64(),
		"deleted_at": nil,
	}
	count, err := r.Collection().CountDocuments(ctx, filter)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *ReportRepository) withTransaction(ctx context.Context, fn func(txCtx mongo.SessionContext) error) error {
	session, err := r.DB().Client().StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)

	_, err = session.WithTransaction(ctx, func(txCtx mongo.SessionContext) (interface{}, error) {
		return nil, fn(txCtx)
	})
	return err
}
