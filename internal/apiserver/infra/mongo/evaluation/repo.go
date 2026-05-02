package evaluation

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	mongoEventOutbox "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/eventoutbox"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// ReportRepository 报告 MongoDB 仓储
type ReportRepository struct {
	base.BaseRepository
	mapper      *ReportMapper
	outboxStore *mongoEventOutbox.Store
}

// NewReportRepository 创建报告仓储
func NewReportRepository(db *mongo.Database, opts ...base.BaseRepositoryOptions) (*ReportRepository, error) {
	return NewReportRepositoryWithTopicResolver(db, nil, opts...)
}

func NewReportRepositoryWithTopicResolver(db *mongo.Database, resolver eventcatalog.TopicResolver, opts ...base.BaseRepositoryOptions) (*ReportRepository, error) {
	outboxStore, err := mongoEventOutbox.NewStoreWithTopicResolver(db, resolver)
	if err != nil {
		return nil, err
	}

	return &ReportRepository{
		BaseRepository: base.NewBaseRepository(db, (&InterpretReportPO{}).CollectionName(), opts...),
		mapper:         NewReportMapper(),
		outboxStore:    outboxStore,
	}, nil
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

func (r *ReportRepository) SaveReportDurably(ctx context.Context, rpt *report.InterpretReport, testeeID testee.ID, events []event.DomainEvent) error {
	if rpt == nil {
		return nil
	}

	return r.withTransaction(ctx, func(txCtx mongo.SessionContext) error {
		if err := r.SaveReportRecord(txCtx, rpt, testeeID); err != nil {
			return err
		}
		if len(events) > 0 {
			if err := r.outboxStore.StageEventsTx(txCtx, events); err != nil {
				return fmt.Errorf("暂存报告事件失败: %w", err)
			}
		}

		return nil
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
		return r.updateTx(txCtx, ctx, rpt)
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
	return r.updateTx(nil, ctx, rpt)
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

func (r *ReportRepository) updateTx(txCtx mongo.SessionContext, auditCtx context.Context, rpt *report.InterpretReport) error {
	po := r.mapper.ToPO(rpt, 0)
	base.ApplyAuditUpdate(auditCtx, po)
	po.BeforeUpdate()

	filter := bson.M{
		"domain_id":  rpt.ID().Uint64(),
		"deleted_at": nil,
	}
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
