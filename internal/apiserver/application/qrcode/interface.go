package qrcode

import "context"

// QRCodeService 小程序码生成服务
// 行为者：Worker（通过 gRPC 调用）
// 职责：生成问卷和量表的小程序码，处理二维码存储和 URL 生成
// 变更来源：小程序码生成策略、存储方案的变化
type QRCodeService interface {
	// GenerateQuestionnaireQRCode 生成问卷小程序码
	// 场景：worker 处理 questionnaire.published 事件后调用
	// 流程：
	//   1. 构建 scene 参数（包含问卷编码和版本）
	//   2. 调用基础设施层生成小程序码
	//   3. 保存二维码图片（当前为占位符，后续接入对象存储）
	//   4. 返回二维码 URL
	GenerateQuestionnaireQRCode(ctx context.Context, code, version string) (string, error)

	// GenerateScaleQRCode 生成量表小程序码
	// 场景：worker 处理 scale.published 事件后调用
	// 流程：
	//   1. 构建 scene 参数（包含量表编码）
	//   2. 调用基础设施层生成小程序码
	//   3. 保存二维码图片（当前为占位符，后续接入对象存储）
	//   4. 返回二维码 URL
	GenerateScaleQRCode(ctx context.Context, code string) (string, error)
}
