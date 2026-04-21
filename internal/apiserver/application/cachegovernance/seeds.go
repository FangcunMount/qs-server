package cachegovernance

// StatisticsWarmupConfig 定义统计查询预热种子。
type StatisticsWarmupConfig struct {
	OrgIDs             []int64
	QuestionnaireCodes []string
	PlanIDs            []uint64
}
