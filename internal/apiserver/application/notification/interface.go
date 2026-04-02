package notification

import (
	"context"
	"time"
)

// TaskOpenedDTO 是 task.opened 小程序通知请求。
type TaskOpenedDTO struct {
	OrgID    int64
	TaskID   string
	TesteeID uint64
	EntryURL string
	OpenAt   time.Time
}

// TaskOpenedResult 是 task.opened 小程序通知结果。
type TaskOpenedResult struct {
	SentCount        int
	RecipientOpenIDs []string
	RecipientSource  string
	TemplateID       string
	Skipped          bool
	Message          string
}

// MiniProgramTaskNotificationService 负责向小程序账号推送 task 消息。
type MiniProgramTaskNotificationService interface {
	SendTaskOpened(ctx context.Context, dto TaskOpenedDTO) (*TaskOpenedResult, error)
}
