package plan

import (
	"context"
	"time"
)

type taskActionTimeKey struct{}

// WithTaskActionTime writes a business action timestamp into context.
// Seeddata uses this to replay historical plan transitions with planned-time-aligned timestamps.
func WithTaskActionTime(ctx context.Context, actionAt time.Time) context.Context {
	if ctx == nil || actionAt.IsZero() {
		return ctx
	}
	return context.WithValue(ctx, taskActionTimeKey{}, actionAt)
}

// TaskActionTimeOrNow returns the contextual business timestamp if provided.
func TaskActionTimeOrNow(ctx context.Context) time.Time {
	if ctx != nil {
		if actionAt, ok := ctx.Value(taskActionTimeKey{}).(time.Time); ok && !actionAt.IsZero() {
			return actionAt
		}
	}
	return time.Now()
}
