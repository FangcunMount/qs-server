package port

import "context"

// WechatSDK 微信SDK接口（出站端口 - 防腐层）
// 用于隔离第三方微信SDK，便于测试和替换
type WechatSDK interface {
	// Code2Session 小程序code换session
	Code2Session(ctx context.Context, appID, jsCode string) (openID, sessionKey, unionID string, err error)

	// DecryptPhoneNumber 解密小程序手机号
	DecryptPhoneNumber(ctx context.Context, appID, sessionKey, encryptedData, iv string) (phone string, err error)

	// GetUserInfo 获取公众号用户信息
	GetUserInfo(ctx context.Context, appID, openID string) (nickname, avatar, unionID string, err error)

	// SendSubscribeMessage 发送小程序订阅消息
	SendSubscribeMessage(ctx context.Context, appID, openID, templateID string, data map[string]interface{}) error

	// SendTemplateMessage 发送公众号模板消息
	SendTemplateMessage(ctx context.Context, appID, openID, templateID string, data map[string]interface{}) error
}
