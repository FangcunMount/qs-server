package handlers

import (
	"context"
	"log/slog"

	domainScale "github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
)

func init() {
	Register("scale_changed_handler", func(deps *Dependencies) HandlerFunc {
		return handleScaleChanged(deps)
	})
}

func handleScaleChanged(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, _ string, payload []byte) error {
		return handleLifecycleChangedEvent(ctx, deps, payload, lifecycleChangedCallbacks[domainScale.ScaleChangedData]{
			parseErrorLabel: "scale changed event",
			action: func(data *domainScale.ScaleChangedData) string {
				return string(data.Action)
			},
			logFields: func(env *EventEnvelope, data *domainScale.ScaleChangedData) []any {
				return []any{
					slog.String("event_id", env.ID),
					slog.Uint64("scale_id", data.ScaleID),
					slog.String("code", data.Code),
					slog.String("version", data.Version),
					slog.String("name", data.Name),
					slog.String("action", string(data.Action)),
				}
			},
			onPublished: func(ctx context.Context, deps *Dependencies, env *EventEnvelope, data *domainScale.ScaleChangedData) error {
				resp, err := deps.InternalClient.HandleScalePublishedPostActions(ctx, data.Code)
				if err != nil {
					deps.Logger.Warn("failed to handle scale publish post-actions",
						slog.String("event_id", env.ID),
						slog.String("code", data.Code),
						slog.String("action", string(data.Action)),
						slog.String("error", err.Error()),
					)
					return nil
				}
				if resp.Success {
					deps.Logger.Info("scale publish post-actions completed",
						slog.String("event_id", env.ID),
						slog.String("code", data.Code),
						slog.String("qrcode_url", resp.QrcodeUrl),
					)
					return nil
				}

				deps.Logger.Warn("scale publish post-actions failed",
					slog.String("event_id", env.ID),
					slog.String("code", data.Code),
					slog.String("message", resp.Message),
				)
				return nil
			},
		})
	}
}
