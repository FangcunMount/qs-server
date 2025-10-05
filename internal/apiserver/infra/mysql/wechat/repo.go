package wechat

import (
	"context"

	"gorm.io/gorm"

	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/wechat"
	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/wechat/port"
	"github.com/fangcun-mount/qs-server/internal/apiserver/infra/mysql"
	"github.com/fangcun-mount/qs-server/internal/pkg/code"
	pkgerrors "github.com/fangcun-mount/qs-server/pkg/errors"
)

// Repository 微信应用存储库实现
type Repository struct {
	mysql.BaseRepository[*AppPO]
	mapper *AppMapper
}

// NewRepository 创建微信应用存储库
func NewRepository(db *gorm.DB) port.AppRepository {
	return &Repository{
		BaseRepository: mysql.NewBaseRepository[*AppPO](db),
		mapper:         NewAppMapper(),
	}
}

// Save 保存微信应用
func (r *Repository) Save(ctx context.Context, app *wechat.App) error {
	po := r.mapper.ToPO(app)
	return r.CreateAndSync(ctx, po, func(saved *AppPO) {
		app.SetID(wechat.NewAppID(saved.ID))
		app.SetCreatedAt(saved.CreatedAt)
		app.SetUpdatedAt(saved.UpdatedAt)
	})
}

// Update 更新微信应用
func (r *Repository) Update(ctx context.Context, app *wechat.App) error {
	po := r.mapper.ToPO(app)
	return r.UpdateAndSync(ctx, po, func(saved *AppPO) {
		app.SetUpdatedAt(saved.UpdatedAt)
	})
}

// Remove 删除微信应用
func (r *Repository) Remove(ctx context.Context, id wechat.AppID) error {
	return r.DeleteByID(ctx, id.Value())
}

// Delete 删除微信应用（实现port接口）
func (r *Repository) Delete(ctx context.Context, id wechat.AppID) error {
	return r.Remove(ctx, id)
}

// FindByID 根据ID查询微信应用
func (r *Repository) FindByID(ctx context.Context, id wechat.AppID) (*wechat.App, error) {
	po, err := r.BaseRepository.FindByID(ctx, id.Value())
	if err != nil {
		return nil, pkgerrors.WithCode(code.ErrDatabase, "app not found")
	}
	return r.mapper.ToDomain(po), nil
}

// FindByAppID 根据AppID查找微信应用
func (r *Repository) FindByAppID(ctx context.Context, appID string) (*wechat.App, error) {
	var po AppPO
	err := r.DB().WithContext(ctx).
		Where("app_id = ?", appID).
		First(&po).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, pkgerrors.WithCode(code.ErrDatabase, "app not found")
		}
		return nil, pkgerrors.WithCode(code.ErrDatabase, "failed to find app: %v", err)
	}

	return r.mapper.ToDomain(&po), nil
}

// FindByPlatformAndAppID 根据平台和AppID查询
func (r *Repository) FindByPlatformAndAppID(ctx context.Context, platform wechat.Platform, appID string) (*wechat.App, error) {
	var po AppPO
	err := r.DB().WithContext(ctx).
		Where("platform = ? AND app_id = ?", string(platform), appID).
		First(&po).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, pkgerrors.WithCode(code.ErrDatabase, "app not found")
		}
		return nil, pkgerrors.WithCode(code.ErrDatabase, "failed to find app: %v", err)
	}

	return r.mapper.ToDomain(&po), nil
}

// FindAllEnabled 查询所有启用的应用
func (r *Repository) FindAllEnabled(ctx context.Context, env wechat.Environment) ([]*wechat.App, error) {
	var pos []*AppPO
	query := r.DB().WithContext(ctx).Where("is_enabled = ?", true)

	// 如果指定了环境，添加环境过滤
	if env != "" {
		query = query.Where("env = ?", string(env))
	}

	err := query.Find(&pos).Error
	if err != nil {
		return nil, pkgerrors.WithCode(code.ErrDatabase, "failed to find apps: %v", err)
	}

	apps := make([]*wechat.App, 0, len(pos))
	for _, po := range pos {
		apps = append(apps, r.mapper.ToDomain(po))
	}

	return apps, nil
}

// ExistsByAppID 检查AppID是否存在
func (r *Repository) ExistsByAppID(ctx context.Context, platform wechat.Platform, appID string) (bool, error) {
	var count int64
	err := r.DB().WithContext(ctx).
		Model(&AppPO{}).
		Where("platform = ? AND app_id = ?", string(platform), appID).
		Count(&count).Error

	if err != nil {
		return false, pkgerrors.WithCode(code.ErrDatabase, "failed to check app existence: %v", err)
	}

	return count > 0, nil
}
