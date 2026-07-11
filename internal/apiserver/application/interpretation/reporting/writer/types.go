package writer

import (
	"context"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/pkg/event"
)

type Generation struct {
	Report *domainreport.InterpretReport
	Events []event.DomainEvent
}

type Generator interface {
	Generate(ctx context.Context, outcome evaloutcome.Outcome) (Generation, error)
}

// Writer 持久化reports 和 transitions Assessment 到 interpreted。
type Writer interface {
	Write(ctx context.Context, outcome evaloutcome.Outcome) error
}

// CompletionNotifier notifies waiters 在之后 interpretation completes。
type CompletionNotifier interface {
	NotifyCompletion(ctx context.Context, outcome evaloutcome.Outcome)
}
