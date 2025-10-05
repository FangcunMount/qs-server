package account

import (
	"context"

	"gorm.io/gorm"

	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/user"
	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/user/account"
	"github.com/fangcun-mount/qs-server/internal/apiserver/domain/user/account/port"
	"github.com/fangcun-mount/qs-server/internal/apiserver/infra/mysql"
	"github.com/fangcun-mount/qs-server/internal/pkg/code"
	pkgerrors "github.com/fangcun-mount/qs-server/pkg/errors"
)

// WechatAccountRepository 微信账户存储库实现
type WechatAccountRepository struct {
	mysql.BaseRepository[*WechatAccountPO]
	mapper *WechatAccountMapper
}

// NewWechatAccountRepository 创建微信账户存储库
func NewWechatAccountRepository(db *gorm.DB) port.WechatAccountRepository {
	return &WechatAccountRepository{
		BaseRepository: mysql.NewBaseRepository[*WechatAccountPO](db),
		mapper:         NewWechatAccountMapper(),
	}
}

// Save 保存微信账户
func (r *WechatAccountRepository) Save(ctx context.Context, acc account.Account) error {
	wxAcc, ok := acc.(*account.WechatAccount)
	if !ok {
		return pkgerrors.WithCode(code.ErrValidation, "invalid account type")
	}

	po := r.mapper.ToPO(wxAcc)
	return r.CreateAndSync(ctx, po, func(saved *WechatAccountPO) {
		wxAcc.SetID(account.NewAccountID(saved.ID))
		wxAcc.SetCreatedAt(saved.CreatedAt)
		wxAcc.SetUpdatedAt(saved.UpdatedAt)
	})
}

// Update 更新微信账户
func (r *WechatAccountRepository) Update(ctx context.Context, acc account.Account) error {
	wxAcc, ok := acc.(*account.WechatAccount)
	if !ok {
		return pkgerrors.WithCode(code.ErrValidation, "invalid account type")
	}

	po := r.mapper.ToPO(wxAcc)
	return r.UpdateAndSync(ctx, po, func(saved *WechatAccountPO) {
		wxAcc.SetUpdatedAt(saved.UpdatedAt)
	})
}

// FindByID 根据ID查找账户
func (r *WechatAccountRepository) FindByID(ctx context.Context, id account.AccountID) (account.Account, error) {
	po, err := r.BaseRepository.FindByID(ctx, id.Value())
	if err != nil {
		return nil, pkgerrors.WithCode(code.ErrDatabase, "account not found")
	}
	return r.mapper.ToDomain(po), nil
}

// FindByUserID 根据用户ID查找所有账户
func (r *WechatAccountRepository) FindByUserID(ctx context.Context, userID user.UserID) ([]account.Account, error) {
	var pos []*WechatAccountPO
	err := r.DB().WithContext(ctx).
		Where("user_id = ?", userID.Value()).
		Find(&pos).Error

	if err != nil {
		return nil, pkgerrors.WithCode(code.ErrDatabase, "failed to find accounts: %v", err)
	}

	accounts := make([]account.Account, 0, len(pos))
	for _, po := range pos {
		accounts = append(accounts, r.mapper.ToDomain(po))
	}

	return accounts, nil
}

// Delete 删除账户
func (r *WechatAccountRepository) Delete(ctx context.Context, id account.AccountID) error {
	return r.DeleteByID(ctx, id.Value())
}

// FindByOpenID 根据OpenID查找微信账户
func (r *WechatAccountRepository) FindByOpenID(
	ctx context.Context,
	wxAppID string,
	platform account.WxPlatform,
	openID string,
) (*account.WechatAccount, error) {
	var po WechatAccountPO
	err := r.DB().WithContext(ctx).
		Where("wx_app_id = ? AND platform = ? AND open_id = ?", wxAppID, string(platform), openID).
		First(&po).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, pkgerrors.WithCode(code.ErrDatabase, "wechat account not found")
		}
		return nil, pkgerrors.WithCode(code.ErrDatabase, "failed to find wechat account: %v", err)
	}

	return r.mapper.ToDomain(&po), nil
}

// FindByUnionID 根据UnionID查找所有微信账户
func (r *WechatAccountRepository) FindByUnionID(ctx context.Context, unionID string) ([]*account.WechatAccount, error) {
	var pos []*WechatAccountPO
	err := r.DB().WithContext(ctx).
		Where("union_id = ?", unionID).
		Find(&pos).Error

	if err != nil {
		return nil, pkgerrors.WithCode(code.ErrDatabase, "failed to find wechat accounts: %v", err)
	}

	accounts := make([]*account.WechatAccount, 0, len(pos))
	for _, po := range pos {
		accounts = append(accounts, r.mapper.ToDomain(po))
	}

	return accounts, nil
}

// FindBoundAccountByUnionID 根据UnionID查找已绑定用户的微信账户
func (r *WechatAccountRepository) FindBoundAccountByUnionID(ctx context.Context, unionID string) (*account.WechatAccount, error) {
	var po WechatAccountPO
	err := r.DB().WithContext(ctx).
		Where("union_id = ? AND user_id IS NOT NULL", unionID).
		First(&po).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil // 未找到已绑定的账户，返回nil而不是错误
		}
		return nil, pkgerrors.WithCode(code.ErrDatabase, "failed to find bound wechat account: %v", err)
	}

	return r.mapper.ToDomain(&po), nil
}

// ExistsByOpenID 检查OpenID是否存在
func (r *WechatAccountRepository) ExistsByOpenID(
	ctx context.Context,
	wxAppID string,
	platform account.WxPlatform,
	openID string,
) (bool, error) {
	var count int64
	err := r.DB().WithContext(ctx).
		Model(&WechatAccountPO{}).
		Where("wx_app_id = ? AND platform = ? AND open_id = ?", wxAppID, string(platform), openID).
		Count(&count).Error

	if err != nil {
		return false, pkgerrors.WithCode(code.ErrDatabase, "failed to check wechat account existence: %v", err)
	}

	return count > 0, nil
}
