package statisticscache

import (
	"context"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

// SystemStatisticsLoader 在查询缓存 miss 时合并并发回源（B1）。
type SystemStatisticsLoader interface {
	LoadSystemStatisticsCoalesced(
		ctx context.Context,
		orgID int64,
		loader func(context.Context) (*domainStatistics.SystemStatistics, error),
	) (*domainStatistics.SystemStatistics, error)
}
