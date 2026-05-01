package scale

import (
	"context"
	"time"
)

// HotRankItem 表示 Redis 热度榜中的一个问卷提交热度项。
type HotRankItem struct {
	QuestionnaireCode string
	Score             int64
}

// HotRankRecorder 记录量表相关问卷提交热度。
type HotRankRecorder interface {
	RecordSubmission(ctx context.Context, questionnaireCode string, submittedAt time.Time) error
}

// HotRankReader 读取量表相关问卷提交热度榜。
type HotRankReader interface {
	TopSubmissions(ctx context.Context, windowDays, limit int) ([]HotRankItem, error)
}
