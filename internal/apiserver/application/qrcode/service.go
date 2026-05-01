package qrcode

import (
	"context"
	"fmt"
	"io"

	"github.com/FangcunMount/component-base/pkg/logger"
	iambridge "github.com/FangcunMount/qs-server/internal/apiserver/port/iambridge"
	qrcodeasset "github.com/FangcunMount/qs-server/internal/apiserver/port/qrcodeasset"
	wechatmini "github.com/FangcunMount/qs-server/internal/apiserver/port/wechatmini"
)

const (
	// QRCodeStorageDir 二维码存储目录
	QRCodeStorageDir = "/data/image/qrcode"
	QRCodeURLPrefix  = "https://qs.fangcunmount.cn/api/v1/qrcodes"
)

// Config 小程序码服务配置
type Config struct {
	// 从 IAM 查询配置
	WeChatAppID string // IAM 中的 wechatappId，例如 "597792321089581614"
	PagePath    string // 小程序页面路径，例如 "pages/questionnaire/index"

	// 可选：直接配置（如果 IAM 未启用时使用）
	AppID     string // 小程序 AppID（直接配置，优先级低于 IAM）
	AppSecret string // 小程序 AppSecret（直接配置，优先级低于 IAM）

	// OSS 对象 key 前缀；为空时直接使用文件名。
	ObjectKeyPrefix string
	// 对外返回的二维码访问前缀；为空时回退默认路由前缀。
	PublicURLPrefix string
}

// service 小程序码生成服务实现
type service struct {
	qrCodeGen        wechatmini.QRCodeGenerator
	config           *Config
	wechatAppService iambridge.WeChatAppConfigProvider
	imageStore       qrcodeasset.ImageStore
}

// NewService 创建小程序码生成服务
func NewService(
	qrCodeGen wechatmini.QRCodeGenerator,
	config *Config,
	wechatAppService iambridge.WeChatAppConfigProvider,
	imageStore qrcodeasset.ImageStore,
) QRCodeService {
	return &service{
		qrCodeGen:        qrCodeGen,
		config:           config,
		wechatAppService: wechatAppService,
		imageStore:       imageStore,
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

		resp, err := s.wechatAppService.ResolveWeChatAppConfig(ctx, s.config.WeChatAppID)
		if err != nil {
			l.Errorw("从 IAM 查询微信应用配置失败",
				"action", "get_wechat_app_config",
				"wechat_app_id", s.config.WeChatAppID,
				"error", err.Error(),
			)
			return "", "", fmt.Errorf("从 IAM 查询微信应用配置失败: %w", err)
		}

		if resp == nil {
			return "", "", fmt.Errorf("IAM 返回的微信应用信息为空")
		}

		appID = resp.AppID
		appSecret = resp.AppSecret

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

	// 构建 scene 参数：前端填写页实际消费 q，v 仅作为附加信息保留
	// scene 最大 32 个字符，只能包含字母、数字、下划线
	scene := fmt.Sprintf("q=%s&v=%s", code, version)
	if len(scene) > 32 {
		// 如果超过 32 字符，仅保留前端必需的 q 参数
		scene = fmt.Sprintf("q=%s", code)
		l.Warnw("scene 参数超过 32 字符，仅保留问卷编码",
			"code", code,
			"version", version,
			"original_scene", fmt.Sprintf("q=%s&v=%s", code, version),
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

	fileName := fmt.Sprintf("questionnaire_%s_%s.png", code, version)
	qrCodeURL, err := s.persistQRCode(ctx, fileName, qrCodeData)
	if err != nil {
		l.Errorw("持久化小程序码失败",
			"action", "generate_questionnaire_qrcode",
			"code", code,
			"version", version,
			"error", err.Error(),
		)
		return "", fmt.Errorf("持久化小程序码失败: %w", err)
	}

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
	scene := fmt.Sprintf("q=%s", code)
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

	fileName := fmt.Sprintf("scale_%s.png", code)
	qrCodeURL, err := s.persistQRCode(ctx, fileName, qrCodeData)
	if err != nil {
		l.Errorw("持久化小程序码失败",
			"action", "generate_scale_qrcode",
			"code", code,
			"error", err.Error(),
		)
		return "", fmt.Errorf("持久化小程序码失败: %w", err)
	}

	l.Infow("量表小程序码生成成功",
		"action", "generate_scale_qrcode",
		"code", code,
		"qrcode_url", qrCodeURL,
		"size", len(qrCodeData),
	)

	return qrCodeURL, nil
}

// GenerateAssessmentEntryQRCode 生成测评入口小程序码
func (s *service) GenerateAssessmentEntryQRCode(ctx context.Context, token string) (string, error) {
	l := logger.L(ctx)

	if token == "" {
		return "", fmt.Errorf("测评入口 token 不能为空")
	}
	if s.qrCodeGen == nil {
		return "", fmt.Errorf("小程序码生成器未配置")
	}

	appID, appSecret, err := s.getWechatAppConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("获取微信应用配置失败: %w", err)
	}

	scene := token
	if len(scene) > 32 {
		return "", fmt.Errorf("测评入口 token 超过 scene 长度限制")
	}

	reader, err := s.qrCodeGen.GenerateUnlimitedQRCode(
		ctx,
		appID,
		appSecret,
		scene,
		s.config.PagePath,
		430,
		false,
		nil,
		false,
	)
	if err != nil {
		l.Errorw("生成测评入口小程序码失败",
			"action", "generate_assessment_entry_qrcode",
			"token", token,
			"error", err.Error(),
		)
		return "", fmt.Errorf("生成小程序码失败: %w", err)
	}

	qrCodeData, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("读取小程序码数据失败: %w", err)
	}

	qrCodeURL, err := s.persistQRCode(ctx, fmt.Sprintf("assessment_entry_%s.png", token), qrCodeData)
	if err != nil {
		return "", fmt.Errorf("持久化小程序码失败: %w", err)
	}

	return qrCodeURL, nil
}

func (s *service) persistQRCode(ctx context.Context, fileName string, data []byte) (string, error) {
	if s.imageStore == nil {
		return "", fmt.Errorf("二维码存储未配置")
	}
	return s.imageStore.StorePNG(ctx, fileName, data)
}
