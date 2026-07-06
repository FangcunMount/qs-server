package result

import (
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	evaluationwaiter "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationwaiter"
)

func NewWaiterCompletionNotifier(waiterRegistry evaluationwaiter.Notifier) CompletionNotifier {
	return interpretationreporting.NewWaiterCompletionNotifier(waiterRegistry)
}
