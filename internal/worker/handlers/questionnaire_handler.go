package handlers

import (
	"context"
	"log/slog"

	"github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	workerobservability "github.com/FangcunMount/qs-server/internal/worker/observability"
)

func handleQuestionnaireChanged(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, _ string, payload []byte) error {
		return handleLifecycleChangedEvent(ctx, deps, payload, lifecycleChangedCallbacks[eventpayload.QuestionnaireChangedData]{
			parseErrorLabel: "questionnaire changed event",
			action: func(data *eventpayload.QuestionnaireChangedData) string {
				return string(data.Action)
			},
			logFields: func(env *EventEnvelope, data *eventpayload.QuestionnaireChangedData) []any {
				return []any{
					slog.String("event_id", env.ID),
					slog.String("code", data.Code),
					slog.String("version", data.Version),
					slog.String("title", data.Title),
					slog.String("action", string(data.Action)),
				}
			},
			onMissingClient: func(deps *Dependencies, env *EventEnvelope, data *eventpayload.QuestionnaireChangedData) {
				workerobservability.ObserveBestEffortSideEffect("questionnaire.changed", workerobservability.BestEffortNotConfigured)
				deps.Logger.Warn("questionnaire publish post-actions client not configured",
					slog.String("event_id", env.ID), slog.String("code", data.Code), slog.String("version", data.Version))
			},
			onPublished: func(ctx context.Context, deps *Dependencies, env *EventEnvelope, data *eventpayload.QuestionnaireChangedData) error {
				resp, err := deps.InternalClient.HandleQuestionnairePublishedPostActions(ctx, data.Code, data.Version)
				if err != nil {
					workerobservability.ObserveBestEffortSideEffect("questionnaire.changed", workerobservability.BestEffortCallFailed)
					deps.Logger.Warn("failed to handle questionnaire publish post-actions",
						slog.String("event_id", env.ID),
						slog.String("code", data.Code),
						slog.String("action", string(data.Action)),
						slog.String("error", err.Error()),
					)
					return nil
				}
				if resp != nil && resp.Success {
					workerobservability.ObserveBestEffortSideEffect("questionnaire.changed", workerobservability.BestEffortCallSucceeded)
					deps.Logger.Info("questionnaire publish post-actions completed",
						slog.String("event_id", env.ID),
						slog.String("code", data.Code),
						slog.String("qrcode_url", resp.QrcodeUrl),
					)
					return nil
				}

				workerobservability.ObserveBestEffortSideEffect("questionnaire.changed", workerobservability.BestEffortResponseRejected)
				message := "empty response"
				if resp != nil {
					message = resp.Message
				}
				deps.Logger.Warn("questionnaire publish post-actions failed",
					slog.String("event_id", env.ID),
					slog.String("code", data.Code),
					slog.String("message", message),
				)
				return nil
			},
		})
	}
}
