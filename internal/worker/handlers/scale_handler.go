package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

func init() {
	Register("scale_changed_handler", func(deps *Dependencies) HandlerFunc {
		return handleScaleChanged(deps)
	})
}

type ScaleChangedPayload struct {
	ScaleID   uint64    `json:"scale_id"`
	Code      string    `json:"code"`
	Version   string    `json:"version"`
	Name      string    `json:"name"`
	Action    string    `json:"action"`
	ChangedAt time.Time `json:"changed_at"`
}

func handleScaleChanged(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		var data ScaleChangedPayload
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse scale changed event: %w", err)
		}

		deps.Logger.Info("processing scale changed",
			slog.String("event_id", env.ID),
			slog.Uint64("scale_id", data.ScaleID),
			slog.String("code", data.Code),
			slog.String("version", data.Version),
			slog.String("name", data.Name),
			slog.String("action", data.Action),
		)

		if data.Action != "published" || deps.InternalClient == nil {
			return nil
		}

		resp, err := deps.InternalClient.GenerateScaleQRCode(ctx, data.Code)
		if err != nil {
			deps.Logger.Warn("failed to generate scale QR code",
				slog.String("event_id", env.ID),
				slog.String("code", data.Code),
				slog.String("action", data.Action),
				slog.String("error", err.Error()),
			)
			return nil
		}
		if resp.Success {
			deps.Logger.Info("scale QR code generated",
				slog.String("event_id", env.ID),
				slog.String("code", data.Code),
				slog.String("qrcode_url", resp.QrcodeUrl),
			)
			return nil
		}

		deps.Logger.Warn("scale QR code generation failed",
			slog.String("event_id", env.ID),
			slog.String("code", data.Code),
			slog.String("message", resp.Message),
		)
		return nil
	}
}
