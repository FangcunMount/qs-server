package statistics

import "time"

// TrendAnalyzer 趋势分析器
// 职责：计算时间序列趋势
type TrendAnalyzer struct{}

// NewTrendAnalyzer 创建趋势分析器
func NewTrendAnalyzer() *TrendAnalyzer {
	return &TrendAnalyzer{}
}

// AnalyzeTrend 分析趋势
// 返回：增长趋势（正数表示增长，负数表示下降，0表示无变化）
func (t *TrendAnalyzer) AnalyzeTrend(dailyCounts []DailyCount, days int) float64 {
	if len(dailyCounts) < days*2 {
		return 0.0
	}

	// 计算最近N天和前N天的平均值
	recentSum := int64(0)
	previousSum := int64(0)

	cutoff := time.Now().AddDate(0, 0, -days)
	recentCount := 0
	previousCount := 0

	for _, dc := range dailyCounts {
		if dc.Date.After(cutoff) || dc.Date.Equal(cutoff) {
			recentSum += dc.Count
			recentCount++
		} else if dc.Date.After(cutoff.AddDate(0, 0, -days)) {
			previousSum += dc.Count
			previousCount++
		}
	}

	if recentCount == 0 || previousCount == 0 {
		return 0.0
	}

	recentAvg := float64(recentSum) / float64(recentCount)
	previousAvg := float64(previousSum) / float64(previousCount)

	if previousAvg == 0 {
		return 0.0
	}

	// 返回增长率百分比
	return (recentAvg - previousAvg) / previousAvg * 100.0
}

// GetLastNDays 获取最近N天的每日计数
func (t *TrendAnalyzer) GetLastNDays(dailyCounts []DailyCount, days int) []DailyCount {
	cutoff := time.Now().AddDate(0, 0, -days)
	result := make([]DailyCount, 0)

	for _, dc := range dailyCounts {
		if dc.Date.After(cutoff) || dc.Date.Equal(cutoff) {
			result = append(result, dc)
		}
	}

	return result
}
