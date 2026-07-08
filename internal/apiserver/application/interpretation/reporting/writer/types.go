package writer

import (
	"context"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
)

// Writer 持久化reports 和 transitions Assessment 到 interpreted。
type Writer interface {
	Write(ctx context.Context, outcome evaloutcome.Outcome) error
}

// CompletionNotifier notifies waiters 在之后 interpretation completes。
type CompletionNotifier interface {
	NotifyCompletion(ctx context.Context, outcome evaloutcome.Outcome)
}
