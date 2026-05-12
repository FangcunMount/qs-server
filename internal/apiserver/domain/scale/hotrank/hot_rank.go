package hotrank

import (
	"context"
	"time"
)

// SubmissionFact 表示可投影到量表热度榜的一次答卷提交事实。
type SubmissionFact struct {
	EventID           string
	QuestionnaireCode string
	SubmittedAt       time.Time
}

// Query 表示热度榜读模型查询条件。
type Query struct {
	WindowDays int
	Limit      int
}

// Entry 表示量表热度榜读模型中的一个问卷提交热度项。
type Entry struct {
	QuestionnaireCode string
	Score             int64
}

// Projection 投影答卷提交事实，维护量表热度读模型。
type Projection interface {
	ProjectSubmission(ctx context.Context, fact SubmissionFact) error
}

// ReadModel 读取量表热度排行榜。
type ReadModel interface {
	Top(ctx context.Context, query Query) ([]Entry, error)
}
