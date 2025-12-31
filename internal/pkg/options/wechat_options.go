package options

import (
	"fmt"

	"github.com/spf13/pflag"
)

// WeChatOptions 微信小程序配置选项
type WeChatOptions struct {
	// WeChatAppID IAM 中的 wechatappId，用于查询微信应用信息
	WeChatAppID string `json:"wechat_app_id" mapstructure:"wechat-app-id"`

	// PagePath 小程序页面路径，例如 "pages/questionnaire/index"
	PagePath string `json:"page_path" mapstructure:"page-path"`

	// 降级配置（如果 IAM 未启用时使用）
	AppID     string `json:"app_id,omitempty"     mapstructure:"app-id"`     // 小程序 AppID（直接配置）
	AppSecret string `json:"app_secret,omitempty" mapstructure:"app-secret"` // 小程序 AppSecret（直接配置）
}

// NewWeChatOptions 创建默认的微信配置
func NewWeChatOptions() *WeChatOptions {
	return &WeChatOptions{
		WeChatAppID: "",
		PagePath:    "pages/questionnaire/index",
		AppID:       "",
		AppSecret:   "",
	}
}

// Validate 验证配置
func (o *WeChatOptions) Validate() []error {
	errs := []error{}

	// 如果配置了 WeChatAppID，则必须配置 PagePath
	if o.WeChatAppID != "" && o.PagePath == "" {
		errs = append(errs, fmt.Errorf("wechat.page-path is required when wechat.wechat-app-id is set"))
	}

	// 如果未配置 WeChatAppID，则必须配置 AppID 和 AppSecret（降级模式）
	if o.WeChatAppID == "" {
		if o.AppID == "" || o.AppSecret == "" {
			// 不强制要求，允许完全禁用微信功能
		}
	}

	return errs
}

// AddFlags 添加命令行参数
func (o *WeChatOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.WeChatAppID, "wechat.wechat-app-id", o.WeChatAppID, "IAM 中的 wechatappId，用于查询微信应用信息")
	fs.StringVar(&o.PagePath, "wechat.page-path", o.PagePath, "小程序页面路径，例如 pages/questionnaire/index")
	fs.StringVar(&o.AppID, "wechat.app-id", o.AppID, "小程序 AppID（直接配置，降级模式）")
	fs.StringVar(&o.AppSecret, "wechat.app-secret", o.AppSecret, "小程序 AppSecret（直接配置，降级模式）")
}
