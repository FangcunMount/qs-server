package handlers

import (
	"context"
	"log/slog"

	domainQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
)

func handleQuestionnaireChanged(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, _ string, payload []byte) error {
		return handleLifecycleChangedEvent(ctx, deps, payload, lifecycleChangedCallbacks[domainQuestionnaire.QuestionnaireChangedData]{
			parseErrorLabel: "questionnaire changed event",
			action: func(data *domainQuestionnaire.QuestionnaireChangedData) string {
				return string(data.Action)
			},
			logFields: func(env *EventEnvelope, data *domainQuestionnaire.QuestionnaireChangedData) []any {
				return []any{
					slog.String("event_id", env.ID),
					slog.String("code", data.Code),
					slog.String("version", data.Version),
					slog.String("title", data.Title),
					slog.String("action", string(data.Action)),
				}
			},
			onPublished: func(ctx context.Context, deps *Dependencies, env *EventEnvelope, data *domainQuestionnaire.QuestionnaireChangedData) error {
				resp, err := deps.InternalClient.HandleQuestionnairePublishedPostActions(ctx, data.Code, data.Version)
				if err != nil {
					deps.Logger.Warn("failed to handle questionnaire publish post-actions",
						slog.String("event_id", env.ID),
						slog.String("code", data.Code),
						slog.String("action", string(data.Action)),
						slog.String("error", err.Error()),
					)
					return nil
				}
				if resp.Success {
					deps.Logger.Info("questionnaire publish post-actions completed",
						slog.String("event_id", env.ID),
						slog.String("code", data.Code),
						slog.String("qrcode_url", resp.QrcodeUrl),
					)
					return nil
				}

				deps.Logger.Warn("questionnaire publish post-actions failed",
					slog.String("event_id", env.ID),
					slog.String("code", data.Code),
					slog.String("message", resp.Message),
				)
				return nil
			},
		})
	}
}
