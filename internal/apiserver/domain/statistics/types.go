package statistics

import "time"

// DailyCount 每日计数
type DailyCount struct {
	Date  time.Time `json:"date"`
	Count int64     `json:"count"`
}
