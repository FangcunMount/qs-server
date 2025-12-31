package wechatapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/silenceper/wechat/v2"
	"github.com/silenceper/wechat/v2/cache"
	miniConfig "github.com/silenceper/wechat/v2/miniprogram/config"
	"github.com/silenceper/wechat/v2/miniprogram/qrcode"
)

// QRCodeGenerator 小程序码生成器实现
type QRCodeGenerator struct {
	cache cache.Cache // SDK 使用的缓存（可选，传 nil 则 SDK 使用内存缓存）
}

// NewQRCodeGenerator 创建小程序码生成器实例
func NewQRCodeGenerator(sdkCache cache.Cache) *QRCodeGenerator {
	return &QRCodeGenerator{
		cache: sdkCache,
	}
}

// GenerateQRCode 生成小程序码（适用于数量较少的场景，最多 10 万个）
func (g *QRCodeGenerator) GenerateQRCode(ctx context.Context, appID, appSecret, path string, width int) (io.Reader, error) {
	if appID == "" || appSecret == "" {
		return nil, errors.New("appID and appSecret cannot be empty")
	}
	if path == "" {
		return nil, errors.New("path cannot be empty")
	}

	// 设置默认宽度
	if width <= 0 {
		width = 430
	}

	logger.L(ctx).Infow("Generating miniprogram QR code",
		"infra_action", "generate_qrcode",
		"app_id", appID,
		"path", path,
		"width", width,
	)

	// 初始化微信 SDK
	wc := wechat.NewWechat()
	cfg := &miniConfig.Config{
		AppID:     appID,
		AppSecret: appSecret,
		Cache:     g.cache,
	}

	miniProgram := wc.GetMiniProgram(cfg)
	qr := miniProgram.GetQRCode()

	// 构建二维码参数
	coderParams := qrcode.QRCoder{
		Path:  path,
		Width: width,
	}

	// 生成小程序码
	response, err := qr.GetWXACode(coderParams)
	if err != nil {
		logger.L(ctx).Errorw("Failed to generate miniprogram QR code",
			"infra_action", "generate_qrcode",
			"app_id", appID,
			"path", path,
			"error", err.Error(),
		)
		return nil, fmt.Errorf("failed to generate miniprogram QR code: %w", err)
	}

	logger.L(ctx).Infow("Miniprogram QR code generated successfully",
		"infra_action", "generate_qrcode",
		"app_id", appID,
		"path", path,
		"size", len(response),
	)

	// 返回字节流
	return bytes.NewReader(response), nil
}

// GenerateUnlimitedQRCode 生成无数量限制的小程序码（适用于大量生成场景）
func (g *QRCodeGenerator) GenerateUnlimitedQRCode(
	ctx context.Context,
	appID, appSecret, scene, page string,
	width int,
	autoColor bool,
	lineColor map[string]int,
	isHyaline bool,
) (io.Reader, error) {
	if appID == "" || appSecret == "" {
		return nil, errors.New("appID and appSecret cannot be empty")
	}
	if scene == "" {
		return nil, errors.New("scene cannot be empty")
	}
	if page == "" {
		return nil, errors.New("page cannot be empty")
	}

	// 设置默认宽度
	if width <= 0 {
		width = 430
	}

	logger.L(ctx).Infow("Generating unlimited miniprogram QR code",
		"infra_action", "generate_unlimited_qrcode",
		"app_id", appID,
		"scene", scene,
		"page", page,
		"width", width,
		"auto_color", autoColor,
		"is_hyaline", isHyaline,
	)

	// 初始化微信 SDK
	wc := wechat.NewWechat()
	cfg := &miniConfig.Config{
		AppID:     appID,
		AppSecret: appSecret,
		Cache:     g.cache,
	}

	miniProgram := wc.GetMiniProgram(cfg)
	qr := miniProgram.GetQRCode()

	// 构建二维码参数（无限制版本使用 QRCoder，但需要设置 Scene 和 Page）
	coderParams := qrcode.QRCoder{
		Scene:     scene,
		Page:      page,
		Width:     width,
		AutoColor: autoColor,
		IsHyaline: isHyaline,
	}

	// 如果提供了 lineColor，转换为 qrcode.Color
	if lineColor != nil && len(lineColor) > 0 {
		color := &qrcode.Color{
			R: fmt.Sprintf("%d", lineColor["r"]),
			G: fmt.Sprintf("%d", lineColor["g"]),
			B: fmt.Sprintf("%d", lineColor["b"]),
		}
		coderParams.LineColor = color
	}

	// 生成小程序码
	response, err := qr.GetWXACodeUnlimit(coderParams)
	if err != nil {
		logger.L(ctx).Errorw("Failed to generate unlimited miniprogram QR code",
			"infra_action", "generate_unlimited_qrcode",
			"app_id", appID,
			"scene", scene,
			"page", page,
			"error", err.Error(),
		)
		return nil, fmt.Errorf("failed to generate unlimited miniprogram QR code: %w", err)
	}

	logger.L(ctx).Infow("Unlimited miniprogram QR code generated successfully",
		"infra_action", "generate_unlimited_qrcode",
		"app_id", appID,
		"scene", scene,
		"page", page,
		"size", len(response),
	)

	// 返回字节流
	return bytes.NewReader(response), nil
}
