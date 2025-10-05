package wechat

import (
	"time"

	"github.com/fangcun-mount/qs-server/internal/pkg/code"
	"github.com/fangcun-mount/qs-server/pkg/errors"
)

// Platform 微信平台类型
type Platform string

const (
	PlatformMini Platform = "mini" // 小程序
	PlatformOA   Platform = "oa"   // 公众号
)

// Environment 环境类型
type Environment string

const (
	EnvProd Environment = "prod"
	EnvTest Environment = "test"
	EnvDev  Environment = "dev"
)

// App 微信应用聚合根
type App struct {
	id             AppID
	name           string
	platform       Platform
	appID          string // 微信AppID
	secret         string
	token          string
	encodingAESKey string
	mchID          string
	serialNo       string
	payCertID      *uint64
	isEnabled      bool
	env            Environment
	remark         string
	createdAt      time.Time
	updatedAt      time.Time
}

// NewApp 创建新的微信应用
func NewApp(
	name string,
	platform Platform,
	appID string,
	secret string,
	env Environment,
) (*App, error) {
	if name == "" {
		return nil, errors.WithCode(code.ErrValidation, "name cannot be empty")
	}
	if appID == "" {
		return nil, errors.WithCode(code.ErrValidation, "appID cannot be empty")
	}
	if platform != PlatformMini && platform != PlatformOA {
		return nil, errors.WithCode(code.ErrValidation, "invalid platform")
	}

	return &App{
		name:      name,
		platform:  platform,
		appID:     appID,
		secret:    secret,
		isEnabled: true,
		env:       env,
		createdAt: time.Now(),
		updatedAt: time.Now(),
	}, nil
}

// Reconstitute 从持久化数据重建微信应用聚合根
func Reconstitute(
	id AppID,
	name string,
	platform Platform,
	appID, secret, token, encodingAESKey string,
	mchID, serialNo string,
	payCertID *uint64,
	isEnabled bool,
	env Environment,
	remark string,
	createdAt, updatedAt time.Time,
) *App {
	return &App{
		id:             id,
		name:           name,
		platform:       platform,
		appID:          appID,
		secret:         secret,
		token:          token,
		encodingAESKey: encodingAESKey,
		mchID:          mchID,
		serialNo:       serialNo,
		payCertID:      payCertID,
		isEnabled:      isEnabled,
		env:            env,
		remark:         remark,
		createdAt:      createdAt,
		updatedAt:      updatedAt,
	}
}

// Getters
func (a *App) ID() AppID              { return a.id }
func (a *App) Name() string           { return a.name }
func (a *App) Platform() Platform     { return a.platform }
func (a *App) AppID() string          { return a.appID }
func (a *App) Secret() string         { return a.secret }
func (a *App) Token() string          { return a.token }
func (a *App) EncodingAESKey() string { return a.encodingAESKey }
func (a *App) MchID() string          { return a.mchID }
func (a *App) SerialNo() string       { return a.serialNo }
func (a *App) PayCertID() *uint64     { return a.payCertID }
func (a *App) IsEnabled() bool        { return a.isEnabled }
func (a *App) Env() Environment       { return a.env }
func (a *App) Remark() string         { return a.remark }
func (a *App) CreatedAt() time.Time   { return a.createdAt }
func (a *App) UpdatedAt() time.Time   { return a.updatedAt }

// Setters for repository
func (a *App) SetID(id AppID)           { a.id = id }
func (a *App) SetCreatedAt(t time.Time) { a.createdAt = t }
func (a *App) SetUpdatedAt(t time.Time) { a.updatedAt = t }

// UpdateSecret 更新应用密钥
func (a *App) UpdateSecret(secret string) error {
	if secret == "" {
		return errors.WithCode(code.ErrValidation, "secret cannot be empty")
	}
	a.secret = secret
	a.updatedAt = time.Now()
	return nil
}

// UpdateServerConfig 更新服务器配置（OA用）
func (a *App) UpdateServerConfig(token, encodingAESKey string) error {
	if a.platform != PlatformOA {
		return errors.WithCode(code.ErrValidation, "server config only for OA platform")
	}
	a.token = token
	a.encodingAESKey = encodingAESKey
	a.updatedAt = time.Now()
	return nil
}

// UpdatePaymentConfig 更新支付配置
func (a *App) UpdatePaymentConfig(mchID, serialNo string, payCertID *uint64) {
	a.mchID = mchID
	a.serialNo = serialNo
	a.payCertID = payCertID
	a.updatedAt = time.Now()
}

// Enable 启用应用
func (a *App) Enable() {
	a.isEnabled = true
	a.updatedAt = time.Now()
}

// Disable 禁用应用
func (a *App) Disable() {
	a.isEnabled = false
	a.updatedAt = time.Now()
}

// UpdateRemark 更新备注
func (a *App) UpdateRemark(remark string) {
	a.remark = remark
	a.updatedAt = time.Now()
}

// IsMiniProgram 是否为小程序
func (a *App) IsMiniProgram() bool {
	return a.platform == PlatformMini
}

// IsOfficialAccount 是否为公众号
func (a *App) IsOfficialAccount() bool {
	return a.platform == PlatformOA
}
