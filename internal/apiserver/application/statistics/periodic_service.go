package statistics

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

type periodicStatsService struct {
	reader PeriodicStatsReader
}

// NewPeriodicStatsService 创建受试者周期统计服务。
func NewPeriodicStatsService(reader PeriodicStatsReader) PeriodicStatsService {
	return &periodicStatsService{reader: reader}
}

func (s *periodicStatsService) GetPeriodicStats(ctx context.Context, orgID int64, testeeID uint64) (*domainStatistics.TesteePeriodicStatisticsResponse, error) {
	if s == nil || s.reader == nil {
		return nil, errors.WithCode(code.ErrModuleInitializationFailed, "statistics periodic reader is unavailable")
	}
	return s.reader.GetPeriodicStats(ctx, orgID, testeeID)
}
