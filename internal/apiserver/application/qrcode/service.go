package qrcode

import (
	"context"
	"fmt"
	"io"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/iam"
	wechatPort "github.com/FangcunMount/qs-server/internal/apiserver/infra/wechatapi/port"
)

// Config 小程序码服务配置
type Config struct {
	// 从 IAM 查询配置
	WeChatAppID string // IAM 中的 wechatappId，例如 "597792321089581614"
	PagePath    string // 小程序页面路径，例如 "pages/questionnaire/index"

	// 可选：直接配置（如果 IAM 未启用时使用）
	AppID     string // 小程序 AppID（直接配置，优先级低于 IAM）
	AppSecret string // 小程序 AppSecret（直接配置，优先级低于 IAM）
}

// service 小程序码生成服务实现
type service struct {
	qrCodeGen        wechatPort.QRCodeGenerator
	config           *Config
	wechatAppService *iam.WeChatAppService
}

// NewService 创建小程序码生成服务
func NewService(qrCodeGen wechatPort.QRCodeGenerator, config *Config, wechatAppService *iam.WeChatAppService) QRCodeService {
	return &service{
		qrCodeGen:        qrCodeGen,
		config:           config,
		wechatAppService: wechatAppService,
	}
}

// getWechatAppConfig 获取微信应用配置（优先从 IAM 查询）
func (s *service) getWechatAppConfig(ctx context.Context) (appID, appSecret string, err error) {
	// 如果配置了 WeChatAppID 且 IAM 服务可用，从 IAM 查询
	if s.config.WeChatAppID != "" && s.wechatAppService != nil && s.wechatAppService.IsEnabled() {
		l := logger.L(ctx)
		l.Infow("从 IAM 查询微信应用配置",
			"action", "get_wechat_app_config",
			"wechat_app_id", s.config.WeChatAppID,
		)

		resp, err := s.wechatAppService.GetWechatApp(ctx, s.config.WeChatAppID)
		if err != nil {
			l.Errorw("从 IAM 查询微信应用配置失败",
				"action", "get_wechat_app_config",
				"wechat_app_id", s.config.WeChatAppID,
				"error", err.Error(),
			)
			return "", "", fmt.Errorf("从 IAM 查询微信应用配置失败: %w", err)
		}

		if resp == nil || resp.App == nil {
			return "", "", fmt.Errorf("IAM 返回的微信应用信息为空")
		}

		// 从响应中提取 AppID 和 AppSecret
		appID = resp.App.GetAppId()
		appSecret = resp.App.GetAppSecret()

		if appID == "" || appSecret == "" {
			return "", "", fmt.Errorf("IAM 返回的微信应用信息不完整: app_id=%s, app_secret=%s", appID, appSecret)
		}

		l.Infow("从 IAM 获取微信应用配置成功",
			"action", "get_wechat_app_config",
			"wechat_app_id", s.config.WeChatAppID,
			"app_id", appID,
		)

		return appID, appSecret, nil
	}

	// 降级：使用直接配置
	if s.config.AppID != "" && s.config.AppSecret != "" {
		return s.config.AppID, s.config.AppSecret, nil
	}

	return "", "", fmt.Errorf("微信应用配置未设置：请配置 WeChatAppID（IAM）或 AppID/AppSecret（直接配置）")
}

// GenerateQuestionnaireQRCode 生成问卷小程序码
func (s *service) GenerateQuestionnaireQRCode(ctx context.Context, code, version string) (string, error) {
	l := logger.L(ctx)

	// 验证参数
	if code == "" {
		return "", fmt.Errorf("问卷编码不能为空")
	}
	if version == "" {
		return "", fmt.Errorf("问卷版本不能为空")
	}

	// 检查配置
	if s.qrCodeGen == nil {
		return "", fmt.Errorf("小程序码生成器未配置")
	}

	l.Infow("开始生成问卷小程序码",
		"action", "generate_questionnaire_qrcode",
		"code", code,
		"version", version,
	)

	// 获取微信应用配置（优先从 IAM 查询）
	appID, appSecret, err := s.getWechatAppConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("获取微信应用配置失败: %w", err)
	}

	// 构建 scene 参数：包含问卷编码和版本
	// scene 最大 32 个字符，只能包含字母、数字、下划线
	scene := fmt.Sprintf("code=%s&v=%s", code, version)
	if len(scene) > 32 {
		// 如果超过 32 字符，只使用编码
		scene = code
		l.Warnw("scene 参数超过 32 字符，仅使用编码",
			"code", code,
			"version", version,
			"original_scene", fmt.Sprintf("code=%s&v=%s", code, version),
		)
	}

	// 调用基础设施层生成小程序码
	reader, err := s.qrCodeGen.GenerateUnlimitedQRCode(
		ctx,
		appID,
		appSecret,
		scene,
		s.config.PagePath,
		430,   // 默认宽度
		false, // autoColor
		nil,   // lineColor
		false, // isHyaline
	)
	if err != nil {
		l.Errorw("生成问卷小程序码失败",
			"action", "generate_questionnaire_qrcode",
			"code", code,
			"version", version,
			"error", err.Error(),
		)
		return "", fmt.Errorf("生成小程序码失败: %w", err)
	}

	// 读取二维码图片数据
	qrCodeData, err := io.ReadAll(reader)
	if err != nil {
		l.Errorw("读取小程序码数据失败",
			"action", "generate_questionnaire_qrcode",
			"code", code,
			"error", err.Error(),
		)
		return "", fmt.Errorf("读取小程序码数据失败: %w", err)
	}

	// TODO: 将二维码保存到文件系统或对象存储（OSS/S3等）
	// 这里返回占位符，实际需要根据存储方案实现
	_ = qrCodeData

	// 临时返回：实际应该保存到对象存储并返回 URL
	qrCodeURL := fmt.Sprintf("/qrcodes/questionnaire_%s_%s.png", code, version)

	l.Infow("问卷小程序码生成成功",
		"action", "generate_questionnaire_qrcode",
		"code", code,
		"version", version,
		"qrcode_url", qrCodeURL,
		"size", len(qrCodeData),
	)

	return qrCodeURL, nil
}

// GenerateScaleQRCode 生成量表小程序码
func (s *service) GenerateScaleQRCode(ctx context.Context, code string) (string, error) {
	l := logger.L(ctx)

	// 验证参数
	if code == "" {
		return "", fmt.Errorf("量表编码不能为空")
	}

	// 检查配置
	if s.qrCodeGen == nil {
		return "", fmt.Errorf("小程序码生成器未配置")
	}

	l.Infow("开始生成量表小程序码",
		"action", "generate_scale_qrcode",
		"code", code,
	)

	// 获取微信应用配置（优先从 IAM 查询）
	appID, appSecret, err := s.getWechatAppConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("获取微信应用配置失败: %w", err)
	}

	// 构建 scene 参数：包含量表编码
	// scene 最大 32 个字符，只能包含字母、数字、下划线
	scene := fmt.Sprintf("scale=%s", code)
	if len(scene) > 32 {
		// 如果超过 32 字符，只使用编码
		scene = code
		l.Warnw("scene 参数超过 32 字符，仅使用编码",
			"code", code,
			"original_scene", fmt.Sprintf("scale=%s", code),
		)
	}

	// 调用基础设施层生成小程序码
	reader, err := s.qrCodeGen.GenerateUnlimitedQRCode(
		ctx,
		appID,
		appSecret,
		scene,
		s.config.PagePath,
		430,   // 默认宽度
		false, // autoColor
		nil,   // lineColor
		false, // isHyaline
	)
	if err != nil {
		l.Errorw("生成量表小程序码失败",
			"action", "generate_scale_qrcode",
			"code", code,
			"error", err.Error(),
		)
		return "", fmt.Errorf("生成小程序码失败: %w", err)
	}

	// 读取二维码图片数据
	qrCodeData, err := io.ReadAll(reader)
	if err != nil {
		l.Errorw("读取小程序码数据失败",
			"action", "generate_scale_qrcode",
			"code", code,
			"error", err.Error(),
		)
		return "", fmt.Errorf("读取小程序码数据失败: %w", err)
	}

	// TODO: 将二维码保存到文件系统或对象存储（OSS/S3等）
	// 这里返回占位符，实际需要根据存储方案实现
	_ = qrCodeData

	// 临时返回：实际应该保存到对象存储并返回 URL
	qrCodeURL := fmt.Sprintf("/qrcodes/scale_%s.png", code)

	l.Infow("量表小程序码生成成功",
		"action", "generate_scale_qrcode",
		"code", code,
		"qrcode_url", qrCodeURL,
		"size", len(qrCodeData),
	)

	return qrCodeURL, nil
}
