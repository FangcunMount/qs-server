package cachegovernance

// StatisticsWarmupConfig 定义统计查询预热种子。
type StatisticsWarmupConfig struct {
	OrgIDs          []int64
	OverviewPresets []string
	WarmOnStartup   bool
}
