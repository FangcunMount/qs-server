package port

import (
	"context"

	"github.com/yshujie/questionnaire-scale/internal/apiserver/domain/wechat"
)

// AppRepository 微信应用仓储接口（出站端口）
type AppRepository interface {
	// Save 保存微信应用
	Save(ctx context.Context, app *wechat.App) error

	// Update 更新微信应用
	Update(ctx context.Context, app *wechat.App) error

	// FindByID 根据ID查找微信应用
	FindByID(ctx context.Context, id wechat.AppID) (*wechat.App, error)

	// FindByAppID 根据微信AppID查找应用
	FindByAppID(ctx context.Context, appID string) (*wechat.App, error)

	// FindByPlatformAndAppID 根据平台和AppID查找微信应用
	FindByPlatformAndAppID(ctx context.Context, platform wechat.Platform, appID string) (*wechat.App, error)

	// FindAllEnabled 查找所有启用的微信应用
	FindAllEnabled(ctx context.Context, env wechat.Environment) ([]*wechat.App, error)

	// Delete 删除微信应用
	Delete(ctx context.Context, id wechat.AppID) error

	// ExistsByAppID 检查AppID是否存在
	ExistsByAppID(ctx context.Context, platform wechat.Platform, appID string) (bool, error)
}
