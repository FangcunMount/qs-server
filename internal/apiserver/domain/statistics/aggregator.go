package statistics

import "time"

// Aggregator 统计聚合器
// 职责：封装统计计算逻辑
type Aggregator struct{}

// NewAggregator 创建统计聚合器
func NewAggregator() *Aggregator {
	return &Aggregator{}
}

// CalculateCompletionRate 计算完成率
func (a *Aggregator) CalculateCompletionRate(total, completed int64) float64 {
	if total == 0 {
		return 0.0
	}
	return float64(completed) / float64(total) * 100.0
}

// CalculateParticipationRate 计算参与率
func (a *Aggregator) CalculateParticipationRate(total, target int64) float64 {
	if target == 0 {
		return 0.0
	}
	return float64(total) / float64(target) * 100.0
}

// AggregateDailyCounts 聚合每日计数
// 将时间序列数据聚合为每日计数
func (a *Aggregator) AggregateDailyCounts(timestamps []time.Time) []DailyCount {
	if len(timestamps) == 0 {
		return []DailyCount{}
	}

	// 按日期分组计数
	dailyMap := make(map[string]int64)
	for _, ts := range timestamps {
		dateKey := ts.Format("2006-01-02")
		dailyMap[dateKey]++
	}

	// 转换为切片并排序
	result := make([]DailyCount, 0, len(dailyMap))
	for dateKey, count := range dailyMap {
		date, _ := time.Parse("2006-01-02", dateKey)
		result = append(result, DailyCount{
			Date:  date,
			Count: count,
		})
	}

	// 按日期排序
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[i].Date.After(result[j].Date) {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	return result
}

// CalculateWindowCount 计算时间窗口内的计数
func (a *Aggregator) CalculateWindowCount(timestamps []time.Time, windowDays int) int64 {
	if windowDays <= 0 {
		return 0
	}

	cutoff := time.Now().AddDate(0, 0, -windowDays)
	count := int64(0)
	for _, ts := range timestamps {
		if ts.After(cutoff) || ts.Equal(cutoff) {
			count++
		}
	}
	return count
}
