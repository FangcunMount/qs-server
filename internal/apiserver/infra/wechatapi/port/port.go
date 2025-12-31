package port

import (
	"context"
	"io"
)

// QRCodeGenerator 小程序码生成器接口
// 由基础设施层实现，负责生成微信小程序码
type QRCodeGenerator interface {
	// GenerateQRCode 生成小程序码（适用于数量较少的场景，最多 10 万个）
	// path: 小程序页面路径，例如 "pages/index/index"
	// width: 二维码宽度，单位 px，最小 280px，最大 1280px，默认 430px
	// 返回二维码图片的字节流
	GenerateQRCode(ctx context.Context, appID, appSecret, path string, width int) (io.Reader, error)

	// GenerateUnlimitedQRCode 生成无数量限制的小程序码（适用于大量生成场景）
	// scene: 场景值，最大 32 个字符，只能包含字母、数字、下划线
	// page: 小程序页面路径，例如 "pages/index/index"
	// width: 二维码宽度，单位 px，最小 280px，最大 1280px，默认 430px
	// autoColor: 是否自动配置线条颜色，默认 false
	// lineColor: 线条颜色，格式为 {"r":0,"g":0,"b":0}，autoColor 为 false 时生效
	// isHyaline: 是否透明底色，默认 false
	// 返回二维码图片的字节流
	GenerateUnlimitedQRCode(
		ctx context.Context,
		appID, appSecret, scene, page string,
		width int,
		autoColor bool,
		lineColor map[string]int,
		isHyaline bool,
	) (io.Reader, error)
}
