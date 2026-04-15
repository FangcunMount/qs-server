package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

func init() {
	Register("questionnaire_changed_handler", func(deps *Dependencies) HandlerFunc {
		return handleQuestionnaireChanged(deps)
	})
}

type QuestionnaireChangedPayload struct {
	Code      string    `json:"code"`
	Version   string    `json:"version"`
	Title     string    `json:"title"`
	Action    string    `json:"action"`
	ChangedAt time.Time `json:"changed_at"`
}

func handleQuestionnaireChanged(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		var data QuestionnaireChangedPayload
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse questionnaire changed event: %w", err)
		}

		deps.Logger.Info("processing questionnaire changed",
			slog.String("event_id", env.ID),
			slog.String("code", data.Code),
			slog.String("version", data.Version),
			slog.String("title", data.Title),
			slog.String("action", data.Action),
		)

		if data.Action != "published" || deps.InternalClient == nil {
			return nil
		}

		resp, err := deps.InternalClient.GenerateQuestionnaireQRCode(ctx, data.Code, data.Version)
		if err != nil {
			deps.Logger.Warn("failed to generate questionnaire QR code",
				slog.String("event_id", env.ID),
				slog.String("code", data.Code),
				slog.String("action", data.Action),
				slog.String("error", err.Error()),
			)
			return nil
		}
		if resp.Success {
			deps.Logger.Info("questionnaire QR code generated",
				slog.String("event_id", env.ID),
				slog.String("code", data.Code),
				slog.String("qrcode_url", resp.QrcodeUrl),
			)
			return nil
		}

		deps.Logger.Warn("questionnaire QR code generation failed",
			slog.String("event_id", env.ID),
			slog.String("code", data.Code),
			slog.String("message", resp.Message),
		)
		return nil
	}
}
