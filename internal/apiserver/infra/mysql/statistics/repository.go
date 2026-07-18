package statistics

import (
	"context"

	gormuow "github.com/FangcunMount/component-base/pkg/uow/gorm"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
	"gorm.io/gorm"
)

// StatisticsRepository 维护统计事实、聚合读模型和统计查询适配。
type StatisticsRepository struct {
	db      *gorm.DB
	limiter backpressure.Acquirer
}

// NewStatisticsRepository 创建统计仓储。
func NewStatisticsRepository(db *gorm.DB, opts ...mysql.BaseRepositoryOptions) *StatisticsRepository {
	options := mysql.BaseRepositoryOptions{}
	if len(opts) > 0 {
		options = opts[0]
	}
	return &StatisticsRepository{db: db, limiter: options.Limiter}
}

func (r *StatisticsRepository) acquire(ctx context.Context) (context.Context, func(), error) {
	if r == nil || r.limiter == nil {
		return ctx, func() {}, nil
	}
	return r.limiter.Acquire(ctx)
}

func (r *StatisticsRepository) withContext(ctx context.Context) *gorm.DB {
	return gormuow.WithContext(ctx, r.db)
}

func (r *StatisticsRepository) writeAssessmentEntryResolveLog(ctx context.Context, po *AssessmentEntryResolveLogPO) error {
	ctx, release, err := r.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	return r.withContext(ctx).Create(po).Error
}

func (r *StatisticsRepository) writeAssessmentEntryIntakeLog(ctx context.Context, po *AssessmentEntryIntakeLogPO) error {
	ctx, release, err := r.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	return r.withContext(ctx).Create(po).Error
}
