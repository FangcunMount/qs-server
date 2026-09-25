package handlers

import (
	"context"
	"fmt"
)

type lifecycleChangedCallbacks[T any] struct {
	parseErrorLabel string
	action          func(*T) string
	logFields       func(*EventEnvelope, *T) []any
	onMissingClient func(*Dependencies, *EventEnvelope, *T)
	onPublished     func(context.Context, *Dependencies, *EventEnvelope, *T) error
}

func handleLifecycleChangedEvent[T any](
	ctx context.Context,
	deps *Dependencies,
	payload []byte,
	callbacks lifecycleChangedCallbacks[T],
) error {
	var data T
	env, err := ParseEventData(payload, &data)
	if err != nil {
		return fmt.Errorf("failed to parse %s: %w", callbacks.parseErrorLabel, err)
	}

	if callbacks.logFields != nil {
		deps.Logger.Info("processing lifecycle change", callbacks.logFields(env, &data)...)
	}

	if callbacks.action == nil || callbacks.action(&data) != "published" || callbacks.onPublished == nil {
		return nil
	}
	if deps.InternalClient == nil {
		if callbacks.onMissingClient != nil {
			callbacks.onMissingClient(deps, env, &data)
		}
		return nil
	}

	return callbacks.onPublished(ctx, deps, env, &data)
}
