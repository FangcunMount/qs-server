package statistics

import (
	"context"

	gormuow "github.com/FangcunMount/component-base/pkg/uow/gorm"
	"gorm.io/gorm"
)

// StatisticsRepository 维护统计事实、聚合读模型和统计查询适配。
type StatisticsRepository struct {
	db *gorm.DB
}

// NewStatisticsRepository 创建统计仓储。
func NewStatisticsRepository(db *gorm.DB, _ ...interface{}) *StatisticsRepository {
	return &StatisticsRepository{db: db}
}

func (r *StatisticsRepository) WithContext(ctx context.Context) *gorm.DB {
	return gormuow.WithContext(ctx, r.db)
}
