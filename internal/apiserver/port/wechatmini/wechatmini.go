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

// SubscribeSendReceipt is evidence of the platform API response, not proof that
// a user received the message. The documented success response has errcode=0
// but does not guarantee a msgid, so PlatformMessageID is optional.
type SubscribeSendReceipt struct {
	Accepted          bool
	PlatformMessageID string
}

// MiniProgramSubscribeReceiptSender is an additive contract for callers that persist send results.
// A returned error does not establish that the platform did not accept the request: the response
// may have been lost after the request was sent. Callers must not retry such results blindly.
type MiniProgramSubscribeReceiptSender interface {
	SendSubscribeMessageWithReceipt(ctx context.Context, appID, appSecret string, msg SubscribeMessage) (SubscribeSendReceipt, error)
}
