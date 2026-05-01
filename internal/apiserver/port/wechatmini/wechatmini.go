package wechatmini

import (
	"context"
	"io"
)

// QRCodeGenerator defines the application-facing mini program QR code generator.
type QRCodeGenerator interface {
	GenerateQRCode(ctx context.Context, appID, appSecret, path string, width int) (io.Reader, error)
	GenerateUnlimitedQRCode(
		ctx context.Context,
		appID, appSecret, scene, page string,
		width int,
		autoColor bool,
		lineColor map[string]int,
		isHyaline bool,
	) (io.Reader, error)
}

// SubscribeMessage describes a mini program subscribe message.
type SubscribeMessage struct {
	ToUser           string
	TemplateID       string
	Page             string
	MiniProgramState string
	Lang             string
	Data             map[string]string
}

// SubscribeTemplate describes a mini program subscribe template.
type SubscribeTemplate struct {
	ID      string
	Title   string
	Content string
}

// MiniProgramSubscribeSender sends and introspects mini program subscribe messages.
type MiniProgramSubscribeSender interface {
	SendSubscribeMessage(ctx context.Context, appID, appSecret string, msg SubscribeMessage) error
	ListTemplates(ctx context.Context, appID, appSecret string) ([]SubscribeTemplate, error)
}
