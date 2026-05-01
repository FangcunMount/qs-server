package scale

import (
	"context"
	"time"
)

// ScaleHotRankSubmissionFact 表示可投影到量表热度榜的一次答卷提交事实。
type ScaleHotRankSubmissionFact struct {
	EventID           string
	QuestionnaireCode string
	SubmittedAt       time.Time
}

// ScaleHotRankQuery 表示热度榜读模型查询条件。
type ScaleHotRankQuery struct {
	WindowDays int
	Limit      int
}

// ScaleHotRankEntry 表示量表热度榜读模型中的一个问卷提交热度项。
type ScaleHotRankEntry struct {
	QuestionnaireCode string
	Score             int64
}

// ScaleHotRankProjection 投影答卷提交事实，维护量表热度读模型。
type ScaleHotRankProjection interface {
	ProjectSubmission(ctx context.Context, fact ScaleHotRankSubmissionFact) error
}

// ScaleHotRankReadModel 读取量表热度排行榜。
type ScaleHotRankReadModel interface {
	Top(ctx context.Context, query ScaleHotRankQuery) ([]ScaleHotRankEntry, error)
}
