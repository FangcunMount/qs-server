package handlers

import (
	"context"
	"log/slog"

	"github.com/FangcunMount/qs-server/internal/pkg/eventpayload"
)

const assessmentModelKindScale = "scale"

func handleAssessmentModelChanged(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, _ string, payload []byte) error {
		return handleLifecycleChangedEvent(ctx, deps, payload, lifecycleChangedCallbacks[eventpayload.AssessmentModelChangedData]{
			parseErrorLabel: "assessment model changed event",
			action: func(data *eventpayload.AssessmentModelChangedData) string {
				return string(data.Action)
			},
			logFields: func(env *EventEnvelope, data *eventpayload.AssessmentModelChangedData) []any {
				return []any{
					slog.String("event_id", env.ID),
					slog.String("kind", data.Kind),
					slog.String("code", data.Code),
					slog.String("version", data.Version),
					slog.String("title", data.Title),
					slog.String("action", string(data.Action)),
				}
			},
			onPublished: func(ctx context.Context, deps *Dependencies, env *EventEnvelope, data *eventpayload.AssessmentModelChangedData) error {
				if data.Kind != assessmentModelKindScale {
					return nil
				}
				resp, err := deps.InternalClient.HandleScalePublishedPostActions(ctx, data.Code)
				if err != nil {
					deps.Logger.Warn("failed to handle assessment model publish post-actions",
						slog.String("event_id", env.ID),
						slog.String("kind", data.Kind),
						slog.String("code", data.Code),
						slog.String("action", string(data.Action)),
						slog.String("error", err.Error()),
					)
					return nil
				}
				if resp.Success {
					deps.Logger.Info("assessment model publish post-actions completed",
						slog.String("event_id", env.ID),
						slog.String("kind", data.Kind),
						slog.String("code", data.Code),
						slog.String("qrcode_url", resp.QrcodeUrl),
					)
					return nil
				}

				deps.Logger.Warn("assessment model publish post-actions failed",
					slog.String("event_id", env.ID),
					slog.String("kind", data.Kind),
					slog.String("code", data.Code),
					slog.String("message", resp.Message),
				)
				return nil
			},
		})
	}
}
